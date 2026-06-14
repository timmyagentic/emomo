package configcenter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientFetchRemoteConfig(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer read-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"version": "2026-06-14T10:00:00Z",
			"updated_at": "2026-06-14T10:00:00Z",
			"config": {
				"search": {
					"query_expansion": {
						"model": "full-config-model"
					}
				}
			},
			"query_expansion": {
				"enabled": true,
				"model": "test-model",
				"api_key": "test-key",
				"base_url": "https://example.com/v1"
			}
		}`))
	}))
	defer srv.Close()

	client := NewClient(ClientConfig{
		URL:     srv.URL,
		Token:   "read-token",
		Timeout: time.Second,
	})

	cfg, err := client.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if cfg.Version != "2026-06-14T10:00:00Z" {
		t.Fatalf("version = %q, want timestamp", cfg.Version)
	}
	if cfg.Config == nil {
		t.Fatal("config missing")
	}
	if cfg.QueryExpansion.Enabled == nil || !*cfg.QueryExpansion.Enabled {
		t.Fatal("query_expansion.enabled missing or false")
	}
	if cfg.QueryExpansion.Model == nil || *cfg.QueryExpansion.Model != "test-model" {
		t.Fatalf("query_expansion.model = %v, want test-model", cfg.QueryExpansion.Model)
	}
	if cfg.QueryExpansion.APIKey == nil || *cfg.QueryExpansion.APIKey != "test-key" {
		t.Fatal("query_expansion.api_key missing")
	}
	if cfg.QueryExpansion.BaseURL == nil || *cfg.QueryExpansion.BaseURL != "https://example.com/v1" {
		t.Fatalf("query_expansion.base_url = %v, want https://example.com/v1", cfg.QueryExpansion.BaseURL)
	}
}

func TestRemoteConfigRuntimeConfigMergesLegacyQueryExpansion(t *testing.T) {
	t.Parallel()

	model := "legacy-model"
	remote := &RemoteConfig{
		Config: map[string]any{
			"vlm": map[string]any{
				"base_url": "https://vlm.example.com/v1",
			},
			"search": map[string]any{
				"query_expansion": map[string]any{
					"enabled": false,
				},
			},
		},
		QueryExpansion: RemoteQueryExpansionConfig{
			Model: &model,
		},
	}

	runtimeConfig := remote.RuntimeConfig()
	search := runtimeConfig["search"].(map[string]any)
	qe := search["query_expansion"].(map[string]any)
	if qe["enabled"] != false {
		t.Fatalf("enabled = %v, want false", qe["enabled"])
	}
	if qe["model"] != "legacy-model" {
		t.Fatalf("model = %v, want legacy-model", qe["model"])
	}
}

func TestClientFetchRejectsUnknownFields(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"query_expansion": {}, "unexpected": true}`))
	}))
	defer srv.Close()

	client := NewClient(ClientConfig{URL: srv.URL})
	if _, err := client.Fetch(context.Background()); err == nil {
		t.Fatal("Fetch() error = nil, want unknown field error")
	}
}
