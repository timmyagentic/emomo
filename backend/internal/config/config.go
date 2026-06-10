package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

// Config aggregates application configuration loaded from files and environment.
type Config struct {
	Server     ServerConfig      `mapstructure:"server"`
	Database   DatabaseConfig    `mapstructure:"database"`
	Qdrant     QdrantConfig      `mapstructure:"qdrant"`
	Storage    StorageConfig     `mapstructure:"storage"`
	VLM        VLMConfig         `mapstructure:"vlm"`
	Embeddings []EmbeddingConfig `mapstructure:"embeddings"` // List of embedding configurations
	Ingest     IngestConfig      `mapstructure:"ingest"`
	Sources    SourcesConfig     `mapstructure:"sources"`
	Search     SearchConfig      `mapstructure:"search"`
}

// ServerConfig defines HTTP server settings.
type ServerConfig struct {
	Port      int             `mapstructure:"port"`
	Mode      string          `mapstructure:"mode"`
	CORS      CORSConfig      `mapstructure:"cors"`
	PublicAPI PublicAPIConfig `mapstructure:"public_api"`
}

// CORSConfig defines Cross-Origin Resource Sharing settings.
type CORSConfig struct {
	AllowedOrigins  []string `mapstructure:"allowed_origins"`
	AllowAllOrigins bool     `mapstructure:"allow_all_origins"`
}

// PublicAPIConfig controls cost and abuse guardrails for public clients.
type PublicAPIConfig struct {
	Enabled             bool  `mapstructure:"enabled"`
	RateLimitEnabled    bool  `mapstructure:"rate_limit_enabled"`
	RequestsPerMinute   int   `mapstructure:"requests_per_minute"`
	Burst               int   `mapstructure:"burst"`
	BodyLimitBytes      int64 `mapstructure:"body_limit_bytes"`
	SearchTopKMax       int32 `mapstructure:"search_top_k_max"`
	SearchQueryMaxRunes int   `mapstructure:"search_query_max_runes"`
	ListLimitMax        int32 `mapstructure:"list_limit_max"`
}

// DatabaseConfig defines database connection and pool settings.
type DatabaseConfig struct {
	Driver          string        `mapstructure:"driver"`            // Database driver: sqlite, postgres
	URL             string        `mapstructure:"url"`               // PostgreSQL connection URL (takes priority)
	Path            string        `mapstructure:"path"`              // SQLite path
	Host            string        `mapstructure:"host"`              // PostgreSQL host
	Port            int           `mapstructure:"port"`              // PostgreSQL port
	User            string        `mapstructure:"user"`              // PostgreSQL user
	Password        string        `mapstructure:"password"`          // PostgreSQL password
	DBName          string        `mapstructure:"dbname"`            // PostgreSQL db name
	SSLMode         string        `mapstructure:"sslmode"`           // PostgreSQL sslmode
	AutoMigrate     bool          `mapstructure:"auto_migrate"`      // Auto migrate schemas on startup
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`    // Connection pool: max idle
	MaxOpenConns    int           `mapstructure:"max_open_conns"`    // Connection pool: max open
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"` // Connection pool: max lifetime
}

// DSN builds the Data Source Name for the configured database.
func (c *DatabaseConfig) DSN() string {
	if c.Driver == "sqlite" {
		return c.Path
	}

	// If URL is explicitly provided, use it
	if c.URL != "" {
		return c.URL
	}

	// Build PostgreSQL DSN
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.DBName, c.SSLMode,
	)
	return dsn
}

// QdrantConfig defines Qdrant connection settings.
type QdrantConfig struct {
	Host       string `mapstructure:"host"`
	Port       int    `mapstructure:"port"`
	Collection string `mapstructure:"collection"` // Default collection name (fallback)
	APIKey     string `mapstructure:"api_key"`    // Qdrant Cloud API Key
	UseTLS     bool   `mapstructure:"use_tls"`    // Enable TLS (auto-enabled when APIKey is set)
}

// StorageConfig holds configuration for S3-compatible storage (R2, S3, etc.).
type StorageConfig struct {
	Type      string `mapstructure:"type"`       // "r2", "s3", "s3compatible"
	Endpoint  string `mapstructure:"endpoint"`   // S3 API endpoint
	AccessKey string `mapstructure:"access_key"` // Access key ID
	SecretKey string `mapstructure:"secret_key"` // Secret access key
	UseSSL    bool   `mapstructure:"use_ssl"`    // Use HTTPS
	Bucket    string `mapstructure:"bucket"`     // Bucket name
	Region    string `mapstructure:"region"`     // Region (for AWS S3)
	PublicURL string `mapstructure:"public_url"` // Public URL prefix (e.g., R2.dev domain)
}

// VLMConfig defines configuration for the Vision Language Model provider.
type VLMConfig struct {
	Provider string `mapstructure:"provider"`
	Model    string `mapstructure:"model"`
	APIKey   string `mapstructure:"api_key"`
	BaseURL  string `mapstructure:"base_url"`
}

// IngestConfig defines ingestion concurrency and batching settings.
type IngestConfig struct {
	Workers    int `mapstructure:"workers"`
	BatchSize  int `mapstructure:"batch_size"`
	RetryCount int `mapstructure:"retry_count"`
}

// SearchConfig defines search runtime settings.
type SearchConfig struct {
	ScoreThreshold float32               `mapstructure:"score_threshold"`
	DefaultProfile string                `mapstructure:"default_profile"`
	Profiles       []SearchProfileConfig `mapstructure:"profiles"`
	Retrieval      RetrievalConfig       `mapstructure:"retrieval"`
	QueryExpansion QueryExpansionConfig  `mapstructure:"query_expansion"`
	Agentic        AgenticSearchConfig   `mapstructure:"agentic"`
}

// SearchProfileConfig groups multiple embedding configs into one search profile.
type SearchProfileConfig struct {
	Name             string `mapstructure:"name"`
	ImageEmbedding   string `mapstructure:"image_embedding"`
	CaptionEmbedding string `mapstructure:"caption_embedding"`
	KeywordEmbedding string `mapstructure:"keyword_embedding"`
	IsDefault        bool   `mapstructure:"is_default"`
}

// RetrievalConfig defines multi-route retrieval limits and scoring weights.
type RetrievalConfig struct {
	ImageTopK   int              `mapstructure:"image_top_k"`
	CaptionTopK int              `mapstructure:"caption_top_k"`
	FinalTopK   int              `mapstructure:"final_top_k"`
	Weights     RetrievalWeights `mapstructure:"weights"`
}

// RetrievalWeights configures weighted rank fusion for multi-route search.
type RetrievalWeights struct {
	Image   float32 `mapstructure:"image"`
	Caption float32 `mapstructure:"caption"`
	Keyword float32 `mapstructure:"keyword"`
}

// QueryExpansionConfig configures optional LLM-based query expansion.
type QueryExpansionConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Model   string `mapstructure:"model"`
	APIKey  string `mapstructure:"api_key"`
	BaseURL string `mapstructure:"base_url"`
}

// AgenticSearchConfig configures optional LLM-planned search and reranking.
type AgenticSearchConfig struct {
	Enabled         bool          `mapstructure:"enabled"`
	PlannerModel    string        `mapstructure:"planner_model"`
	RerankerModel   string        `mapstructure:"reranker_model"`
	PlannerTimeout  time.Duration `mapstructure:"planner_timeout"`
	RerankerTimeout time.Duration `mapstructure:"reranker_timeout"`
	RerankTopK      int           `mapstructure:"rerank_top_k"`
	FallbackOnError bool          `mapstructure:"fallback_on_error"`
}

// SourcesConfig defines configuration for available data sources.
type SourcesConfig struct {
	LocalDir LocalDirConfig `mapstructure:"localdir"`
}

// LocalDirConfig defines configuration for the local static directory source.
type LocalDirConfig struct {
	Enabled      bool   `mapstructure:"enabled"`
	RootPath     string `mapstructure:"root_path"`
	SourceID     string `mapstructure:"source_id"`
	ManifestPath string `mapstructure:"manifest_path"`
	QueuePath    string `mapstructure:"queue_path"`
}

// Load reads configuration from file/environment and returns a Config.
// Parameters:
//   - configPath: optional explicit path to a config file.
//
// Returns:
//   - *Config: loaded configuration with defaults applied.
//   - error: non-nil if loading or unmarshalling fails.
func Load(configPath string) (*Config, error) {
	// Load .env file if exists
	LoadDotEnv()

	v := viper.New()

	// Set config file path
	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath("./configs")
		v.AddConfigPath(".")
	}

	// Enable environment variable override
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Set defaults
	setDefaults(v)

	// Read config file
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	// Bind environment variables for sensitive/override data
	bindEnvVars(v)

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Resolve environment variables for all embedding configurations
	for i := range cfg.Embeddings {
		cfg.Embeddings[i].ResolveEnvVars()
	}

	return &cfg, nil
}

// LoadDotEnv loads the local .env file into the process environment if present.
// Existing process environment values keep priority over file values.
func LoadDotEnv() {
	_ = godotenv.Load()
}

// setDefaults sets all default configuration values.
func setDefaults(v *viper.Viper) {
	// Server defaults
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.mode", "debug")
	v.SetDefault("server.cors.allow_all_origins", true)
	v.SetDefault("server.cors.allowed_origins", []string{})
	v.SetDefault("server.public_api.enabled", true)
	v.SetDefault("server.public_api.rate_limit_enabled", true)
	v.SetDefault("server.public_api.requests_per_minute", 60)
	v.SetDefault("server.public_api.burst", 20)
	v.SetDefault("server.public_api.body_limit_bytes", 16*1024)
	v.SetDefault("server.public_api.search_top_k_max", 100)
	v.SetDefault("server.public_api.search_query_max_runes", 160)
	v.SetDefault("server.public_api.list_limit_max", 60)

	// Database defaults
	v.SetDefault("database.driver", "sqlite")
	v.SetDefault("database.path", "./data/memes.db")
	v.SetDefault("database.host", "localhost")
	v.SetDefault("database.port", 5432)
	v.SetDefault("database.user", "")
	v.SetDefault("database.password", "")
	v.SetDefault("database.dbname", "emomo")
	v.SetDefault("database.sslmode", "disable")
	v.SetDefault("database.auto_migrate", true)
	v.SetDefault("database.max_idle_conns", 10)
	v.SetDefault("database.max_open_conns", 100)
	v.SetDefault("database.conn_max_lifetime", "1h")

	// Qdrant defaults
	v.SetDefault("qdrant.host", "localhost")
	v.SetDefault("qdrant.port", 6334)
	v.SetDefault("qdrant.collection", "emomo")
	v.SetDefault("qdrant.api_key", "")
	v.SetDefault("qdrant.use_tls", false)

	// Storage defaults
	v.SetDefault("storage.endpoint", "localhost:9000")
	v.SetDefault("storage.use_ssl", false)
	v.SetDefault("storage.bucket", "memes")

	// VLM defaults
	v.SetDefault("vlm.provider", "openai")
	v.SetDefault("vlm.model", "gpt-4o-mini")
	v.SetDefault("vlm.base_url", "https://api.openai.com/v1")

	// Ingest defaults
	v.SetDefault("ingest.workers", 5)
	v.SetDefault("ingest.batch_size", 10)
	v.SetDefault("ingest.retry_count", 3)

	// Sources defaults
	v.SetDefault("sources.localdir.enabled", true)
	v.SetDefault("sources.localdir.root_path", "./data/memes")
	v.SetDefault("sources.localdir.source_id", "localdir")
	v.SetDefault("sources.localdir.manifest_path", "")
	v.SetDefault("sources.localdir.queue_path", "")

	// Search defaults
	v.SetDefault("search.score_threshold", 0.0)
	v.SetDefault("search.default_profile", "")
	v.SetDefault("search.retrieval.image_top_k", 100)
	v.SetDefault("search.retrieval.caption_top_k", 100)
	v.SetDefault("search.retrieval.final_top_k", 100)
	v.SetDefault("search.retrieval.weights.image", 0.70)
	v.SetDefault("search.retrieval.weights.caption", 0.00)
	v.SetDefault("search.retrieval.weights.keyword", 0.30)
	v.SetDefault("search.query_expansion.enabled", true)
	v.SetDefault("search.query_expansion.model", "gpt-4o-mini")
	v.SetDefault("search.agentic.enabled", false)
	v.SetDefault("search.agentic.planner_timeout", "8s")
	v.SetDefault("search.agentic.reranker_timeout", "10s")
	v.SetDefault("search.agentic.rerank_top_k", 40)
	v.SetDefault("search.agentic.fallback_on_error", true)
}

// bindEnvVars binds environment variables to configuration keys.
func bindEnvVars(v *viper.Viper) {
	// Server
	v.BindEnv("server.port", "PORT")
	v.BindEnv("server.public_api.enabled", "PUBLIC_API_ENABLED")
	v.BindEnv("server.public_api.rate_limit_enabled", "PUBLIC_API_RATE_LIMIT_ENABLED")
	v.BindEnv("server.public_api.requests_per_minute", "PUBLIC_API_REQUESTS_PER_MINUTE")
	v.BindEnv("server.public_api.burst", "PUBLIC_API_BURST")
	v.BindEnv("server.public_api.body_limit_bytes", "PUBLIC_API_BODY_LIMIT_BYTES")
	v.BindEnv("server.public_api.search_top_k_max", "PUBLIC_API_SEARCH_TOP_K_MAX")
	v.BindEnv("server.public_api.search_query_max_runes", "PUBLIC_API_SEARCH_QUERY_MAX_RUNES")
	v.BindEnv("server.public_api.list_limit_max", "PUBLIC_API_LIST_LIMIT_MAX")

	// Database
	v.BindEnv("database.driver", "DATABASE_DRIVER")
	v.BindEnv("database.url", "DATABASE_URL")
	v.BindEnv("database.path", "DATABASE_PATH")
	v.BindEnv("database.host", "DATABASE_HOST")
	v.BindEnv("database.port", "DATABASE_PORT")
	v.BindEnv("database.user", "DATABASE_USER")
	v.BindEnv("database.password", "DATABASE_PASSWORD")
	v.BindEnv("database.dbname", "DATABASE_DBNAME")
	v.BindEnv("database.sslmode", "DATABASE_SSLMODE")
	v.BindEnv("database.auto_migrate", "DATABASE_AUTO_MIGRATE")

	// Qdrant
	v.BindEnv("qdrant.host", "QDRANT_HOST")
	v.BindEnv("qdrant.port", "QDRANT_PORT")
	v.BindEnv("qdrant.collection", "QDRANT_COLLECTION")
	v.BindEnv("qdrant.api_key", "QDRANT_API_KEY")
	v.BindEnv("qdrant.use_tls", "QDRANT_USE_TLS")

	// Storage
	v.BindEnv("storage.type", "STORAGE_TYPE")
	v.BindEnv("storage.endpoint", "STORAGE_ENDPOINT")
	v.BindEnv("storage.access_key", "STORAGE_ACCESS_KEY")
	v.BindEnv("storage.secret_key", "STORAGE_SECRET_KEY")
	v.BindEnv("storage.use_ssl", "STORAGE_USE_SSL")
	v.BindEnv("storage.bucket", "STORAGE_BUCKET")
	v.BindEnv("storage.region", "STORAGE_REGION")
	v.BindEnv("storage.public_url", "STORAGE_PUBLIC_URL")

	// VLM
	v.BindEnv("vlm.api_key", "OPENAI_API_KEY")
	v.BindEnv("vlm.base_url", "OPENAI_BASE_URL")
	v.BindEnv("vlm.model", "VLM_MODEL")

	// Search
	v.BindEnv("search.score_threshold", "SEARCH_SCORE_THRESHOLD")
	v.BindEnv("search.query_expansion.model", "QUERY_EXPANSION_MODEL")
	v.BindEnv("search.query_expansion.api_key", "QUERY_EXPANSION_API_KEY")
	v.BindEnv("search.query_expansion.base_url", "QUERY_EXPANSION_BASE_URL")
	v.BindEnv("search.agentic.enabled", "AGENTIC_SEARCH_ENABLED")
	v.BindEnv("search.agentic.planner_model", "AGENTIC_SEARCH_PLANNER_MODEL")
	v.BindEnv("search.agentic.reranker_model", "AGENTIC_SEARCH_RERANKER_MODEL")
	v.BindEnv("search.agentic.planner_timeout", "AGENTIC_SEARCH_PLANNER_TIMEOUT")
	v.BindEnv("search.agentic.reranker_timeout", "AGENTIC_SEARCH_RERANKER_TIMEOUT")
	v.BindEnv("search.agentic.rerank_top_k", "AGENTIC_SEARCH_RERANK_TOP_K")
	v.BindEnv("search.agentic.fallback_on_error", "AGENTIC_SEARCH_FALLBACK_ON_ERROR")

	// Sources
	v.BindEnv("sources.localdir.root_path", "LOCAL_MEMES_DIR")
	v.BindEnv("sources.localdir.source_id", "LOCALDIR_SOURCE_ID")
	v.BindEnv("sources.localdir.manifest_path", "LOCALDIR_MANIFEST_PATH")
	v.BindEnv("sources.localdir.queue_path", "LOCALDIR_QUEUE_PATH")
}

// GetStorageConfig returns the storage configuration.
func (c *Config) GetStorageConfig() *StorageConfig {
	return &c.Storage
}

// GetDefaultEmbedding returns the default embedding configuration.
// Returns nil if no embeddings are configured or no default is set.
func (c *Config) GetDefaultEmbedding() *EmbeddingConfig {
	// First, look for explicitly marked default
	for i := range c.Embeddings {
		if c.Embeddings[i].IsDefault {
			return &c.Embeddings[i]
		}
	}

	// Fall back to first embedding if available
	if len(c.Embeddings) > 0 {
		return &c.Embeddings[0]
	}

	return nil
}

// GetEmbeddingByName returns the embedding configuration with the given name.
// Returns nil if not found.
func (c *Config) GetEmbeddingByName(name string) *EmbeddingConfig {
	for i := range c.Embeddings {
		if c.Embeddings[i].Name == name {
			return &c.Embeddings[i]
		}
	}
	return nil
}

// GetDefaultCollection returns the default Qdrant collection name.
// It uses the default embedding's collection, or falls back to Qdrant.Collection.
func (c *Config) GetDefaultCollection() string {
	if defaultEmb := c.GetDefaultEmbedding(); defaultEmb != nil {
		if defaultEmb.Collection != "" {
			return defaultEmb.Collection
		}
	}
	return c.Qdrant.Collection
}

// GetSearchProfileByName returns the search profile with the given name.
func (c *Config) GetSearchProfileByName(name string) *SearchProfileConfig {
	for i := range c.Search.Profiles {
		if c.Search.Profiles[i].Name == name {
			return &c.Search.Profiles[i]
		}
	}
	return nil
}

// GetDefaultSearchProfile returns the configured default search profile.
func (c *Config) GetDefaultSearchProfile() *SearchProfileConfig {
	if c.Search.DefaultProfile != "" {
		if profile := c.GetSearchProfileByName(c.Search.DefaultProfile); profile != nil {
			return profile
		}
	}
	for i := range c.Search.Profiles {
		if c.Search.Profiles[i].IsDefault {
			return &c.Search.Profiles[i]
		}
	}
	if len(c.Search.Profiles) > 0 {
		return &c.Search.Profiles[0]
	}
	return nil
}
