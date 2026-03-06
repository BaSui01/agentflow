// =============================================================================
// AgentFlow OpenAI-Compatible Provider Base
// =============================================================================
// Shared implementation for all OpenAI-compatible LLM providers.
// Providers like DeepSeek, Qwen, GLM, Grok, Doubao, MiniMax embed this
// and only override what differs (Name, BaseURL, default model, headers).
// =============================================================================

package openaicompat

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

// Config holds the configuration for an OpenAI-compatible provider.
type Config struct {
	// ProviderName is the unique identifier for this provider (e.g., "deepseek", "qwen").
	ProviderName string

	// APIKey is the authentication key for the provider's API.
	APIKey string

	// BaseURL is the base URL for the provider's API (e.g., "https://api.deepseek.com").
	BaseURL string

	// DefaultModel is the model to use when none is specified in the request.
	DefaultModel string

	// FallbackModel is used when both request and DefaultModel are empty.
	FallbackModel string

	// Timeout is the HTTP client timeout. Defaults to 30s if zero.
	Timeout time.Duration

	// EndpointPath is the chat completions endpoint path. Defaults to "/v1/chat/completions".
	EndpointPath string

	// ModelsEndpoint is the models list endpoint path. Defaults to "/v1/models".
	ModelsEndpoint string

	// BuildHeaders is an optional function to set custom headers on each request.
	// If nil, the default "Authorization: Bearer <apiKey>" header is used.
	BuildHeaders func(req *http.Request, apiKey string)

	// RequestHook is an optional function to modify the request body before sending.
	// Use this for provider-specific fields (e.g., DeepSeek's ReasoningMode model selection).
	RequestHook func(req *llm.ChatRequest, body *providerbase.OpenAICompatRequest)

	// SupportsTools indicates whether this provider supports native function calling.
	// Defaults to true if not set.
	SupportsTools *bool

	// AuthHeaderName 自定义认证头名称。为空时使用默认的 "Authorization: Bearer <key>"。
	// 设置后使用 "<AuthHeaderName>: <key>"（不加 Bearer 前缀）。
	AuthHeaderName string

	// APIKeys 多 API Key 列表，轮询使用。如果非空，优先于 APIKey。
	APIKeys []providers.APIKeyEntry
}

// Provider is the base implementation for all OpenAI-compatible LLM providers.
// Embed this in your provider struct and override Name() if needed.
type Provider struct {
	Cfg           Config
	Client        *http.Client
	Logger        *zap.Logger
	RewriterChain *middleware.RewriterChain
	keyIndex      uint64 // 多 Key 轮询索引
}

// New creates a new OpenAI-compatible provider with the given config.
func New(cfg Config, logger *zap.Logger) *Provider {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	if cfg.EndpointPath == "" {
		cfg.EndpointPath = "/v1/chat/completions"
	}
	if cfg.ModelsEndpoint == "" {
		cfg.ModelsEndpoint = "/v1/models"
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Provider{
		Cfg:    cfg,
		Client: tlsutil.SecureHTTPClient(timeout),
		Logger: logger,
		RewriterChain: middleware.NewRewriterChain(
			middleware.NewEmptyToolsCleaner(),
		),
	}
}

// Name returns the provider name.
func (p *Provider) Name() string { return p.Cfg.ProviderName }

// SupportsStructuredOutput returns true because OpenAI-compatible providers
// support native JSON Schema response_format.
func (p *Provider) SupportsStructuredOutput() bool { return true }

// SupportsNativeFunctionCalling returns whether this provider supports tool calling.
func (p *Provider) SupportsNativeFunctionCalling() bool {
	if p.Cfg.SupportsTools != nil {
		return *p.Cfg.SupportsTools
	}
	return true
}

// SetBuildHeaders sets custom header builder for the provider.
func (p *Provider) SetBuildHeaders(fn func(req *http.Request, apiKey string)) {
	p.Cfg.BuildHeaders = fn
}

// ApplyHeaders applies provider-specific headers to the request.
func (p *Provider) ApplyHeaders(req *http.Request, apiKey string) {
	p.buildHeaders(req, apiKey)
}

// ResolveAPIKey returns the effective API key for this request context.
// Resolution order: context override -> APIKeys round-robin -> APIKey.
func (p *Provider) ResolveAPIKey(ctx context.Context) string {
	return p.resolveAPIKey(ctx)
}

// buildHeaders applies headers to the HTTP request.
func (p *Provider) buildHeaders(req *http.Request, apiKey string) {
	if p.Cfg.BuildHeaders != nil {
		p.Cfg.BuildHeaders(req, apiKey)
		return
	}
	if p.Cfg.AuthHeaderName != "" {
		req.Header.Set(p.Cfg.AuthHeaderName, apiKey)
	} else {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	req.Header.Set("Content-Type", "application/json")
}

// resolveAPIKey returns the API key, checking for context override first.
func (p *Provider) resolveAPIKey(ctx context.Context) string {
	// 1. 优先使用 context 中的凭据覆盖
	if c, ok := llm.CredentialOverrideFromContext(ctx); ok {
		if strings.TrimSpace(c.APIKey) != "" {
			return strings.TrimSpace(c.APIKey)
		}
	}
	// 2. 多 Key 轮询
	if len(p.Cfg.APIKeys) > 0 {
		idx := atomic.AddUint64(&p.keyIndex, 1) - 1
		return p.Cfg.APIKeys[idx%uint64(len(p.Cfg.APIKeys))].Key
	}
	// 3. 单 Key
	return p.Cfg.APIKey
}

// endpoint builds the full URL for a given path.
func (p *Provider) endpoint(path string) string {
	return fmt.Sprintf("%s%s", strings.TrimRight(p.Cfg.BaseURL, "/"), path)
}

// NewRequest builds an HTTP request with provider-specific headers applied.
func (p *Provider) NewRequest(ctx context.Context, method, path string, body io.Reader, apiKey string) (*http.Request, error) {
	httpReq, err := http.NewRequestWithContext(ctx, method, p.endpoint(path), body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	p.buildHeaders(httpReq, apiKey)
	return httpReq, nil
}

// Do executes an HTTP request and maps network errors to types.Error.
func (p *Provider) Do(httpReq *http.Request) (*http.Response, error) {
	resp, err := p.Client.Do(httpReq)
	if err != nil {
		return nil, &types.Error{
			Code:    llm.ErrUpstreamError,
			Message: err.Error(), Cause: err, HTTPStatus: http.StatusBadGateway,
			Retryable: true,
			Provider:  p.Name(),
		}
	}
	return resp, nil
}

// DoJSON sends a JSON request and decodes a JSON response.
func (p *Provider) DoJSON(ctx context.Context, method, path string, payload any, apiKey string, out any) error {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to marshal request: %w", err)
		}
		body = bytes.NewReader(data)
	}

	httpReq, err := p.NewRequest(ctx, method, path, body, apiKey)
	if err != nil {
		return err
	}
	resp, err := p.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		msg := providerbase.ReadErrorMessage(resp.Body)
		return providerbase.MapHTTPError(resp.StatusCode, msg, p.Name())
	}

	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return &types.Error{
			Code:    llm.ErrUpstreamError,
			Message: err.Error(), Cause: err, HTTPStatus: http.StatusBadGateway,
			Retryable: true,
			Provider:  p.Name(),
		}
	}
	return nil
}

// HealthCheck verifies the provider is reachable.
func (p *Provider) HealthCheck(ctx context.Context) (*llm.HealthStatus, error) {
	start := time.Now()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, p.endpoint(p.Cfg.ModelsEndpoint), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	p.buildHeaders(httpReq, p.resolveAPIKey(ctx))

	resp, err := p.Client.Do(httpReq)
	latency := time.Since(start)
	if err != nil {
		return &llm.HealthStatus{Healthy: false, Latency: latency}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg := providerbase.ReadErrorMessage(resp.Body)
		return &llm.HealthStatus{Healthy: false, Latency: latency},
			fmt.Errorf("%s health check failed: status=%d msg=%s", p.Cfg.ProviderName, resp.StatusCode, msg)
	}

	return &llm.HealthStatus{Healthy: true, Latency: latency}, nil
}

// ListModels returns the list of available models.
func (p *Provider) ListModels(ctx context.Context) ([]llm.Model, error) {
	apiKey := p.resolveAPIKey(ctx)
	return providerbase.ListModelsOpenAICompat(
		ctx, p.Client, p.Cfg.BaseURL, apiKey, p.Cfg.ProviderName,
		p.Cfg.ModelsEndpoint, p.buildHeaders,
	)
}

// Endpoints 返回该提供者使用的所有 API 端点完整 URL。
func (p *Provider) Endpoints() llm.ProviderEndpoints {
	return llm.ProviderEndpoints{
		Completion: p.endpoint(p.Cfg.EndpointPath),
		Models:     p.endpoint(p.Cfg.ModelsEndpoint),
		BaseURL:    p.Cfg.BaseURL,
	}
}

// Completion performs a non-streaming chat completion.
func (p *Provider) Completion(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	// Apply rewriter chain
	rewrittenReq, err := p.RewriterChain.Execute(ctx, req)
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
	model := providerbase.ChooseModel(req, p.Cfg.DefaultModel, p.Cfg.FallbackModel)

	body := providerbase.OpenAICompatRequest{
		Model:               model,
		Messages:            providerbase.ConvertMessagesToOpenAI(req.Messages),
		Tools:               providerbase.ConvertToolsToOpenAI(req.Tools),
		MaxTokens:           req.MaxTokens,
		Temperature:         req.Temperature,
		TopP:                req.TopP,
		Stop:                req.Stop,
		FrequencyPenalty:    req.FrequencyPenalty,
		PresencePenalty:     req.PresencePenalty,
		RepetitionPenalty:   req.RepetitionPenalty,
		N:                   req.N,
		LogProbs:            req.LogProbs,
		TopLogProbs:         req.TopLogProbs,
		ParallelToolCalls:   req.ParallelToolCalls,
		ServiceTier:         req.ServiceTier,
		User:                req.User,
		MaxCompletionTokens: req.MaxCompletionTokens,
		Store:               req.Store,
		Modalities:          req.Modalities,
	}
	if req.ToolChoice != nil {
		body.ToolChoice = req.ToolChoice
	}
	if rf := providerbase.ConvertResponseFormat(req.ResponseFormat); rf != nil {
		body.ResponseFormat = rf
	}

	// 传递 reasoning_effort
	if req.ReasoningEffort != "" {
		body.ReasoningEffort = &req.ReasoningEffort
	}

	// 传递 web_search_options
	if req.WebSearchOptions != nil {
		body.WebSearchOptions = convertWebSearchOptions(req.WebSearchOptions)
	}

	// Apply provider-specific request hook
	if p.Cfg.RequestHook != nil {
		p.Cfg.RequestHook(req, &body)
	}

	var oaResp providerbase.OpenAICompatResponse
	if err := p.DoJSON(ctx, http.MethodPost, p.Cfg.EndpointPath, body, apiKey, &oaResp); err != nil {
		return nil, err
	}

	result := providerbase.ToLLMChatResponse(oaResp, p.Name())
	if oaResp.Created != 0 {
		result.CreatedAt = time.Unix(oaResp.Created, 0)
	}
	result.ServiceTier = oaResp.ServiceTier
	return result, nil
}

// Stream performs a streaming chat completion via SSE.
func (p *Provider) Stream(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	// Apply rewriter chain
	rewrittenReq, err := p.RewriterChain.Execute(ctx, req)
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
	model := providerbase.ChooseModel(req, p.Cfg.DefaultModel, p.Cfg.FallbackModel)

	body := providerbase.OpenAICompatRequest{
		Model:               model,
		Messages:            providerbase.ConvertMessagesToOpenAI(req.Messages),
		Tools:               providerbase.ConvertToolsToOpenAI(req.Tools),
		MaxTokens:           req.MaxTokens,
		Temperature:         req.Temperature,
		TopP:                req.TopP,
		Stop:                req.Stop,
		Stream:              true,
		FrequencyPenalty:    req.FrequencyPenalty,
		PresencePenalty:     req.PresencePenalty,
		RepetitionPenalty:   req.RepetitionPenalty,
		N:                   req.N,
		LogProbs:            req.LogProbs,
		TopLogProbs:         req.TopLogProbs,
		ParallelToolCalls:   req.ParallelToolCalls,
		ServiceTier:         req.ServiceTier,
		User:                req.User,
		MaxCompletionTokens: req.MaxCompletionTokens,
		Store:               req.Store,
		Modalities:          req.Modalities,
	}
	if req.ToolChoice != nil {
		body.ToolChoice = req.ToolChoice
	}
	if rf := providerbase.ConvertResponseFormat(req.ResponseFormat); rf != nil {
		body.ResponseFormat = rf
	}
	if req.StreamOptions != nil {
		body.StreamOptions = &providerbase.StreamOptions{
			IncludeUsage:      req.StreamOptions.IncludeUsage,
			ChunkIncludeUsage: req.StreamOptions.ChunkIncludeUsage,
		}
	}

	// 传递 reasoning_effort
	if req.ReasoningEffort != "" {
		body.ReasoningEffort = &req.ReasoningEffort
	}

	// 传递 web_search_options
	if req.WebSearchOptions != nil {
		body.WebSearchOptions = convertWebSearchOptions(req.WebSearchOptions)
	}

	// Apply provider-specific request hook
	if p.Cfg.RequestHook != nil {
		p.Cfg.RequestHook(req, &body)
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := p.NewRequest(ctx, http.MethodPost, p.Cfg.EndpointPath, bytes.NewReader(payload), apiKey)
	if err != nil {
		return nil, err
	}

	resp, err := p.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		msg := providerbase.ReadErrorMessage(resp.Body)
		return nil, providerbase.MapHTTPError(resp.StatusCode, msg, p.Name())
	}

	return StreamSSE(ctx, resp.Body, p.Name()), nil
}

// StreamSSE parses an SSE stream from an OpenAI-compatible API and returns a channel of StreamChunks.
// This is the shared SSE parsing logic used by all OpenAI-compatible providers.
// The caller is responsible for ensuring the response status is OK before calling this.
func StreamSSE(ctx context.Context, body io.ReadCloser, providerName string) <-chan llm.StreamChunk {
	ch := make(chan llm.StreamChunk)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				// panic recovered; body and ch will be closed by defers below
			}
		}()
		defer body.Close()
		defer close(ch)
		reader := bufio.NewReader(body)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if err != io.EOF {
					select {
					case <-ctx.Done():
						return
					case ch <- llm.StreamChunk{Err: &types.Error{
						Code: llm.ErrUpstreamError, Message: err.Error(), Cause: err, HTTPStatus: http.StatusBadGateway, Retryable: true, Provider: providerName,
					}}:
					}
				}
				return
			}
			line = strings.TrimSpace(line)
			if line == "" || !strings.HasPrefix(line, "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "[DONE]" {
				return
			}

			var oaResp providerbase.OpenAICompatResponse
			if err := json.Unmarshal([]byte(data), &oaResp); err != nil {
				select {
				case <-ctx.Done():
					return
				case ch <- llm.StreamChunk{Err: &types.Error{
					Code: llm.ErrUpstreamError, Message: err.Error(), Cause: err, HTTPStatus: http.StatusBadGateway, Retryable: true, Provider: providerName,
				}}:
				}
				return
			}

			// 处理流式 usage（stream_options.include_usage 时最后一个 chunk 会包含）
			if oaResp.Usage != nil {
				streamUsage := &llm.ChatUsage{
					PromptTokens:     oaResp.Usage.PromptTokens,
					CompletionTokens: oaResp.Usage.CompletionTokens,
					TotalTokens:      oaResp.Usage.TotalTokens,
				}
				// 如果没有 choices，发送一个只包含 usage 的 chunk
				if len(oaResp.Choices) == 0 {
					select {
					case <-ctx.Done():
						return
					case ch <- llm.StreamChunk{
						ID:       oaResp.ID,
						Provider: providerName,
						Model:    oaResp.Model,
						Usage:    streamUsage,
					}:
					}
					continue
				}
			}

			for _, choice := range oaResp.Choices {
				chunk := llm.StreamChunk{
					ID:           oaResp.ID,
					Provider:     providerName,
					Model:        oaResp.Model,
					Index:        choice.Index,
					FinishReason: choice.FinishReason,
					Delta: types.Message{
						Role: llm.RoleAssistant,
					},
				}
				if choice.Delta != nil {
					chunk.Delta.Content = choice.Delta.Content
					chunk.Delta.Refusal = choice.Delta.Refusal
					chunk.Delta.ReasoningContent = choice.Delta.ReasoningContent
					if len(choice.Delta.ToolCalls) > 0 {
						chunk.Delta.ToolCalls = make([]types.ToolCall, 0, len(choice.Delta.ToolCalls))
						for _, tc := range choice.Delta.ToolCalls {
							chunk.Delta.ToolCalls = append(chunk.Delta.ToolCalls, types.ToolCall{
								ID:        tc.ID,
								Name:      tc.Function.Name,
								Arguments: tc.Function.Arguments,
							})
						}
					}
				}
				if oaResp.Usage != nil {
					chunk.Usage = &llm.ChatUsage{
						PromptTokens:     oaResp.Usage.PromptTokens,
						CompletionTokens: oaResp.Usage.CompletionTokens,
						TotalTokens:      oaResp.Usage.TotalTokens,
					}
				}
				select {
				case <-ctx.Done():
					return
				case ch <- chunk:
				}
			}
		}
	}()
	return ch
}

// convertWebSearchOptions converts llm.WebSearchOptions to the wire format.
func convertWebSearchOptions(opts *llm.WebSearchOptions) *providerbase.WebSearchOptions {
	if opts == nil {
		return nil
	}
	result := &providerbase.WebSearchOptions{
		SearchContextSize: opts.SearchContextSize,
	}
	if opts.UserLocation != nil {
		result.UserLocation = &providerbase.WebSearchUserLocation{
			Type: "approximate",
			Approximate: &providerbase.WebSearchApproxLocation{
				Country:  opts.UserLocation.Country,
				Region:   opts.UserLocation.Region,
				City:     opts.UserLocation.City,
				Timezone: opts.UserLocation.Timezone,
			},
		}
	}
	return result
}
