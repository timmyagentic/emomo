package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/timmy/emomo/internal/service"
)

func TestSearchHandlersRejectBlankQuery(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{name: "non-streaming search", path: "/api/v1/search"},
		{name: "streaming search", path: "/api/v1/search/stream"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			embedding := &recordingEmbeddingProvider{}
			router := newSearchTestRouter(embedding)

			req := httptest.NewRequest(http.MethodPost, tt.path, bytes.NewBufferString(`{"query":"   "}`))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
			}
			if calls := embedding.queryCalls.Load(); calls != 0 {
				t.Fatalf("EmbedQuery calls = %d, want 0", calls)
			}
		})
	}
}

func newSearchTestRouter(embedding *recordingEmbeddingProvider) *gin.Engine {
	gin.SetMode(gin.TestMode)
	searchService := service.NewSearchService(
		nil,
		nil,
		nil,
		embedding,
		nil,
		nil,
		nil,
		&service.SearchConfig{},
	)
	handler := NewSearchHandler(searchService)
	router := gin.New()
	router.POST("/api/v1/search", handler.TextSearch)
	router.POST("/api/v1/search/stream", handler.TextSearchStream)
	return router
}

type recordingEmbeddingProvider struct {
	queryCalls atomic.Int32
}

func (p *recordingEmbeddingProvider) Embed(context.Context, string) ([]float32, error) {
	return nil, errUnexpectedEmbeddingCall
}

func (p *recordingEmbeddingProvider) EmbedBatch(context.Context, []string) ([][]float32, error) {
	return nil, errUnexpectedEmbeddingCall
}

func (p *recordingEmbeddingProvider) EmbedQuery(context.Context, string) ([]float32, error) {
	p.queryCalls.Add(1)
	return nil, errUnexpectedEmbeddingCall
}

func (p *recordingEmbeddingProvider) EmbedDocument(context.Context, service.EmbeddingDocument) ([]float32, error) {
	return nil, errUnexpectedEmbeddingCall
}

func (p *recordingEmbeddingProvider) GetModel() string {
	return "test-embedding"
}

func (p *recordingEmbeddingProvider) GetDimensions() int {
	return 1
}

var errUnexpectedEmbeddingCall = &simpleError{msg: "embedding should not be called for invalid query"}
