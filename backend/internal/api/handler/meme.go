package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/timmy/emomo/internal/service"
)

// MemeHandler handles meme-related endpoints.
type MemeHandler struct {
	searchService *service.SearchService
}

// NewMemeHandler creates a new meme handler.
// Parameters:
//   - searchService: search service instance.
//
// Returns:
//   - *MemeHandler: initialized handler.
func NewMemeHandler(searchService *service.SearchService) *MemeHandler {
	return &MemeHandler{
		searchService: searchService,
	}
}

// ListMemes handles GET /api/v1/memes.
// Parameters:
//   - c: Gin request context.
//
// Returns: none (writes JSON response).
func (h *MemeHandler) ListMemes(c *gin.Context) {
	category := c.Query("category")

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	result, err := h.searchService.ListMemes(c.Request.Context(), category, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to list memes: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetMeme handles GET /api/v1/memes/:id.
// Parameters:
//   - c: Gin request context.
//
// Returns: none (writes JSON response).
func (h *MemeHandler) GetMeme(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Meme ID is required",
		})
		return
	}

	meme, err := h.searchService.GetMemeByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Meme not found",
		})
		return
	}

	c.JSON(http.StatusOK, meme)
}
