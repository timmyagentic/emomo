package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	pb "github.com/timmy/emomo/gen/emomo/v1"
)

const searchRerankerPrompt = `你是 emomo 表情包搜索重排器。你只输出 JSON，不要 markdown。

根据用户查询、检索计划和候选证据，判断候选是否相关并排序。优先满足 must_terms、OCR 精确匹配、text_presence、主体/情绪/场景一致性。不要虚构图片内容，只能依据 description、ocr_text、route_evidence。

输出 JSON：
{
  "ranked": [
    {"meme_id": "...", "relevance": 0.95, "keep": true, "reason": "简短理由"}
  ]
}`

// SearchReranker reranks already-retrieved candidates.
type SearchReranker interface {
	Rerank(ctx context.Context, req *pb.SearchRequest, plan SearchPlan, candidates []SearchCandidate) ([]SearchCandidate, error)
}

// LLMSearchRerankerConfig controls LLM reranking.
type LLMSearchRerankerConfig struct {
	TopK        int
	Timeout     time.Duration
	MaxTokens   int
	Temperature float32
}

// LLMSearchReranker asks an LLM to rerank top candidates using text evidence.
type LLMSearchReranker struct {
	client LLMJSONClient
	cfg    LLMSearchRerankerConfig
}

// NewLLMSearchReranker creates an LLM-backed reranker.
func NewLLMSearchReranker(client LLMJSONClient, cfg LLMSearchRerankerConfig) *LLMSearchReranker {
	if cfg.TopK <= 0 {
		cfg.TopK = 40
	}
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = 1600
	}
	return &LLMSearchReranker{client: client, cfg: cfg}
}

type rerankResponseJSON struct {
	Ranked []struct {
		MemeID    string  `json:"meme_id"`
		Relevance float32 `json:"relevance"`
		Keep      *bool   `json:"keep"`
		Reason    string  `json:"reason"`
	} `json:"ranked"`
}

type rerankCandidateJSON struct {
	MemeID        string        `json:"meme_id"`
	Score         float32       `json:"score"`
	Description   string        `json:"description"`
	OCRText       string        `json:"ocr_text"`
	TextPresence  string        `json:"text_presence"`
	RouteEvidence RouteEvidence `json:"route_evidence"`
}

// Rerank returns candidates in reranked order. On parse/client error it returns
// the original candidate order alongside the error so callers can fallback.
func (r *LLMSearchReranker) Rerank(ctx context.Context, req *pb.SearchRequest, plan SearchPlan, candidates []SearchCandidate) ([]SearchCandidate, error) {
	if len(candidates) == 0 {
		return candidates, nil
	}
	if r == nil || r.client == nil {
		return candidates, fmt.Errorf("search reranker is not configured")
	}

	limit := r.cfg.TopK
	if limit <= 0 || limit > len(candidates) {
		limit = len(candidates)
	}
	payload := map[string]any{
		"query":      req.GetQuery(),
		"plan":       plan.toPromptPayload(),
		"candidates": candidatesToRerankPayload(candidates[:limit]),
	}
	userPayload, err := json.Marshal(payload)
	if err != nil {
		return candidates, fmt.Errorf("failed to marshal reranker payload: %w", err)
	}
	raw, err := r.client.Complete(ctx, LLMJSONRequest{
		SystemPrompt: searchRerankerPrompt,
		UserPrompt:   string(userPayload),
		MaxTokens:    r.cfg.MaxTokens,
		Temperature:  r.cfg.Temperature,
		Timeout:      r.cfg.Timeout,
	})
	if err != nil {
		return candidates, err
	}

	var parsed rerankResponseJSON
	if err := json.Unmarshal([]byte(extractJSONObject(raw)), &parsed); err != nil {
		return candidates, fmt.Errorf("failed to parse reranker JSON: %w", err)
	}
	reranked, complete := applyRerankResponse(candidates, limit, parsed)
	if !complete {
		return candidates, fmt.Errorf("reranker output omitted one or more candidates")
	}
	return reranked, nil
}

func candidatesToRerankPayload(candidates []SearchCandidate) []rerankCandidateJSON {
	out := make([]rerankCandidateJSON, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Result == nil || candidate.Result.Meme == nil {
			continue
		}
		out = append(out, rerankCandidateJSON{
			MemeID:        candidate.Result.Meme.GetId(),
			Score:         candidate.Result.GetScore(),
			Description:   candidate.Result.GetDescription(),
			OCRText:       candidate.PayloadOCRText,
			TextPresence:  candidate.Result.GetTextPresence().String(),
			RouteEvidence: candidate.Evidence,
		})
	}
	return out
}

func applyRerankResponse(candidates []SearchCandidate, rerankLimit int, response rerankResponseJSON) ([]SearchCandidate, bool) {
	byID := make(map[string]SearchCandidate, len(candidates))
	expected := make(map[string]struct{}, len(candidates))
	kept := make(map[string]struct{}, len(candidates))
	dropped := make(map[string]struct{}, len(candidates))
	if rerankLimit <= 0 || rerankLimit > len(candidates) {
		rerankLimit = len(candidates)
	}
	for i, candidate := range candidates {
		if candidate.Result == nil || candidate.Result.Meme == nil {
			continue
		}
		id := candidate.Result.Meme.GetId()
		if i < rerankLimit {
			byID[id] = candidate
			expected[id] = struct{}{}
		}
	}

	reranked := make([]SearchCandidate, 0, len(candidates))
	for _, item := range response.Ranked {
		id := strings.TrimSpace(item.MemeID)
		candidate, ok := byID[id]
		if !ok {
			continue
		}
		keep := true
		if item.Keep != nil {
			keep = *item.Keep
		}
		if _, ok := expected[id]; ok {
			delete(expected, id)
		}
		if !keep {
			dropped[id] = struct{}{}
			continue
		}
		if _, ok := kept[id]; ok {
			continue
		}
		if item.Relevance > 0 {
			candidate.Result.Score = item.Relevance
			candidate.FusionScore = item.Relevance
		}
		candidate.RerankReason = strings.TrimSpace(item.Reason)
		reranked = append(reranked, candidate)
		kept[id] = struct{}{}
	}
	if len(expected) > 0 {
		return candidates, false
	}

	for _, candidate := range candidates {
		if candidate.Result == nil || candidate.Result.Meme == nil {
			continue
		}
		id := candidate.Result.Meme.GetId()
		if _, ok := kept[id]; ok {
			continue
		}
		if _, ok := dropped[id]; ok {
			continue
		}
		reranked = append(reranked, candidate)
	}
	return reranked, true
}

func (p SearchPlan) toPromptPayload() map[string]any {
	return map[string]any{
		"intent":         p.Intent,
		"dense_query":    p.DenseQuery,
		"sparse_query":   p.SparseQuery,
		"must_terms":     p.MustTerms,
		"negative_terms": p.NegativeTerms,
		"text_presence":  p.TextPresence.String(),
		"route_weights": map[string]float32{
			"image":   p.Weights.Image,
			"caption": p.Weights.Caption,
			"keyword": p.Weights.Keyword,
		},
		"reason": p.Reason,
	}
}
