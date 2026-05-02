package service

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/timmy/emomo/internal/domain"
	"github.com/timmy/emomo/internal/logger"
	"github.com/timmy/emomo/internal/repository"
	"github.com/timmy/emomo/internal/source"
	"github.com/timmy/emomo/internal/storage"
	_ "golang.org/x/image/webp"
	"gorm.io/gorm"
)

// IngestService handles the data ingestion pipeline.
type IngestService struct {
	memeRepo       *repository.MemeRepository
	vectorRepo     *repository.MemeVectorRepository
	annotationRepo *repository.MemeAnnotationRepository
	qdrantRepo     *repository.QdrantRepository
	storage        storage.ObjectStorage
	vlm            *VLMService
	embedding      EmbeddingProvider
	indexes        []IngestVectorIndex
	logger         *logger.Logger
	workers        int
	batchSize      int
	collection     string // Target Qdrant collection name
}

// IngestConfig holds configuration for the ingest service.
type IngestConfig struct {
	Workers       int
	BatchSize     int
	Collection    string // Target Qdrant collection name
	VectorType    domain.MemeVectorType
	VectorIndexes []IngestVectorIndex
}

// IngestVectorIndex describes one vector route to write during ingestion.
type IngestVectorIndex struct {
	VectorType domain.MemeVectorType
	Collection string
	Embedding  EmbeddingProvider
	QdrantRepo *repository.QdrantRepository
	UseSparse  bool
}

// NewIngestService creates a new ingest service.
// Parameters:
//   - memeRepo: repository for meme records.
//   - vectorRepo: repository for meme vectors.
//   - annotationRepo: repository for meme annotations.
//   - qdrantRepo: Qdrant repository for vector storage.
//   - objectStorage: object storage client for image files.
//   - vlm: vision-language model service for descriptions.
//   - embedding: embedding provider for vector generation.
//   - log: logger instance.
//   - cfg: ingest configuration settings.
//
// Returns:
//   - *IngestService: initialized ingest service.
func NewIngestService(
	memeRepo *repository.MemeRepository,
	vectorRepo *repository.MemeVectorRepository,
	annotationRepo *repository.MemeAnnotationRepository,
	qdrantRepo *repository.QdrantRepository,
	objectStorage storage.ObjectStorage,
	vlm *VLMService,
	embedding EmbeddingProvider,
	log *logger.Logger,
	cfg *IngestConfig,
) *IngestService {
	indexes := cfg.VectorIndexes
	if len(indexes) == 0 && qdrantRepo != nil && embedding != nil {
		vectorType := normalizeIngestVectorType(cfg.VectorType)
		indexes = []IngestVectorIndex{
			{
				VectorType: vectorType,
				Collection: cfg.Collection,
				Embedding:  embedding,
				QdrantRepo: qdrantRepo,
				UseSparse:  true,
			},
		}
	}

	return &IngestService{
		memeRepo:       memeRepo,
		vectorRepo:     vectorRepo,
		annotationRepo: annotationRepo,
		qdrantRepo:     qdrantRepo,
		storage:        objectStorage,
		vlm:            vlm,
		embedding:      embedding,
		indexes:        indexes,
		logger:         log,
		workers:        cfg.Workers,
		batchSize:      cfg.BatchSize,
		collection:     cfg.Collection,
	}
}

// log returns a logger from context if available, otherwise returns the default logger
func (s *IngestService) log(ctx context.Context) *logger.Logger {
	if l := logger.FromContext(ctx); l != nil {
		return l
	}
	return s.logger
}

// IngestStats holds statistics for an ingestion run.
type IngestStats struct {
	TotalItems     int64
	ProcessedItems int64
	SkippedItems   int64
	FailedItems    int64
	StartTime      time.Time
	EndTime        time.Time
}

// IngestOptions holds options for ingestion.
type IngestOptions struct {
	Force bool // If true, skip existence checks and force re-process
}

// IngestFromSource ingests memes from a data source.
// Parameters:
//   - ctx: context for cancellation and deadlines.
//   - src: data source implementation.
//   - limit: maximum number of items to ingest.
//   - opts: ingestion options (nil uses defaults).
//
// Returns:
//   - *IngestStats: statistics for the ingest run.
//   - error: non-nil if ingestion fails.
func (s *IngestService) IngestFromSource(ctx context.Context, src source.Source, limit int, opts *IngestOptions) (*IngestStats, error) {
	if opts == nil {
		opts = &IngestOptions{}
	}

	// Inject tracing fields into context
	ctx = logger.WithFields(ctx, logger.Fields{
		logger.FieldComponent: "ingest",
		logger.FieldJobID:     uuid.New().String(),
		logger.FieldSource:    src.GetSourceID(),
	})

	stats := &IngestStats{
		StartTime: time.Now(),
	}

	logger.CtxInfo(ctx, "Starting ingestion: source=%s, limit=%d, force=%v",
		src.GetSourceID(), limit, opts.Force)

	// Create work channel and results channel
	itemsChan := make(chan source.MemeItem, s.workers*2)
	resultsChan := make(chan *processResult, s.workers*2)

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < s.workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			s.worker(ctx, workerID, src.GetSourceID(), itemsChan, resultsChan, opts)
		}(i)
	}

	// Start result collector
	done := make(chan struct{})
	go func() {
		for result := range resultsChan {
			atomic.AddInt64(&stats.ProcessedItems, 1)
			if result.skipped {
				atomic.AddInt64(&stats.SkippedItems, 1)
			} else if result.err != nil {
				atomic.AddInt64(&stats.FailedItems, 1)
				logger.CtxError(ctx, "Failed to process item: source_id=%s, error=%v",
					result.sourceID, result.err)
			}
		}
		close(done)
	}()

	// Fetch items from source
	cursor := ""
	totalFetched := 0
	for {
		if ctx.Err() != nil {
			break
		}

		remaining := limit - totalFetched
		if remaining <= 0 {
			break
		}

		batchLimit := s.batchSize
		if batchLimit > remaining {
			batchLimit = remaining
		}

		items, nextCursor, err := src.FetchBatch(ctx, cursor, batchLimit)
		if err != nil {
			logger.CtxError(ctx, "Failed to fetch batch: error=%v", err)
			break
		}

		if len(items) == 0 {
			break
		}

		atomic.AddInt64(&stats.TotalItems, int64(len(items)))
		totalFetched += len(items)

		for _, item := range items {
			select {
			case itemsChan <- item:
			case <-ctx.Done():
				break
			}
		}

		if nextCursor == "" {
			break
		}
		cursor = nextCursor
	}

	// Close items channel and wait for workers
	close(itemsChan)
	wg.Wait()

	// Close results channel and wait for collector
	close(resultsChan)
	<-done

	stats.EndTime = time.Now()
	duration := stats.EndTime.Sub(stats.StartTime)

	logger.With(logger.Fields{
		logger.FieldDurationMs: duration.Milliseconds(),
		logger.FieldCount:      stats.ProcessedItems,
	}).Info(ctx, "Ingestion completed: total=%d, processed=%d, skipped=%d, failed=%d",
		stats.TotalItems, stats.ProcessedItems, stats.SkippedItems, stats.FailedItems)

	return stats, nil
}

type processResult struct {
	sourceID string
	skipped  bool
	err      error
}

// errSkipDuplicate is a sentinel error to indicate MD5 duplicate skip
var errSkipDuplicate = fmt.Errorf("skipped: duplicate MD5")

// errSkipUnsupportedImageFormat is a sentinel error for unsupported source images.
var errSkipUnsupportedImageFormat = errors.New("skipped: unsupported image format")

// errSkipOptionalVectorIndex is used when an auxiliary vector route has no input.
var errSkipOptionalVectorIndex = errors.New("skipped: optional vector index")

func (s *IngestService) worker(ctx context.Context, workerID int, sourceType string, items <-chan source.MemeItem, results chan<- *processResult, opts *IngestOptions) {
	for item := range items {
		select {
		case <-ctx.Done():
			return
		default:
		}

		result := &processResult{sourceID: item.SourceID}

		// Process the item with the new multi-embedding logic
		if err := s.processItem(ctx, sourceType, &item, opts); err != nil {
			if errors.Is(err, errSkipDuplicate) || errors.Is(err, errSkipUnsupportedImageFormat) {
				result.skipped = true
			} else {
				result.err = err
			}
		}

		results <- result
	}
}

func (s *IngestService) processItem(ctx context.Context, sourceType string, item *source.MemeItem, opts *IngestOptions) error {
	// Read image data
	imageData, err := s.readImage(item)
	if err != nil {
		return fmt.Errorf("failed to read image: %w", err)
	}

	// Detect actual image format from magic bytes (don't trust file extension)
	actualFormat := detectImageFormat(imageData)
	if actualFormat == "unknown" {
		actualFormat = item.Format // Fallback to extension if detection fails
	}

	if !isSupportedStaticImageFormat(actualFormat) {
		return fmt.Errorf("%w: %s", errSkipUnsupportedImageFormat, actualFormat)
	}

	// Convert WebP to JPEG for storage and VLM compatibility while preserving
	// the static-image-only resource policy.
	processedFormat := actualFormat
	if shouldConvertStaticImageToJPEG(actualFormat) {
		converted, err := convertToJPEG(imageData, actualFormat)
		if err != nil {
			return fmt.Errorf("failed to convert %s to JPEG: %w", actualFormat, err)
		}
		logger.CtxDebug(ctx, "Converted %s to JPEG: original_size=%d, converted_size=%d",
			actualFormat, len(imageData), len(converted))
		imageData = converted
		processedFormat = "jpeg"
	} else if actualFormat != item.Format {
		// Log when actual format differs from extension.
		logger.CtxDebug(ctx, "Format mismatch: extension=%s, actual=%s, using actual format",
			item.Format, actualFormat)
	}

	// Calculate content hash of the processed/converted image.
	contentHash := calculateMD5(imageData)

	// Check if we have an existing meme record for resource reuse.
	existingMeme, err := s.memeRepo.GetByContentHash(ctx, contentHash)
	hasExistingMeme := err == nil && existingMeme != nil
	targetIndexes := s.indexes
	if len(targetIndexes) == 0 {
		return fmt.Errorf("no ingest vector indexes configured")
	}
	if hasExistingMeme {
		targetIndexes, err = s.missingVectorIndexes(ctx, existingMeme.ID, opts.Force)
		if err != nil {
			return err
		}
		if len(targetIndexes) == 0 {
			return errSkipDuplicate
		}
	}

	var memeID string
	var storageKey string
	var storageURL string
	var vlmDescription string
	var ocrText string
	var annotationID string
	uploaded := false
	createdNewMeme := false       // Track if we created a new meme record for rollback
	createdNewAnnotation := false // Track if we created a new annotation record for rollback

	// rollbackMeme cleans up the meme record if we created one
	rollbackMeme := func() {
		if createdNewMeme && memeID != "" {
			if delErr := s.memeRepo.Delete(ctx, memeID); delErr != nil {
				logger.CtxError(ctx, "Failed to rollback meme record: meme_id=%s, error=%v", memeID, delErr)
			} else {
				logger.CtxDebug(ctx, "Rolled back meme record: meme_id=%s", memeID)
			}
		}
	}

	// rollbackAnnotation cleans up the annotation record if we created one
	rollbackAnnotation := func() {
		if createdNewAnnotation && annotationID != "" && s.annotationRepo != nil {
			if delErr := s.annotationRepo.Delete(ctx, annotationID); delErr != nil {
				logger.CtxError(ctx, "Failed to rollback annotation record: annotation_id=%s, error=%v", annotationID, delErr)
			} else {
				logger.CtxDebug(ctx, "Rolled back annotation record: annotation_id=%s", annotationID)
			}
		}
	}

	// rollbackStorage cleans up the storage upload if we uploaded
	rollbackStorage := func() {
		if uploaded {
			if delErr := s.storage.Delete(ctx, storageKey); delErr != nil {
				logger.CtxError(ctx, "Failed to rollback storage upload: storage_key=%s, error=%v", storageKey, delErr)
			} else {
				logger.CtxDebug(ctx, "Rolled back storage upload: storage_key=%s", storageKey)
			}
		}
	}

	if hasExistingMeme {
		// REUSE existing resources: S3 path
		memeID = existingMeme.ID
		storageKey = existingMeme.StorageKey
		storageURL = s.storage.GetURL(storageKey)

		logger.CtxInfo(ctx, "Reusing existing meme record: content_hash=%s, meme_id=%s, collection=%s",
			contentHash, memeID, s.collection)
	} else {
		// NEW meme: full processing pipeline
		memeID = uuid.New().String()

		// Get image dimensions
		width, height, err := getImageDimensions(imageData)
		if err != nil {
			logger.CtxWarn(ctx, "Failed to get image dimensions: error=%v", err)
			width, height = 0, 0
		}
		imageInfo := domain.ImageInfo{
			Width:  int32(width),
			Height: int32(height),
			Format: domain.ImageFormatFromString(processedFormat),
		}

		// Upload to storage (use MD5 prefix for bucketing)
		storageKey = fmt.Sprintf("%s/%s.%s", contentHash[:2], contentHash, processedFormat)
		contentType := getContentType(processedFormat)

		// Check if file already exists in storage
		existsInStorage, err := s.storage.Exists(ctx, storageKey)
		if err != nil {
			return fmt.Errorf("failed to check storage existence: %w", err)
		}

		if !existsInStorage {
			if err := s.storage.Upload(ctx, storageKey, bytes.NewReader(imageData), int64(len(imageData)), contentType); err != nil {
				return fmt.Errorf("failed to upload to storage: %w", err)
			}
			uploaded = true
		}

		storageURL = s.storage.GetURL(storageKey)

		// Create meme record without AI-derived annotation fields.
		meme := &domain.Meme{
			ID:          memeID,
			StorageKey:  storageKey,
			ContentHash: contentHash,
			ImageInfo:   imageInfo,
			Tags:        item.Tags,
			Category:    item.Category,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		// Save meme to database first
		if err := s.memeRepo.Upsert(ctx, meme); err != nil {
			// Rollback storage if we uploaded
			rollbackStorage()
			return fmt.Errorf("failed to save meme to database: %w", err)
		}
		createdNewMeme = true // Mark that we created a new meme record
	}

	annotation := s.prepareAnnotationBestEffort(ctx, memeID, contentHash, imageData, processedFormat)
	vlmDescription = annotation.Description
	ocrText = annotation.OCRText
	annotationID = annotation.ID
	ocrReliable := annotation.OCRReliable
	createdNewAnnotation = annotation.Created

	compactDesc := compactDescription(vlmDescription)
	captionText := buildCaptionEmbeddingText(
		ocrText,
		compactDesc,
		item.Category,
		item.Tags,
		extractEmotionWords(vlmDescription),
	)
	bm25Text := buildBM25Text(ocrText, compactDesc, item.Tags)
	payload := &repository.MemePayload{
		MemeID:         memeID,
		Category:       item.Category,
		Tags:           item.Tags,
		VLMDescription: vlmDescription,
		OCRText:        ocrText,
		TextPresence:   string(classifyTextPresence(ocrText, ocrReliable)),
		StorageURL:     storageURL,
	}

	if err := s.upsertVectorIndexes(ctx, targetIndexes, vectorUpsertInput{
		MemeID:         memeID,
		ContentHash:    contentHash,
		AnnotationID:   annotationID,
		Force:          opts.Force,
		ImageURL:       storageURL,
		ImageData:      imageData,
		ImageMediaType: getContentType(processedFormat),
		CaptionText:    captionText,
		BM25Text:       bm25Text,
		Payload:        payload,
	}); err != nil {
		if createdNewMeme {
			s.rollbackVectorIndexes(ctx, memeID, targetIndexes)
		}
		rollbackAnnotation()
		rollbackMeme()
		rollbackStorage()
		return err
	}

	logger.CtxDebug(ctx, "Successfully processed item: meme_id=%s, vectors=%d, reused=%v",
		memeID, len(targetIndexes), hasExistingMeme)

	return nil
}

func (s *IngestService) extractOCRText(ctx context.Context, imageData []byte, format string) (string, error) {
	if s.vlm == nil {
		return "", nil
	}
	text, err := s.vlm.ExtractOCRText(ctx, imageData, format)
	if err != nil {
		return "", err
	}
	return normalizeOCRText(text), nil
}

type annotationResult struct {
	ID          string
	Description string
	OCRText     string
	OCRReliable bool
	Created     bool
}

func (s *IngestService) prepareAnnotationBestEffort(ctx context.Context, memeID, contentHash string, imageData []byte, format string) annotationResult {
	result := annotationResult{}
	if s.vlm == nil {
		return result
	}

	analyzerModel := s.vlm.GetModel()
	if s.annotationRepo != nil && analyzerModel != "" {
		existingAnnotation, err := s.annotationRepo.GetByMemeIDAndModel(ctx, memeID, analyzerModel)
		if err == nil && existingAnnotation != nil {
			result.ID = existingAnnotation.ID
			result.Description = existingAnnotation.Description
			result.OCRText = normalizeOCRText(existingAnnotation.OCRText)
			result.OCRReliable = hasReliableTextPresence(existingAnnotation, result.OCRText)
			if shouldExtractOCRForAnnotation(existingAnnotation, result.OCRText) {
				ocrText, err := s.extractOCRText(ctx, imageData, format)
				if err != nil {
					logger.CtxWarn(ctx, "Failed to extract OCR text: content_hash=%s, error=%v", contentHash, err)
				} else {
					result.OCRText = ocrText
					result.OCRReliable = true
					if updateErr := s.annotationRepo.UpdateOCRText(ctx, existingAnnotation.ID, ocrText); updateErr != nil {
						logger.CtxWarn(ctx, "Failed to update OCR text: annotation_id=%s, error=%v", existingAnnotation.ID, updateErr)
					}
				}
			}
			logger.CtxDebug(ctx, "Reusing existing annotation: content_hash=%s, analyzer_model=%s", contentHash, analyzerModel)
			return result
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			logger.CtxWarn(ctx, "Failed to load meme annotation: meme_id=%s, analyzer_model=%s, error=%v", memeID, analyzerModel, err)
		}
	}

	description, err := s.vlm.DescribeImage(ctx, imageData, format)
	if err != nil {
		logger.CtxWarn(ctx, "Failed to generate VLM description; continuing with image vector only: content_hash=%s, error=%v", contentHash, err)
		return result
	}
	result.Description = description

	ocrText, err := s.extractOCRText(ctx, imageData, format)
	if err != nil {
		logger.CtxWarn(ctx, "Failed to extract OCR text: content_hash=%s, error=%v", contentHash, err)
	} else {
		result.OCRText = ocrText
		result.OCRReliable = true
	}

	if s.annotationRepo == nil || analyzerModel == "" {
		return result
	}

	annotationRecord := buildMemeAnnotation(uuid.New().String(), memeID, analyzerModel, result.Description, result.OCRText, result.OCRReliable)
	if err := s.annotationRepo.Create(ctx, annotationRecord); err != nil {
		logger.CtxWarn(ctx, "Failed to save meme annotation; continuing without annotation link: meme_id=%s, error=%v", memeID, err)
		return result
	}

	result.ID = annotationRecord.ID
	result.Created = true
	logger.CtxDebug(ctx, "Created new meme annotation: content_hash=%s, analyzer_model=%s, annotation_id=%s",
		contentHash, analyzerModel, result.ID)
	return result
}

func buildMemeAnnotation(id, memeID, analyzerModel, description, ocrText string, ocrReliable bool) *domain.MemeAnnotation {
	labels := domain.AnnotationLabels{}
	if ocrReliable {
		textPresence, _ := domain.TextPresenceFromOCRText(ocrText)
		labels.Text = &domain.TextLabel{Present: textPresence == domain.TextPresenceWithText}
	}
	return &domain.MemeAnnotation{
		ID:            id,
		MemeID:        memeID,
		AnalyzerModel: analyzerModel,
		Description:   description,
		OCRText:       ocrText,
		Labels:        labels,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
}

func classifyTextPresence(ocrText string, ocrReliable bool) domain.TextPresence {
	presence, _ := classifyTextPresenceWithCount(ocrText, ocrReliable)
	return presence
}

func classifyTextPresenceWithCount(ocrText string, ocrReliable bool) (domain.TextPresence, int) {
	if !ocrReliable {
		return domain.TextPresenceUnknown, 0
	}
	return domain.TextPresenceFromOCRText(ocrText)
}

func hasReliableTextPresence(annotation *domain.MemeAnnotation, ocrText string) bool {
	if annotation == nil {
		return ocrText != ""
	}
	return annotation.Labels.Text != nil || ocrText != ""
}

func shouldExtractOCRForAnnotation(annotation *domain.MemeAnnotation, ocrText string) bool {
	if ocrText != "" {
		return false
	}
	return annotation == nil || annotation.Labels.Text == nil
}

type vectorUpsertInput struct {
	MemeID         string
	ContentHash    string
	AnnotationID   string
	Force          bool
	ImageURL       string
	ImageData      []byte
	ImageMediaType string
	CaptionText    string
	BM25Text       string
	Payload        *repository.MemePayload
}

func (s *IngestService) missingVectorIndexes(ctx context.Context, memeID string, force bool) ([]IngestVectorIndex, error) {
	if len(s.indexes) == 0 {
		return nil, fmt.Errorf("no ingest vector indexes configured")
	}
	if force {
		return s.indexes, nil
	}
	if s.vectorRepo == nil {
		return s.indexes, nil
	}

	missing := make([]IngestVectorIndex, 0, len(s.indexes))
	for _, index := range s.indexes {
		exists, err := s.vectorRepo.ExistsByMemeIDCollectionAndVectorType(ctx, memeID, index.Collection, normalizeIngestVectorType(index.VectorType))
		if err != nil {
			return nil, fmt.Errorf("failed to check vector existence: %w", err)
		}
		if !exists {
			missing = append(missing, index)
		}
	}
	return missing, nil
}

func (s *IngestService) upsertVectorIndexes(ctx context.Context, indexes []IngestVectorIndex, input vectorUpsertInput) error {
	var errs []error
	written := 0
	skipped := 0
	for _, index := range indexes {
		if err := s.upsertVectorIndex(ctx, index, input); err != nil {
			if errors.Is(err, errSkipOptionalVectorIndex) {
				skipped++
				logger.CtxDebug(ctx, "Skipped optional vector index: meme_id=%s, collection=%s, vector_type=%s",
					input.MemeID, index.Collection, domain.MemeVectorTypeToString(normalizeIngestVectorType(index.VectorType)))
				continue
			}
			logger.CtxWarn(ctx, "Failed to upsert vector index: meme_id=%s, collection=%s, vector_type=%s, error=%v",
				input.MemeID, index.Collection, normalizeIngestVectorType(index.VectorType), err)
			errs = append(errs, err)
			continue
		}
		written++
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	if written == 0 && skipped > 0 {
		return fmt.Errorf("no vector indexes were written")
	}
	return nil
}

func (s *IngestService) rollbackVectorIndexes(ctx context.Context, memeID string, indexes []IngestVectorIndex) {
	if memeID == "" || s.vectorRepo == nil {
		return
	}

	vectors, err := s.vectorRepo.GetByMemeID(ctx, memeID)
	if err != nil {
		logger.CtxError(ctx, "Failed to load vector records for rollback: meme_id=%s, error=%v", memeID, err)
		return
	}

	reposByRoute := make(map[string]*repository.QdrantRepository, len(indexes))
	for _, index := range indexes {
		vectorType := normalizeIngestVectorType(index.VectorType)
		reposByRoute[vectorRouteKey(index.Collection, vectorType)] = index.QdrantRepo
	}

	for _, vector := range vectors {
		routeKey := vectorRouteKey(vector.Collection, normalizeIngestVectorType(vector.VectorType))
		qdrantRepo, ok := reposByRoute[routeKey]
		if !ok {
			continue
		}
		if qdrantRepo != nil {
			if delErr := qdrantRepo.Delete(ctx, vector.QdrantPointID); delErr != nil {
				logger.CtxError(ctx, "Failed to rollback Qdrant point: point_id=%s, error=%v", vector.QdrantPointID, delErr)
			}
		}
		if delErr := s.vectorRepo.Delete(ctx, vector.ID); delErr != nil {
			logger.CtxError(ctx, "Failed to rollback vector record: vector_id=%s, error=%v", vector.ID, delErr)
		}
	}
}

func (s *IngestService) upsertVectorIndex(ctx context.Context, index IngestVectorIndex, input vectorUpsertInput) error {
	if index.Embedding == nil {
		return fmt.Errorf("embedding provider is nil for collection %s", index.Collection)
	}
	if index.QdrantRepo == nil {
		return fmt.Errorf("qdrant repo is nil for collection %s", index.Collection)
	}

	vectorType := normalizeIngestVectorType(index.VectorType)
	doc := EmbeddingDocument{}
	inputHash := input.ContentHash
	switch vectorType {
	case domain.MemeVectorTypeCaption:
		if input.CaptionText == "" {
			return fmt.Errorf("%w: caption vector requires caption text", errSkipOptionalVectorIndex)
		}
		doc.Text = input.CaptionText
		inputHash = calculateSHA256(input.CaptionText)
	case domain.MemeVectorTypeImage:
		if input.ImageURL == "" {
			return fmt.Errorf("image vector requires image url")
		}
		doc.ImageURL = input.ImageURL
		doc.ImageData = input.ImageData
		doc.ImageMediaType = input.ImageMediaType
	default:
		return fmt.Errorf("unsupported vector type: %s", domain.MemeVectorTypeToString(vectorType))
	}

	embedding, err := index.Embedding.EmbedDocument(ctx, doc)
	if err != nil {
		return fmt.Errorf("failed to generate %s embedding: %w", domain.MemeVectorTypeToString(vectorType), err)
	}

	var existing *domain.MemeVector
	if input.Force && s.vectorRepo != nil {
		current, err := s.vectorRepo.GetByMemeIDCollectionAndVectorType(ctx, input.MemeID, index.Collection, vectorType)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("failed to load existing vector before force replacement: %w", err)
		}
		if err == nil {
			existing = current
		}
	}

	pointID := uuid.New().String()
	if index.UseSparse {
		if err := index.QdrantRepo.UpsertHybrid(ctx, pointID, embedding, input.BM25Text, input.Payload); err != nil {
			return fmt.Errorf("failed to upsert hybrid vector: %w", err)
		}
	} else {
		if err := index.QdrantRepo.Upsert(ctx, pointID, embedding, input.Payload); err != nil {
			return fmt.Errorf("failed to upsert dense vector: %w", err)
		}
	}

	if s.vectorRepo == nil {
		return nil
	}

	vectorRecord := &domain.MemeVector{
		ID:             uuid.New().String(),
		MemeID:         input.MemeID,
		Collection:     index.Collection,
		VectorType:     vectorType,
		EmbeddingModel: index.Embedding.GetModel(),
		InputHash:      inputHash,
		AnnotationID:   input.AnnotationID,
		QdrantPointID:  pointID,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	if existing != nil {
		oldPointID := existing.QdrantPointID
		vectorRecord.ID = existing.ID
		vectorRecord.CreatedAt = existing.CreatedAt
		if err := s.vectorRepo.Update(ctx, vectorRecord); err != nil {
			if delErr := index.QdrantRepo.Delete(ctx, pointID); delErr != nil {
				logger.CtxError(ctx, "Failed to rollback replacement Qdrant point: point_id=%s, error=%v", pointID, delErr)
			}
			return fmt.Errorf("failed to update vector record: %w", err)
		}
		if oldPointID != "" && oldPointID != pointID {
			if delErr := index.QdrantRepo.Delete(ctx, oldPointID); delErr != nil {
				logger.CtxWarn(ctx, "Failed to delete old Qdrant point after force replacement: point_id=%s, error=%v", oldPointID, delErr)
			}
		}
		return nil
	}

	if err := s.vectorRepo.Create(ctx, vectorRecord); err != nil {
		if delErr := index.QdrantRepo.Delete(ctx, pointID); delErr != nil {
			logger.CtxError(ctx, "Failed to rollback Qdrant upsert: point_id=%s, error=%v", pointID, delErr)
		}
		return fmt.Errorf("failed to save vector record: %w", err)
	}

	return nil
}

func vectorRouteKey(collection string, vectorType domain.MemeVectorType) string {
	return collection + "\x00" + domain.MemeVectorTypeToString(vectorType)
}

func normalizeIngestVectorType(vectorType domain.MemeVectorType) domain.MemeVectorType {
	switch vectorType {
	case domain.MemeVectorTypeCaption:
		return vectorType
	default:
		return domain.MemeVectorTypeImage
	}
}

func IngestVectorTypeForDocumentMode(documentMode string) domain.MemeVectorType {
	if normalizeEmbeddingDocumentMode(documentMode) == embeddingDocumentText {
		return domain.MemeVectorTypeCaption
	}
	return domain.MemeVectorTypeImage
}

func (s *IngestService) readImage(item *source.MemeItem) ([]byte, error) {
	if item.LocalPath != "" {
		return os.ReadFile(item.LocalPath)
	}
	// TODO: Implement HTTP download for URL-based sources
	return nil, fmt.Errorf("URL-based sources not implemented yet")
}

func calculateMD5(data []byte) string {
	hash := md5.Sum(data)
	return hex.EncodeToString(hash[:])
}

func calculateSHA256(text string) string {
	hash := sha256.Sum256([]byte(text))
	return hex.EncodeToString(hash[:])
}

func getImageDimensions(data []byte) (int, int, error) {
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0, 0, err
	}
	return config.Width, config.Height, nil
}

func getContentType(format string) string {
	switch format {
	case "jpeg", "jpg":
		return "image/jpeg"
	case "png":
		return "image/png"
	case "webp":
		return "image/webp"
	default:
		return "application/octet-stream"
	}
}

// isSupportedStaticImageFormat checks if a source format is accepted by the
// static-image-only ingestion policy.
func isSupportedStaticImageFormat(format string) bool {
	switch format {
	case "jpg", "jpeg", "png", "webp":
		return true
	default:
		return false
	}
}

func shouldConvertStaticImageToJPEG(format string) bool {
	return format == "webp"
}

// detectImageFormat detects the actual image format by examining magic bytes.
// This is more reliable than trusting file extensions.
func detectImageFormat(data []byte) string {
	if len(data) < 12 {
		return "unknown"
	}

	// JPEG/JPG: starts with FF D8 (more accurate than checking third byte)
	// JPEG files start with FF D8 and end with FF D9
	if len(data) >= 2 && data[0] == 0xFF && data[1] == 0xD8 {
		return "jpeg"
	}

	// PNG: starts with 89 50 4E 47 0D 0A 1A 0A (8-byte signature)
	if len(data) >= 8 && data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 &&
		data[4] == 0x0D && data[5] == 0x0A && data[6] == 0x1A && data[7] == 0x0A {
		return "png"
	}

	// GIF: starts with "GIF87a" or "GIF89a"
	if len(data) >= 6 && data[0] == 0x47 && data[1] == 0x49 && data[2] == 0x46 && // "GIF"
		data[3] == 0x38 && (data[4] == 0x37 || data[4] == 0x39) && data[5] == 0x61 { // "87a" or "89a"
		return "gif"
	}

	// WebP: starts with "RIFF" and contains "WEBP" at offset 8
	if len(data) >= 12 && data[0] == 0x52 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x46 && // "RIFF"
		data[8] == 0x57 && data[9] == 0x45 && data[10] == 0x42 && data[11] == 0x50 { // "WEBP"
		return "webp"
	}

	// BMP: starts with "BM" (42 4D)
	if len(data) >= 2 && data[0] == 0x42 && data[1] == 0x4D {
		return "bmp"
	}

	// TIFF: starts with either "II" (little-endian) or "MM" (big-endian) followed by 42
	if len(data) >= 4 {
		// Little-endian: 49 49 2A 00
		if data[0] == 0x49 && data[1] == 0x49 && data[2] == 0x2A && data[3] == 0x00 {
			return "tiff"
		}
		// Big-endian: 4D 4D 00 2A
		if data[0] == 0x4D && data[1] == 0x4D && data[2] == 0x00 && data[3] == 0x2A {
			return "tiff"
		}
	}

	// ICO: starts with 00 00 01 00
	if len(data) >= 4 && data[0] == 0x00 && data[1] == 0x00 && data[2] == 0x01 && data[3] == 0x00 {
		return "ico"
	}

	// AVIF: starts with ftypavif (at offset 4-11)
	if len(data) >= 12 && data[4] == 0x66 && data[5] == 0x74 && data[6] == 0x79 && data[7] == 0x70 && // "ftyp"
		data[8] == 0x61 && data[9] == 0x76 && data[10] == 0x69 && data[11] == 0x66 { // "avif"
		return "avif"
	}

	return "unknown"
}

// convertToJPEG converts a supported static image to JPEG.
func convertToJPEG(imageData []byte, format string) ([]byte, error) {
	reader := bytes.NewReader(imageData)
	img, _, err := image.Decode(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	// Encode to JPEG
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		return nil, fmt.Errorf("failed to encode to JPEG: %w", err)
	}

	return buf.Bytes(), nil
}

// RetryPending backfills missing vector records for existing memes.
func (s *IngestService) RetryPending(ctx context.Context, limit int) (*IngestStats, error) {
	stats := &IngestStats{
		StartTime: time.Now(),
	}

	memes, err := s.memeRepo.List(ctx, limit, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to list memes: %w", err)
	}

	stats.TotalItems = int64(len(memes))

	for _, meme := range memes {
		select {
		case <-ctx.Done():
			break
		default:
		}

		targetIndexes, err := s.missingVectorIndexes(ctx, meme.ID, false)
		if err != nil {
			logger.CtxError(ctx, "Failed to check vector completeness: meme_id=%s, error=%v", meme.ID, err)
			stats.FailedItems++
			continue
		}
		if len(targetIndexes) == 0 {
			stats.ProcessedItems++
			continue
		}

		// Download from storage
		reader, err := s.storage.Download(ctx, meme.StorageKey)
		if err != nil {
			logger.CtxError(ctx, "Failed to download from storage: error=%v", err)
			stats.FailedItems++
			continue
		}

		imageData, err := io.ReadAll(reader)
		reader.Close()
		if err != nil {
			logger.CtxError(ctx, "Failed to read image data: error=%v", err)
			stats.FailedItems++
			continue
		}

		// Get or create VLM description for current VLM model
		var description string
		var ocrText string
		var annotationID string
		imageFormat := domain.ImageFormatToString(meme.ImageInfo.Format)
		annotation := s.prepareAnnotationBestEffort(ctx, meme.ID, meme.ContentHash, imageData, imageFormat)
		description = annotation.Description
		ocrText = annotation.OCRText
		annotationID = annotation.ID
		ocrReliable := annotation.OCRReliable

		compactDesc := compactDescription(description)
		captionText := buildCaptionEmbeddingText(
			ocrText,
			compactDesc,
			meme.Category,
			meme.Tags,
			extractEmotionWords(description),
		)
		bm25Text := buildBM25Text(ocrText, compactDesc, meme.Tags)
		imageURL := s.storage.GetURL(meme.StorageKey)
		payload := &repository.MemePayload{
			MemeID:         meme.ID,
			Category:       meme.Category,
			Tags:           meme.Tags,
			VLMDescription: description,
			OCRText:        ocrText,
			TextPresence:   string(classifyTextPresence(ocrText, ocrReliable)),
			StorageURL:     imageURL,
		}

		if err := s.upsertVectorIndexes(ctx, targetIndexes, vectorUpsertInput{
			MemeID:         meme.ID,
			ContentHash:    meme.ContentHash,
			AnnotationID:   annotationID,
			Force:          false,
			ImageURL:       imageURL,
			ImageData:      imageData,
			ImageMediaType: getContentType(imageFormat),
			CaptionText:    captionText,
			BM25Text:       bm25Text,
			Payload:        payload,
		}); err != nil {
			logger.CtxError(ctx, "Failed to upsert vector indexes: meme_id=%s, error=%v", meme.ID, err)
			stats.FailedItems++
			continue
		}

		logger.CtxDebug(ctx, "Retry processed: meme_id=%s, vectors=%d",
			meme.ID, len(targetIndexes))

		stats.ProcessedItems++
	}

	stats.EndTime = time.Now()
	return stats, nil
}
