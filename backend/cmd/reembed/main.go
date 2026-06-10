// reembed re-creates Qdrant points for memes that already exist in Postgres,
// using a chosen embedding profile.
//
// Use case: a new embedding model/collection has been added to config.yaml
// (e.g. emomo_jina_v4) but the meme records and their VLM descriptions are
// already in Postgres. This tool walks the memes table, reuses the existing
// VLM description (no Gemini calls), asks the configured embedding provider
// to embed the meme image (Jina v4 image mode = pass the R2 URL), upserts
// the resulting dense + BM25 vector into the target Qdrant collection, and
// writes a meme_vectors row so that subsequent runs skip the meme.
//
// Example:
//
//	go run ./cmd/reembed --embedding jina --limit 5 --workers 4
//	go run ./cmd/reembed --embedding jina --workers 8        # full backfill
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pb "github.com/timmy/emomo/gen/emomo/v1"
	"github.com/timmy/emomo/internal/config"
	"github.com/timmy/emomo/internal/domain"
	"github.com/timmy/emomo/internal/logger"
	"github.com/timmy/emomo/internal/persistence"
	"github.com/timmy/emomo/internal/repository"
	"github.com/timmy/emomo/internal/service"
	"github.com/timmy/emomo/internal/storage"
)

func main() {
	config.LoadDotEnv()
	appLogger := logger.NewServiceFromEnv("emomo-reembed")
	logger.SetDefaultLogger(appLogger)
	defer logger.Sync()

	configPath := flag.String("config", "", "Path to config file (defaults to ./configs/config.yaml)")
	embeddingName := flag.String("embedding", "", "Embedding config name (e.g. 'jina'). Defaults to the config's default embedding")
	profileName := flag.String("profile", "", "Search profile name for multi-vector backfill (e.g. 'qwen3vl')")
	vectorType := flag.String("vector-type", "all", "Vector type to backfill when using --profile: image, keyword, caption, or all")
	limit := flag.Int("limit", 0, "Maximum memes to (re)embed; 0 = no limit")
	workers := flag.Int("workers", 4, "Number of concurrent workers")
	dryRun := flag.Bool("dry-run", false, "Plan only: count memes that would be embedded but do not call any APIs")
	force := flag.Bool("force", false, "Re-embed even if a meme_vectors row already exists for the target collection")
	autoMigrate := flag.Bool("auto-migrate", false, "Run database auto-migrations before reembed")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		appLogger.WithError(err).Fatal("Failed to load config")
	}
	cfg.Database.AutoMigrate = *autoMigrate

	db, err := repository.InitDB(&cfg.Database)
	if err != nil {
		appLogger.WithError(err).Fatal("Failed to initialize database")
	}

	memeRepo := repository.NewMemeRepository(db)
	vectorRepo := repository.NewMemeVectorRepository(db)
	annotationRepo := repository.NewMemeAnnotationRepository(db)

	embeddingRegistry, err := service.NewEmbeddingRegistry(&service.EmbeddingRegistryConfig{
		Embeddings:        cfg.Embeddings,
		QdrantHost:        cfg.Qdrant.Host,
		QdrantPort:        cfg.Qdrant.Port,
		QdrantAPIKey:      cfg.Qdrant.APIKey,
		QdrantUseTLS:      cfg.Qdrant.UseTLS,
		DefaultCollection: cfg.Qdrant.Collection,
		Logger:            appLogger,
	})
	if err != nil {
		appLogger.WithError(err).Fatal("Failed to initialize embedding registry")
	}
	defer embeddingRegistry.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := embeddingRegistry.EnsureCollections(ctx); err != nil {
		appLogger.WithError(err).Fatal("Failed to ensure Qdrant collections")
	}

	storageCfg := cfg.GetStorageConfig()
	objectStorage, err := storage.NewStorage(&storage.S3Config{
		Type:      storage.StorageType(storageCfg.Type),
		Endpoint:  storageCfg.Endpoint,
		AccessKey: storageCfg.AccessKey,
		SecretKey: storageCfg.SecretKey,
		UseSSL:    storageCfg.UseSSL,
		Bucket:    storageCfg.Bucket,
		Region:    storageCfg.Region,
		PublicURL: storageCfg.PublicURL,
	})
	if err != nil {
		appLogger.WithError(err).Fatal("Failed to initialize storage")
	}

	activeProfileName := *profileName
	if activeProfileName == "" && *embeddingName == "" {
		if defaultProfile := cfg.GetDefaultSearchProfile(); defaultProfile != nil {
			activeProfileName = defaultProfile.Name
		}
	}
	vectorIndexes := buildReembedVectorIndexes(cfg, embeddingRegistry, activeProfileName, *embeddingName, *vectorType, appLogger)
	modeName := activeProfileName
	if modeName == "" {
		modeName = *embeddingName
	}
	if modeName == "" {
		modeName = embeddingRegistry.DefaultName()
	}

	appLogger.WithFields(logger.Fields{
		"mode":           modeName,
		"profile":        activeProfileName,
		"embedding":      *embeddingName,
		"vector_type":    *vectorType,
		"vector_indexes": len(vectorIndexes),
		"limit":          *limit,
		"workers":        *workers,
		"dry_run":        *dryRun,
		"force":          *force,
		"auto_migrate":   *autoMigrate,
	}).Info("Starting reembed")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		appLogger.Warn("Received shutdown signal, canceling...")
		cancel()
	}()

	w := &worker{
		log:            appLogger,
		memeRepo:       memeRepo,
		vectorRepo:     vectorRepo,
		annotationRepo: annotationRepo,
		objectStorage:  objectStorage,
		vectorIndexes:  vectorIndexes,
		dryRun:         *dryRun,
		force:          *force,
	}

	stats, err := w.run(ctx, *limit, *workers)
	if err != nil && !errors.Is(err, context.Canceled) {
		appLogger.WithError(err).Fatal("reembed failed")
	}

	appLogger.WithFields(logger.Fields{
		"scanned":         stats.Scanned,
		"skipped_existed": stats.SkippedExisted,
		"skipped_no_url":  stats.SkippedNoURL,
		"reembedded":      stats.Reembedded,
		"failed":          stats.Failed,
		"mode":            modeName,
	}).Info("Reembed completed")
}

func buildReembedVectorIndexes(
	cfg *config.Config,
	registry *service.EmbeddingRegistry,
	profileName string,
	embeddingName string,
	vectorType string,
	log *logger.Logger,
) []service.IngestVectorIndex {
	if profileName == "" && embeddingName == "" {
		if defaultProfile := cfg.GetDefaultSearchProfile(); defaultProfile != nil {
			profileName = defaultProfile.Name
		}
	}
	if profileName != "" {
		profile := cfg.GetSearchProfileByName(profileName)
		if profile == nil {
			log.WithField("profile", profileName).Fatal("Unknown search profile")
		}
		indexes, err := registry.BuildProfileIngestIndexes(profile)
		if err != nil {
			log.WithError(err).Fatal("Failed to build profile ingest indexes")
		}
		return filterVectorIndexes(indexes, vectorType, log)
	}

	name := embeddingName
	if name == "" {
		name = registry.DefaultName()
	}
	provider, qdrantRepo, ok := registry.Get(name)
	if !ok {
		log.WithField("embedding", name).Fatal("Unknown embedding configuration name")
	}
	embCfg, _ := registry.GetConfig(name)
	resolvedType := pb.VectorType_VECTOR_TYPE_CAPTION
	useSparse := true
	if embCfg.GetDocumentMode() == "image" {
		resolvedType = pb.VectorType_VECTOR_TYPE_IMAGE
		useSparse = false
	}
	return []service.IngestVectorIndex{
		{
			VectorType: resolvedType,
			Collection: qdrantRepo.GetCollectionName(),
			Embedding:  provider,
			QdrantRepo: qdrantRepo,
			UseSparse:  useSparse,
		},
	}
}

func filterVectorIndexes(indexes []service.IngestVectorIndex, vectorType string, log *logger.Logger) []service.IngestVectorIndex {
	switch vectorType {
	case "", "all":
		return indexes
	case "image", "caption", "keyword":
		requestedType, err := persistence.ParseVectorType(vectorType)
		if err != nil {
			log.WithError(err).WithField("vector_type", vectorType).Fatal("Unsupported vector type; use image, caption, keyword, or all")
		}
		filtered := make([]service.IngestVectorIndex, 0, len(indexes))
		for _, index := range indexes {
			if index.VectorType == requestedType {
				filtered = append(filtered, index)
			}
		}
		if len(filtered) == 0 {
			log.WithField("vector_type", vectorType).Fatal("Profile does not contain requested vector type")
		}
		return filtered
	default:
		log.WithField("vector_type", vectorType).Fatal("Unsupported vector type; use image, caption, keyword, or all")
		return nil
	}
}

// =============================================================================
// Worker
// =============================================================================

type worker struct {
	log            *logger.Logger
	memeRepo       *repository.MemeRepository
	vectorRepo     *repository.MemeVectorRepository
	annotationRepo *repository.MemeAnnotationRepository
	objectStorage  storage.ObjectStorage
	vectorIndexes  []service.IngestVectorIndex
	dryRun         bool
	force          bool
}

type runStats struct {
	Scanned        int64
	SkippedExisted int64
	SkippedNoURL   int64
	Reembedded     int64
	Failed         int64
}

const pageSize = 200

// run streams stored memes page-by-page and feeds them through a fixed pool of
// workers. Each worker may concurrently call the embedding API and upsert into
// Qdrant; the user can throttle via --workers to respect provider rate limits.
func (w *worker) run(ctx context.Context, limit, workers int) (runStats, error) {
	if workers <= 0 {
		workers = 1
	}

	jobs := make(chan domain.Meme, workers*2)
	stats := runStats{}

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for meme := range jobs {
				if ctx.Err() != nil {
					return
				}
				w.processOne(ctx, meme, &stats)
			}
		}(i)
	}

	go func() {
		defer close(jobs)

		offset := 0
		emitted := 0
		for {
			if ctx.Err() != nil {
				return
			}

			memes, err := w.memeRepo.List(ctx, pageSize, offset)
			if err != nil {
				w.log.WithError(err).WithField("offset", offset).Error("Failed to list memes; aborting page")
				return
			}
			if len(memes) == 0 {
				return
			}

			for _, meme := range memes {
				if limit > 0 && emitted >= limit {
					return
				}
				select {
				case <-ctx.Done():
					return
				case jobs <- meme:
					emitted++
				}
			}

			if len(memes) < pageSize {
				return
			}
			offset += pageSize
		}
	}()

	wg.Wait()
	return stats, ctx.Err()
}

// processOne handles a single meme. It is called from a worker goroutine, so
// it talks to its own copy of `meme` and only mutates `stats` via atomics.
func (w *worker) processOne(ctx context.Context, meme domain.Meme, stats *runStats) {
	atomic.AddInt64(&stats.Scanned, 1)

	if meme.StorageKey == "" {
		atomic.AddInt64(&stats.SkippedNoURL, 1)
		w.log.WithField("meme_id", meme.ID).Warn("Meme has no storage_key; cannot derive image URL")
		return
	}
	existsInStorage, err := w.objectStorage.Exists(ctx, meme.StorageKey)
	if err != nil {
		atomic.AddInt64(&stats.Failed, 1)
		w.log.WithError(err).WithField("meme_id", meme.ID).Error("Failed to check storage object")
		return
	}
	if !existsInStorage {
		atomic.AddInt64(&stats.SkippedNoURL, 1)
		w.log.WithField("meme_id", meme.ID).Warn("Storage object is missing")
		return
	}
	imageURL := w.objectStorage.GetURL(meme.StorageKey)
	if imageURL == "" {
		atomic.AddInt64(&stats.SkippedNoURL, 1)
		w.log.WithField("meme_id", meme.ID).Warn("Storage backend returned empty URL")
		return
	}

	// Look up an existing annotation (any model) so we can populate the
	// Qdrant payload + BM25 sparse vector. Reembed never invokes the VLM.
	annotation := w.lookupAnnotation(ctx, meme.ID)
	vlmDescription := ""
	ocrText := ""
	annotationID := ""
	textPresence := pb.TextPresence_TEXT_PRESENCE_UNKNOWN
	if annotation != nil {
		vlmDescription = annotation.Description
		ocrText = service.NormalizeOCRText(annotation.OCRText)
		annotationID = annotation.ID
		textPresence = domain.TextPresenceFromLabels(annotation.Labels)
	}

	captionText := service.BuildCaptionEmbeddingText(
		ocrText,
		service.CompactDescription(vlmDescription),
		meme.Category,
		meme.Tags,
		service.ExtractEmotionWords(vlmDescription),
	)
	bm25Text := service.BuildBM25Text(ocrText, service.CompactDescription(vlmDescription), meme.Tags)
	payload := &repository.MemePayload{
		MemeID:         meme.ID,
		Category:       meme.Category,
		Tags:           meme.Tags,
		VLMDescription: vlmDescription,
		OCRText:        ocrText,
		TextPresence:   persistence.TextPresenceToString(textPresence),
		StorageURL:     imageURL,
	}

	if w.dryRun {
		planned := 0
		for _, index := range w.vectorIndexes {
			if w.shouldProcessIndex(ctx, meme, index, captionText, bm25Text, stats) {
				planned++
			}
		}
		atomic.AddInt64(&stats.Reembedded, int64(planned))
		w.log.WithFields(logger.Fields{
			"meme_id":         meme.ID,
			"content_hash":    meme.ContentHash,
			"image_url":       imageURL,
			"has_annotation":  annotation != nil,
			"planned_vectors": planned,
		}).Info("[dry-run] would re-embed meme")
		return
	}

	embedStart := time.Now()
	wrote := 0
	for _, index := range w.vectorIndexes {
		if !w.shouldProcessIndex(ctx, meme, index, captionText, bm25Text, stats) {
			continue
		}
		if err := w.processVectorIndex(ctx, meme, index, vectorPayloadInput{
			ImageURL:     imageURL,
			CaptionText:  captionText,
			BM25Text:     bm25Text,
			AnnotationID: annotationID,
			Payload:      payload,
		}); err != nil {
			atomic.AddInt64(&stats.Failed, 1)
			w.log.WithError(err).WithFields(logger.Fields{
				"meme_id":     meme.ID,
				"collection":  index.Collection,
				"vector_type": index.VectorType,
			}).Error("Failed to re-embed vector index")
			continue
		}
		wrote++
		atomic.AddInt64(&stats.Reembedded, 1)
	}

	if wrote == 0 {
		atomic.AddInt64(&stats.SkippedExisted, 1)
		return
	}
	w.log.WithFields(logger.Fields{
		"meme_id":     meme.ID,
		"vectors":     wrote,
		"duration_ms": time.Since(embedStart).Milliseconds(),
	}).Info("Re-embedded meme")
}

type vectorPayloadInput struct {
	ImageURL     string
	CaptionText  string
	BM25Text     string
	AnnotationID string
	Payload      *repository.MemePayload
}

func (w *worker) shouldProcessIndex(ctx context.Context, meme domain.Meme, index service.IngestVectorIndex, captionText string, bm25Text string, stats *runStats) bool {
	if index.VectorType == pb.VectorType_VECTOR_TYPE_CAPTION && captionText == "" {
		atomic.AddInt64(&stats.SkippedNoURL, 1)
		w.log.WithFields(logger.Fields{
			"meme_id":     meme.ID,
			"collection":  index.Collection,
			"vector_type": persistence.VectorTypeShortName(index.VectorType),
		}).Warn("Skipping caption vector because caption text is empty")
		return false
	}
	if index.VectorType == pb.VectorType_VECTOR_TYPE_KEYWORD && bm25Text == "" {
		atomic.AddInt64(&stats.SkippedNoURL, 1)
		w.log.WithFields(logger.Fields{
			"meme_id":     meme.ID,
			"collection":  index.Collection,
			"vector_type": persistence.VectorTypeShortName(index.VectorType),
		}).Warn("Skipping keyword vector because BM25 text is empty")
		return false
	}
	if !w.force {
		exists, err := w.vectorIndexExists(ctx, meme.ID, index)
		if err != nil {
			atomic.AddInt64(&stats.Failed, 1)
			w.log.WithError(err).WithFields(logger.Fields{
				"meme_id":     meme.ID,
				"collection":  index.Collection,
				"vector_type": persistence.VectorTypeShortName(index.VectorType),
			}).Error("Failed to check vector existence")
			return false
		}
		if exists {
			return false
		}
	}
	return true
}

func (w *worker) vectorIndexExists(ctx context.Context, memeID string, index service.IngestVectorIndex) (bool, error) {
	exists, err := w.vectorRepo.ExistsByMemeIDCollectionAndVectorType(ctx, memeID, index.Collection, index.VectorType)
	if err != nil || exists {
		return exists, err
	}
	if index.SparseOnly && index.VectorType == pb.VectorType_VECTOR_TYPE_KEYWORD {
		return w.vectorRepo.ExistsByMemeIDCollectionAndVectorType(ctx, memeID, index.Collection, pb.VectorType_VECTOR_TYPE_CAPTION)
	}
	return false, nil
}

func (w *worker) processVectorIndex(ctx context.Context, meme domain.Meme, index service.IngestVectorIndex, input vectorPayloadInput) error {
	if w.force {
		existing, err := w.vectorRepo.GetByMemeIDCollectionAndVectorType(ctx, meme.ID, index.Collection, index.VectorType)
		if err == nil && existing != nil {
			if delErr := index.QdrantRepo.Delete(ctx, existing.QdrantPointID); delErr != nil {
				w.log.WithError(delErr).WithField("point_id", existing.QdrantPointID).Warn("Failed to delete old Qdrant point before force reembed")
			}
			if delErr := w.vectorRepo.Delete(ctx, existing.ID); delErr != nil {
				return fmt.Errorf("failed to delete old vector record before force reembed: %w", delErr)
			}
		} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("failed to load old vector record before force reembed: %w", err)
		}
	}

	doc := service.EmbeddingDocument{}
	inputHash := meme.ContentHash
	switch index.VectorType {
	case pb.VectorType_VECTOR_TYPE_KEYWORD:
		inputHash = calculateTextSHA256(input.BM25Text)
	case pb.VectorType_VECTOR_TYPE_CAPTION:
		doc.Text = input.CaptionText
		inputHash = calculateTextSHA256(input.CaptionText)
	case pb.VectorType_VECTOR_TYPE_IMAGE:
		doc.ImageURL = input.ImageURL
	default:
		return fmt.Errorf("unsupported vector type: %s", persistence.VectorTypeShortName(index.VectorType))
	}

	pointID := uuid.New().String()
	embeddingModel := repository.SparseVectorModel
	if index.SparseOnly {
		if err := index.QdrantRepo.UpsertSparse(ctx, pointID, input.BM25Text, input.Payload); err != nil {
			return fmt.Errorf("UpsertSparse failed: %w", err)
		}
	} else {
		embedding, err := w.embedWithRetry(ctx, index.Embedding, doc, meme.ID)
		if err != nil {
			return fmt.Errorf("EmbedDocument failed after retries: %w", err)
		}
		embeddingModel = index.Embedding.GetModel()
		if index.UseSparse {
			if err := index.QdrantRepo.UpsertHybrid(ctx, pointID, embedding, input.BM25Text, input.Payload); err != nil {
				return fmt.Errorf("UpsertHybrid failed: %w", err)
			}
		} else {
			if err := index.QdrantRepo.Upsert(ctx, pointID, embedding, input.Payload); err != nil {
				return fmt.Errorf("Upsert failed: %w", err)
			}
		}
	}

	vectorRecord := &domain.MemeVector{
		ID:             uuid.New().String(),
		MemeID:         meme.ID,
		Collection:     index.Collection,
		VectorType:     index.VectorType,
		EmbeddingModel: embeddingModel,
		InputHash:      inputHash,
		AnnotationID:   input.AnnotationID,
		QdrantPointID:  pointID,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	if err := w.vectorRepo.Create(ctx, vectorRecord); err != nil {
		if delErr := index.QdrantRepo.Delete(ctx, pointID); delErr != nil {
			w.log.WithError(delErr).WithField("point_id", pointID).Error("Failed to roll back Qdrant point after meme_vectors insert failure")
		}
		return fmt.Errorf("meme_vectors insert failed: %w", err)
	}
	return nil
}

// embedWithRetry calls the embedding provider with exponential backoff for
// transient failures (HTTP 429 throttling and 5xx). Jina v4 in particular
// tends to return 429 under modest concurrency on the free tier; retrying
// after a short sleep is enough to recover instead of dropping the meme.
func (w *worker) embedWithRetry(ctx context.Context, provider service.EmbeddingProvider, doc service.EmbeddingDocument, memeID string) ([]float32, error) {
	const maxAttempts = 5
	backoff := 2 * time.Second

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		vec, err := provider.EmbedDocument(ctx, doc)
		if err == nil {
			if attempt > 1 {
				w.log.WithFields(logger.Fields{
					"meme_id": memeID,
					"attempt": attempt,
				}).Info("EmbedDocument succeeded after retry")
			}
			return vec, nil
		}
		lastErr = err
		if !isTransientEmbeddingError(err) || attempt == maxAttempts {
			return nil, err
		}
		wait := backoff
		w.log.WithError(err).WithFields(logger.Fields{
			"meme_id":   memeID,
			"attempt":   attempt,
			"sleep_sec": int(wait.Seconds()),
		}).Warn("Transient EmbedDocument failure; retrying")
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
		backoff *= 2
	}
	return nil, lastErr
}

func isTransientEmbeddingError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	if strings.Contains(msg, "429") {
		return true
	}
	if strings.Contains(msg, "status 5") { // 500/502/503/504
		return true
	}
	if strings.Contains(msg, "context deadline exceeded") || strings.Contains(msg, "timeout") {
		return true
	}
	return false
}

func calculateTextSHA256(text string) string {
	hash := sha256.Sum256([]byte(text))
	return hex.EncodeToString(hash[:])
}

// lookupAnnotation returns any existing annotation for the meme. It does
// not error out when none is found — the meme can still be embedded purely
// from its image; we just won't have OCR/desc to populate the payload.
func (w *worker) lookupAnnotation(ctx context.Context, memeID string) *domain.MemeAnnotation {
	annotations, err := w.annotationRepo.GetByMemeID(ctx, memeID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		w.log.WithError(err).WithField("meme_id", memeID).Warn("Failed to load meme annotation; continuing without it")
		return nil
	}
	if len(annotations) == 0 {
		return nil
	}
	// Prefer the most recently created annotation if multiple analyzer models exist.
	best := annotations[0]
	for i := 1; i < len(annotations); i++ {
		if annotations[i].CreatedAt.After(best.CreatedAt) {
			best = annotations[i]
		}
	}
	return &best
}
