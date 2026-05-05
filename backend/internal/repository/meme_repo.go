package repository

import (
	"context"
	"fmt"

	"github.com/timmy/emomo/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// MemeRepository handles meme data operations.
type MemeRepository struct {
	db *gorm.DB
}

// NewMemeRepository creates a new MemeRepository.
// Parameters:
//   - db: GORM database handle used for queries.
//
// Returns:
//   - *MemeRepository: repository instance bound to db.
func NewMemeRepository(db *gorm.DB) *MemeRepository {
	return &MemeRepository{db: db}
}

// Create inserts a new meme record.
// Parameters:
//   - ctx: context for cancellation and deadlines.
//   - meme: meme record to persist.
//
// Returns:
//   - error: non-nil if the insert fails.
func (r *MemeRepository) Create(ctx context.Context, meme *domain.Meme) error {
	return r.db.WithContext(ctx).Create(meme).Error
}

// Upsert creates or updates a meme record keyed by content hash.
// Parameters:
//   - ctx: context for cancellation and deadlines.
//   - meme: meme record to create or update.
//
// Returns:
//   - error: non-nil if the upsert fails.
func (r *MemeRepository) Upsert(ctx context.Context, meme *domain.Meme) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "content_hash"}},
		UpdateAll: true,
	}).Create(meme).Error
}

// Update updates an existing meme record.
// Parameters:
//   - ctx: context for cancellation and deadlines.
//   - meme: meme record with updated fields.
//
// Returns:
//   - error: non-nil if the update fails.
func (r *MemeRepository) Update(ctx context.Context, meme *domain.Meme) error {
	return r.db.WithContext(ctx).Save(meme).Error
}

// GetByID retrieves a meme by its ID.
// Parameters:
//   - ctx: context for cancellation and deadlines.
//   - id: meme ID.
//
// Returns:
//   - *domain.Meme: meme record if found.
//   - error: non-nil if lookup fails.
func (r *MemeRepository) GetByID(ctx context.Context, id string) (*domain.Meme, error) {
	var meme domain.Meme
	if err := r.db.WithContext(ctx).First(&meme, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &meme, nil
}

// GetByContentHash retrieves a meme by its content hash for deduplication.
// Parameters:
//   - ctx: context for cancellation and deadlines.
//   - contentHash: hash of the processed meme content.
//
// Returns:
//   - *domain.Meme: meme record if found.
//   - error: non-nil if lookup fails.
func (r *MemeRepository) GetByContentHash(ctx context.Context, contentHash string) (*domain.Meme, error) {
	var meme domain.Meme
	if err := r.db.WithContext(ctx).First(&meme, "content_hash = ?", contentHash).Error; err != nil {
		return nil, err
	}
	return &meme, nil
}

// ExistsByContentHash checks if a meme with the given content hash exists.
// Parameters:
//   - ctx: context for cancellation and deadlines.
//   - contentHash: hash of the processed meme content.
//
// Returns:
//   - bool: true if a record exists.
//   - error: non-nil if the lookup fails.
func (r *MemeRepository) ExistsByContentHash(ctx context.Context, contentHash string) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&domain.Meme{}).Where("content_hash = ?", contentHash).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// List retrieves memes with pagination.
// Parameters:
//   - ctx: context for cancellation and deadlines.
//   - limit: maximum number of records to return.
//   - offset: number of records to skip.
//
// Returns:
//   - []domain.Meme: matching meme records.
//   - error: non-nil if the query fails.
func (r *MemeRepository) List(ctx context.Context, limit, offset int) ([]domain.Meme, error) {
	var memes []domain.Meme
	query := r.db.WithContext(ctx).Order("created_at DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}
	if err := query.Find(&memes).Error; err != nil {
		return nil, err
	}
	return memes, nil
}

// ListByCategory retrieves memes by category with pagination.
// Parameters:
//   - ctx: context for cancellation and deadlines.
//   - category: category name to filter by; empty means all.
//   - limit: maximum number of records to return.
//   - offset: number of records to skip.
//
// Returns:
//   - []domain.Meme: matching meme records.
//   - error: non-nil if the query fails.
func (r *MemeRepository) ListByCategory(ctx context.Context, category string, limit, offset int) ([]domain.Meme, error) {
	var memes []domain.Meme
	query := r.db.WithContext(ctx)
	if category != "" {
		query = query.Where("category = ?", category)
	}
	if err := query.
		Limit(limit).
		Offset(offset).
		Order("created_at DESC").
		Find(&memes).Error; err != nil {
		return nil, err
	}
	return memes, nil
}

// GetCategories retrieves all unique categories.
// Parameters:
//   - ctx: context for cancellation and deadlines.
//
// Returns:
//   - []string: distinct category names.
//   - error: non-nil if the query fails.
func (r *MemeRepository) GetCategories(ctx context.Context) ([]string, error) {
	var categories []string
	if err := r.db.WithContext(ctx).
		Model(&domain.Meme{}).
		Distinct("category").
		Pluck("category", &categories).Error; err != nil {
		return nil, err
	}
	return categories, nil
}

// Count counts all memes.
// Parameters:
//   - ctx: context for cancellation and deadlines.
//
// Returns:
//   - int64: number of matching records.
//   - error: non-nil if the query fails.
func (r *MemeRepository) Count(ctx context.Context) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&domain.Meme{}).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// GetByIDs retrieves memes by a list of IDs.
// Parameters:
//   - ctx: context for cancellation and deadlines.
//   - ids: list of meme IDs.
//
// Returns:
//   - []domain.Meme: matching meme records.
//   - error: non-nil if the query fails.
func (r *MemeRepository) GetByIDs(ctx context.Context, ids []string) ([]domain.Meme, error) {
	if len(ids) == 0 {
		return []domain.Meme{}, nil
	}
	var memes []domain.Meme
	if err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&memes).Error; err != nil {
		return nil, fmt.Errorf("failed to get memes by IDs: %w", err)
	}
	return memes, nil
}

// Delete removes a meme by ID.
// Parameters:
//   - ctx: context for cancellation and deadlines.
//   - id: meme ID to delete.
//
// Returns:
//   - error: non-nil if the delete fails.
func (r *MemeRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&domain.Meme{}, "id = ?", id).Error
}
