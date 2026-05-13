// =============================================================================
// AgentFlow Gemini-Compatible Provider Base
// =============================================================================
// Shared implementation for all Gemini generateContent API-compatible LLM providers.
// Providers that implement the Gemini generateContent API format embed this and
// only override what differs (Name, BaseURL, default model, headers).
// =============================================================================

package geminicompat

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

// Config holds the configuration for a Gemini-compatible provider.
type Config struct {
	// ProviderName is the unique identifier for this provider.
	ProviderName string

	// APIKey is the authentication key for the provider's API.
	APIKey string

	// APIKeys is a list of API keys for round-robin usage. Takes priority over APIKey.
	APIKeys []providers.APIKeyEntry

	// BaseURL is the base URL for the provider's API (e.g., "https://generativelanguage.googleapis.com").
	BaseURL string

	// DefaultModel is the model to use when none is specified in the request.
	DefaultModel string

	// FallbackModel is used when both request and DefaultModel are empty.
	FallbackModel string

	// Timeout is the HTTP client timeout. Defaults to 60s if zero.
	Timeout time.Duration

	// ModelsEndpoint is the models list endpoint path. Defaults to "/v1beta/models".
	ModelsEndpoint string

	// BuildHeaders is an optional function to set custom headers on each request.
	// If nil, the default "x-goog-api-key: <apiKey>" header is used.
	BuildHeaders func(req *http.Request, apiKey string)

	// RequestHook is an optional function to modify the request body before sending.
	RequestHook func(req *llm.ChatRequest, body *providerbase.GeminiCompatRequest)

	// ValidateRequest is an optional function to reject incompatible request/model combinations.
	ValidateRequest func(req *llm.ChatRequest, body *providerbase.GeminiCompatRequest) error

	// SupportsTools indicates whether this provider supports native function calling.
	// Defaults to true if not set.
	SupportsTools *bool

	// AuthHeaderName custom auth header name. Empty means "x-goog-api-key".
	AuthHeaderName string
}

// Provider is the base implementation for all Gemini generateContent API-compatible LLM providers.
// Embed this in your provider struct and override Name() if needed.
type Provider struct {
	Cfg           Config
	Client        *http.Client
	Logger        *zap.Logger
	RewriterChain *middleware.RewriterChain
	keyIndex      uint64 // round-robin index for multi-key
}

// New creates a new Gemini-compatible provider with the given config.
func New(cfg Config, logger *zap.Logger) *Provider {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	if cfg.ModelsEndpoint == "" {
		cfg.ModelsEndpoint = "/v1beta/models"
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

// SupportsStructuredOutput returns true because Gemini compat providers
// support structured output via responseMimeType + responseSchema.
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
func (p *Provider) ResolveAPIKey(ctx context.Context) string {
	return p.resolveAPIKey(ctx)
}

// BaseParams returns shared Gemini-compatible transport parameters for
// provider-local capability adapters.
func (p *Provider) BaseParams(ctx context.Context) providerbase.GeminiCompatParams {
	return providerbase.GeminiCompatParams{
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
		req.Header.Set("x-goog-api-key", apiKey)
	}
	req.Header.Set("Content-Type", "application/json")
}

// resolveAPIKey returns the API key, checking for context override first.
func (p *Provider) resolveAPIKey(ctx context.Context) string {
	if c, ok := llm.CredentialOverrideFromContext(ctx); ok {
		if strings.TrimSpace(c.APIKey) != "" {
			return strings.TrimSpace(c.APIKey)
		}
	}
	if len(p.Cfg.APIKeys) > 0 {
		idx := atomic.AddUint64(&p.keyIndex, 1) - 1
		return p.Cfg.APIKeys[idx%uint64(len(p.Cfg.APIKeys))].Key
	}
	return p.Cfg.APIKey
}

// endpoint builds the full URL for a given path.
func (p *Provider) endpoint(path string) string {
	return fmt.Sprintf("%s%s", strings.TrimRight(p.Cfg.BaseURL, "/"), path)
}

// completionEndpoint builds the generateContent endpoint for a specific model.
func (p *Provider) completionEndpoint(model string) string {
	return fmt.Sprintf("%s/v1beta/models/%s:generateContent", strings.TrimRight(p.Cfg.BaseURL, "/"), model)
}

// streamEndpoint builds the streamGenerateContent endpoint for a specific model.
func (p *Provider) streamEndpoint(model string) string {
	return fmt.Sprintf("%s/v1beta/models/%s:streamGenerateContent?alt=sse", strings.TrimRight(p.Cfg.BaseURL, "/"), model)
}

// NewRequest builds an HTTP request with provider-specific headers applied.
func (p *Provider) NewRequest(ctx context.Context, method, url string, body io.Reader, apiKey string) (*http.Request, error) {
	httpReq, err := http.NewRequestWithContext(ctx, method, url, body)
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
func (p *Provider) DoJSON(ctx context.Context, method, url string, payload any, apiKey string, out any) error {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to marshal request: %w", err)
		}
		body = bytes.NewReader(data)
	}

	httpReq, err := p.NewRequest(ctx, method, url, body, apiKey)
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
	return providerbase.ListModelsGeminiCompat(
		ctx, p.Client, p.Cfg.BaseURL, apiKey, p.Cfg.ProviderName,
		p.Cfg.ModelsEndpoint, p.buildHeaders,
	)
}

// Endpoints returns the full API endpoint URLs used by this provider.
func (p *Provider) Endpoints() llm.ProviderEndpoints {
	model := p.Cfg.DefaultModel
	if model == "" {
		model = p.Cfg.FallbackModel
	}
	if model == "" {
		model = "gemini-2.5-flash"
	}
	return llm.ProviderEndpoints{
		Completion: p.completionEndpoint(model),
		Stream:     p.streamEndpoint(model),
		Models:     p.endpoint(p.Cfg.ModelsEndpoint),
		BaseURL:    p.Cfg.BaseURL,
	}
}

// buildRequestBody constructs the common Gemini-compatible request body.
func (p *Provider) buildRequestBody(req *llm.ChatRequest, isStream bool) (providerbase.GeminiCompatRequest, error) {
	model := providerbase.ChooseModel(req, p.Cfg.DefaultModel, p.Cfg.FallbackModel)
	systemInstruction, contents := providerbase.ConvertMessagesToGemini(req.Messages)
	tools := providerbase.ConvertToolsToGemini(req.Tools, req.WebSearchOptions)

	body := providerbase.GeminiCompatRequest{
		Contents:          contents,
		SystemInstruction: systemInstruction,
		Tools:             tools,
	}

	// Generation config
	genConfig := &providerbase.GeminiCompatGenerationConfig{}
	if req.Temperature != 0 {
		genConfig.Temperature = &req.Temperature
	}
	if req.TopP != 0 {
		genConfig.TopP = &req.TopP
	}
	if req.MaxTokens > 0 {
		genConfig.MaxOutputTokens = int32(req.MaxTokens)
	}
	if len(req.Stop) > 0 {
		genConfig.StopSequences = req.Stop
	}

	// Response format
	if req.ResponseFormat != nil {
		switch req.ResponseFormat.Type {
		case llm.ResponseFormatJSONObject:
			genConfig.ResponseMimeType = "application/json"
		case llm.ResponseFormatJSONSchema:
			genConfig.ResponseMimeType = "application/json"
			if req.ResponseFormat.JSONSchema != nil {
				genConfig.ResponseSchema = req.ResponseFormat.JSONSchema.Schema
			}
		}
	}

	// Thinking config
	if tt := resolveGeminiCompatThinkingConfig(req); tt != nil {
		genConfig.ThinkingConfig = tt
	}

	if !isEmptyGeminiGenerationConfig(genConfig) {
		body.GenerationConfig = genConfig
	} else {
		body.GenerationConfig = nil
	}

	// Tool config
	if req.ToolChoice != nil {
		body.ToolConfig = providerbase.ConvertToolChoiceToGemini(req.ToolChoice)
	}

	if p.Cfg.ValidateRequest != nil {
		if err := p.Cfg.ValidateRequest(req, &body); err != nil {
			return providerbase.GeminiCompatRequest{}, err
		}
	}
	if p.Cfg.RequestHook != nil {
		p.Cfg.RequestHook(req, &body)
	}

	_ = model // Used for endpoint resolution in stream/completion
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

	model := providerbase.ChooseModel(req, p.Cfg.DefaultModel, p.Cfg.FallbackModel)
	endpoint := p.completionEndpoint(model)

	var gr providerbase.GeminiCompatResponse
	if err := p.DoJSON(ctx, http.MethodPost, endpoint, body, apiKey, &gr); err != nil {
		return nil, err
	}

	return providerbase.ToLLMChatResponseFromGemini(gr, p.Name()), nil
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

	model := providerbase.ChooseModel(req, p.Cfg.DefaultModel, p.Cfg.FallbackModel)
	endpoint := p.streamEndpoint(model)

	httpReq, err := p.NewRequest(ctx, http.MethodPost, endpoint, bytes.NewReader(payload), apiKey)
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

	return providerbase.StreamGeminiSSE(ctx, resp.Body, p.Name()), nil
}

// =============================================================================
// Internal helpers
// =============================================================================

func resolveGeminiCompatThinkingConfig(req *llm.ChatRequest) *providerbase.GeminiCompatThinking {
	if req == nil {
		return nil
	}

	includeThoughts := req.IncludeThoughts
	thinkingLevel := strings.TrimSpace(req.ThinkingLevel)
	thinkingBudget := req.ThinkingBudget
	mode := strings.ToLower(strings.TrimSpace(req.ReasoningMode))

	// If no thinking-related params, check legacy mode
	if thinkingLevel == "" && thinkingBudget == nil && includeThoughts == nil && mode == "" {
		return nil
	}

	// Formal fields take priority
	if thinkingLevel != "" || thinkingBudget != nil || includeThoughts != nil {
		cfg := &providerbase.GeminiCompatThinking{}
		if includeThoughts != nil {
			cfg.IncludeThoughts = *includeThoughts
		} else {
			cfg.IncludeThoughts = true
		}
		if thinkingBudget != nil {
			cfg.ThinkingBudget = thinkingBudget
		}
		if thinkingLevel != "" {
			cfg.ThinkingLevel = thinkingLevel
		}
		return cfg
	}

	// Legacy mode fallback
	if mode != "" && mode != "disabled" {
		cfg := &providerbase.GeminiCompatThinking{
			IncludeThoughts: true,
		}
		switch mode {
		case "minimal":
			cfg.ThinkingLevel = "minimal"
		case "low":
			cfg.ThinkingLevel = "low"
		case "medium":
			cfg.ThinkingLevel = "medium"
		case "high":
			cfg.ThinkingLevel = "high"
		default:
			cfg.ThinkingLevel = "medium"
		}
		return cfg
	}

	return nil
}

func isEmptyGeminiGenerationConfig(cfg *providerbase.GeminiCompatGenerationConfig) bool {
	if cfg == nil {
		return true
	}
	return cfg.Temperature == nil &&
		cfg.TopP == nil &&
		cfg.TopK == nil &&
		cfg.MaxOutputTokens == 0 &&
		len(cfg.StopSequences) == 0 &&
		cfg.ResponseMimeType == "" &&
		cfg.ResponseSchema == nil &&
		cfg.ThinkingConfig == nil
}
