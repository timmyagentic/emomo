package service

import (
	"context"
	"fmt"
	"sort"

	"github.com/timmy/emomo/internal/domain"
	"github.com/timmy/emomo/internal/logger"
	"github.com/timmy/emomo/internal/repository"
	"github.com/timmy/emomo/internal/storage"
)

// SearchConfig holds configuration for search service.
type SearchConfig struct {
	ScoreThreshold    float32
	DefaultCollection string // Default search collection key (embedding config name)
	DefaultProfile    string
	Retrieval         RetrievalConfig
}

// CollectionConfig holds configuration for a single collection.
type CollectionConfig struct {
	QdrantRepo *repository.QdrantRepository
	Embedding  EmbeddingProvider
}

// RetrievalConfig controls multi-route search limits and weights.
type RetrievalConfig struct {
	ImageTopK   int
	CaptionTopK int
	FinalTopK   int
	Weights     RetrievalWeights
}

// RetrievalWeights controls weighted rank fusion across search routes.
type RetrievalWeights struct {
	Image   float32
	Caption float32
	Keyword float32
}

// SearchProfileConfig binds image and caption vector collections into one profile.
type SearchProfileConfig struct {
	Image   *CollectionConfig
	Caption *CollectionConfig
}

// SearchService handles meme search operations.
type SearchService struct {
	memeRepo           *repository.MemeRepository
	memeAnnotationRepo *repository.MemeAnnotationRepository
	defaultQdrantRepo  *repository.QdrantRepository
	defaultEmbedding   EmbeddingProvider
	queryExpansion     *QueryExpansionService
	storage            storage.ObjectStorage
	logger             *logger.Logger
	scoreThreshold     float32
	defaultCollection  string
	defaultProfile     string
	retrieval          RetrievalConfig

	// Multi-collection support: collection name -> config
	collections map[string]*CollectionConfig
	profiles    map[string]*SearchProfileConfig
}

// NewSearchService creates a new search service.
// Parameters:
//   - memeRepo: repository for meme records.
//   - memeAnnotationRepo: repository for meme annotations (metadata access).
//   - qdrantRepo: default Qdrant repository.
//   - embedding: default embedding provider.
//   - queryExpansion: optional query expansion service.
//   - objectStorage: object storage client for URL generation.
//   - log: logger instance.
//   - cfg: search configuration settings.
//
// Returns:
//   - *SearchService: initialized search service.
func NewSearchService(
	memeRepo *repository.MemeRepository,
	memeAnnotationRepo *repository.MemeAnnotationRepository,
	qdrantRepo *repository.QdrantRepository,
	embedding EmbeddingProvider,
	queryExpansion *QueryExpansionService,
	objectStorage storage.ObjectStorage,
	log *logger.Logger,
	cfg *SearchConfig,
) *SearchService {
	var threshold float32
	var defaultCollection string
	var defaultProfile string
	retrieval := defaultRetrievalConfig()
	if cfg != nil {
		threshold = cfg.ScoreThreshold
		defaultCollection = cfg.DefaultCollection
		defaultProfile = cfg.DefaultProfile
		retrieval = normalizeRetrievalConfig(cfg.Retrieval)
	}
	return &SearchService{
		memeRepo:           memeRepo,
		memeAnnotationRepo: memeAnnotationRepo,
		defaultQdrantRepo:  qdrantRepo,
		defaultEmbedding:   embedding,
		queryExpansion:     queryExpansion,
		storage:            objectStorage,
		logger:             log,
		scoreThreshold:     threshold,
		defaultCollection:  defaultCollection,
		defaultProfile:     defaultProfile,
		retrieval:          retrieval,
		collections:        make(map[string]*CollectionConfig),
		profiles:           make(map[string]*SearchProfileConfig),
	}
}

// RegisterCollection registers a collection configuration for multi-collection search.
// Parameters:
//   - name: collection name key.
//   - qdrantRepo: Qdrant repository for the collection.
//   - embedding: embedding provider for the collection.
//
// Returns: none.
func (s *SearchService) RegisterCollection(name string, qdrantRepo *repository.QdrantRepository, embedding EmbeddingProvider) {
	s.collections[name] = &CollectionConfig{
		QdrantRepo: qdrantRepo,
		Embedding:  embedding,
	}
}

// RegisterProfile registers a multi-route search profile.
func (s *SearchService) RegisterProfile(
	name string,
	imageRepo *repository.QdrantRepository,
	imageEmbedding EmbeddingProvider,
	captionRepo *repository.QdrantRepository,
	captionEmbedding EmbeddingProvider,
) {
	s.profiles[name] = &SearchProfileConfig{
		Image: &CollectionConfig{
			QdrantRepo: imageRepo,
			Embedding:  imageEmbedding,
		},
		Caption: &CollectionConfig{
			QdrantRepo: captionRepo,
			Embedding:  captionEmbedding,
		},
	}
}

// GetAvailableCollections returns the list of available collection keys.
// Parameters: none.
// Returns:
//   - []string: collection keys including default and registered ones.
func (s *SearchService) GetAvailableCollections() []string {
	collections := make([]string, 0, len(s.collections)+1)
	if s.defaultCollection != "" {
		collections = append(collections, s.defaultCollection)
	}

	remaining := make([]string, 0, len(s.collections))
	for name := range s.collections {
		if name != s.defaultCollection {
			remaining = append(remaining, name)
		}
	}

	sort.Strings(remaining)
	collections = append(collections, remaining...)

	return collections
}

// GetAvailableProfiles returns the list of available search profile keys.
func (s *SearchService) GetAvailableProfiles() []string {
	profiles := make([]string, 0, len(s.profiles)+1)
	if s.defaultProfile != "" {
		profiles = append(profiles, s.defaultProfile)
	}

	remaining := make([]string, 0, len(s.profiles))
	for name := range s.profiles {
		if name != s.defaultProfile {
			remaining = append(remaining, name)
		}
	}

	sort.Strings(remaining)
	profiles = append(profiles, remaining...)

	return profiles
}

func (s *SearchService) resolveCollection(name string) (*repository.QdrantRepository, EmbeddingProvider, string, error) {
	if name == "" {
		return s.defaultQdrantRepo, s.defaultEmbedding, s.defaultCollection, nil
	}

	cfg, ok := s.collections[name]
	if !ok {
		return nil, nil, "", fmt.Errorf("unknown collection: %s", name)
	}

	return cfg.QdrantRepo, cfg.Embedding, name, nil
}

func (s *SearchService) resolveProfile(name string) (*SearchProfileConfig, string, bool) {
	if name == "" {
		name = s.defaultProfile
	}
	if name == "" {
		return nil, "", false
	}
	cfg, ok := s.profiles[name]
	return cfg, name, ok
}

func defaultRetrievalConfig() RetrievalConfig {
	return RetrievalConfig{
		ImageTopK:   100,
		CaptionTopK: 100,
		FinalTopK:   20,
		Weights: RetrievalWeights{
			Image:   0.60,
			Caption: 0.30,
			Keyword: 0.10,
		},
	}
}

func normalizeRetrievalConfig(cfg RetrievalConfig) RetrievalConfig {
	defaults := defaultRetrievalConfig()
	if cfg.ImageTopK <= 0 {
		cfg.ImageTopK = defaults.ImageTopK
	}
	if cfg.CaptionTopK <= 0 {
		cfg.CaptionTopK = defaults.CaptionTopK
	}
	if cfg.FinalTopK <= 0 {
		cfg.FinalTopK = defaults.FinalTopK
	}
	if cfg.Weights.Image == 0 && cfg.Weights.Caption == 0 && cfg.Weights.Keyword == 0 {
		cfg.Weights = defaults.Weights
	}
	return cfg
}

func (s *SearchService) resolveRequestedProfile(req *SearchRequest) (*SearchProfileConfig, string, bool, error) {
	requested := req.Profile
	if requested == "" {
		requested = req.Collection
	}
	if requested == "" {
		requested = s.defaultProfile
	}
	if requested == "" {
		return nil, "", false, nil
	}
	profile, name, ok := s.resolveProfile(requested)
	if ok {
		return profile, name, true, nil
	}
	if req.Profile != "" {
		return nil, "", false, fmt.Errorf("unknown profile: %s", req.Profile)
	}
	return nil, "", false, nil
}

// log returns a logger from context if available, otherwise returns the default logger
func (s *SearchService) log(ctx context.Context) *logger.Logger {
	if l := logger.FromContext(ctx); l != nil {
		return l
	}
	return s.logger
}

// SearchRequest represents a text search request.
type SearchRequest struct {
	Query        string  `json:"query" binding:"required"`
	TopK         int     `json:"top_k"`
	Category     *string `json:"category,omitempty"`
	TextPresence *string `json:"text_presence,omitempty"`
	Collection   string  `json:"collection,omitempty"` // Optional: specify which collection to search
	Profile      string  `json:"profile,omitempty"`    // Optional: specify multi-route search profile
}

// SearchResult represents a single search result.
type SearchResult struct {
	ID           string           `json:"id"`
	URL          string           `json:"url"`
	Score        float32          `json:"score"`
	Description  string           `json:"description"`
	Category     string           `json:"category"`
	Tags         []string         `json:"tags"`
	TextPresence string           `json:"text_presence,omitempty"`
	ImageInfo    domain.ImageInfo `json:"image_info,omitempty"`
}

// SearchResponse represents the search response.
type SearchResponse struct {
	Results       []SearchResult `json:"results"`
	Total         int            `json:"total"`
	Query         string         `json:"query"`
	ExpandedQuery string         `json:"expanded_query,omitempty"`
	Collection    string         `json:"collection,omitempty"` // Which collection was searched
	Profile       string         `json:"profile,omitempty"`    // Which profile was searched
}

// SearchProgress represents a progress update during streaming search.
type SearchProgress struct {
	Stage         string `json:"stage"`                    // Current stage of the search
	Message       string `json:"message,omitempty"`        // User-friendly message
	ThinkingText  string `json:"thinking_text,omitempty"`  // LLM thinking content (for streaming)
	IsDelta       bool   `json:"is_delta,omitempty"`       // Whether thinking_text is incremental
	ExpandedQuery string `json:"expanded_query,omitempty"` // Expanded query (when available)
}

// TextSearch performs a hybrid text search (dense + BM25).
// Parameters:
//   - ctx: context for cancellation and deadlines.
//   - req: search request parameters.
//
// Returns:
//   - *SearchResponse: search results and metadata.
//   - error: non-nil if search fails.
func (s *SearchService) TextSearch(ctx context.Context, req *SearchRequest) (*SearchResponse, error) {
	// Set defaults
	if req.TopK <= 0 {
		req.TopK = 20
	}
	if req.TopK > 100 {
		req.TopK = 100
	}

	originalQuery := req.Query
	route := classifyQuery(originalQuery)
	expandedQuery := ""

	// Inject search tracing fields into context
	ctx = logger.WithFields(ctx, logger.Fields{
		logger.FieldComponent: "search",
		logger.FieldSearchID:  fmt.Sprintf("%d", ctx.Value("request_id")), // Will be overwritten if request_id exists
	})

	// Expand query using LLM if enabled (skip exact-match routes)
	if route != QueryRouteExact && s.queryExpansion != nil && s.queryExpansion.IsEnabled() {
		expanded, err := s.queryExpansion.Expand(ctx, req.Query)
		if err != nil {
			logger.CtxWarn(ctx, "Query expansion failed, using original query: query=%q, error=%v",
				req.Query, err)
		} else if expanded != req.Query {
			expandedQuery = expanded
			logger.CtxInfo(ctx, "Query expanded: original=%q, expanded=%q", req.Query, expanded)
		}
	}

	// Use expanded query for embedding if available
	queryForEmbedding := originalQuery
	if expandedQuery != "" {
		queryForEmbedding = expandedQuery
	}

	if profile, profileName, ok, err := s.resolveRequestedProfile(req); err != nil {
		return nil, err
	} else if ok {
		return s.searchProfile(ctx, req, profileName, profile, originalQuery, queryForEmbedding, expandedQuery)
	}

	qdrantRepo, embedding, collectionName, err := s.resolveCollection(req.Collection)
	if err != nil {
		return nil, err
	}

	logger.CtxInfo(ctx, "Performing text search: query=%q, query_for_embedding=%q, top_k=%d, collection=%s, route=%s",
		originalQuery, queryForEmbedding, req.TopK, collectionName, route)

	// Generate query embedding using the appropriate embedding provider
	queryEmbedding, err := embedding.EmbedQuery(ctx, queryForEmbedding)
	if err != nil {
		return nil, fmt.Errorf("failed to generate query embedding: %w", err)
	}

	// Build filters
	filters := &repository.SearchFilters{
		Category:     req.Category,
		TextPresence: req.TextPresence,
	}

	plan := buildHybridPlan(route, req.TopK)
	usingHybrid := true

	qdrantResults, err := qdrantRepo.HybridSearch(ctx, queryEmbedding, originalQuery, req.TopK, &plan, filters)
	if err != nil {
		usingHybrid = false
		logger.CtxWarn(ctx, "Hybrid search failed, falling back to dense search: error=%v", err)
		qdrantResults, err = qdrantRepo.Search(ctx, queryEmbedding, req.TopK, filters)
		if err != nil {
			return nil, fmt.Errorf("failed to search in Qdrant: %w", err)
		}
	}

	results := make([]SearchResult, 0, req.TopK)
	for _, qr := range qdrantResults {
		if qr.Payload == nil {
			continue
		}
		if !usingHybrid && s.scoreThreshold > 0 && qr.Score < s.scoreThreshold {
			continue
		}
		results = append(results, SearchResult{
			ID:           qr.Payload.MemeID,
			URL:          qr.Payload.StorageURL,
			Score:        qr.Score,
			Description:  qr.Payload.VLMDescription,
			Category:     qr.Payload.Category,
			Tags:         qr.Payload.Tags,
			TextPresence: qr.Payload.TextPresence,
		})
	}

	// Slice to TopK
	if len(results) > req.TopK {
		results = results[:req.TopK]
	}

	// Optionally enrich with full meme data from database
	if len(results) > 0 {
		ids := make([]string, len(results))
		for i, r := range results {
			ids[i] = r.ID
		}

		memes, err := s.memeRepo.GetByIDs(ctx, ids)
		if err != nil {
			logger.CtxWarn(ctx, "Failed to enrich results from database: error=%v", err)
		} else {
			memeMap := make(map[string]*domain.Meme)
			for i := range memes {
				memeMap[memes[i].ID] = &memes[i]
			}

			for i := range results {
				if meme, ok := memeMap[results[i].ID]; ok {
					results[i].ImageInfo = meme.ImageInfo
				}
			}
		}
	}

	return &SearchResponse{
		Results:       results,
		Total:         len(results),
		Query:         originalQuery,
		ExpandedQuery: expandedQuery,
		Collection:    collectionName,
	}, nil
}

func (s *SearchService) searchProfile(
	ctx context.Context,
	req *SearchRequest,
	profileName string,
	profile *SearchProfileConfig,
	originalQuery string,
	queryForEmbedding string,
	expandedQuery string,
) (*SearchResponse, error) {
	if profile == nil || profile.Image == nil || profile.Caption == nil ||
		profile.Image.QdrantRepo == nil || profile.Image.Embedding == nil ||
		profile.Caption.QdrantRepo == nil || profile.Caption.Embedding == nil {
		return nil, fmt.Errorf("profile %q is incomplete", profileName)
	}

	logger.CtxInfo(ctx, "Performing profile search: query=%q, query_for_embedding=%q, top_k=%d, profile=%s",
		originalQuery, queryForEmbedding, req.TopK, profileName)

	imageQueryEmbedding, err := profile.Image.Embedding.EmbedQuery(ctx, queryForEmbedding)
	if err != nil {
		return nil, fmt.Errorf("failed to generate image route query embedding: %w", err)
	}

	captionQueryEmbedding, err := profile.Caption.Embedding.EmbedQuery(ctx, queryForEmbedding)
	if err != nil {
		return nil, fmt.Errorf("failed to generate caption route query embedding: %w", err)
	}

	filters := &repository.SearchFilters{
		Category:     req.Category,
		TextPresence: req.TextPresence,
	}

	imageResults, imageErr := profile.Image.QdrantRepo.Search(ctx, imageQueryEmbedding, s.retrieval.ImageTopK, filters)
	if imageErr != nil {
		logger.CtxWarn(ctx, "Profile image search failed: profile=%s, error=%v", profileName, imageErr)
		imageResults = nil
	}

	captionResults, captionErr := profile.Caption.QdrantRepo.Search(ctx, captionQueryEmbedding, s.retrieval.CaptionTopK, filters)
	if captionErr != nil {
		logger.CtxWarn(ctx, "Profile caption search failed: profile=%s, error=%v", profileName, captionErr)
		captionResults = nil
	}

	keywordResults, keywordErr := profile.Caption.QdrantRepo.SparseSearch(ctx, originalQuery, s.retrieval.CaptionTopK, filters)
	if keywordErr != nil {
		logger.CtxWarn(ctx, "Profile keyword search failed: profile=%s, error=%v", profileName, keywordErr)
		keywordResults = nil
	}

	if imageErr != nil && captionErr != nil && keywordErr != nil {
		return nil, fmt.Errorf("all profile search routes failed: image=%v, caption=%v, keyword=%v", imageErr, captionErr, keywordErr)
	}

	finalTopK := req.TopK
	if finalTopK <= 0 {
		finalTopK = s.retrieval.FinalTopK
	}
	results := fuseProfileResults(imageResults, captionResults, keywordResults, s.retrieval.Weights, finalTopK)
	s.enrichSearchResults(ctx, results)

	return &SearchResponse{
		Results:       results,
		Total:         len(results),
		Query:         originalQuery,
		ExpandedQuery: expandedQuery,
		Profile:       profileName,
	}, nil
}

type routeResults struct {
	results []repository.SearchResult
	weight  float32
}

func fuseProfileResults(
	imageResults []repository.SearchResult,
	captionResults []repository.SearchResult,
	keywordResults []repository.SearchResult,
	weights RetrievalWeights,
	topK int,
) []SearchResult {
	if topK <= 0 {
		topK = 20
	}
	routes := []routeResults{
		{results: imageResults, weight: weights.Image},
		{results: captionResults, weight: weights.Caption},
		{results: keywordResults, weight: weights.Keyword},
	}

	type scoredResult struct {
		result SearchResult
		score  float32
	}
	byMemeID := make(map[string]*scoredResult)
	maxScore := float32(0)
	for _, route := range routes {
		if route.weight <= 0 {
			continue
		}
		for rank, qr := range route.results {
			if qr.Payload == nil || qr.Payload.MemeID == "" {
				continue
			}
			rankScore := route.weight * (1 / float32(rank+60))
			item, ok := byMemeID[qr.Payload.MemeID]
			if !ok {
				item = &scoredResult{
					result: SearchResult{
						ID:           qr.Payload.MemeID,
						URL:          qr.Payload.StorageURL,
						Description:  qr.Payload.VLMDescription,
						Category:     qr.Payload.Category,
						Tags:         qr.Payload.Tags,
						TextPresence: qr.Payload.TextPresence,
					},
				}
				byMemeID[qr.Payload.MemeID] = item
			}
			item.score += rankScore
			if item.score > maxScore {
				maxScore = item.score
			}
		}
	}

	items := make([]scoredResult, 0, len(byMemeID))
	for _, item := range byMemeID {
		if maxScore > 0 {
			item.result.Score = item.score / maxScore
		}
		items = append(items, *item)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].result.Score == items[j].result.Score {
			return items[i].result.ID < items[j].result.ID
		}
		return items[i].result.Score > items[j].result.Score
	})

	if len(items) > topK {
		items = items[:topK]
	}

	results := make([]SearchResult, len(items))
	for i := range items {
		results[i] = items[i].result
	}
	return results
}

func (s *SearchService) enrichSearchResults(ctx context.Context, results []SearchResult) {
	if len(results) == 0 || s.memeRepo == nil {
		return
	}
	ids := make([]string, len(results))
	for i, r := range results {
		ids[i] = r.ID
	}

	memes, err := s.memeRepo.GetByIDs(ctx, ids)
	if err != nil {
		logger.CtxWarn(ctx, "Failed to enrich results from database: error=%v", err)
		return
	}

	memeMap := make(map[string]*domain.Meme)
	for i := range memes {
		memeMap[memes[i].ID] = &memes[i]
	}

	for i := range results {
		if meme, ok := memeMap[results[i].ID]; ok {
			results[i].ImageInfo = meme.ImageInfo
		}
	}
}

// TextSearchWithProgress performs a hybrid text search with progress updates.
// Parameters:
//   - ctx: context for cancellation and deadlines.
//   - req: search request parameters.
//   - progressCh: channel for sending progress updates.
//
// Returns:
//   - *SearchResponse: search results and metadata.
//   - error: non-nil if search fails.
func (s *SearchService) TextSearchWithProgress(ctx context.Context, req *SearchRequest, progressCh chan<- SearchProgress) (*SearchResponse, error) {
	defer close(progressCh)

	// Set defaults
	if req.TopK <= 0 {
		req.TopK = 20
	}
	if req.TopK > 100 {
		req.TopK = 100
	}

	originalQuery := req.Query
	route := classifyQuery(originalQuery)
	expandedQuery := ""

	// Stage 1: Query Expansion (with streaming)
	if route != QueryRouteExact && s.queryExpansion != nil && s.queryExpansion.IsEnabled() {
		// Send start event
		progressCh <- SearchProgress{
			Stage:   "query_expansion_start",
			Message: "AI 正在理解搜索意图...",
		}

		// Create token channel for streaming
		tokenCh := make(chan string, 100)
		expandDone := make(chan struct{})
		var expandErr error

		go func() {
			defer close(expandDone)
			expandedQuery, expandErr = s.queryExpansion.ExpandStream(ctx, req.Query, tokenCh)
		}()

		// Stream thinking tokens
		for token := range tokenCh {
			progressCh <- SearchProgress{
				Stage:        "thinking",
				ThinkingText: token,
				IsDelta:      true,
			}
		}

		<-expandDone

		if expandErr != nil {
			logger.CtxWarn(ctx, "Query expansion failed, using original query: query=%q, error=%v",
				req.Query, expandErr)
			// Silent fallback - continue with original query
			expandedQuery = ""
		} else if expandedQuery != req.Query && expandedQuery != "" {
			logger.CtxInfo(ctx, "Query expanded: original=%q, expanded=%q", req.Query, expandedQuery)

			progressCh <- SearchProgress{
				Stage:         "query_expansion_done",
				Message:       "理解完成",
				ExpandedQuery: expandedQuery,
			}
		}
	}

	// Use expanded query for embedding if available
	queryForEmbedding := originalQuery
	if expandedQuery != "" {
		queryForEmbedding = expandedQuery
	}

	// Stage 2: Generate Embedding
	progressCh <- SearchProgress{
		Stage:   "embedding",
		Message: "正在生成语义向量...",
	}

	if profile, profileName, ok, err := s.resolveRequestedProfile(req); err != nil {
		return nil, err
	} else if ok {
		progressCh <- SearchProgress{
			Stage:   "searching",
			Message: "在表情库中搜索...",
		}
		result, err := s.searchProfile(ctx, req, profileName, profile, originalQuery, queryForEmbedding, expandedQuery)
		if err != nil {
			return nil, err
		}
		if len(result.Results) > 0 {
			progressCh <- SearchProgress{
				Stage:   "enriching",
				Message: "加载表情包详情...",
			}
		}
		return result, nil
	}

	qdrantRepo, embedding, collectionName, err := s.resolveCollection(req.Collection)
	if err != nil {
		return nil, err
	}

	logger.CtxInfo(ctx, "Performing text search: query=%q, query_for_embedding=%q, top_k=%d, collection=%s, route=%s",
		originalQuery, queryForEmbedding, req.TopK, collectionName, route)

	queryEmbedding, err := embedding.EmbedQuery(ctx, queryForEmbedding)
	if err != nil {
		return nil, fmt.Errorf("failed to generate query embedding: %w", err)
	}

	// Stage 3: Search in Qdrant
	progressCh <- SearchProgress{
		Stage:   "searching",
		Message: "在表情库中搜索...",
	}

	filters := &repository.SearchFilters{
		Category:     req.Category,
		TextPresence: req.TextPresence,
	}

	plan := buildHybridPlan(route, req.TopK)
	usingHybrid := true

	qdrantResults, err := qdrantRepo.HybridSearch(ctx, queryEmbedding, originalQuery, req.TopK, &plan, filters)
	if err != nil {
		usingHybrid = false
		logger.CtxWarn(ctx, "Hybrid search failed, falling back to dense search: error=%v", err)
		progressCh <- SearchProgress{
			Stage:   "searching",
			Message: "混合检索失败，切换为语义检索...",
		}
		qdrantResults, err = qdrantRepo.Search(ctx, queryEmbedding, req.TopK, filters)
		if err != nil {
			return nil, fmt.Errorf("failed to search in Qdrant: %w", err)
		}
	}

	results := make([]SearchResult, 0, req.TopK)
	for _, qr := range qdrantResults {
		if qr.Payload == nil {
			continue
		}
		if !usingHybrid && s.scoreThreshold > 0 && qr.Score < s.scoreThreshold {
			continue
		}
		result := SearchResult{
			ID:           qr.Payload.MemeID,
			URL:          qr.Payload.StorageURL,
			Score:        qr.Score,
			Description:  qr.Payload.VLMDescription,
			Category:     qr.Payload.Category,
			Tags:         qr.Payload.Tags,
			TextPresence: qr.Payload.TextPresence,
		}
		results = append(results, result)
	}

	// Slice to TopK
	if len(results) > req.TopK {
		results = results[:req.TopK]
	}

	// Stage 4: Enrich with database data
	if len(results) > 0 {
		progressCh <- SearchProgress{
			Stage:   "enriching",
			Message: "加载表情包详情...",
		}

		ids := make([]string, len(results))
		for i, r := range results {
			ids[i] = r.ID
		}

		memes, err := s.memeRepo.GetByIDs(ctx, ids)
		if err != nil {
			logger.CtxWarn(ctx, "Failed to enrich results from database: error=%v", err)
		} else {
			memeMap := make(map[string]*domain.Meme)
			for i := range memes {
				memeMap[memes[i].ID] = &memes[i]
			}

			for i := range results {
				if meme, ok := memeMap[results[i].ID]; ok {
					results[i].ImageInfo = meme.ImageInfo
				}
			}
		}
	}

	return &SearchResponse{
		Results:       results,
		Total:         len(results),
		Query:         originalQuery,
		ExpandedQuery: expandedQuery,
		Collection:    collectionName,
	}, nil
}

// GetCategories returns all available categories.
// Parameters:
//   - ctx: context for cancellation and deadlines.
//
// Returns:
//   - []string: distinct category names.
//   - error: non-nil if lookup fails.
func (s *SearchService) GetCategories(ctx context.Context) ([]string, error) {
	return s.memeRepo.GetCategories(ctx)
}

// GetMemeByID retrieves a meme by its ID.
// Parameters:
//   - ctx: context for cancellation and deadlines.
//   - id: meme ID.
//
// Returns:
//   - *domain.Meme: meme record if found.
//   - error: non-nil if lookup fails.
func (s *SearchService) GetMemeByID(ctx context.Context, id string) (*domain.Meme, error) {
	return s.memeRepo.GetByID(ctx, id)
}

// MemeListResponse represents the response for listing memes.
type MemeListResponse struct {
	Results []SearchResult `json:"results"`
	Total   int            `json:"total"`
	Limit   int            `json:"limit"`
	Offset  int            `json:"offset"`
}

// ListMemes retrieves memes with optional category filter.
// Parameters:
//   - ctx: context for cancellation and deadlines.
//   - category: category name to filter by; empty means all.
//   - limit: maximum number of records to return.
//   - offset: number of records to skip.
//
// Returns:
//   - *MemeListResponse: list results in search-compatible format.
//   - error: non-nil if retrieval fails.
//
// Returns results in the same format as search results for API consistency.
func (s *SearchService) ListMemes(ctx context.Context, category string, limit, offset int) (*MemeListResponse, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	memes, err := s.memeRepo.ListByCategory(ctx, category, limit, offset)
	if err != nil {
		return nil, err
	}

	// Convert domain.Meme to SearchResult format for API consistency
	results := make([]SearchResult, len(memes))
	for i, meme := range memes {
		// Generate URL from storage_key
		url := ""
		if meme.StorageKey != "" && s.storage != nil {
			url = s.storage.GetURL(meme.StorageKey)
		}

		results[i] = SearchResult{
			ID:          meme.ID,
			URL:         url,
			Score:       0,  // No score for listing (not a search)
			Description: "", // VLM description moved to meme_annotations table; use search for descriptions
			Category:    meme.Category,
			Tags:        meme.Tags,
			ImageInfo:   meme.ImageInfo,
		}
	}

	return &MemeListResponse{
		Results: results,
		Total:   len(results),
		Limit:   limit,
		Offset:  offset,
	}, nil
}

// GetStats returns search-related statistics.
// Parameters:
//   - ctx: context for cancellation and deadlines.
//
// Returns:
//   - map[string]interface{}: aggregated stats for search and ingest.
//   - error: non-nil if statistics cannot be computed.
func (s *SearchService) GetStats(ctx context.Context) (map[string]interface{}, error) {
	totalCount, err := s.memeRepo.Count(ctx)
	if err != nil {
		return nil, err
	}

	categories, err := s.memeRepo.GetCategories(ctx)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"total_memes":           totalCount,
		"total_categories":      len(categories),
		"available_collections": s.GetAvailableCollections(),
		"available_profiles":    s.GetAvailableProfiles(),
	}, nil
}
