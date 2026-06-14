package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/timmy/emomo/internal/config"
)

type secretRefError struct {
	Field     string
	SecretEnv string
}

func main() {
	var allowMissingSecretRefs bool
	flag.BoolVar(&allowMissingSecretRefs, "allow-missing-secret-refs", false, "omit secret values even when no *_SECRET binding env is configured")
	flag.Parse()

	_ = os.Setenv("CONFIG_CENTER_SKIP_REMOTE", "true")

	cfg, err := config.Load(os.Getenv("CONFIG_PATH"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	var missing []secretRefError
	payload := map[string]any{
		"version": time.Now().UTC().Format(time.RFC3339),
		"config":  buildConfigPayload(cfg, &missing),
	}

	if len(missing) > 0 && !allowMissingSecretRefs {
		fmt.Fprintln(os.Stderr, "refusing to publish raw secrets to KV; create Cloudflare Secrets Store bindings and set:")
		for _, item := range missing {
			fmt.Fprintf(os.Stderr, "  %s for %s\n", item.SecretEnv, item.Field)
		}
		os.Exit(1)
	}

	if err := json.NewEncoder(os.Stdout).Encode(payload); err != nil {
		fmt.Fprintf(os.Stderr, "encode payload: %v\n", err)
		os.Exit(1)
	}
}

func buildConfigPayload(cfg *config.Config, missing *[]secretRefError) map[string]any {
	return map[string]any{
		"server":     serverPayload(cfg),
		"database":   databasePayload(cfg, missing),
		"qdrant":     qdrantPayload(cfg, missing),
		"storage":    storagePayload(cfg, missing),
		"vlm":        vlmPayload(cfg, missing),
		"embeddings": embeddingsPayload(cfg, missing),
		"ingest":     ingestPayload(cfg),
		"sources":    sourcesPayload(cfg),
		"search":     searchPayload(cfg, missing),
		"logging":    loggingPayload(cfg, missing),
	}
}

func serverPayload(cfg *config.Config) map[string]any {
	return map[string]any{
		"port": cfg.Server.Port,
		"mode": cfg.Server.Mode,
		"cors": map[string]any{
			"allowed_origins":   cfg.Server.CORS.AllowedOrigins,
			"allow_all_origins": cfg.Server.CORS.AllowAllOrigins,
		},
		"public_api": map[string]any{
			"enabled":                cfg.Server.PublicAPI.Enabled,
			"rate_limit_enabled":     cfg.Server.PublicAPI.RateLimitEnabled,
			"requests_per_minute":    cfg.Server.PublicAPI.RequestsPerMinute,
			"burst":                  cfg.Server.PublicAPI.Burst,
			"body_limit_bytes":       cfg.Server.PublicAPI.BodyLimitBytes,
			"search_top_k_max":       cfg.Server.PublicAPI.SearchTopKMax,
			"search_query_max_runes": cfg.Server.PublicAPI.SearchQueryMaxRunes,
			"list_limit_max":         cfg.Server.PublicAPI.ListLimitMax,
		},
	}
}

func databasePayload(cfg *config.Config, missing *[]secretRefError) map[string]any {
	out := map[string]any{
		"driver":            cfg.Database.Driver,
		"path":              cfg.Database.Path,
		"host":              cfg.Database.Host,
		"port":              cfg.Database.Port,
		"user":              cfg.Database.User,
		"dbname":            cfg.Database.DBName,
		"sslmode":           cfg.Database.SSLMode,
		"auto_migrate":      cfg.Database.AutoMigrate,
		"max_idle_conns":    cfg.Database.MaxIdleConns,
		"max_open_conns":    cfg.Database.MaxOpenConns,
		"conn_max_lifetime": cfg.Database.ConnMaxLifetime.String(),
	}
	addSecretRef(out, "database.url", "url", cfg.Database.URL, "DATABASE_URL_SECRET", missing)
	addSecretRef(out, "database.password", "password", cfg.Database.Password, "DATABASE_PASSWORD_SECRET", missing)
	return out
}

func qdrantPayload(cfg *config.Config, missing *[]secretRefError) map[string]any {
	out := map[string]any{
		"host":       cfg.Qdrant.Host,
		"port":       cfg.Qdrant.Port,
		"collection": cfg.Qdrant.Collection,
		"use_tls":    cfg.Qdrant.UseTLS,
	}
	addSecretRef(out, "qdrant.api_key", "api_key", cfg.Qdrant.APIKey, "QDRANT_API_KEY_SECRET", missing)
	return out
}

func storagePayload(cfg *config.Config, missing *[]secretRefError) map[string]any {
	out := map[string]any{
		"type":       cfg.Storage.Type,
		"endpoint":   cfg.Storage.Endpoint,
		"use_ssl":    cfg.Storage.UseSSL,
		"bucket":     cfg.Storage.Bucket,
		"region":     cfg.Storage.Region,
		"public_url": cfg.Storage.PublicURL,
	}
	addSecretRef(out, "storage.access_key", "access_key", cfg.Storage.AccessKey, "STORAGE_ACCESS_KEY_SECRET", missing)
	addSecretRef(out, "storage.secret_key", "secret_key", cfg.Storage.SecretKey, "STORAGE_SECRET_KEY_SECRET", missing)
	return out
}

func vlmPayload(cfg *config.Config, missing *[]secretRefError) map[string]any {
	out := map[string]any{
		"provider":               cfg.VLM.Provider,
		"model":                  cfg.VLM.Model,
		"base_url":               cfg.VLM.BaseURL,
		"local_analyzer_command": cfg.VLM.LocalAnalyzerCommand,
		"local_analyzer_lang":    cfg.VLM.LocalAnalyzerLang,
		"local_analyzer_psm":     cfg.VLM.LocalAnalyzerPSM,
	}
	addSecretRef(out, "vlm.api_key", "api_key", cfg.VLM.APIKey, "OPENAI_API_KEY_SECRET", missing)
	return out
}

func embeddingsPayload(cfg *config.Config, missing *[]secretRefError) []map[string]any {
	out := make([]map[string]any, 0, len(cfg.Embeddings))
	for _, embedding := range cfg.Embeddings {
		item := map[string]any{
			"name":          embedding.Name,
			"provider":      embedding.Provider,
			"model":         embedding.Model,
			"base_url":      embedding.BaseURL,
			"document_mode": embedding.GetDocumentMode(),
			"dimensions":    embedding.Dimensions,
			"collection":    embedding.Collection,
			"is_default":    embedding.IsDefault,
		}
		secretEnv := embeddingSecretEnv(embedding)
		addSecretRef(item, fmt.Sprintf("embeddings.%s.api_key", embedding.Name), "api_key", embedding.APIKey, secretEnv, missing)
		out = append(out, item)
	}
	return out
}

func ingestPayload(cfg *config.Config) map[string]any {
	return map[string]any{
		"workers":     cfg.Ingest.Workers,
		"batch_size":  cfg.Ingest.BatchSize,
		"retry_count": cfg.Ingest.RetryCount,
	}
}

func sourcesPayload(cfg *config.Config) map[string]any {
	return map[string]any{
		"localdir": map[string]any{
			"enabled":       cfg.Sources.LocalDir.Enabled,
			"root_path":     cfg.Sources.LocalDir.RootPath,
			"source_id":     cfg.Sources.LocalDir.SourceID,
			"manifest_path": cfg.Sources.LocalDir.ManifestPath,
			"queue_path":    cfg.Sources.LocalDir.QueuePath,
		},
	}
}

func searchPayload(cfg *config.Config, missing *[]secretRefError) map[string]any {
	profiles := make([]map[string]any, 0, len(cfg.Search.Profiles))
	for _, profile := range cfg.Search.Profiles {
		profiles = append(profiles, map[string]any{
			"name":              profile.Name,
			"image_embedding":   profile.ImageEmbedding,
			"caption_embedding": profile.CaptionEmbedding,
			"keyword_embedding": profile.KeywordEmbedding,
			"is_default":        profile.IsDefault,
		})
	}

	queryExpansion := map[string]any{
		"enabled":  cfg.Search.QueryExpansion.Enabled,
		"model":    cfg.Search.QueryExpansion.Model,
		"base_url": cfg.Search.QueryExpansion.BaseURL,
	}
	addSecretRef(queryExpansion, "search.query_expansion.api_key", "api_key", cfg.Search.QueryExpansion.APIKey, "QUERY_EXPANSION_API_KEY_SECRET", missing)

	return map[string]any{
		"score_threshold": cfg.Search.ScoreThreshold,
		"default_profile": cfg.Search.DefaultProfile,
		"profiles":        profiles,
		"retrieval": map[string]any{
			"image_top_k":   cfg.Search.Retrieval.ImageTopK,
			"caption_top_k": cfg.Search.Retrieval.CaptionTopK,
			"final_top_k":   cfg.Search.Retrieval.FinalTopK,
			"weights": map[string]any{
				"image":   cfg.Search.Retrieval.Weights.Image,
				"caption": cfg.Search.Retrieval.Weights.Caption,
				"keyword": cfg.Search.Retrieval.Weights.Keyword,
			},
		},
		"query_expansion": queryExpansion,
		"agentic": map[string]any{
			"enabled":           cfg.Search.Agentic.Enabled,
			"planner_model":     cfg.Search.Agentic.PlannerModel,
			"reranker_model":    cfg.Search.Agentic.RerankerModel,
			"planner_timeout":   cfg.Search.Agentic.PlannerTimeout.String(),
			"reranker_timeout":  cfg.Search.Agentic.RerankerTimeout.String(),
			"rerank_top_k":      cfg.Search.Agentic.RerankTopK,
			"fallback_on_error": cfg.Search.Agentic.FallbackOnError,
		},
	}
}

func loggingPayload(cfg *config.Config, missing *[]secretRefError) map[string]any {
	out := map[string]any{
		"level":               cfg.Logging.Level,
		"format":              cfg.Logging.Format,
		"service_name":        cfg.Logging.ServiceName,
		"environment":         cfg.Logging.Environment,
		"log_file":            cfg.Logging.LogFile,
		"log_file_only":       cfg.Logging.LogFileOnly,
		"max_size":            cfg.Logging.MaxSize,
		"max_backups":         cfg.Logging.MaxBackups,
		"max_age":             cfg.Logging.MaxAge,
		"compress":            cfg.Logging.Compress,
		"loki_enabled":        cfg.Logging.LokiEnabled,
		"loki_url":            cfg.Logging.LokiURL,
		"loki_username":       cfg.Logging.LokiUsername,
		"loki_project":        cfg.Logging.LokiProject,
		"cluster_name":        cfg.Logging.ClusterName,
		"loki_batch_size":     cfg.Logging.LokiBatchSize,
		"loki_queue_size":     cfg.Logging.LokiQueueSize,
		"loki_flush_interval": cfg.Logging.LokiFlushInterval.String(),
		"loki_timeout":        cfg.Logging.LokiTimeout.String(),
	}
	addSecretRef(out, "logging.loki_password", "loki_password", cfg.Logging.LokiPassword, "LOKI_PASSWORD_SECRET", missing)
	return out
}

func addSecretRef(out map[string]any, fieldPath string, field string, value string, secretEnv string, missing *[]secretRefError) {
	secretRef := strings.TrimSpace(os.Getenv(secretEnv))
	if secretRef != "" {
		out[field+"_secret"] = secretRef
		return
	}
	if strings.TrimSpace(value) == "" {
		return
	}
	*missing = append(*missing, secretRefError{
		Field:     fieldPath,
		SecretEnv: secretEnv,
	})
}

func embeddingSecretEnv(embedding config.EmbeddingConfig) string {
	if embedding.APIKeyEnv != "" {
		return embedding.APIKeyEnv + "_SECRET"
	}
	return "EMBEDDING_" + normalizeEnvName(embedding.Name) + "_API_KEY_SECRET"
}

var nonEnvChar = regexp.MustCompile(`[^A-Z0-9]+`)

func normalizeEnvName(value string) string {
	normalized := strings.ToUpper(value)
	normalized = nonEnvChar.ReplaceAllString(normalized, "_")
	normalized = strings.Trim(normalized, "_")
	if normalized == "" {
		return "DEFAULT"
	}
	return normalized
}
