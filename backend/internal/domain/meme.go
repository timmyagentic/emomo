// Package domain hosts emomo's GORM-backed application data model.
//
// The structured-value columns (memes.image_info, meme_annotations.labels)
// are typed directly as generated protobuf message pointers — *pb.ImageInfo,
// *pb.MemeAnnotationLabels — and serialized via the protojson GORM
// serializer registered in backend/internal/persistence. This is intentionally
// limited to the allowlisted structured JSON columns; relational table shape,
// indexes, and migrations remain owned by the GORM models and repository/db.go.
//
// Pointer types are used (rather than value types) because every protobuf
// message embeds a protoimpl.MessageState containing a pragma.DoNotCopy /
// sync.Mutex pair; embedding by value would have GORM Find/Save copy a lock
// every time it walks the struct, which go vet rightly flags.
package domain

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	pb "github.com/timmy/emomo/gen/emomo/v1"
)

// StringArray persists a Go string slice as a JSON array TEXT column.
type StringArray []string

// Value implements the driver.Valuer interface for database serialization.
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
type Meme struct {
	ID          string         `gorm:"type:text;primaryKey" json:"id"`
	StorageKey  string         `gorm:"type:text;not null" json:"storage_key"`
	ContentHash string         `gorm:"type:text;not null;uniqueIndex:idx_memes_content_hash" json:"content_hash"`
	ImageInfo   *pb.ImageInfo  `gorm:"type:text;not null;serializer:protojson" json:"image_info"`
	Tags        StringArray    `gorm:"type:text" json:"tags"`
	Category    string         `gorm:"type:text;index:idx_memes_category" json:"category"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// TableName returns the database table name for Meme.
func (Meme) TableName() string {
	return "memes"
}
