package config

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadDefaultsSearchRetrievalFinalTopKTo100(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("search:\n  retrieval:\n    image_top_k: 42\n"), 0o600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Search.Retrieval.ImageTopK != 42 {
		t.Fatalf("image_top_k = %d, want config override 42", cfg.Search.Retrieval.ImageTopK)
	}
	if cfg.Search.Retrieval.FinalTopK != 100 {
		t.Fatalf("final_top_k default = %d, want 100", cfg.Search.Retrieval.FinalTopK)
	}
	if cfg.Search.Retrieval.Weights.Image != 0.70 {
		t.Fatalf("image weight default = %v, want 0.70", cfg.Search.Retrieval.Weights.Image)
	}
	if cfg.Search.Retrieval.Weights.Caption != 0.00 {
		t.Fatalf("caption weight default = %v, want 0.00", cfg.Search.Retrieval.Weights.Caption)
	}
	if cfg.Search.Retrieval.Weights.Keyword != 0.30 {
		t.Fatalf("keyword weight default = %v, want 0.30", cfg.Search.Retrieval.Weights.Keyword)
	}
}

func TestLoadDefaultsAgenticSearchConfig(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("search: {}\n"), 0o600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Search.Agentic.Enabled {
		t.Fatal("agentic.enabled = true, want false by default")
	}
	if cfg.Search.Agentic.PlannerTimeout != 8*time.Second {
		t.Fatalf("planner_timeout = %s, want 8s", cfg.Search.Agentic.PlannerTimeout)
	}
	if cfg.Search.Agentic.RerankerTimeout != 10*time.Second {
		t.Fatalf("reranker_timeout = %s, want 10s", cfg.Search.Agentic.RerankerTimeout)
	}
	if cfg.Search.Agentic.RerankTopK != 40 {
		t.Fatalf("rerank_top_k = %d, want 40", cfg.Search.Agentic.RerankTopK)
	}
	if !cfg.Search.Agentic.FallbackOnError {
		t.Fatal("fallback_on_error = false, want true by default")
	}
}

func TestLoadDefaultsConfigCenter(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.ConfigCenter.Enabled {
		t.Fatal("config_center.enabled = true, want false by default")
	}
	if cfg.ConfigCenter.Required {
		t.Fatal("config_center.required = true, want false by default")
	}
	if cfg.ConfigCenter.URL != "" {
		t.Fatalf("config_center.url = %q, want empty", cfg.ConfigCenter.URL)
	}
	if cfg.ConfigCenter.PollInterval != time.Minute {
		t.Fatalf("config_center.poll_interval = %s, want 1m", cfg.ConfigCenter.PollInterval)
	}
	if cfg.ConfigCenter.Timeout != 5*time.Second {
		t.Fatalf("config_center.timeout = %s, want 5s", cfg.ConfigCenter.Timeout)
	}
	if cfg.Logging.Level != "info" {
		t.Fatalf("logging.level = %q, want info", cfg.Logging.Level)
	}
	if cfg.Logging.LokiProject != "emomo" {
		t.Fatalf("logging.loki_project = %q, want emomo", cfg.Logging.LokiProject)
	}
}

func TestLoadBindsConfigCenterEnv(t *testing.T) {
	t.Setenv("CONFIG_CENTER_SKIP_REMOTE", "true")
	t.Setenv("CONFIG_CENTER_ENABLED", "true")
	t.Setenv("CONFIG_CENTER_REQUIRED", "true")
	t.Setenv("CONFIG_CENTER_URL", "https://config.example.com/v1/config/emomo/production/emomo-api")
	t.Setenv("CONFIG_CENTER_TOKEN", "read-token")
	t.Setenv("CONFIG_CENTER_POLL_INTERVAL", "15s")
	t.Setenv("CONFIG_CENTER_TIMEOUT", "2s")
	t.Setenv("QUERY_EXPANSION_ENABLED", "false")

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if !cfg.ConfigCenter.Enabled {
		t.Fatal("config_center.enabled = false, want true")
	}
	if !cfg.ConfigCenter.Required {
		t.Fatal("config_center.required = false, want true")
	}
	if cfg.ConfigCenter.URL != "https://config.example.com/v1/config/emomo/production/emomo-api" {
		t.Fatalf("config_center.url = %q", cfg.ConfigCenter.URL)
	}
	if cfg.ConfigCenter.Token != "read-token" {
		t.Fatalf("config_center.token = %q, want read-token", cfg.ConfigCenter.Token)
	}
	if cfg.ConfigCenter.PollInterval != 15*time.Second {
		t.Fatalf("config_center.poll_interval = %s, want 15s", cfg.ConfigCenter.PollInterval)
	}
	if cfg.ConfigCenter.Timeout != 2*time.Second {
		t.Fatalf("config_center.timeout = %s, want 2s", cfg.ConfigCenter.Timeout)
	}
	if cfg.Search.QueryExpansion.Enabled {
		t.Fatal("query_expansion.enabled = true, want env override false")
	}
}

func TestLoadMergesConfigCenterConfigAboveEnv(t *testing.T) {
	t.Setenv("QUERY_EXPANSION_MODEL", "local-model")
	t.Setenv("CONFIG_CENTER_ENABLED", "true")
	t.Setenv("CONFIG_CENTER_REQUIRED", "true")
	t.Setenv("CONFIG_CENTER_TOKEN", "read-token")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer read-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"version": "remote-version",
			"config": {
				"database": {
					"driver": "postgres",
					"url": "postgresql://remote"
				},
				"search": {
					"query_expansion": {
						"enabled": true,
						"model": "remote-model",
						"api_key": "remote-key",
						"base_url": "https://remote.example.com/v1"
					}
				},
				"logging": {
					"level": "warn",
					"loki_enabled": true,
					"loki_password": "remote-loki-token"
				}
			}
		}`))
	}))
	defer srv.Close()
	t.Setenv("CONFIG_CENTER_URL", srv.URL)

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("search:\n  query_expansion:\n    model: yaml-model\n"), 0o600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Database.Driver != "postgres" {
		t.Fatalf("database.driver = %q, want postgres", cfg.Database.Driver)
	}
	if cfg.Database.URL != "postgresql://remote" {
		t.Fatalf("database.url = %q, want remote", cfg.Database.URL)
	}
	if cfg.Search.QueryExpansion.Model != "remote-model" {
		t.Fatalf("query expansion model = %q, want remote-model", cfg.Search.QueryExpansion.Model)
	}
	if cfg.Search.QueryExpansion.APIKey != "remote-key" {
		t.Fatalf("query expansion api key = %q, want remote-key", cfg.Search.QueryExpansion.APIKey)
	}
	if cfg.Search.QueryExpansion.BaseURL != "https://remote.example.com/v1" {
		t.Fatalf("query expansion base url = %q, want remote", cfg.Search.QueryExpansion.BaseURL)
	}
	if cfg.Logging.Level != "warn" {
		t.Fatalf("logging.level = %q, want warn", cfg.Logging.Level)
	}
	if !cfg.Logging.LokiEnabled {
		t.Fatal("logging.loki_enabled = false, want true")
	}
	if cfg.Logging.LokiPassword != "remote-loki-token" {
		t.Fatalf("logging.loki_password = %q, want remote-loki-token", cfg.Logging.LokiPassword)
	}
}

func TestLoadRequiredConfigCenterFailsClosed(t *testing.T) {
	t.Setenv("CONFIG_CENTER_ENABLED", "true")
	t.Setenv("CONFIG_CENTER_REQUIRED", "true")
	t.Setenv("CONFIG_CENTER_URL", "http://127.0.0.1:1/config")
	t.Setenv("CONFIG_CENTER_TIMEOUT", "1ms")

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	if _, err := Load(configPath); err == nil {
		t.Fatal("Load() error = nil, want config center failure")
	}
}

func TestLoadDefaultsPublicAPIConfig(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("server: {}\n"), 0o600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if !cfg.Server.PublicAPI.Enabled {
		t.Fatal("public_api.enabled = false, want true")
	}
	if !cfg.Server.PublicAPI.RateLimitEnabled {
		t.Fatal("public_api.rate_limit_enabled = false, want true")
	}
	if cfg.Server.PublicAPI.RequestsPerMinute != 60 {
		t.Fatalf("requests_per_minute = %d, want 60", cfg.Server.PublicAPI.RequestsPerMinute)
	}
	if cfg.Server.PublicAPI.Burst != 20 {
		t.Fatalf("burst = %d, want 20", cfg.Server.PublicAPI.Burst)
	}
	if cfg.Server.PublicAPI.BodyLimitBytes != 16*1024 {
		t.Fatalf("body_limit_bytes = %d, want 16384", cfg.Server.PublicAPI.BodyLimitBytes)
	}
	if cfg.Server.PublicAPI.SearchTopKMax != 100 {
		t.Fatalf("search_top_k_max = %d, want 100", cfg.Server.PublicAPI.SearchTopKMax)
	}
	if cfg.Server.PublicAPI.SearchQueryMaxRunes != 160 {
		t.Fatalf("search_query_max_runes = %d, want 160", cfg.Server.PublicAPI.SearchQueryMaxRunes)
	}
	if cfg.Server.PublicAPI.ListLimitMax != 60 {
		t.Fatalf("list_limit_max = %d, want 60", cfg.Server.PublicAPI.ListLimitMax)
	}
}

func TestLoadBindsLocalAnalyzerEnv(t *testing.T) {
	t.Setenv("VLM_PROVIDER", "local_text_presence")
	t.Setenv("VLM_MODEL", "local-text-presence-test")
	t.Setenv("LOCAL_ANALYZER_COMMAND", "/usr/local/bin/tesseract")
	t.Setenv("LOCAL_ANALYZER_LANG", "eng")
	t.Setenv("LOCAL_ANALYZER_PSM", "7")

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("vlm: {}\n"), 0o600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.VLM.Provider != "local_text_presence" {
		t.Fatalf("vlm.provider = %q, want local_text_presence", cfg.VLM.Provider)
	}
	if cfg.VLM.Model != "local-text-presence-test" {
		t.Fatalf("vlm.model = %q, want local-text-presence-test", cfg.VLM.Model)
	}
	if cfg.VLM.LocalAnalyzerCommand != "/usr/local/bin/tesseract" {
		t.Fatalf("vlm.local_analyzer_command = %q, want /usr/local/bin/tesseract", cfg.VLM.LocalAnalyzerCommand)
	}
	if cfg.VLM.LocalAnalyzerLang != "eng" {
		t.Fatalf("vlm.local_analyzer_lang = %q, want eng", cfg.VLM.LocalAnalyzerLang)
	}
	if cfg.VLM.LocalAnalyzerPSM != "7" {
		t.Fatalf("vlm.local_analyzer_psm = %q, want 7", cfg.VLM.LocalAnalyzerPSM)
	}
}

func TestConfigDefaultSearchProfileUsesExplicitDefault(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Search: SearchConfig{
			DefaultProfile: "qwen3vl",
			Profiles: []SearchProfileConfig{
				{Name: "legacy", ImageEmbedding: "jina", CaptionEmbedding: "jina"},
				{Name: "qwen3vl", ImageEmbedding: "qwen3vl_image", KeywordEmbedding: "qwen3vl_caption", IsDefault: true},
			},
		},
	}

	profile := cfg.GetDefaultSearchProfile()
	if profile == nil {
		t.Fatal("expected default search profile")
	}
	if profile.Name != "qwen3vl" {
		t.Fatalf("default profile name = %q, want qwen3vl", profile.Name)
	}
}

func TestConfigGetSearchProfileByName(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Search: SearchConfig{
			Profiles: []SearchProfileConfig{
				{Name: "qwen3vl", ImageEmbedding: "qwen3vl_image", KeywordEmbedding: "qwen3vl_caption"},
			},
		},
	}

	profile := cfg.GetSearchProfileByName("qwen3vl")
	if profile == nil {
		t.Fatal("expected qwen3vl profile")
	}
	if profile.ImageEmbedding != "qwen3vl_image" {
		t.Fatalf("image embedding = %q, want qwen3vl_image", profile.ImageEmbedding)
	}
	if profile.CaptionEmbedding != "" {
		t.Fatalf("caption embedding = %q, want empty", profile.CaptionEmbedding)
	}
	if profile.KeywordEmbedding != "qwen3vl_caption" {
		t.Fatalf("keyword embedding = %q, want qwen3vl_caption", profile.KeywordEmbedding)
	}
}
