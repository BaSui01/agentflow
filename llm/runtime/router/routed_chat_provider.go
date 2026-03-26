package router

import (
	"context"
	"strings"

	llmroot "github.com/BaSui01/agentflow/llm"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// RoutedChatProviderOptions controls routed provider behavior.
type RoutedChatProviderOptions struct {
	DefaultStrategy RoutingStrategy
	Fallback        Provider
	Logger          *zap.Logger
	TierRouter      *TierRouter
}

// RoutedChatProvider routes chat requests to providers selected by MultiProviderRouter.
type RoutedChatProvider struct {
	router          *MultiProviderRouter
	defaultStrategy RoutingStrategy
	fallback        Provider
	logger          *zap.Logger
	tierRouter      *TierRouter
}

// NewRoutedChatProvider creates a routed provider entrypoint.
func NewRoutedChatProvider(router *MultiProviderRouter, opts RoutedChatProviderOptions) *RoutedChatProvider {
	logger := opts.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	defaultStrategy := opts.DefaultStrategy
	if defaultStrategy == "" {
		defaultStrategy = StrategyQPSBased
	}
	return &RoutedChatProvider{
		router:          router,
		defaultStrategy: defaultStrategy,
		fallback:        opts.Fallback,
		logger:          logger,
		tierRouter:      opts.TierRouter,
	}
}

func (p *RoutedChatProvider) Completion(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	if req == nil {
		return nil, types.NewInvalidRequestError("chat request is required")
	}

	selection, err := p.selectProvider(ctx, req)
	if err != nil {
		if p.canFallback(req) {
			return p.fallback.Completion(ctx, req)
		}
		return nil, err
	}

	resolvedModel := firstNonEmpty(selection.RemoteModel, req.Model)
	llmroot.RecordResolvedProviderCall(ctx, llmroot.ResolvedProviderCall{
		Provider: selection.ProviderCode,
		Model:    resolvedModel,
		BaseURL:  selection.BaseURL,
	})
	routedReq := cloneChatRequest(req, resolvedModel)
	resp, callErr := selection.Provider.Completion(ctx, routedReq)
	if callErr != nil {
		p.recordAPIKeyUsage(ctx, selection, false, callErr.Error())
		return nil, callErr
	}
	p.recordAPIKeyUsage(ctx, selection, true, "")
	if resp != nil {
		if strings.TrimSpace(resp.Provider) == "" {
			resp.Provider = selection.ProviderCode
		}
		if strings.TrimSpace(resp.Model) == "" {
			resp.Model = req.Model
		}
	}
	return resp, nil
}

func (p *RoutedChatProvider) Stream(ctx context.Context, req *ChatRequest) (<-chan StreamChunk, error) {
	if req == nil {
		return nil, types.NewInvalidRequestError("chat request is required")
	}

	selection, err := p.selectProvider(ctx, req)
	if err != nil {
		if p.canFallback(req) {
			return p.fallback.Stream(ctx, req)
		}
		return nil, err
	}

	resolvedModel := firstNonEmpty(selection.RemoteModel, req.Model)
	llmroot.RecordResolvedProviderCall(ctx, llmroot.ResolvedProviderCall{
		Provider: selection.ProviderCode,
		Model:    resolvedModel,
		BaseURL:  selection.BaseURL,
	})
	routedReq := cloneChatRequest(req, resolvedModel)
	source, streamErr := selection.Provider.Stream(ctx, routedReq)
	if streamErr != nil {
		p.recordAPIKeyUsage(ctx, selection, false, streamErr.Error())
		return nil, streamErr
	}

	out := make(chan StreamChunk)
	go func() {
		defer close(out)
		success := true
		errMsg := ""
		for chunk := range source {
			if chunk.Err != nil {
				success = false
				errMsg = chunk.Err.Error()
			}
			if strings.TrimSpace(chunk.Provider) == "" {
				chunk.Provider = selection.ProviderCode
			}
			if strings.TrimSpace(chunk.Model) == "" {
				chunk.Model = req.Model
			}
			out <- chunk
		}
		p.recordAPIKeyUsage(ctx, selection, success, errMsg)
	}()
	return out, nil
}

func (p *RoutedChatProvider) HealthCheck(ctx context.Context) (*HealthStatus, error) {
	if p.fallback != nil {
		return p.fallback.HealthCheck(ctx)
	}
	return &HealthStatus{Healthy: true}, nil
}

func (p *RoutedChatProvider) Name() string {
	return "multi-provider-router"
}

func (p *RoutedChatProvider) SupportsNativeFunctionCalling() bool {
	if p.fallback != nil {
		return p.fallback.SupportsNativeFunctionCalling()
	}
	return true
}

func (p *RoutedChatProvider) ListModels(ctx context.Context) ([]Model, error) {
	if p.fallback != nil {
		return p.fallback.ListModels(ctx)
	}
	return nil, nil
}

func (p *RoutedChatProvider) Endpoints() ProviderEndpoints {
	if p.fallback != nil {
		return p.fallback.Endpoints()
	}
	return ProviderEndpoints{}
}

func (p *RoutedChatProvider) selectProvider(ctx context.Context, req *ChatRequest) (*ProviderSelection, error) {
	if p.router == nil {
		return nil, types.NewServiceUnavailableError("multi-provider router is not configured")
	}

	model := strings.TrimSpace(req.Model)
	if model == "" {
		return nil, types.NewInvalidRequestError("model is required")
	}

	if p.tierRouter != nil {
		model = p.tierRouter.ResolveModel(req)
	}

	providerHint := extractProviderHint(req)
	strategy := extractRoutingStrategy(req, p.defaultStrategy)

	if providerHint != "" {
		return p.router.SelectProviderByCodeWithModel(ctx, providerHint, model, strategy)
	}
	return p.router.SelectProviderWithModel(ctx, model, strategy)
}

func (p *RoutedChatProvider) canFallback(req *ChatRequest) bool {
	return p.fallback != nil && extractProviderHint(req) == ""
}

func (p *RoutedChatProvider) recordAPIKeyUsage(ctx context.Context, selection *ProviderSelection, success bool, errMsg string) {
	if p.router == nil || selection == nil || selection.ProviderID == 0 || selection.APIKeyID == 0 {
		return
	}
	if err := p.router.RecordAPIKeyUsage(ctx, selection.ProviderID, selection.APIKeyID, success, errMsg); err != nil {
		p.logger.Warn("failed to record api key usage",
			zap.Uint("provider_id", selection.ProviderID),
			zap.Uint("api_key_id", selection.APIKeyID),
			zap.Bool("success", success),
			zap.Error(err))
	}
}

func extractProviderHint(req *ChatRequest) string {
	if req == nil || len(req.Metadata) == 0 {
		return ""
	}
	candidates := []string{
		req.Metadata[llmcore.MetadataKeyChatProvider],
		req.Metadata["provider"],
		req.Metadata["provider_hint"],
	}
	for _, candidate := range candidates {
		if v := strings.TrimSpace(candidate); v != "" {
			return v
		}
	}
	return ""
}

func extractRoutingStrategy(req *ChatRequest, defaultStrategy RoutingStrategy) RoutingStrategy {
	if req == nil || len(req.Metadata) == 0 {
		return defaultStrategy
	}
	raw := strings.ToLower(strings.TrimSpace(req.Metadata["route_policy"]))
	switch raw {
	case "", "balanced":
		return defaultStrategy
	case "cost", "cost_first":
		return StrategyCostBased
	case "health", "health_first":
		return StrategyHealthBased
	case "qps":
		return StrategyQPSBased
	case "latency", "latency_first":
		return StrategyLatencyBased
	default:
		return defaultStrategy
	}
}

func cloneChatRequest(req *ChatRequest, model string) *ChatRequest {
	if req == nil {
		return nil
	}
	cloned := *req
	cloned.Model = model
	return &cloned
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
