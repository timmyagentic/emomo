package middleware

import (
	"math"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/timmy/emomo/internal/logger"
)

const defaultRateLimitBucketTTL = 10 * time.Minute

// PublicGuardConfig controls public API request limits.
type PublicGuardConfig struct {
	Enabled           bool
	RateLimitEnabled  bool
	RequestsPerMinute int
	Burst             int
	BodyLimitBytes    int64
	BucketTTL         time.Duration

	now func() time.Time
}

// PublicGuard applies body-size and per-client route rate limits.
type PublicGuard struct {
	cfg     PublicGuardConfig
	mu      sync.Mutex
	buckets map[string]*rateBucket
}

type rateBucket struct {
	tokens float64
	last   time.Time
}

// NewPublicGuard creates a public API guard with normalized defaults.
func NewPublicGuard(cfg PublicGuardConfig) *PublicGuard {
	if cfg.now == nil {
		cfg.now = time.Now
	}
	if cfg.RequestsPerMinute <= 0 {
		cfg.RequestsPerMinute = 60
	}
	if cfg.Burst <= 0 {
		cfg.Burst = cfg.RequestsPerMinute
	}
	if cfg.BucketTTL <= 0 {
		cfg.BucketTTL = defaultRateLimitBucketTTL
	}
	return &PublicGuard{
		cfg:     cfg,
		buckets: make(map[string]*rateBucket),
	}
}

// Middleware returns the Gin middleware for public API protection.
func (g *PublicGuard) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if g == nil || !g.cfg.Enabled {
			c.Next()
			return
		}

		if g.cfg.BodyLimitBytes > 0 {
			if c.Request.ContentLength > g.cfg.BodyLimitBytes {
				logPublicGuardReject(c, "body_limit")
				c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{"error": "request body too large"})
				return
			}
			if c.Request.Body != nil {
				c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, g.cfg.BodyLimitBytes)
			}
		}

		if g.cfg.RateLimitEnabled && !g.allow(c) {
			logPublicGuardReject(c, "rate_limit")
			c.Header("Retry-After", strconv.Itoa(retryAfterSeconds(g.cfg.RequestsPerMinute)))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "rate limit exceeded"})
			return
		}

		c.Next()
	}
}

func logPublicGuardReject(c *gin.Context, reason string) {
	logger.With(logger.Fields{
		"public_guard_reason": reason,
		"client_ip":           remoteClientIP(c),
		"method":              c.Request.Method,
		"path":                c.Request.URL.Path,
	}).Warn(c.Request.Context(), "Public API request rejected")
}

func (g *PublicGuard) allow(c *gin.Context) bool {
	key := g.rateLimitKey(c)
	now := g.cfg.now()

	g.mu.Lock()
	defer g.mu.Unlock()

	g.pruneBuckets(now)

	bucket := g.buckets[key]
	if bucket == nil {
		g.buckets[key] = &rateBucket{
			tokens: float64(g.cfg.Burst - 1),
			last:   now,
		}
		return true
	}

	elapsed := now.Sub(bucket.last).Seconds()
	if elapsed > 0 {
		refillPerSecond := float64(g.cfg.RequestsPerMinute) / 60.0
		bucket.tokens = math.Min(float64(g.cfg.Burst), bucket.tokens+(elapsed*refillPerSecond))
		bucket.last = now
	}
	if bucket.tokens < 1 {
		return false
	}
	bucket.tokens--
	return true
}

func (g *PublicGuard) pruneBuckets(now time.Time) {
	for key, bucket := range g.buckets {
		if now.Sub(bucket.last) > g.cfg.BucketTTL {
			delete(g.buckets, key)
		}
	}
}

func (g *PublicGuard) rateLimitKey(c *gin.Context) string {
	clientIP := remoteClientIP(c)
	route := c.FullPath()
	if route == "" {
		route = c.Request.URL.Path
	}
	return clientIP + " " + c.Request.Method + " " + route
}

func retryAfterSeconds(requestsPerMinute int) int {
	if requestsPerMinute <= 0 {
		return 60
	}
	seconds := int(math.Ceil(60.0 / float64(requestsPerMinute)))
	if seconds < 1 {
		return 1
	}
	return seconds
}

func remoteClientIP(c *gin.Context) string {
	host, _, err := net.SplitHostPort(c.Request.RemoteAddr)
	if err == nil {
		return host
	}
	return c.Request.RemoteAddr
}
