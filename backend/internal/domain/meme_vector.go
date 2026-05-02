package domain

import "time"

// MemeVector represents the relationship between a meme and its vector in a specific collection.
// This allows the same meme to be embedded using different models and stored per collection.
type MemeVector struct {
	ID             string         `gorm:"type:text;primaryKey" json:"id"`
	MemeID         string         `gorm:"type:text;not null;uniqueIndex:idx_meme_vectors_meme_collection_type;index:idx_meme_vectors_meme" json:"meme_id"`
	Collection     string         `gorm:"type:text;not null;uniqueIndex:idx_meme_vectors_meme_collection_type" json:"collection"`
	VectorType     MemeVectorType `gorm:"type:integer;not null;default:1;uniqueIndex:idx_meme_vectors_meme_collection_type" json:"vector_type"`
	EmbeddingModel string         `gorm:"type:text;not null" json:"embedding_model"`
	InputHash      string         `gorm:"type:text" json:"input_hash"`
	AnnotationID   string         `gorm:"type:text;index:idx_meme_vectors_annotation" json:"annotation_id,omitempty"`
	QdrantPointID  string         `gorm:"type:text;not null" json:"qdrant_point_id"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

// TableName returns the database table name for MemeVector.
// Parameters: none.
// Returns:
//   - string: table name for GORM mapping.
func (MemeVector) TableName() string {
	return "meme_vectors"
}
