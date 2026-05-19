package logger

import (
	"testing"

	"github.com/sirupsen/logrus"
)

func TestNewServiceFromEnvOverridesServiceName(t *testing.T) {
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("LOG_FORMAT", "json")
	t.Setenv("APP_ENV", "local")

	log := NewServiceFromEnv("emomo-worker")

	if got := log.Data["service"]; got != "emomo-worker" {
		t.Fatalf("service field = %#v, want emomo-worker", got)
	}
	if got := log.Entry.Logger.GetLevel(); got != logrus.DebugLevel {
		t.Fatalf("level = %s, want debug", got)
	}
}

func TestLoadFromEnvDoesNotEnableDefaultFileLoggingForEnvironmentOnly(t *testing.T) {
	t.Setenv("APP_ENV", "")
	t.Setenv("ENVIRONMENT", "production")
	t.Setenv("LOG_FILE", "")

	cfg := LoadFromEnv()

	if cfg.Environment != "production" {
		t.Fatalf("Environment = %q, want production", cfg.Environment)
	}
	if cfg.LogFile != "" {
		t.Fatalf("LogFile = %q, want empty unless LOG_FILE is explicitly set", cfg.LogFile)
	}
}
