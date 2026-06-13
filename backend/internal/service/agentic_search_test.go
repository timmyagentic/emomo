package service

import (
	"context"
	"errors"
	"reflect"
	"testing"

	pb "github.com/timmy/emomo/gen/emomo/v1"
	"github.com/timmy/emomo/internal/repository"
)

type fakeLLMJSONClient struct {
	response string
	err      error
}

func (f fakeLLMJSONClient) Complete(ctx context.Context, req LLMJSONRequest) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.response, nil
}

type fakeSearchPlanner struct {
	plan SearchPlan
	err  error
}

func (f fakeSearchPlanner) Plan(context.Context, *pb.SearchRequest, RetrievalConfig) (SearchPlan, error) {
	if f.err != nil {
		return SearchPlan{}, f.err
	}
	return f.plan, nil
}

type unexpectedSearchReranker struct{}

func (unexpectedSearchReranker) Rerank(context.Context, *pb.SearchRequest, SearchPlan, []SearchCandidate) ([]SearchCandidate, error) {
	return nil, errors.New("reranker should not be called during planner fallback")
}

func TestSearchProfileAgenticPlannerFailureFallsBackToLegacySearch(t *testing.T) {
	t.Parallel()

	searchService := &SearchService{
		retrieval: defaultRetrievalConfig(),
		agentic: NewAgenticSearchService(
			fakeSearchPlanner{err: errors.New("planner unavailable")},
			unexpectedSearchReranker{},
			AgenticSearchConfig{Enabled: true, FallbackOnError: true},
		),
	}
	legacyCalled := false
	legacyResponse := &pb.SearchResponse{
		Query:         "我不理解",
		ExpandedQuery: "我不理解 疑惑 无语 表情包",
		Profile:       "qwen3vl",
	}

	resp, err := searchService.searchProfileAgentic(
		context.Background(),
		&pb.SearchRequest{Query: "我不理解"},
		10,
		"qwen3vl",
		nil,
		"我不理解",
		func() (*pb.SearchResponse, error) {
			legacyCalled = true
			return legacyResponse, nil
		},
	)
	if err != nil {
		t.Fatalf("searchProfileAgentic() error = %v, want nil", err)
	}
	if !legacyCalled {
		t.Fatal("legacy fallback was not called")
	}
	if resp != legacyResponse {
		t.Fatalf("response = %#v, want legacy response", resp)
	}
}

func TestLLMSearchPlannerNormalizesPlanJSON(t *testing.T) {
	t.Parallel()

	planner := NewLLMSearchPlanner(fakeLLMJSONClient{response: `{
		"intent":"ocr",
		"dense_query":"疑惑 无语 表情包",
		"sparse_query":"我不理解",
		"must_terms":["我不理解"],
		"negative_terms":["开心"],
		"text_presence":"with_text",
		"route_weights":{"image":2,"caption":1,"keyword":1},
		"top_k":{"image":300,"caption":0,"keyword":5},
		"reason":"OCR exact match"
	}`}, LLMSearchPlannerConfig{})

	plan, err := planner.Plan(context.Background(), &pb.SearchRequest{Query: "我不理解"}, defaultRetrievalConfig())
	if err != nil {
		t.Fatalf("Plan() error = %v, want nil", err)
	}

	if plan.Intent != "ocr" {
		t.Fatalf("Intent = %q, want ocr", plan.Intent)
	}
	if plan.DenseQuery != "疑惑 无语 表情包" {
		t.Fatalf("DenseQuery = %q", plan.DenseQuery)
	}
	if plan.SparseQuery != "我不理解" {
		t.Fatalf("SparseQuery = %q", plan.SparseQuery)
	}
	if plan.TextPresence != pb.TextPresence_TEXT_PRESENCE_WITH_TEXT {
		t.Fatalf("TextPresence = %v, want WITH_TEXT", plan.TextPresence)
	}
	if got, want := plan.MustTerms, []string{"我不理解"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("MustTerms = %v, want %v", got, want)
	}
	if plan.Weights.Image != 0.5 || plan.Weights.Caption != 0.25 || plan.Weights.Keyword != 0.25 {
		t.Fatalf("Weights = %+v, want normalized 0.5/0.25/0.25", plan.Weights)
	}
	if plan.ImageTopK != 200 {
		t.Fatalf("ImageTopK = %d, want clamp to 200", plan.ImageTopK)
	}
	if plan.CaptionTopK != 100 {
		t.Fatalf("CaptionTopK = %d, want default 100", plan.CaptionTopK)
	}
	if plan.KeywordTopK != 5 {
		t.Fatalf("KeywordTopK = %d, want 5", plan.KeywordTopK)
	}
}

func TestLLMSearchPlannerReturnsErrorForInvalidJSON(t *testing.T) {
	t.Parallel()

	planner := NewLLMSearchPlanner(fakeLLMJSONClient{response: `not-json`}, LLMSearchPlannerConfig{})

	if _, err := planner.Plan(context.Background(), &pb.SearchRequest{Query: "无语"}, defaultRetrievalConfig()); err == nil {
		t.Fatal("Plan() error = nil, want parse error")
	}
}

func TestLLMSearchPlannerReturnsErrorFromClient(t *testing.T) {
	t.Parallel()

	planner := NewLLMSearchPlanner(fakeLLMJSONClient{err: errors.New("llm unavailable")}, LLMSearchPlannerConfig{})

	if _, err := planner.Plan(context.Background(), &pb.SearchRequest{Query: "无语"}, defaultRetrievalConfig()); err == nil {
		t.Fatal("Plan() error = nil, want client error")
	}
}

func TestFuseProfileCandidatesPreservesRouteEvidenceAndUsesDynamicWeights(t *testing.T) {
	t.Parallel()

	imageResults := []repository.SearchResult{
		{ID: "point-image-1", Score: 0.90, Payload: &repository.MemePayload{MemeID: "meme-a", StorageURL: "a.jpg"}},
	}
	captionResults := []repository.SearchResult{
		{ID: "point-caption-1", Score: 0.80, Payload: &repository.MemePayload{MemeID: "meme-b", StorageURL: "b.jpg"}},
	}
	keywordResults := []repository.SearchResult{
		{ID: "point-keyword-1", Score: 0.70, Payload: &repository.MemePayload{MemeID: "meme-b", StorageURL: "b.jpg"}},
	}

	candidates := fuseProfileCandidates(imageResults, captionResults, keywordResults, RetrievalWeights{
		Image:   0.1,
		Caption: 0.2,
		Keyword: 0.7,
	}, 20, false)

	if len(candidates) != 2 {
		t.Fatalf("fuseProfileCandidates returned %d candidates, want 2", len(candidates))
	}
	if got := candidates[0].Result.GetMeme().GetId(); got != "meme-b" {
		t.Fatalf("first candidate = %q, want meme-b due keyword-heavy weight", got)
	}
	if candidates[0].Evidence.CaptionRank != 1 || candidates[0].Evidence.KeywordRank != 1 {
		t.Fatalf("route evidence = %+v, want caption and keyword rank 1", candidates[0].Evidence)
	}
	if candidates[0].Evidence.ImageRank != 0 {
		t.Fatalf("image rank = %d, want 0 for missing image route", candidates[0].Evidence.ImageRank)
	}
}

func TestLLMSearchRerankerAppliesRankingAndFiltering(t *testing.T) {
	t.Parallel()

	candidates := []SearchCandidate{
		{Result: &pb.SearchResult{Meme: &pb.Meme{Id: "meme-a"}, Score: 0.8}},
		{Result: &pb.SearchResult{Meme: &pb.Meme{Id: "meme-b"}, Score: 0.7}},
		{Result: &pb.SearchResult{Meme: &pb.Meme{Id: "meme-c"}, Score: 0.6}},
	}
	reranker := NewLLMSearchReranker(fakeLLMJSONClient{response: `{
		"ranked":[
			{"meme_id":"meme-b","relevance":0.95,"keep":true,"reason":"best"},
			{"meme_id":"meme-c","relevance":0.7,"keep":true,"reason":"ok"},
			{"meme_id":"missing","relevance":0.9,"keep":true,"reason":"ignore"},
			{"meme_id":"meme-a","relevance":0.2,"keep":false,"reason":"wrong text"}
		]
	}`}, LLMSearchRerankerConfig{TopK: 3})

	reranked, err := reranker.Rerank(context.Background(), &pb.SearchRequest{Query: "我不理解"}, SearchPlan{DenseQuery: "我不理解"}, candidates)
	if err != nil {
		t.Fatalf("Rerank() error = %v, want nil", err)
	}

	if len(reranked) != 2 {
		t.Fatalf("Rerank returned %d candidates, want kept ranked candidates", len(reranked))
	}
	if got := reranked[0].Result.GetMeme().GetId(); got != "meme-b" {
		t.Fatalf("first reranked candidate = %q, want meme-b", got)
	}
	if got := reranked[1].Result.GetMeme().GetId(); got != "meme-c" {
		t.Fatalf("second reranked candidate = %q, want meme-c", got)
	}
}

func TestLLMSearchRerankerPartialOutputKeepsOriginalOrder(t *testing.T) {
	t.Parallel()

	candidates := []SearchCandidate{
		{Result: &pb.SearchResult{Meme: &pb.Meme{Id: "meme-a"}, Score: 0.8}},
		{Result: &pb.SearchResult{Meme: &pb.Meme{Id: "meme-b"}, Score: 0.7}},
		{Result: &pb.SearchResult{Meme: &pb.Meme{Id: "meme-c"}, Score: 0.6}},
	}
	reranker := NewLLMSearchReranker(fakeLLMJSONClient{response: `{
		"ranked":[
			{"meme_id":"meme-b","relevance":0.95,"keep":true,"reason":"only one returned"}
		]
	}`}, LLMSearchRerankerConfig{TopK: 3})

	reranked, err := reranker.Rerank(context.Background(), &pb.SearchRequest{Query: "我不理解"}, SearchPlan{DenseQuery: "我不理解"}, candidates)
	if err == nil {
		t.Fatal("Rerank() error = nil, want incomplete output error")
	}

	for i, want := range []string{"meme-a", "meme-b", "meme-c"} {
		if got := reranked[i].Result.GetMeme().GetId(); got != want {
			t.Fatalf("reranked[%d] = %q, want original order %q", i, got, want)
		}
	}
}

func TestLLMSearchRerankerInvalidJSONKeepsOriginalOrder(t *testing.T) {
	t.Parallel()

	candidates := []SearchCandidate{
		{Result: &pb.SearchResult{Meme: &pb.Meme{Id: "meme-a"}, Score: 0.8}},
		{Result: &pb.SearchResult{Meme: &pb.Meme{Id: "meme-b"}, Score: 0.7}},
	}
	reranker := NewLLMSearchReranker(fakeLLMJSONClient{response: `not-json`}, LLMSearchRerankerConfig{TopK: 2})

	reranked, err := reranker.Rerank(context.Background(), &pb.SearchRequest{Query: "无语"}, SearchPlan{DenseQuery: "无语"}, candidates)
	if err == nil {
		t.Fatal("Rerank() error = nil, want parse error")
	}
	if got := reranked[0].Result.GetMeme().GetId(); got != "meme-a" {
		t.Fatalf("first candidate after failed rerank = %q, want original meme-a", got)
	}
}
