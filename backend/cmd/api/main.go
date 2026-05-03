package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	pb "github.com/timmy/emomo/gen/emomo/v1"
	"github.com/timmy/emomo/internal/api"
	"github.com/timmy/emomo/internal/config"
	"github.com/timmy/emomo/internal/logger"
	"github.com/timmy/emomo/internal/repository"
	"github.com/timmy/emomo/internal/service"
	"github.com/timmy/emomo/internal/source"
	"github.com/timmy/emomo/internal/source/localdir"
	"github.com/timmy/emomo/internal/storage"
)

func buildSources(cfg *config.Config) map[string]source.Source {
	sources := make(map[string]source.Source)
	if cfg.Sources.LocalDir.Enabled {
		sources["localdir"] = localdir.NewAdapter(localdir.Options{
			RootPath:     cfg.Sources.LocalDir.RootPath,
			SourceID:     cfg.Sources.LocalDir.SourceID,
			ManifestPath: cfg.Sources.LocalDir.ManifestPath,
			QueuePath:    cfg.Sources.LocalDir.QueuePath,
		})
	}
	return sources
}

func serviceRetrievalConfig(cfg config.RetrievalConfig) service.RetrievalConfig {
	return service.RetrievalConfig{
		ImageTopK:   cfg.ImageTopK,
		CaptionTopK: cfg.CaptionTopK,
		FinalTopK:   cfg.FinalTopK,
		Weights: service.RetrievalWeights{
			Image:   cfg.Weights.Image,
			Caption: cfg.Weights.Caption,
			Keyword: cfg.Weights.Keyword,
		},
	}
}

func registerSearchProfiles(searchService *service.SearchService, registry *service.EmbeddingRegistry, profiles []config.SearchProfileConfig) {
	for _, profile := range profiles {
		imageProvider, imageRepo, hasImage := registry.Get(profile.ImageEmbedding)
		captionProvider, captionRepo, hasCaption := registry.Get(profile.CaptionEmbedding)
		if !hasImage || !hasCaption {
			logger.Warn("Skipping search profile with missing embeddings: profile=%s, image=%s, caption=%s",
				profile.Name, profile.ImageEmbedding, profile.CaptionEmbedding)
			continue
		}
		searchService.RegisterProfile(profile.Name, imageRepo, imageProvider, captionRepo, captionProvider)
	}
}

func main() {
	// Initialize logger first (with defaults)
	appLogger := logger.New(&logger.Config{
		Level:       "info",
		Format:      "json",
		ServiceName: "emomo-api",
	})
	logger.SetDefaultLogger(appLogger)
	defer logger.Sync() // Ensure logs are flushed on exit

	// Load configuration
	config.LoadDotEnv()
	configPath := os.Getenv("CONFIG_PATH")
	cfg, err := config.Load(configPath)
	if err != nil {
		appLogger.WithError(err).Fatal("Failed to load config")
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

	ctx := context.Background()

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

	// Initialize embedding registry (replaces ~70 lines of manual initialization)
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

	// Ensure all Qdrant collections exist
	if err := embeddingRegistry.EnsureCollections(ctx); err != nil {
		appLogger.WithError(err).Warn("Some collections may not be ready")
	}

	// Get default embedding provider and Qdrant repo
	defaultProvider, defaultQdrantRepo := embeddingRegistry.Default()
	defaultEmbeddingName := embeddingRegistry.DefaultName()
	defaultQdrantCollection := defaultQdrantRepo.GetCollectionName()
	defaultVectorType := pb.VectorType_VECTOR_TYPE_UNSPECIFIED
	if defaultEmbeddingCfg := cfg.GetDefaultEmbedding(); defaultEmbeddingCfg != nil {
		defaultVectorType = service.IngestVectorTypeForDocumentMode(defaultEmbeddingCfg.GetDocumentMode())
	}

	// Initialize query expansion service
	// Use Query Expansion's own APIKey/BaseURL if configured, otherwise fall back to VLM's
	qeAPIKey := cfg.Search.QueryExpansion.APIKey
	if qeAPIKey == "" {
		qeAPIKey = cfg.VLM.APIKey
	}
	qeBaseURL := cfg.Search.QueryExpansion.BaseURL
	if qeBaseURL == "" {
		qeBaseURL = cfg.VLM.BaseURL
	}
	queryExpansionService := service.NewQueryExpansionService(&service.QueryExpansionConfig{
		Enabled: cfg.Search.QueryExpansion.Enabled,
		Model:   cfg.Search.QueryExpansion.Model,
		APIKey:  qeAPIKey,
		BaseURL: qeBaseURL,
	})

	if queryExpansionService.IsEnabled() {
		appLogger.WithFields(logger.Fields{
			"model": cfg.Search.QueryExpansion.Model,
		}).Info("Query expansion enabled")
	}

	// Create search service
	searchService := service.NewSearchService(
		memeRepo,
		annotationRepo,
		defaultQdrantRepo,
		defaultProvider,
		queryExpansionService,
		objectStorage,
		appLogger,
		&service.SearchConfig{
			ScoreThreshold:    cfg.Search.ScoreThreshold,
			DefaultCollection: defaultEmbeddingName,
			DefaultProfile:    cfg.Search.DefaultProfile,
			Retrieval:         serviceRetrievalConfig(cfg.Search.Retrieval),
		},
	)

	// Register all embedding collections with search service
	for _, name := range embeddingRegistry.Names() {
		provider, qdrantRepo, _ := embeddingRegistry.Get(name)
		searchService.RegisterCollection(name, qdrantRepo, provider)
	}
	registerSearchProfiles(searchService, embeddingRegistry, cfg.Search.Profiles)

	appLogger.WithFields(logger.Fields{
		"available_collections": searchService.GetAvailableCollections(),
		"available_profiles":    searchService.GetAvailableProfiles(),
		"default_collection":    defaultEmbeddingName,
		"default_profile":       cfg.Search.DefaultProfile,
		"default_qdrant":        defaultQdrantCollection,
	}).Info("Embedding collections registered")

	// Initialize VLM service
	vlmService := service.NewVLMService(&service.VLMConfig{
		Provider: cfg.VLM.Provider,
		Model:    cfg.VLM.Model,
		APIKey:   cfg.VLM.APIKey,
		BaseURL:  cfg.VLM.BaseURL,
	})

	var ingestIndexes []service.IngestVectorIndex
	if defaultProfile := cfg.GetDefaultSearchProfile(); defaultProfile != nil {
		ingestIndexes, err = embeddingRegistry.BuildProfileIngestIndexes(defaultProfile)
		if err != nil {
			appLogger.WithError(err).Fatal("Failed to build ingest vector indexes")
		}
	}

	// Initialize ingest service (uses default search profile when configured)
	ingestService := service.NewIngestService(
		memeRepo,
		vectorRepo,
		annotationRepo,
		defaultQdrantRepo,
		objectStorage,
		vlmService,
		defaultProvider,
		appLogger,
		&service.IngestConfig{
			Workers:       cfg.Ingest.Workers,
			BatchSize:     cfg.Ingest.BatchSize,
			Collection:    defaultQdrantCollection,
			VectorType:    defaultVectorType,
			VectorIndexes: ingestIndexes,
		},
	)

	// Initialize data sources
	sources := buildSources(cfg)

	// Setup router
	router := api.SetupRouter(searchService, ingestService, sources, cfg, appLogger)

	// Create HTTP server
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.Port),
		Handler: router,
	}

	// Start server in goroutine
	go func() {
		appLogger.WithFields(logger.Fields{
			"port":                  cfg.Server.Port,
			"mode":                  cfg.Server.Mode,
			"default_collection":    defaultEmbeddingName,
			"default_qdrant":        defaultQdrantCollection,
			"default_profile":       cfg.Search.DefaultProfile,
			"available_collections": searchService.GetAvailableCollections(),
			"available_profiles":    searchService.GetAvailableProfiles(),
		}).Info("Starting API server")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			appLogger.WithError(err).Fatal("Failed to start server")
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server...")

	// Graceful shutdown with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("Server forced to shutdown: %v", err)
	}

	logger.Info("Server exited")
}
