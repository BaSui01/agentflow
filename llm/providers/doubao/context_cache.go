package doubao

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	providerbase "github.com/BaSui01/agentflow/llm/providers/base"

	"github.com/BaSui01/agentflow/types"

	"github.com/BaSui01/agentflow/llm"
)

// ContextCacheRequest 创建上下文缓存的请求。
type ContextCacheRequest struct {
	Model    string                             `json:"model"`
	Messages []providerbase.OpenAICompatMessage `json:"messages"`
	Mode     string                             `json:"mode,omitempty"` // "session"
	TTL      int                                `json:"ttl,omitempty"`  // 缓存过期时间（秒）
}

// ContextCacheResponse 创建上下文缓存的响应。
type ContextCacheResponse struct {
	ID        string `json:"id"`
	Model     string `json:"model"`
	CreatedAt int64  `json:"created_at"`
}

// CreateContextCache 创建上下文缓存。
// 将一组消息缓存到服务端，返回缓存 ID，后续可通过 ContextID 复用。
func (p *DoubaoProvider) CreateContextCache(ctx context.Context, model string, messages []types.Message, mode string, ttl int) (*ContextCacheResponse, error) {
	reqBody := ContextCacheRequest{
		Model:    model,
		Messages: providerbase.ConvertMessagesToOpenAI(messages),
		Mode:     mode,
		TTL:      ttl,
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal context cache request: %w", err)
	}

	endpoint := fmt.Sprintf("%s/api/v3/context/create", p.Cfg.BaseURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	p.ApplyHeaders(httpReq, p.ResolveAPIKey(ctx))

	resp, err := p.Client.Do(httpReq)
	if err != nil {
		return nil, &types.Error{
			Code:       llm.ErrUpstreamError,
			Message:    err.Error(),
			HTTPStatus: http.StatusBadGateway,
			Retryable:  true,
			Provider:   p.Name(),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		msg := providerbase.ReadErrorMessage(resp.Body)
		return nil, providerbase.MapHTTPError(resp.StatusCode, msg, p.Name())
	}

	var result ContextCacheResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, &types.Error{
			Code:       llm.ErrUpstreamError,
			Message:    err.Error(),
			HTTPStatus: http.StatusBadGateway,
			Retryable:  true,
			Provider:   p.Name(),
		}
	}

	return &result, nil
}

// ContextChatRequest 使用缓存上下文的聊天请求。
type ContextChatRequest struct {
	Model       string                             `json:"model"`
	ContextID   string                             `json:"context_id"`
	Messages    []providerbase.OpenAICompatMessage `json:"messages"`
	Stream      bool                               `json:"stream,omitempty"`
	MaxTokens   int                                `json:"max_tokens,omitempty"`
	Temperature float32                            `json:"temperature,omitempty"`
	TopP        float32                            `json:"top_p,omitempty"`
	Stop        []string                           `json:"stop,omitempty"`
}

// CompletionWithContext 使用缓存的上下文进行聊天补全。
func (p *DoubaoProvider) CompletionWithContext(ctx context.Context, contextID string, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	model := providerbase.ChooseModel(req, p.Cfg.DefaultModel, p.Cfg.FallbackModel)

	reqBody := ContextChatRequest{
		Model:       model,
		ContextID:   contextID,
		Messages:    providerbase.ConvertMessagesToOpenAI(req.Messages),
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stop:        req.Stop,
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal context chat request: %w", err)
	}

	endpoint := fmt.Sprintf("%s/api/v3/context/chat/completions", p.Cfg.BaseURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	p.ApplyHeaders(httpReq, p.ResolveAPIKey(ctx))

	resp, err := p.Client.Do(httpReq)
	if err != nil {
		return nil, &types.Error{
			Code:       llm.ErrUpstreamError,
			Message:    err.Error(),
			HTTPStatus: http.StatusBadGateway,
			Retryable:  true,
			Provider:   p.Name(),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		msg := providerbase.ReadErrorMessage(resp.Body)
		return nil, providerbase.MapHTTPError(resp.StatusCode, msg, p.Name())
	}

	var oaResp providerbase.OpenAICompatResponse
	if err := json.NewDecoder(resp.Body).Decode(&oaResp); err != nil {
		return nil, &types.Error{
			Code:       llm.ErrUpstreamError,
			Message:    err.Error(),
			HTTPStatus: http.StatusBadGateway,
			Retryable:  true,
			Provider:   p.Name(),
		}
	}

	return providerbase.ToLLMChatResponse(oaResp, p.Name()), nil
}
