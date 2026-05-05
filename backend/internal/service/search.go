package service

import (
	"context"
	"fmt"
	"sort"

	pb "github.com/timmy/emomo/gen/emomo/v1"
	"github.com/timmy/emomo/internal/domain"
	"github.com/timmy/emomo/internal/logger"
	"github.com/timmy/emomo/internal/persistence"
	"github.com/timmy/emomo/internal/repository"
	"github.com/timmy/emomo/internal/storage"
	"google.golang.org/protobuf/types/known/timestamppb"
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
		FinalTopK:   100,
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

func (s *SearchService) resolveRequestedProfile(req *pb.SearchRequest) (*SearchProfileConfig, string, bool, error) {
	requested := req.GetProfile()
	if requested == "" {
		requested = req.GetCollection()
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
	if req.GetProfile() != "" {
		return nil, "", false, fmt.Errorf("unknown profile: %s", req.GetProfile())
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

// buildSearchFilters projects the protobuf request into the repository's
// generic filter struct. UNSPECIFIED text presence becomes "no filter".
func buildSearchFilters(req *pb.SearchRequest) *repository.SearchFilters {
	filters := &repository.SearchFilters{}
	if cat := req.GetCategory(); cat != "" {
		filters.Category = &cat
	}
	if presence := req.GetTextPresence(); presence != pb.TextPresence_TEXT_PRESENCE_UNSPECIFIED {
		s := persistence.TextPresenceToString(presence)
		filters.TextPresence = &s
	}
	return filters
}

// memeToPb projects a domain.Meme record into the wire-shape protobuf
// message, populating url from the storage backend. Returns nil for nil
// inputs.
func (s *SearchService) memeToPb(meme *domain.Meme) *pb.Meme {
	if meme == nil {
		return nil
	}
	url := ""
	if meme.StorageKey != "" && s.storage != nil {
		url = s.storage.GetURL(meme.StorageKey)
	}
	return &pb.Meme{
		Id:          meme.ID,
		StorageKey:  meme.StorageKey,
		ContentHash: meme.ContentHash,
		ImageInfo:   meme.ImageInfo,
		Tags:        []string(meme.Tags),
		Category:    meme.Category,
		Url:         url,
		CreatedAt:   timestamppb.New(meme.CreatedAt),
		UpdatedAt:   timestamppb.New(meme.UpdatedAt),
	}
}

// applyTopKDefaults clamps top_k into the [1, 100] range, applying a 20
// default when unset.
func applyTopKDefaults(topK int32) int32 {
	if topK <= 0 {
		return 20
	}
	if topK > 100 {
		return 100
	}
	return topK
}

// TextSearch performs a hybrid text search (dense + BM25) against the
// configured collection or profile.
func (s *SearchService) TextSearch(ctx context.Context, req *pb.SearchRequest) (*pb.SearchResponse, error) {
	topK := applyTopKDefaults(req.GetTopK())
	originalQuery := req.GetQuery()
	route := classifyQuery(originalQuery)
	expandedQuery := ""

	ctx = logger.WithFields(ctx, logger.Fields{
		logger.FieldComponent: "search",
		logger.FieldSearchID:  fmt.Sprintf("%d", ctx.Value("request_id")),
	})

	if route != QueryRouteExact && s.queryExpansion != nil && s.queryExpansion.IsEnabled() {
		expanded, err := s.queryExpansion.Expand(ctx, originalQuery)
		if err != nil {
			logger.CtxWarn(ctx, "Query expansion failed, using original query: query=%q, error=%v",
				originalQuery, err)
		} else if expanded != originalQuery {
			expandedQuery = expanded
			logger.CtxInfo(ctx, "Query expanded: original=%q, expanded=%q", originalQuery, expanded)
		}
	}

	queryForEmbedding := originalQuery
	if expandedQuery != "" {
		queryForEmbedding = expandedQuery
	}

	if profile, profileName, ok, err := s.resolveRequestedProfile(req); err != nil {
		return nil, err
	} else if ok {
		return s.searchProfile(ctx, req, topK, profileName, profile, originalQuery, queryForEmbedding, expandedQuery)
	}

	qdrantRepo, embedding, collectionName, err := s.resolveCollection(req.GetCollection())
	if err != nil {
		return nil, err
	}

	logger.CtxInfo(ctx, "Performing text search: query=%q, query_for_embedding=%q, top_k=%d, collection=%s, route=%s",
		originalQuery, queryForEmbedding, topK, collectionName, route)

	queryEmbedding, err := embedding.EmbedQuery(ctx, queryForEmbedding)
	if err != nil {
		return nil, fmt.Errorf("failed to generate query embedding: %w", err)
	}

	filters := buildSearchFilters(req)
	plan := buildHybridPlan(route, int(topK))
	usingHybrid := true

	qdrantResults, err := qdrantRepo.HybridSearch(ctx, queryEmbedding, originalQuery, int(topK), &plan, filters)
	if err != nil {
		usingHybrid = false
		logger.CtxWarn(ctx, "Hybrid search failed, falling back to dense search: error=%v", err)
		qdrantResults, err = qdrantRepo.Search(ctx, queryEmbedding, int(topK), filters)
		if err != nil {
			return nil, fmt.Errorf("failed to search in Qdrant: %w", err)
		}
	}

	results := s.buildSearchResultsFromQdrant(qdrantResults, usingHybrid, int(topK))
	s.enrichSearchResults(ctx, results)

	return &pb.SearchResponse{
		Results:       results,
		Total:         int32(len(results)),
		Query:         originalQuery,
		ExpandedQuery: expandedQuery,
		Collection:    collectionName,
	}, nil
}

func (s *SearchService) buildSearchResultsFromQdrant(qdrantResults []repository.SearchResult, usingHybrid bool, topK int) []*pb.SearchResult {
	results := make([]*pb.SearchResult, 0, topK)
	for _, qr := range qdrantResults {
		if qr.Payload == nil {
			continue
		}
		if !usingHybrid && s.scoreThreshold > 0 && qr.Score < s.scoreThreshold {
			continue
		}
		results = append(results, &pb.SearchResult{
			Meme: &pb.Meme{
				Id:       qr.Payload.MemeID,
				Url:      qr.Payload.StorageURL,
				Category: qr.Payload.Category,
				Tags:     qr.Payload.Tags,
			},
			Score:        qr.Score,
			Description:  qr.Payload.VLMDescription,
			TextPresence: persistence.TextPresenceFromString(qr.Payload.TextPresence),
		})
	}
	if len(results) > topK {
		results = results[:topK]
	}
	return results
}

func (s *SearchService) searchProfile(
	ctx context.Context,
	req *pb.SearchRequest,
	topK int32,
	profileName string,
	profile *SearchProfileConfig,
	originalQuery string,
	queryForEmbedding string,
	expandedQuery string,
) (*pb.SearchResponse, error) {
	if profile == nil || profile.Image == nil || profile.Caption == nil ||
		profile.Image.QdrantRepo == nil || profile.Image.Embedding == nil ||
		profile.Caption.QdrantRepo == nil || profile.Caption.Embedding == nil {
		return nil, fmt.Errorf("profile %q is incomplete", profileName)
	}

	logger.CtxInfo(ctx, "Performing profile search: query=%q, query_for_embedding=%q, top_k=%d, profile=%s",
		originalQuery, queryForEmbedding, topK, profileName)

	imageQueryEmbedding, err := profile.Image.Embedding.EmbedQuery(ctx, queryForEmbedding)
	if err != nil {
		return nil, fmt.Errorf("failed to generate image route query embedding: %w", err)
	}

	captionQueryEmbedding, err := profile.Caption.Embedding.EmbedQuery(ctx, queryForEmbedding)
	if err != nil {
		return nil, fmt.Errorf("failed to generate caption route query embedding: %w", err)
	}

	filters := buildSearchFilters(req)

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

	finalTopK := int(topK)
	if finalTopK <= 0 {
		finalTopK = s.retrieval.FinalTopK
	}
	results := fuseProfileResults(imageResults, captionResults, keywordResults, s.retrieval.Weights, finalTopK)
	s.enrichSearchResults(ctx, results)

	return &pb.SearchResponse{
		Results:       results,
		Total:         int32(len(results)),
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
) []*pb.SearchResult {
	if topK <= 0 {
		topK = 20
	}
	routes := []routeResults{
		{results: imageResults, weight: weights.Image},
		{results: captionResults, weight: weights.Caption},
		{results: keywordResults, weight: weights.Keyword},
	}

	type scoredResult struct {
		result *pb.SearchResult
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
					result: &pb.SearchResult{
						Meme: &pb.Meme{
							Id:       qr.Payload.MemeID,
							Url:      qr.Payload.StorageURL,
							Category: qr.Payload.Category,
							Tags:     qr.Payload.Tags,
						},
						Description:  qr.Payload.VLMDescription,
						TextPresence: persistence.TextPresenceFromString(qr.Payload.TextPresence),
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

	items := make([]*scoredResult, 0, len(byMemeID))
	for _, item := range byMemeID {
		if maxScore > 0 {
			item.result.Score = item.score / maxScore
		}
		items = append(items, item)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].result.Score == items[j].result.Score {
			return items[i].result.Meme.GetId() < items[j].result.Meme.GetId()
		}
		return items[i].result.Score > items[j].result.Score
	})

	if len(items) > topK {
		items = items[:topK]
	}

	results := make([]*pb.SearchResult, len(items))
	for i := range items {
		results[i] = items[i].result
	}
	return results
}

// enrichSearchResults backfills full meme metadata (image_info, content_hash,
// timestamps) into already-built SearchResult.Meme objects, using the
// relational store as the source of truth.
func (s *SearchService) enrichSearchResults(ctx context.Context, results []*pb.SearchResult) {
	if len(results) == 0 || s.memeRepo == nil {
		return
	}
	ids := make([]string, 0, len(results))
	for _, r := range results {
		if r != nil && r.Meme != nil {
			ids = append(ids, r.Meme.GetId())
		}
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

	for _, r := range results {
		if r == nil || r.Meme == nil {
			continue
		}
		if dbMeme, ok := memeMap[r.Meme.GetId()]; ok {
			r.Meme.StorageKey = dbMeme.StorageKey
			r.Meme.ContentHash = dbMeme.ContentHash
			r.Meme.ImageInfo = dbMeme.ImageInfo
			if r.Meme.Url == "" && dbMeme.StorageKey != "" && s.storage != nil {
				r.Meme.Url = s.storage.GetURL(dbMeme.StorageKey)
			}
			// keep existing tags/category from qdrant payload if non-empty;
			// fall back to relational record otherwise.
			if len(r.Meme.Tags) == 0 {
				r.Meme.Tags = []string(dbMeme.Tags)
			}
			if r.Meme.Category == "" {
				r.Meme.Category = dbMeme.Category
			}
			r.Meme.CreatedAt = timestamppb.New(dbMeme.CreatedAt)
			r.Meme.UpdatedAt = timestamppb.New(dbMeme.UpdatedAt)
		}
	}
}

// TextSearchWithProgress performs a hybrid text search with progress updates.
// The progress channel is closed by this function before it returns.
func (s *SearchService) TextSearchWithProgress(
	ctx context.Context,
	req *pb.SearchRequest,
	progressCh chan<- *pb.SearchProgressEvent,
) (*pb.SearchResponse, error) {
	defer close(progressCh)

	topK := applyTopKDefaults(req.GetTopK())
	originalQuery := req.GetQuery()
	route := classifyQuery(originalQuery)
	expandedQuery := ""

	if route != QueryRouteExact && s.queryExpansion != nil && s.queryExpansion.IsEnabled() {
		progressCh <- &pb.SearchProgressEvent{
			Stage:   pb.SearchStage_SEARCH_STAGE_QUERY_EXPANSION_START,
			Message: "AI 正在理解搜索意图...",
		}

		tokenCh := make(chan string, 100)
		expandDone := make(chan struct{})
		var expandErr error

		go func() {
			defer close(expandDone)
			expandedQuery, expandErr = s.queryExpansion.ExpandStream(ctx, originalQuery, tokenCh)
		}()

		for token := range tokenCh {
			progressCh <- &pb.SearchProgressEvent{
				Stage: pb.SearchStage_SEARCH_STAGE_THINKING,
				Payload: &pb.SearchProgressEvent_Thinking{
					Thinking: &pb.ThinkingDelta{
						Text:    token,
						IsDelta: true,
					},
				},
			}
		}

		<-expandDone

		if expandErr != nil {
			logger.CtxWarn(ctx, "Query expansion failed, using original query: query=%q, error=%v",
				originalQuery, expandErr)
			expandedQuery = ""
		} else if expandedQuery != originalQuery && expandedQuery != "" {
			logger.CtxInfo(ctx, "Query expanded: original=%q, expanded=%q", originalQuery, expandedQuery)
			progressCh <- &pb.SearchProgressEvent{
				Stage:   pb.SearchStage_SEARCH_STAGE_QUERY_EXPANSION_DONE,
				Message: "理解完成",
				Payload: &pb.SearchProgressEvent_Expansion{
					Expansion: &pb.ExpansionDone{
						ExpandedQuery: expandedQuery,
					},
				},
			}
		}
	}

	queryForEmbedding := originalQuery
	if expandedQuery != "" {
		queryForEmbedding = expandedQuery
	}

	progressCh <- &pb.SearchProgressEvent{
		Stage:   pb.SearchStage_SEARCH_STAGE_EMBEDDING,
		Message: "正在生成语义向量...",
	}

	if profile, profileName, ok, err := s.resolveRequestedProfile(req); err != nil {
		return nil, err
	} else if ok {
		progressCh <- &pb.SearchProgressEvent{
			Stage:   pb.SearchStage_SEARCH_STAGE_SEARCHING,
			Message: "在表情库中搜索...",
		}
		result, err := s.searchProfile(ctx, req, topK, profileName, profile, originalQuery, queryForEmbedding, expandedQuery)
		if err != nil {
			return nil, err
		}
		if len(result.GetResults()) > 0 {
			progressCh <- &pb.SearchProgressEvent{
				Stage:   pb.SearchStage_SEARCH_STAGE_ENRICHING,
				Message: "加载表情包详情...",
			}
		}
		return result, nil
	}

	qdrantRepo, embedding, collectionName, err := s.resolveCollection(req.GetCollection())
	if err != nil {
		return nil, err
	}

	logger.CtxInfo(ctx, "Performing text search: query=%q, query_for_embedding=%q, top_k=%d, collection=%s, route=%s",
		originalQuery, queryForEmbedding, topK, collectionName, route)

	queryEmbedding, err := embedding.EmbedQuery(ctx, queryForEmbedding)
	if err != nil {
		return nil, fmt.Errorf("failed to generate query embedding: %w", err)
	}

	progressCh <- &pb.SearchProgressEvent{
		Stage:   pb.SearchStage_SEARCH_STAGE_SEARCHING,
		Message: "在表情库中搜索...",
	}

	filters := buildSearchFilters(req)
	plan := buildHybridPlan(route, int(topK))
	usingHybrid := true

	qdrantResults, err := qdrantRepo.HybridSearch(ctx, queryEmbedding, originalQuery, int(topK), &plan, filters)
	if err != nil {
		usingHybrid = false
		logger.CtxWarn(ctx, "Hybrid search failed, falling back to dense search: error=%v", err)
		progressCh <- &pb.SearchProgressEvent{
			Stage:   pb.SearchStage_SEARCH_STAGE_SEARCHING,
			Message: "混合检索失败，切换为语义检索...",
		}
		qdrantResults, err = qdrantRepo.Search(ctx, queryEmbedding, int(topK), filters)
		if err != nil {
			return nil, fmt.Errorf("failed to search in Qdrant: %w", err)
		}
	}

	results := s.buildSearchResultsFromQdrant(qdrantResults, usingHybrid, int(topK))
	if len(results) > 0 {
		progressCh <- &pb.SearchProgressEvent{
			Stage:   pb.SearchStage_SEARCH_STAGE_ENRICHING,
			Message: "加载表情包详情...",
		}
		s.enrichSearchResults(ctx, results)
	}

	return &pb.SearchResponse{
		Results:       results,
		Total:         int32(len(results)),
		Query:         originalQuery,
		ExpandedQuery: expandedQuery,
		Collection:    collectionName,
	}, nil
}

// GetCategories returns all available categories.
func (s *SearchService) GetCategories(ctx context.Context) ([]string, error) {
	return s.memeRepo.GetCategories(ctx)
}

// GetMeme retrieves a meme by its ID.
func (s *SearchService) GetMeme(ctx context.Context, req *pb.GetMemeRequest) (*pb.GetMemeResponse, error) {
	meme, err := s.memeRepo.GetByID(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	return &pb.GetMemeResponse{Meme: s.memeToPb(meme)}, nil
}

// ListMemes retrieves memes with optional category filter.
func (s *SearchService) ListMemes(ctx context.Context, req *pb.ListMemesRequest) (*pb.ListMemesResponse, error) {
	limit := req.GetLimit()
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	offset := req.GetOffset()
	if offset < 0 {
		offset = 0
	}

	memes, err := s.memeRepo.ListByCategory(ctx, req.GetCategory(), int(limit), int(offset))
	if err != nil {
		return nil, err
	}

	results := make([]*pb.SearchResult, len(memes))
	for i, meme := range memes {
		results[i] = &pb.SearchResult{
			Meme: s.memeToPb(&meme),
		}
	}

	return &pb.ListMemesResponse{
		Results: results,
		Total:   int32(len(results)),
		Limit:   limit,
		Offset:  offset,
	}, nil
}

// GetStats returns search-related statistics.
func (s *SearchService) GetStats(ctx context.Context) (*pb.GetStatsResponse, error) {
	totalCount, err := s.memeRepo.Count(ctx)
	if err != nil {
		return nil, err
	}

	categories, err := s.memeRepo.GetCategories(ctx)
	if err != nil {
		return nil, err
	}

	return &pb.GetStatsResponse{
		TotalMemes:           totalCount,
		TotalCategories:      int32(len(categories)),
		AvailableCollections: s.GetAvailableCollections(),
		AvailableProfiles:    s.GetAvailableProfiles(),
	}, nil
}
