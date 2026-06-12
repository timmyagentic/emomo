package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
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
	// vlmAnalyzeSystemPrompt drives the single-call OCR + description pipeline
	// used by AnalyzeImage. Earlier revisions of this file split the work
	// across separate description and OCR-only prompts; merging them halved
	// the per-meme VLM round-trips and is now the only path.
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

// ImageAnalyzer is the ingest-facing contract for image OCR/semantic analysis.
type ImageAnalyzer interface {
	GetModel() string
	AnalyzeImage(ctx context.Context, imageData []byte, format string) (*VLMAnalysis, error)
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
	Provider             string
	Model                string
	APIKey               string
	BaseURL              string
	LocalAnalyzerCommand string
	LocalAnalyzerLang    string
	LocalAnalyzerPSM     string
}

// NewImageAnalyzer creates the configured image analyzer implementation.
func NewImageAnalyzer(cfg *VLMConfig) (ImageAnalyzer, error) {
	if cfg == nil {
		cfg = &VLMConfig{}
	}
	switch normalizeProviderName(cfg.Provider) {
	case "", "openai", "openai-compatible", "openai_compatible":
		return NewVLMService(cfg), nil
	case "local_text_presence", "local-text-presence":
		return NewLocalTextPresenceAnalyzer(cfg), nil
	default:
		return nil, fmt.Errorf("unsupported VLM provider %q", cfg.Provider)
	}
}

func normalizeProviderName(provider string) string {
	return strings.ToLower(strings.TrimSpace(provider))
}

// NewVLMService creates a new VLM service.
// Parameters:
//   - cfg: VLM configuration including provider, model, and API key.
//
// Returns:
//   - *VLMService: initialized VLM client wrapper.
func NewVLMService(cfg *VLMConfig) *VLMService {
	if cfg == nil {
		cfg = &VLMConfig{}
	}
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

// LocalTextPresenceAnalyzer performs local OCR for mandatory has_text labels.
type LocalTextPresenceAnalyzer struct {
	model   string
	command string
	lang    string
	psm     string
}

// NewLocalTextPresenceAnalyzer creates a local analyzer backed by tesseract.
func NewLocalTextPresenceAnalyzer(cfg *VLMConfig) *LocalTextPresenceAnalyzer {
	if cfg == nil {
		cfg = &VLMConfig{}
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = "local-text-presence-tesseract"
	}
	command := strings.TrimSpace(cfg.LocalAnalyzerCommand)
	if command == "" {
		command = "tesseract"
	}
	lang := strings.TrimSpace(cfg.LocalAnalyzerLang)
	if lang == "" {
		lang = "chi_sim+eng"
	}
	psm := strings.TrimSpace(cfg.LocalAnalyzerPSM)
	if psm == "" {
		psm = "6"
	}
	return &LocalTextPresenceAnalyzer{
		model:   model,
		command: command,
		lang:    lang,
		psm:     psm,
	}
}

func (a *LocalTextPresenceAnalyzer) GetModel() string {
	return a.model
}

func (a *LocalTextPresenceAnalyzer) AnalyzeImage(ctx context.Context, imageData []byte, format string) (*VLMAnalysis, error) {
	tmp, err := os.CreateTemp("", "emomo-local-analyzer-*."+sanitizeLocalAnalyzerExt(format))
	if err != nil {
		return nil, fmt.Errorf("failed to create local analyzer image temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(imageData); err != nil {
		tmp.Close()
		return nil, fmt.Errorf("failed to write local analyzer image temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return nil, fmt.Errorf("failed to close local analyzer image temp file: %w", err)
	}

	args := []string{tmpPath, "stdout"}
	if a.lang != "" {
		args = append(args, "-l", a.lang)
	}
	if a.psm != "" {
		args = append(args, "--psm", a.psm)
	}

	cmd := exec.CommandContext(ctx, a.command, args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("local text-presence analyzer failed: command=%s: %w: %s", a.command, err, strings.TrimSpace(stderr.String()))
	}

	return &VLMAnalysis{
		OCRText: strings.TrimSpace(string(out)),
	}, nil
}

func sanitizeLocalAnalyzerExt(format string) string {
	switch strings.ToLower(strings.TrimPrefix(strings.TrimSpace(format), ".")) {
	case "jpg", "jpeg":
		return "jpg"
	case "png":
		return "png"
	case "webp":
		return "webp"
	default:
		return "jpg"
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
//   - `do_sample: false` switches compatible models to greedy decoding,
//     making the OCR + JSON payload from AnalyzeImage reproducible.
//   - `thinking: {"type": "disabled"}` would suppress providers that expose a
//     separate reasoning channel for latency-sensitive callers. We currently
//     leave it ON (default) for providers that support it because hybrid
//     thinking can improve OCR accuracy and scene reasoning, and the reasoning
//     channel does not consume the `content` token budget on those providers.
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
// VLMAnalysis with both fields populated. It is the only VLM entry point used
// by the ingest pipeline; earlier split-prompt methods (description-only +
// OCR-only) have been removed.
//
// Reliability strategy — z.ai's vision endpoint does not support API-level
// `response_format`, so when the model drifts off-format (returns prose, a
// markdown-fenced JSON, an empty object, or truncates mid-token) we cannot
// detect that server-side. AnalyzeImage therefore retries the call exactly
// once: on the second attempt the prompt is augmented with a user message
// that quotes the previous bad reply and asks the model to re-emit the
// raw JSON. This template nudges compatible models back into the JSON channel;
// the cost is one extra round-trip on the unlucky path.
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
//     callers should surface the error rather than persisting a meme without
//     the required text-presence annotation.
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

	// Log the full raw reply (no truncation) so post-mortem diagnosis can see
	// the exact byte sequence that broke parsing — most failures are caused
	// by malformed escapes hiding mid-string and the previous 200-rune cap
	// hid them. The retry round-trip embeds the previous raw via
	// buildAnalyzeRetryUserPrompt, which has its own 1000-rune cap so a
	// runaway reply can't blow the model's context window. Bounded by
	// MaxTokens=8092 on the model side, single replies are typically well
	// under 30KB even on Chinese-heavy outputs.
	logger.CtxWarn(ctx, "VLM AnalyzeImage first attempt produced unusable output, retrying once: error=%v, raw=%q", err, raw)

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
// The function applies three escalating layers of tolerance:
//  1. strip markdown code fences (`stripJSONCodeFence`) and try strict
//     `json.Unmarshal`;
//  2. if the model prefixes its reply with chatter like "好的，下面是 JSON：",
//     slice between the outermost `{` and `}` and try again;
//  3. if standard parsing still fails, run `sanitizeMalformedJSONString` over
//     the candidate to repair common vision-model malformations: unescaped
//     control characters inside string values (most often a literal LF
//     between two OCR lines, e.g. `"ocr_text":"你可以的<LF>你是棒棒的小汪汪"`)
//     and orphan backslash escapes such as `\ ` before a `|` separator.
//
// When everything fails we still return a non-nil analysis whose Description
// is the raw response so the failure surface is debuggable, but the second
// return value is false and callers MUST NOT persist that analysis.
func parseVLMAnalysis(raw string) (*VLMAnalysis, bool) {
	if raw == "" {
		return &VLMAnalysis{}, false
	}

	cleaned := stripJSONCodeFence(raw)

	if analysis, ok := tryUnmarshalAnalyzePayload(cleaned); ok {
		return analysis, true
	}

	// Slice the outermost {...} block — rescues replies prefixed with
	// "好的，下面是 JSON：".
	bracedSlice := ""
	if start := strings.Index(cleaned, "{"); start >= 0 {
		if end := strings.LastIndex(cleaned, "}"); end > start {
			bracedSlice = cleaned[start : end+1]
			if analysis, ok := tryUnmarshalAnalyzePayload(bracedSlice); ok {
				return analysis, true
			}
		}
	}

	// Last layer: repair in-string control chars and illegal backslash
	// escapes, then try one more time. We run sanitize on both the
	// cleaned full response and the {...} slice because GLM occasionally
	// emits chatter outside the JSON object plus malformations inside it,
	// and either of the two slicing strategies from above can produce a
	// valid input once sanitized.
	for _, candidate := range []string{cleaned, bracedSlice} {
		if candidate == "" {
			continue
		}
		sanitized := sanitizeMalformedJSONString(candidate)
		if sanitized == candidate {
			continue
		}
		if analysis, ok := tryUnmarshalAnalyzePayload(sanitized); ok {
			return analysis, true
		}
	}

	return &VLMAnalysis{Description: raw}, false
}

// tryUnmarshalAnalyzePayload runs json.Unmarshal against the given input and
// projects it into a VLMAnalysis. Returns ok=true only when at least one of
// description / ocr_text ended up populated, mirroring parseVLMAnalysis's
// "produced a real object" contract.
func tryUnmarshalAnalyzePayload(s string) (*VLMAnalysis, bool) {
	var payload vlmAnalyzePayload
	if err := json.Unmarshal([]byte(s), &payload); err != nil {
		return nil, false
	}
	analysis := &VLMAnalysis{
		Description: strings.TrimSpace(payload.Description),
		OCRText:     strings.TrimSpace(payload.OCRText),
	}
	return analysis, analysis.Description != "" || analysis.OCRText != ""
}

// sanitizeMalformedJSONString repairs two common vision-model JSON
// malformations so a downstream `json.Unmarshal` can succeed. It tracks
// whether the cursor is currently inside a JSON string literal and only
// rewrites bytes when in-string; bytes between tokens (where raw control
// characters and orphan backslashes do not occur in well-formed VLM
// output anyway) are passed through unchanged.
//
// Two repair rules apply inside string literals:
//
//  1. Unescaped control characters (LF / CR / Tab / U+0000-001F) — by far
//     the most common failure for our pipeline. The spec requires every
//     character below 0x20 inside a string to be escaped, but vision models
//     can emit raw newlines between OCR lines, e.g.
//     `"ocr_text":"你可以的<LF>你是棒棒的小汪汪"`. We rewrite LF→`\n`,
//     CR→`\r`, Tab→`\t`, others→`\u00XX`.
//
//  2. Orphan backslash escapes — models can produce strings like
//     `"...学位\ | ...学校\ | ..."` (a literal backslash followed by a
//     space) when serializing OCR text that visually contains run
//     separators. We keep the eight legal escapes (`\"`, `\\`, `\/`,
//     `\b`, `\f`, `\n`, `\r`, `\t`) plus `\u`+4hex, and for every other
//     `\X` we drop the backslash and preserve `X` (`\ ` → ` `, `\|` →
//     `|`, `\中` → `中`).
//
// The transform is deliberately content-preserving: we only ever escape
// or drop the offending byte, never the surrounding OCR characters. The
// state machine treats `"` as a string boundary (toggling in/out)
// unless it follows an unescaped `\`, mirroring the JSON grammar; this
// means an *unescaped* literal `"` inside a string value (a different
// model bug we observed once in production) will misalign the toggle and leak
// into out-of-string mode — that case is rare and not handled here.
//
// Output remains valid UTF-8 because we only ever inspect ASCII bytes
// (`<0x80`); UTF-8 continuation bytes (0x80–0xBF) cannot match `\` or
// any control character so they pass through untouched.
//
// Callers should treat this as a recovery path of last resort — strict
// JSON producers should not need it, but it is acceptable for a
// best-effort VLM reply parser.
func sanitizeMalformedJSONString(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	inString := false
	for i := 0; i < len(s); {
		c := s[i]
		if !inString {
			b.WriteByte(c)
			if c == '"' {
				inString = true
			}
			i++
			continue
		}
		switch {
		case c == '\\':
			if i+1 >= len(s) {
				i++
				continue
			}
			next := s[i+1]
			switch next {
			case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
				b.WriteByte(c)
				b.WriteByte(next)
				i += 2
			case 'u':
				if i+5 < len(s) && isHexByte(s[i+2]) && isHexByte(s[i+3]) && isHexByte(s[i+4]) && isHexByte(s[i+5]) {
					b.WriteString(s[i : i+6])
					i += 6
				} else {
					b.WriteByte(next)
					i += 2
				}
			default:
				b.WriteByte(next)
				i += 2
			}
		case c == '"':
			b.WriteByte(c)
			inString = false
			i++
		case c == '\n':
			b.WriteString(`\n`)
			i++
		case c == '\r':
			b.WriteString(`\r`)
			i++
		case c == '\t':
			b.WriteString(`\t`)
			i++
		case c < 0x20:
			fmt.Fprintf(&b, `\u%04x`, c)
			i++
		default:
			b.WriteByte(c)
			i++
		}
	}
	return b.String()
}

func isHexByte(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
}

// truncateForLog clips s to at most max runes (not bytes), appending an
// ellipsis if the input was cut. Used to embed the previous raw reply in
// the retry user message — keeping the runtime context bounded so we do
// not blow past the model's context window when an upstream returns
// megabytes of garbage. Diagnostic log lines write the full untruncated
// raw reply directly via fmt %q.
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
