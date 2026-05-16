package logger

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

func TestLoadFromEnvReadsLokiConfig(t *testing.T) {
	t.Setenv("ENVIRONMENT", "production")
	t.Setenv("LOKI_ENABLED", "true")
	t.Setenv("LOKI_URL", "https://logs.example.com/loki/api/v1/push")
	t.Setenv("LOKI_USERNAME", "12345")
	t.Setenv("LOKI_PASSWORD", "secret")
	t.Setenv("LOKI_BATCH_SIZE", "25")
	t.Setenv("LOKI_QUEUE_SIZE", "250")
	t.Setenv("LOKI_FLUSH_INTERVAL", "1500ms")
	t.Setenv("LOKI_TIMEOUT", "4s")
	t.Setenv("LOKI_PROJECT", "emomo-test")
	t.Setenv("CLUSTER_NAME", "prod-us")

	cfg := LoadFromEnv()

	if cfg.Environment != "production" {
		t.Fatalf("Environment = %q, want production", cfg.Environment)
	}
	if !cfg.LokiEnabled {
		t.Fatal("LokiEnabled = false, want true")
	}
	if cfg.LokiURL != "https://logs.example.com/loki/api/v1/push" {
		t.Fatalf("LokiURL = %q", cfg.LokiURL)
	}
	if cfg.LokiUsername != "12345" || cfg.LokiPassword != "secret" {
		t.Fatal("Loki credentials were not loaded")
	}
	if cfg.LokiBatchSize != 25 || cfg.LokiQueueSize != 250 {
		t.Fatalf("unexpected Loki sizing: batch=%d queue=%d", cfg.LokiBatchSize, cfg.LokiQueueSize)
	}
	if cfg.LokiFlushInterval != 1500*time.Millisecond || cfg.LokiTimeout != 4*time.Second {
		t.Fatalf("unexpected Loki timings: flush=%s timeout=%s", cfg.LokiFlushInterval, cfg.LokiTimeout)
	}
	if cfg.LokiProject != "emomo-test" || cfg.ClusterName != "prod-us" {
		t.Fatalf("unexpected Loki labels: project=%q cluster=%q", cfg.LokiProject, cfg.ClusterName)
	}
}

func TestLokiHookPushesStructuredLogEntries(t *testing.T) {
	var gotAuthUser, gotAuthPass string
	var gotPayload lokiPushRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		gotAuthUser, gotAuthPass, _ = r.BasicAuth()
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if err := json.Unmarshal(body, &gotPayload); err != nil {
			t.Fatalf("unmarshal payload: %v\n%s", err, body)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	hook := newLokiHook(lokiHookConfig{
		URL:           server.URL,
		Username:      "tenant",
		Password:      "token",
		Project:       "emomo",
		Service:       "emomo-api",
		Environment:   "production",
		Cluster:       "prod-cn",
		BatchSize:     10,
		QueueSize:     10,
		FlushInterval: time.Hour,
		Timeout:       time.Second,
	})
	defer hook.Close()

	log := logrus.New()
	log.SetOutput(io.Discard)
	log.AddHook(hook)

	log.WithFields(logrus.Fields{
		"service":     "emomo-api",
		"component":   "api",
		"request_id":  "req-123",
		"method":      "GET",
		"path":        "/api/v1/search",
		"status":      200,
		"duration_ms": 37,
		"query":       "cat",
	}).Info("Request completed")

	if err := hook.Close(); err != nil {
		t.Fatalf("close hook: %v", err)
	}

	if gotAuthUser != "tenant" || gotAuthPass != "token" {
		t.Fatalf("basic auth = %q/%q, want tenant/token", gotAuthUser, gotAuthPass)
	}
	if len(gotPayload.Streams) != 1 {
		t.Fatalf("streams = %d, want 1", len(gotPayload.Streams))
	}

	stream := gotPayload.Streams[0]
	wantLabels := map[string]string{
		"project":     "emomo",
		"service":     "emomo-api",
		"environment": "production",
		"cluster":     "prod-cn",
		"level":       "info",
		"component":   "api",
	}
	for k, want := range wantLabels {
		if stream.Stream[k] != want {
			t.Fatalf("label %s = %q, want %q", k, stream.Stream[k], want)
		}
	}
	if len(stream.Values) != 1 {
		t.Fatalf("values = %d, want 1", len(stream.Values))
	}
	if len(stream.Values[0]) != 3 {
		t.Fatalf("value tuple length = %d, want timestamp, line, metadata", len(stream.Values[0]))
	}

	var line string
	if err := json.Unmarshal(stream.Values[0][1], &line); err != nil {
		t.Fatalf("unmarshal log line: %v", err)
	}
	var lineFields map[string]any
	if err := json.Unmarshal([]byte(line), &lineFields); err != nil {
		t.Fatalf("unmarshal log line fields: %v", err)
	}
	if lineFields["message"] != "Request completed" || lineFields["request_id"] != "req-123" {
		t.Fatalf("unexpected log line fields: %#v", lineFields)
	}

	var metadata map[string]string
	if err := json.Unmarshal(stream.Values[0][2], &metadata); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}
	wantMetadata := map[string]string{
		"request_id":  "req-123",
		"method":      "GET",
		"path":        "/api/v1/search",
		"status":      "200",
		"duration_ms": "37",
		"query":       "cat",
	}
	for k, want := range wantMetadata {
		if metadata[k] != want {
			t.Fatalf("metadata %s = %q, want %q", k, metadata[k], want)
		}
	}
}
