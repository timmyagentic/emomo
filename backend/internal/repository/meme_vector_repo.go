package repository

import (
	"context"

	pb "github.com/timmy/emomo/gen/emomo/v1"
	"github.com/timmy/emomo/internal/domain"
	"github.com/timmy/emomo/internal/persistence"
	"gorm.io/gorm"
)

// MemeVectorRepository handles meme vector data operations.
type MemeVectorRepository struct {
	db *gorm.DB
}

// NewMemeVectorRepository creates a new MemeVectorRepository.
// Parameters:
//   - db: GORM database handle used for queries.
//
// Returns:
//   - *MemeVectorRepository: repository instance bound to db.
func NewMemeVectorRepository(db *gorm.DB) *MemeVectorRepository {
	return &MemeVectorRepository{db: db}
}

// Create inserts a new meme vector record.
// Parameters:
//   - ctx: context for cancellation and deadlines.
//   - vector: meme vector record to persist.
//
// Returns:
//   - error: non-nil if the insert fails.
func (r *MemeVectorRepository) Create(ctx context.Context, vector *domain.MemeVector) error {
	vector.VectorType = persistence.NormalizeVectorType(vector.VectorType)
	return r.db.WithContext(ctx).Create(vector).Error
}

// Update persists changes to an existing meme vector record.
func (r *MemeVectorRepository) Update(ctx context.Context, vector *domain.MemeVector) error {
	vector.VectorType = persistence.NormalizeVectorType(vector.VectorType)
	return r.db.WithContext(ctx).Save(vector).Error
}

// ExistsByMemeIDAndCollection checks if a vector record exists for the meme and collection.
// Parameters:
//   - ctx: context for cancellation and deadlines.
//   - memeID: meme identifier.
//   - collection: Qdrant collection name.
//
// Returns:
//   - bool: true if a record exists.
//   - error: non-nil if the lookup fails.
func (r *MemeVectorRepository) ExistsByMemeIDAndCollection(ctx context.Context, memeID, collection string) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&domain.MemeVector{}).
		Where("meme_id = ? AND collection = ?", memeID, collection).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// ExistsByMemeIDCollectionAndVectorType checks if a typed vector exists for the meme and collection.
// Parameters:
//   - ctx: context for cancellation and deadlines.
//   - memeID: meme identifier.
//   - collection: Qdrant collection name.
//   - vectorType: vector type such as image or caption.
//
// Returns:
//   - bool: true if a matching active or historical record exists.
//   - error: non-nil if the lookup fails.
func (r *MemeVectorRepository) ExistsByMemeIDCollectionAndVectorType(ctx context.Context, memeID, collection string, vectorType pb.VectorType) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&domain.MemeVector{}).
		Where("meme_id = ? AND collection = ? AND vector_type = ?", memeID, collection, persistence.NormalizeVectorType(vectorType)).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// GetByMemeIDAndCollection retrieves a vector record by meme ID and collection.
// Parameters:
//   - ctx: context for cancellation and deadlines.
//   - memeID: meme identifier.
//   - collection: Qdrant collection name.
//
// Returns:
//   - *domain.MemeVector: matching vector record if found.
//   - error: non-nil if the lookup fails.
func (r *MemeVectorRepository) GetByMemeIDAndCollection(ctx context.Context, memeID, collection string) (*domain.MemeVector, error) {
	var vector domain.MemeVector
	if err := r.db.WithContext(ctx).
		Where("meme_id = ? AND collection = ?", memeID, collection).
		First(&vector).Error; err != nil {
		return nil, err
	}
	return &vector, nil
}

// GetByMemeIDCollectionAndVectorType retrieves a vector record by meme ID, collection, and vector type.
func (r *MemeVectorRepository) GetByMemeIDCollectionAndVectorType(ctx context.Context, memeID, collection string, vectorType pb.VectorType) (*domain.MemeVector, error) {
	var vector domain.MemeVector
	if err := r.db.WithContext(ctx).
		Where("meme_id = ? AND collection = ? AND vector_type = ?", memeID, collection, persistence.NormalizeVectorType(vectorType)).
		First(&vector).Error; err != nil {
		return nil, err
	}
	return &vector, nil
}

// GetByMemeID retrieves all vector records for a given meme ID.
// Parameters:
//   - ctx: context for cancellation and deadlines.
//   - memeID: meme identifier.
//
// Returns:
//   - []domain.MemeVector: matching vector records.
//   - error: non-nil if the query fails.
func (r *MemeVectorRepository) GetByMemeID(ctx context.Context, memeID string) ([]domain.MemeVector, error) {
	var vectors []domain.MemeVector
	if err := r.db.WithContext(ctx).
		Where("meme_id = ?", memeID).
		Find(&vectors).Error; err != nil {
		return nil, err
	}
	return vectors, nil
}

// GetByCollection retrieves vector records for a given collection.
// Parameters:
//   - ctx: context for cancellation and deadlines.
//   - collection: Qdrant collection name.
//   - limit: maximum number of records to return.
//   - offset: number of records to skip.
//
// Returns:
//   - []domain.MemeVector: matching vector records.
//   - error: non-nil if the query fails.
func (r *MemeVectorRepository) GetByCollection(ctx context.Context, collection string, limit, offset int) ([]domain.MemeVector, error) {
	var vectors []domain.MemeVector
	query := r.db.WithContext(ctx).Where("collection = ?", collection)
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}
	if err := query.Find(&vectors).Error; err != nil {
		return nil, err
	}
	return vectors, nil
}

// CountByCollection counts the number of vectors in a collection.
// Parameters:
//   - ctx: context for cancellation and deadlines.
//   - collection: Qdrant collection name.
//
// Returns:
//   - int64: number of vector records in the collection.
//   - error: non-nil if the query fails.
func (r *MemeVectorRepository) CountByCollection(ctx context.Context, collection string) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&domain.MemeVector{}).
		Where("collection = ?", collection).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// Delete removes a meme vector by ID.
// Parameters:
//   - ctx: context for cancellation and deadlines.
//   - id: vector record ID.
//
// Returns:
//   - error: non-nil if the delete fails.
func (r *MemeVectorRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&domain.MemeVector{}, "id = ?", id).Error
}

// DeleteByMemeIDAndCollection deletes a vector record by meme ID and collection.
// Parameters:
//   - ctx: context for cancellation and deadlines.
//   - memeID: meme identifier.
//   - collection: Qdrant collection name.
//
// Returns:
//   - error: non-nil if the delete fails.
func (r *MemeVectorRepository) DeleteByMemeIDAndCollection(ctx context.Context, memeID, collection string) error {
	return r.db.WithContext(ctx).
		Where("meme_id = ? AND collection = ?", memeID, collection).
		Delete(&domain.MemeVector{}).Error
}

// DeleteByMemeIDCollectionAndVectorType deletes a vector record by meme ID, collection, and vector type.
func (r *MemeVectorRepository) DeleteByMemeIDCollectionAndVectorType(ctx context.Context, memeID, collection string, vectorType pb.VectorType) error {
	return r.db.WithContext(ctx).
		Where("meme_id = ? AND collection = ? AND vector_type = ?", memeID, collection, persistence.NormalizeVectorType(vectorType)).
		Delete(&domain.MemeVector{}).Error
}
