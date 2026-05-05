package persistence

import (
	"fmt"
	"strings"

	pb "github.com/timmy/emomo/gen/emomo/v1"
)

// TextPresenceToString maps the TextPresence enum to the stable lowercase
// keyword used inside Qdrant payloads and as filter values. The mapping must
// stay byte-stable: existing rows depend on these exact strings.
//
// Returns "" for TEXT_PRESENCE_UNSPECIFIED, signalling "no filter / no value".
func TextPresenceToString(p pb.TextPresence) string {
	switch p {
	case pb.TextPresence_TEXT_PRESENCE_WITH_TEXT:
		return "with_text"
	case pb.TextPresence_TEXT_PRESENCE_WITHOUT_TEXT:
		return "without_text"
	case pb.TextPresence_TEXT_PRESENCE_UNKNOWN:
		return "unknown"
	default:
		return ""
	}
}

// TextPresenceFromString parses a Qdrant-payload keyword back into the enum.
// Empty input maps to UNSPECIFIED; unrecognized inputs are treated the same
// way (no error) so legacy / partial payloads do not break read paths.
func TextPresenceFromString(s string) pb.TextPresence {
	switch s {
	case "with_text":
		return pb.TextPresence_TEXT_PRESENCE_WITH_TEXT
	case "without_text":
		return pb.TextPresence_TEXT_PRESENCE_WITHOUT_TEXT
	case "unknown":
		return pb.TextPresence_TEXT_PRESENCE_UNKNOWN
	default:
		return pb.TextPresence_TEXT_PRESENCE_UNSPECIFIED
	}
}

// ImageFormatFromExt maps a lowercase file extension (jpg/jpeg/png/webp) to
// the protobuf enum. Returns IMAGE_FORMAT_UNSPECIFIED for empty or unknown
// inputs; callers that need to reject unknowns should compare against
// UNSPECIFIED themselves.
func ImageFormatFromExt(ext string) pb.ImageFormat {
	switch strings.ToLower(strings.TrimPrefix(ext, ".")) {
	case "jpg", "jpeg":
		return pb.ImageFormat_IMAGE_FORMAT_JPEG
	case "png":
		return pb.ImageFormat_IMAGE_FORMAT_PNG
	case "webp":
		return pb.ImageFormat_IMAGE_FORMAT_WEBP
	default:
		return pb.ImageFormat_IMAGE_FORMAT_UNSPECIFIED
	}
}

// ImageFormatToExt maps an ImageFormat enum to a stable lowercase extension
// suitable for storage URLs and HTTP Content-Type derivations. Returns
// "unknown" for UNSPECIFIED so callers always have a non-empty placeholder.
func ImageFormatToExt(f pb.ImageFormat) string {
	switch f {
	case pb.ImageFormat_IMAGE_FORMAT_JPEG:
		return "jpeg"
	case pb.ImageFormat_IMAGE_FORMAT_PNG:
		return "png"
	case pb.ImageFormat_IMAGE_FORMAT_WEBP:
		return "webp"
	default:
		return "unknown"
	}
}

// VectorTypeShortName returns the canonical lowercase slug ("image"/"caption")
// emomo uses on CLI flags, log fields, and Qdrant collection composite keys.
// Falls back to "unspecified" for the zero enum.
func VectorTypeShortName(v pb.VectorType) string {
	switch v {
	case pb.VectorType_VECTOR_TYPE_IMAGE:
		return "image"
	case pb.VectorType_VECTOR_TYPE_CAPTION:
		return "caption"
	default:
		return "unspecified"
	}
}

// ParseVectorType parses a free-form CLI / config string into the enum. It
// accepts both the short slug ("image", "caption") and the protobuf-style
// fully-qualified name ("VECTOR_TYPE_IMAGE"). Returns an error on unknown
// inputs so callers can distinguish "not provided" from "garbled" — the old
// bug where ParseMemeVectorType silently fell back to image is intentionally
// fixed by this stricter contract.
func ParseVectorType(value string) (pb.VectorType, error) {
	if value == "" {
		return pb.VectorType_VECTOR_TYPE_UNSPECIFIED, nil
	}
	upper := strings.ToUpper(value)
	if !strings.HasPrefix(upper, "VECTOR_TYPE_") {
		upper = "VECTOR_TYPE_" + upper
	}
	if v, ok := pb.VectorType_value[upper]; ok {
		return pb.VectorType(v), nil
	}
	return pb.VectorType_VECTOR_TYPE_UNSPECIFIED, fmt.Errorf("unknown vector type %q", value)
}

// NormalizeVectorType promotes the zero enum to VECTOR_TYPE_IMAGE, matching
// the historical default before the column became NOT NULL with default 1.
// Used in repository queries and ingest defaults.
func NormalizeVectorType(v pb.VectorType) pb.VectorType {
	if v == pb.VectorType_VECTOR_TYPE_UNSPECIFIED {
		return pb.VectorType_VECTOR_TYPE_IMAGE
	}
	return v
}
