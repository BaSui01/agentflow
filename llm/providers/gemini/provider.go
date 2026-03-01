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

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/middleware"
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/BaSui01/agentflow/pkg/tlsutil"
	"go.uber.org/zap"
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
		cfg.Region = "us-central1"
	}

	// 设置默认 BaseURL
	if cfg.BaseURL == "" {
		if cfg.ProjectID != "" {
			cfg.BaseURL = fmt.Sprintf("https://%s-aiplatform.googleapis.com", cfg.Region)
		} else {
			cfg.BaseURL = "https://generativelanguage.googleapis.com"
		}
	}

	return &GeminiProvider{
		cfg:    cfg,
		client: tlsutil.SecureHTTPClient(timeout),
		logger: logger,
		rewriterChain: middleware.NewRewriterChain(
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
		msg := providers.ReadErrorMessage(resp.Body)
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
		return nil, &llm.Error{
			Code:       llm.ErrUpstreamError,
			Message:    err.Error(),
			HTTPStatus: http.StatusBadGateway,
			Retryable:  true,
			Provider:   p.Name(),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		msg := providers.ReadErrorMessage(resp.Body)
		return nil, providers.MapHTTPError(resp.StatusCode, msg, p.Name())
	}

	var modelsResp struct {
		Models []struct {
			Name             string   `json:"name"`
			BaseModelID      string   `json:"baseModelId"`
			Version          string   `json:"version"`
			DisplayName      string   `json:"displayName"`
			Description      string   `json:"description"`
			InputTokenLimit  int      `json:"inputTokenLimit"`
			OutputTokenLimit int      `json:"outputTokenLimit"`
			SupportedMethods []string `json:"supportedGenerationMethods"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&modelsResp); err != nil {
		return nil, &llm.Error{
			Code:       llm.ErrUpstreamError,
			Message:    err.Error(),
			HTTPStatus: http.StatusBadGateway,
			Retryable:  true,
			Provider:   p.Name(),
		}
	}

	// 转换为统一格式
	models := make([]llm.Model, 0, len(modelsResp.Models))
	for _, m := range modelsResp.Models {
		// 提取模型 ID（去掉 "models/" 前缀）
		modelID := strings.TrimPrefix(m.Name, "models/")
		model := llm.Model{
			ID:              modelID,
			Object:          "model",
			OwnedBy:         "google",
			MaxInputTokens:  m.InputTokenLimit,
			MaxOutputTokens: m.OutputTokenLimit,
			Capabilities:    convertGeminiCapabilities(m.SupportedMethods),
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
	InlineData       *geminiInlineData       `json:"inlineData,omitempty"`
	FunctionCall     *geminiFunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *geminiFunctionResponse `json:"functionResponse,omitempty"`
}

type geminiInlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"` // base64 encoded
}

type geminiFunctionCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

type geminiFunctionResponse struct {
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
}

type geminiTool struct {
	FunctionDeclarations []geminiFunctionDeclaration `json:"functionDeclarations,omitempty"`
}

type geminiFunctionDeclaration struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"` // JSON Schema
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
	FunctionCallingConfig *geminiFunctionCallingConfig `json:"functionCallingConfig,omitempty"`
}

type geminiFunctionCallingConfig struct {
	Mode                 string   `json:"mode,omitempty"`                 // AUTO, ANY, NONE
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
}

type geminiCandidate struct {
	Content       geminiContent `json:"content"`
	FinishReason  string        `json:"finishReason,omitempty"`
	Index         int           `json:"index"`
	SafetyRatings []any         `json:"safetyRatings,omitempty"`
}

type geminiUsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
	ThoughtsTokenCount   int `json:"thoughtsTokenCount,omitempty"`
}

type geminiPromptFeedback struct {
	BlockReason  string `json:"blockReason,omitempty"`
	BlockMessage string `json:"blockReasonMessage,omitempty"`
}

type geminiResponse struct {
	Candidates     []geminiCandidate     `json:"candidates"`
	UsageMetadata  *geminiUsageMetadata  `json:"usageMetadata,omitempty"`
	PromptFeedback *geminiPromptFeedback `json:"promptFeedback,omitempty"`
	ModelVersion   string                `json:"modelVersion,omitempty"`
	ResponseID     string                `json:"responseId,omitempty"`
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
func convertToGeminiContents(msgs []llm.Message) (*geminiContent, []geminiContent) {
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

		// 文本内容（tool 消息通过 functionResponse 表达，不重复发送 text）
		if m.Content != "" && m.Role != llm.RoleTool {
			content.Parts = append(content.Parts, geminiPart{
				Text: m.Content,
			})
		}

		// 工具调用
		if len(m.ToolCalls) > 0 {
			for _, tc := range m.ToolCalls {
				var args map[string]any
				if err := json.Unmarshal(tc.Arguments, &args); err == nil {
					content.Parts = append(content.Parts, geminiPart{
						FunctionCall: &geminiFunctionCall{
							Name: tc.Name,
							Args: args,
						},
					})
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

func convertToGeminiTools(tools []llm.ToolSchema) []geminiTool {
	if len(tools) == 0 {
		return nil
	}

	declarations := make([]geminiFunctionDeclaration, 0, len(tools))
	for _, t := range tools {
		var params map[string]any
		if err := json.Unmarshal(t.Parameters, &params); err == nil {
			declarations = append(declarations, geminiFunctionDeclaration{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  params,
			})
		}
	}

	if len(declarations) == 0 {
		return nil
	}

	return []geminiTool{{
		FunctionDeclarations: declarations,
	}}
}

func (p *GeminiProvider) Completion(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	// 统一入口：应用改写器链
	rewrittenReq, err := p.rewriterChain.Execute(ctx, req)
	if err != nil {
		return nil, &llm.Error{
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
		Tools:             convertToGeminiTools(req.Tools),
		ToolConfig:        convertToolChoice(req.ToolChoice),
		SystemInstruction: systemInstruction,
		SafetySettings:    convertSafetySettings(p.cfg.SafetySettings),
	}

	// 生成配置
	body.GenerationConfig = buildGenerationConfig(req)

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	model := providers.ChooseModel(req, p.cfg.Model, defaultModel)
	endpoint := p.completionEndpoint(model)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	p.buildHeaders(httpReq, apiKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, &llm.Error{
			Code:       llm.ErrUpstreamError,
			Message:    err.Error(),
			HTTPStatus: http.StatusBadGateway,
			Retryable:  true,
			Provider:   p.Name(),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		msg := providers.ReadErrorMessage(resp.Body)
		return nil, providers.MapHTTPError(resp.StatusCode, msg, p.Name())
	}

	var geminiResp geminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&geminiResp); err != nil {
		return nil, &llm.Error{
			Code:       llm.ErrUpstreamError,
			Message:    err.Error(),
			HTTPStatus: http.StatusBadGateway,
			Retryable:  true,
			Provider:   p.Name(),
		}
	}

	// 检查 promptFeedback（安全过滤导致的拒绝）
	if err := checkPromptFeedback(geminiResp, p.Name()); err != nil {
		return nil, err
	}

	return toGeminiChatResponse(geminiResp, p.Name(), model), nil
}

func (p *GeminiProvider) Stream(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	// 统一入口：应用改写器链
	rewrittenReq, err := p.rewriterChain.Execute(ctx, req)
	if err != nil {
		return nil, &llm.Error{
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
		Tools:             convertToGeminiTools(req.Tools),
		ToolConfig:        convertToolChoice(req.ToolChoice),
		SystemInstruction: systemInstruction,
		SafetySettings:    convertSafetySettings(p.cfg.SafetySettings),
	}

	body.GenerationConfig = buildGenerationConfig(req)

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, &llm.Error{
			Code:       llm.ErrInvalidRequest,
			Message:    fmt.Sprintf("failed to marshal request: %v", err),
			HTTPStatus: http.StatusBadRequest,
			Provider:   p.Name(),
			Cause:      err,
		}
	}
	model := providers.ChooseModel(req, p.cfg.Model, defaultModel)
	endpoint := p.streamEndpoint(model)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, &llm.Error{
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
		return nil, &llm.Error{
			Code:       llm.ErrUpstreamError,
			Message:    err.Error(),
			HTTPStatus: http.StatusBadGateway,
			Retryable:  true,
			Provider:   p.Name(),
			Cause:      err,
		}
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		msg := providers.ReadErrorMessage(resp.Body)
		return nil, providers.MapHTTPError(resp.StatusCode, msg, p.Name())
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
						Err: &llm.Error{
							Code:       llm.ErrUpstreamError,
							Message:    err.Error(),
							HTTPStatus: http.StatusBadGateway,
							Retryable:  true,
							Provider:   p.Name(),
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

			// Gemini SSE 格式：data: {json}\n\n
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "" {
				continue
			}

			var geminiResp geminiResponse
			if err := json.Unmarshal([]byte(data), &geminiResp); err != nil {
				continue
			}

			// 处理每个候选响应
			for _, candidate := range geminiResp.Candidates {
				chunk := llm.StreamChunk{
					ID:           geminiResp.ResponseID,
					Provider:     p.Name(),
					Model:        model,
					Index:        candidate.Index,
					FinishReason: normalizeFinishReason(candidate.FinishReason),
					Delta: llm.Message{
						Role: llm.RoleAssistant,
					},
				}

				toolCallIndex := 0
				for _, part := range candidate.Content.Parts {
					// Thinking content
					if part.Thought != nil && *part.Thought {
						chunk.Delta.ReasoningContent = strPtr(part.Text)
						continue
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
						chunk.Delta.ToolCalls = append(chunk.Delta.ToolCalls, llm.ToolCall{
							ID:        toolCallID,
							Name:      part.FunctionCall.Name,
							Arguments: argsJSON,
						})
						toolCallIndex++
					}
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
		msg := llm.Message{
			Role: llm.RoleAssistant,
		}

		toolCallIndex := 0
		for _, part := range candidate.Content.Parts {
			// Thinking content
			if part.Thought != nil && *part.Thought {
				msg.ReasoningContent = strPtr(part.Text)
				continue
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
				msg.ToolCalls = append(msg.ToolCalls, llm.ToolCall{
					ID:        toolCallID,
					Name:      part.FunctionCall.Name,
					Arguments: argsJSON,
				})
				toolCallIndex++
			}
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
	return &llm.Error{
		Code:       llm.ErrContentFiltered,
		Message:    msg,
		HTTPStatus: http.StatusBadRequest,
		Provider:   provider,
	}
}

// convertToolChoice maps ChatRequest.ToolChoice to Gemini's ToolConfig.
func convertToolChoice(toolChoice any) *geminiToolConfig {
	if toolChoice == nil {
		return nil
	}
	switch v := toolChoice.(type) {
	case string:
		switch v {
		case "auto":
			return &geminiToolConfig{FunctionCallingConfig: &geminiFunctionCallingConfig{Mode: "AUTO"}}
		case "required", "any":
			return &geminiToolConfig{FunctionCallingConfig: &geminiFunctionCallingConfig{Mode: "ANY"}}
		case "none":
			return &geminiToolConfig{FunctionCallingConfig: &geminiFunctionCallingConfig{Mode: "NONE"}}
		}
	case map[string]any:
		// OpenAI-style {"type":"function","function":{"name":"fn"}}
		if fn, ok := v["function"].(map[string]any); ok {
			if name, ok := fn["name"].(string); ok {
				return &geminiToolConfig{
					FunctionCallingConfig: &geminiFunctionCallingConfig{
						Mode:                 "ANY",
						AllowedFunctionNames: []string{name},
					},
				}
			}
		}
	}
	return nil
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

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
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
