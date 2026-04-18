package agent

import (
	"context"
	"strings"

	"github.com/BaSui01/agentflow/types"
)

// ExecutionOptionsResolver resolves a provider-agnostic runtime view from the
// current agent configuration and request-scoped overrides.
type ExecutionOptionsResolver interface {
	Resolve(ctx context.Context, cfg types.AgentConfig, input *Input) types.ExecutionOptions
}

// DefaultExecutionOptionsResolver is the runtime's canonical resolver.
type DefaultExecutionOptionsResolver struct{}

func NewDefaultExecutionOptionsResolver() ExecutionOptionsResolver {
	return DefaultExecutionOptionsResolver{}
}

// Resolve builds execution options from AgentConfig plus context-scoped hints
// and RunConfig overrides. The returned value is detached from cfg.
func (DefaultExecutionOptionsResolver) Resolve(ctx context.Context, cfg types.AgentConfig, input *Input) types.ExecutionOptions {
	options := cfg.ExecutionOptions()
	if model, ok := types.LLMModel(ctx); ok && strings.TrimSpace(model) != "" {
		options.Model.Model = strings.TrimSpace(model)
	}
	if provider, ok := types.LLMProvider(ctx); ok && strings.TrimSpace(provider) != "" {
		options.Model.Provider = strings.TrimSpace(provider)
	}
	if routePolicy, ok := types.LLMRoutePolicy(ctx); ok && strings.TrimSpace(routePolicy) != "" {
		options.Model.RoutePolicy = strings.TrimSpace(routePolicy)
	}
	if rc := ResolveRunConfig(ctx, input); rc != nil {
		rc.ApplyToExecutionOptions(&options)
	}
	return options
}
