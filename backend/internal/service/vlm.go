package service

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/go-resty/resty/v2"
)

// EmotionWords is the shared emotion lexicon used by VLM and query expansion.
// Keep this list in sync with query_expansion.go.
var EmotionWords = []string{
	"无语", "尴尬", "开心", "暴怒", "委屈", "嫌弃", "震惊", "疑惑", "得意", "摆烂",
	"emo", "社死", "破防", "裂开", "绝望", "狂喜", "阴阳怪气", "幸灾乐祸", "无奈", "崩溃",
	"感动", "害怕", "可爱", "呆萌", "嘲讽", "鄙视", "期待", "失望", "愤怒", "悲伤",
}

// InternetMemes is the shared meme-slang lexicon used by VLM and query expansion.
// Keep this list in sync with query_expansion.go.
var InternetMemes = []string{
	"芭比Q了(完蛋了)", "绝绝子(太绝了)", "yyds(永远的神)", "真的栓Q(真的谢谢)",
	"CPU(被PUA)", "一整个xx住", "xx子", "我不理解", "好耶", "啊这", "6",
	"笑死", "裂开", "麻了", "蚌埠住了", "绷不住了", "DNA动了",
}

const (
	// VLM System Prompt - 定义角色和规则
	vlmSystemPrompt = `你是表情包语义分析专家，负责生成用于展示、OCR 辅助和 caption/BM25 辅助检索的描述文本。默认检索链路会直接使用图片向量，描述不是图片检索的前置条件。

【分析步骤】
1. 文字提取（最高优先级）：完整提取图片中所有文字，理解文字含义和表达意图
2. 主体识别：识别人物/动物/卡通形象类型（如熊猫头、蘑菇头、柴犬、猫咪等）
3. 表情动作：描述面部表情和肢体动作
4. 情绪标签：选择最匹配的情绪词（无语/尴尬/开心/暴怒/委屈/嫌弃/震惊/疑惑/得意/摆烂/emo/社死/破防/裂开/绝望/狂喜/阴阳怪气/幸灾乐祸/无奈/崩溃/感动/害怕/可爱/呆萌）
5. 网络梗识别：如涉及流行语需解释含义（芭比Q了/绝绝子/yyds/栓Q/一整个xx住等）

【输出要求】
- 80-150字自然段落，禁止使用序号或分点
- 优先级：文字内容 > 情绪表达 > 画面描述
- 必须嵌入搜索关键词（情绪词、动作词、主体类型词）
- 无文字图片：重点描述表情、动作和情绪，不要写"图中无文字"`

	// VLM User Prompt - 包含Few-shot示例
	vlmUserPrompt = `请分析这张表情包图片。

【参考示例】
示例1：一只熊猫头表情包，文字写着"我不理解"，露出一脸疑惑、无语的表情，歪着脑袋眼神空洞，表达对某事完全不理解、懵逼的状态，适合在困惑、震惊、无法理解对方行为时使用。

示例2：柴犬表情包，狗狗露出标志性的微笑，眼睛眯成一条缝，表情开心、得意、满足，像是在说"我就知道会这样"，带有幸灾乐祸、阴阳怪气的感觉，适合表达暗爽或看好戏的心情。

示例3：蘑菇头表情包，小人双手叉腰，配文"就这？"，表情嫌弃、不屑、鄙视，表达对某事物的轻蔑和失望，觉得不过如此、不值一提。

示例4：一只猫咪瘫倒在地，四仰八叉，表情疲惫、无力、摆烂，眼神空洞望向天花板，表达累了、不想动、彻底放弃挣扎的emo状态。

现在请分析图片并生成描述：`

	// OCR System Prompt - 仅识别图片中的文字
	vlmOCRSystemPrompt = `你是OCR文字识别助手，只负责提取图片中的文字内容。`

	// OCR User Prompt - 只输出识别文本
	vlmOCRUserPrompt = `请只输出图片中的文字内容，保持原有顺序与换行，不要解释或添加任何前缀。
如果图片中没有文字，请输出空字符串。`
)

// VLMService handles image description generation using Vision Language Models.
type VLMService struct {
	client   *resty.Client
	model    string
	apiKey   string
	endpoint string
}

// VLMConfig holds configuration for VLM service.
type VLMConfig struct {
	Provider string
	Model    string
	APIKey   string
	BaseURL  string
}

// NewVLMService creates a new VLM service.
// Parameters:
//   - cfg: VLM configuration including provider, model, and API key.
//
// Returns:
//   - *VLMService: initialized VLM client wrapper.
func NewVLMService(cfg *VLMConfig) *VLMService {
	client := resty.New()
	client.SetHeader("Authorization", "Bearer "+cfg.APIKey)
	client.SetHeader("Content-Type", "application/json")
	// Set timeout to prevent hanging requests
	client.SetTimeout(60 * time.Second)

	// Default to OpenAI compatible endpoint if not specified
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	endpoint := baseURL + "/chat/completions"

	return &VLMService{
		client:   client,
		model:    cfg.Model,
		apiKey:   cfg.APIKey,
		endpoint: endpoint,
	}
}

// GetModel returns the model name being used.
// Parameters: none.
// Returns:
//   - string: model identifier.
func (s *VLMService) GetModel() string {
	return s.model
}

// OpenAI-compatible Chat Completion API request/response structures
type openAIRequest struct {
	Model     string          `json:"model"`
	Messages  []openAIMessage `json:"messages"`
	MaxTokens int             `json:"max_tokens"`
}

type openAIMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"` // string for system, []interface{} for user with images
}

type openAITextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type openAIImageContent struct {
	Type     string         `json:"type"`
	ImageURL openAIImageURL `json:"image_url"`
}

type openAIImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

// DescribeImage generates a description for an image.
// Parameters:
//   - ctx: context for cancellation and deadlines.
//   - imageData: raw image bytes (must be in a VLM-supported format: jpg, png).
//   - format: image format extension (jpg, png).
//
// Returns:
//   - string: generated description text.
//   - error: non-nil if the API request fails.
func (s *VLMService) DescribeImage(ctx context.Context, imageData []byte, format string) (string, error) {
	// Determine MIME type
	mimeType := getMIMEType(format)

	// Encode image to base64
	base64Image := base64.StdEncoding.EncodeToString(imageData)
	dataURL := fmt.Sprintf("data:%s;base64,%s", mimeType, base64Image)

	// Build request with system/user separation
	req := openAIRequest{
		Model: s.model,
		Messages: []openAIMessage{
			{
				Role:    "system",
				Content: vlmSystemPrompt,
			},
			{
				Role: "user",
				Content: []interface{}{
					openAITextContent{
						Type: "text",
						Text: vlmUserPrompt,
					},
					openAIImageContent{
						Type: "image_url",
						ImageURL: openAIImageURL{
							URL:    dataURL,
							Detail: "auto", // Use auto for better text recognition
						},
					},
				},
			},
		},
		MaxTokens: 300,
	}

	// Send request
	var resp openAIResponse
	httpResp, err := s.client.R().
		SetContext(ctx).
		SetBody(req).
		SetResult(&resp).
		Post(s.endpoint)

	if err != nil {
		return "", fmt.Errorf("failed to call VLM API: %w", err)
	}

	// Check HTTP status code
	if httpResp.StatusCode() < 200 || httpResp.StatusCode() >= 300 {
		// Try to get error message from response body
		errorMsg := fmt.Sprintf("HTTP %d", httpResp.StatusCode())
		if resp.Error != nil {
			errorMsg = fmt.Sprintf("HTTP %d: %s", httpResp.StatusCode(), resp.Error.Message)
		} else {
			// Include response body for debugging
			errorMsg = fmt.Sprintf("HTTP %d: %s", httpResp.StatusCode(), string(httpResp.Body()))
		}
		return "", fmt.Errorf("VLM API returned error: %s", errorMsg)
	}

	if resp.Error != nil {
		return "", fmt.Errorf("VLM API error: %s", resp.Error.Message)
	}

	if len(resp.Choices) == 0 {
		// Include more context in error message
		errorMsg := fmt.Sprintf("no choices in response (status: %d)", httpResp.StatusCode())
		if len(httpResp.Body()) > 0 {
			errorMsg += fmt.Sprintf(", response body: %s", string(httpResp.Body()))
		}
		return "", fmt.Errorf("no response from VLM API: %s", errorMsg)
	}

	return resp.Choices[0].Message.Content, nil
}

// ExtractOCRText extracts text from an image using the VLM OCR prompt.
// Parameters:
//   - ctx: context for cancellation and deadlines.
//   - imageData: raw image bytes (must be in a VLM-supported format: jpg, png).
//   - format: image format extension (jpg, png).
//
// Returns:
//   - string: extracted OCR text (may be empty).
//   - error: non-nil if the API request fails.
func (s *VLMService) ExtractOCRText(ctx context.Context, imageData []byte, format string) (string, error) {
	// Determine MIME type
	mimeType := getMIMEType(format)

	// Encode image to base64
	base64Image := base64.StdEncoding.EncodeToString(imageData)
	dataURL := fmt.Sprintf("data:%s;base64,%s", mimeType, base64Image)

	req := openAIRequest{
		Model: s.model,
		Messages: []openAIMessage{
			{
				Role:    "system",
				Content: vlmOCRSystemPrompt,
			},
			{
				Role: "user",
				Content: []interface{}{
					openAITextContent{
						Type: "text",
						Text: vlmOCRUserPrompt,
					},
					openAIImageContent{
						Type: "image_url",
						ImageURL: openAIImageURL{
							URL:    dataURL,
							Detail: "auto",
						},
					},
				},
			},
		},
		MaxTokens: 400,
	}

	var resp openAIResponse
	httpResp, err := s.client.R().
		SetContext(ctx).
		SetBody(req).
		SetResult(&resp).
		Post(s.endpoint)

	if err != nil {
		return "", fmt.Errorf("failed to call VLM OCR API: %w", err)
	}

	if httpResp.StatusCode() < 200 || httpResp.StatusCode() >= 300 {
		errorMsg := fmt.Sprintf("HTTP %d", httpResp.StatusCode())
		if resp.Error != nil {
			errorMsg = fmt.Sprintf("HTTP %d: %s", httpResp.StatusCode(), resp.Error.Message)
		} else {
			errorMsg = fmt.Sprintf("HTTP %d: %s", httpResp.StatusCode(), string(httpResp.Body()))
		}
		return "", fmt.Errorf("VLM OCR API returned error: %s", errorMsg)
	}

	if resp.Error != nil {
		return "", fmt.Errorf("VLM OCR API error: %s", resp.Error.Message)
	}

	if len(resp.Choices) == 0 {
		errorMsg := fmt.Sprintf("no choices in response (status: %d)", httpResp.StatusCode())
		if len(httpResp.Body()) > 0 {
			errorMsg += fmt.Sprintf(", response body: %s", string(httpResp.Body()))
		}
		return "", fmt.Errorf("no response from VLM OCR API: %s", errorMsg)
	}

	return resp.Choices[0].Message.Content, nil
}

// DescribeImageFromURL generates a description for an image from URL.
// Parameters:
//   - ctx: context for cancellation and deadlines.
//   - imageURL: publicly accessible image URL.
//
// Returns:
//   - string: generated description text.
//   - error: non-nil if the API request fails.
func (s *VLMService) DescribeImageFromURL(ctx context.Context, imageURL string) (string, error) {
	// Build request with system/user separation
	req := openAIRequest{
		Model: s.model,
		Messages: []openAIMessage{
			{
				Role:    "system",
				Content: vlmSystemPrompt,
			},
			{
				Role: "user",
				Content: []interface{}{
					openAITextContent{
						Type: "text",
						Text: vlmUserPrompt,
					},
					openAIImageContent{
						Type: "image_url",
						ImageURL: openAIImageURL{
							URL:    imageURL,
							Detail: "auto", // Use auto for better text recognition
						},
					},
				},
			},
		},
		MaxTokens: 300,
	}

	var resp openAIResponse
	httpResp, err := s.client.R().
		SetContext(ctx).
		SetBody(req).
		SetResult(&resp).
		Post(s.endpoint)

	if err != nil {
		return "", fmt.Errorf("failed to call VLM API: %w", err)
	}

	// Check HTTP status code
	if httpResp.StatusCode() < 200 || httpResp.StatusCode() >= 300 {
		// Try to get error message from response body
		errorMsg := fmt.Sprintf("HTTP %d", httpResp.StatusCode())
		if resp.Error != nil {
			errorMsg = fmt.Sprintf("HTTP %d: %s", httpResp.StatusCode(), resp.Error.Message)
		} else {
			// Include response body for debugging
			errorMsg = fmt.Sprintf("HTTP %d: %s", httpResp.StatusCode(), string(httpResp.Body()))
		}
		return "", fmt.Errorf("VLM API returned error: %s", errorMsg)
	}

	if resp.Error != nil {
		return "", fmt.Errorf("VLM API error: %s", resp.Error.Message)
	}

	if len(resp.Choices) == 0 {
		// Include more context in error message
		errorMsg := fmt.Sprintf("no choices in response (status: %d)", httpResp.StatusCode())
		if len(httpResp.Body()) > 0 {
			errorMsg += fmt.Sprintf(", response body: %s", string(httpResp.Body()))
		}
		return "", fmt.Errorf("no response from VLM API: %s", errorMsg)
	}

	return resp.Choices[0].Message.Content, nil
}

func getMIMEType(format string) string {
	switch format {
	case "jpg", "jpeg":
		return "image/jpeg"
	case "png":
		return "image/png"
	case "webp":
		return "image/webp"
	default:
		return "image/jpeg"
	}
}
