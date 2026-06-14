package main

import (
	"testing"

	"github.com/timmy/emomo/internal/configcenter"
	"github.com/timmy/emomo/internal/service"
)

func strPtr(value string) *string {
	return &value
}

func boolPtr(value bool) *bool {
	return &value
}

func TestApplyRemoteQueryExpansionConfigOverridesOnlyProvidedFields(t *testing.T) {
	t.Parallel()

	base := service.QueryExpansionConfig{
		Enabled: true,
		Model:   "local-model",
		APIKey:  "local-key",
		BaseURL: "https://local.example.com/v1",
	}

	next := applyRemoteQueryExpansionConfig(base, configcenter.RemoteQueryExpansionConfig{
		Model:   strPtr("remote-model"),
		BaseURL: strPtr("https://remote.example.com/v1"),
	})

	if !next.Enabled {
		t.Fatal("enabled = false, want base value true")
	}
	if next.Model != "remote-model" {
		t.Fatalf("model = %q, want remote-model", next.Model)
	}
	if next.APIKey != "local-key" {
		t.Fatalf("api key was overridden unexpectedly")
	}
	if next.BaseURL != "https://remote.example.com/v1" {
		t.Fatalf("base url = %q, want remote", next.BaseURL)
	}
}

func TestApplyRemoteQueryExpansionConfigCanDisableExpansion(t *testing.T) {
	t.Parallel()

	base := service.QueryExpansionConfig{
		Enabled: true,
		Model:   "local-model",
		APIKey:  "local-key",
		BaseURL: "https://local.example.com/v1",
	}

	next := applyRemoteQueryExpansionConfig(base, configcenter.RemoteQueryExpansionConfig{
		Enabled: boolPtr(false),
	})

	if next.Enabled {
		t.Fatal("enabled = true, want false")
	}
	if next.Model != "local-model" {
		t.Fatalf("model = %q, want base model", next.Model)
	}
}

func TestQueryExpansionRuntimeConfigFromRemoteUsesFullConfig(t *testing.T) {
	t.Parallel()

	base := service.QueryExpansionConfig{
		Enabled: true,
		Model:   "local-model",
		APIKey:  "local-key",
		BaseURL: "https://local.example.com/v1",
	}

	next, ok := queryExpansionRuntimeConfigFromRemote(base, map[string]any{
		"vlm": map[string]any{
			"api_key":  "remote-vlm-key",
			"base_url": "https://remote-vlm.example.com/v1",
		},
		"search": map[string]any{
			"query_expansion": map[string]any{
				"enabled": false,
				"model":   "remote-model",
			},
		},
	})

	if !ok {
		t.Fatal("ok = false, want true")
	}
	if next.Enabled {
		t.Fatal("enabled = true, want false")
	}
	if next.Model != "remote-model" {
		t.Fatalf("model = %q, want remote-model", next.Model)
	}
	if next.APIKey != "remote-vlm-key" {
		t.Fatalf("api key = %q, want VLM fallback key", next.APIKey)
	}
	if next.BaseURL != "https://remote-vlm.example.com/v1" {
		t.Fatalf("base url = %q, want VLM fallback url", next.BaseURL)
	}
}
