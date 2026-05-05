package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/qdrant/go-client/qdrant"
	pb "github.com/timmy/emomo/gen/emomo/v1"
	"github.com/timmy/emomo/internal/domain"
	_ "github.com/timmy/emomo/internal/persistence" // register protojson serializer
	"github.com/timmy/emomo/internal/repository"
	"github.com/timmy/emomo/internal/source"
	"google.golang.org/grpc"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestIsSupportedStaticImageFormatRejectsGIF(t *testing.T) {
	t.Parallel()

	if isSupportedStaticImageFormat("gif") {
		t.Fatal("isSupportedStaticImageFormat(\"gif\") = true, want false")
	}
}

func TestProcessItemRejectsGIFMagicBytes(t *testing.T) {
	t.Parallel()

	imagePath := filepath.Join(t.TempDir(), "looks-static.jpg")
	if err := os.WriteFile(imagePath, []byte("GIF89a-static"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	service := &IngestService{}
	err := service.processItem(context.Background(), "test", &source.MemeItem{
		SourceID:  "deceptive-gif",
		LocalPath: imagePath,
		Format:    "jpeg",
	}, &IngestOptions{})

	if !errors.Is(err, errSkipUnsupportedImageFormat) {
		t.Fatalf("processItem() error = %v, want errSkipUnsupportedImageFormat", err)
	}
}

func TestIngestFromSourceTreatsNonPositiveLimitAsUnlimited(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	items := make([]source.MemeItem, 0, 3)
	for i := 0; i < 3; i++ {
		imagePath := filepath.Join(tempDir, fmt.Sprintf("skip-%d.gif", i))
		if err := os.WriteFile(imagePath, []byte("GIF89a-static"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		items = append(items, source.MemeItem{
			SourceID:  fmt.Sprintf("gif-%d", i),
			LocalPath: imagePath,
			Format:    "gif",
		})
	}

	ingest := &IngestService{
		workers:   1,
		batchSize: 2,
	}

	stats, err := ingest.IngestFromSource(context.Background(), &staticTestSource{
		sourceID: "static",
		items:    items,
	}, 0, nil)
	if err != nil {
		t.Fatalf("IngestFromSource() error = %v", err)
	}

	if stats.TotalItems != 3 {
		t.Fatalf("TotalItems = %d, want 3", stats.TotalItems)
	}
	if stats.ProcessedItems != 3 {
		t.Fatalf("ProcessedItems = %d, want 3", stats.ProcessedItems)
	}
	if stats.SkippedItems != 3 {
		t.Fatalf("SkippedItems = %d, want 3", stats.SkippedItems)
	}
}

func TestProcessItemRollsBackNewMemeWhenVectorWriteFails(t *testing.T) {
	t.Parallel()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&domain.Meme{}, &domain.MemeVector{}, &domain.MemeAnnotation{}); err != nil {
		t.Fatalf("failed to migrate test database: %v", err)
	}

	imagePath := filepath.Join(t.TempDir(), "meme.png")
	if err := os.WriteFile(imagePath, testPNG1x1, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	store := newMemoryObjectStorage()
	vlm := NewVLMService(&VLMConfig{
		Model:   "test-vlm",
		APIKey:  "test-key",
		BaseURL: "https://vlm.test/v1",
	})
	vlm.client.SetTransport(roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return jsonResponse(t, http.StatusOK, openAIResponse{
			Choices: []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			}{
				{Message: struct {
					Content string `json:"content"`
				}{Content: "开心质问的表情包"}},
			},
		}), nil
	}))

	ingest := NewIngestService(
		repository.NewMemeRepository(db),
		repository.NewMemeVectorRepository(db),
		repository.NewMemeAnnotationRepository(db),
		nil,
		nil,
		store,
		vlm,
		nil,
		nil,
		&IngestConfig{
			Workers:    1,
			BatchSize:  1,
			Collection: "broken_collection",
			VectorIndexes: []IngestVectorIndex{
				{
					VectorType: pb.VectorType_VECTOR_TYPE_IMAGE,
					Collection: "broken_collection",
					Embedding:  fixedEmbeddingProvider{},
				},
			},
		},
	)

	err = ingest.processItem(context.Background(), "test", &source.MemeItem{
		SourceID:  "new-meme",
		LocalPath: imagePath,
		Format:    "png",
		Category:  "reaction",
		Tags:      []string{"happy"},
	}, &IngestOptions{})
	if err == nil {
		t.Fatal("processItem() error = nil, want vector write failure")
	}

	var memeCount int64
	if err := db.Model(&domain.Meme{}).Count(&memeCount).Error; err != nil {
		t.Fatalf("count memes: %v", err)
	}
	if memeCount != 0 {
		t.Fatalf("meme count after rollback = %d, want 0", memeCount)
	}

	var descriptionCount int64
	if err := db.Model(&domain.MemeAnnotation{}).Count(&descriptionCount).Error; err != nil {
		t.Fatalf("count descriptions: %v", err)
	}
	if descriptionCount != 0 {
		t.Fatalf("description count after rollback = %d, want 0", descriptionCount)
	}

	if len(store.objects) != 0 {
		t.Fatalf("storage objects after rollback = %d, want 0", len(store.objects))
	}
	if store.deleteCount != 1 {
		t.Fatalf("storage delete count = %d, want 1", store.deleteCount)
	}
}

type staticTestSource struct {
	sourceID string
	items    []source.MemeItem
}

func (s *staticTestSource) GetSourceID() string {
	return s.sourceID
}

func (s *staticTestSource) GetDisplayName() string {
	return s.sourceID
}

func (s *staticTestSource) SupportsIncremental() bool {
	return false
}

func (s *staticTestSource) FetchBatch(ctx context.Context, cursor string, limit int) ([]source.MemeItem, string, error) {
	_ = ctx
	start := 0
	if cursor != "" {
		parsed, err := strconv.Atoi(cursor)
		if err != nil {
			return nil, "", err
		}
		start = parsed
	}
	if start >= len(s.items) {
		return []source.MemeItem{}, "", nil
	}
	end := len(s.items)
	if limit > 0 && start+limit < end {
		end = start + limit
	}
	next := ""
	if end < len(s.items) {
		next = strconv.Itoa(end)
	}
	return s.items[start:end], next, nil
}

func TestProcessItemWritesImageVectorWhenVLMFails(t *testing.T) {
	t.Parallel()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&domain.Meme{}, &domain.MemeVector{}, &domain.MemeAnnotation{}); err != nil {
		t.Fatalf("failed to migrate test database: %v", err)
	}

	imagePath := filepath.Join(t.TempDir(), "meme.png")
	if err := os.WriteFile(imagePath, testPNG1x1, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	qdrantRepo, fakeQdrant := newTestQdrantRepository(t)
	store := newMemoryObjectStorage()
	vlm := NewVLMService(&VLMConfig{
		Model:   "test-vlm",
		APIKey:  "test-key",
		BaseURL: "https://vlm.test/v1",
	})
	vlm.client.SetTransport(roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return jsonResponse(t, http.StatusInternalServerError, openAIResponse{}), nil
	}))

	ingest := NewIngestService(
		repository.NewMemeRepository(db),
		repository.NewMemeVectorRepository(db),
		repository.NewMemeAnnotationRepository(db),
		repository.NewMemeMetadataRepository(db),
		qdrantRepo,
		store,
		vlm,
		fixedEmbeddingProvider{},
		nil,
		&IngestConfig{
			Workers:   1,
			BatchSize: 1,
			VectorIndexes: []IngestVectorIndex{
				{
					VectorType: pb.VectorType_VECTOR_TYPE_IMAGE,
					Collection: "image_collection",
					Embedding:  fixedEmbeddingProvider{},
					QdrantRepo: qdrantRepo,
				},
				{
					VectorType: pb.VectorType_VECTOR_TYPE_CAPTION,
					Collection: "caption_collection",
					Embedding:  fixedEmbeddingProvider{},
					QdrantRepo: qdrantRepo,
				},
			},
		},
	)

	err = ingest.processItem(context.Background(), "test", &source.MemeItem{
		SourceID:  "new-meme",
		LocalPath: imagePath,
		Format:    "png",
		Category:  "reaction",
		Tags:      []string{"happy"},
	}, &IngestOptions{})
	if err != nil {
		t.Fatalf("processItem() error = %v, want nil", err)
	}

	var vectors []domain.MemeVector
	if err := db.Find(&vectors).Error; err != nil {
		t.Fatalf("failed to load vectors: %v", err)
	}
	if len(vectors) == 0 {
		t.Fatal("vector count = 0, want at least image vector")
	}
	hasImageVector := false
	for _, vector := range vectors {
		if vector.VectorType == pb.VectorType_VECTOR_TYPE_IMAGE {
			hasImageVector = true
		}
	}
	if !hasImageVector {
		t.Fatalf("vectors = %+v, want image vector", vectors)
	}
	if fakeQdrant.upserts == 0 {
		t.Fatal("qdrant upserts = 0, want at least one image vector upsert")
	}
}

func TestProcessItemForceKeepsOldVectorWhenReplacementFails(t *testing.T) {
	t.Parallel()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&domain.Meme{}, &domain.MemeVector{}, &domain.MemeAnnotation{}); err != nil {
		t.Fatalf("failed to migrate test database: %v", err)
	}

	imagePath := filepath.Join(t.TempDir(), "meme.png")
	if err := os.WriteFile(imagePath, testPNG1x1, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	meme := &domain.Meme{
		ID:          "meme-existing",
		StorageKey:  "ab/existing.png",
		ContentHash: calculateMD5(testPNG1x1),
		ImageInfo: &pb.ImageInfo{
			Width:  1,
			Height: 1,
			Format: pb.ImageFormat_IMAGE_FORMAT_PNG,
		},
		Category:  "reaction",
		Tags:      domain.StringArray{"happy"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := repository.NewMemeRepository(db).Create(context.Background(), meme); err != nil {
		t.Fatalf("failed to create meme: %v", err)
	}

	vectorRepo := repository.NewMemeVectorRepository(db)
	oldVector := &domain.MemeVector{
		ID:             "vector-existing",
		MemeID:         meme.ID,
		Collection:     "image_collection",
		VectorType:     pb.VectorType_VECTOR_TYPE_IMAGE,
		EmbeddingModel: "test-embedding",
		InputHash:      meme.ContentHash,
		QdrantPointID:  "00000000-0000-0000-0000-000000000001",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	if err := vectorRepo.Create(context.Background(), oldVector); err != nil {
		t.Fatalf("failed to create old vector: %v", err)
	}

	annotation := &domain.MemeAnnotation{
		ID:            "annotation-existing",
		MemeID:        meme.ID,
		AnalyzerModel: "test-vlm",
		Description:   "开心表情",
		OCRText:       "你好",
		Labels: &pb.MemeAnnotationLabels{
			HasText: true,
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := repository.NewMemeAnnotationRepository(db).Create(context.Background(), annotation); err != nil {
		t.Fatalf("failed to create annotation: %v", err)
	}

	store := newMemoryObjectStorage()
	store.objects[meme.StorageKey] = testPNG1x1
	vlm := NewVLMService(&VLMConfig{
		Model:   "test-vlm",
		APIKey:  "test-key",
		BaseURL: "https://vlm.test/v1",
	})
	ingest := NewIngestService(
		repository.NewMemeRepository(db),
		vectorRepo,
		repository.NewMemeAnnotationRepository(db),
		nil,
		nil,
		store,
		vlm,
		fixedEmbeddingProvider{},
		nil,
		&IngestConfig{
			Workers:   1,
			BatchSize: 1,
			VectorIndexes: []IngestVectorIndex{
				{
					VectorType: pb.VectorType_VECTOR_TYPE_IMAGE,
					Collection: "image_collection",
					Embedding:  fixedEmbeddingProvider{},
				},
			},
		},
	)

	err = ingest.processItem(context.Background(), "test", &source.MemeItem{
		SourceID:  "existing-meme",
		LocalPath: imagePath,
		Format:    "png",
		Category:  "reaction",
		Tags:      []string{"happy"},
	}, &IngestOptions{Force: true})
	if err == nil {
		t.Fatal("processItem() error = nil, want replacement write failure")
	}

	var vector domain.MemeVector
	if err := db.First(&vector, "id = ?", oldVector.ID).Error; err != nil {
		t.Fatalf("old vector was removed after failed force replacement: %v", err)
	}
}

func TestNewIngestServiceFallbackIndexUsesConfiguredVectorType(t *testing.T) {
	t.Parallel()

	ingest := NewIngestService(
		nil,
		nil,
		nil,
		nil,
		&repository.QdrantRepository{},
		nil,
		nil,
		fixedEmbeddingProvider{},
		nil,
		&IngestConfig{
			Collection: "caption_collection",
			VectorType: pb.VectorType_VECTOR_TYPE_CAPTION,
		},
	)

	if len(ingest.indexes) != 1 {
		t.Fatalf("fallback indexes = %d, want 1", len(ingest.indexes))
	}
	if ingest.indexes[0].VectorType != pb.VectorType_VECTOR_TYPE_CAPTION {
		t.Fatalf("fallback vector type = %q, want %q", ingest.indexes[0].VectorType, pb.VectorType_VECTOR_TYPE_CAPTION)
	}
}

func TestMissingVectorIndexesForceKeepsExistingVectorRecords(t *testing.T) {
	t.Parallel()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&domain.MemeVector{}); err != nil {
		t.Fatalf("failed to migrate meme_vectors: %v", err)
	}

	repo := repository.NewMemeVectorRepository(db)
	ctx := context.Background()
	existing := &domain.MemeVector{
		ID:             "vector-caption",
		MemeID:         "meme-1",
		Collection:     "caption_collection",
		VectorType:     pb.VectorType_VECTOR_TYPE_CAPTION,
		EmbeddingModel: "test-embedding",
		InputHash:      "old-caption-hash",
		QdrantPointID:  "00000000-0000-0000-0000-000000000001",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	if err := repo.Create(ctx, existing); err != nil {
		t.Fatalf("failed to create existing vector: %v", err)
	}

	ingest := &IngestService{
		vectorRepo: repo,
		indexes: []IngestVectorIndex{
			{
				VectorType: pb.VectorType_VECTOR_TYPE_CAPTION,
				Collection: "caption_collection",
			},
		},
	}

	missing, err := ingest.missingVectorIndexes(ctx, "meme-1", true)
	if err != nil {
		t.Fatalf("missingVectorIndexes() error = %v", err)
	}
	if len(missing) != 1 {
		t.Fatalf("missingVectorIndexes() returned %d indexes, want 1", len(missing))
	}

	exists, err := repo.ExistsByMemeIDCollectionAndVectorType(ctx, "meme-1", "caption_collection", pb.VectorType_VECTOR_TYPE_CAPTION)
	if err != nil {
		t.Fatalf("ExistsByMemeIDCollectionAndVectorType() error = %v", err)
	}
	if !exists {
		t.Fatal("existing vector record was removed before replacement")
	}
}

func TestProcessItemReturnsNoOpWhenOnlyMissingCaptionVectorAndVLMFails(t *testing.T) {
	t.Parallel()

	// Regression test for a retry-path semantic bug:
	// when a meme is hash-hit (full re-import path), the only missing route
	// is caption, AND VLM analysis fails (so caption text is empty), the
	// caption route is correctly `errSkipOptionalVectorIndex`-skipped. The
	// previous implementation then returned `no vector indexes were written`
	// as an error, polluting the failure metric for items that were already
	// in a stable state with no useful retry work to do.

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&domain.Meme{}, &domain.MemeVector{}, &domain.MemeAnnotation{}); err != nil {
		t.Fatalf("failed to migrate test database: %v", err)
	}

	imagePath := filepath.Join(t.TempDir(), "meme.png")
	if err := os.WriteFile(imagePath, testPNG1x1, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	contentHash := calculateMD5(testPNG1x1)
	memeID := "meme-hashhit"
	storageKey := contentHash[:2] + "/" + contentHash + ".png"

	memeRepo := repository.NewMemeRepository(db)
	if err := memeRepo.Create(context.Background(), &domain.Meme{
		ID:          memeID,
		StorageKey:  storageKey,
		ContentHash: contentHash,
		ImageInfo: &pb.ImageInfo{
			Width:  1,
			Height: 1,
			Format: pb.ImageFormat_IMAGE_FORMAT_PNG,
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("failed to seed meme: %v", err)
	}

	vectorRepo := repository.NewMemeVectorRepository(db)
	if err := vectorRepo.Create(context.Background(), &domain.MemeVector{
		ID:             "vector-image-existing",
		MemeID:         memeID,
		Collection:     "image_collection",
		VectorType:     pb.VectorType_VECTOR_TYPE_IMAGE,
		EmbeddingModel: "fixed-test-embedding",
		InputHash:      contentHash,
		QdrantPointID:  "00000000-0000-0000-0000-000000000001",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}); err != nil {
		t.Fatalf("failed to seed image vector: %v", err)
	}

	qdrantRepo, fakeQdrant := newTestQdrantRepository(t)
	store := newMemoryObjectStorage()
	store.objects[storageKey] = testPNG1x1

	vlm := NewVLMService(&VLMConfig{
		Model:   "test-vlm",
		APIKey:  "test-key",
		BaseURL: "https://vlm.test/v1",
	})
	vlm.client.SetTransport(roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return jsonResponse(t, http.StatusInternalServerError, openAIResponse{}), nil
	}))

	ingest := NewIngestService(
		memeRepo,
		vectorRepo,
		repository.NewMemeAnnotationRepository(db),
		nil,
		qdrantRepo,
		store,
		vlm,
		fixedEmbeddingProvider{},
		nil,
		&IngestConfig{
			Workers:   1,
			BatchSize: 1,
			VectorIndexes: []IngestVectorIndex{
				{
					VectorType: pb.VectorType_VECTOR_TYPE_IMAGE,
					Collection: "image_collection",
					Embedding:  fixedEmbeddingProvider{},
					QdrantRepo: qdrantRepo,
				},
				{
					VectorType: pb.VectorType_VECTOR_TYPE_CAPTION,
					Collection: "caption_collection",
					Embedding:  fixedEmbeddingProvider{},
					QdrantRepo: qdrantRepo,
				},
			},
		},
	)

	err = ingest.processItem(context.Background(), "test", &source.MemeItem{
		SourceID:  "different-source-same-image",
		LocalPath: imagePath,
		Format:    "png",
	}, &IngestOptions{})
	if err != nil {
		t.Fatalf("processItem() error = %v, want nil (no-op skip)", err)
	}

	var vectors []domain.MemeVector
	if err := db.Find(&vectors).Error; err != nil {
		t.Fatalf("failed to load vectors: %v", err)
	}
	if len(vectors) != 1 {
		t.Fatalf("vector count = %d, want 1 (only the seeded image vector should remain)", len(vectors))
	}
	if vectors[0].VectorType != pb.VectorType_VECTOR_TYPE_IMAGE {
		t.Fatalf("vector type = %q, want VECTOR_TYPE_IMAGE", vectors[0].VectorType)
	}

	if fakeQdrant.upserts != 0 {
		t.Fatalf("qdrant upserts = %d, want 0 (caption route should be skipped, image already exists)", fakeQdrant.upserts)
	}

	var memeCount int64
	if err := db.Model(&domain.Meme{}).Count(&memeCount).Error; err != nil {
		t.Fatalf("count memes: %v", err)
	}
	if memeCount != 1 {
		t.Fatalf("meme count = %d, want 1 (existing meme must remain, no new meme created)", memeCount)
	}
}

var testPNG1x1 = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
	0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53,
	0xde, 0x00, 0x00, 0x00, 0x0c, 0x49, 0x44, 0x41,
	0x54, 0x08, 0xd7, 0x63, 0xf8, 0xcf, 0xc0, 0x00,
	0x00, 0x03, 0x01, 0x01, 0x00, 0xc9, 0xfe, 0x92,
	0xef, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e,
	0x44, 0xae, 0x42, 0x60, 0x82,
}

type fixedEmbeddingProvider struct{}

func (fixedEmbeddingProvider) Embed(context.Context, string) ([]float32, error) {
	return []float32{0.1, 0.2}, nil
}

func (fixedEmbeddingProvider) EmbedBatch(context.Context, []string) ([][]float32, error) {
	return [][]float32{{0.1, 0.2}}, nil
}

func (fixedEmbeddingProvider) EmbedQuery(context.Context, string) ([]float32, error) {
	return []float32{0.1, 0.2}, nil
}

func (fixedEmbeddingProvider) EmbedDocument(context.Context, EmbeddingDocument) ([]float32, error) {
	return []float32{0.1, 0.2}, nil
}

func (fixedEmbeddingProvider) GetModel() string {
	return "fixed-test-embedding"
}

func (fixedEmbeddingProvider) GetDimensions() int {
	return 2
}

type memoryObjectStorage struct {
	objects     map[string][]byte
	deleteCount int
}

func newMemoryObjectStorage() *memoryObjectStorage {
	return &memoryObjectStorage{objects: make(map[string][]byte)}
}

func (s *memoryObjectStorage) EnsureBucket(context.Context) error {
	return nil
}

func (s *memoryObjectStorage) Upload(_ context.Context, key string, reader io.Reader, _ int64, _ string) error {
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	s.objects[key] = data
	return nil
}

func (s *memoryObjectStorage) Download(_ context.Context, key string) (io.ReadCloser, error) {
	data, ok := s.objects[key]
	if !ok {
		return nil, fmt.Errorf("object not found: %s", key)
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (s *memoryObjectStorage) GetURL(key string) string {
	return "https://storage.test/" + key
}

func (s *memoryObjectStorage) Delete(_ context.Context, key string) error {
	delete(s.objects, key)
	s.deleteCount++
	return nil
}

func (s *memoryObjectStorage) Exists(_ context.Context, key string) (bool, error) {
	_, ok := s.objects[key]
	return ok, nil
}

type testQdrantServer struct {
	qdrant.UnimplementedPointsServer
	upserts int
	deletes int
}

func (s *testQdrantServer) Upsert(context.Context, *qdrant.UpsertPoints) (*qdrant.PointsOperationResponse, error) {
	s.upserts++
	return &qdrant.PointsOperationResponse{}, nil
}

func (s *testQdrantServer) Delete(context.Context, *qdrant.DeletePoints) (*qdrant.PointsOperationResponse, error) {
	s.deletes++
	return &qdrant.PointsOperationResponse{}, nil
}

func newTestQdrantRepository(t *testing.T) (*repository.QdrantRepository, *testQdrantServer) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen for fake qdrant: %v", err)
	}

	server := grpc.NewServer()
	fake := &testQdrantServer{}
	qdrant.RegisterPointsServer(server, fake)
	go func() {
		_ = server.Serve(listener)
	}()
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
	})

	host, portValue, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatalf("failed to split fake qdrant address: %v", err)
	}
	port, err := strconv.Atoi(portValue)
	if err != nil {
		t.Fatalf("failed to parse fake qdrant port: %v", err)
	}

	repo, err := repository.NewQdrantRepository(&repository.QdrantConnectionConfig{
		Host:       host,
		Port:       port,
		Collection: "test_collection",
	})
	if err != nil {
		t.Fatalf("failed to create qdrant repository: %v", err)
	}
	t.Cleanup(func() {
		_ = repo.Close()
	})
	return repo, fake
}
