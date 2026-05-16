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
	limits        PublicRequestLimits
}

// NewMemeHandler creates a new meme handler.
func NewMemeHandler(searchService *service.SearchService, limits ...PublicRequestLimits) *MemeHandler {
	requestLimits := PublicRequestLimits{}
	if len(limits) > 0 {
		requestLimits = limits[0]
	}
	return &MemeHandler{
		searchService: searchService,
		limits:        normalizePublicRequestLimits(requestLimits),
	}
}

// ListMemes handles GET /api/v1/memes — request fields come from the query
// string (?category=&limit=&offset=).
func (h *MemeHandler) ListMemes(c *gin.Context) {
	req := normalizeListMemesParams(c.Query("limit"), c.Query("offset"), c.Query("category"), h.limits)

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

func normalizeListMemesParams(limitRaw, offsetRaw, category string, limits publicRequestLimits) *pb.ListMemesRequest {
	limits = normalizePublicRequestLimits(limits)

	limit := 20
	if limitRaw != "" {
		parsed, err := strconv.Atoi(limitRaw)
		if err == nil {
			limit = parsed
		}
	}
	if limit <= 0 {
		limit = 20
	}
	if int32(limit) > limits.ListLimitMax {
		limit = int(limits.ListLimitMax)
	}

	offset := 0
	if offsetRaw != "" {
		parsed, err := strconv.Atoi(offsetRaw)
		if err == nil {
			offset = parsed
		}
	}
	if offset < 0 {
		offset = 0
	}

	return &pb.ListMemesRequest{
		Category: category,
		Limit:    int32(limit),
		Offset:   int32(offset),
	}
}
