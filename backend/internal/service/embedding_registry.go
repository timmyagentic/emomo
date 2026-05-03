package service

import (
	"context"
	"fmt"
	"sync"

	pb "github.com/timmy/emomo/gen/emomo/v1"
	"github.com/timmy/emomo/internal/config"
	"github.com/timmy/emomo/internal/logger"
	"github.com/timmy/emomo/internal/repository"
)

// EmbeddingRegistry manages all embedding configurations, providers, and their associated Qdrant repositories.
// It provides a unified interface for accessing embedding capabilities across the application.
type EmbeddingRegistry struct {
	configs     map[string]*config.EmbeddingConfig
	providers   map[string]EmbeddingProvider
	qdrantRepos map[string]*repository.QdrantRepository
	defaultName string
	logger      *logger.Logger
	mu          sync.RWMutex
}

// EmbeddingRegistryConfig holds configuration for creating an EmbeddingRegistry.
type EmbeddingRegistryConfig struct {
	Embeddings        []config.EmbeddingConfig
	QdrantHost        string
	QdrantPort        int
	QdrantAPIKey      string
	QdrantUseTLS      bool
	DefaultCollection string // Fallback collection name if not specified in embedding config
	Logger            *logger.Logger
}

// NewEmbeddingRegistry creates a new registry with all configured embeddings.
// It initializes providers and Qdrant repositories for each valid embedding configuration.
// Invalid configurations are logged and skipped rather than causing failure.
func NewEmbeddingRegistry(cfg *EmbeddingRegistryConfig) (*EmbeddingRegistry, error) {
	if cfg.Logger == nil {
		return nil, fmt.Errorf("logger is required")
	}

	r := &EmbeddingRegistry{
		configs:     make(map[string]*config.EmbeddingConfig),
		providers:   make(map[string]EmbeddingProvider),
		qdrantRepos: make(map[string]*repository.QdrantRepository),
		logger:      cfg.Logger,
	}

	if len(cfg.Embeddings) == 0 {
		return nil, fmt.Errorf("at least one embedding configuration is required")
	}

	for i := range cfg.Embeddings {
		embCfg := cfg.Embeddings[i].Clone()

		// Resolve environment variables
		embCfg.ResolveEnvVars()

		// Validate configuration (without requiring API key for listing)
		if err := embCfg.Validate(); err != nil {
			logger.Warn("Skipping invalid embedding config: index=%d, error=%v", i, err)
			continue
		}

		// Check API key is available
		if embCfg.APIKey == "" {
			logger.Warn("Skipping embedding config: no API key configured, name=%s, api_key_env=%s",
				embCfg.Name, embCfg.APIKeyEnv)
			continue
		}

		// Create embedding provider
		provider, err := NewEmbeddingProvider(&EmbeddingProviderConfig{
			Provider:     embCfg.Provider,
			Model:        embCfg.Model,
			APIKey:       embCfg.APIKey,
			BaseURL:      embCfg.BaseURL,
			DocumentMode: embCfg.GetDocumentMode(),
			Dimensions:   embCfg.Dimensions,
		})
		if err != nil {
			logger.Warn("Failed to create embedding provider, skipping: name=%s, error=%v",
				embCfg.Name, err)
			continue
		}

		// Determine collection name
		collection := embCfg.GetCollection(cfg.DefaultCollection)

		// Create Qdrant repository
		qdrantRepo, err := repository.NewQdrantRepository(&repository.QdrantConnectionConfig{
			Host:            cfg.QdrantHost,
			Port:            cfg.QdrantPort,
			Collection:      collection,
			APIKey:          cfg.QdrantAPIKey,
			UseTLS:          cfg.QdrantUseTLS,
			VectorDimension: embCfg.Dimensions,
		})
		if err != nil {
			logger.Warn("Failed to create Qdrant repository, skipping: name=%s, collection=%s, error=%v",
				embCfg.Name, collection, err)
			continue
		}

		// Store in registry
		r.configs[embCfg.Name] = embCfg
		r.providers[embCfg.Name] = provider
		r.qdrantRepos[embCfg.Name] = qdrantRepo

		// Track default
		if embCfg.IsDefault {
			if r.defaultName != "" {
				logger.Warn("Multiple default embeddings configured, using latest: existing=%s, new=%s",
					r.defaultName, embCfg.Name)
			}
			r.defaultName = embCfg.Name
		}

		logger.Info("Registered embedding: name=%s, provider=%s, model=%s, collection=%s, dim=%d, default=%v",
			embCfg.Name, embCfg.Provider, embCfg.Model, collection, embCfg.Dimensions, embCfg.IsDefault)
	}

	// Ensure we have at least one valid embedding
	if len(r.configs) == 0 {
		return nil, fmt.Errorf("no valid embedding configurations found")
	}

	// If no default was explicitly set, use the first one
	if r.defaultName == "" {
		for name := range r.configs {
			r.defaultName = name
			logger.Info("Using first embedding as default: name=%s", name)
			break
		}
	}

	return r, nil
}

// Default returns the default embedding provider and its Qdrant repository.
func (r *EmbeddingRegistry) Default() (EmbeddingProvider, *repository.QdrantRepository) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.providers[r.defaultName], r.qdrantRepos[r.defaultName]
}

// DefaultName returns the name of the default embedding configuration.
func (r *EmbeddingRegistry) DefaultName() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.defaultName
}

// Get returns the embedding provider and Qdrant repository for the given name.
// If name is empty, returns the default embedding.
// Returns false if the named embedding is not found.
func (r *EmbeddingRegistry) Get(name string) (EmbeddingProvider, *repository.QdrantRepository, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if name == "" {
		name = r.defaultName
	}

	provider, hasProvider := r.providers[name]
	qdrantRepo, hasRepo := r.qdrantRepos[name]

	if !hasProvider || !hasRepo {
		return nil, nil, false
	}

	return provider, qdrantRepo, true
}

// GetProvider returns just the embedding provider for the given name.
// If name is empty, returns the default provider.
func (r *EmbeddingRegistry) GetProvider(name string) (EmbeddingProvider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if name == "" {
		name = r.defaultName
	}

	provider, ok := r.providers[name]
	return provider, ok
}

// GetQdrantRepo returns just the Qdrant repository for the given name.
// If name is empty, returns the default repository.
func (r *EmbeddingRegistry) GetQdrantRepo(name string) (*repository.QdrantRepository, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if name == "" {
		name = r.defaultName
	}

	repo, ok := r.qdrantRepos[name]
	return repo, ok
}

// GetConfig returns the embedding configuration for the given name.
// If name is empty, returns the default configuration.
func (r *EmbeddingRegistry) GetConfig(name string) (*config.EmbeddingConfig, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if name == "" {
		name = r.defaultName
	}

	cfg, ok := r.configs[name]
	return cfg, ok
}

// Names returns all registered embedding configuration names.
func (r *EmbeddingRegistry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.configs))
	for name := range r.configs {
		names = append(names, name)
	}
	return names
}

// Count returns the number of registered embeddings.
func (r *EmbeddingRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.configs)
}

// Has checks if an embedding with the given name is registered.
func (r *EmbeddingRegistry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.configs[name]
	return ok
}

// EnsureCollections ensures all Qdrant collections exist.
// Errors are logged but do not stop the process.
func (r *EmbeddingRegistry) EnsureCollections(ctx context.Context) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var lastErr error
	for name, repo := range r.qdrantRepos {
		if err := repo.EnsureCollection(ctx); err != nil {
			logger.CtxWarn(ctx, "Failed to ensure collection: name=%s, error=%v", name, err)
			lastErr = err
		}
	}
	return lastErr
}

// Close releases all resources held by the registry.
// This should be called when the application shuts down.
func (r *EmbeddingRegistry) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()

	for name, repo := range r.qdrantRepos {
		if err := repo.Close(); err != nil {
			logger.Warn("Error closing Qdrant repository: name=%s, error=%v", name, err)
		}
	}

	// Clear maps
	r.configs = make(map[string]*config.EmbeddingConfig)
	r.providers = make(map[string]EmbeddingProvider)
	r.qdrantRepos = make(map[string]*repository.QdrantRepository)
}

// ForEach iterates over all registered embeddings and calls the provided function.
// The function receives the name, provider, and Qdrant repository for each embedding.
// If the function returns an error, iteration stops and the error is returned.
func (r *EmbeddingRegistry) ForEach(fn func(name string, provider EmbeddingProvider, repo *repository.QdrantRepository) error) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for name := range r.configs {
		if err := fn(name, r.providers[name], r.qdrantRepos[name]); err != nil {
			return err
		}
	}
	return nil
}

// GetCollectionName returns the Qdrant collection name for the given embedding.
// If name is empty, returns the default embedding's collection.
func (r *EmbeddingRegistry) GetCollectionName(name string) (string, bool) {
	cfg, ok := r.GetConfig(name)
	if !ok {
		return "", false
	}
	return cfg.Collection, true
}

// BuildProfileIngestIndexes resolves a search profile into ingest vector routes.
func (r *EmbeddingRegistry) BuildProfileIngestIndexes(profile *config.SearchProfileConfig) ([]IngestVectorIndex, error) {
	if profile == nil {
		return nil, fmt.Errorf("search profile is nil")
	}

	indexes := make([]IngestVectorIndex, 0, 2)
	if profile.ImageEmbedding != "" {
		provider, repo, ok := r.Get(profile.ImageEmbedding)
		if !ok {
			return nil, fmt.Errorf("unknown image embedding %q for profile %q", profile.ImageEmbedding, profile.Name)
		}
		indexes = append(indexes, IngestVectorIndex{
			VectorType: pb.VectorType_VECTOR_TYPE_IMAGE,
			Collection: repo.GetCollectionName(),
			Embedding:  provider,
			QdrantRepo: repo,
			UseSparse:  false,
		})
	}

	if profile.CaptionEmbedding != "" {
		provider, repo, ok := r.Get(profile.CaptionEmbedding)
		if !ok {
			return nil, fmt.Errorf("unknown caption embedding %q for profile %q", profile.CaptionEmbedding, profile.Name)
		}
		indexes = append(indexes, IngestVectorIndex{
			VectorType: pb.VectorType_VECTOR_TYPE_CAPTION,
			Collection: repo.GetCollectionName(),
			Embedding:  provider,
			QdrantRepo: repo,
			UseSparse:  true,
		})
	}

	if len(indexes) == 0 {
		return nil, fmt.Errorf("profile %q has no vector indexes", profile.Name)
	}
	return indexes, nil
}
