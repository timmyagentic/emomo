package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
)

// LLMJSONRequest describes one non-streaming OpenAI-compatible JSON task.
type LLMJSONRequest struct {
	SystemPrompt string
	UserPrompt   string
	MaxTokens    int
	Temperature  float32
	Timeout      time.Duration
}

// LLMJSONClient is the small interface used by agentic planner/reranker code.
type LLMJSONClient interface {
	Complete(ctx context.Context, req LLMJSONRequest) (string, error)
}

// OpenAICompatibleJSONClientConfig configures an OpenAI-compatible chat client.
type OpenAICompatibleJSONClientConfig struct {
	Model   string
	APIKey  string
	BaseURL string
	Timeout time.Duration
}

// OpenAICompatibleJSONClient calls /chat/completions and expects JSON text.
type OpenAICompatibleJSONClient struct {
	client   *resty.Client
	model    string
	endpoint string
	timeout  time.Duration
}

// NewOpenAICompatibleJSONClient creates a JSON-only chat-completion client.
func NewOpenAICompatibleJSONClient(cfg OpenAICompatibleJSONClientConfig) (*OpenAICompatibleJSONClient, error) {
	if strings.TrimSpace(cfg.Model) == "" {
		return nil, fmt.Errorf("model is required")
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("api key is required")
	}
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	client := resty.New().
		SetHeader("Authorization", "Bearer "+cfg.APIKey).
		SetHeader("Content-Type", "application/json").
		SetTimeout(timeout)
	return &OpenAICompatibleJSONClient{
		client:   client,
		model:    cfg.Model,
		endpoint: baseURL + "/chat/completions",
		timeout:  timeout,
	}, nil
}

type llmJSONMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type llmJSONChatRequest struct {
	Model       string           `json:"model"`
	Messages    []llmJSONMessage `json:"messages"`
	MaxTokens   int              `json:"max_tokens"`
	Temperature float32          `json:"temperature"`
}

type llmJSONChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Complete sends a chat completion request and returns the assistant content.
func (c *OpenAICompatibleJSONClient) Complete(ctx context.Context, req LLMJSONRequest) (string, error) {
	if c == nil {
		return "", fmt.Errorf("llm json client is nil")
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 800
	}
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = c.timeout
	}
	callCtx := ctx
	cancel := func() {}
	if timeout > 0 {
		callCtx, cancel = context.WithTimeout(ctx, timeout)
	}
	defer cancel()

	body := llmJSONChatRequest{
		Model: c.model,
		Messages: []llmJSONMessage{
			{Role: "system", Content: req.SystemPrompt},
			{Role: "user", Content: req.UserPrompt},
		},
		MaxTokens:   maxTokens,
		Temperature: req.Temperature,
	}

	var resp llmJSONChatResponse
	httpResp, err := c.client.R().
		SetContext(callCtx).
		SetBody(body).
		SetResult(&resp).
		Post(c.endpoint)
	if err != nil {
		return "", fmt.Errorf("llm json API call failed: %w", err)
	}
	if httpResp.StatusCode() < 200 || httpResp.StatusCode() >= 300 {
		if resp.Error != nil && resp.Error.Message != "" {
			return "", fmt.Errorf("llm json API error: %s", resp.Error.Message)
		}
		return "", fmt.Errorf("llm json API error: status %d", httpResp.StatusCode())
	}
	if len(resp.Choices) == 0 || strings.TrimSpace(resp.Choices[0].Message.Content) == "" {
		return "", fmt.Errorf("llm json API returned empty content")
	}
	return strings.TrimSpace(resp.Choices[0].Message.Content), nil
}
