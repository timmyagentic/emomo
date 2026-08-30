package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	pb "github.com/timmy/emomo/gen/emomo/v1"
	"github.com/timmy/emomo/internal/config"
	"github.com/timmy/emomo/internal/logger"
	"google.golang.org/protobuf/encoding/protojson"
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

func TestSetupRouterExposesPreviewWithoutEnablingUnconfiguredSubmission(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server.Mode = "test"
	cfg.Server.CORS.AllowAllOrigins = true
	cfg.Server.PublicAPI.BodyLimitBytes = 16 * 1024
	cfg.Feedback.ProductVersion = "test"
	cfg.Feedback.PublicFallbackURL = "https://github.com/timmyagentic/emomo/issues/new"
	router := SetupRouter(nil, cfg, logger.New(&logger.Config{Level: "error", Format: "json", ServiceName: "router-test"}))

	preview := httptest.NewRecorder()
	previewRequest := httptest.NewRequest(http.MethodPost, "/api/v1/feedback/preview", strings.NewReader(`{"description":"建议增加收藏"}`))
	previewRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(preview, previewRequest)
	if preview.Code != http.StatusOK {
		t.Fatalf("preview status=%d body=%s", preview.Code, preview.Body.String())
	}
	var previewBody pb.FeedbackPreviewResponse
	if err := protojson.Unmarshal(preview.Body.Bytes(), &previewBody); err != nil {
		t.Fatal(err)
	}
	if previewBody.SubmissionEnabled || previewBody.PublicFallbackUrl == "" {
		t.Fatalf("unexpected preview availability: %#v", &previewBody)
	}

	submit := httptest.NewRecorder()
	submitRequest := httptest.NewRequest(http.MethodPost, "/api/v1/feedback/submit", strings.NewReader(`{"description":"建议增加收藏","userApproved":true}`))
	submitRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(submit, submitRequest)
	if submit.Code != http.StatusServiceUnavailable {
		t.Fatalf("submit status=%d, want %d", submit.Code, http.StatusServiceUnavailable)
	}
}
