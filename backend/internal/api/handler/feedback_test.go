package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	pb "github.com/timmy/emomo/gen/emomo/v1"
	"github.com/timmyagentic/awesome-agent-app-features/feedback"
	feedbackhttp "github.com/timmyagentic/awesome-agent-app-features/feedback/httpclient"
)

type recordingFeedbackSubmitter struct {
	calls   int
	payload []byte
	receipt feedbackhttp.Receipt
	err     error
}

func (submitter *recordingFeedbackSubmitter) Submit(_ context.Context, approved feedback.Approved) (feedbackhttp.Receipt, error) {
	submitter.calls++
	submitter.payload, _ = json.Marshal(approved)
	return submitter.receipt, submitter.err
}

func TestFeedbackPreviewThenExplicitSubmit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	submitter := &recordingFeedbackSubmitter{receipt: feedbackhttp.Receipt{ReferenceURL: "https://github.com/timmyagentic/emomo/issues/123"}}
	handler := NewFeedbackHandler(FeedbackHostConfig{
		Endpoint:          "https://feedback.example.com/v1/feedback",
		PublicFallbackURL: "https://github.com/timmyagentic/emomo/issues/new",
		ProductVersion:    "v1.2.3",
	}, submitter)
	router := gin.New()
	router.POST("/api/v1/feedback/preview", handler.Preview)
	router.POST("/api/v1/feedback/submit", handler.Submit)

	description := "搜索建议，token sk-abcdefghijklmnopqrstuvwxyz0123456789 不应外发"
	previewRecorder := httptest.NewRecorder()
	previewRequest := httptest.NewRequest(http.MethodPost, "/api/v1/feedback/preview", strings.NewReader(`{"description":`+mustJSONString(t, description)+`}`))
	previewRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(previewRecorder, previewRequest)
	if previewRecorder.Code != http.StatusOK {
		t.Fatalf("preview status = %d, body=%s", previewRecorder.Code, previewRecorder.Body.String())
	}
	if submitter.calls != 0 {
		t.Fatalf("preview made %d relay request(s)", submitter.calls)
	}
	var preview pb.FeedbackPreviewResponse
	if err := protojsonUnmarshal.Unmarshal(previewRecorder.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.Preview == nil || preview.Preview.Environment == nil {
		t.Fatalf("incomplete preview: %#v", preview.Preview)
	}
	if preview.Preview.Environment.Product != "emomo" || preview.Preview.Environment.Version != "v1.2.3" || preview.Preview.Environment.Agent != "semantic-search" {
		t.Fatalf("unexpected environment: %#v", preview.Preview.Environment)
	}
	if strings.Contains(preview.Preview.Description, "sk-abcdefghijklmnopqrstuvwxyz0123456789") || !strings.Contains(preview.Preview.Description, "[REDACTED]") {
		t.Fatalf("preview was not redacted: %q", preview.Preview.Description)
	}
	if !preview.SubmissionEnabled || preview.PublicFallbackUrl == "" {
		t.Fatalf("unexpected availability: %#v", &preview)
	}

	cancelRecorder := httptest.NewRecorder()
	cancelRequest := httptest.NewRequest(http.MethodPost, "/api/v1/feedback/submit", strings.NewReader(`{"description":`+mustJSONString(t, description)+`,"user_approved":false}`))
	cancelRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(cancelRecorder, cancelRequest)
	if cancelRecorder.Code != http.StatusBadRequest || submitter.calls != 0 {
		t.Fatalf("unapproved submit status=%d calls=%d", cancelRecorder.Code, submitter.calls)
	}

	submitRecorder := httptest.NewRecorder()
	submitRequest := httptest.NewRequest(http.MethodPost, "/api/v1/feedback/submit", strings.NewReader(`{"description":`+mustJSONString(t, description)+`,"user_approved":true}`))
	submitRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(submitRecorder, submitRequest)
	if submitRecorder.Code != http.StatusOK || submitter.calls != 1 {
		t.Fatalf("submit status=%d calls=%d body=%s", submitRecorder.Code, submitter.calls, submitRecorder.Body.String())
	}
	var wire struct {
		UserApproved bool   `json:"user_approved"`
		Description  string `json:"description"`
	}
	if err := json.Unmarshal(submitter.payload, &wire); err != nil {
		t.Fatal(err)
	}
	if !wire.UserApproved || wire.Description != preview.Preview.Description {
		t.Fatalf("submitted payload differs from preview: %#v vs %q", wire, preview.Preview.Description)
	}
}

func TestFeedbackSubmitUnavailableWithoutRelay(t *testing.T) {
	handler := NewFeedbackHandler(FeedbackHostConfig{PublicFallbackURL: "https://github.com/timmyagentic/emomo/issues/new"})
	router := gin.New()
	router.POST("/api/v1/feedback/preview", handler.Preview)
	router.POST("/api/v1/feedback/submit", handler.Submit)

	previewRecorder := httptest.NewRecorder()
	previewRequest := httptest.NewRequest(http.MethodPost, "/api/v1/feedback/preview", strings.NewReader(`{"description":"建议增加收藏"}`))
	router.ServeHTTP(previewRecorder, previewRequest)
	var preview pb.FeedbackPreviewResponse
	if err := protojsonUnmarshal.Unmarshal(previewRecorder.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.SubmissionEnabled || preview.PublicFallbackUrl == "" {
		t.Fatalf("unexpected preview availability: %#v", &preview)
	}

	submitRecorder := httptest.NewRecorder()
	submitRequest := httptest.NewRequest(http.MethodPost, "/api/v1/feedback/submit", strings.NewReader(`{"description":"建议增加收藏","user_approved":true}`))
	router.ServeHTTP(submitRecorder, submitRequest)
	if submitRecorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("submit status = %d, want 503", submitRecorder.Code)
	}
}

func TestFeedbackRejectsUnsafePublicFallback(t *testing.T) {
	handler := NewFeedbackHandler(FeedbackHostConfig{PublicFallbackURL: "javascript:alert(1)"})
	if handler.config.PublicFallbackURL != "" {
		t.Fatalf("unsafe fallback survived: %q", handler.config.PublicFallbackURL)
	}
}

func mustJSONString(t *testing.T, value string) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}
