package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// CORSConfig holds CORS configuration.
type CORSConfig struct {
	AllowedOrigins  []string
	AllowAllOrigins bool
}

// CORS returns middleware that handles Cross-Origin Resource Sharing.
// Parameters:
//   - config: CORS configuration values.
//
// Returns:
//   - gin.HandlerFunc: middleware handler.
func CORS(config CORSConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")

		// Determine allowed origin
		var allowedOrigin string
		if config.AllowAllOrigins {
			allowedOrigin = "*"
			// When using *, credentials must be false
			c.Writer.Header().Set("Access-Control-Allow-Credentials", "false")
		} else {
			// Check if origin is in allowed list
			allowed := false
			for _, allowedOriginItem := range config.AllowedOrigins {
				if origin == allowedOriginItem || allowedOriginItem == "*" {
					allowed = true
					allowedOrigin = origin
					break
				}
			}

			if !allowed && len(config.AllowedOrigins) > 0 {
				// Origin not allowed, don't set CORS headers
				c.Next()
				return
			}

			// If no origins configured or origin matches, allow it
			if allowedOrigin == "" {
				allowedOrigin = origin
			}
			c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		}

		c.Writer.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")
		c.Writer.Header().Set("Access-Control-Expose-Headers", "Content-Length")

		// Handle preflight requests
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

// IsOriginAllowed checks if an origin is allowed based on the configuration.
// Parameters:
//   - origin: origin header value to check.
//   - config: CORS configuration values.
//
// Returns:
//   - bool: true if the origin is allowed.
func IsOriginAllowed(origin string, config CORSConfig) bool {
	if config.AllowAllOrigins {
		return true
	}

	for _, allowedOrigin := range config.AllowedOrigins {
		if allowedOrigin == "*" || strings.EqualFold(origin, allowedOrigin) {
			return true
		}
	}

	return false
}
