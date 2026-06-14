package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/timmy/emomo/internal/api"
	"github.com/timmy/emomo/internal/config"
	"github.com/timmy/emomo/internal/configcenter"
	"github.com/timmy/emomo/internal/logger"
	"github.com/timmy/emomo/internal/repository"
	"github.com/timmy/emomo/internal/service"
	"github.com/timmy/emomo/internal/storage"
)

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
		if !hasImage {
			logger.Warn("Skipping search profile with missing image embedding: profile=%s, image=%s",
				profile.Name, profile.ImageEmbedding)
			continue
		}
		var captionProvider service.EmbeddingProvider
		var captionRepo *repository.QdrantRepository
		if profile.CaptionEmbedding != "" {
			var hasCaption bool
			captionProvider, captionRepo, hasCaption = registry.Get(profile.CaptionEmbedding)
			if !hasCaption {
				logger.Warn("Skipping search profile with missing caption embedding: profile=%s, caption=%s",
					profile.Name, profile.CaptionEmbedding)
				continue
			}
		}
		var keywordRepo *repository.QdrantRepository
		if profile.KeywordEmbedding != "" {
			var hasKeyword bool
			_, keywordRepo, hasKeyword = registry.Get(profile.KeywordEmbedding)
			if !hasKeyword {
				logger.Warn("Skipping search profile with missing keyword embedding: profile=%s, keyword=%s",
					profile.Name, profile.KeywordEmbedding)
				continue
			}
		} else if captionRepo != nil {
			keywordRepo = captionRepo
		}
		searchService.RegisterProfile(profile.Name, imageRepo, imageProvider, captionRepo, captionProvider, keywordRepo)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func loggerEnvConfigFromConfig(cfg config.LoggingConfig, serviceName string) *logger.EnvConfig {
	if cfg.ServiceName != "" {
		serviceName = cfg.ServiceName
	}

	return &logger.EnvConfig{
		Level:             cfg.Level,
		Format:            cfg.Format,
		ServiceName:       serviceName,
		Environment:       cfg.Environment,
		LogFile:           cfg.LogFile,
		LogFileOnly:       cfg.LogFileOnly,
		MaxSize:           cfg.MaxSize,
		MaxBackups:        cfg.MaxBackups,
		MaxAge:            cfg.MaxAge,
		Compress:          cfg.Compress,
		LokiEnabled:       cfg.LokiEnabled,
		LokiURL:           cfg.LokiURL,
		LokiUsername:      cfg.LokiUsername,
		LokiPassword:      cfg.LokiPassword,
		LokiProject:       cfg.LokiProject,
		ClusterName:       cfg.ClusterName,
		LokiBatchSize:     cfg.LokiBatchSize,
		LokiQueueSize:     cfg.LokiQueueSize,
		LokiFlushInterval: cfg.LokiFlushInterval,
		LokiTimeout:       cfg.LokiTimeout,
	}
}

func queryExpansionRuntimeConfig(cfg *config.Config) service.QueryExpansionConfig {
	apiKey := cfg.Search.QueryExpansion.APIKey
	if apiKey == "" {
		apiKey = cfg.VLM.APIKey
	}
	baseURL := cfg.Search.QueryExpansion.BaseURL
	if baseURL == "" {
		baseURL = cfg.VLM.BaseURL
	}

	return service.QueryExpansionConfig{
		Enabled: cfg.Search.QueryExpansion.Enabled,
		Model:   cfg.Search.QueryExpansion.Model,
		APIKey:  apiKey,
		BaseURL: baseURL,
	}
}

func applyRemoteQueryExpansionConfig(
	base service.QueryExpansionConfig,
	remote configcenter.RemoteQueryExpansionConfig,
) service.QueryExpansionConfig {
	next := base
	if remote.Enabled != nil {
		next.Enabled = *remote.Enabled
	}
	if remote.Model != nil {
		next.Model = *remote.Model
	}
	if remote.APIKey != nil {
		next.APIKey = *remote.APIKey
	}
	if remote.BaseURL != nil {
		next.BaseURL = *remote.BaseURL
	}
	return next
}

func mapValue(values map[string]any, key string) map[string]any {
	if values == nil {
		return nil
	}
	nested, _ := values[key].(map[string]any)
	return nested
}

func stringValue(values map[string]any, key string) (string, bool) {
	if values == nil {
		return "", false
	}
	value, ok := values[key]
	if !ok {
		return "", false
	}
	typed, ok := value.(string)
	return typed, ok
}

func boolValue(values map[string]any, key string) (bool, bool) {
	if values == nil {
		return false, false
	}
	value, ok := values[key]
	if !ok {
		return false, false
	}
	switch typed := value.(type) {
	case bool:
		return typed, true
	case string:
		parsed, err := strconv.ParseBool(typed)
		return parsed, err == nil
	default:
		return false, false
	}
}

func queryExpansionRuntimeConfigFromRemote(
	base service.QueryExpansionConfig,
	runtimeConfig map[string]any,
) (service.QueryExpansionConfig, bool) {
	if len(runtimeConfig) == 0 {
		return base, false
	}

	next := base
	changed := false

	if vlm := mapValue(runtimeConfig, "vlm"); vlm != nil {
		if value, ok := stringValue(vlm, "api_key"); ok {
			next.APIKey = value
			changed = true
		}
		if value, ok := stringValue(vlm, "base_url"); ok {
			next.BaseURL = value
			changed = true
		}
	}

	search := mapValue(runtimeConfig, "search")
	qe := mapValue(search, "query_expansion")
	if qe == nil {
		return next, changed
	}
	if value, ok := boolValue(qe, "enabled"); ok {
		next.Enabled = value
		changed = true
	}
	if value, ok := stringValue(qe, "model"); ok {
		next.Model = value
		changed = true
	}
	if value, ok := stringValue(qe, "api_key"); ok {
		next.APIKey = value
		changed = true
	}
	if value, ok := stringValue(qe, "base_url"); ok {
		next.BaseURL = value
		changed = true
	}

	return next, changed
}

func startConfigCenterPolling(
	ctx context.Context,
	cfg config.ConfigCenterConfig,
	baseQueryExpansion service.QueryExpansionConfig,
	queryExpansionService *service.QueryExpansionService,
	appLogger *logger.Logger,
) {
	if queryExpansionService == nil {
		return
	}
	if !cfg.Enabled && cfg.URL == "" {
		return
	}
	if cfg.URL == "" {
		appLogger.Warn("Config center enabled but CONFIG_CENTER_URL is empty")
		return
	}

	pollInterval := cfg.PollInterval
	if pollInterval <= 0 {
		pollInterval = time.Minute
	}

	client := configcenter.NewClient(configcenter.ClientConfig{
		URL:     cfg.URL,
		Token:   cfg.Token,
		Timeout: cfg.Timeout,
	})

	appLogger.WithFields(logger.Fields{
		"poll_interval": pollInterval.String(),
	}).Info("Config center polling enabled")

	fetchAndApply := func() {
		remote, err := client.Fetch(ctx)
		if err != nil {
			appLogger.WithError(err).Warn("Failed to fetch config center config")
			return
		}
		runtimeConfig := remote.RuntimeConfig()
		next, ok := queryExpansionRuntimeConfigFromRemote(baseQueryExpansion, runtimeConfig)
		if !ok {
			return
		}

		if !queryExpansionService.UpdateConfig(&next) {
			return
		}

		snapshot := queryExpansionService.Snapshot()
		appLogger.WithFields(logger.Fields{
			"enabled":        snapshot.Enabled,
			"model":          snapshot.Model,
			"base_url":       snapshot.BaseURL,
			"remote_version": remote.Version,
			"remote_updated": remote.UpdatedAt,
		}).Info("Query expansion config updated from config center")
	}

	fetchAndApply()

	go func() {
		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				fetchAndApply()
			}
		}
	}()
}

func buildAgenticSearchService(cfg *config.Config, appLogger *logger.Logger) *service.AgenticSearchService {
	agenticCfg := cfg.Search.Agentic
	if !agenticCfg.Enabled {
		return nil
	}

	apiKey := firstNonEmpty(cfg.Search.QueryExpansion.APIKey, cfg.VLM.APIKey)
	baseURL := firstNonEmpty(cfg.Search.QueryExpansion.BaseURL, cfg.VLM.BaseURL)
	plannerModel := firstNonEmpty(agenticCfg.PlannerModel, cfg.Search.QueryExpansion.Model, cfg.VLM.Model)
	rerankerModel := firstNonEmpty(agenticCfg.RerankerModel, cfg.Search.QueryExpansion.Model, cfg.VLM.Model)
	if apiKey == "" || plannerModel == "" || rerankerModel == "" {
		appLogger.Warn("Agentic search disabled because LLM credentials or models are incomplete")
		return nil
	}

	plannerClient, err := service.NewOpenAICompatibleJSONClient(service.OpenAICompatibleJSONClientConfig{
		Model:   plannerModel,
		APIKey:  apiKey,
		BaseURL: baseURL,
		Timeout: agenticCfg.PlannerTimeout,
	})
	if err != nil {
		appLogger.WithError(err).Warn("Agentic search planner client unavailable")
		return nil
	}
	rerankerClient, err := service.NewOpenAICompatibleJSONClient(service.OpenAICompatibleJSONClientConfig{
		Model:   rerankerModel,
		APIKey:  apiKey,
		BaseURL: baseURL,
		Timeout: agenticCfg.RerankerTimeout,
	})
	if err != nil {
		appLogger.WithError(err).Warn("Agentic search reranker client unavailable")
		return nil
	}

	planner := service.NewLLMSearchPlanner(plannerClient, service.LLMSearchPlannerConfig{
		Timeout: agenticCfg.PlannerTimeout,
	})
	reranker := service.NewLLMSearchReranker(rerankerClient, service.LLMSearchRerankerConfig{
		TopK:    agenticCfg.RerankTopK,
		Timeout: agenticCfg.RerankerTimeout,
	})
	appLogger.WithFields(logger.Fields{
		"planner_model":  plannerModel,
		"reranker_model": rerankerModel,
		"rerank_top_k":   agenticCfg.RerankTopK,
	}).Info("Agentic search enabled")
	return service.NewAgenticSearchService(planner, reranker, service.AgenticSearchConfig{
		Enabled:         true,
		FallbackOnError: agenticCfg.FallbackOnError,
		RerankTopK:      agenticCfg.RerankTopK,
	})
}

func main() {
	// Load .env so config-center bootstrap variables are available before
	// loading the complete runtime config.
	config.LoadDotEnv()
	bootstrapLogger := logger.New(&logger.Config{
		Level:       "info",
		Format:      "json",
		ServiceName: "emomo-api",
	})
	logger.SetDefaultLogger(bootstrapLogger)

	// Load configuration
	configPath := os.Getenv("CONFIG_PATH")
	cfg, err := config.Load(configPath)
	if err != nil {
		bootstrapLogger.WithError(err).Fatal("Failed to load config")
	}

	appLogger := logger.NewFromEnv(loggerEnvConfigFromConfig(cfg.Logging, "emomo-api"))
	logger.SetDefaultLogger(appLogger)
	defer logger.Sync() // Ensure logs are flushed on exit

	// Initialize database
	db, err := repository.InitDB(&cfg.Database)
	if err != nil {
		appLogger.WithError(err).Fatal("Failed to initialize database")
	}

	// Initialize repositories
	memeRepo := repository.NewMemeRepository(db)
	annotationRepo := repository.NewMemeAnnotationRepository(db)

	ctx, cancelRuntime := context.WithCancel(context.Background())
	defer cancelRuntime()

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
	// Initialize query expansion service. Query Expansion's own APIKey/BaseURL
	// override VLM credentials; otherwise it falls back to VLM.
	queryExpansionCfg := queryExpansionRuntimeConfig(cfg)
	queryExpansionService := service.NewQueryExpansionService(&queryExpansionCfg)

	if queryExpansionService.IsEnabled() {
		appLogger.WithFields(logger.Fields{
			"model": queryExpansionService.Snapshot().Model,
		}).Info("Query expansion enabled")
	}
	startConfigCenterPolling(ctx, cfg.ConfigCenter, queryExpansionCfg, queryExpansionService, appLogger)
	agenticSearchService := buildAgenticSearchService(cfg, appLogger)

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
			Agentic:           agenticSearchService,
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

	// Setup router
	router := api.SetupRouter(searchService, cfg, appLogger)

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

	cancelRuntime()

	logger.Info("Shutting down server...")

	// Graceful shutdown with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("Server forced to shutdown: %v", err)
	}

	logger.Info("Server exited")
}
