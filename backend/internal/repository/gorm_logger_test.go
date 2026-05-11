package repository

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/timmy/emomo/internal/logger"
	gormlogger "gorm.io/gorm/logger"
)

func TestStructuredGormLoggerWritesComponentField(t *testing.T) {
	var buf bytes.Buffer
	log := logger.New(&logger.Config{
		Level:       "info",
		Format:      "json",
		Output:      &buf,
		ServiceName: "gorm-test",
	})
	previous := logger.GetDefault()
	logger.SetDefaultLogger(log)
	t.Cleanup(func() {
		logger.SetDefaultLogger(previous)
	})

	gormLog := newGormLogger(gormlogger.Warn)
	gormLog.Warn(context.Background(), "slow query: %s", "select 1")

	var entry map[string]interface{}
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &entry); err != nil {
		t.Fatalf("failed to decode log output %q: %v", buf.String(), err)
	}
	if got := entry[logger.FieldComponent]; got != "db" {
		t.Fatalf("component = %#v, want db", got)
	}
	message, _ := entry["message"].(string)
	if !strings.Contains(message, "slow query: select 1") {
		t.Fatalf("message = %q, want slow query details", message)
	}
}
