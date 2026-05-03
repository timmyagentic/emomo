package repository

import (
	"context"

	pb "github.com/timmy/emomo/gen/emomo/v1"
	"github.com/timmy/emomo/internal/domain"
	"github.com/timmy/emomo/internal/persistence"
	"gorm.io/gorm"
)

// MemeAnnotationRepository handles meme annotation data operations.
type MemeAnnotationRepository struct {
	db *gorm.DB
}

// NewMemeAnnotationRepository creates a new MemeAnnotationRepository.
func NewMemeAnnotationRepository(db *gorm.DB) *MemeAnnotationRepository {
	return &MemeAnnotationRepository{db: db}
}

// Create inserts a new meme annotation record.
func (r *MemeAnnotationRepository) Create(ctx context.Context, annotation *domain.MemeAnnotation) error {
	return r.db.WithContext(ctx).Create(annotation).Error
}

// UpdateOCRText updates OCR text and its derived text-presence fields.
func (r *MemeAnnotationRepository) UpdateOCRText(ctx context.Context, id, ocrText string) error {
	presence, _ := domain.TextPresenceFromOCRText(ocrText)
	labels := &pb.MemeAnnotationLabels{
		Text: &pb.TextLabel{Present: presence == pb.TextPresence_TEXT_PRESENCE_WITH_TEXT},
	}
	labelsJSON, err := persistence.MarshalProtoColumn(labels)
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).
		Model(&domain.MemeAnnotation{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"ocr_text": ocrText,
			"labels":   labelsJSON,
		}).Error
}

// GetByMemeIDAndModel retrieves an annotation by meme and analyzer model.
func (r *MemeAnnotationRepository) GetByMemeIDAndModel(ctx context.Context, memeID, analyzerModel string) (*domain.MemeAnnotation, error) {
	var annotation domain.MemeAnnotation
	if err := r.db.WithContext(ctx).
		Where("meme_id = ? AND analyzer_model = ?", memeID, analyzerModel).
		First(&annotation).Error; err != nil {
		return nil, err
	}
	return &annotation, nil
}

// ExistsByMemeIDAndModel checks if an annotation exists for the meme and analyzer model.
func (r *MemeAnnotationRepository) ExistsByMemeIDAndModel(ctx context.Context, memeID, analyzerModel string) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&domain.MemeAnnotation{}).
		Where("meme_id = ? AND analyzer_model = ?", memeID, analyzerModel).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// GetByID retrieves an annotation by its ID.
func (r *MemeAnnotationRepository) GetByID(ctx context.Context, id string) (*domain.MemeAnnotation, error) {
	var annotation domain.MemeAnnotation
	if err := r.db.WithContext(ctx).First(&annotation, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &annotation, nil
}

// GetByMemeID retrieves all annotations for a given meme ID.
func (r *MemeAnnotationRepository) GetByMemeID(ctx context.Context, memeID string) ([]domain.MemeAnnotation, error) {
	var annotations []domain.MemeAnnotation
	if err := r.db.WithContext(ctx).
		Where("meme_id = ?", memeID).
		Find(&annotations).Error; err != nil {
		return nil, err
	}
	return annotations, nil
}

// Search performs a simple keyword search on annotations.
func (r *MemeAnnotationRepository) Search(ctx context.Context, query string, limit int) ([]domain.MemeAnnotation, error) {
	var annotations []domain.MemeAnnotation
	if err := r.db.WithContext(ctx).
		Where("LOWER(description) LIKE LOWER(?) OR LOWER(ocr_text) LIKE LOWER(?)", "%"+query+"%", "%"+query+"%").
		Limit(limit).
		Find(&annotations).Error; err != nil {
		return nil, err
	}
	return annotations, nil
}

// Delete removes a meme annotation by ID.
func (r *MemeAnnotationRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&domain.MemeAnnotation{}, "id = ?", id).Error
}
