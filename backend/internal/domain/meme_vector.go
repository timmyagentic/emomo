package domain

import (
	"time"

	pb "github.com/timmy/emomo/gen/emomo/v1"
)

// MemeVector records the relationship between a meme and its embedding in a
// specific Qdrant collection / vector route.
//
// VectorType persists the protobuf enum's numeric value via the protoenum
// serializer (registered in internal/persistence). The serializer guards
// against the Postgres driver's default Stringer-based encoding for typed
// int32 enums, which would otherwise insert "VECTOR_TYPE_IMAGE" /
// "VECTOR_TYPE_CAPTION" strings into an INTEGER column and fail at write
// time. SQLite happens to follow the reflect-int path so this only
// surfaces on Postgres.
type MemeVector struct {
	ID             string        `gorm:"type:text;primaryKey" json:"id"`
	MemeID         string        `gorm:"type:text;not null;uniqueIndex:idx_meme_vectors_meme_collection_type;index:idx_meme_vectors_meme" json:"meme_id"`
	Collection     string        `gorm:"type:text;not null;uniqueIndex:idx_meme_vectors_meme_collection_type" json:"collection"`
	VectorType     pb.VectorType `gorm:"type:integer;not null;default:1;uniqueIndex:idx_meme_vectors_meme_collection_type;serializer:protoenum" json:"vector_type"`
	EmbeddingModel string        `gorm:"type:text;not null" json:"embedding_model"`
	InputHash      string        `gorm:"type:text" json:"input_hash"`
	AnnotationID   string        `gorm:"type:text;index:idx_meme_vectors_annotation" json:"annotation_id,omitempty"`
	QdrantPointID  string        `gorm:"type:text;not null" json:"qdrant_point_id"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
}

// TableName returns the database table name for MemeVector.
func (MemeVector) TableName() string {
	return "meme_vectors"
}
