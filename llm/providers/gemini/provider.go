package gemini

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	providerbase "github.com/BaSui01/agentflow/llm/providers/base"

	"github.com/BaSui01/agentflow/types"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/middleware"
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/BaSui01/agentflow/pkg/tlsutil"
	"go.uber.org/zap"
)

// 官方端点（可被配置 BaseURL 覆盖）：
// - Google AI（API Key）：https://generativelanguage.googleapis.com
//   - POST /v1beta/models/{model}:generateContent
//   - POST /v1beta/models/{model}:streamGenerateContent?alt=sse
//   - GET  /v1beta/models
//
// - Vertex AI（ProjectID 非空）：https://{region}-aiplatform.googleapis.com
//   - POST /v1/projects/{project}/locations/{region}/publishers/google/models/{model}:generateContent
//   - POST /v1/projects/{project}/locations/{region}/publishers/google/models/{model}:streamGenerateContent?alt=sse
//   - GET  /v1/projects/{project}/locations/{region}/publishers/google/models
const (
	defaultGoogleAIBaseURL = "https://generativelanguage.googleapis.com"
	vertexAIHostPattern    = "https://%s-aiplatform.googleapis.com"
	defaultVertexRegion    = "us-central1"
)

// GeminiProvider 实现 Google Gemini 的 LLM Provider
// Gemini API 特点：
// 1. 使用 x-goog-api-key 请求头认证
// 2. 多模态支持（文本、图片、音频、视频）
// 3. 支持长上下文（最高 1M tokens）
// 4. 原生工具调用支持
type GeminiProvider struct {
	cfg           providers.GeminiConfig
	client        *http.Client
	logger        *zap.Logger
	rewriterChain *middleware.RewriterChain
	keyIndex      uint64 // 多 Key 轮询索引
}

// NewGeminiProvider 创建 Gemini Provider
func NewGeminiProvider(cfg providers.GeminiConfig, logger *zap.Logger) *GeminiProvider {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}

	// Vertex AI 模式：设置默认 Region
	if cfg.ProjectID != "" && cfg.Region == "" {
		cfg.Region = defaultVertexRegion
	}

	// 设置默认 BaseURL（未配置时使用官方端点）
	if cfg.BaseURL == "" {
		if cfg.ProjectID != "" {
			cfg.BaseURL = fmt.Sprintf(vertexAIHostPattern, cfg.Region)
		} else {
			cfg.BaseURL = defaultGoogleAIBaseURL
		}
	}

	return &GeminiProvider{
		cfg:    cfg,
		client: tlsutil.SecureHTTPClient(timeout),
		logger: logger,
		rewriterChain: middleware.NewRewriterChain(
			middleware.NewXMLToolRewriter(),
			middleware.NewEmptyToolsCleaner(),
		),
	}
}

func (p *GeminiProvider) Name() string { return "gemini" }

func (p *GeminiProvider) HealthCheck(ctx context.Context) (*llm.HealthStatus, error) {
	start := time.Now()
	endpoint := p.modelsEndpoint()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	p.buildHeaders(httpReq, p.resolveAPIKey(ctx))

	resp, err := p.client.Do(httpReq)
	latency := time.Since(start)
	if err != nil {
		return &llm.HealthStatus{Healthy: false, Latency: latency}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg := providerbase.ReadErrorMessage(resp.Body)
		return &llm.HealthStatus{Healthy: false, Latency: latency}, fmt.Errorf("gemini health check failed: status=%d msg=%s", resp.StatusCode, msg)
	}
	return &llm.HealthStatus{Healthy: true, Latency: latency}, nil
}

func (p *GeminiProvider) SupportsNativeFunctionCalling() bool { return true }

// SupportsStructuredOutput returns true because Gemini supports native
// structured output via responseMimeType + responseSchema.
func (p *GeminiProvider) SupportsStructuredOutput() bool { return true }

// ListModels 获取 Gemini 支持的模型列表
func (p *GeminiProvider) ListModels(ctx context.Context) ([]llm.Model, error) {
	endpoint := p.modelsEndpoint()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	p.buildHeaders(httpReq, p.resolveAPIKey(ctx))

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, &types.Error{
			Code:    llm.ErrUpstreamError,
			Message: err.Error(), Cause: err, HTTPStatus: http.StatusBadGateway,
			Retryable: true,
			Provider:  p.Name(),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		msg := providerbase.ReadErrorMessage(resp.Body)
		return nil, providerbase.MapHTTPError(resp.StatusCode, msg, p.Name())
	}

	type geminiModelPayload struct {
		Name                  string   `json:"name"`
		ID                    string   `json:"id"`
		Model                 string   `json:"model"`
		BaseModelID           string   `json:"baseModelId"`
		Version               string   `json:"version"`
		DisplayName           string   `json:"displayName"`
		Description           string   `json:"description"`
		InputTokenLimit       int      `json:"inputTokenLimit"`
		OutputTokenLimit      int      `json:"outputTokenLimit"`
		MaxInputTokens        int      `json:"max_input_tokens"`
		MaxOutputTokens       int      `json:"max_output_tokens"`
		SupportedMethods      []string `json:"supportedGenerationMethods"`
		SupportedMethodsSnake []string `json:"supported_generation_methods"`
	}
	var modelsResp struct {
		Models []geminiModelPayload `json:"models"`
		Data   []geminiModelPayload `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&modelsResp); err != nil {
		return nil, &types.Error{
			Code:    llm.ErrUpstreamError,
			Message: err.Error(), Cause: err, HTTPStatus: http.StatusBadGateway,
			Retryable: true,
			Provider:  p.Name(),
		}
	}

	source := modelsResp.Models
	if len(source) == 0 && len(modelsResp.Data) > 0 {
		source = modelsResp.Data
	}

	// 转换为统一格式
	models := make([]llm.Model, 0, len(source))
	for _, m := range source {
		// 提取模型 ID（去掉 "models/" 前缀）
		modelID := strings.TrimSpace(firstNonEmpty(
			strings.TrimPrefix(strings.TrimSpace(m.Name), "models/"),
			strings.TrimSpace(m.ID),
			strings.TrimSpace(m.Model),
		))
		if modelID == "" {
			continue
		}
		caps := convertGeminiCapabilities(m.SupportedMethods)
		if len(caps) == 0 {
			caps = convertGeminiCapabilities(m.SupportedMethodsSnake)
		}
		maxInput := m.InputTokenLimit
		if maxInput == 0 {
			maxInput = m.MaxInputTokens
		}
		maxOutput := m.OutputTokenLimit
		if maxOutput == 0 {
			maxOutput = m.MaxOutputTokens
		}
		model := llm.Model{
			ID:              modelID,
			Object:          "model",
			OwnedBy:         "google",
			MaxInputTokens:  maxInput,
			MaxOutputTokens: maxOutput,
			Capabilities:    caps,
		}
		if len(model.Capabilities) == 0 {
			model.Capabilities = []string{"chat"}
		}
		models = append(models, model)
	}

	return models, nil
}

// convertGeminiCapabilities 将 Gemini 的 supportedGenerationMethods 转换为统一能力标识
func convertGeminiCapabilities(methods []string) []string {
	if len(methods) == 0 {
		return nil
	}
	capMap := map[string]string{
		"generateContent":  "chat",
		"embedContent":     "embedding",
		"countTokens":      "token_counting",
		"createTunedModel": "fine_tuning",
		"generateAnswer":   "question_answering",
	}
	var caps []string
	for _, m := range methods {
		if cap, ok := capMap[m]; ok {
			caps = append(caps, cap)
		}
	}
	if len(caps) == 0 {
		return nil
	}
	return caps
}

// Gemini 消息结构
type geminiContent struct {
	Role  string       `json:"role,omitempty"` // user, model
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text             string                  `json:"text,omitempty"`
	Thought          *bool                   `json:"thought,omitempty"` // true = thinking content
	ThoughtSignature string                  `json:"thoughtSignature,omitempty"`
	InlineData       *geminiInlineData       `json:"inlineData,omitempty"`
	FunctionCall     *geminiFunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *geminiFunctionResponse `json:"functionResponse,omitempty"`
}

func (p *geminiPart) UnmarshalJSON(data []byte) error {
	var aux struct {
		Text                  string                  `json:"text"`
		Thought               *bool                   `json:"thought"`
		ThoughtSnake          *bool                   `json:"is_thought"`
		ThoughtSignature      string                  `json:"thoughtSignature"`
		ThoughtSignatureSnake string                  `json:"thought_signature"`
		InlineData            *geminiInlineData       `json:"inlineData"`
		InlineDataSnake       *geminiInlineData       `json:"inline_data"`
		FunctionCall          *geminiFunctionCall     `json:"functionCall"`
		FunctionCallSnake     *geminiFunctionCall     `json:"function_call"`
		FunctionResponse      *geminiFunctionResponse `json:"functionResponse"`
		FunctionResponseSnake *geminiFunctionResponse `json:"function_response"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	p.Text = aux.Text
	p.Thought = firstBoolPtr(aux.Thought, aux.ThoughtSnake)
	p.ThoughtSignature = strings.TrimSpace(firstNonEmpty(aux.ThoughtSignature, aux.ThoughtSignatureSnake))
	p.InlineData = firstInlineData(aux.InlineData, aux.InlineDataSnake)
	p.FunctionCall = firstFunctionCall(aux.FunctionCall, aux.FunctionCallSnake)
	p.FunctionResponse = firstFunctionResponse(aux.FunctionResponse, aux.FunctionResponseSnake)
	return nil
}

type geminiInlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"` // base64 encoded
}

type geminiFunctionCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

func (f *geminiFunctionCall) UnmarshalJSON(data []byte) error {
	var aux struct {
		Name      string         `json:"name"`
		Args      map[string]any `json:"args"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	f.Name = strings.TrimSpace(aux.Name)
	f.Args = aux.Args
	if len(f.Args) == 0 {
		f.Args = aux.Arguments
	}
	return nil
}

type geminiFunctionResponse struct {
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
}

type geminiTool struct {
	FunctionDeclarations []geminiFunctionDeclaration `json:"functionDeclarations,omitempty"`
	GoogleSearch         *geminiGoogleSearch         `json:"google_search,omitempty"` // google_search grounding
}

// geminiGoogleSearch 是 google_search grounding 工具的标记结构体（空对象）
type geminiGoogleSearch struct{}

type geminiFunctionDeclaration struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"` // JSON Schema
}

// Grounding Metadata 结构体

type geminiGroundingMetadata struct {
	WebSearchQueries  []string                 `json:"webSearchQueries,omitempty"`
	SearchEntryPoint  *geminiSearchEntryPoint  `json:"searchEntryPoint,omitempty"`
	GroundingChunks   []geminiGroundingChunk   `json:"groundingChunks,omitempty"`
	GroundingSupports []geminiGroundingSupport `json:"groundingSupports,omitempty"`
}

type geminiSearchEntryPoint struct {
	RenderedContent string `json:"renderedContent,omitempty"`
}

type geminiGroundingChunk struct {
	Web *geminiGroundingChunkWeb `json:"web,omitempty"`
}

type geminiGroundingChunkWeb struct {
	URI   string `json:"uri,omitempty"`
	Title string `json:"title,omitempty"`
}

type geminiGroundingSupport struct {
	Segment               *geminiGroundingSegment `json:"segment,omitempty"`
	GroundingChunkIndices []int                   `json:"groundingChunkIndices,omitempty"`
}

type geminiGroundingSegment struct {
	StartIndex int    `json:"startIndex,omitempty"`
	EndIndex   int    `json:"endIndex,omitempty"`
	Text       string `json:"text,omitempty"`
}

type geminiGenerationConfig struct {
	Temperature      float32               `json:"temperature,omitempty"`
	TopP             float32               `json:"topP,omitempty"`
	TopK             int                   `json:"topK,omitempty"`
	MaxOutputTokens  int                   `json:"maxOutputTokens,omitempty"`
	StopSequences    []string              `json:"stopSequences,omitempty"`
	ResponseMimeType string                `json:"responseMimeType,omitempty"`
	ResponseSchema   map[string]any        `json:"responseSchema,omitempty"`
	ThinkingConfig   *geminiThinkingConfig `json:"thinkingConfig,omitempty"`
}

type geminiThinkingConfig struct {
	ThinkingLevel   string `json:"thinkingLevel,omitempty"`   // minimal, low, medium, high
	IncludeThoughts bool   `json:"includeThoughts,omitempty"` // include thought parts in response
}

type geminiToolConfig struct {
	FunctionCallingConfig            *geminiFunctionCallingConfig `json:"functionCallingConfig,omitempty"`
	IncludeServerSideToolInvocations *bool                        `json:"includeServerSideToolInvocations,omitempty"`
}

type geminiFunctionCallingConfig struct {
	Mode                 string   `json:"mode,omitempty"`                 // AUTO, ANY, NONE, VALIDATED
	AllowedFunctionNames []string `json:"allowedFunctionNames,omitempty"` // restrict callable functions
}

type geminiSafetySetting struct {
	Category  string `json:"category"`
	Threshold string `json:"threshold"`
}

type geminiRequest struct {
	Contents          []geminiContent         `json:"contents"`
	Tools             []geminiTool            `json:"tools,omitempty"`
	ToolConfig        *geminiToolConfig       `json:"toolConfig,omitempty"`
	GenerationConfig  *geminiGenerationConfig `json:"generationConfig,omitempty"`
	SystemInstruction *geminiContent          `json:"systemInstruction,omitempty"`
	SafetySettings    []geminiSafetySetting   `json:"safetySettings,omitempty"`
	CachedContent     string                  `json:"cachedContent,omitempty"`
}

func sanitizeGeminiCachedContentRequest(body *geminiRequest) {
	if body == nil || strings.TrimSpace(body.CachedContent) == "" {
		return
	}

	// Gemini cachedContent captures cacheable context such as systemInstruction/tools.
	// When the current request also carries those mutable fields, prefer preserving
	// request semantics and drop cache reuse instead of sending an invalid combination.
	if body.SystemInstruction != nil || len(body.Tools) > 0 || body.ToolConfig != nil {
		body.CachedContent = ""
	}
}

type geminiCandidate struct {
	Content           geminiContent            `json:"content"`
	FinishReason      string                   `json:"finishReason,omitempty"`
	Index             int                      `json:"index"`
	SafetyRatings     []any                    `json:"safetyRatings,omitempty"`
	GroundingMetadata *geminiGroundingMetadata `json:"groundingMetadata,omitempty"`
}

func (c *geminiCandidate) UnmarshalJSON(data []byte) error {
	var aux struct {
		Content                geminiContent            `json:"content"`
		FinishReason           string                   `json:"finishReason"`
		FinishReasonSnake      string                   `json:"finish_reason"`
		Index                  int                      `json:"index"`
		SafetyRatings          []any                    `json:"safetyRatings"`
		SafetyRatingsSnake     []any                    `json:"safety_ratings"`
		GroundingMetadata      *geminiGroundingMetadata `json:"groundingMetadata"`
		GroundingMetadataSnake *geminiGroundingMetadata `json:"grounding_metadata"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	c.Content = aux.Content
	c.FinishReason = strings.TrimSpace(firstNonEmpty(aux.FinishReason, aux.FinishReasonSnake))
	c.Index = aux.Index
	if len(aux.SafetyRatings) > 0 {
		c.SafetyRatings = aux.SafetyRatings
	} else {
		c.SafetyRatings = aux.SafetyRatingsSnake
	}
	c.GroundingMetadata = firstGroundingMetadata(aux.GroundingMetadata, aux.GroundingMetadataSnake)
	return nil
}

type geminiUsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
	ThoughtsTokenCount   int `json:"thoughtsTokenCount,omitempty"`
}

func (u *geminiUsageMetadata) UnmarshalJSON(data []byte) error {
	var aux struct {
		PromptTokenCount          *int `json:"promptTokenCount"`
		PromptTokenCountSnake     *int `json:"prompt_token_count"`
		PromptTokens              *int `json:"prompt_tokens"`
		CandidatesTokenCount      *int `json:"candidatesTokenCount"`
		CandidatesTokenCountSnake *int `json:"candidates_token_count"`
		CompletionTokens          *int `json:"completion_tokens"`
		OutputTokens              *int `json:"output_tokens"`
		TotalTokenCount           *int `json:"totalTokenCount"`
		TotalTokenCountSnake      *int `json:"total_token_count"`
		TotalTokens               *int `json:"total_tokens"`
		ThoughtsTokenCount        *int `json:"thoughtsTokenCount"`
		ThoughtsTokenCountSnake   *int `json:"thoughts_token_count"`
		ReasoningTokens           *int `json:"reasoning_tokens"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	u.PromptTokenCount = firstInt(aux.PromptTokenCount, aux.PromptTokenCountSnake, aux.PromptTokens)
	u.CandidatesTokenCount = firstInt(
		aux.CandidatesTokenCount,
		aux.CandidatesTokenCountSnake,
		aux.CompletionTokens,
		aux.OutputTokens,
	)
	u.TotalTokenCount = firstInt(aux.TotalTokenCount, aux.TotalTokenCountSnake, aux.TotalTokens)
	u.ThoughtsTokenCount = firstInt(aux.ThoughtsTokenCount, aux.ThoughtsTokenCountSnake, aux.ReasoningTokens)
	return nil
}

type geminiPromptFeedback struct {
	BlockReason  string `json:"blockReason,omitempty"`
	BlockMessage string `json:"blockReasonMessage,omitempty"`
}

func (p *geminiPromptFeedback) UnmarshalJSON(data []byte) error {
	var aux struct {
		BlockReason       string `json:"blockReason"`
		BlockReasonSnake  string `json:"block_reason"`
		BlockMessage      string `json:"blockReasonMessage"`
		BlockMessageSnake string `json:"block_reason_message"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	p.BlockReason = strings.TrimSpace(firstNonEmpty(aux.BlockReason, aux.BlockReasonSnake))
	p.BlockMessage = strings.TrimSpace(firstNonEmpty(aux.BlockMessage, aux.BlockMessageSnake))
	return nil
}

type geminiResponse struct {
	Candidates     []geminiCandidate     `json:"candidates"`
	UsageMetadata  *geminiUsageMetadata  `json:"usageMetadata,omitempty"`
	PromptFeedback *geminiPromptFeedback `json:"promptFeedback,omitempty"`
	ModelVersion   string                `json:"modelVersion,omitempty"`
	ResponseID     string                `json:"responseId,omitempty"`
}

func (r *geminiResponse) UnmarshalJSON(data []byte) error {
	var aux struct {
		Candidates          []geminiCandidate     `json:"candidates"`
		UsageMetadata       *geminiUsageMetadata  `json:"usageMetadata"`
		UsageMetadataSnake  *geminiUsageMetadata  `json:"usage_metadata"`
		Usage               *geminiUsageMetadata  `json:"usage"`
		PromptFeedback      *geminiPromptFeedback `json:"promptFeedback"`
		PromptFeedbackSnake *geminiPromptFeedback `json:"prompt_feedback"`
		ModelVersion        string                `json:"modelVersion"`
		ModelVersionSnake   string                `json:"model_version"`
		ResponseID          string                `json:"responseId"`
		ResponseIDSnake     string                `json:"response_id"`
		ID                  string                `json:"id"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	r.Candidates = aux.Candidates
	r.UsageMetadata = firstUsageMetadata(aux.UsageMetadata, aux.UsageMetadataSnake, aux.Usage)
	r.PromptFeedback = firstPromptFeedback(aux.PromptFeedback, aux.PromptFeedbackSnake)
	r.ModelVersion = strings.TrimSpace(firstNonEmpty(aux.ModelVersion, aux.ModelVersionSnake))
	r.ResponseID = strings.TrimSpace(firstNonEmpty(aux.ResponseID, aux.ResponseIDSnake, aux.ID))
	return nil
}

func (p *GeminiProvider) isVertexAI() bool {
	return p.cfg.ProjectID != ""
}

func (p *GeminiProvider) completionEndpoint(model string) string {
	base := strings.TrimRight(p.cfg.BaseURL, "/")
	if p.isVertexAI() {
		return fmt.Sprintf("%s/v1/projects/%s/locations/%s/publishers/google/models/%s:generateContent",
			base, p.cfg.ProjectID, p.cfg.Region, model)
	}
	return fmt.Sprintf("%s/v1beta/models/%s:generateContent", base, model)
}

func (p *GeminiProvider) streamEndpoint(model string) string {
	base := strings.TrimRight(p.cfg.BaseURL, "/")
	if p.isVertexAI() {
		return fmt.Sprintf("%s/v1/projects/%s/locations/%s/publishers/google/models/%s:streamGenerateContent?alt=sse",
			base, p.cfg.ProjectID, p.cfg.Region, model)
	}
	return fmt.Sprintf("%s/v1beta/models/%s:streamGenerateContent?alt=sse", base, model)
}

func (p *GeminiProvider) modelsEndpoint() string {
	base := strings.TrimRight(p.cfg.BaseURL, "/")
	if p.isVertexAI() {
		return fmt.Sprintf("%s/v1/projects/%s/locations/%s/publishers/google/models",
			base, p.cfg.ProjectID, p.cfg.Region)
	}
	return fmt.Sprintf("%s/v1beta/models", base)
}

// Endpoints 返回该提供者使用的所有 API 端点完整 URL。
func (p *GeminiProvider) Endpoints() llm.ProviderEndpoints {
	// 使用默认模型来展示端点格式
	model := p.cfg.Model
	if model == "" {
		model = "gemini-2.5-flash"
	}
	return llm.ProviderEndpoints{
		Completion: p.completionEndpoint(model),
		Stream:     p.streamEndpoint(model),
		Models:     p.modelsEndpoint(),
		BaseURL:    p.cfg.BaseURL,
	}
}

func (p *GeminiProvider) buildHeaders(req *http.Request, apiKey string) {
	if p.cfg.AuthType == "oauth" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	} else {
		req.Header.Set("x-goog-api-key", apiKey)
	}
	req.Header.Set("Content-Type", "application/json")
}

// resolveAPIKey 解析 API Key，支持上下文覆盖和多 Key 轮询
func (p *GeminiProvider) resolveAPIKey(ctx context.Context) string {
	if c, ok := llm.CredentialOverrideFromContext(ctx); ok {
		if strings.TrimSpace(c.APIKey) != "" {
			return strings.TrimSpace(c.APIKey)
		}
	}
	if len(p.cfg.APIKeys) > 0 {
		idx := atomic.AddUint64(&p.keyIndex, 1) - 1
		return p.cfg.APIKeys[idx%uint64(len(p.cfg.APIKeys))].Key
	}
	return p.cfg.APIKey
}

// convertToGeminiContents 将统一格式转换为 Gemini 格式
func convertToGeminiContents(msgs []types.Message) (*geminiContent, []geminiContent) {
	var systemInstruction *geminiContent
	var contents []geminiContent

	for _, m := range msgs {
		// 提取 system 消息
		if m.Role == llm.RoleSystem {
			systemInstruction = &geminiContent{
				Parts: []geminiPart{{Text: m.Content}},
			}
			continue
		}

		// 转换角色名称，Gemini 仅支持 user/model
		role := "user"
		if m.Role == llm.RoleAssistant {
			role = "model"
		}

		content := geminiContent{
			Role: role,
		}
		partIndex := 0

		if m.Role == llm.RoleAssistant && m.ReasoningContent != nil && strings.TrimSpace(*m.ReasoningContent) != "" {
			thoughtPart := geminiPart{
				Text:    *m.ReasoningContent,
				Thought: boolPtr(true),
			}
			if sig := geminiThoughtSignatureByIndex(m, partIndex); sig != "" {
				thoughtPart.ThoughtSignature = sig
			}
			content.Parts = append(content.Parts, thoughtPart)
			partIndex++
		}

		// 文本内容（tool 消息通过 functionResponse 表达，不重复发送 text）
		if m.Content != "" && m.Role != llm.RoleTool {
			textPart := geminiPart{
				Text: m.Content,
			}
			if sig := geminiThoughtSignatureByIndex(m, partIndex); sig != "" {
				textPart.ThoughtSignature = sig
			}
			content.Parts = append(content.Parts, textPart)
			partIndex++
		}

		// 工具调用
		if len(m.ToolCalls) > 0 {
			for _, tc := range m.ToolCalls {
				var args map[string]any
				if err := json.Unmarshal(tc.Arguments, &args); err == nil {
					callPart := geminiPart{
						FunctionCall: &geminiFunctionCall{
							Name: tc.Name,
							Args: args,
						},
					}
					if sig := geminiThoughtSignatureByIndex(m, partIndex); sig != "" {
						callPart.ThoughtSignature = sig
					}
					content.Parts = append(content.Parts, callPart)
					partIndex++
				}
			}
		}

		// 工具响应：tool role 通过 user + functionResponse 发送
		if m.Role == llm.RoleTool && m.ToolCallID != "" {
			var response map[string]any
			if err := json.Unmarshal([]byte(m.Content), &response); err == nil {
				content.Parts = append(content.Parts, geminiPart{
					FunctionResponse: &geminiFunctionResponse{
						Name:     m.Name,
						Response: response,
					},
				})
			} else {
				// 如果不是 JSON，包装为简单响应
				content.Parts = append(content.Parts, geminiPart{
					FunctionResponse: &geminiFunctionResponse{
						Name: m.Name,
						Response: map[string]any{
							"result": m.Content,
						},
					},
				})
			}
		}

		if len(content.Parts) > 0 {
			contents = append(contents, content)
		}
	}

	return systemInstruction, contents
}

// convertToGeminiTools 将统一工具列表转换为 Gemini 格式。
// 当 wsOpts 不为 nil 或工具列表中包含 web_search/google_search 时，自动注入 google_search grounding 工具。
// Gemini API 要求 FunctionDeclarations 和 GoogleSearch 在不同的 tool 条目中。
func convertToGeminiTools(tools []types.ToolSchema, wsOpts *llm.WebSearchOptions) []geminiTool {
	needGoogleSearch := wsOpts != nil

	declarations := make([]geminiFunctionDeclaration, 0, len(tools))
	for _, t := range tools {
		// 跳过 web_search / google_search 占位工具
		if t.Name == "web_search" || t.Name == "google_search" {
			needGoogleSearch = true
			continue
		}
		var params map[string]any
		if err := json.Unmarshal(t.Parameters, &params); err == nil {
			declarations = append(declarations, geminiFunctionDeclaration{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  params,
			})
		}
	}

	var result []geminiTool

	// 普通函数工具
	if len(declarations) > 0 {
		result = append(result, geminiTool{
			FunctionDeclarations: declarations,
		})
	}

	// google_search grounding 工具（必须在独立的 tool 条目中）
	if needGoogleSearch {
		result = append(result, geminiTool{
			GoogleSearch: &geminiGoogleSearch{},
		})
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

func (p *GeminiProvider) Completion(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	// 统一入口：应用改写器链
	rewrittenReq, err := p.rewriterChain.Execute(ctx, req)
	if err != nil {
		return nil, &types.Error{
			Code:       llm.ErrInvalidRequest,
			Message:    fmt.Sprintf("request rewrite failed: %v", err),
			HTTPStatus: http.StatusBadRequest,
			Provider:   p.Name(),
		}
	}
	req = rewrittenReq

	apiKey := p.resolveAPIKey(ctx)

	systemInstruction, contents := convertToGeminiContents(req.Messages)

	body := geminiRequest{
		Contents:          contents,
		Tools:             convertToGeminiTools(req.Tools, req.WebSearchOptions),
		ToolConfig:        convertToolChoice(req.ToolChoice, req.IncludeServerSideToolInvocations),
		SystemInstruction: systemInstruction,
		SafetySettings:    convertSafetySettings(p.cfg.SafetySettings),
		CachedContent:     strings.TrimSpace(req.CachedContent),
	}
	sanitizeGeminiCachedContentRequest(&body)

	// 生成配置
	body.GenerationConfig = buildGenerationConfig(req)

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	model := providerbase.ChooseModel(req, p.cfg.Model, defaultModel)
	endpoint := p.completionEndpoint(model)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	p.buildHeaders(httpReq, apiKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, &types.Error{
			Code:    llm.ErrUpstreamError,
			Message: err.Error(), Cause: err, HTTPStatus: http.StatusBadGateway,
			Retryable: true,
			Provider:  p.Name(),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		msg := providerbase.ReadErrorMessage(resp.Body)
		return nil, providerbase.MapHTTPError(resp.StatusCode, msg, p.Name())
	}

	var geminiResp geminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&geminiResp); err != nil {
		return nil, &types.Error{
			Code:    llm.ErrUpstreamError,
			Message: err.Error(), Cause: err, HTTPStatus: http.StatusBadGateway,
			Retryable: true,
			Provider:  p.Name(),
		}
	}

	// 检查 promptFeedback（安全过滤导致的拒绝）
	if err := checkPromptFeedback(geminiResp, p.Name()); err != nil {
		return nil, err
	}

	return toGeminiChatResponse(geminiResp, p.Name(), model), nil
}

func (p *GeminiProvider) Stream(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	// 对齐 Google streamGenerateContent：SSE data 行含 JSON，可选 [DONE] 或 error 负载。
	// 文档：https://ai.google.dev/gemini-api/docs/text-generation（Streaming）
	rewrittenReq, err := p.rewriterChain.Execute(ctx, req)
	if err != nil {
		return nil, &types.Error{
			Code:       llm.ErrInvalidRequest,
			Message:    fmt.Sprintf("request rewrite failed: %v", err),
			HTTPStatus: http.StatusBadRequest,
			Provider:   p.Name(),
		}
	}
	req = rewrittenReq

	apiKey := p.resolveAPIKey(ctx)

	systemInstruction, contents := convertToGeminiContents(req.Messages)

	body := geminiRequest{
		Contents:          contents,
		Tools:             convertToGeminiTools(req.Tools, req.WebSearchOptions),
		ToolConfig:        convertToolChoice(req.ToolChoice, req.IncludeServerSideToolInvocations),
		SystemInstruction: systemInstruction,
		SafetySettings:    convertSafetySettings(p.cfg.SafetySettings),
		CachedContent:     strings.TrimSpace(req.CachedContent),
	}
	sanitizeGeminiCachedContentRequest(&body)

	body.GenerationConfig = buildGenerationConfig(req)

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, &types.Error{
			Code:       llm.ErrInvalidRequest,
			Message:    fmt.Sprintf("failed to marshal request: %v", err),
			HTTPStatus: http.StatusBadRequest,
			Provider:   p.Name(),
			Cause:      err,
		}
	}
	model := providerbase.ChooseModel(req, p.cfg.Model, defaultModel)
	endpoint := p.streamEndpoint(model)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, &types.Error{
			Code:       llm.ErrInternalError,
			Message:    fmt.Sprintf("failed to create request: %v", err),
			HTTPStatus: http.StatusInternalServerError,
			Provider:   p.Name(),
			Cause:      err,
		}
	}
	p.buildHeaders(httpReq, apiKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, &types.Error{
			Code:    llm.ErrUpstreamError,
			Message: err.Error(), Cause: err, HTTPStatus: http.StatusBadGateway,
			Retryable: true,
			Provider:  p.Name(),
		}
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		msg := providerbase.ReadErrorMessage(resp.Body)
		return nil, providerbase.MapHTTPError(resp.StatusCode, msg, p.Name())
	}

	ch := make(chan llm.StreamChunk)
	go func() {
		defer resp.Body.Close()
		defer close(ch)
		reader := bufio.NewReader(resp.Body)

		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if err != io.EOF {
					select {
					case <-ctx.Done():
						return
					case ch <- llm.StreamChunk{
						Err: &types.Error{
							Code:    llm.ErrUpstreamError,
							Message: err.Error(), Cause: err, HTTPStatus: http.StatusBadGateway,
							Retryable: true,
							Provider:  p.Name(),
						},
					}:
					}
				}
				return
			}

			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			// Gemini SSE 格式：data: {json}\n\n；官方 streamGenerateContent 文档：
			// https://ai.google.dev/gemini-api/docs/text-generation#streaming
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "" {
				continue
			}
			if data == "[DONE]" {
				return
			}

			// 流式中的错误负载：{"error":{"message":"...","code":...}}
			var errPayload struct {
				Err *struct {
					Message string `json:"message"`
					Code    int    `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal([]byte(data), &errPayload); err == nil && errPayload.Err != nil {
				msg := strings.TrimSpace(errPayload.Err.Message)
				if msg == "" {
					msg = "stream error"
				}
				select {
				case <-ctx.Done():
					return
				case ch <- llm.StreamChunk{
					Err: &types.Error{
						Code:       llm.ErrUpstreamError,
						Message:    msg,
						HTTPStatus: errPayload.Err.Code,
						Provider:   p.Name(),
					},
				}:
				}
				return
			}

			var geminiResp geminiResponse
			if err := json.Unmarshal([]byte(data), &geminiResp); err != nil {
				continue
			}

			// 检查 promptFeedback（安全过滤导致的流式阻断）
			if err := checkPromptFeedback(geminiResp, p.Name()); err != nil {
				select {
				case <-ctx.Done():
					return
				case ch <- llm.StreamChunk{Err: err.(*types.Error)}:
				}
				return
			}

			// 处理每个候选响应
			for _, candidate := range geminiResp.Candidates {
				chunk := llm.StreamChunk{
					ID:           geminiResp.ResponseID,
					Provider:     p.Name(),
					Model:        model,
					Index:        candidate.Index,
					FinishReason: normalizeFinishReason(candidate.FinishReason),
					Delta: types.Message{
						Role: llm.RoleAssistant,
					},
				}

				toolCallIndex := 0
				for partIndex, part := range candidate.Content.Parts {
					// Thinking content
					if part.Thought != nil && *part.Thought {
						appendGeminiThoughtPart(&chunk.Delta, part, partIndex, p.Name())
						continue
					}
					if strings.TrimSpace(part.ThoughtSignature) != "" {
						chunk.Delta.OpaqueReasoning = append(chunk.Delta.OpaqueReasoning, types.OpaqueReasoning{
							Provider:  p.Name(),
							Kind:      "thought_signature",
							State:     part.ThoughtSignature,
							PartIndex: partIndex,
						})
					}
					if part.Text != "" {
						chunk.Delta.Content += part.Text
					}
					if part.FunctionCall != nil {
						argsJSON, err := json.Marshal(part.FunctionCall.Args)
						if err != nil {
							continue
						}
						toolCallID := fmt.Sprintf("call_%s_%d_%d", part.FunctionCall.Name, candidate.Index, toolCallIndex)
						chunk.Delta.ToolCalls = append(chunk.Delta.ToolCalls, types.ToolCall{
							ID:        toolCallID,
							Name:      part.FunctionCall.Name,
							Arguments: argsJSON,
						})
						toolCallIndex++
					}
				}

				// 提取 grounding annotations
				if candidate.GroundingMetadata != nil {
					chunk.Delta.Annotations = append(chunk.Delta.Annotations, extractGroundingAnnotations(candidate.GroundingMetadata)...)
				}

				select {
				case <-ctx.Done():
					return
				case ch <- chunk:
				}
			}

			// Usage metadata
			if geminiResp.UsageMetadata != nil {
				select {
				case <-ctx.Done():
					return
				case ch <- llm.StreamChunk{
					Provider: p.Name(),
					Model:    model,
					Usage:    convertUsageMetadata(geminiResp.UsageMetadata),
				}:
				}
			}
		}
	}()

	return ch, nil
}

func toGeminiChatResponse(gr geminiResponse, provider, model string) *llm.ChatResponse {
	choices := make([]llm.ChatChoice, 0, len(gr.Candidates))

	for _, candidate := range gr.Candidates {
		msg := types.Message{
			Role: llm.RoleAssistant,
		}

		toolCallIndex := 0
		for partIndex, part := range candidate.Content.Parts {
			// Thinking content
			if part.Thought != nil && *part.Thought {
				appendGeminiThoughtPart(&msg, part, partIndex, provider)
				continue
			}
			if strings.TrimSpace(part.ThoughtSignature) != "" {
				msg.OpaqueReasoning = append(msg.OpaqueReasoning, types.OpaqueReasoning{
					Provider:  provider,
					Kind:      "thought_signature",
					State:     part.ThoughtSignature,
					PartIndex: partIndex,
				})
			}
			if part.Text != "" {
				msg.Content += part.Text
			}
			if part.FunctionCall != nil {
				argsJSON, err := json.Marshal(part.FunctionCall.Args)
				if err != nil {
					continue
				}
				toolCallID := fmt.Sprintf("call_%s_%d", part.FunctionCall.Name, toolCallIndex)
				if gr.ResponseID != "" {
					toolCallID = fmt.Sprintf("call_%s_%s_%d", gr.ResponseID, part.FunctionCall.Name, toolCallIndex)
				}
				msg.ToolCalls = append(msg.ToolCalls, types.ToolCall{
					ID:        toolCallID,
					Name:      part.FunctionCall.Name,
					Arguments: argsJSON,
				})
				toolCallIndex++
			}
		}

		// 提取 grounding annotations
		if candidate.GroundingMetadata != nil {
			msg.Annotations = append(msg.Annotations, extractGroundingAnnotations(candidate.GroundingMetadata)...)
		}

		choices = append(choices, llm.ChatChoice{
			Index:        candidate.Index,
			FinishReason: normalizeFinishReason(candidate.FinishReason),
			Message:      msg,
		})
	}

	resp := &llm.ChatResponse{
		ID:       gr.ResponseID,
		Provider: provider,
		Model:    model,
		Choices:  choices,
	}

	if gr.UsageMetadata != nil {
		resp.Usage = *convertUsageMetadata(gr.UsageMetadata)
	}

	return resp
}

func appendGeminiThoughtPart(msg *types.Message, part geminiPart, partIndex int, provider string) {
	if msg == nil {
		return
	}
	if strings.TrimSpace(part.Text) != "" {
		if msg.ReasoningContent == nil || strings.TrimSpace(*msg.ReasoningContent) == "" {
			msg.ReasoningContent = strPtr(part.Text)
		} else {
			joined := strings.TrimSpace(*msg.ReasoningContent + "\n\n" + part.Text)
			msg.ReasoningContent = strPtr(joined)
		}
		msg.ReasoningSummaries = append(msg.ReasoningSummaries, types.ReasoningSummary{
			Provider: provider,
			Kind:     "thought_summary",
			Text:     part.Text,
			ID:       fmt.Sprintf("part_%d", partIndex),
		})
	}
	if strings.TrimSpace(part.ThoughtSignature) != "" {
		msg.OpaqueReasoning = append(msg.OpaqueReasoning, types.OpaqueReasoning{
			Provider:  provider,
			Kind:      "thought_signature",
			State:     part.ThoughtSignature,
			PartIndex: partIndex,
			ID:        fmt.Sprintf("part_%d", partIndex),
		})
	}
}

// =============================================================================
// Helper functions
// =============================================================================

const defaultModel = "gemini-2.5-flash"

// normalizeFinishReason maps Gemini finish reasons to OpenAI-compatible values.
func normalizeFinishReason(reason string) string {
	switch reason {
	case "STOP":
		return "stop"
	case "MAX_TOKENS":
		return "length"
	case "SAFETY", "RECITATION", "BLOCKLIST", "PROHIBITED_CONTENT", "SPII":
		return "content_filter"
	case "LANGUAGE":
		return "content_filter"
	case "":
		return ""
	default:
		return strings.ToLower(reason)
	}
}

// convertUsageMetadata converts Gemini usage metadata to ChatUsage.
func convertUsageMetadata(m *geminiUsageMetadata) *llm.ChatUsage {
	usage := &llm.ChatUsage{
		PromptTokens:     m.PromptTokenCount,
		CompletionTokens: m.CandidatesTokenCount,
		TotalTokens:      m.TotalTokenCount,
	}
	if m.ThoughtsTokenCount > 0 {
		usage.CompletionTokensDetails = &llm.CompletionTokensDetails{
			ReasoningTokens: m.ThoughtsTokenCount,
		}
	}
	return usage
}

// checkPromptFeedback returns an error if the response was blocked by safety filters.
func checkPromptFeedback(resp geminiResponse, provider string) error {
	if resp.PromptFeedback == nil {
		return nil
	}
	if resp.PromptFeedback.BlockReason == "" {
		return nil
	}
	msg := fmt.Sprintf("request blocked by safety filter: %s", resp.PromptFeedback.BlockReason)
	if resp.PromptFeedback.BlockMessage != "" {
		msg = fmt.Sprintf("%s — %s", msg, resp.PromptFeedback.BlockMessage)
	}
	return &types.Error{
		Code:       llm.ErrContentFiltered,
		Message:    msg,
		HTTPStatus: http.StatusBadRequest,
		Provider:   provider,
	}
}

// convertToolChoice maps ChatRequest.ToolChoice to Gemini's ToolConfig.
func convertToolChoice(toolChoice any, includeServerSide *bool) *geminiToolConfig {
	var cfg *geminiToolConfig
	switch v := toolChoice.(type) {
	case string:
		switch v {
		case "auto":
			cfg = &geminiToolConfig{FunctionCallingConfig: &geminiFunctionCallingConfig{Mode: "AUTO"}}
		case "required", "any":
			cfg = &geminiToolConfig{FunctionCallingConfig: &geminiFunctionCallingConfig{Mode: "ANY"}}
		case "none":
			cfg = &geminiToolConfig{FunctionCallingConfig: &geminiFunctionCallingConfig{Mode: "NONE"}}
		case "validated":
			cfg = &geminiToolConfig{FunctionCallingConfig: &geminiFunctionCallingConfig{Mode: "VALIDATED"}}
		}
	case map[string]any:
		// OpenAI-style {"type":"function","function":{"name":"fn"}}
		if fn, ok := v["function"].(map[string]any); ok {
			if name, ok := fn["name"].(string); ok {
				cfg = &geminiToolConfig{
					FunctionCallingConfig: &geminiFunctionCallingConfig{
						Mode:                 "ANY",
						AllowedFunctionNames: []string{name},
					},
				}
				break
			}
		}
		mode, _ := v["mode"].(string)
		if mode == "" {
			mode, _ = v["Mode"].(string)
		}
		mode = strings.ToUpper(strings.TrimSpace(mode))
		if mode != "" {
			cfg = &geminiToolConfig{
				FunctionCallingConfig: &geminiFunctionCallingConfig{
					Mode: mode,
				},
			}
			if allowed := geminiAllowedFunctionNames(v["allowed_function_names"]); len(allowed) > 0 {
				cfg.FunctionCallingConfig.AllowedFunctionNames = allowed
			} else if allowed := geminiAllowedFunctionNames(v["allowedFunctionNames"]); len(allowed) > 0 {
				cfg.FunctionCallingConfig.AllowedFunctionNames = allowed
			}
		}
		if include, ok := v["include_server_side_tool_invocations"].(bool); ok {
			includeServerSide = &include
		} else if include, ok := v["includeServerSideToolInvocations"].(bool); ok {
			includeServerSide = &include
		}
	}
	if cfg != nil && cfg.FunctionCallingConfig != nil {
		switch cfg.FunctionCallingConfig.Mode {
		case "AUTO", "ANY", "NONE", "VALIDATED":
		default:
			cfg.FunctionCallingConfig.Mode = "AUTO"
		}
	}
	if includeServerSide != nil {
		if cfg == nil {
			cfg = &geminiToolConfig{}
		}
		cfg.IncludeServerSideToolInvocations = includeServerSide
	}
	return cfg
}

func geminiAllowedFunctionNames(raw any) []string {
	switch v := raw.(type) {
	case []string:
		out := make([]string, 0, len(v))
		for _, item := range v {
			item = strings.TrimSpace(item)
			if item != "" {
				out = append(out, item)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			name, ok := item.(string)
			if !ok {
				continue
			}
			name = strings.TrimSpace(name)
			if name != "" {
				out = append(out, name)
			}
		}
		return out
	default:
		return nil
	}
}

// buildGenerationConfig constructs geminiGenerationConfig from ChatRequest.
func buildGenerationConfig(req *llm.ChatRequest) *geminiGenerationConfig {
	cfg := &geminiGenerationConfig{
		Temperature:     req.Temperature,
		TopP:            req.TopP,
		MaxOutputTokens: req.MaxTokens,
		StopSequences:   req.Stop,
	}

	// ResponseFormat
	if req.ResponseFormat != nil {
		switch req.ResponseFormat.Type {
		case llm.ResponseFormatJSONObject:
			cfg.ResponseMimeType = "application/json"
		case llm.ResponseFormatJSONSchema:
			cfg.ResponseMimeType = "application/json"
			if req.ResponseFormat.JSONSchema != nil {
				cfg.ResponseSchema = req.ResponseFormat.JSONSchema.Schema
			}
		}
	}

	// Thinking / Reasoning mode
	if req.ReasoningMode != "" {
		cfg.ThinkingConfig = &geminiThinkingConfig{
			IncludeThoughts: true,
		}
		switch req.ReasoningMode {
		case "minimal", "low", "medium", "high":
			cfg.ThinkingConfig.ThinkingLevel = req.ReasoningMode
		default:
			cfg.ThinkingConfig.ThinkingLevel = "medium"
		}
	}

	// Return nil if all fields are zero-value to keep request clean
	if cfg.Temperature == 0 && cfg.TopP == 0 && cfg.MaxOutputTokens == 0 &&
		len(cfg.StopSequences) == 0 && cfg.ResponseMimeType == "" && cfg.ThinkingConfig == nil {
		return nil
	}
	return cfg
}

func geminiThoughtSignatureByIndex(msg types.Message, index int) string {
	for _, opaque := range msg.OpaqueReasoning {
		provider := strings.TrimSpace(opaque.Provider)
		if provider != "" && provider != "gemini" {
			continue
		}
		if strings.TrimSpace(opaque.Kind) != "thought_signature" {
			continue
		}
		if opaque.PartIndex == index {
			return strings.TrimSpace(opaque.State)
		}
	}
	return ""
}

func boolPtr(v bool) *bool { return &v }

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func firstInt(values ...*int) int {
	for _, v := range values {
		if v != nil {
			return *v
		}
	}
	return 0
}

func firstBoolPtr(values ...*bool) *bool {
	for _, v := range values {
		if v != nil {
			out := *v
			return &out
		}
	}
	return nil
}

func firstInlineData(values ...*geminiInlineData) *geminiInlineData {
	for _, v := range values {
		if v != nil {
			out := *v
			return &out
		}
	}
	return nil
}

func firstFunctionCall(values ...*geminiFunctionCall) *geminiFunctionCall {
	for _, v := range values {
		if v != nil {
			out := *v
			return &out
		}
	}
	return nil
}

func firstFunctionResponse(values ...*geminiFunctionResponse) *geminiFunctionResponse {
	for _, v := range values {
		if v != nil {
			out := *v
			return &out
		}
	}
	return nil
}

func firstUsageMetadata(values ...*geminiUsageMetadata) *geminiUsageMetadata {
	for _, v := range values {
		if v != nil {
			out := *v
			return &out
		}
	}
	return nil
}

func firstPromptFeedback(values ...*geminiPromptFeedback) *geminiPromptFeedback {
	for _, v := range values {
		if v != nil {
			out := *v
			return &out
		}
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// convertSafetySettings converts config safety settings to request format.
func convertSafetySettings(settings []providers.GeminiSafetySetting) []geminiSafetySetting {
	if len(settings) == 0 {
		return nil
	}
	out := make([]geminiSafetySetting, len(settings))
	for i, s := range settings {
		out[i] = geminiSafetySetting{Category: s.Category, Threshold: s.Threshold}
	}
	return out
}

// extractGroundingAnnotations 从 Gemini grounding metadata 中提取引用标注。
func extractGroundingAnnotations(gm *geminiGroundingMetadata) []types.Annotation {
	if gm == nil {
		return nil
	}

	var annotations []types.Annotation

	if len(gm.GroundingSupports) > 0 {
		// 有 GroundingSupports → 用 segment 的位置信息精确定位引用
		for _, support := range gm.GroundingSupports {
			for _, idx := range support.GroundingChunkIndices {
				if idx < 0 || idx >= len(gm.GroundingChunks) {
					continue
				}
				chunk := gm.GroundingChunks[idx]
				if chunk.Web == nil {
					continue
				}
				ann := types.Annotation{
					Type:  "url_citation",
					URL:   chunk.Web.URI,
					Title: chunk.Web.Title,
				}
				if support.Segment != nil {
					ann.StartIndex = support.Segment.StartIndex
					ann.EndIndex = support.Segment.EndIndex
				}
				annotations = append(annotations, ann)
			}
		}
	} else if len(gm.GroundingChunks) > 0 {
		// 无 supports → 仅列出所有 GroundingChunks 作为无位置引用
		for _, chunk := range gm.GroundingChunks {
			if chunk.Web == nil {
				continue
			}
			annotations = append(annotations, types.Annotation{
				Type:  "url_citation",
				URL:   chunk.Web.URI,
				Title: chunk.Web.Title,
			})
		}
	}

	return annotations
}

func firstGroundingMetadata(values ...*geminiGroundingMetadata) *geminiGroundingMetadata {
	for _, v := range values {
		if v != nil {
			out := *v
			return &out
		}
	}
	return nil
}
