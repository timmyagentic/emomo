package service

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-resty/resty/v2"
)

const (
	// Query Expansion Prompt - 与 vlm.go 词汇表保持同步
	queryExpansionPrompt = `你是表情包搜索查询扩展器。将用户的简短查询扩展为语义丰富的描述，提高向量搜索匹配度。

【核心原则】
- 保留原始意图，添加同义词、情绪词和场景描述
- 输出50-80字自然描述，直接输出文本，无需任何前缀

【情绪词库】
无语/尴尬/开心/暴怒/委屈/嫌弃/震惊/疑惑/得意/摆烂/emo/社死/破防/裂开/绝望/狂喜/阴阳怪气/幸灾乐祸/无奈/崩溃/感动/害怕/可爱/呆萌/嘲讽/鄙视/期待/失望

【网络梗】
芭比Q了(完蛋)/绝绝子(太绝)/yyds(永远的神)/栓Q(谢谢)/CPU(被PUA)/emo(低落)/摆烂(放弃)/社死(社会性死亡)/破防(崩溃)/一整个xx住/蚌埠住了/绷不住/DNA动了

【主体类型】
熊猫头/蘑菇头/柴犬/猫咪/兔子/小黄人/派大星/海绵宝宝

【示例】
输入: 无语
输出: 无语、无奈、嫌弃的情绪，翻白眼、面无表情、一脸嫌弃的样子，对某事无话可说不想理会，可能是熊猫头或蘑菇头表情包

输入: 熊猫头
输出: 熊猫头表情包，经典黑白熊猫脸，圆圆的脑袋配各种搞怪表情，可表达无语、开心、疑惑、震惊、嫌弃等多种情绪

输入: 芭比Q了
输出: 完蛋了、糟糕了、大事不妙，芭比Q网络流行语表示完蛋，惊恐绝望崩溃的表情，事情搞砸了要完蛋了

输入: 好耶
输出: 开心、兴奋、欢呼雀跃，好耶表示非常高兴激动，手舞足蹈眉开眼笑庆祝的样子，可爱得意满足

输入: 累了毁灭吧
输出: 疲惫、emo、摆烂、放弃挣扎，累到不想动想要毁灭世界，瘫倒无力眼神空洞，彻底破防不想努力了`
)

// QueryExpansionService handles query expansion using an LLM.
type QueryExpansionService struct {
	mu       sync.RWMutex
	client   *resty.Client
	model    string
	baseURL  string
	endpoint string
	apiKey   string
	enabled  bool
}

// QueryExpansionConfig holds configuration for query expansion service.
type QueryExpansionConfig struct {
	Enabled bool
	Model   string
	APIKey  string
	BaseURL string
}

// QueryExpansionSnapshot exposes non-secret runtime state for logs and tests.
type QueryExpansionSnapshot struct {
	Enabled  bool
	Model    string
	BaseURL  string
	Endpoint string
}

// NewQueryExpansionService creates a new query expansion service.
// Parameters:
//   - cfg: query expansion configuration (nil disables expansion).
//
// Returns:
//   - *QueryExpansionService: initialized service instance.
func NewQueryExpansionService(cfg *QueryExpansionConfig) *QueryExpansionService {
	s := &QueryExpansionService{}
	s.UpdateConfig(cfg)
	return s
}

// UpdateConfig replaces the query expansion runtime configuration.
// Parameters:
//   - cfg: query expansion configuration (nil disables expansion).
//
// Returns:
//   - bool: true when the effective runtime configuration changed.
func (s *QueryExpansionService) UpdateConfig(cfg *QueryExpansionConfig) bool {
	next := buildQueryExpansionRuntime(cfg)

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.enabled == next.enabled &&
		s.model == next.model &&
		s.baseURL == next.baseURL &&
		s.endpoint == next.endpoint &&
		s.apiKey == next.apiKey {
		return false
	}

	s.client = next.client
	s.model = next.model
	s.baseURL = next.baseURL
	s.endpoint = next.endpoint
	s.apiKey = next.apiKey
	s.enabled = next.enabled
	return true
}

type queryExpansionRuntime struct {
	client   *resty.Client
	model    string
	baseURL  string
	endpoint string
	apiKey   string
	enabled  bool
}

func buildQueryExpansionRuntime(cfg *QueryExpansionConfig) queryExpansionRuntime {
	if cfg == nil || !cfg.Enabled {
		return queryExpansionRuntime{}
	}

	client := resty.New()
	apiKey := strings.TrimSpace(cfg.APIKey)
	if apiKey != "" {
		client.SetHeader("Authorization", "Bearer "+apiKey)
	}
	client.SetHeader("Content-Type", "application/json")
	client.SetTimeout(30 * time.Second)

	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	baseURL = strings.TrimRight(baseURL, "/")
	endpoint := baseURL + "/chat/completions"

	return queryExpansionRuntime{
		client:   client,
		model:    strings.TrimSpace(cfg.Model),
		baseURL:  baseURL,
		endpoint: endpoint,
		apiKey:   apiKey,
		enabled:  true,
	}
}

func (s *QueryExpansionService) snapshot() queryExpansionRuntime {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return queryExpansionRuntime{
		client:   s.client,
		model:    s.model,
		baseURL:  s.baseURL,
		endpoint: s.endpoint,
		apiKey:   s.apiKey,
		enabled:  s.enabled,
	}
}

// IsEnabled returns whether query expansion is enabled.
// Parameters: none.
// Returns:
//   - bool: true when expansion is enabled.
func (s *QueryExpansionService) IsEnabled() bool {
	return s.snapshot().enabled
}

// Snapshot returns the non-secret query expansion runtime configuration.
func (s *QueryExpansionService) Snapshot() QueryExpansionSnapshot {
	cfg := s.snapshot()
	return QueryExpansionSnapshot{
		Enabled:  cfg.enabled,
		Model:    cfg.model,
		BaseURL:  cfg.baseURL,
		Endpoint: cfg.endpoint,
	}
}

// queryExpansionRequest represents the request to the LLM API
type queryExpansionRequest struct {
	Model       string                  `json:"model"`
	Messages    []queryExpansionMessage `json:"messages"`
	MaxTokens   int                     `json:"max_tokens"`
	Temperature float32                 `json:"temperature"`
}

type queryExpansionMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type queryExpansionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// queryExpansionStreamRequest represents a streaming request to the LLM API
type queryExpansionStreamRequest struct {
	Model       string                  `json:"model"`
	Messages    []queryExpansionMessage `json:"messages"`
	MaxTokens   int                     `json:"max_tokens"`
	Temperature float32                 `json:"temperature"`
	Stream      bool                    `json:"stream"`
}

// streamDelta represents a delta in the streaming response
type streamDelta struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
}

// Expand expands a short query into a richer semantic description.
// Parameters:
//   - ctx: context for cancellation and deadlines.
//   - query: original query string.
//
// Returns:
//   - string: expanded query text (or original on fallback).
//   - error: non-nil if the expansion request fails.
func (s *QueryExpansionService) Expand(ctx context.Context, query string) (string, error) {
	cfg := s.snapshot()
	if !cfg.enabled {
		return query, nil
	}

	// Skip expansion for already long queries (likely already descriptive)
	if len([]rune(query)) > 50 {
		return query, nil
	}

	req := queryExpansionRequest{
		Model: cfg.model,
		Messages: []queryExpansionMessage{
			{
				Role:    "system",
				Content: queryExpansionPrompt,
			},
			{
				Role:    "user",
				Content: query,
			},
		},
		MaxTokens:   150,
		Temperature: 0.3, // Lower temperature for more consistent expansions
	}

	var resp queryExpansionResponse
	httpResp, err := cfg.client.R().
		SetContext(ctx).
		SetBody(req).
		SetResult(&resp).
		Post(cfg.endpoint)

	if err != nil {
		// On error, fall back to original query
		return query, fmt.Errorf("query expansion API call failed: %w", err)
	}

	if httpResp.StatusCode() < 200 || httpResp.StatusCode() >= 300 {
		if resp.Error != nil {
			return query, fmt.Errorf("query expansion API error: %s", resp.Error.Message)
		}
		return query, fmt.Errorf("query expansion API error: status %d", httpResp.StatusCode())
	}

	if len(resp.Choices) == 0 || resp.Choices[0].Message.Content == "" {
		return query, nil
	}

	expanded := strings.TrimSpace(resp.Choices[0].Message.Content)

	// Validate expansion - if it's too short or seems invalid, return original
	if len([]rune(expanded)) < 10 {
		return query, nil
	}

	return expanded, nil
}

// ExpandWithFallback expands a query and returns the original on any error.
// Parameters:
//   - ctx: context for cancellation and deadlines.
//   - query: original query string.
//
// Returns:
//   - string: expanded query or original when expansion fails.
func (s *QueryExpansionService) ExpandWithFallback(ctx context.Context, query string) string {
	expanded, err := s.Expand(ctx, query)
	if err != nil {
		return query
	}
	return expanded
}

// ExpandStream expands a query with streaming token output.
// Parameters:
//   - ctx: context for cancellation and deadlines.
//   - query: original query string.
//   - tokenCh: channel to receive individual tokens.
//
// Returns:
//   - string: complete expanded query.
//   - error: non-nil if the expansion request fails.
func (s *QueryExpansionService) ExpandStream(ctx context.Context, query string, tokenCh chan<- string) (string, error) {
	defer close(tokenCh)

	cfg := s.snapshot()
	if !cfg.enabled {
		return query, nil
	}

	// Skip expansion for already long queries
	if len([]rune(query)) > 50 {
		return query, nil
	}

	req := queryExpansionStreamRequest{
		Model: cfg.model,
		Messages: []queryExpansionMessage{
			{
				Role:    "system",
				Content: queryExpansionPrompt,
			},
			{
				Role:    "user",
				Content: query,
			},
		},
		MaxTokens:   150,
		Temperature: 0.3,
		Stream:      true,
	}

	reqBody, err := json.Marshal(req)
	if err != nil {
		return query, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request manually for streaming
	httpReq, err := http.NewRequestWithContext(ctx, "POST", cfg.endpoint, strings.NewReader(string(reqBody)))
	if err != nil {
		return query, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if cfg.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+cfg.apiKey)
	}
	httpReq.Header.Set("Accept", "text/event-stream")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return query, fmt.Errorf("stream request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return query, fmt.Errorf("stream API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	// Parse SSE stream
	var fullContent strings.Builder
	scanner := bufio.NewScanner(resp.Body)

	for scanner.Scan() {
		line := scanner.Text()

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}

		// Parse data lines
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")

			// Check for stream end
			if data == "[DONE]" {
				break
			}

			// Parse JSON delta
			var delta streamDelta
			if err := json.Unmarshal([]byte(data), &delta); err != nil {
				continue // Skip malformed data
			}

			if len(delta.Choices) > 0 && delta.Choices[0].Delta.Content != "" {
				content := delta.Choices[0].Delta.Content
				fullContent.WriteString(content)
				tokenCh <- content
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return query, fmt.Errorf("stream read error: %w", err)
	}

	expanded := strings.TrimSpace(fullContent.String())

	// Validate expansion
	if len([]rune(expanded)) < 10 {
		return query, nil
	}

	return expanded, nil
}
