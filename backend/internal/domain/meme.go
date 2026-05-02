package domain

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

// StringArray is a custom type for storing string arrays as JSON in the database.
type StringArray []string

// Value implements the driver.Valuer interface for database serialization.
// Parameters: none.
// Returns:
//   - driver.Value: JSON-encoded string representation of the slice.
//   - error: non-nil if marshaling fails.
func (a StringArray) Value() (driver.Value, error) {
	if a == nil {
		return "[]", nil
	}
	b, err := json.Marshal(a)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

// Scan implements the sql.Scanner interface for database deserialization.
// Parameters:
//   - value: raw database value to decode.
//
// Returns:
//   - error: non-nil if decoding fails or the type is unexpected.
func (a *StringArray) Scan(value interface{}) error {
	if value == nil {
		*a = StringArray{}
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		str, ok := value.(string)
		if !ok {
			return errors.New("failed to scan StringArray")
		}
		bytes = []byte(str)
	}
	return json.Unmarshal(bytes, a)
}

// Meme represents a meme/sticker in the system.
// Fields include stable identifiers, storage location, intrinsic image info, and user-facing labels.
type Meme struct {
	ID          string      `gorm:"type:text;primaryKey" json:"id"`
	StorageKey  string      `gorm:"type:text;not null" json:"storage_key"`
	ContentHash string      `gorm:"type:text;not null;uniqueIndex:idx_memes_content_hash" json:"content_hash"`
	ImageInfo   ImageInfo   `gorm:"type:text;not null" json:"image_info"`
	Tags        StringArray `gorm:"type:text" json:"tags"`
	Category    string      `gorm:"type:text;index:idx_memes_category" json:"category"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

// TableName returns the database table name for Meme.
// Parameters: none.
// Returns:
//   - string: table name for GORM mapping.
func (Meme) TableName() string {
	return "memes"
}

// MemeSearchResult represents a search result with a similarity score.
type MemeSearchResult struct {
	Meme
	Score float32 `json:"score"`
}
