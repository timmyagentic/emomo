package domain

import (
	"time"

	pb "github.com/timmy/emomo/gen/emomo/v1"
)

// MemeVector records the relationship between a meme and its embedding in a
// specific Qdrant collection / vector route. The VectorType column directly
// stores the protobuf enum's numeric value (default 1 = IMAGE).
type MemeVector struct {
	ID             string         `gorm:"type:text;primaryKey" json:"id"`
	MemeID         string         `gorm:"type:text;not null;uniqueIndex:idx_meme_vectors_meme_collection_type;index:idx_meme_vectors_meme" json:"meme_id"`
	Collection     string         `gorm:"type:text;not null;uniqueIndex:idx_meme_vectors_meme_collection_type" json:"collection"`
	VectorType     pb.VectorType  `gorm:"type:integer;not null;default:1;uniqueIndex:idx_meme_vectors_meme_collection_type" json:"vector_type"`
	EmbeddingModel string         `gorm:"type:text;not null" json:"embedding_model"`
	InputHash      string         `gorm:"type:text" json:"input_hash"`
	AnnotationID   string         `gorm:"type:text;index:idx_meme_vectors_annotation" json:"annotation_id,omitempty"`
	QdrantPointID  string         `gorm:"type:text;not null" json:"qdrant_point_id"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

// TableName returns the database table name for MemeVector.
func (MemeVector) TableName() string {
	return "meme_vectors"
}
