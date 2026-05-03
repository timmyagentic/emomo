package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	pb "github.com/timmy/emomo/gen/emomo/v1"
	"github.com/timmy/emomo/internal/service"
)

// MemeHandler handles meme-related endpoints.
type MemeHandler struct {
	searchService *service.SearchService
}

// NewMemeHandler creates a new meme handler.
func NewMemeHandler(searchService *service.SearchService) *MemeHandler {
	return &MemeHandler{
		searchService: searchService,
	}
}

// ListMemes handles GET /api/v1/memes — request fields come from the query
// string (?category=&limit=&offset=).
func (h *MemeHandler) ListMemes(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	req := &pb.ListMemesRequest{
		Category: c.Query("category"),
		Limit:    int32(limit),
		Offset:   int32(offset),
	}

	resp, err := h.searchService.ListMemes(c.Request.Context(), req)
	if err != nil {
		writeError(c, http.StatusInternalServerError, err)
		return
	}
	writeProtoJSON(c, http.StatusOK, resp)
}

// GetMeme handles GET /api/v1/memes/:id — request fields come from the URL
// path parameter.
func (h *MemeHandler) GetMeme(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		writeError(c, http.StatusBadRequest, errMemeIDRequired)
		return
	}

	resp, err := h.searchService.GetMeme(c.Request.Context(), &pb.GetMemeRequest{Id: id})
	if err != nil {
		writeError(c, http.StatusNotFound, err)
		return
	}
	writeProtoJSON(c, http.StatusOK, resp)
}

var errMemeIDRequired = &simpleError{msg: "meme id is required"}

type simpleError struct{ msg string }

func (e *simpleError) Error() string { return e.msg }
