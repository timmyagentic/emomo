package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	pb "github.com/timmy/emomo/gen/emomo/v1"
	"github.com/timmy/emomo/internal/config"
	"github.com/timmy/emomo/internal/logger"
	"github.com/timmy/emomo/internal/repository"
	"github.com/timmy/emomo/internal/service"
	"github.com/timmy/emomo/internal/source"
	"github.com/timmy/emomo/internal/source/localdir"
	"github.com/timmy/emomo/internal/storage"
)

const (
	importEntrypointEnv   = "EMOMO_IMPORT_DATA_ENTRYPOINT"
	importEntrypointValue = "script"
)

func isScriptEntrypoint(getenv func(string) string) bool {
	return getenv(importEntrypointEnv) == importEntrypointValue
}

func requireScriptEntrypoint() {
	if isScriptEntrypoint(os.Getenv) {
		return
	}
	fmt.Fprintf(os.Stderr, "cmd/ingest is internal. Use ./scripts/import-data.sh -p <local-image-dir> to import data.\n")
	os.Exit(2)
}

// resolveIngestLogPath decides where the ingest CLI should persist its log
// for this run. Resolution order:
//
//  1. LOG_FILE — explicit absolute or relative path, used as-is.
//  2. LOG_DIR (default ./logs) + auto-generated filename
//     ingest-YYYYMMDD-HHMMSS.log so each run lands in its own file, which
//     keeps post-mortem inspection of a single import scoped and avoids
//     concurrent runs racing on the same file.
//
// Paths are kept relative on purpose; the import-data.sh wrapper cd's into
// backend/ before invoking us, so "./logs" lands at backend/logs/.
func resolveIngestLogPath() string {
	if explicit := os.Getenv("LOG_FILE"); explicit != "" {
		return explicit
	}
	dir := envOr("LOG_DIR", "./logs")
	name := fmt.Sprintf("ingest-%s.log", time.Now().Format("20060102-150405"))
	return filepath.Join(dir, name)
}

// envOr returns os.Getenv(key) or fallback when the env is unset/empty.
// Local helper to avoid pulling in a config package just to read two vars.
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func selectSource(cfg *config.Config, sourceType string, pathOverride string) (source.Source, error) {
	if sourceType != "localdir" {
		return nil, fmt.Errorf("unsupported source type %q; supported source: localdir", sourceType)
	}
	if !cfg.Sources.LocalDir.Enabled {
		return nil, fmt.Errorf("source %q is disabled", sourceType)
	}

	rootPath := cfg.Sources.LocalDir.RootPath
	if pathOverride != "" {
		rootPath = pathOverride
	}
	return localdir.NewAdapter(localdir.Options{
		RootPath:     rootPath,
		SourceID:     cfg.Sources.LocalDir.SourceID,
		ManifestPath: cfg.Sources.LocalDir.ManifestPath,
		QueuePath:    cfg.Sources.LocalDir.QueuePath,
	}), nil
}

func main() {
	requireScriptEntrypoint()

	logFilePath := resolveIngestLogPath()

	output := io.Writer(os.Stdout)
	var fileWriter io.WriteCloser
	if w, err := logger.OpenRotatingFile(logFilePath); err != nil {
		fmt.Fprintf(os.Stderr, "warn: ingest log file %s unavailable, falling back to stdout-only: %v\n", logFilePath, err)
	} else {
		fileWriter = w
		output = io.MultiWriter(os.Stdout, w)
	}
	defer func() {
		if fileWriter != nil {
			_ = fileWriter.Close()
		}
	}()

	appLogger := logger.New(&logger.Config{
		Level:       envOr("LOG_LEVEL", "info"),
		Format:      "json",
		Output:      output,
		ServiceName: "emomo-ingest",
	})
	logger.SetDefaultLogger(appLogger)
	defer logger.Sync()

	if fileWriter != nil {
		appLogger.WithField("log_file", logFilePath).Info("Ingest log file initialized")
	}

	// Parse command line flags
	sourceType := flag.String("source", "localdir", "Data source to ingest from")
	sourcePath := flag.String("path", "", "Local static image directory path; overrides sources.localdir.root_path")
	limit := flag.Int("limit", 0, "Maximum number of items to ingest; 0 means no limit")
	retryPending := flag.Bool("retry", false, "Retry pending items instead of ingesting new ones")
	force := flag.Bool("force", false, "Force re-process items, skip duplicate checks")
	autoMigrate := flag.Bool("auto-migrate", false, "Run database auto-migrations before ingest")
	configPath := flag.String("config", "", "Path to config file")
	embeddingName := flag.String("embedding", "", "Embedding config name (e.g., 'jina', 'qwen3'). If empty, uses default")
	profileName := flag.String("profile", "", "Search profile name for multi-vector ingestion (e.g., 'qwen3vl'). Defaults to search.default_profile")
	flag.Parse()

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		appLogger.WithError(err).Fatal("Failed to load config")
	}

	if *autoMigrate {
		cfg.Database.AutoMigrate = true
	} else {
		cfg.Database.AutoMigrate = false
	}

	// Initialize database
	db, err := repository.InitDB(&cfg.Database)
	if err != nil {
		appLogger.WithError(err).Fatal("Failed to initialize database")
	}

	// Initialize repositories
	memeRepo := repository.NewMemeRepository(db)
	vectorRepo := repository.NewMemeVectorRepository(db)
	annotationRepo := repository.NewMemeAnnotationRepository(db)
	metadataRepo := repository.NewMemeMetadataRepository(db)

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

	// Ensure Qdrant collection exists
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := embeddingRegistry.EnsureCollections(ctx); err != nil {
		appLogger.WithError(err).Fatal("Failed to ensure Qdrant collections")
	}

	var ingestIndexes []service.IngestVectorIndex
	var qdrantRepo *repository.QdrantRepository
	var embeddingProvider service.EmbeddingProvider
	collectionName := ""
	activeProfile := ""
	activeEmbedding := ""
	fallbackVectorType := pb.VectorType_VECTOR_TYPE_UNSPECIFIED

	if *embeddingName == "" {
		var profileCfg *config.SearchProfileConfig
		if *profileName != "" {
			profileCfg = cfg.GetSearchProfileByName(*profileName)
			if profileCfg == nil {
				appLogger.WithField("profile", *profileName).Fatal("Unknown search profile")
			}
		} else {
			profileCfg = cfg.GetDefaultSearchProfile()
		}
		if profileCfg != nil {
			ingestIndexes, err = embeddingRegistry.BuildProfileIngestIndexes(profileCfg)
			if err != nil {
				appLogger.WithError(err).Fatal("Failed to build profile ingest indexes")
			}
			activeProfile = profileCfg.Name
		}
	}

	if len(ingestIndexes) == 0 {
		name := *embeddingName
		if name == "" {
			name = embeddingRegistry.DefaultName()
		}
		var ok bool
		embeddingProvider, qdrantRepo, ok = embeddingRegistry.Get(name)
		if !ok {
			appLogger.WithField("embedding", name).Fatal("Unknown embedding configuration name")
		}
		if embCfg, ok := embeddingRegistry.GetConfig(name); ok {
			fallbackVectorType = service.IngestVectorTypeForDocumentMode(embCfg.GetDocumentMode())
		}
		activeEmbedding = name
		collectionName = qdrantRepo.GetCollectionName()
	} else {
		embeddingProvider, qdrantRepo = embeddingRegistry.Default()
		if len(ingestIndexes) > 0 {
			collectionName = ingestIndexes[0].Collection
		}
	}

	appLogger.WithFields(logger.Fields{
		"source":            *sourceType,
		"limit":             *limit,
		"retry":             *retryPending,
		"force":             *force,
		"embedding":         activeEmbedding,
		"profile":           activeProfile,
		"qdrant_collection": collectionName,
		"vector_indexes":    len(ingestIndexes),
	}).Info("Starting ingestion")

	// Initialize S3-compatible storage
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

	if err := objectStorage.EnsureBucket(ctx); err != nil {
		appLogger.WithError(err).Fatal("Failed to ensure storage bucket")
	}

	// Initialize VLM service
	vlmService := service.NewVLMService(&service.VLMConfig{
		Provider: cfg.VLM.Provider,
		Model:    cfg.VLM.Model,
		APIKey:   cfg.VLM.APIKey,
		BaseURL:  cfg.VLM.BaseURL,
	})

	// Initialize ingest service
	ingestService := service.NewIngestService(
		memeRepo,
		vectorRepo,
		annotationRepo,
		metadataRepo,
		qdrantRepo,
		objectStorage,
		vlmService,
		embeddingProvider,
		appLogger,
		&service.IngestConfig{
			Workers:       cfg.Ingest.Workers,
			BatchSize:     cfg.Ingest.BatchSize,
			Collection:    collectionName,
			VectorType:    fallbackVectorType,
			VectorIndexes: ingestIndexes,
		},
	)

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		appLogger.Info("Received shutdown signal, canceling...")
		cancel()
	}()

	// Run ingestion
	if *retryPending {
		stats, err := ingestService.RetryPending(ctx, *limit)
		if err != nil {
			appLogger.WithError(err).Fatal("Failed to retry pending items")
		}
		appLogger.WithFields(logger.Fields{
			"total":     stats.TotalItems,
			"processed": stats.ProcessedItems,
			"failed":    stats.FailedItems,
		}).Info("Retry completed")
	} else {
		src, err := selectSource(cfg, *sourceType, *sourcePath)
		if err != nil {
			appLogger.WithError(err).WithField("source", *sourceType).Fatal("Failed to select source")
		}

		stats, err := ingestService.IngestFromSource(ctx, src, *limit, &service.IngestOptions{
			Force: *force,
		})
		if err != nil {
			appLogger.WithError(err).Fatal("Failed to ingest from source")
		}
		appLogger.WithFields(logger.Fields{
			"total":      stats.TotalItems,
			"processed":  stats.ProcessedItems,
			"skipped":    stats.SkippedItems,
			"failed":     stats.FailedItems,
			"collection": collectionName,
			"model":      embeddingProvider.GetModel(),
			"profile":    activeProfile,
		}).Info("Ingestion completed")
	}
}
