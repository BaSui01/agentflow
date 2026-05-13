// =============================================================================
// AgentFlow Anthropic-Compatible Provider Base
// =============================================================================
// Shared implementation for all Anthropic Messages API-compatible LLM providers.
// Providers that implement the Anthropic Messages API format (e.g., DeepSeek's
// https://api.deepseek.com/anthropic endpoint) embed this and only override
// what differs (Name, BaseURL, default model, headers).
// =============================================================================

package anthropiccompat

import (
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

	llm "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/llm/middleware"
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/BaSui01/agentflow/pkg/tlsutil"
	"go.uber.org/zap"
)

// Config holds the configuration for an Anthropic-compatible provider.
type Config struct {
	// ProviderName is the unique identifier for this provider.
	ProviderName string

	// APIKey is the authentication key for the provider's API.
	APIKey string

	// APIKeys is a list of API keys for round-robin usage. Takes priority over APIKey.
	APIKeys []providers.APIKeyEntry

	// BaseURL is the base URL for the provider's API (e.g., "https://api.deepseek.com/anthropic").
	BaseURL string

	// DefaultModel is the model to use when none is specified in the request.
	DefaultModel string

	// FallbackModel is used when both request and DefaultModel are empty.
	FallbackModel string

	// Timeout is the HTTP client timeout. Defaults to 60s if zero.
	Timeout time.Duration

	// EndpointPath is the Messages API endpoint path. Defaults to "/v1/messages".
	EndpointPath string

	// ModelsEndpoint is the models list endpoint path. Defaults to "/v1/models".
	ModelsEndpoint string

	// BuildHeaders is an optional function to set custom headers on each request.
	// If nil, the default "x-api-key: <apiKey>" header is used.
	BuildHeaders func(req *http.Request, apiKey string)

	// RequestHook is an optional function to modify the request body before sending.
	RequestHook func(req *llm.ChatRequest, body *providerbase.AnthropicCompatRequest)

	// ValidateRequest is an optional function to reject incompatible request/model combinations.
	ValidateRequest func(req *llm.ChatRequest, body *providerbase.AnthropicCompatRequest) error

	// SupportsTools indicates whether this provider supports native function calling.
	// Defaults to true if not set.
	SupportsTools *bool

	// AuthHeaderName custom auth header name. Empty means "x-api-key" with value.
	AuthHeaderName string
}

// Provider is the base implementation for all Anthropic Messages API-compatible LLM providers.
// Embed this in your provider struct and override Name() if needed.
type Provider struct {
	Cfg           Config
	Client        *http.Client
	Logger        *zap.Logger
	RewriterChain *middleware.RewriterChain
	keyIndex      uint64 // round-robin index for multi-key
}

// New creates a new Anthropic-compatible provider with the given config.
func New(cfg Config, logger *zap.Logger) *Provider {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	if cfg.EndpointPath == "" {
		cfg.EndpointPath = "/v1/messages"
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
			middleware.NewXMLToolRewriter(),
			middleware.NewEmptyToolsCleaner(),
		),
	}
}

// Name returns the provider name.
func (p *Provider) Name() string { return p.Cfg.ProviderName }

// SupportsStructuredOutput returns false because Anthropic compat providers
// do not support native JSON Schema response_format (they use tool_use).
func (p *Provider) SupportsStructuredOutput() bool { return false }

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
func (p *Provider) ResolveAPIKey(ctx context.Context) string {
	return p.resolveAPIKey(ctx)
}

// BaseParams returns shared Anthropic-compatible transport parameters for
// provider-local capability adapters.
func (p *Provider) BaseParams(ctx context.Context) providerbase.AnthropicCompatParams {
	return providerbase.AnthropicCompatParams{
		Client:           p.Client,
		BaseURL:          p.Cfg.BaseURL,
		APIKey:           p.ResolveAPIKey(ctx),
		ProviderName:     p.Name(),
		BuildHeadersFunc: p.ApplyHeaders,
	}
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
		req.Header.Set("x-api-key", apiKey)
		req.Header.Set("anthropic-version", "2023-06-01")
	}
	req.Header.Set("Content-Type", "application/json")
}

// resolveAPIKey returns the API key, checking for context override first.
func (p *Provider) resolveAPIKey(ctx context.Context) string {
	// 1. Context credential override
	if c, ok := llm.CredentialOverrideFromContext(ctx); ok {
		if strings.TrimSpace(c.APIKey) != "" {
			return strings.TrimSpace(c.APIKey)
		}
	}
	// 2. Multi-key round-robin
	if len(p.Cfg.APIKeys) > 0 {
		idx := atomic.AddUint64(&p.keyIndex, 1) - 1
		return p.Cfg.APIKeys[idx%uint64(len(p.Cfg.APIKeys))].Key
	}
	// 3. Single key
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
	return providerbase.ListModelsAnthropicCompat(
		ctx, p.Client, p.Cfg.BaseURL, apiKey, p.Cfg.ProviderName,
		p.Cfg.ModelsEndpoint, p.buildHeaders,
	)
}

// Endpoints returns the full API endpoint URLs used by this provider.
func (p *Provider) Endpoints() llm.ProviderEndpoints {
	return llm.ProviderEndpoints{
		Completion: p.endpoint(p.Cfg.EndpointPath),
		Models:     p.endpoint(p.Cfg.ModelsEndpoint),
		BaseURL:    p.Cfg.BaseURL,
	}
}

// buildRequestBody constructs the common Anthropic-compatible request body.
func (p *Provider) buildRequestBody(req *llm.ChatRequest, isStream bool) (providerbase.AnthropicCompatRequest, error) {
	model := providerbase.ChooseModel(req, p.Cfg.DefaultModel, p.Cfg.FallbackModel)
	system, messages := providerbase.ConvertMessagesToAnthropic(req.Messages)
	tools := providerbase.ConvertToolsToAnthropic(req.Tools)

	body := providerbase.AnthropicCompatRequest{
		Model:     model,
		Messages:  messages,
		System:    system,
		MaxTokens: chooseMaxTokens(req),
		Tools:     tools,
		Stream:    isStream,
	}

	if req.Temperature != 0 {
		t := float64(req.Temperature)
		body.Temperature = &t
	}
	if req.TopP != 0 {
		t := float64(req.TopP)
		body.TopP = &t
	}
	if len(req.Stop) > 0 {
		body.StopSequences = req.Stop
	}
	if req.ToolChoice != nil {
		body.ToolChoice = providerbase.NormalizeAnthropicToolChoice(req.ToolChoice)
	}

	// Handle thinking mode
	if tt := resolveAnthropicCompatThinkingMode(req); tt != "" && tt != "disabled" {
		maxTok := chooseMaxTokens(req)
		budget := maxTok * 3 / 4
		if budget < 1024 {
			budget = 1024
		}
		if budget >= maxTok {
			budget = maxTok - 1
		}
		body.Thinking = providerbase.NormalizeAnthropicThinking(tt, budget)
	}

	if p.Cfg.ValidateRequest != nil {
		if err := p.Cfg.ValidateRequest(req, &body); err != nil {
			return providerbase.AnthropicCompatRequest{}, err
		}
	}
	if p.Cfg.RequestHook != nil {
		p.Cfg.RequestHook(req, &body)
	}
	return body, nil
}

// Completion performs a non-streaming chat completion.
func (p *Provider) Completion(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	rewrittenReq, err := p.RewriterChain.Execute(ctx, req)
	if err != nil {
		return nil, providerbase.RewriteChainError(err, p.Name())
	}
	req = rewrittenReq

	apiKey := p.resolveAPIKey(ctx)
	body, err := p.buildRequestBody(req, false)
	if err != nil {
		return nil, err
	}

	var ar providerbase.AnthropicCompatResponse
	if err := p.DoJSON(ctx, http.MethodPost, p.Cfg.EndpointPath, body, apiKey, &ar); err != nil {
		return nil, err
	}

	return providerbase.ToLLMChatResponseFromAnthropic(ar, p.Name()), nil
}

// Stream performs a streaming chat completion via SSE.
func (p *Provider) Stream(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	rewrittenReq, err := p.RewriterChain.Execute(ctx, req)
	if err != nil {
		return nil, providerbase.RewriteChainError(err, p.Name())
	}
	req = rewrittenReq

	apiKey := p.resolveAPIKey(ctx)
	body, err := p.buildRequestBody(req, true)
	if err != nil {
		return nil, err
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

	return providerbase.StreamAnthropicSSE(ctx, resp.Body, p.Name()), nil
}

// =============================================================================
// Internal helpers
// =============================================================================

func chooseMaxTokens(req *llm.ChatRequest) int {
	if req != nil && req.MaxTokens > 0 {
		return req.MaxTokens
	}
	// Anthropic Messages API requires max_tokens
	return 4096
}

func resolveAnthropicCompatThinkingMode(req *llm.ChatRequest) string {
	if req == nil {
		return ""
	}
	// ThinkingType takes priority over legacy ReasoningMode
	if tt := strings.ToLower(strings.TrimSpace(req.ThinkingType)); tt != "" {
		return tt
	}
	return strings.ToLower(strings.TrimSpace(req.ReasoningMode))
}
