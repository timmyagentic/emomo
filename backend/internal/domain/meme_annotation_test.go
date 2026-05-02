package domain

import "testing"

func TestTextPresenceFromOCRText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		ocrText      string
		wantPresence TextPresence
		wantCount    int
	}{
		{
			name:         "with text",
			ocrText:      "你 好",
			wantPresence: TextPresenceWithText,
			wantCount:    2,
		},
		{
			name:         "empty text",
			ocrText:      "  \n\t ",
			wantPresence: TextPresenceWithoutText,
			wantCount:    0,
		},
		{
			name:         "explicit no text marker",
			ocrText:      "无文字",
			wantPresence: TextPresenceWithoutText,
			wantCount:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotPresence, gotCount := TextPresenceFromOCRText(tt.ocrText)
			if gotPresence != tt.wantPresence {
				t.Fatalf("TextPresenceFromOCRText(%q) presence = %q, want %q", tt.ocrText, gotPresence, tt.wantPresence)
			}
			if gotCount != tt.wantCount {
				t.Fatalf("TextPresenceFromOCRText(%q) count = %d, want %d", tt.ocrText, gotCount, tt.wantCount)
			}
		})
	}
}

func TestMemeAnnotationUsesAnnotationsTable(t *testing.T) {
	t.Parallel()

	if got := (MemeAnnotation{}).TableName(); got != "meme_annotations" {
		t.Fatalf("MemeAnnotation.TableName() = %q, want meme_annotations", got)
	}
}
