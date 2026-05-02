package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/timmy/emomo/internal/config"
	"github.com/timmy/emomo/internal/domain"
	"github.com/timmy/emomo/internal/logger"
	"github.com/timmy/emomo/internal/repository"
	"github.com/timmy/emomo/internal/service"
	"github.com/timmy/emomo/internal/source"
	"github.com/timmy/emomo/internal/source/localdir"
	"github.com/timmy/emomo/internal/storage"
)

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
	// Initialize logger first (with defaults)
	appLogger := logger.New(&logger.Config{
		Level:       "info",
		Format:      "json",
		ServiceName: "emomo-ingest",
	})
	logger.SetDefaultLogger(appLogger)
	defer logger.Sync() // Ensure logs are flushed on exit

	// Parse command line flags
	sourceType := flag.String("source", "localdir", "Data source to ingest from")
	sourcePath := flag.String("path", "", "Local static image directory path; overrides sources.localdir.root_path")
	limit := flag.Int("limit", 100, "Maximum number of items to ingest")
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
	fallbackVectorType := domain.MemeVectorTypeUnspecified

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
