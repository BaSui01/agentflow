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
	"time"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/middleware"
	"github.com/BaSui01/agentflow/llm/providers"
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
}

// NewGeminiProvider 创建 Gemini Provider
func NewGeminiProvider(cfg providers.GeminiConfig, logger *zap.Logger) *GeminiProvider {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}

	// 设置默认 BaseURL
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://generativelanguage.googleapis.com"
	}

	return &GeminiProvider{
		cfg: cfg,
		client: &http.Client{
			Timeout: timeout,
		},
		logger: logger,
		rewriterChain: middleware.NewRewriterChain(
			middleware.NewEmptyToolsCleaner(),
		),
	}
}

func (p *GeminiProvider) Name() string { return "gemini" }

func (p *GeminiProvider) HealthCheck(ctx context.Context) (*llm.HealthStatus, error) {
	start := time.Now()
	endpoint := fmt.Sprintf("%s/v1beta/models", strings.TrimRight(p.cfg.BaseURL, "/"))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	p.buildHeaders(httpReq, p.cfg.APIKey)

	resp, err := p.client.Do(httpReq)
	latency := time.Since(start)
	if err != nil {
		return &llm.HealthStatus{Healthy: false, Latency: latency}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg := readGeminiErrMsg(resp.Body)
		return &llm.HealthStatus{Healthy: false, Latency: latency}, fmt.Errorf("gemini health check failed: status=%d msg=%s", resp.StatusCode, msg)
	}
	return &llm.HealthStatus{Healthy: true, Latency: latency}, nil
}

func (p *GeminiProvider) SupportsNativeFunctionCalling() bool { return true }

// ListModels 获取 Gemini 支持的模型列表
func (p *GeminiProvider) ListModels(ctx context.Context) ([]llm.Model, error) {
	endpoint := fmt.Sprintf("%s/v1beta/models", strings.TrimRight(p.cfg.BaseURL, "/"))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	p.buildHeaders(httpReq, p.cfg.APIKey)

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
		msg := readGeminiErrMsg(resp.Body)
		return nil, mapGeminiError(resp.StatusCode, msg, p.Name())
	}

	var modelsResp struct {
		Models []struct {
			Name               string   `json:"name"`
			BaseModelID        string   `json:"baseModelId"`
			Version            string   `json:"version"`
			DisplayName        string   `json:"displayName"`
			Description        string   `json:"description"`
			InputTokenLimit    int      `json:"inputTokenLimit"`
			OutputTokenLimit   int      `json:"outputTokenLimit"`
			SupportedMethods   []string `json:"supportedGenerationMethods"`
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
		models = append(models, llm.Model{
			ID:      modelID,
			Object:  "model",
			OwnedBy: "google",
		})
	}

	return models, nil
}

// Gemini 消息结构
type geminiContent struct {
	Role  string       `json:"role,omitempty"` // user, model
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text             string                  `json:"text,omitempty"`
	InlineData       *geminiInlineData       `json:"inlineData,omitempty"`
	FunctionCall     *geminiFunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *geminiFunctionResponse `json:"functionResponse,omitempty"`
}

type geminiInlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"` // base64 encoded
}

type geminiFunctionCall struct {
	Name string                 `json:"name"`
	Args map[string]interface{} `json:"args"`
}

type geminiFunctionResponse struct {
	Name     string                 `json:"name"`
	Response map[string]interface{} `json:"response"`
}

type geminiTool struct {
	FunctionDeclarations []geminiFunctionDeclaration `json:"functionDeclarations,omitempty"`
}

type geminiFunctionDeclaration struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"` // JSON Schema
}

type geminiGenerationConfig struct {
	Temperature     float32  `json:"temperature,omitempty"`
	TopP            float32  `json:"topP,omitempty"`
	TopK            int      `json:"topK,omitempty"`
	MaxOutputTokens int      `json:"maxOutputTokens,omitempty"`
	StopSequences   []string `json:"stopSequences,omitempty"`
}

type geminiRequest struct {
	Contents          []geminiContent         `json:"contents"`
	Tools             []geminiTool            `json:"tools,omitempty"`
	GenerationConfig  *geminiGenerationConfig `json:"generationConfig,omitempty"`
	SystemInstruction *geminiContent          `json:"systemInstruction,omitempty"`
}

type geminiCandidate struct {
	Content       geminiContent `json:"content"`
	FinishReason  string        `json:"finishReason,omitempty"`
	Index         int           `json:"index"`
	SafetyRatings []interface{} `json:"safetyRatings,omitempty"`
}

type geminiUsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

type geminiResponse struct {
	Candidates    []geminiCandidate    `json:"candidates"`
	UsageMetadata *geminiUsageMetadata `json:"usageMetadata,omitempty"`
	ModelVersion  string               `json:"modelVersion,omitempty"`
	ResponseID    string               `json:"responseId,omitempty"`
}

type geminiErrorResp struct {
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error"`
}

func (p *GeminiProvider) buildHeaders(req *http.Request, apiKey string) {
	// Gemini 使用 x-goog-api-key 认证
	req.Header.Set("x-goog-api-key", apiKey)
	req.Header.Set("Content-Type", "application/json")
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

		// 转换角色名称
		role := string(m.Role)
		if role == "assistant" {
			role = "model" // Gemini 使用 "model" 而不是 "assistant"
		}

		content := geminiContent{
			Role: role,
		}

		// 文本内容
		if m.Content != "" {
			content.Parts = append(content.Parts, geminiPart{
				Text: m.Content,
			})
		}

		// 工具调用
		if len(m.ToolCalls) > 0 {
			for _, tc := range m.ToolCalls {
				var args map[string]interface{}
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

		// 工具响应
		if m.Role == llm.RoleTool && m.ToolCallID != "" {
			var response map[string]interface{}
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
						Response: map[string]interface{}{
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
		var params map[string]interface{}
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

	apiKey := p.cfg.APIKey
	if c, ok := llm.CredentialOverrideFromContext(ctx); ok {
		if strings.TrimSpace(c.APIKey) != "" {
			apiKey = strings.TrimSpace(c.APIKey)
		}
	}

	systemInstruction, contents := convertToGeminiContents(req.Messages)

	body := geminiRequest{
		Contents:          contents,
		Tools:             convertToGeminiTools(req.Tools),
		SystemInstruction: systemInstruction,
	}

	// 生成配置
	if req.Temperature > 0 || req.TopP > 0 || req.MaxTokens > 0 || len(req.Stop) > 0 {
		body.GenerationConfig = &geminiGenerationConfig{
			Temperature:     req.Temperature,
			TopP:            req.TopP,
			MaxOutputTokens: req.MaxTokens,
			StopSequences:   req.Stop,
		}
	}

	payload, _ := json.Marshal(body)
	model := chooseGeminiModel(req, p.cfg.Model)
	endpoint := fmt.Sprintf("%s/v1beta/models/%s:generateContent", strings.TrimRight(p.cfg.BaseURL, "/"), model)

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

	apiKey := p.cfg.APIKey
	if c, ok := llm.CredentialOverrideFromContext(ctx); ok {
		if strings.TrimSpace(c.APIKey) != "" {
			apiKey = strings.TrimSpace(c.APIKey)
		}
	}

	systemInstruction, contents := convertToGeminiContents(req.Messages)

	body := geminiRequest{
		Contents:          contents,
		Tools:             convertToGeminiTools(req.Tools),
		SystemInstruction: systemInstruction,
	}

	if req.Temperature > 0 || req.TopP > 0 || req.MaxTokens > 0 {
		body.GenerationConfig = &geminiGenerationConfig{
			Temperature:     req.Temperature,
			TopP:            req.TopP,
			MaxOutputTokens: req.MaxTokens,
		}
	}

	payload, _ := json.Marshal(body)
	model := chooseGeminiModel(req, p.cfg.Model)
	endpoint := fmt.Sprintf("%s/v1beta/models/%s:streamGenerateContent", strings.TrimRight(p.cfg.BaseURL, "/"), model)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	p.buildHeaders(httpReq, apiKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, err
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
					ch <- llm.StreamChunk{
						Err: &llm.Error{
							Code:       llm.ErrUpstreamError,
							Message:    err.Error(),
							HTTPStatus: http.StatusBadGateway,
							Retryable:  true,
							Provider:   p.Name(),
						},
					}
				}
				return
			}

			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			// Gemini 流式响应是 JSON 数组格式
			// 每行是一个完整的 JSON 对象
			var geminiResp geminiResponse
			if err := json.Unmarshal([]byte(line), &geminiResp); err != nil {
				continue
			}

			// 处理每个候选响应
			for _, candidate := range geminiResp.Candidates {
				chunk := llm.StreamChunk{
					Provider:     p.Name(),
					Model:        model,
					Index:        candidate.Index,
					FinishReason: candidate.FinishReason,
					Delta: llm.Message{
						Role: llm.RoleAssistant,
					},
				}

				// 解析内容
				toolCallIndex := 0
				for _, part := range candidate.Content.Parts {
					if part.Text != "" {
						chunk.Delta.Content += part.Text
					}

					if part.FunctionCall != nil {
						argsJSON, _ := json.Marshal(part.FunctionCall.Args)
						// 生成唯一的工具调用 ID
						toolCallID := fmt.Sprintf("call_%s_%d_%d", part.FunctionCall.Name, candidate.Index, toolCallIndex)
						chunk.Delta.ToolCalls = append(chunk.Delta.ToolCalls, llm.ToolCall{
							ID:        toolCallID,
							Name:      part.FunctionCall.Name,
							Arguments: argsJSON,
						})
						toolCallIndex++
					}
				}

				ch <- chunk
			}

			// 最后一个 chunk 包含 usage
			if geminiResp.UsageMetadata != nil {
				ch <- llm.StreamChunk{
					Provider: p.Name(),
					Model:    model,
					Usage: &llm.ChatUsage{
						PromptTokens:     geminiResp.UsageMetadata.PromptTokenCount,
						CompletionTokens: geminiResp.UsageMetadata.CandidatesTokenCount,
						TotalTokens:      geminiResp.UsageMetadata.TotalTokenCount,
					},
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

		// 解析内容
		toolCallIndex := 0
		for _, part := range candidate.Content.Parts {
			if part.Text != "" {
				msg.Content += part.Text
			}

			if part.FunctionCall != nil {
				argsJSON, _ := json.Marshal(part.FunctionCall.Args)
				// 生成唯一的工具调用 ID
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
			FinishReason: candidate.FinishReason,
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
		resp.Usage = llm.ChatUsage{
			PromptTokens:     gr.UsageMetadata.PromptTokenCount,
			CompletionTokens: gr.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      gr.UsageMetadata.TotalTokenCount,
		}
	}

	return resp
}

func readGeminiErrMsg(body io.Reader) string {
	data, _ := io.ReadAll(body)
	var errResp geminiErrorResp
	if err := json.Unmarshal(data, &errResp); err == nil && errResp.Error.Message != "" {
		return fmt.Sprintf("%s (status: %s)", errResp.Error.Message, errResp.Error.Status)
	}
	return string(data)
}

func mapGeminiError(status int, msg string, provider string) *llm.Error {
	switch status {
	case http.StatusUnauthorized:
		return &llm.Error{Code: llm.ErrUnauthorized, Message: msg, HTTPStatus: status, Provider: provider}
	case http.StatusForbidden:
		return &llm.Error{Code: llm.ErrForbidden, Message: msg, HTTPStatus: status, Provider: provider}
	case http.StatusTooManyRequests:
		return &llm.Error{Code: llm.ErrRateLimited, Message: msg, HTTPStatus: status, Retryable: true, Provider: provider}
	case http.StatusBadRequest:
		if strings.Contains(msg, "quota") || strings.Contains(msg, "limit") {
			return &llm.Error{Code: llm.ErrQuotaExceeded, Message: msg, HTTPStatus: status, Provider: provider}
		}
		return &llm.Error{Code: llm.ErrInvalidRequest, Message: msg, HTTPStatus: status, Provider: provider}
	case http.StatusServiceUnavailable, http.StatusBadGateway, http.StatusGatewayTimeout:
		return &llm.Error{Code: llm.ErrUpstreamError, Message: msg, HTTPStatus: status, Retryable: true, Provider: provider}
	default:
		return &llm.Error{Code: llm.ErrUpstreamError, Message: msg, HTTPStatus: status, Retryable: status >= 500, Provider: provider}
	}
}

func chooseGeminiModel(req *llm.ChatRequest, defaultModel string) string {
	if req != nil && req.Model != "" {
		return req.Model
	}
	if defaultModel != "" {
		return defaultModel
	}
	// Gemini 默认模型 (2026: Gemini 3)
	return "gemini-3-pro"
}
