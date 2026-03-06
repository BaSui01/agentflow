package usecase

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/agent/discovery"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
)

// AgentResolver resolves an agent ID to a live Agent instance.
// This decouples the handler from how agents are stored/managed at runtime.
type AgentResolver func(ctx context.Context, agentID string) (agent.Agent, error)

// AgentOperation identifies the business intent for resolver fallback messages.
type AgentOperation string

const (
	AgentOperationExecute AgentOperation = "execution"
	AgentOperationStream  AgentOperation = "streaming"
	AgentOperationPlan    AgentOperation = "planning"
)

// AgentExecuteRequest is the request payload for agent execute/plan/stream operations.
type AgentExecuteRequest struct {
	AgentID     string            `json:"agent_id"`
	Content     string            `json:"content"`
	Provider    string            `json:"provider,omitempty"`
	Model       string            `json:"model,omitempty"`
	RoutePolicy string            `json:"route_policy,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	Context     map[string]any    `json:"context,omitempty"`
	Variables   map[string]string `json:"variables,omitempty"`
}

// AgentExecuteResponse is the response payload for agent execute operations.
type AgentExecuteResponse struct {
	TraceID      string         `json:"trace_id"`
	Content      string         `json:"content"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	TokensUsed   int            `json:"tokens_used,omitempty"`
	Cost         float64        `json:"cost,omitempty"`
	Duration     string         `json:"duration"`
	FinishReason string         `json:"finish_reason,omitempty"`
}

// AgentService encapsulates runtime agent resolution and endpoint availability checks.
type AgentService interface {
	ResolveForOperation(ctx context.Context, agentID string, op AgentOperation) (agent.Agent, *types.Error)
	ListAgents(ctx context.Context) ([]*discovery.AgentInfo, *types.Error)
	GetAgent(ctx context.Context, agentID string) (*discovery.AgentInfo, *types.Error)
	ExecuteAgent(ctx context.Context, req AgentExecuteRequest, traceID string) (*AgentExecuteResponse, time.Duration, *types.Error)
	PlanAgent(ctx context.Context, req AgentExecuteRequest, traceID string) (*agent.PlanResult, *types.Error)
	ExecuteAgentStream(ctx context.Context, req AgentExecuteRequest, traceID string, emitter agent.RuntimeStreamEmitter) *types.Error
}

// DefaultAgentService is the default AgentService implementation used by AgentHandler.
type DefaultAgentService struct {
	registry discovery.Registry
	resolver AgentResolver
}

// NewDefaultAgentService constructs a service with resolver+registry fallback strategy.
func NewDefaultAgentService(registry discovery.Registry, resolver AgentResolver) *DefaultAgentService {
	return &DefaultAgentService{
		registry: registry,
		resolver: resolver,
	}
}

// ResolveForOperation resolves an agent for execute/stream/plan operations.
func (s *DefaultAgentService) ResolveForOperation(ctx context.Context, agentID string, op AgentOperation) (agent.Agent, *types.Error) {
	if s.resolver != nil {
		ag, err := s.resolver(ctx, agentID)
		if err != nil {
			return nil, types.NewNotFoundError(fmt.Sprintf("agent %q not found", agentID))
		}
		return ag, nil
	}

	// Resolver not configured. First confirm the agent exists in discovery registry.
	if s.registry != nil {
		if _, err := s.registry.GetAgent(ctx, agentID); err != nil {
			return nil, types.NewNotFoundError("agent not found")
		}
	}

	return nil, types.NewInternalError(
		fmt.Sprintf("agent %s is not configured — no agent resolver available", op)).
		WithHTTPStatus(http.StatusNotImplemented)
}

func (s *DefaultAgentService) ListAgents(ctx context.Context) ([]*discovery.AgentInfo, *types.Error) {
	if s.registry == nil {
		return nil, types.NewServiceUnavailableError("agent registry is not configured").
			WithHTTPStatus(http.StatusServiceUnavailable)
	}
	agents, err := s.registry.ListAgents(ctx)
	if err != nil {
		return nil, types.NewInternalError("failed to list agents").WithCause(err)
	}
	return agents, nil
}

func (s *DefaultAgentService) GetAgent(ctx context.Context, agentID string) (*discovery.AgentInfo, *types.Error) {
	if s.registry == nil {
		return nil, types.NewServiceUnavailableError("agent registry is not configured").
			WithHTTPStatus(http.StatusServiceUnavailable)
	}
	info, err := s.registry.GetAgent(ctx, agentID)
	if err != nil {
		return nil, types.NewNotFoundError("agent not found")
	}
	return info, nil
}

func (s *DefaultAgentService) ExecuteAgent(ctx context.Context, req AgentExecuteRequest, traceID string) (*AgentExecuteResponse, time.Duration, *types.Error) {
	ag, err := s.ResolveForOperation(ctx, req.AgentID, AgentOperationExecute)
	if err != nil {
		return nil, 0, err
	}

	execCtx := applyAgentRoutingContext(ctx, req)
	input := toAgentInput(req, traceID)
	start := time.Now()
	output, execErr := ag.Execute(execCtx, input)
	duration := time.Since(start)
	if execErr != nil {
		return nil, duration, ToTypesAgentError(execErr)
	}

	return &AgentExecuteResponse{
		TraceID:      output.TraceID,
		Content:      output.Content,
		Metadata:     output.Metadata,
		TokensUsed:   output.TokensUsed,
		Cost:         output.Cost,
		Duration:     duration.String(),
		FinishReason: output.FinishReason,
	}, duration, nil
}

func (s *DefaultAgentService) PlanAgent(ctx context.Context, req AgentExecuteRequest, traceID string) (*agent.PlanResult, *types.Error) {
	ag, err := s.ResolveForOperation(ctx, req.AgentID, AgentOperationPlan)
	if err != nil {
		return nil, err
	}
	plan, planErr := ag.Plan(applyAgentRoutingContext(ctx, req), toAgentInput(req, traceID))
	if planErr != nil {
		return nil, ToTypesAgentError(planErr)
	}
	return plan, nil
}

func (s *DefaultAgentService) ExecuteAgentStream(ctx context.Context, req AgentExecuteRequest, traceID string, emitter agent.RuntimeStreamEmitter) *types.Error {
	ag, err := s.ResolveForOperation(ctx, req.AgentID, AgentOperationStream)
	if err != nil {
		return err
	}
	streamCtx := agent.WithRuntimeStreamEmitter(applyAgentRoutingContext(ctx, req), emitter)
	_, execErr := ag.Execute(streamCtx, toAgentInput(req, traceID))
	if execErr != nil {
		return ToTypesAgentError(execErr)
	}
	return nil
}

func toAgentInput(req AgentExecuteRequest, traceID string) *agent.Input {
	return &agent.Input{
		TraceID:   traceID,
		Content:   req.Content,
		Context:   req.Context,
		Variables: req.Variables,
	}
}

// ToTypesAgentError converts an error to *types.Error when needed.
func ToTypesAgentError(err error) *types.Error {
	if err == nil {
		return nil
	}
	if typedErr, ok := err.(*types.Error); ok {
		return typedErr
	}
	return types.NewInternalError("agent operation failed").WithCause(err)
}

func applyAgentRoutingContext(ctx context.Context, req AgentExecuteRequest) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}

	rc := &agent.RunConfig{
		Metadata: NormalizeRouteMetadata(req.Metadata),
		Tags:     NormalizeRouteTags(req.Tags),
	}
	hasRunConfig := len(rc.Metadata) > 0 || len(rc.Tags) > 0

	model := strings.TrimSpace(req.Model)
	if model != "" {
		rc.Model = agent.StringPtr(model)
		ctx = types.WithLLMModel(ctx, model)
		hasRunConfig = true
	}

	provider, providerErr := NormalizeProviderHint(req.Provider)
	if providerErr == nil && provider != "" {
		rc.Provider = agent.StringPtr(provider)
		ctx = types.WithLLMProvider(ctx, provider)
		hasRunConfig = true
	}

	routePolicy, routeErr := NormalizeRoutePolicy(req.RoutePolicy)
	if routeErr == nil && routePolicy != "" {
		policy := string(routePolicy)
		rc.RoutePolicy = agent.StringPtr(policy)
		ctx = types.WithLLMRoutePolicy(ctx, policy)
		hasRunConfig = true
	}

	if provider != "" || routePolicy != "" {
		if rc.Metadata == nil {
			rc.Metadata = make(map[string]string)
		}
		if provider != "" {
			rc.Metadata[llmcore.MetadataKeyChatProvider] = provider
		}
		if routePolicy != "" {
			rc.Metadata["route_policy"] = string(routePolicy)
		}
	}

	if hasRunConfig {
		ctx = agent.WithRunConfig(ctx, rc)
	}
	return ctx
}
