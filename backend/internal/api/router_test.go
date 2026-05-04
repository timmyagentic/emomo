package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/timmy/emomo/internal/config"
	"github.com/timmy/emomo/internal/logger"
)

func TestSetupRouterDoesNotExposeIngestRoutes(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server.Mode = "test"
	cfg.Server.CORS.AllowAllOrigins = true

	router := SetupRouter(nil, cfg, logger.New(&logger.Config{
		Level:       "error",
		Format:      "json",
		ServiceName: "router-test",
	}))

	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{
			name:   "trigger ingest",
			method: http.MethodPost,
			path:   "/api/v1/ingest",
			body:   `{"source":"localdir","limit":1}`,
		},
		{
			name:   "ingest status",
			method: http.MethodGet,
			path:   "/api/v1/ingest/status",
			body:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			if tt.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			resp := httptest.NewRecorder()

			router.ServeHTTP(resp, req)

			if resp.Code != http.StatusNotFound {
				t.Fatalf("%s %s status = %d, want %d", tt.method, tt.path, resp.Code, http.StatusNotFound)
			}
		})
	}
}
