package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestPublicGuardRejectsOversizedBodies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	guard := NewPublicGuard(PublicGuardConfig{
		Enabled:        true,
		BodyLimitBytes: 4,
	})
	called := false
	router.POST("/api/v1/search", guard.Middleware(), func(c *gin.Context) {
		called = true
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/search", strings.NewReader(`{"query":"too large"}`))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusRequestEntityTooLarge)
	}
	if called {
		t.Fatal("handler was called for oversized body")
	}
}

func TestPublicGuardRateLimitsByClientAndRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	guard := NewPublicGuard(PublicGuardConfig{
		Enabled:           true,
		RateLimitEnabled:  true,
		RequestsPerMinute: 1,
		Burst:             2,
		now: func() time.Time {
			return time.Unix(1000, 0)
		},
	})
	router.GET("/api/v1/memes", guard.Middleware(), func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/memes", nil)
		req.RemoteAddr = "203.0.113.10:1234"
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d status = %d, want %d", i+1, rec.Code, http.StatusOK)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/memes", nil)
	req.RemoteAddr = "203.0.113.10:1234"
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("third request status = %d, want %d", rec.Code, http.StatusTooManyRequests)
	}
	if retryAfter := rec.Header().Get("Retry-After"); retryAfter == "" {
		t.Fatal("Retry-After header is empty")
	}
}

func TestPublicGuardRateLimitIgnoresSpoofedForwardedFor(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	guard := NewPublicGuard(PublicGuardConfig{
		Enabled:           true,
		RateLimitEnabled:  true,
		RequestsPerMinute: 1,
		Burst:             1,
		now: func() time.Time {
			return time.Unix(1000, 0)
		},
	})
	router.GET("/api/v1/memes", guard.Middleware(), func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	for i, forwardedFor := range []string{"198.51.100.1", "198.51.100.2"} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/memes", nil)
		req.RemoteAddr = "203.0.113.10:1234"
		req.Header.Set("X-Forwarded-For", forwardedFor)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if i == 0 && rec.Code != http.StatusOK {
			t.Fatalf("first request status = %d, want %d", rec.Code, http.StatusOK)
		}
		if i == 1 && rec.Code != http.StatusTooManyRequests {
			t.Fatalf("spoofed forwarded-for request status = %d, want %d", rec.Code, http.StatusTooManyRequests)
		}
	}
}

func TestPublicGuardPrunesIdleRateBuckets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Unix(1000, 0)
	guard := NewPublicGuard(PublicGuardConfig{
		Enabled:           true,
		RateLimitEnabled:  true,
		RequestsPerMinute: 60,
		Burst:             1,
		now: func() time.Time {
			return now
		},
	})

	router := gin.New()
	router.GET("/api/v1/memes", guard.Middleware(), func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/memes", nil)
	req.RemoteAddr = "203.0.113.10:1234"
	router.ServeHTTP(httptest.NewRecorder(), req)

	if len(guard.buckets) != 1 {
		t.Fatalf("bucket count = %d, want 1", len(guard.buckets))
	}

	now = now.Add(11 * time.Minute)
	req = httptest.NewRequest(http.MethodGet, "/api/v1/memes", nil)
	req.RemoteAddr = "203.0.113.11:1234"
	router.ServeHTTP(httptest.NewRecorder(), req)

	if len(guard.buckets) != 1 {
		t.Fatalf("bucket count after pruning = %d, want 1", len(guard.buckets))
	}
}
