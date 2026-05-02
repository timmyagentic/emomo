package domain

import (
	"strings"
	"testing"
)

func TestImageInfoUsesProtobufEnumAndRoundTripsThroughDatabaseValue(t *testing.T) {
	info := ImageInfo{
		Width:  512,
		Height: 384,
		Format: ImageFormatJPEG,
	}

	value, err := info.Value()
	if err != nil {
		t.Fatalf("ImageInfo.Value() error = %v", err)
	}
	encoded, ok := value.(string)
	if !ok {
		t.Fatalf("ImageInfo.Value() type = %T, want string", value)
	}
	if !strings.Contains(encoded, `"format":1`) {
		t.Fatalf("ImageInfo.Value() = %s, want enum stored as number", encoded)
	}

	var decoded ImageInfo
	if err := decoded.Scan(encoded); err != nil {
		t.Fatalf("ImageInfo.Scan() error = %v", err)
	}
	if decoded.Width != info.Width || decoded.Height != info.Height || decoded.Format != info.Format {
		t.Fatalf("decoded ImageInfo = %+v, want %+v", decoded, info)
	}
}

func TestImageInfoScanAcceptsLegacyProtojsonPayload(t *testing.T) {
	// Rows previously written via protojson(UseEnumNumbers=true, EmitUnpopulated=true)
	// look exactly like this; the new encoding/json based Scan must keep loading them.
	legacy := `{"width":256,"height":128,"format":2}`

	var decoded ImageInfo
	if err := decoded.Scan(legacy); err != nil {
		t.Fatalf("ImageInfo.Scan(legacy) error = %v", err)
	}
	if decoded.Width != 256 || decoded.Height != 128 || decoded.Format != ImageFormatPNG {
		t.Fatalf("decoded ImageInfo = %+v, want 256x128 png", decoded)
	}
}

func TestAnnotationLabelsStoresTextPresenceAsNestedBoolean(t *testing.T) {
	labels := AnnotationLabels{
		Text: &TextLabel{Present: true},
	}

	value, err := labels.Value()
	if err != nil {
		t.Fatalf("AnnotationLabels.Value() error = %v", err)
	}
	encoded, ok := value.(string)
	if !ok {
		t.Fatalf("AnnotationLabels.Value() type = %T, want string", value)
	}
	if strings.Contains(encoded, "text_presence") {
		t.Fatalf("AnnotationLabels.Value() = %s, must not duplicate text_presence as a top-level field", encoded)
	}
	if !strings.Contains(encoded, `"text":{"present":true}`) {
		t.Fatalf("AnnotationLabels.Value() = %s, want nested text.present boolean", encoded)
	}

	var decoded AnnotationLabels
	if err := decoded.Scan(encoded); err != nil {
		t.Fatalf("AnnotationLabels.Scan() error = %v", err)
	}
	if decoded.Text == nil || !decoded.Text.Present {
		t.Fatalf("decoded AnnotationLabels = %+v, want text.present=true", decoded)
	}
}

func TestAnnotationLabelsScanAcceptsLegacyShapes(t *testing.T) {
	cases := map[string]struct {
		input        string
		wantHasLabel bool
		wantPresent  bool
	}{
		"empty object":        {input: `{}`, wantHasLabel: false},
		"explicit null text":  {input: `{"text":null}`, wantHasLabel: false},
		"present false":       {input: `{"text":{"present":false}}`, wantHasLabel: true, wantPresent: false},
		"present true":        {input: `{"text":{"present":true}}`, wantHasLabel: true, wantPresent: true},
		"unknown extra field": {input: `{"text":{"present":true},"unknown":42}`, wantHasLabel: true, wantPresent: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var decoded AnnotationLabels
			if err := decoded.Scan(tc.input); err != nil {
				t.Fatalf("Scan(%q) error = %v", tc.input, err)
			}
			if tc.wantHasLabel {
				if decoded.Text == nil {
					t.Fatalf("Scan(%q) text = nil, want non-nil", tc.input)
				}
				if decoded.Text.Present != tc.wantPresent {
					t.Fatalf("Scan(%q) text.present = %v, want %v", tc.input, decoded.Text.Present, tc.wantPresent)
				}
			} else if decoded.Text != nil {
				t.Fatalf("Scan(%q) text = %+v, want nil", tc.input, decoded.Text)
			}
		})
	}
}

func TestVectorTypeIsProtobufEnum(t *testing.T) {
	vectorType := MemeVectorTypeImage
	if int32(vectorType) <= 0 {
		t.Fatalf("MemeVectorType image = %d, want positive protobuf enum value", vectorType)
	}
}

func TestImageInfoToProtoAndFromProtoRoundTrip(t *testing.T) {
	info := ImageInfo{Width: 1, Height: 2, Format: ImageFormatWebP}
	roundTripped := ImageInfoFromProto(info.ToProto())
	if roundTripped != info {
		t.Fatalf("round trip = %+v, want %+v", roundTripped, info)
	}
}

func TestAnnotationLabelsToProtoAndFromProtoRoundTrip(t *testing.T) {
	labels := AnnotationLabels{Text: &TextLabel{Present: true}}
	roundTripped := AnnotationLabelsFromProto(labels.ToProto())
	if roundTripped.Text == nil || !roundTripped.Text.Present {
		t.Fatalf("round trip = %+v, want text.present=true", roundTripped)
	}

	empty := AnnotationLabels{}
	roundTrippedEmpty := AnnotationLabelsFromProto(empty.ToProto())
	if roundTrippedEmpty.Text != nil {
		t.Fatalf("empty round trip = %+v, want nil text", roundTrippedEmpty)
	}
}
