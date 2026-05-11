package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	pb "github.com/timmy/emomo/gen/emomo/v1"
)

const (
	defaultAgenticRouteTopK = 100
	maxAgenticRouteTopK     = 200
)

const searchPlannerPrompt = `你是 emomo 表情包检索规划器。你只输出 JSON，不要 markdown。

根据用户查询生成检索计划。系统有三路召回：
- image: 文本 query 到多模态图片向量，适合主体、动作、情绪、场景。
- caption: 文本 query 到 OCR/VLM caption 向量，适合描述、情绪、场景、主体补充。
- keyword: 原始关键词到 OCR/VLM BM25 sparse，适合图中文字、短梗、精确词。

输出 JSON 字段：
{
  "intent": "ocr|emotion|subject|scene|exact|mixed",
  "dense_query": "用于 image/caption dense embedding 的中文查询",
  "sparse_query": "用于 BM25 的关键词查询",
  "must_terms": ["必须匹配的词"],
  "negative_terms": ["明显不想要的词"],
  "text_presence": "unspecified|with_text|without_text|unknown",
  "route_weights": {"image": 0.6, "caption": 0.3, "keyword": 0.1},
  "top_k": {"image": 100, "caption": 100, "keyword": 100},
  "reason": "一句话说明计划"
}

不要让 dense_query 引入用户没有表达的具体主体。`

// SearchPlan is the validated internal plan used to execute profile search.
type SearchPlan struct {
	Intent        string
	DenseQuery    string
	SparseQuery   string
	MustTerms     []string
	NegativeTerms []string
	TextPresence  pb.TextPresence
	Weights       RetrievalWeights
	ImageTopK     int
	CaptionTopK   int
	KeywordTopK   int
	Reason        string
}

// SearchPlanner produces an executable search plan.
type SearchPlanner interface {
	Plan(ctx context.Context, req *pb.SearchRequest, defaults RetrievalConfig) (SearchPlan, error)
}

// LLMSearchPlannerConfig controls planner generation.
type LLMSearchPlannerConfig struct {
	Timeout     time.Duration
	MaxTokens   int
	Temperature float32
}

// LLMSearchPlanner asks an LLM for a structured search plan.
type LLMSearchPlanner struct {
	client LLMJSONClient
	cfg    LLMSearchPlannerConfig
}

// NewLLMSearchPlanner creates an LLM-backed planner.
func NewLLMSearchPlanner(client LLMJSONClient, cfg LLMSearchPlannerConfig) *LLMSearchPlanner {
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = 900
	}
	return &LLMSearchPlanner{client: client, cfg: cfg}
}

type searchPlanJSON struct {
	Intent        string   `json:"intent"`
	DenseQuery    string   `json:"dense_query"`
	SparseQuery   string   `json:"sparse_query"`
	MustTerms     []string `json:"must_terms"`
	NegativeTerms []string `json:"negative_terms"`
	TextPresence  string   `json:"text_presence"`
	RouteWeights  struct {
		Image   float32 `json:"image"`
		Caption float32 `json:"caption"`
		Keyword float32 `json:"keyword"`
	} `json:"route_weights"`
	TopK struct {
		Image   int `json:"image"`
		Caption int `json:"caption"`
		Keyword int `json:"keyword"`
	} `json:"top_k"`
	Reason string `json:"reason"`
}

// Plan returns a normalized SearchPlan or an error that callers can fallback from.
func (p *LLMSearchPlanner) Plan(ctx context.Context, req *pb.SearchRequest, defaults RetrievalConfig) (SearchPlan, error) {
	if p == nil || p.client == nil {
		return SearchPlan{}, fmt.Errorf("search planner is not configured")
	}
	query := strings.TrimSpace(req.GetQuery())
	payload := map[string]any{
		"query":         query,
		"top_k":         req.GetTopK(),
		"category":      req.GetCategory(),
		"text_presence": req.GetTextPresence().String(),
	}
	userPayload, err := json.Marshal(payload)
	if err != nil {
		return SearchPlan{}, fmt.Errorf("failed to marshal planner payload: %w", err)
	}
	raw, err := p.client.Complete(ctx, LLMJSONRequest{
		SystemPrompt: searchPlannerPrompt,
		UserPrompt:   string(userPayload),
		MaxTokens:    p.cfg.MaxTokens,
		Temperature:  p.cfg.Temperature,
		Timeout:      p.cfg.Timeout,
	})
	if err != nil {
		return SearchPlan{}, err
	}

	var parsed searchPlanJSON
	if err := json.Unmarshal([]byte(extractJSONObject(raw)), &parsed); err != nil {
		return SearchPlan{}, fmt.Errorf("failed to parse search plan JSON: %w", err)
	}
	return normalizeSearchPlan(parsed, query, defaults), nil
}

func normalizeSearchPlan(raw searchPlanJSON, query string, defaults RetrievalConfig) SearchPlan {
	defaults = normalizeRetrievalConfig(defaults)
	plan := SearchPlan{
		Intent:        normalizePlanIntent(raw.Intent),
		DenseQuery:    strings.TrimSpace(raw.DenseQuery),
		SparseQuery:   strings.TrimSpace(raw.SparseQuery),
		MustTerms:     dedupeStrings(raw.MustTerms),
		NegativeTerms: dedupeStrings(raw.NegativeTerms),
		TextPresence:  parsePlanTextPresence(raw.TextPresence),
		Weights:       normalizePlanWeights(RetrievalWeights{Image: raw.RouteWeights.Image, Caption: raw.RouteWeights.Caption, Keyword: raw.RouteWeights.Keyword}, defaults.Weights),
		ImageTopK:     normalizePlanTopK(raw.TopK.Image, defaults.ImageTopK),
		CaptionTopK:   normalizePlanTopK(raw.TopK.Caption, defaults.CaptionTopK),
		KeywordTopK:   normalizePlanTopK(raw.TopK.Keyword, defaults.CaptionTopK),
		Reason:        strings.TrimSpace(raw.Reason),
	}
	if plan.DenseQuery == "" {
		plan.DenseQuery = query
	}
	if plan.SparseQuery == "" {
		plan.SparseQuery = query
	}
	return plan
}

func normalizePlanIntent(intent string) string {
	switch strings.ToLower(strings.TrimSpace(intent)) {
	case "ocr", "emotion", "subject", "scene", "exact", "mixed":
		return strings.ToLower(strings.TrimSpace(intent))
	default:
		return "mixed"
	}
}

func parsePlanTextPresence(value string) pb.TextPresence {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "with_text", "with text", "text", "has_text":
		return pb.TextPresence_TEXT_PRESENCE_WITH_TEXT
	case "without_text", "without text", "no_text", "no text":
		return pb.TextPresence_TEXT_PRESENCE_WITHOUT_TEXT
	case "unknown":
		return pb.TextPresence_TEXT_PRESENCE_UNKNOWN
	default:
		return pb.TextPresence_TEXT_PRESENCE_UNSPECIFIED
	}
}

func normalizePlanWeights(weights RetrievalWeights, fallback RetrievalWeights) RetrievalWeights {
	if weights.Image < 0 {
		weights.Image = 0
	}
	if weights.Caption < 0 {
		weights.Caption = 0
	}
	if weights.Keyword < 0 {
		weights.Keyword = 0
	}
	sum := weights.Image + weights.Caption + weights.Keyword
	if sum == 0 {
		return fallback
	}
	return RetrievalWeights{
		Image:   weights.Image / sum,
		Caption: weights.Caption / sum,
		Keyword: weights.Keyword / sum,
	}
}

func normalizePlanTopK(value int, fallback int) int {
	if fallback <= 0 {
		fallback = defaultAgenticRouteTopK
	}
	if value <= 0 {
		value = fallback
	}
	if value > maxAgenticRouteTopK {
		return maxAgenticRouteTopK
	}
	return value
}

func extractJSONObject(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "```") {
		raw = strings.TrimPrefix(raw, "```json")
		raw = strings.TrimPrefix(raw, "```")
		raw = strings.TrimSuffix(raw, "```")
		raw = strings.TrimSpace(raw)
	}
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start >= 0 && end >= start {
		return raw[start : end+1]
	}
	return raw
}
