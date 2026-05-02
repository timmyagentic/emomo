package repository

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/timmy/emomo/internal/config"
	"github.com/timmy/emomo/internal/domain"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestInitDBAutoMigrateCreatesCleanCoreTables(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "clean.db")
	db, err := InitDB(&config.DatabaseConfig{
		Driver:          "sqlite",
		Path:            dbPath,
		AutoMigrate:     true,
		MaxIdleConns:    1,
		MaxOpenConns:    1,
		ConnMaxLifetime: 0,
	})
	if err != nil {
		t.Fatalf("InitDB() error = %v", err)
	}
	defer closeGormDB(t, db)

	for _, table := range []string{"memes", "meme_annotations", "meme_vectors"} {
		if !db.Migrator().HasTable(table) {
			t.Fatalf("expected table %s", table)
		}
	}
	for _, table := range []string{"data_sources", "ingest_jobs", "meme_descriptions"} {
		if db.Migrator().HasTable(table) {
			t.Fatalf("unexpected non-core table %s", table)
		}
	}
	if !hasSQLiteIndex(t, db, "idx_meme_vectors_meme_collection_type") {
		t.Fatal("idx_meme_vectors_meme_collection_type was not created")
	}

	repo := NewMemeVectorRepository(db)
	ctx := context.Background()
	base := &domain.MemeVector{
		MemeID:         "meme-1",
		Collection:     "same_collection",
		EmbeddingModel: "test-embedding",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	image := *base
	image.ID = "vector-image"
	image.VectorType = domain.MemeVectorTypeImage
	image.InputHash = "image-hash"
	image.QdrantPointID = "00000000-0000-0000-0000-000000000001"
	if err := repo.Create(ctx, &image); err != nil {
		t.Fatalf("failed to create image vector: %v", err)
	}

	caption := *base
	caption.ID = "vector-caption"
	caption.VectorType = domain.MemeVectorTypeCaption
	caption.InputHash = "caption-hash"
	caption.QdrantPointID = "00000000-0000-0000-0000-000000000002"
	if err := repo.Create(ctx, &caption); err != nil {
		t.Fatalf("failed to create caption vector with same meme+collection: %v", err)
	}
}

func TestInitDBAutoMigrateMigratesLegacyDescriptionsToAnnotations(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "legacy-annotations.db")
	setupDB, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open setup db: %v", err)
	}
	if err := setupLegacyDescriptionsSchema(setupDB); err != nil {
		t.Fatalf("failed to set up legacy schema: %v", err)
	}
	closeGormDB(t, setupDB)

	db, err := InitDB(&config.DatabaseConfig{
		Driver:          "sqlite",
		Path:            dbPath,
		AutoMigrate:     true,
		MaxIdleConns:    1,
		MaxOpenConns:    1,
		ConnMaxLifetime: 0,
	})
	if err != nil {
		t.Fatalf("InitDB() error = %v", err)
	}
	defer closeGormDB(t, db)

	var annotation domain.MemeAnnotation
	if err := db.First(&annotation, "id = ?", "desc-1").Error; err != nil {
		t.Fatalf("failed to load migrated annotation: %v", err)
	}
	if annotation.AnalyzerModel != "legacy-vlm" {
		t.Fatalf("AnalyzerModel = %q, want legacy-vlm", annotation.AnalyzerModel)
	}
	if annotation.Labels.Text == nil || !annotation.Labels.Text.Present {
		t.Fatalf("Labels.Text = %+v, want present=true", annotation.Labels.Text)
	}

	var unknownAnnotation domain.MemeAnnotation
	if err := db.First(&unknownAnnotation, "id = ?", "desc-2").Error; err != nil {
		t.Fatalf("failed to load migrated unknown annotation: %v", err)
	}
	if unknownAnnotation.Labels.Text != nil {
		t.Fatalf("blank legacy OCR Labels.Text = %+v, want nil", unknownAnnotation.Labels.Text)
	}

	var vector domain.MemeVector
	if err := db.First(&vector, "id = ?", "vector-1").Error; err != nil {
		t.Fatalf("failed to load migrated vector: %v", err)
	}
	if vector.AnnotationID != "desc-1" {
		t.Fatalf("AnnotationID = %q, want desc-1", vector.AnnotationID)
	}
}

func TestInitDBAutoMigrateBackfillsCleanFieldsFromLegacyColumns(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "legacy-memes.db")
	setupDB, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open setup db: %v", err)
	}
	if err := setupLegacyMemesAndVectorsSchema(setupDB); err != nil {
		t.Fatalf("failed to set up legacy memes schema: %v", err)
	}
	closeGormDB(t, setupDB)

	db, err := InitDB(&config.DatabaseConfig{
		Driver:          "sqlite",
		Path:            dbPath,
		AutoMigrate:     true,
		MaxIdleConns:    1,
		MaxOpenConns:    1,
		ConnMaxLifetime: 0,
	})
	if err != nil {
		t.Fatalf("InitDB() error = %v", err)
	}
	defer closeGormDB(t, db)

	var meme domain.Meme
	if err := db.First(&meme, "id = ?", "meme-legacy").Error; err != nil {
		t.Fatalf("failed to load migrated meme: %v", err)
	}
	if meme.ContentHash != "legacy-md5" {
		t.Fatalf("ContentHash = %q, want legacy-md5", meme.ContentHash)
	}
	if meme.ImageInfo.Width != 320 || meme.ImageInfo.Height != 240 || meme.ImageInfo.Format != domain.ImageFormatPNG {
		t.Fatalf("ImageInfo = %+v, want 320x240 png", meme.ImageInfo)
	}

	var vector domain.MemeVector
	if err := db.First(&vector, "id = ?", "vector-legacy").Error; err != nil {
		t.Fatalf("failed to load migrated vector: %v", err)
	}
	if vector.VectorType != domain.MemeVectorTypeCaption {
		t.Fatalf("VectorType = %v, want caption enum", vector.VectorType)
	}
}

func setupLegacyDescriptionsSchema(db *gorm.DB) error {
	if err := db.AutoMigrate(&domain.MemeVector{}); err != nil {
		return err
	}
	if err := db.Exec(`ALTER TABLE meme_vectors ADD COLUMN description_id TEXT`).Error; err != nil {
		return err
	}
	if err := db.Exec(`
		CREATE TABLE meme_descriptions (
			id TEXT PRIMARY KEY,
			meme_id TEXT NOT NULL,
			md5_hash TEXT NOT NULL,
			vlm_model TEXT NOT NULL,
			description TEXT NOT NULL,
			ocr_text TEXT,
			created_at DATETIME
		);
	`).Error; err != nil {
		return err
	}
	if err := db.Exec(`
		INSERT INTO meme_descriptions (id, meme_id, md5_hash, vlm_model, description, ocr_text, created_at)
		VALUES ('desc-1', 'meme-1', 'md5-1', 'legacy-vlm', '带文字的猫表情', '你好', CURRENT_TIMESTAMP);
		INSERT INTO meme_descriptions (id, meme_id, md5_hash, vlm_model, description, ocr_text, created_at)
		VALUES ('desc-2', 'meme-2', 'md5-2', 'legacy-vlm', '没有 OCR 历史结果的表情', '', CURRENT_TIMESTAMP);
	`).Error; err != nil {
		return err
	}
	if err := db.Create(&domain.MemeVector{
		ID:             "vector-1",
		MemeID:         "meme-1",
		Collection:     "meme_caption_qwen3vl_1024",
		VectorType:     domain.MemeVectorTypeCaption,
		EmbeddingModel: "Qwen/Qwen3-VL-Embedding-8B",
		InputHash:      "caption-hash",
		QdrantPointID:  "00000000-0000-0000-0000-000000000001",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}).Error; err != nil {
		return err
	}
	return db.Exec(`UPDATE meme_vectors SET description_id = 'desc-1' WHERE id = 'vector-1'`).Error
}

func setupLegacyMemesAndVectorsSchema(db *gorm.DB) error {
	if err := db.Exec(`
		CREATE TABLE memes (
			id TEXT PRIMARY KEY,
			storage_key TEXT,
			width INTEGER,
			height INTEGER,
			format TEXT,
			md5_hash TEXT,
			tags TEXT,
			category TEXT,
			created_at DATETIME,
			updated_at DATETIME
		);
		INSERT INTO memes (
			id, storage_key, width, height, format, md5_hash, tags, category, created_at, updated_at
		)
		VALUES (
			'meme-legacy', 'ab/legacy.png', 320, 240, 'png', 'legacy-md5', '[]', 'reaction',
			CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
		);
	`).Error; err != nil {
		return err
	}
	return db.Exec(`
		CREATE TABLE meme_vectors (
			id TEXT PRIMARY KEY,
			meme_id TEXT,
			collection TEXT,
			vector_type TEXT,
			embedding_model TEXT,
			input_hash TEXT,
			qdrant_point_id TEXT,
			created_at DATETIME,
			updated_at DATETIME
		);
		INSERT INTO meme_vectors (
			id, meme_id, collection, vector_type, embedding_model, input_hash, qdrant_point_id, created_at, updated_at
		)
		VALUES (
			'vector-legacy', 'meme-legacy', 'meme_caption_qwen3vl_1024', 'caption',
			'Qwen/Qwen3-VL-Embedding-8B', 'caption-hash',
			'00000000-0000-0000-0000-000000000001', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
		);
	`).Error
}

func hasSQLiteIndex(t *testing.T, db *gorm.DB, name string) bool {
	t.Helper()

	var count int
	if err := db.Raw(
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?",
		name,
	).Scan(&count).Error; err != nil {
		t.Fatalf("failed to inspect sqlite index %q: %v", name, err)
	}
	return count > 0
}

func closeGormDB(t *testing.T, db *gorm.DB) {
	t.Helper()

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("failed to get sql.DB: %v", err)
	}
	if err := sqlDB.Close(); err != nil && err != sql.ErrConnDone {
		t.Fatalf("failed to close db: %v", err)
	}
}
