package repository

import (
	"context"
	"testing"
	"time"

	"github.com/timmy/emomo/internal/domain"
	_ "github.com/timmy/emomo/internal/persistence"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newMetadataTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&domain.MemeMetadata{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestMemeMetadataRepositoryUpsertInsertsThenUpdatesInPlace(t *testing.T) {
	db := newMetadataTestDB(t)
	repo := NewMemeMetadataRepository(db)
	ctx := context.Background()

	now := time.Now()
	first := &domain.MemeMetadata{
		ID:             "row-1",
		MemeID:         "meme-A",
		Source:         "xiaohongshu",
		SourceItemID:   "note-1",
		SourceURL:      "https://www.xiaohongshu.com/explore/note-1",
		Title:          "原标题",
		Author:         "alice",
		PublishedAt:    "2026-01-01",
		SearchKeywords: domain.StringArray{"学生党表情包"},
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := repo.Upsert(ctx, first); err != nil {
		t.Fatalf("Upsert(initial) error = %v", err)
	}

	second := &domain.MemeMetadata{
		ID:             "row-2-but-conflict",
		MemeID:         "meme-A",
		Source:         "xiaohongshu",
		SourceItemID:   "note-1",
		SourceURL:      "https://www.xiaohongshu.com/explore/note-1?tab=share",
		Title:          "新标题",
		Author:         "alice",
		PublishedAt:    "2026-02-02",
		SearchKeywords: domain.StringArray{"学生党表情包", "考试表情包"},
		CreatedAt:      now.Add(time.Minute),
		UpdatedAt:      now.Add(time.Minute),
	}
	if err := repo.Upsert(ctx, second); err != nil {
		t.Fatalf("Upsert(conflict) error = %v", err)
	}

	rows, err := repo.GetByMemeID(ctx, "meme-A")
	if err != nil {
		t.Fatalf("GetByMemeID() error = %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("GetByMemeID() returned %d rows, want 1 (upsert should not duplicate)", len(rows))
	}
	got := rows[0]
	if got.ID != "row-1" {
		t.Fatalf("ID = %q, want row-1 (existing surrogate ID must be preserved on conflict)", got.ID)
	}
	if got.Title != "新标题" {
		t.Fatalf("Title = %q, want 新标题 (update should land)", got.Title)
	}
	if got.SourceURL != "https://www.xiaohongshu.com/explore/note-1?tab=share" {
		t.Fatalf("SourceURL = %q, did not update", got.SourceURL)
	}
	if len(got.SearchKeywords) != 2 {
		t.Fatalf("SearchKeywords = %v, want 2 entries after update", got.SearchKeywords)
	}
}

func TestMemeMetadataRepositoryAllowsMultipleSourcesPerMeme(t *testing.T) {
	db := newMetadataTestDB(t)
	repo := NewMemeMetadataRepository(db)
	ctx := context.Background()
	now := time.Now()

	rows := []*domain.MemeMetadata{
		{
			ID:           "row-xhs",
			MemeID:       "meme-shared",
			Source:       "xiaohongshu",
			SourceItemID: "xhs-note",
			CreatedAt:    now,
			UpdatedAt:    now,
		},
		{
			ID:           "row-bqb",
			MemeID:       "meme-shared",
			Source:       "chinesebqb",
			SourceItemID: "bqb-id",
			CreatedAt:    now,
			UpdatedAt:    now,
		},
	}
	for i, row := range rows {
		if err := repo.Upsert(ctx, row); err != nil {
			t.Fatalf("Upsert(%d) error = %v", i, err)
		}
	}

	got, err := repo.GetByMemeID(ctx, "meme-shared")
	if err != nil {
		t.Fatalf("GetByMemeID() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("GetByMemeID() = %d rows, want 2 (different sources should coexist)", len(got))
	}
}

func TestMemeMetadataRepositoryDeleteByMemeID(t *testing.T) {
	db := newMetadataTestDB(t)
	repo := NewMemeMetadataRepository(db)
	ctx := context.Background()
	now := time.Now()

	if err := repo.Upsert(ctx, &domain.MemeMetadata{
		ID:        "row-1",
		MemeID:    "meme-X",
		Source:    "xiaohongshu",
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("Upsert error = %v", err)
	}

	if err := repo.DeleteByMemeID(ctx, "meme-X"); err != nil {
		t.Fatalf("DeleteByMemeID error = %v", err)
	}
	got, err := repo.GetByMemeID(ctx, "meme-X")
	if err != nil {
		t.Fatalf("GetByMemeID error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("GetByMemeID after delete = %d rows, want 0", len(got))
	}
}

func TestMemeMetadataRepositoryUpsertRejectsMissingKeys(t *testing.T) {
	repo := NewMemeMetadataRepository(newMetadataTestDB(t))
	ctx := context.Background()

	if err := repo.Upsert(ctx, nil); err == nil {
		t.Fatal("Upsert(nil) error = nil, want error")
	}
	if err := repo.Upsert(ctx, &domain.MemeMetadata{Source: "xiaohongshu"}); err == nil {
		t.Fatal("Upsert(no MemeID) error = nil, want error")
	}
	if err := repo.Upsert(ctx, &domain.MemeMetadata{MemeID: "m"}); err == nil {
		t.Fatal("Upsert(no Source) error = nil, want error")
	}
}
