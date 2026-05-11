package middleware

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/timmy/emomo/internal/logger"
)

func TestLoggerMiddlewareWritesStructuredRequestFields(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var buf bytes.Buffer
	log := logger.New(&logger.Config{
		Level:       "info",
		Format:      "json",
		Output:      &buf,
		ServiceName: "middleware-test",
	})

	router := gin.New()
	router.Use(LoggerMiddleware(log))
	router.GET("/ping", func(c *gin.Context) {
		c.String(http.StatusTeapot, "pong")
	})

	req := httptest.NewRequest(http.MethodGet, "/ping?x=1", nil)
	req.RemoteAddr = "192.0.2.10:1234"
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if got := rec.Header().Get("X-Request-ID"); got == "" {
		t.Fatal("X-Request-ID header is empty")
	}

	entries := decodeLogEntries(t, buf.Bytes())
	completed := findLogEntry(t, entries, "Request completed")

	assertLogField(t, completed, "component", "api")
	assertLogField(t, completed, "method", http.MethodGet)
	assertLogField(t, completed, "path", "/ping")
	assertLogField(t, completed, "query", "x=1")
	assertLogField(t, completed, "client_ip", "192.0.2.10")
	assertLogField(t, completed, "status", float64(http.StatusTeapot))
	if _, ok := completed["duration_ms"]; !ok {
		t.Fatal("completed log is missing duration_ms")
	}
	if _, ok := completed["size"]; !ok {
		t.Fatal("completed log is missing size")
	}
	if _, ok := completed["request_id"]; !ok {
		t.Fatal("completed log is missing request_id")
	}
}

func decodeLogEntries(t *testing.T, raw []byte) []map[string]interface{} {
	t.Helper()

	lines := bytes.Split(bytes.TrimSpace(raw), []byte("\n"))
	entries := make([]map[string]interface{}, 0, len(lines))
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		var entry map[string]interface{}
		if err := json.Unmarshal(line, &entry); err != nil {
			t.Fatalf("failed to decode log line %q: %v", string(line), err)
		}
		entries = append(entries, entry)
	}
	return entries
}

func findLogEntry(t *testing.T, entries []map[string]interface{}, message string) map[string]interface{} {
	t.Helper()

	for _, entry := range entries {
		if entry["message"] == message {
			return entry
		}
	}
	t.Fatalf("log entry %q not found in %#v", message, entries)
	return nil
}

func assertLogField(t *testing.T, entry map[string]interface{}, key string, want interface{}) {
	t.Helper()

	if got := entry[key]; got != want {
		t.Fatalf("log field %s = %#v, want %#v", key, got, want)
	}
}
