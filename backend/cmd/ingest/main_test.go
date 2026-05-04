package main

import (
	"strings"
	"testing"

	"github.com/timmy/emomo/internal/config"
)

func TestSelectSourceRejectsStagingSources(t *testing.T) {
	cfg := &config.Config{}
	cfg.Sources.LocalDir.Enabled = true
	cfg.Sources.LocalDir.RootPath = "/tmp/memes"

	_, err := selectSource(cfg, "staging:legacy", "")

	if err == nil {
		t.Fatal("expected staging source to be rejected")
	}
	if !strings.Contains(err.Error(), "unsupported source type") {
		t.Fatalf("expected unsupported source type error, got %v", err)
	}
}

func TestSelectSourceReturnsLocalDirWhenEnabled(t *testing.T) {
	cfg := &config.Config{}
	cfg.Sources.LocalDir.Enabled = true
	cfg.Sources.LocalDir.RootPath = "/tmp/memes"

	src, err := selectSource(cfg, "localdir", "")

	if err != nil {
		t.Fatalf("expected localdir source, got error %v", err)
	}
	if got := src.GetSourceID(); got != "localdir" {
		t.Fatalf("expected source id localdir, got %q", got)
	}
}

func TestSelectSourceUsesLocalDirPathOverride(t *testing.T) {
	cfg := &config.Config{}
	cfg.Sources.LocalDir.Enabled = true
	cfg.Sources.LocalDir.RootPath = "/tmp/memes"

	src, err := selectSource(cfg, "localdir", "/tmp/override")

	if err != nil {
		t.Fatalf("expected localdir source, got error %v", err)
	}
	if got := src.GetDisplayName(); !strings.Contains(got, "/tmp/override") {
		t.Fatalf("expected display name to include override path, got %q", got)
	}
}

func TestSelectSourceRejectsDisabledLocalDir(t *testing.T) {
	cfg := &config.Config{}
	cfg.Sources.LocalDir.Enabled = false
	cfg.Sources.LocalDir.RootPath = "/tmp/memes"

	_, err := selectSource(cfg, "localdir", "")

	if err == nil {
		t.Fatal("expected disabled localdir source to be rejected")
	}
	if !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("expected disabled source error, got %v", err)
	}
}

func TestSelectSourceRejectsChineseBQB(t *testing.T) {
	cfg := &config.Config{}
	cfg.Sources.LocalDir.Enabled = true
	cfg.Sources.LocalDir.RootPath = "/tmp/memes"

	_, err := selectSource(cfg, "chinesebqb", "")

	if err == nil {
		t.Fatal("expected chinesebqb source to be rejected")
	}
	if !strings.Contains(err.Error(), "unsupported source type") {
		t.Fatalf("expected unsupported source error, got %v", err)
	}
}

func TestIsScriptEntrypoint(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want bool
	}{
		{
			name: "script marker present",
			env: map[string]string{
				importEntrypointEnv: importEntrypointValue,
			},
			want: true,
		},
		{
			name: "script marker missing",
			env:  map[string]string{},
			want: false,
		},
		{
			name: "script marker has wrong value",
			env: map[string]string{
				importEntrypointEnv: "direct",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isScriptEntrypoint(func(key string) string {
				return tt.env[key]
			})
			if got != tt.want {
				t.Fatalf("isScriptEntrypoint() = %v, want %v", got, tt.want)
			}
		})
	}
}
