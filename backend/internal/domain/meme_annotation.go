package domain

import (
	"strings"
	"time"
)

// TextPresence represents whether OCR found visible text in a meme image.
type TextPresence string

const (
	TextPresenceUnknown     TextPresence = "unknown"
	TextPresenceWithText    TextPresence = "with_text"
	TextPresenceWithoutText TextPresence = "without_text"
)

// MemeAnnotation stores optional VLM/OCR output and structured analyzer labels for a meme.
type MemeAnnotation struct {
	ID            string           `gorm:"type:text;primaryKey" json:"id"`
	MemeID        string           `gorm:"type:text;not null;uniqueIndex:idx_meme_annotations_meme_model;index:idx_meme_annotations_meme" json:"meme_id"`
	AnalyzerModel string           `gorm:"type:text;not null;uniqueIndex:idx_meme_annotations_meme_model" json:"analyzer_model"`
	Description   string           `gorm:"type:text" json:"description"`
	OCRText       string           `gorm:"type:text" json:"ocr_text"`
	Labels        AnnotationLabels `gorm:"type:text;not null" json:"labels"`
	CreatedAt     time.Time        `json:"created_at"`
	UpdatedAt     time.Time        `json:"updated_at"`
}

// TableName returns the database table name for MemeAnnotation.
func (MemeAnnotation) TableName() string {
	return "meme_annotations"
}

// TextPresenceFromOCRText classifies normalized OCR text into a filterable state.
func TextPresenceFromOCRText(text string) (TextPresence, int) {
	normalized := normalizeOCRPresenceText(text)
	if normalized == "" {
		return TextPresenceWithoutText, 0
	}
	count := 0
	for _, r := range normalized {
		if !isWhitespaceRune(r) {
			count++
		}
	}
	return TextPresenceWithText, count
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
