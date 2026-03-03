package handlers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/agent/discovery"
	"github.com/BaSui01/agentflow/types"
)

// AgentOperation identifies the business intent for resolver fallback messages.
type AgentOperation string

const (
	AgentOperationExecute AgentOperation = "execution"
	AgentOperationStream  AgentOperation = "streaming"
	AgentOperationPlan    AgentOperation = "planning"
)

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

	input := toAgentInput(req, traceID)
	start := time.Now()
	output, execErr := ag.Execute(ctx, input)
	duration := time.Since(start)
	if execErr != nil {
		return nil, duration, toTypesAgentError(execErr)
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
	plan, planErr := ag.Plan(ctx, toAgentInput(req, traceID))
	if planErr != nil {
		return nil, toTypesAgentError(planErr)
	}
	return plan, nil
}

func (s *DefaultAgentService) ExecuteAgentStream(ctx context.Context, req AgentExecuteRequest, traceID string, emitter agent.RuntimeStreamEmitter) *types.Error {
	ag, err := s.ResolveForOperation(ctx, req.AgentID, AgentOperationStream)
	if err != nil {
		return err
	}
	streamCtx := agent.WithRuntimeStreamEmitter(ctx, emitter)
	_, execErr := ag.Execute(streamCtx, toAgentInput(req, traceID))
	if execErr != nil {
		return toTypesAgentError(execErr)
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

func toTypesAgentError(err error) *types.Error {
	if err == nil {
		return nil
	}
	if typedErr, ok := err.(*types.Error); ok {
		return typedErr
	}
	return types.NewInternalError("agent operation failed").WithCause(err)
}
