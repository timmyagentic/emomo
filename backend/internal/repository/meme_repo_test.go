package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	pb "github.com/timmy/emomo/gen/emomo/v1"
	"github.com/timmy/emomo/internal/domain"
	_ "github.com/timmy/emomo/internal/persistence"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestMemeRepositoryListTreatsNonPositiveLimitAsUnlimited(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&domain.Meme{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	for i := 0; i < 3; i++ {
		id := fmt.Sprintf("meme-%d", i)
		meme := &domain.Meme{
			ID:          id,
			StorageKey:  id + ".png",
			ContentHash: id,
			ImageInfo: &pb.ImageInfo{
				Width:  1,
				Height: 1,
				Format: pb.ImageFormat_IMAGE_FORMAT_PNG,
			},
			CreatedAt: time.Now().Add(time.Duration(i) * time.Second),
			UpdatedAt: time.Now().Add(time.Duration(i) * time.Second),
		}
		if err := db.Create(meme).Error; err != nil {
			t.Fatalf("create meme %d: %v", i, err)
		}
	}

	memes, err := NewMemeRepository(db).List(context.Background(), 0, 0)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(memes) != 3 {
		t.Fatalf("List(limit=0) returned %d memes, want 3", len(memes))
	}
}
