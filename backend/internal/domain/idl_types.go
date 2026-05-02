// Package domain hosts the application's pure-Go data model.
//
// We intentionally do NOT reuse protobuf generated structs (idl.ImageInfo,
// idl.MemeAnnotationLabels, idl.TextLabel ...) inside domain values because
// every protobuf message embeds protoimpl.MessageState, which contains a
// pragma.DoNotCopy / sync.Mutex pair. Embedding those by value (via type
// aliasing or struct literals) makes every GORM Find/range/append/assignment
// copy a lock — which go vet rightly flags as "copies lock value", and which
// the protobuf-go authors explicitly call out as undefined behaviour
// (atomics + lazy reflection state must not be shallow copied).
//
// Instead, the domain layer uses plain Go structs that mirror the protobuf
// schema's *value semantics*. Conversion to/from the wire IDL types happens
// at boundaries via ToProto / *FromProto helpers below. ImageFormat and
// MemeVectorType remain as type aliases of the generated enums — those are
// plain int32 values with no embedded mutex and are safe to share.
package domain

import (
	"database/sql/driver"
	"encoding/json"
	"errors"

	idl "github.com/timmy/emomo/internal/idl/emomo/v1"
)

// ImageFormat is an alias of the protobuf-generated ImageFormat enum.
// Protobuf enums are int32 values without any mutex/state, so sharing the
// type is safe and avoids needless string<->int conversion at boundaries.
type ImageFormat = idl.ImageFormat

const (
	ImageFormatUnspecified = idl.ImageFormat_IMAGE_FORMAT_UNSPECIFIED
	ImageFormatJPEG        = idl.ImageFormat_IMAGE_FORMAT_JPEG
	ImageFormatPNG         = idl.ImageFormat_IMAGE_FORMAT_PNG
	ImageFormatWebP        = idl.ImageFormat_IMAGE_FORMAT_WEBP
)

// MemeVectorType is an alias of the protobuf-generated VectorType enum.
type MemeVectorType = idl.VectorType

const (
	MemeVectorTypeUnspecified = idl.VectorType_VECTOR_TYPE_UNSPECIFIED
	MemeVectorTypeImage       = idl.VectorType_VECTOR_TYPE_IMAGE
	MemeVectorTypeCaption     = idl.VectorType_VECTOR_TYPE_CAPTION
)

// ImageInfo stores intrinsic image properties as a single structured value.
// It is a pure Go struct — safe to copy, range over, embed by value, etc. —
// and is JSON-compatible with the wire format produced by ImageInfoFromProto
// for backwards compatibility with rows previously written by protojson.
type ImageInfo struct {
	Width  int32       `json:"width"`
	Height int32       `json:"height"`
	Format ImageFormat `json:"format"`
}

// Value implements driver.Valuer so GORM can persist ImageInfo as a JSON TEXT.
// The encoding is the same shape protojson produced with UseEnumNumbers=true:
// {"width":W,"height":H,"format":N} where N is the ImageFormat enum number.
func (i ImageInfo) Value() (driver.Value, error) {
	b, err := json.Marshal(i)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

// Scan implements sql.Scanner so GORM can hydrate ImageInfo from a JSON TEXT.
// It accepts both bytes and string forms and tolerates empty / NULL values.
func (i *ImageInfo) Scan(value any) error {
	if value == nil {
		*i = ImageInfo{}
		return nil
	}
	bytes, err := scanJSONBytes(value)
	if err != nil {
		return err
	}
	if len(bytes) == 0 {
		*i = ImageInfo{}
		return nil
	}
	return json.Unmarshal(bytes, i)
}

// ToProto converts the domain value to its protobuf wire form. Use at API
// / RPC boundaries; never assign the result back into a domain field.
func (i ImageInfo) ToProto() *idl.ImageInfo {
	return &idl.ImageInfo{
		Width:  i.Width,
		Height: i.Height,
		Format: i.Format,
	}
}

// ImageInfoFromProto converts a protobuf message to the domain value.
// Nil messages produce a zero-value ImageInfo.
func ImageInfoFromProto(pb *idl.ImageInfo) ImageInfo {
	if pb == nil {
		return ImageInfo{}
	}
	return ImageInfo{
		Width:  pb.GetWidth(),
		Height: pb.GetHeight(),
		Format: pb.GetFormat(),
	}
}

// TextLabel mirrors idl.TextLabel as a pure Go struct so it can be embedded
// in AnnotationLabels and constructed in callers without copying a mutex.
type TextLabel struct {
	Present bool `json:"present"`
}

// AnnotationLabels stores structured labels produced by the analyzer.
// It is a pure Go struct; the Text sub-label is a pointer so absence and
// "present=false" can be distinguished, matching protobuf message semantics.
type AnnotationLabels struct {
	Text *TextLabel `json:"text,omitempty"`
}

// Value implements driver.Valuer. The encoding is JSON-compatible with what
// protojson previously produced: '{}' when no labels are set, otherwise
// '{"text":{"present":true|false}}'.
func (l AnnotationLabels) Value() (driver.Value, error) {
	b, err := json.Marshal(l)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

// Scan implements sql.Scanner. It tolerates the historical protojson outputs
// '{"text":null}' (treated as no label) and '{}' (also no label).
func (l *AnnotationLabels) Scan(value any) error {
	*l = AnnotationLabels{}
	if value == nil {
		return nil
	}
	bytes, err := scanJSONBytes(value)
	if err != nil {
		return err
	}
	if len(bytes) == 0 {
		return nil
	}
	return json.Unmarshal(bytes, l)
}

// HasText returns true when OCR has reliably found visible text in the image.
func (l AnnotationLabels) HasText() bool {
	return l.Text != nil && l.Text.Present
}

// ToProto converts the domain value to its protobuf wire form.
func (l AnnotationLabels) ToProto() *idl.MemeAnnotationLabels {
	pb := &idl.MemeAnnotationLabels{}
	if l.Text != nil {
		pb.Text = &idl.TextLabel{Present: l.Text.Present}
	}
	return pb
}

// AnnotationLabelsFromProto converts a protobuf message to the domain value.
// Nil messages produce a zero-value AnnotationLabels.
func AnnotationLabelsFromProto(pb *idl.MemeAnnotationLabels) AnnotationLabels {
	if pb == nil {
		return AnnotationLabels{}
	}
	out := AnnotationLabels{}
	if t := pb.GetText(); t != nil {
		out.Text = &TextLabel{Present: t.GetPresent()}
	}
	return out
}

// TextPresenceFromLabels classifies the structured analyzer labels into the
// search-facing TextPresence enum used by Qdrant payloads and SQL filters.
func TextPresenceFromLabels(labels AnnotationLabels) TextPresence {
	if labels.Text == nil {
		return TextPresenceUnknown
	}
	if labels.Text.Present {
		return TextPresenceWithText
	}
	return TextPresenceWithoutText
}

// ImageFormatFromString maps a lowercase format identifier (as detected by
// magic bytes during ingestion) to the corresponding protobuf enum value.
func ImageFormatFromString(format string) ImageFormat {
	switch format {
	case "jpg", "jpeg":
		return ImageFormatJPEG
	case "png":
		return ImageFormatPNG
	case "webp":
		return ImageFormatWebP
	default:
		return ImageFormatUnspecified
	}
}

// ImageFormatToString maps an ImageFormat enum back to a stable lowercase
// identifier suitable for storage URLs and HTTP content types.
func ImageFormatToString(format ImageFormat) string {
	switch format {
	case ImageFormatJPEG:
		return "jpeg"
	case ImageFormatPNG:
		return "png"
	case ImageFormatWebP:
		return "webp"
	default:
		return "unknown"
	}
}

// ParseMemeVectorType parses a free-form string (e.g. from CLI flags or
// configuration) into the canonical MemeVectorType enum, defaulting to image.
func ParseMemeVectorType(value string) MemeVectorType {
	switch value {
	case "caption":
		return MemeVectorTypeCaption
	default:
		return MemeVectorTypeImage
	}
}

// MemeVectorTypeToString stringifies a MemeVectorType into its short slug.
func MemeVectorTypeToString(vectorType MemeVectorType) string {
	switch vectorType {
	case MemeVectorTypeCaption:
		return "caption"
	case MemeVectorTypeImage:
		return "image"
	default:
		return "unspecified"
	}
}

func scanJSONBytes(value any) ([]byte, error) {
	switch v := value.(type) {
	case []byte:
		return v, nil
	case string:
		return []byte(v), nil
	case json.RawMessage:
		return []byte(v), nil
	default:
		return nil, errors.New("failed to scan JSON value")
	}
}
