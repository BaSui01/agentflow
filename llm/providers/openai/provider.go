package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/BaSui01/agentflow/llm/providers/openaicompat"
	"go.uber.org/zap"
)

// previousResponseIDKey 是 Responses API 中 previous_response_id 的 context key。
type previousResponseIDKey struct{}

// WithPreviousResponseID 在 ctx 中写入 previous_response_id。
func WithPreviousResponseID(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, previousResponseIDKey{}, id)
}

// PreviousResponseIDFromContext 从 ctx 读取 previous_response_id。
func PreviousResponseIDFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(previousResponseIDKey{}).(string)
	return v, ok && v != ""
}

// OpenAIProvider 实现 OpenAI LLM 提供者.
// 支持传统 Chat Completions API 和新的 Responses API (2025).
// 传统 API 通过嵌入的 openaicompat.Provider 处理；Responses API 通过 Completion 覆写实现.
type OpenAIProvider struct {
	*openaicompat.Provider
	openaiCfg providers.OpenAIConfig
}

// NewOpenAIProvider 创建新的 OpenAI 提供者实例.
func NewOpenAIProvider(cfg providers.OpenAIConfig, logger *zap.Logger) *OpenAIProvider {
	p := &OpenAIProvider{
		Provider: openaicompat.New(openaicompat.Config{
			ProviderName:  "openai",
			APIKey:        cfg.APIKey,
			BaseURL:       cfg.BaseURL,
			DefaultModel:  cfg.Model,
			FallbackModel: "gpt-5.2", // 2026: GPT-5.2
			Timeout:       cfg.Timeout,
		}, logger),
		openaiCfg: cfg,
	}

	// Set custom headers for OpenAI (Organization support)
	p.SetBuildHeaders(func(req *http.Request, apiKey string) {
		req.Header.Set("Authorization", "Bearer "+apiKey)
		if cfg.Organization != "" {
			req.Header.Set("OpenAI-Organization", cfg.Organization)
		}
		req.Header.Set("Content-Type", "application/json")
	})

	return p
}

// Completion 覆写基类方法，支持 Responses API 路由.
// 当 UseResponsesAPI 启用时走 /v1/responses，否则委托给 openaicompat.Provider.Completion.
func (p *OpenAIProvider) Completion(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	if !p.openaiCfg.UseResponsesAPI {
		return p.Provider.Completion(ctx, req)
	}

	// Apply rewriter chain (与基类保持一致)
	rewrittenReq, err := p.RewriterChain.Execute(ctx, req)
	if err != nil {
		return nil, &llm.Error{
			Code: llm.ErrInvalidRequest, Message: fmt.Sprintf("request rewrite failed: %v", err),
			HTTPStatus: http.StatusBadRequest, Provider: p.Name(),
		}
	}
	req = rewrittenReq

	apiKey := p.Provider.Cfg.APIKey
	if c, ok := llm.CredentialOverrideFromContext(ctx); ok {
		if strings.TrimSpace(c.APIKey) != "" {
			apiKey = strings.TrimSpace(c.APIKey)
		}
	}

	return p.completionWithResponsesAPI(ctx, req, apiKey)
}

// --- Responses API Types (2025) ---

type openAIResponsesRequest struct {
	Model              string                 `json:"model"`
	Input              []openAIResponsesInput `json:"input"`
	MaxOutputTokens    int                    `json:"max_output_tokens,omitempty"`
	Temperature        float32                `json:"temperature,omitempty"`
	TopP               float32                `json:"top_p,omitempty"`
	Tools              []providers.OpenAICompatTool `json:"tools,omitempty"`
	ToolChoice         any            `json:"tool_choice,omitempty"`
	PreviousResponseID string                 `json:"previous_response_id,omitempty"`
	Store              bool                   `json:"store,omitempty"`
	Metadata           map[string]string      `json:"metadata,omitempty"`
}

type openAIResponsesInput struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIResponsesResponse struct {
	ID          string                  `json:"id"`
	Object      string                  `json:"object"`
	CreatedAt   int64                   `json:"created_at"`
	Status      string                  `json:"status"`
	CompletedAt int64                   `json:"completed_at,omitempty"`
	Model       string                  `json:"model"`
	Output      []openAIResponsesOutput `json:"output"`
	Usage       *providers.OpenAICompatUsage `json:"usage,omitempty"`
}

type openAIResponsesOutput struct {
	Type    string          `json:"type"`
	ID      string          `json:"id"`
	Status  string          `json:"status"`
	Role    string          `json:"role"`
	Content []openAIContent `json:"content"`
}

type openAIContent struct {
	Type        string          `json:"type"`
	Text        string          `json:"text,omitempty"`
	Annotations []any   `json:"annotations,omitempty"`
	ID          string          `json:"id,omitempty"`
	Name        string          `json:"name,omitempty"`
	Arguments   json.RawMessage `json:"arguments,omitempty"`
}

// completionWithResponsesAPI 使用新的 Responses API (/v1/responses).
func (p *OpenAIProvider) completionWithResponsesAPI(ctx context.Context, req *llm.ChatRequest, apiKey string) (*llm.ChatResponse, error) {
	input := make([]openAIResponsesInput, 0, len(req.Messages))
	for _, msg := range req.Messages {
		input = append(input, openAIResponsesInput{
			Role:    string(msg.Role),
			Content: msg.Content,
		})
	}

	body := openAIResponsesRequest{
		Model:           providers.ChooseModel(req, p.openaiCfg.Model, "gpt-5.2"),
		Input:           input,
		MaxOutputTokens: req.MaxTokens,
		Temperature:     req.Temperature,
		TopP:            req.TopP,
		Tools:           providers.ConvertToolsToOpenAI(req.Tools),
		Store:           true,
	}
	if req.ToolChoice != "" {
		body.ToolChoice = req.ToolChoice
	}
	if req.PreviousResponseID != "" {
		body.PreviousResponseID = req.PreviousResponseID
	} else if prevID, ok := PreviousResponseIDFromContext(ctx); ok {
		body.PreviousResponseID = prevID
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal responses api request: %w", err)
	}

	endpoint := fmt.Sprintf("%s/v1/responses", strings.TrimRight(p.openaiCfg.BaseURL, "/"))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	// 复用 OpenAI 的自定义 header（含 Organization）
	if p.Provider.Cfg.BuildHeaders != nil {
		p.Provider.Cfg.BuildHeaders(httpReq, apiKey)
	}

	resp, err := p.Provider.Client.Do(httpReq)
	if err != nil {
		return nil, &llm.Error{
			Code: llm.ErrUpstreamError, Message: err.Error(),
			HTTPStatus: http.StatusBadGateway, Retryable: true, Provider: p.Name(),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		msg := providers.ReadErrorMessage(resp.Body)
		return nil, providers.MapHTTPError(resp.StatusCode, msg, p.Name())
	}

	var responsesResp openAIResponsesResponse
	if err := json.NewDecoder(resp.Body).Decode(&responsesResp); err != nil {
		return nil, &llm.Error{
			Code: llm.ErrUpstreamError, Message: err.Error(),
			HTTPStatus: http.StatusBadGateway, Retryable: true, Provider: p.Name(),
		}
	}

	return toResponsesAPIChatResponse(responsesResp, p.Name()), nil
}

// toResponsesAPIChatResponse 将 Responses API 响应转换为统一的 llm.ChatResponse.
func toResponsesAPIChatResponse(resp openAIResponsesResponse, provider string) *llm.ChatResponse {
	choices := make([]llm.ChatChoice, 0, len(resp.Output))
	for idx, output := range resp.Output {
		if output.Type != "message" {
			continue
		}
		msg := llm.Message{Role: llm.Role(output.Role)}
		for _, content := range output.Content {
			switch content.Type {
			case "output_text":
				msg.Content += content.Text
			case "tool_call":
				msg.ToolCalls = append(msg.ToolCalls, llm.ToolCall{
					ID: content.ID, Name: content.Name, Arguments: content.Arguments,
				})
			}
		}
		choices = append(choices, llm.ChatChoice{
			Index: idx, FinishReason: output.Status, Message: msg,
		})
	}

	chatResp := &llm.ChatResponse{
		ID: resp.ID, Provider: provider, Model: resp.Model, Choices: choices,
	}
	if resp.Usage != nil {
		chatResp.Usage = llm.ChatUsage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		}
	}
	if resp.CreatedAt != 0 {
		chatResp.CreatedAt = time.Unix(resp.CreatedAt, 0)
	}
	return chatResp
}
