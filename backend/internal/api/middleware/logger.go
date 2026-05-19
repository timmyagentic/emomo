package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/timmy/emomo/internal/logger"
)

// LoggerMiddleware returns a Gin middleware that injects a request-scoped logger.
// Parameters:
//   - log: base logger to enrich with request fields.
//
// Returns:
//   - gin.HandlerFunc: middleware handler.
func LoggerMiddleware(log *logger.Logger) gin.HandlerFunc {
	if log == nil {
		log = logger.GetDefault()
	}

	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		// Generate request ID
		requestID := uuid.New().String()

		// Inject tracing fields into context (using standard field constants)
		ctx := c.Request.Context()
		requestLogger := log.WithFields(logger.Fields{
			logger.FieldRequestID: requestID,
			logger.FieldComponent: "api",
			"method":              c.Request.Method,
			"path":                path,
			"query":               query,
			"client_ip":           c.ClientIP(),
		})
		ctx = requestLogger.WithContext(ctx)
		c.Request = c.Request.WithContext(ctx)

		// Also store logger in Gin's context for convenience
		c.Set("logger", requestLogger)

		// Add request ID to response headers
		c.Header("X-Request-ID", requestID)

		// Log request start
		logger.CtxInfo(ctx, "Request started")

		// Process request
		c.Next()

		// Calculate latency
		latency := time.Since(start)
		status := c.Writer.Status()

		// Build full path with query
		fullPath := path
		if query != "" {
			fullPath = path + "?" + query
		}

		// Log request completion with metric fields (using Entry API)
		logger.With(logger.Fields{
			logger.FieldStatus:     status,
			logger.FieldDurationMs: latency.Milliseconds(),
			logger.FieldSize:       c.Writer.Size(),
			"full_path":            fullPath,
		}).Info(ctx, "Request completed")
	}
}

// GetLogger extracts logger from Gin context or request context.
// Parameters:
//   - c: Gin request context.
//
// Returns:
//   - *logger.Logger: request-scoped logger or default logger.
func GetLogger(c *gin.Context) *logger.Logger {
	if l, exists := c.Get("logger"); exists {
		if log, ok := l.(*logger.Logger); ok {
			return log
		}
	}
	return logger.FromContext(c.Request.Context())
}
