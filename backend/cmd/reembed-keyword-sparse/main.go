package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	pb "github.com/timmy/emomo/gen/emomo/v1"
	"github.com/timmy/emomo/internal/config"
	"github.com/timmy/emomo/internal/domain"
	"github.com/timmy/emomo/internal/logger"
	"github.com/timmy/emomo/internal/persistence"
	"github.com/timmy/emomo/internal/repository"
	"github.com/timmy/emomo/internal/service"
	"github.com/timmy/emomo/internal/storage"
	"gorm.io/gorm"
)

type keywordCandidate struct {
	ID           string
	StorageKey   string
	ContentHash  string
	Tags         domain.StringArray
	Category     string
	AnnotationID string
	Description  string
	OCRText      string
	HasText      bool
}

type runStats struct {
	scanned             int64
	planned             int64
	written             int64
	skippedEmptyBM25    int64
	skippedNoStorage    int64
	failed              int64
	rollbackFailures    int64
	firstFailureMessage string
	mu                  sync.Mutex
}

func main() {
	config.LoadDotEnv()

	configPath := flag.String("config", "", "Path to config file (defaults to ./configs/config.yaml)")
	profileName := flag.String("profile", "qwen3vl", "Search profile name whose keyword_embedding collection should be backfilled")
	limit := flag.Int("limit", 0, "Maximum missing memes to process; 0 = no limit")
	workers := flag.Int("workers", 4, "Number of concurrent workers")
	retries := flag.Int("retries", 3, "Retry attempts for transient Qdrant writes")
	retryDelay := flag.Duration("retry-delay", 2*time.Second, "Initial delay between transient Qdrant write retries")
	dryRun := flag.Bool("dry-run", false, "Plan only: do not write Qdrant or Postgres")
	checkStorage := flag.Bool("check-storage", true, "Verify the object exists before writing a sparse point")
	flag.Parse()

	log := logger.NewServiceFromEnv("emomo-reembed-keyword-sparse")
	logger.SetDefaultLogger(log)
	defer logger.Sync()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.WithError(err).Fatal("Failed to load config")
	}
	cfg.Database.AutoMigrate = false

	db, err := repository.InitDB(&cfg.Database)
	if err != nil {
		log.WithError(err).Fatal("Failed to initialize database")
	}
	vectorRepo := repository.NewMemeVectorRepository(db)

	profile := cfg.GetSearchProfileByName(*profileName)
	if profile == nil {
		log.WithField("profile", *profileName).Fatal("Unknown search profile")
	}
	keywordEmbeddingName := profile.KeywordEmbedding
	if keywordEmbeddingName == "" {
		keywordEmbeddingName = profile.CaptionEmbedding
	}
	if keywordEmbeddingName == "" {
		log.WithField("profile", *profileName).Fatal("Profile has neither keyword_embedding nor caption_embedding")
	}
	keywordEmbedding := cfg.GetEmbeddingByName(keywordEmbeddingName)
	if keywordEmbedding == nil {
		log.WithField("embedding", keywordEmbeddingName).Fatal("Unknown keyword embedding")
	}
	keywordCollection := keywordEmbedding.GetCollection(cfg.Qdrant.Collection)

	qdrantRepo, err := repository.NewQdrantRepository(&repository.QdrantConnectionConfig{
		Host:            cfg.Qdrant.Host,
		Port:            cfg.Qdrant.Port,
		Collection:      keywordCollection,
		APIKey:          cfg.Qdrant.APIKey,
		UseTLS:          cfg.Qdrant.UseTLS,
		VectorDimension: keywordEmbedding.Dimensions,
	})
	if err != nil {
		log.WithError(err).Fatal("Failed to initialize Qdrant repository")
	}
	defer qdrantRepo.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Warn("Received shutdown signal, canceling...")
		cancel()
	}()

	if err := qdrantRepo.EnsureCollection(ctx); err != nil {
		log.WithError(err).Fatal("Failed to ensure Qdrant collection")
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
		log.WithError(err).Fatal("Failed to initialize storage")
	}

	candidates, err := loadKeywordCandidates(ctx, db, keywordCollection, *limit)
	if err != nil {
		log.WithError(err).Fatal("Failed to load keyword sparse candidates")
	}
	log.WithFields(logger.Fields{
		"profile":       *profileName,
		"embedding":     keywordEmbeddingName,
		"collection":    keywordCollection,
		"candidates":    len(candidates),
		"limit":         *limit,
		"workers":       *workers,
		"retries":       *retries,
		"retry_delay":   retryDelay.String(),
		"dry_run":       *dryRun,
		"check_storage": *checkStorage,
	}).Info("Loaded keyword sparse candidates")

	stats := processCandidates(ctx, candidates, processDeps{
		vectorRepo:    vectorRepo,
		qdrantRepo:    qdrantRepo,
		objectStorage: objectStorage,
		collection:    keywordCollection,
		dryRun:        *dryRun,
		checkStorage:  *checkStorage,
		workers:       *workers,
		retries:       *retries,
		retryDelay:    *retryDelay,
	})

	entry := log.WithFields(logger.Fields{
		"scanned":            stats.scanned,
		"planned":            stats.planned,
		"written":            stats.written,
		"skipped_empty_bm25": stats.skippedEmptyBM25,
		"skipped_no_storage": stats.skippedNoStorage,
		"failed":             stats.failed,
		"rollback_failures":  stats.rollbackFailures,
		"dry_run":            *dryRun,
	})
	if stats.firstFailureMessage != "" {
		entry = entry.WithField("first_failure", stats.firstFailureMessage)
	}
	entry.Info("Keyword sparse reembed completed")
	if stats.failed > 0 && !*dryRun {
		logger.Sync()
		os.Exit(1)
	}
}

func loadKeywordCandidates(ctx context.Context, db *gorm.DB, collection string, limit int) ([]keywordCandidate, error) {
	query, args := buildKeywordCandidateQuery(collection, limit)

	var candidates []keywordCandidate
	if err := db.WithContext(ctx).Raw(query, args...).Scan(&candidates).Error; err != nil {
		return nil, err
	}
	return candidates, nil
}

func buildKeywordCandidateQuery(collection string, limit int) (string, []any) {
	query := `
SELECT
	m.id,
	m.storage_key,
	m.content_hash,
	m.tags,
	m.category,
	COALESCE(a.id, '') AS annotation_id,
	COALESCE(a.description, '') AS description,
	COALESCE(a.ocr_text, '') AS ocr_text,
	COALESCE((a.labels::jsonb ->> 'hasText')::boolean, (a.labels::jsonb ->> 'has_text')::boolean, false) AS has_text
FROM memes m
LEFT JOIN LATERAL (
	SELECT id, description, ocr_text, labels
	FROM meme_annotations ma
	WHERE ma.meme_id = m.id
	ORDER BY ma.updated_at DESC
	LIMIT 1
) a ON true
WHERE m.storage_key <> ''
	AND NOT EXISTS (
		SELECT 1
		FROM meme_vectors mv
		WHERE mv.meme_id = m.id
			AND mv.collection = ?
			AND mv.vector_type IN (?, ?)
	)
ORDER BY m.created_at DESC`
	args := []any{
		collection,
		int32(pb.VectorType_VECTOR_TYPE_CAPTION),
		int32(pb.VectorType_VECTOR_TYPE_KEYWORD),
	}
	if limit > 0 {
		query += "\nLIMIT ?"
		args = append(args, limit)
	}
	return query, args
}

type processDeps struct {
	vectorRepo    *repository.MemeVectorRepository
	qdrantRepo    *repository.QdrantRepository
	objectStorage storage.ObjectStorage
	collection    string
	dryRun        bool
	checkStorage  bool
	workers       int
	retries       int
	retryDelay    time.Duration
}

func processCandidates(ctx context.Context, candidates []keywordCandidate, deps processDeps) *runStats {
	if deps.workers <= 0 {
		deps.workers = 1
	}
	jobs := make(chan keywordCandidate, deps.workers*2)
	stats := &runStats{}

	var wg sync.WaitGroup
	for i := 0; i < deps.workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for candidate := range jobs {
				processCandidate(ctx, candidate, deps, stats)
			}
		}()
	}

	for _, candidate := range candidates {
		if ctx.Err() != nil {
			break
		}
		jobs <- candidate
	}
	close(jobs)
	wg.Wait()
	return stats
}

func processCandidate(ctx context.Context, candidate keywordCandidate, deps processDeps, stats *runStats) {
	atomic.AddInt64(&stats.scanned, 1)

	ocrText := service.NormalizeOCRText(candidate.OCRText)
	description := service.CompactDescription(candidate.Description)
	bm25Text := service.BuildBM25Text(ocrText, description, []string(candidate.Tags))
	if strings.TrimSpace(bm25Text) == "" {
		atomic.AddInt64(&stats.skippedEmptyBM25, 1)
		return
	}
	atomic.AddInt64(&stats.planned, 1)

	if deps.checkStorage {
		exists, err := deps.objectStorage.Exists(ctx, candidate.StorageKey)
		if err != nil {
			recordFailure(stats, fmt.Errorf("storage check failed for meme %s: %w", candidate.ID, err))
			return
		}
		if !exists {
			atomic.AddInt64(&stats.skippedNoStorage, 1)
			return
		}
	}

	if deps.dryRun {
		return
	}

	pointID := uuid.New().String()
	payload := &repository.MemePayload{
		MemeID:         candidate.ID,
		Category:       candidate.Category,
		Tags:           []string(candidate.Tags),
		VLMDescription: candidate.Description,
		OCRText:        ocrText,
		TextPresence:   textPresenceString(candidate),
		StorageURL:     deps.objectStorage.GetURL(candidate.StorageKey),
	}

	if err := withRetry(ctx, deps.retries, deps.retryDelay, func() error {
		return deps.qdrantRepo.UpsertSparse(ctx, pointID, bm25Text, payload)
	}); err != nil {
		recordFailure(stats, fmt.Errorf("qdrant upsert failed for meme %s: %w", candidate.ID, err))
		return
	}

	vectorRecord := &domain.MemeVector{
		ID:             uuid.New().String(),
		MemeID:         candidate.ID,
		Collection:     deps.collection,
		VectorType:     pb.VectorType_VECTOR_TYPE_KEYWORD,
		EmbeddingModel: repository.SparseVectorModel,
		InputHash:      calculateTextSHA256(bm25Text),
		AnnotationID:   nullableString(candidate.AnnotationID),
		QdrantPointID:  pointID,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	if err := deps.vectorRepo.Create(ctx, vectorRecord); err != nil {
		if delErr := withRetry(ctx, deps.retries, deps.retryDelay, func() error {
			return deps.qdrantRepo.Delete(ctx, pointID)
		}); delErr != nil {
			atomic.AddInt64(&stats.rollbackFailures, 1)
		}
		recordFailure(stats, fmt.Errorf("meme_vectors insert failed for meme %s: %w", candidate.ID, err))
		return
	}

	atomic.AddInt64(&stats.written, 1)
}

func withRetry(ctx context.Context, retries int, initialDelay time.Duration, fn func() error) error {
	if retries < 0 {
		retries = 0
	}
	if initialDelay <= 0 {
		initialDelay = time.Second
	}

	var err error
	delay := initialDelay
	for attempt := 0; attempt <= retries; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		err = fn()
		if err == nil {
			return nil
		}
		if attempt == retries {
			return err
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		delay *= 2
	}
	return err
}

func textPresenceString(candidate keywordCandidate) string {
	if candidate.AnnotationID == "" {
		return persistence.TextPresenceToString(pb.TextPresence_TEXT_PRESENCE_UNKNOWN)
	}
	if candidate.HasText {
		return persistence.TextPresenceToString(pb.TextPresence_TEXT_PRESENCE_WITH_TEXT)
	}
	return persistence.TextPresenceToString(pb.TextPresence_TEXT_PRESENCE_WITHOUT_TEXT)
}

func nullableString(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return value
}

func calculateTextSHA256(text string) string {
	hash := sha256.Sum256([]byte(text))
	return hex.EncodeToString(hash[:])
}

func recordFailure(stats *runStats, err error) {
	atomic.AddInt64(&stats.failed, 1)
	if err == nil {
		return
	}
	stats.mu.Lock()
	defer stats.mu.Unlock()
	if stats.firstFailureMessage == "" {
		stats.firstFailureMessage = err.Error()
	}
}
