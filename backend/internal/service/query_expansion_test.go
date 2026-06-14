package service

import "testing"

func TestQueryExpansionServiceUpdateConfig(t *testing.T) {
	t.Parallel()

	svc := NewQueryExpansionService(&QueryExpansionConfig{
		Enabled: true,
		Model:   "model-a",
		APIKey:  "key-a",
		BaseURL: "https://api-a.example.com/v1/",
	})

	snapshot := svc.Snapshot()
	if !snapshot.Enabled {
		t.Fatal("enabled = false, want true")
	}
	if snapshot.Model != "model-a" {
		t.Fatalf("model = %q, want model-a", snapshot.Model)
	}
	if snapshot.BaseURL != "https://api-a.example.com/v1" {
		t.Fatalf("base URL = %q, want trimmed URL", snapshot.BaseURL)
	}
	if snapshot.Endpoint != "https://api-a.example.com/v1/chat/completions" {
		t.Fatalf("endpoint = %q", snapshot.Endpoint)
	}

	changed := svc.UpdateConfig(&QueryExpansionConfig{
		Enabled: true,
		Model:   "model-b",
		APIKey:  "key-b",
		BaseURL: "https://api-b.example.com/v1",
	})
	if !changed {
		t.Fatal("UpdateConfig() changed = false, want true")
	}

	snapshot = svc.Snapshot()
	if snapshot.Model != "model-b" {
		t.Fatalf("model = %q, want model-b", snapshot.Model)
	}
	if snapshot.BaseURL != "https://api-b.example.com/v1" {
		t.Fatalf("base URL = %q, want api-b", snapshot.BaseURL)
	}
}

func TestQueryExpansionServiceUpdateConfigNoChange(t *testing.T) {
	t.Parallel()

	cfg := &QueryExpansionConfig{
		Enabled: true,
		Model:   "model-a",
		APIKey:  "key-a",
		BaseURL: "https://api-a.example.com/v1",
	}
	svc := NewQueryExpansionService(cfg)

	if svc.UpdateConfig(cfg) {
		t.Fatal("UpdateConfig() changed = true, want false")
	}
}

func TestQueryExpansionServiceUpdateConfigNilDisables(t *testing.T) {
	t.Parallel()

	svc := NewQueryExpansionService(&QueryExpansionConfig{
		Enabled: true,
		Model:   "model-a",
		APIKey:  "key-a",
		BaseURL: "https://api-a.example.com/v1",
	})

	if !svc.UpdateConfig(nil) {
		t.Fatal("UpdateConfig(nil) changed = false, want true")
	}
	if svc.IsEnabled() {
		t.Fatal("IsEnabled() = true, want false")
	}
}
