package repository

import (
	"context"
	"fmt"

	"github.com/timmy/emomo/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// MemeMetadataRepository handles provenance metadata for memes.
//
// The persisted rows are intentionally separate from `memes` so the
// search-facing tables stay free of crawler-specific fields. Each row is
// keyed by (source, source_item_id, meme_id); see domain.MemeMetadata for
// the rationale.
type MemeMetadataRepository struct {
	db *gorm.DB
}

// NewMemeMetadataRepository creates a new MemeMetadataRepository.
func NewMemeMetadataRepository(db *gorm.DB) *MemeMetadataRepository {
	return &MemeMetadataRepository{db: db}
}

// Upsert inserts a new metadata row, or updates the existing one identified
// by (source, source_item_id, meme_id). The caller is responsible for
// generating the surrogate ID and timestamps; on conflict the surrogate
// fields are preserved (Postgres returns the row's existing primary key).
func (r *MemeMetadataRepository) Upsert(ctx context.Context, metadata *domain.MemeMetadata) error {
	if metadata == nil {
		return fmt.Errorf("metadata is nil")
	}
	if metadata.MemeID == "" {
		return fmt.Errorf("metadata.MemeID is required")
	}
	if metadata.Source == "" {
		return fmt.Errorf("metadata.Source is required")
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "source"},
			{Name: "source_item_id"},
			{Name: "meme_id"},
		},
		DoUpdates: clause.AssignmentColumns([]string{
			"source_url",
			"title",
			"author",
			"published_at",
			"search_keywords",
			"updated_at",
		}),
	}).Create(metadata).Error
}

// GetByMemeID returns all metadata rows attached to a meme (across sources).
func (r *MemeMetadataRepository) GetByMemeID(ctx context.Context, memeID string) ([]domain.MemeMetadata, error) {
	var rows []domain.MemeMetadata
	if err := r.db.WithContext(ctx).
		Where("meme_id = ?", memeID).
		Order("source, source_item_id").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// DeleteByMemeID removes every metadata row for the given meme. Used by
// rollback paths in ingest when the parent meme record fails to land.
func (r *MemeMetadataRepository) DeleteByMemeID(ctx context.Context, memeID string) error {
	return r.db.WithContext(ctx).
		Where("meme_id = ?", memeID).
		Delete(&domain.MemeMetadata{}).Error
}
