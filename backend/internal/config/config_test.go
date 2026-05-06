package config

import (
	"os"
	"path/filepath"
	"testing"
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
}

func TestConfigDefaultSearchProfileUsesExplicitDefault(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Search: SearchConfig{
			DefaultProfile: "qwen3vl",
			Profiles: []SearchProfileConfig{
				{Name: "legacy", ImageEmbedding: "jina", CaptionEmbedding: "jina"},
				{Name: "qwen3vl", ImageEmbedding: "qwen3vl_image", CaptionEmbedding: "qwen3vl_caption", IsDefault: true},
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
				{Name: "qwen3vl", ImageEmbedding: "qwen3vl_image", CaptionEmbedding: "qwen3vl_caption"},
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
	if profile.CaptionEmbedding != "qwen3vl_caption" {
		t.Fatalf("caption embedding = %q, want qwen3vl_caption", profile.CaptionEmbedding)
	}
}
