package repository

import (
	"context"
	"testing"
	"time"

	"github.com/timmy/emomo/internal/domain"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestMemeVectorRepositorySeparatesVectorTypesWithinCollection(t *testing.T) {
	t.Parallel()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&domain.MemeVector{}); err != nil {
		t.Fatalf("failed to migrate meme_vectors: %v", err)
	}

	repo := NewMemeVectorRepository(db)
	ctx := context.Background()
	base := domain.MemeVector{
		MemeID:         "meme-1",
		Collection:     "meme_caption_qwen3vl_1024",
		EmbeddingModel: "Qwen/Qwen3-VL-Embedding-8B",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	image := base
	image.ID = "vector-image"
	image.VectorType = domain.MemeVectorTypeImage
	image.QdrantPointID = "00000000-0000-0000-0000-000000000001"
	image.InputHash = "md5"

	caption := base
	caption.ID = "vector-caption"
	caption.VectorType = domain.MemeVectorTypeCaption
	caption.QdrantPointID = "00000000-0000-0000-0000-000000000002"
	caption.InputHash = "sha256-caption"

	if err := repo.Create(ctx, &image); err != nil {
		t.Fatalf("failed to create image vector: %v", err)
	}
	if err := repo.Create(ctx, &caption); err != nil {
		t.Fatalf("failed to create caption vector with same md5+collection: %v", err)
	}

	exists, err := repo.ExistsByMemeIDCollectionAndVectorType(ctx, "meme-1", "meme_caption_qwen3vl_1024", domain.MemeVectorTypeCaption)
	if err != nil {
		t.Fatalf("ExistsByMemeIDCollectionAndVectorType returned error: %v", err)
	}
	if !exists {
		t.Fatal("expected caption vector to exist")
	}
}
