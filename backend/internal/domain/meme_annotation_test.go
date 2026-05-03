package domain

import (
	"testing"

	pb "github.com/timmy/emomo/gen/emomo/v1"
)

func TestTextPresenceFromOCRText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		ocrText      string
		wantPresence pb.TextPresence
		wantCount    int
	}{
		{
			name:         "with text",
			ocrText:      "你 好",
			wantPresence: pb.TextPresence_TEXT_PRESENCE_WITH_TEXT,
			wantCount:    2,
		},
		{
			name:         "empty text",
			ocrText:      "  \n\t ",
			wantPresence: pb.TextPresence_TEXT_PRESENCE_WITHOUT_TEXT,
			wantCount:    0,
		},
		{
			name:         "explicit no text marker",
			ocrText:      "无文字",
			wantPresence: pb.TextPresence_TEXT_PRESENCE_WITHOUT_TEXT,
			wantCount:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotPresence, gotCount := TextPresenceFromOCRText(tt.ocrText)
			if gotPresence != tt.wantPresence {
				t.Fatalf("TextPresenceFromOCRText(%q) presence = %v, want %v", tt.ocrText, gotPresence, tt.wantPresence)
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
