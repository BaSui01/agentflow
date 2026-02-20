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
	"time"

	"github.com/BaSui01/agentflow/internal/tlsutil"
	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/middleware"
	"github.com/BaSui01/agentflow/llm/providers"
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
	RequestHook func(req *llm.ChatRequest, body *providers.OpenAICompatRequest)

	// SupportsTools indicates whether this provider supports native function calling.
	// Defaults to true if not set.
	SupportsTools *bool
}

// Provider is the base implementation for all OpenAI-compatible LLM providers.
// Embed this in your provider struct and override Name() if needed.
type Provider struct {
	Cfg           Config
	Client        *http.Client
	Logger        *zap.Logger
	RewriterChain *middleware.RewriterChain
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

// buildHeaders applies headers to the HTTP request.
func (p *Provider) buildHeaders(req *http.Request, apiKey string) {
	if p.Cfg.BuildHeaders != nil {
		p.Cfg.BuildHeaders(req, apiKey)
		return
	}
	// Default: Bearer token auth
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
}

// resolveAPIKey returns the API key, checking for context override first.
func (p *Provider) resolveAPIKey(ctx context.Context) string {
	if c, ok := llm.CredentialOverrideFromContext(ctx); ok {
		if strings.TrimSpace(c.APIKey) != "" {
			return strings.TrimSpace(c.APIKey)
		}
	}
	return p.Cfg.APIKey
}

// endpoint builds the full URL for a given path.
func (p *Provider) endpoint(path string) string {
	return fmt.Sprintf("%s%s", strings.TrimRight(p.Cfg.BaseURL, "/"), path)
}

// HealthCheck verifies the provider is reachable.
func (p *Provider) HealthCheck(ctx context.Context) (*llm.HealthStatus, error) {
	start := time.Now()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, p.endpoint(p.Cfg.ModelsEndpoint), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	p.buildHeaders(httpReq, p.Cfg.APIKey)

	resp, err := p.Client.Do(httpReq)
	latency := time.Since(start)
	if err != nil {
		return &llm.HealthStatus{Healthy: false, Latency: latency}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg := providers.ReadErrorMessage(resp.Body)
		return &llm.HealthStatus{Healthy: false, Latency: latency},
			fmt.Errorf("%s health check failed: status=%d msg=%s", p.Cfg.ProviderName, resp.StatusCode, msg)
	}

	return &llm.HealthStatus{Healthy: true, Latency: latency}, nil
}

// ListModels returns the list of available models.
func (p *Provider) ListModels(ctx context.Context) ([]llm.Model, error) {
	return providers.ListModelsOpenAICompat(
		ctx, p.Client, p.Cfg.BaseURL, p.Cfg.APIKey, p.Cfg.ProviderName,
		p.Cfg.ModelsEndpoint, p.buildHeaders,
	)
}

// Completion performs a non-streaming chat completion.
func (p *Provider) Completion(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	// Apply rewriter chain
	rewrittenReq, err := p.RewriterChain.Execute(ctx, req)
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
	model := providers.ChooseModel(req, p.Cfg.DefaultModel, p.Cfg.FallbackModel)

	body := providers.OpenAICompatRequest{
		Model:       model,
		Messages:    providers.ConvertMessagesToOpenAI(req.Messages),
		Tools:       providers.ConvertToolsToOpenAI(req.Tools),
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stop:        req.Stop,
	}
	if req.ToolChoice != "" {
		body.ToolChoice = req.ToolChoice
	}

	// Apply provider-specific request hook
	if p.Cfg.RequestHook != nil {
		p.Cfg.RequestHook(req, &body)
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint(p.Cfg.EndpointPath), bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	p.buildHeaders(httpReq, apiKey)

	resp, err := p.Client.Do(httpReq)
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

	var oaResp providers.OpenAICompatResponse
	if err := json.NewDecoder(resp.Body).Decode(&oaResp); err != nil {
		return nil, &llm.Error{
			Code: llm.ErrUpstreamError, Message: err.Error(),
			HTTPStatus: http.StatusBadGateway, Retryable: true, Provider: p.Name(),
		}
	}

	result := providers.ToLLMChatResponse(oaResp, p.Name())
	if oaResp.Created != 0 {
		result.CreatedAt = time.Unix(oaResp.Created, 0)
	}
	return result, nil
}

// Stream performs a streaming chat completion via SSE.
func (p *Provider) Stream(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	// Apply rewriter chain
	rewrittenReq, err := p.RewriterChain.Execute(ctx, req)
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
	model := providers.ChooseModel(req, p.Cfg.DefaultModel, p.Cfg.FallbackModel)

	body := providers.OpenAICompatRequest{
		Model:       model,
		Messages:    providers.ConvertMessagesToOpenAI(req.Messages),
		Tools:       providers.ConvertToolsToOpenAI(req.Tools),
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stop:        req.Stop,
		Stream:      true,
	}
	if req.ToolChoice != "" {
		body.ToolChoice = req.ToolChoice
	}

	// Apply provider-specific request hook
	if p.Cfg.RequestHook != nil {
		p.Cfg.RequestHook(req, &body)
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint(p.Cfg.EndpointPath), bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	p.buildHeaders(httpReq, apiKey)

	resp, err := p.Client.Do(httpReq)
	if err != nil {
		return nil, &llm.Error{
			Code: llm.ErrUpstreamError, Message: err.Error(),
			HTTPStatus: http.StatusBadGateway, Retryable: true, Provider: p.Name(),
		}
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		msg := providers.ReadErrorMessage(resp.Body)
		return nil, providers.MapHTTPError(resp.StatusCode, msg, p.Name())
	}

	return StreamSSE(ctx, resp.Body, p.Name()), nil
}

// StreamSSE parses an SSE stream from an OpenAI-compatible API and returns a channel of StreamChunks.
// This is the shared SSE parsing logic used by all OpenAI-compatible providers.
// The caller is responsible for ensuring the response status is OK before calling this.
func StreamSSE(ctx context.Context, body io.ReadCloser, providerName string) <-chan llm.StreamChunk {
	ch := make(chan llm.StreamChunk)
	go func() {
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
					case ch <- llm.StreamChunk{Err: &llm.Error{
						Code: llm.ErrUpstreamError, Message: err.Error(),
						HTTPStatus: http.StatusBadGateway, Retryable: true, Provider: providerName,
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

			var oaResp providers.OpenAICompatResponse
			if err := json.Unmarshal([]byte(data), &oaResp); err != nil {
				select {
				case <-ctx.Done():
					return
				case ch <- llm.StreamChunk{Err: &llm.Error{
					Code: llm.ErrUpstreamError, Message: err.Error(),
					HTTPStatus: http.StatusBadGateway, Retryable: true, Provider: providerName,
				}}:
				}
				return
			}

			for _, choice := range oaResp.Choices {
				chunk := llm.StreamChunk{
					ID:           oaResp.ID,
					Provider:     providerName,
					Model:        oaResp.Model,
					Index:        choice.Index,
					FinishReason: choice.FinishReason,
					Delta: llm.Message{
						Role: llm.RoleAssistant,
					},
				}
				if choice.Delta != nil {
					chunk.Delta.Content = choice.Delta.Content
					if len(choice.Delta.ToolCalls) > 0 {
						chunk.Delta.ToolCalls = make([]llm.ToolCall, 0, len(choice.Delta.ToolCalls))
						for _, tc := range choice.Delta.ToolCalls {
							chunk.Delta.ToolCalls = append(chunk.Delta.ToolCalls, llm.ToolCall{
								ID:        tc.ID,
								Name:      tc.Function.Name,
								Arguments: tc.Function.Arguments,
							})
						}
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


