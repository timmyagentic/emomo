package domain

import (
	"strings"
	"time"

	pb "github.com/timmy/emomo/gen/emomo/v1"
)

// MemeAnnotation stores VLM/OCR output and structured analyzer labels for a
// meme + analyzer model pair. The labels JSON column is typed as a generated
// protobuf message pointer so the on-disk shape stays in lock-step with
// proto/emomo/v1/types.proto.
type MemeAnnotation struct {
	ID            string                   `gorm:"type:text;primaryKey" json:"id"`
	MemeID        string                   `gorm:"type:text;not null;uniqueIndex:idx_meme_annotations_meme_model;index:idx_meme_annotations_meme" json:"meme_id"`
	AnalyzerModel string                   `gorm:"type:text;not null;uniqueIndex:idx_meme_annotations_meme_model" json:"analyzer_model"`
	Description   string                   `gorm:"type:text" json:"description"`
	OCRText       string                   `gorm:"type:text" json:"ocr_text"`
	Labels        *pb.MemeAnnotationLabels `gorm:"type:text;not null;serializer:protojson" json:"labels"`
	CreatedAt     time.Time                `json:"created_at"`
	UpdatedAt     time.Time                `json:"updated_at"`
}

// TableName returns the database table name for MemeAnnotation.
func (MemeAnnotation) TableName() string {
	return "meme_annotations"
}

// HasText reports whether the annotation's structured labels reliably mark
// visible text as present. Used by ingest / search filters.
func (m *MemeAnnotation) HasText() bool {
	if m == nil || m.Labels == nil {
		return false
	}
	return m.Labels.GetText().GetPresent()
}

// TextPresenceFromOCRText classifies normalized OCR text into the protobuf
// enum + a non-whitespace character count. Empty or sentinel inputs ("无文字"
// etc.) yield WITHOUT_TEXT/0; any non-trivial content yields WITH_TEXT plus
// the count of meaningful characters (used to score reliability).
func TextPresenceFromOCRText(text string) (pb.TextPresence, int) {
	normalized := normalizeOCRPresenceText(text)
	if normalized == "" {
		return pb.TextPresence_TEXT_PRESENCE_WITHOUT_TEXT, 0
	}
	count := 0
	for _, r := range normalized {
		if !isWhitespaceRune(r) {
			count++
		}
	}
	return pb.TextPresence_TEXT_PRESENCE_WITH_TEXT, count
}

// TextPresenceFromLabels reads structured analyzer labels and returns the
// search-facing TextPresence enum. Absence of labels is treated as
// TEXT_PRESENCE_UNKNOWN (the analyzer hasn't classified yet).
func TextPresenceFromLabels(labels *pb.MemeAnnotationLabels) pb.TextPresence {
	if labels == nil || labels.GetText() == nil {
		return pb.TextPresence_TEXT_PRESENCE_UNKNOWN
	}
	if labels.GetText().GetPresent() {
		return pb.TextPresence_TEXT_PRESENCE_WITH_TEXT
	}
	return pb.TextPresence_TEXT_PRESENCE_WITHOUT_TEXT
}

func normalizeOCRPresenceText(text string) string {
	trimmed := strings.TrimSpace(text)
	trimmed = strings.Trim(trimmed, "\"'`")
	trimmed = strings.Trim(trimmed, " .，。;；:：!！?？")
	if trimmed == "" {
		return ""
	}
	switch strings.ToLower(trimmed) {
	case "none", "no text", "no_text", "n/a", "null":
		return ""
	}
	switch trimmed {
	case "无文字", "没有文字", "无内容", "无文本", "无字", "无文字内容":
		return ""
	}
	return strings.Join(strings.Fields(trimmed), " ")
}

func isWhitespaceRune(r rune) bool {
	return r == ' ' || r == '\n' || r == '\t' || r == '\r'
}
