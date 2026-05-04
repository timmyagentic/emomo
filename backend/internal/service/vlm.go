package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"

	"github.com/timmy/emomo/internal/logger"
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

	// Analyze System Prompt - 一次调用同时产出 OCR 与描述。
	// 取代了拆成两次 HTTP 请求的 vlmSystemPrompt + vlmOCRSystemPrompt 路径。
	vlmAnalyzeSystemPrompt = `你是表情包语义分析专家。对给定图片同时完成两件事：

【任务一 OCR】
完整提取图片中所有可见文字（汉字/英文/数字/基础标点）。
- 保留原图字面，不要"修正"你认为应该是的字。
- 图中没有任何文字时，ocr_text 必须是字符串 ""。

【任务二 描述】
基于画面 + 文字含义，写一段不少于 60 字的自然段，覆盖：
- 主体（熊猫头/蘑菇头/柴犬/猫咪/小黄人/海绵宝宝/古装人物 等）
- 表情动作
- 情绪标签（无语/尴尬/开心/暴怒/委屈/嫌弃/震惊/疑惑/得意/摆烂/emo/社死/破防/裂开/绝望/狂喜/阴阳怪气/幸灾乐祸/无奈/崩溃/感动/害怕/可爱/呆萌）
- 网络梗（如涉及：芭比Q了/绝绝子/yyds/栓Q/CPU/一整个xx住/蚌埠住了 等需解释含义）
- 适用场景

【输出格式】
严格按下方 JSON 输出，不要 markdown 代码栏，不要解释：
{"ocr_text": "...", "description": "..."}`

	vlmAnalyzeUserPrompt = `请分析这张表情包图片，按 JSON 格式输出。

【参考示例】
有文字：{"ocr_text":"我不理解","description":"熊猫头表情包，配文\"我不理解\"，露出疑惑、无语的表情，歪着脑袋眼神空洞，表达对某事完全不能理解、懵逼的状态，适合在困惑、震惊、无法理解对方行为时使用。"}
无文字：{"ocr_text":"","description":"一只猫咪瘫倒在地，四仰八叉，表情疲惫、无力、摆烂，眼神空洞望向天花板，表达累了、不想动、彻底放弃挣扎的 emo 状态。"}

现在请直接输出本图的 JSON：`
)

// VLMAnalysis bundles the two products of a single VLM call: the OCR text
// extracted from the image and the natural-language description.
type VLMAnalysis struct {
	Description string
	OCRText     string
}

// vlmAnalyzePayload is the JSON shape the model is asked to produce. Keeping
// it tightly scoped helps `json.Unmarshal` succeed even if the model drifts
// (e.g. emits extra fields).
type vlmAnalyzePayload struct {
	OCRText     string `json:"ocr_text"`
	Description string `json:"description"`
}

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

// OpenAI-compatible Chat Completion API request/response structures.
//
// `DoSample` and `Thinking` are z.ai-specific extensions that ride on top of
// the OpenAI shape; both are pointer + omitempty so callers that don't need
// them get the OpenAI-compatible default payload.
//
//   - `do_sample: false` switches GLM-4.6V to greedy decoding, making OCR /
//     JSON output reproducible. We use this for AnalyzeImage / ExtractOCRText.
//   - `thinking: {"type": "disabled"}` would suppress the GLM-4.6V "deep
//     thinking" channel for latency-sensitive callers. We currently leave it
//     ON (default) because hybrid thinking improves OCR accuracy and scene
//     reasoning, and reasoning_content does not consume the `content` token
//     budget on z.ai (it streams on a separate channel).
//
// Note: z.ai's vision endpoint (ChatCompletionVisionRequest) does NOT expose
// `response_format` today; structured-output / JSON mode is only available on
// the text-only GLM-4.x models. We therefore rely on prompt discipline +
// client-side parsing in `parseVLMAnalysis` to keep the JSON contract intact.
type openAIRequest struct {
	Model     string          `json:"model"`
	Messages  []openAIMessage `json:"messages"`
	MaxTokens int             `json:"max_tokens"`
	DoSample  *bool           `json:"do_sample,omitempty"`
	Thinking  *openAIThinking `json:"thinking,omitempty"`
}

// openAIThinking maps onto z.ai's `ChatThinking` schema. The only field is
// `type`, which is either "enabled" or "disabled". Unused at the moment but
// kept so future call sites can opt into `{Type:"disabled"}` without changing
// the request struct.
type openAIThinking struct {
	Type string `json:"type"`
}

// boolPtr returns a pointer to the given bool. Used for the `do_sample`
// field which must round-trip as a literal `false` (not "missing") when we
// want greedy decoding.
func boolPtr(b bool) *bool { return &b }

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
		MaxTokens: 8092,
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

	return strings.TrimSpace(resp.Choices[0].Message.Content), nil
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
		// OCR is a deterministic transcription task; greedy decoding makes the
		// raw OCR string reproducible. Hybrid thinking remains on (default) —
		// reasoning lives on `reasoning_content` and does not pollute the
		// final OCR string emitted on `content`.
		DoSample: boolPtr(false),
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
		MaxTokens: 8092,
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

	return strings.TrimSpace(resp.Choices[0].Message.Content), nil
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

// AnalyzeImage performs OCR + description in a single VLM call, returning a
// VLMAnalysis with both fields populated. It is the preferred entry point for
// new ingest paths; the legacy DescribeImage / ExtractOCRText methods are kept
// for the OCR-only backfill path in IngestService.prepareAnnotationBestEffort.
//
// Reliability strategy — z.ai's vision endpoint does not support API-level
// `response_format`, so when the model drifts off-format (returns prose, a
// markdown-fenced JSON, an empty object, or truncates mid-token) we cannot
// detect that server-side. AnalyzeImage therefore retries the call exactly
// once: on the second attempt the prompt is augmented with a user message
// that quotes the previous bad reply and asks the model to re-emit the
// raw JSON. This template is well-known to nudge GLM-4.x back into the
// JSON channel; the cost is one extra round-trip on the unlucky path.
//
// Parameters:
//   - ctx: context for cancellation and deadlines.
//   - imageData: raw image bytes (must be in a VLM-supported format: jpg, png, webp).
//   - format: image format extension (jpg, png, webp).
//
// Returns:
//   - *VLMAnalysis: parsed analysis (Description + OCRText). Never nil on success.
//   - error: non-nil when both the initial call and the retry fail to produce
//     a structurally valid JSON payload (or the API itself errors). On error
//     callers should fall back to image-only ingest rather than persisting
//     a malformed annotation.
func (s *VLMService) AnalyzeImage(ctx context.Context, imageData []byte, format string) (*VLMAnalysis, error) {
	mimeType := getMIMEType(format)
	base64Image := base64.StdEncoding.EncodeToString(imageData)
	dataURL := fmt.Sprintf("data:%s;base64,%s", mimeType, base64Image)

	baseMessages := []openAIMessage{
		{
			Role:    "system",
			Content: vlmAnalyzeSystemPrompt,
		},
		{
			Role: "user",
			Content: []interface{}{
				openAITextContent{
					Type: "text",
					Text: vlmAnalyzeUserPrompt,
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
	}

	analysis, raw, err := s.analyzeOnce(ctx, baseMessages)
	if err == nil {
		return analysis, nil
	}

	logger.CtxWarn(ctx, "VLM AnalyzeImage first attempt produced unusable output, retrying once: error=%v, raw=%q", err, truncateForLog(raw, 200))

	retryMessages := make([]openAIMessage, 0, len(baseMessages)+1)
	retryMessages = append(retryMessages, baseMessages...)
	retryMessages = append(retryMessages, openAIMessage{
		Role:    "user",
		Content: buildAnalyzeRetryUserPrompt(raw),
	})

	analysis, _, retryErr := s.analyzeOnce(ctx, retryMessages)
	if retryErr != nil {
		return nil, fmt.Errorf("VLM AnalyzeImage failed after retry (initial=%v): %w", err, retryErr)
	}
	return analysis, nil
}

// analyzeOnce performs a single VLM analyze round-trip. It is the unit of
// retry inside AnalyzeImage: the message list is the only thing the retry
// changes, every request parameter (model, max_tokens, do_sample) stays
// pinned so we get a clean apples-to-apples second attempt.
//
// Returns the parsed analysis on success. On failure the second value is the
// raw model `content` (possibly empty) so the retry path can quote it back to
// the model; the third value is the underlying error. A response that yielded
// no structured fields is reported as an error here — callers must not
// persist it.
func (s *VLMService) analyzeOnce(ctx context.Context, messages []openAIMessage) (*VLMAnalysis, string, error) {
	req := openAIRequest{
		Model:     s.model,
		Messages:  messages,
		MaxTokens: 8092,
		// AnalyzeImage demands deterministic JSON output. We keep the
		// model's hybrid thinking channel ON (it improves OCR accuracy and
		// scene reasoning) and only kill stochastic sampling, which is the
		// API-level lever we actually want for reproducible JSON.
		DoSample: boolPtr(false),
	}

	var resp openAIResponse
	httpResp, err := s.client.R().
		SetContext(ctx).
		SetBody(req).
		SetResult(&resp).
		Post(s.endpoint)
	if err != nil {
		return nil, "", fmt.Errorf("failed to call VLM API: %w", err)
	}

	if httpResp.StatusCode() < 200 || httpResp.StatusCode() >= 300 {
		errorMsg := fmt.Sprintf("HTTP %d", httpResp.StatusCode())
		if resp.Error != nil {
			errorMsg = fmt.Sprintf("HTTP %d: %s", httpResp.StatusCode(), resp.Error.Message)
		} else {
			errorMsg = fmt.Sprintf("HTTP %d: %s", httpResp.StatusCode(), string(httpResp.Body()))
		}
		return nil, "", fmt.Errorf("VLM API returned error: %s", errorMsg)
	}

	if resp.Error != nil {
		return nil, "", fmt.Errorf("VLM API error: %s", resp.Error.Message)
	}

	if len(resp.Choices) == 0 {
		errorMsg := fmt.Sprintf("no choices in response (status: %d)", httpResp.StatusCode())
		if len(httpResp.Body()) > 0 {
			errorMsg += fmt.Sprintf(", response body: %s", string(httpResp.Body()))
		}
		return nil, "", fmt.Errorf("no response from VLM API: %s", errorMsg)
	}

	raw := strings.TrimSpace(resp.Choices[0].Message.Content)
	analysis, structured := parseVLMAnalysis(raw)
	if !structured {
		return nil, raw, fmt.Errorf("VLM response was not parseable as the expected JSON object")
	}
	return analysis, raw, nil
}

// buildAnalyzeRetryUserPrompt is the single user-turn nudge appended to the
// retry conversation. We embed the previous bad reply (truncated) so "上一次"
// has a concrete referent — without it GLM-4.x tends to argue that its prior
// reply was already JSON.
func buildAnalyzeRetryUserPrompt(prevRaw string) string {
	if strings.TrimSpace(prevRaw) == "" {
		return `上一次回包不是合法的 {"ocr_text":"...","description":"..."} JSON，请重新输出。只输出 JSON 一行，不要 markdown 代码栏，不要解释、不要思考过程外的任何文字。`
	}
	return fmt.Sprintf(`你上一次的回包是：
"""
%s
"""

这不是合法的 {"ocr_text":"...","description":"..."} JSON。请重新输出。只输出 JSON 一行，不要 markdown 代码栏，不要解释、不要思考过程外的任何文字。`, truncateForLog(prevRaw, 1000))
}

// parseVLMAnalysis parses the VLM JSON response. The second return value
// reports whether parsing produced a real JSON object with at least one
// populated field — callers (currently AnalyzeImage's retry / failure path)
// treat `false` as a hard failure that warrants either retrying the call or
// abandoning the annotation rather than persisting a fallback description.
//
// The function stays tolerant about how the JSON is wrapped:
//   - leading/trailing markdown code fences are stripped (`stripJSONCodeFence`);
//   - if the model prefixes its reply with chatter like "好的，下面是 JSON：",
//     we fall back to slicing between the outermost `{` and `}`.
//
// When everything fails we still return a non-nil analysis whose Description
// is the raw response so the failure surface is debuggable, but the second
// return value is false and callers MUST NOT persist that analysis.
func parseVLMAnalysis(raw string) (*VLMAnalysis, bool) {
	if raw == "" {
		return &VLMAnalysis{}, false
	}

	cleaned := stripJSONCodeFence(raw)

	var payload vlmAnalyzePayload
	if err := json.Unmarshal([]byte(cleaned), &payload); err == nil {
		analysis := &VLMAnalysis{
			Description: strings.TrimSpace(payload.Description),
			OCRText:     strings.TrimSpace(payload.OCRText),
		}
		return analysis, analysis.Description != "" || analysis.OCRText != ""
	}

	// As a last-ditch attempt, locate the outermost {...} block and try once
	// more — this rescues replies prefixed with "好的，下面是 JSON：".
	if start := strings.Index(cleaned, "{"); start >= 0 {
		if end := strings.LastIndex(cleaned, "}"); end > start {
			if err := json.Unmarshal([]byte(cleaned[start:end+1]), &payload); err == nil {
				analysis := &VLMAnalysis{
					Description: strings.TrimSpace(payload.Description),
					OCRText:     strings.TrimSpace(payload.OCRText),
				}
				return analysis, analysis.Description != "" || analysis.OCRText != ""
			}
		}
	}

	return &VLMAnalysis{Description: raw}, false
}

// truncateForLog clips s to at most max runes (not bytes), appending an
// ellipsis if the input was cut. Used both for human-readable error logs and
// for embedding the previous raw reply in the retry user message — keeping
// the runtime context bounded so we do not blow past the model's context
// window when an upstream returns megabytes of garbage.
func truncateForLog(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "..."
}

// stripJSONCodeFence removes a leading "```json" / trailing "```" pair if the
// model wrapped its JSON in a markdown code block despite the prompt asking
// it not to.
func stripJSONCodeFence(s string) string {
	trimmed := strings.TrimSpace(s)
	if !strings.HasPrefix(trimmed, "```") {
		return trimmed
	}
	trimmed = strings.TrimPrefix(trimmed, "```json")
	trimmed = strings.TrimPrefix(trimmed, "```JSON")
	trimmed = strings.TrimPrefix(trimmed, "```")
	trimmed = strings.TrimSpace(trimmed)
	trimmed = strings.TrimSuffix(trimmed, "```")
	return strings.TrimSpace(trimmed)
}
