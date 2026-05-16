package api

import (
	"github.com/gin-gonic/gin"
	"github.com/timmy/emomo/internal/api/handler"
	"github.com/timmy/emomo/internal/api/middleware"
	"github.com/timmy/emomo/internal/config"
	"github.com/timmy/emomo/internal/logger"
	"github.com/timmy/emomo/internal/service"
)

// SetupRouter configures the Gin router with all routes and middleware.
// Parameters:
//   - searchService: search service used by API handlers.
//   - cfg: application configuration for server settings.
//   - log: logger instance for middleware.
//
// Returns:
//   - *gin.Engine: configured Gin router.
func SetupRouter(
	searchService *service.SearchService,
	cfg *config.Config,
	log *logger.Logger,
) *gin.Engine {
	// Set Gin mode
	switch cfg.Server.Mode {
	case "release":
		gin.SetMode(gin.ReleaseMode)
	case "test":
		gin.SetMode(gin.TestMode)
	default:
		gin.SetMode(gin.DebugMode)
	}

	r := gin.New()

	// Add middleware
	r.Use(gin.Recovery())
	r.Use(middleware.LoggerMiddleware(log))
	r.Use(middleware.CORS(middleware.CORSConfig{
		AllowedOrigins:  cfg.Server.CORS.AllowedOrigins,
		AllowAllOrigins: cfg.Server.CORS.AllowAllOrigins,
	}))

	publicLimits := handler.PublicRequestLimits{
		SearchTopKMax:       cfg.Server.PublicAPI.SearchTopKMax,
		SearchQueryMaxRunes: cfg.Server.PublicAPI.SearchQueryMaxRunes,
		ListLimitMax:        cfg.Server.PublicAPI.ListLimitMax,
	}

	// Create handlers
	healthHandler := handler.NewHealthHandler()
	searchHandler := handler.NewSearchHandler(searchService, publicLimits)
	memeHandler := handler.NewMemeHandler(searchService, publicLimits)
	adminHandler := handler.NewAdminHandler(log)

	// Admin page (root)
	r.GET("/", adminHandler.AdminPage)

	// Health check
	r.GET("/health", healthHandler.Health)

	// API v1 routes
	v1 := r.Group("/api/v1")
	v1.Use(middleware.NewPublicGuard(middleware.PublicGuardConfig{
		Enabled:           cfg.Server.PublicAPI.Enabled,
		RateLimitEnabled:  cfg.Server.PublicAPI.RateLimitEnabled,
		RequestsPerMinute: cfg.Server.PublicAPI.RequestsPerMinute,
		Burst:             cfg.Server.PublicAPI.Burst,
		BodyLimitBytes:    cfg.Server.PublicAPI.BodyLimitBytes,
	}).Middleware())
	{
		// Search - register stream route first to avoid matching /search first
		v1.POST("/search/stream", searchHandler.TextSearchStream)
		v1.POST("/search", searchHandler.TextSearch)

		// Categories
		v1.GET("/categories", searchHandler.GetCategories)

		// Memes
		v1.GET("/memes", memeHandler.ListMemes)
		v1.GET("/memes/:id", memeHandler.GetMeme)

		// Stats
		v1.GET("/stats", searchHandler.GetStats)
	}

	return r
}
