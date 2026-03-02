package handlers

import (
	"context"
	"fmt"
	"net/http"

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

