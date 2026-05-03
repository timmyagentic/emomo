package handler

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	pb "github.com/timmy/emomo/gen/emomo/v1"
	"github.com/timmy/emomo/internal/service"
)

// SearchHandler handles search-related endpoints.
type SearchHandler struct {
	searchService *service.SearchService
}

// NewSearchHandler creates a new search handler.
func NewSearchHandler(searchService *service.SearchService) *SearchHandler {
	return &SearchHandler{
		searchService: searchService,
	}
}

// TextSearch handles POST /api/v1/search.
func (h *SearchHandler) TextSearch(c *gin.Context) {
	req := &pb.SearchRequest{}
	if err := readProtoJSON(c, req); err != nil {
		writeError(c, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
		return
	}
	overrideQueryParams(c, req)

	resp, err := h.searchService.TextSearch(c.Request.Context(), req)
	if err != nil {
		writeError(c, http.StatusInternalServerError, fmt.Errorf("search failed: %w", err))
		return
	}
	writeProtoJSON(c, http.StatusOK, resp)
}

// GetCategories handles GET /api/v1/categories.
func (h *SearchHandler) GetCategories(c *gin.Context) {
	categories, err := h.searchService.GetCategories(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, fmt.Errorf("failed to get categories: %w", err))
		return
	}
	writeProtoJSON(c, http.StatusOK, &pb.GetCategoriesResponse{
		Categories: categories,
		Total:      int32(len(categories)),
	})
}

// GetStats handles GET /api/v1/stats.
func (h *SearchHandler) GetStats(c *gin.Context) {
	resp, err := h.searchService.GetStats(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, fmt.Errorf("failed to get stats: %w", err))
		return
	}
	writeProtoJSON(c, http.StatusOK, resp)
}

// TextSearchStream handles POST /api/v1/search/stream with SSE. Each event is
// a single `data:` line carrying a protojson-encoded SearchProgressEvent.
// The `event:` line carries the lowercase stage label for human debugging
// only; clients should route on the `stage` field of the JSON payload.
func (h *SearchHandler) TextSearchStream(c *gin.Context) {
	req := &pb.SearchRequest{}
	if err := readProtoJSON(c, req); err != nil {
		writeError(c, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
		return
	}
	overrideQueryParams(c, req)

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	ctx := c.Request.Context()
	progressCh := make(chan *pb.SearchProgressEvent, 100)

	var (
		searchResult *pb.SearchResponse
		searchErr    error
	)
	done := make(chan struct{})

	go func() {
		defer close(done)
		searchResult, searchErr = h.searchService.TextSearchWithProgress(ctx, req, progressCh)
	}()

	w := c.Writer

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-progressCh:
			if !ok {
				<-done
				if searchErr != nil {
					writeSSEEvent(c, &pb.SearchProgressEvent{
						Stage: pb.SearchStage_SEARCH_STAGE_ERROR,
						Payload: &pb.SearchProgressEvent_Error{
							Error: &pb.SearchError{Error: searchErr.Error()},
						},
					})
				} else if searchResult != nil {
					writeSSEEvent(c, &pb.SearchProgressEvent{
						Stage: pb.SearchStage_SEARCH_STAGE_COMPLETE,
						Payload: &pb.SearchProgressEvent_Complete{
							Complete: &pb.SearchComplete{
								Results:       searchResult.GetResults(),
								Total:         searchResult.GetTotal(),
								Query:         searchResult.GetQuery(),
								ExpandedQuery: searchResult.GetExpandedQuery(),
								Collection:    searchResult.GetCollection(),
								Profile:       searchResult.GetProfile(),
							},
						},
					})
				}
				w.Flush()
				return
			}
			writeSSEEvent(c, event)
			w.Flush()
		}
	}
}

// writeSSEEvent writes a SearchProgressEvent as a single SSE event. The
// `event:` line carries the lowercase stage label for debugging; the `data:`
// line carries the protojson body.
func writeSSEEvent(c *gin.Context, event *pb.SearchProgressEvent) {
	body, err := protojsonMarshal.Marshal(event)
	if err != nil {
		fmt.Fprintf(c.Writer, "event: error\ndata: {\"stage\":8,\"payload\":{\"error\":{\"error\":%q}}}\n\n", err.Error())
		return
	}
	fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", sseStageName(event.GetStage()), body)
}

// sseStageName converts a SearchStage enum to a backwards-compatible
// lowercase short name for the SSE `event:` line. The frontend used to
// receive these names (event: thinking, event: complete, ...) and falls back
// to them when the JSON body lacks a `stage` field; we preserve them so the
// wire-level shape of the SSE protocol stays stable.
func sseStageName(stage pb.SearchStage) string {
	name := pb.SearchStage_name[int32(stage)]
	name = strings.TrimPrefix(name, "SEARCH_STAGE_")
	if name == "" || name == "UNSPECIFIED" {
		return "progress"
	}
	return strings.ToLower(name)
}

// overrideQueryParams lets callers override `collection` and `profile` via
// query string when the JSON body did not specify them. This preserves the
// pre-refactor behaviour of accepting both POST body fields and ?collection=
// / ?profile= query params.
func overrideQueryParams(c *gin.Context, req *pb.SearchRequest) {
	if collection := c.Query("collection"); collection != "" && req.GetCollection() == "" {
		req.Collection = collection
	}
	if profile := c.Query("profile"); profile != "" && req.GetProfile() == "" {
		req.Profile = profile
	}
}
