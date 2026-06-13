package service

import (
	"context"
	"fmt"
	"sort"

	"github.com/google/uuid"
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
	Agentic           *AgenticSearchService
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

const withTextResultBoost = 1.06

// SearchProfileConfig binds vector collections into one profile. Caption and
// keyword are optional so deployments can run image-first search while the
// caption dense embedding strategy is being evaluated.
type SearchProfileConfig struct {
	Image   *CollectionConfig
	Caption *CollectionConfig
	Keyword *CollectionConfig
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
	agentic            *AgenticSearchService

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
		agentic:            cfgAgentic(cfg),
		collections:        make(map[string]*CollectionConfig),
		profiles:           make(map[string]*SearchProfileConfig),
	}
}

func cfgAgentic(cfg *SearchConfig) *AgenticSearchService {
	if cfg == nil {
		return nil
	}
	return cfg.Agentic
}

// RegisterCollection registers a collection configuration for multi-collection search.
func (s *SearchService) RegisterCollection(name string, qdrantRepo *repository.QdrantRepository, embedding EmbeddingProvider) {
	s.collections[name] = &CollectionConfig{
		QdrantRepo: qdrantRepo,
		Embedding:  embedding,
	}
}

// RegisterProfile registers a search profile. The caption route is optional.
func (s *SearchService) RegisterProfile(
	name string,
	imageRepo *repository.QdrantRepository,
	imageEmbedding EmbeddingProvider,
	captionRepo *repository.QdrantRepository,
	captionEmbedding EmbeddingProvider,
	keywordRepo *repository.QdrantRepository,
) {
	profile := &SearchProfileConfig{
		Image: &CollectionConfig{
			QdrantRepo: imageRepo,
			Embedding:  imageEmbedding,
		},
	}
	if captionRepo != nil && captionEmbedding != nil {
		profile.Caption = &CollectionConfig{
			QdrantRepo: captionRepo,
			Embedding:  captionEmbedding,
		}
	}
	if keywordRepo != nil {
		profile.Keyword = &CollectionConfig{
			QdrantRepo: keywordRepo,
		}
	}
	s.profiles[name] = profile
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

func hasUsableImageRoute(profile *SearchProfileConfig) bool {
	return profile != nil &&
		profile.Image != nil &&
		profile.Image.QdrantRepo != nil &&
		profile.Image.Embedding != nil
}

func hasUsableCaptionRoute(profile *SearchProfileConfig) bool {
	return profile != nil &&
		profile.Caption != nil &&
		profile.Caption.QdrantRepo != nil &&
		profile.Caption.Embedding != nil
}

func keywordRoute(profile *SearchProfileConfig) *CollectionConfig {
	if profile == nil {
		return nil
	}
	if profile.Keyword != nil && profile.Keyword.QdrantRepo != nil {
		return profile.Keyword
	}
	if profile.Caption != nil && profile.Caption.QdrantRepo != nil {
		return profile.Caption
	}
	return nil
}

func defaultRetrievalConfig() RetrievalConfig {
	return RetrievalConfig{
		ImageTopK:   100,
		CaptionTopK: 100,
		FinalTopK:   100,
		Weights: RetrievalWeights{
			Image:   0.70,
			Caption: 0.00,
			Keyword: 0.30,
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

// applyTopKDefaults clamps top_k into the [1, 100] range, applying a 100
// default when unset.
func applyTopKDefaults(topK int32) int32 {
	if topK <= 0 {
		return 100
	}
	if topK > 100 {
		return 100
	}
	return topK
}

func withSearchLogFields(ctx context.Context) context.Context {
	searchID := logger.GetSearchID(ctx)
	if searchID == "" {
		searchID = logger.GetRequestID(ctx)
	}
	if searchID == "" {
		searchID = uuid.New().String()
	}
	return logger.WithFields(ctx, logger.Fields{
		logger.FieldComponent: "search",
		logger.FieldSearchID:  searchID,
	})
}

func (s *SearchService) prepareLegacyQuery(ctx context.Context, originalQuery string, route QueryRoute) (string, string) {
	expandedQuery := ""
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
	return queryForEmbedding, expandedQuery
}

func (s *SearchService) prepareLegacyQueryWithProgress(
	ctx context.Context,
	originalQuery string,
	route QueryRoute,
	progressCh chan<- *pb.SearchProgressEvent,
) (string, string) {
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
	return queryForEmbedding, expandedQuery
}

// TextSearch performs a hybrid text search (dense + BM25) against the
// configured collection or profile.
func (s *SearchService) TextSearch(ctx context.Context, req *pb.SearchRequest) (*pb.SearchResponse, error) {
	topK := applyTopKDefaults(req.GetTopK())
	originalQuery := req.GetQuery()
	route := classifyQuery(originalQuery)
	agenticEnabled := s.agentic.IsEnabled()

	ctx = withSearchLogFields(ctx)

	if profile, profileName, ok, err := s.resolveRequestedProfile(req); err != nil {
		return nil, err
	} else if ok {
		if agenticEnabled {
			return s.searchProfileAgentic(ctx, req, topK, profileName, profile, originalQuery, func() (*pb.SearchResponse, error) {
				queryForEmbedding, expandedQuery := s.prepareLegacyQuery(ctx, originalQuery, route)
				return s.searchProfile(ctx, req, topK, profileName, profile, originalQuery, queryForEmbedding, expandedQuery)
			})
		}
		queryForEmbedding, expandedQuery := s.prepareLegacyQuery(ctx, originalQuery, route)
		return s.searchProfile(ctx, req, topK, profileName, profile, originalQuery, queryForEmbedding, expandedQuery)
	}

	queryForEmbedding, expandedQuery := s.prepareLegacyQuery(ctx, originalQuery, route)
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

	results := s.buildSearchResultsFromQdrant(qdrantResults, usingHybrid, int(topK), shouldBoostWithTextResults(req, pb.TextPresence_TEXT_PRESENCE_UNSPECIFIED))
	s.enrichSearchResults(ctx, results)

	return &pb.SearchResponse{
		Results:       results,
		Total:         int32(len(results)),
		Query:         originalQuery,
		ExpandedQuery: expandedQuery,
		Collection:    collectionName,
	}, nil
}

func (s *SearchService) buildSearchResultsFromQdrant(qdrantResults []repository.SearchResult, usingHybrid bool, topK int, boostWithTextResults bool) []*pb.SearchResult {
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
	if boostWithTextResults {
		boostAndSortWithTextResults(results)
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
	if !hasUsableImageRoute(profile) {
		return nil, fmt.Errorf("profile %q is incomplete", profileName)
	}

	logger.CtxInfo(ctx, "Performing profile search: query=%q, query_for_embedding=%q, top_k=%d, profile=%s",
		originalQuery, queryForEmbedding, topK, profileName)

	imageQueryEmbedding, err := profile.Image.Embedding.EmbedQuery(ctx, queryForEmbedding)
	if err != nil {
		return nil, fmt.Errorf("failed to generate image route query embedding: %w", err)
	}

	filters := buildSearchFilters(req)

	imageResults, imageErr := profile.Image.QdrantRepo.Search(ctx, imageQueryEmbedding, s.retrieval.ImageTopK, filters)
	if imageErr != nil {
		logger.CtxWarn(ctx, "Profile image search failed: profile=%s, error=%v", profileName, imageErr)
		imageResults = nil
	}

	captionRouteAvailable := hasUsableCaptionRoute(profile)
	keywordConfig := keywordRoute(profile)
	captionSearchEnabled := captionRouteAvailable && s.retrieval.Weights.Caption > 0
	keywordSearchEnabled := keywordConfig != nil && s.retrieval.Weights.Keyword > 0

	var captionResults []repository.SearchResult
	var captionErr error
	if captionSearchEnabled {
		captionQueryEmbedding, err := profile.Caption.Embedding.EmbedQuery(ctx, queryForEmbedding)
		if err != nil {
			return nil, fmt.Errorf("failed to generate caption route query embedding: %w", err)
		}
		captionResults, captionErr = profile.Caption.QdrantRepo.Search(ctx, captionQueryEmbedding, s.retrieval.CaptionTopK, filters)
		if captionErr != nil {
			logger.CtxWarn(ctx, "Profile caption search failed: profile=%s, error=%v", profileName, captionErr)
			captionResults = nil
		}
	}

	var keywordResults []repository.SearchResult
	var keywordErr error
	if keywordSearchEnabled {
		keywordResults, keywordErr = keywordConfig.QdrantRepo.SparseSearch(ctx, originalQuery, s.retrieval.CaptionTopK, filters)
		if keywordErr != nil {
			logger.CtxWarn(ctx, "Profile keyword search failed: profile=%s, error=%v", profileName, keywordErr)
			keywordResults = nil
		}
	}

	if imageErr != nil &&
		(!captionSearchEnabled || captionErr != nil) &&
		(!keywordSearchEnabled || keywordErr != nil) {
		return nil, fmt.Errorf("all profile search routes failed: image=%v, caption=%v, keyword=%v", imageErr, captionErr, keywordErr)
	}

	finalTopK := int(topK)
	if finalTopK <= 0 {
		finalTopK = s.retrieval.FinalTopK
	}
	results := fuseProfileResults(
		imageResults,
		captionResults,
		keywordResults,
		s.retrieval.Weights,
		finalTopK,
		shouldBoostWithTextResults(req, pb.TextPresence_TEXT_PRESENCE_UNSPECIFIED),
	)
	s.enrichSearchResults(ctx, results)

	return &pb.SearchResponse{
		Results:       results,
		Total:         int32(len(results)),
		Query:         originalQuery,
		ExpandedQuery: expandedQuery,
		Profile:       profileName,
	}, nil
}

func (s *SearchService) searchProfileAgentic(
	ctx context.Context,
	req *pb.SearchRequest,
	topK int32,
	profileName string,
	profile *SearchProfileConfig,
	originalQuery string,
	legacyFallback func() (*pb.SearchResponse, error),
) (*pb.SearchResponse, error) {
	plan, ok, err := s.planAgenticProfileSearch(ctx, req, profileName, originalQuery)
	if err != nil {
		return nil, err
	}
	if !ok {
		if legacyFallback == nil {
			return nil, fmt.Errorf("agentic search planner failed and no legacy fallback is configured")
		}
		return legacyFallback()
	}

	return s.searchProfileWithPlan(ctx, req, topK, profileName, profile, originalQuery, plan)
}

func (s *SearchService) planAgenticProfileSearch(
	ctx context.Context,
	req *pb.SearchRequest,
	profileName string,
	originalQuery string,
) (SearchPlan, bool, error) {
	plan, err := s.agentic.planner.Plan(ctx, req, s.retrieval)
	if err != nil {
		logger.CtxWarn(ctx, "Agentic search planner failed: query=%q, profile=%s, error=%v",
			originalQuery, profileName, err)
		if !s.agentic.FallbackOnError() {
			return SearchPlan{}, false, fmt.Errorf("agentic search planner failed: %w", err)
		}
		logger.CtxInfo(ctx, "Falling back to legacy profile search after agentic planner failure: query=%q, profile=%s",
			originalQuery, profileName)
		return SearchPlan{}, false, nil
	}

	logger.CtxInfo(ctx, "Agentic search plan: query=%q, intent=%s, dense_query=%q, sparse_query=%q, weights=image:%.3f caption:%.3f keyword:%.3f, top_k=image:%d caption:%d keyword:%d, reason=%q",
		originalQuery, plan.Intent, plan.DenseQuery, plan.SparseQuery,
		plan.Weights.Image, plan.Weights.Caption, plan.Weights.Keyword,
		plan.ImageTopK, plan.CaptionTopK, plan.KeywordTopK, plan.Reason)
	return plan, true, nil
}

func (s *SearchService) searchProfileWithPlan(
	ctx context.Context,
	req *pb.SearchRequest,
	topK int32,
	profileName string,
	profile *SearchProfileConfig,
	originalQuery string,
	plan SearchPlan,
) (*pb.SearchResponse, error) {
	if !hasUsableImageRoute(profile) {
		return nil, fmt.Errorf("profile %q is incomplete", profileName)
	}

	logger.CtxInfo(ctx, "Performing agentic profile search: query=%q, dense_query=%q, sparse_query=%q, top_k=%d, profile=%s",
		originalQuery, plan.DenseQuery, plan.SparseQuery, topK, profileName)

	imageQueryEmbedding, err := profile.Image.Embedding.EmbedQuery(ctx, plan.DenseQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to generate image route query embedding: %w", err)
	}

	filters := buildSearchFiltersWithPlan(req, plan)

	imageResults, imageErr := profile.Image.QdrantRepo.Search(ctx, imageQueryEmbedding, plan.ImageTopK, filters)
	if imageErr != nil {
		logger.CtxWarn(ctx, "Agentic profile image search failed: profile=%s, error=%v", profileName, imageErr)
		imageResults = nil
	}

	captionRouteAvailable := hasUsableCaptionRoute(profile)
	keywordConfig := keywordRoute(profile)
	captionSearchEnabled := captionRouteAvailable && plan.Weights.Caption > 0
	keywordSearchEnabled := keywordConfig != nil && plan.Weights.Keyword > 0

	var captionResults []repository.SearchResult
	var captionErr error
	if captionSearchEnabled {
		captionQueryEmbedding, err := profile.Caption.Embedding.EmbedQuery(ctx, plan.DenseQuery)
		if err != nil {
			return nil, fmt.Errorf("failed to generate caption route query embedding: %w", err)
		}
		captionResults, captionErr = profile.Caption.QdrantRepo.Search(ctx, captionQueryEmbedding, plan.CaptionTopK, filters)
		if captionErr != nil {
			logger.CtxWarn(ctx, "Agentic profile caption search failed: profile=%s, error=%v", profileName, captionErr)
			captionResults = nil
		}
	}

	var keywordResults []repository.SearchResult
	var keywordErr error
	if keywordSearchEnabled {
		keywordResults, keywordErr = keywordConfig.QdrantRepo.SparseSearch(ctx, plan.SparseQuery, plan.KeywordTopK, filters)
		if keywordErr != nil {
			logger.CtxWarn(ctx, "Agentic profile keyword search failed: profile=%s, error=%v", profileName, keywordErr)
			keywordResults = nil
		}
	}

	if imageErr != nil &&
		(!captionSearchEnabled || captionErr != nil) &&
		(!keywordSearchEnabled || keywordErr != nil) {
		return nil, fmt.Errorf("all agentic profile search routes failed: image=%v, caption=%v, keyword=%v", imageErr, captionErr, keywordErr)
	}

	finalTopK := int(topK)
	if finalTopK <= 0 {
		finalTopK = s.retrieval.FinalTopK
	}
	candidateLimit := finalTopK
	if s.agentic != nil && s.agentic.RerankTopK() > candidateLimit {
		candidateLimit = s.agentic.RerankTopK()
	}
	candidates := fuseProfileCandidates(
		imageResults,
		captionResults,
		keywordResults,
		plan.Weights,
		candidateLimit,
		shouldBoostWithTextResults(req, plan.TextPresence),
	)
	logTopRouteEvidence(ctx, candidates, 10)

	if s.agentic != nil && s.agentic.reranker != nil && len(candidates) > 0 {
		reranked, err := s.agentic.reranker.Rerank(ctx, req, plan, candidates)
		if err != nil {
			logger.CtxWarn(ctx, "Agentic search reranker failed, keeping fusion order: query=%q, error=%v", originalQuery, err)
		} else {
			candidates = reranked
			logger.CtxInfo(ctx, "Agentic search reranked candidates: query=%q, kept=%d", originalQuery, len(candidates))
		}
	}

	results := candidatesToSearchResults(candidates, finalTopK)
	s.enrichSearchResults(ctx, results)

	return &pb.SearchResponse{
		Results:       results,
		Total:         int32(len(results)),
		Query:         originalQuery,
		ExpandedQuery: plan.DenseQuery,
		Profile:       profileName,
	}, nil
}

func buildSearchFiltersWithPlan(req *pb.SearchRequest, plan SearchPlan) *repository.SearchFilters {
	filters := buildSearchFilters(req)
	if filters == nil {
		filters = &repository.SearchFilters{}
	}
	if req.GetTextPresence() == pb.TextPresence_TEXT_PRESENCE_UNSPECIFIED &&
		plan.TextPresence != pb.TextPresence_TEXT_PRESENCE_UNSPECIFIED {
		presence := persistence.TextPresenceToString(plan.TextPresence)
		filters.TextPresence = &presence
	}
	if filters.Category == nil && filters.TextPresence == nil {
		return nil
	}
	return filters
}

func shouldBoostWithTextResults(req *pb.SearchRequest, plannedTextPresence pb.TextPresence) bool {
	return req.GetTextPresence() == pb.TextPresence_TEXT_PRESENCE_UNSPECIFIED &&
		plannedTextPresence == pb.TextPresence_TEXT_PRESENCE_UNSPECIFIED
}

func logTopRouteEvidence(ctx context.Context, candidates []SearchCandidate, limit int) {
	if limit <= 0 || len(candidates) == 0 {
		return
	}
	if len(candidates) < limit {
		limit = len(candidates)
	}
	for i := 0; i < limit; i++ {
		candidate := candidates[i]
		if candidate.Result == nil || candidate.Result.Meme == nil {
			continue
		}
		logger.CtxDebug(ctx, "Agentic search candidate evidence: rank=%d, meme_id=%s, score=%.4f, image_rank=%d, caption_rank=%d, keyword_rank=%d",
			i+1, candidate.Result.Meme.GetId(), candidate.Result.GetScore(),
			candidate.Evidence.ImageRank, candidate.Evidence.CaptionRank, candidate.Evidence.KeywordRank)
	}
}

type routeResults struct {
	results []repository.SearchResult
	weight  float32
	route   string
}

// RouteEvidence records where a candidate appeared before cross-route fusion.
type RouteEvidence struct {
	ImageRank    int     `json:"image_rank"`
	ImageScore   float32 `json:"image_score"`
	CaptionRank  int     `json:"caption_rank"`
	CaptionScore float32 `json:"caption_score"`
	KeywordRank  int     `json:"keyword_rank"`
	KeywordScore float32 `json:"keyword_score"`
}

// SearchCandidate is the internal representation used by agentic reranking.
type SearchCandidate struct {
	Result         *pb.SearchResult
	Evidence       RouteEvidence
	FusionScore    float32
	PayloadOCRText string
	RerankReason   string
}

func fuseProfileResults(
	imageResults []repository.SearchResult,
	captionResults []repository.SearchResult,
	keywordResults []repository.SearchResult,
	weights RetrievalWeights,
	topK int,
	boostWithTextResults bool,
) []*pb.SearchResult {
	candidates := fuseProfileCandidates(imageResults, captionResults, keywordResults, weights, topK, boostWithTextResults)
	return candidatesToSearchResults(candidates, topK)
}

func fuseProfileCandidates(
	imageResults []repository.SearchResult,
	captionResults []repository.SearchResult,
	keywordResults []repository.SearchResult,
	weights RetrievalWeights,
	topK int,
	boostWithTextResults bool,
) []SearchCandidate {
	if topK <= 0 {
		topK = 20
	}
	routes := []routeResults{
		{results: imageResults, weight: weights.Image, route: "image"},
		{results: captionResults, weight: weights.Caption, route: "caption"},
		{results: keywordResults, weight: weights.Keyword, route: "keyword"},
	}

	byMemeID := make(map[string]*SearchCandidate)
	for _, route := range routes {
		if route.weight <= 0 {
			continue
		}
		seenInRoute := make(map[string]struct{}, len(route.results))
		for rank, qr := range route.results {
			if qr.Payload == nil || qr.Payload.MemeID == "" {
				continue
			}
			if _, seen := seenInRoute[qr.Payload.MemeID]; seen {
				continue
			}
			seenInRoute[qr.Payload.MemeID] = struct{}{}
			rankScore := route.weight * (1 / float32(rank+60))
			item, ok := byMemeID[qr.Payload.MemeID]
			if !ok {
				item = &SearchCandidate{
					Result: &pb.SearchResult{
						Meme: &pb.Meme{
							Id:       qr.Payload.MemeID,
							Url:      qr.Payload.StorageURL,
							Category: qr.Payload.Category,
							Tags:     qr.Payload.Tags,
						},
						Description:  qr.Payload.VLMDescription,
						TextPresence: persistence.TextPresenceFromString(qr.Payload.TextPresence),
					},
					PayloadOCRText: qr.Payload.OCRText,
				}
				byMemeID[qr.Payload.MemeID] = item
			}
			applyRouteEvidence(item, route.route, rank+1, qr.Score, qr.Payload)
			item.FusionScore += rankScore
		}
	}

	maxScore := float32(0)
	for _, item := range byMemeID {
		if boostWithTextResults && item.Result.GetTextPresence() == pb.TextPresence_TEXT_PRESENCE_WITH_TEXT {
			item.FusionScore *= withTextResultBoost
		}
		if item.FusionScore > maxScore {
			maxScore = item.FusionScore
		}
	}

	items := make([]SearchCandidate, 0, len(byMemeID))
	for _, item := range byMemeID {
		if maxScore > 0 {
			item.Result.Score = item.FusionScore / maxScore
			item.FusionScore = item.Result.Score
		}
		items = append(items, *item)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Result.Score == items[j].Result.Score {
			return items[i].Result.Meme.GetId() < items[j].Result.Meme.GetId()
		}
		return items[i].Result.Score > items[j].Result.Score
	})

	if len(items) > topK {
		items = items[:topK]
	}

	return items
}

func boostAndSortWithTextResults(results []*pb.SearchResult) {
	for _, result := range results {
		if result.GetTextPresence() == pb.TextPresence_TEXT_PRESENCE_WITH_TEXT {
			result.Score *= withTextResultBoost
		}
	}
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].GetScore() == results[j].GetScore() {
			return results[i].GetMeme().GetId() < results[j].GetMeme().GetId()
		}
		return results[i].GetScore() > results[j].GetScore()
	})
}

func applyRouteEvidence(candidate *SearchCandidate, route string, rank int, score float32, payload *repository.MemePayload) {
	if candidate == nil {
		return
	}
	if candidate.PayloadOCRText == "" && payload != nil {
		candidate.PayloadOCRText = payload.OCRText
	}
	switch route {
	case "image":
		candidate.Evidence.ImageRank = rank
		candidate.Evidence.ImageScore = score
	case "caption":
		candidate.Evidence.CaptionRank = rank
		candidate.Evidence.CaptionScore = score
	case "keyword":
		candidate.Evidence.KeywordRank = rank
		candidate.Evidence.KeywordScore = score
	}
}

func candidatesToSearchResults(candidates []SearchCandidate, topK int) []*pb.SearchResult {
	if topK <= 0 {
		topK = len(candidates)
	}
	if len(candidates) > topK {
		candidates = candidates[:topK]
	}
	results := make([]*pb.SearchResult, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Result != nil {
			results = append(results, candidate.Result)
		}
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
	agenticEnabled := s.agentic.IsEnabled()
	ctx = withSearchLogFields(ctx)

	if profile, profileName, ok, err := s.resolveRequestedProfile(req); err != nil {
		return nil, err
	} else if ok {
		if agenticEnabled {
			plan, planned, err := s.planAgenticProfileSearch(ctx, req, profileName, originalQuery)
			if err != nil {
				return nil, err
			}
			if planned {
				progressCh <- &pb.SearchProgressEvent{
					Stage:   pb.SearchStage_SEARCH_STAGE_EMBEDDING,
					Message: "正在生成语义向量...",
				}
				progressCh <- &pb.SearchProgressEvent{
					Stage:   pb.SearchStage_SEARCH_STAGE_SEARCHING,
					Message: "在表情库中搜索...",
				}
				result, err := s.searchProfileWithPlan(ctx, req, topK, profileName, profile, originalQuery, plan)
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
		}

		queryForEmbedding, expandedQuery := s.prepareLegacyQueryWithProgress(ctx, originalQuery, route, progressCh)
		progressCh <- &pb.SearchProgressEvent{
			Stage:   pb.SearchStage_SEARCH_STAGE_EMBEDDING,
			Message: "正在生成语义向量...",
		}
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

	queryForEmbedding, expandedQuery := s.prepareLegacyQueryWithProgress(ctx, originalQuery, route, progressCh)
	progressCh <- &pb.SearchProgressEvent{
		Stage:   pb.SearchStage_SEARCH_STAGE_EMBEDDING,
		Message: "正在生成语义向量...",
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

	results := s.buildSearchResultsFromQdrant(qdrantResults, usingHybrid, int(topK), shouldBoostWithTextResults(req, pb.TextPresence_TEXT_PRESENCE_UNSPECIFIED))
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
