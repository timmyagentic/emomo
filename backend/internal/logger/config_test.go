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
