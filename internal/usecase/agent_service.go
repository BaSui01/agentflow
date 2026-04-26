package usecase

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	discovery "github.com/BaSui01/agentflow/agent/capabilities/tools"
	agent "github.com/BaSui01/agentflow/agent/runtime"
	agentteam "github.com/BaSui01/agentflow/agent/team"
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
	maxExecuteAgentCount                 = 5
)

// AgentExecuteRequest is the request payload for agent execute/stream operations.
type AgentExecuteRequest struct {
	AgentID     string            `json:"agent_id"`
	AgentIDs    []string          `json:"agent_ids,omitempty"`
	Mode        string            `json:"mode,omitempty"`
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
	TraceID               string         `json:"trace_id"`
	Content               string         `json:"content"`
	Metadata              map[string]any `json:"metadata,omitempty"`
	TokensUsed            int            `json:"tokens_used,omitempty"`
	Cost                  float64        `json:"cost,omitempty"`
	Duration              string         `json:"duration"`
	FinishReason          string         `json:"finish_reason,omitempty"`
	CurrentStage          string         `json:"current_stage,omitempty"`
	IterationCount        int            `json:"iteration_count,omitempty"`
	SelectedReasoningMode string         `json:"selected_reasoning_mode,omitempty"`
	StopReason            string         `json:"stop_reason,omitempty"`
	CheckpointID          string         `json:"checkpoint_id,omitempty"`
	Resumable             bool           `json:"resumable"`
}

// AgentService encapsulates runtime agent resolution and endpoint availability checks.
type AgentService interface {
	ResolveForOperation(ctx context.Context, agentID string, op AgentOperation) (agent.Agent, *types.Error)
	ListAgents(ctx context.Context) ([]*discovery.AgentInfo, *types.Error)
	GetAgent(ctx context.Context, agentID string) (*discovery.AgentInfo, *types.Error)
	ExecuteAgent(ctx context.Context, req AgentExecuteRequest, traceID string) (*AgentExecuteResponse, time.Duration, *types.Error)
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

// ResolveForOperation resolves an agent for execute/stream operations.
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
	execCtx := applyAgentRoutingContext(ctx, req)
	input := toAgentInput(req, traceID)
	start := time.Now()
	output, execErr := s.executeWithResolvedAgents(execCtx, req, input)
	duration := time.Since(start)
	if execErr != nil {
		return nil, duration, ToTypesAgentError(execErr)
	}
	fields := extractExecutionFields(output)

	return &AgentExecuteResponse{
		TraceID:               output.TraceID,
		Content:               output.Content,
		Metadata:              output.Metadata,
		TokensUsed:            output.TokensUsed,
		Cost:                  output.Cost,
		Duration:              duration.String(),
		FinishReason:          output.FinishReason,
		CurrentStage:          fields.CurrentStage,
		IterationCount:        fields.IterationCount,
		SelectedReasoningMode: fields.SelectedReasoningMode,
		StopReason:            fields.StopReason,
		CheckpointID:          fields.CheckpointID,
		Resumable:             fields.Resumable,
	}, duration, nil
}

// PlanAgent remains an internal helper for package-level tests and direct usecase calls.
// It is no longer part of the public HTTP/API execution surface.
func (s *DefaultAgentService) PlanAgent(ctx context.Context, req AgentExecuteRequest, traceID string) (*agent.PlanResult, *types.Error) {
	if len(req.AgentIDs) > 0 {
		return nil, types.NewInvalidRequestError("agent_ids is not supported for planning").WithHTTPStatus(http.StatusBadRequest)
	}
	ag, err := s.ResolveForOperation(ctx, req.AgentID, AgentOperationExecute)
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
	if len(req.AgentIDs) > 0 {
		return types.NewInvalidRequestError("agent_ids is not supported for streaming").WithHTTPStatus(http.StatusBadRequest)
	}
	ag, err := s.ResolveForOperation(ctx, req.AgentID, AgentOperationStream)
	if err != nil {
		return err
	}
	streamCtx, handoffErr := s.attachRuntimeHandoffTargets(applyAgentRoutingContext(ctx, req), req, ag.ID())
	if handoffErr != nil {
		return handoffErr
	}
	streamCtx = agent.WithRuntimeStreamEmitter(streamCtx, emitter)
	_, execErr := ag.Execute(streamCtx, toAgentInput(req, traceID))
	if execErr != nil {
		return ToTypesAgentError(execErr)
	}
	return nil
}

func (s *DefaultAgentService) executeWithResolvedAgents(ctx context.Context, req AgentExecuteRequest, input *agent.Input) (*agent.Output, error) {
	agentIDs := normalizedAgentIDs(req)
	if len(req.AgentIDs) > maxExecuteAgentCount {
		return nil, types.NewInvalidRequestError("agent_ids length exceeds maximum of 5").WithHTTPStatus(http.StatusBadRequest)
	}
	if len(agentIDs) <= 1 {
		agentID := strings.TrimSpace(req.AgentID)
		if agentID == "" && len(agentIDs) == 1 {
			agentID = agentIDs[0]
		}
		ag, err := s.ResolveForOperation(ctx, agentID, AgentOperationExecute)
		if err != nil {
			return nil, err
		}
		ctx, handoffErr := s.attachRuntimeHandoffTargets(ctx, req, ag.ID())
		if handoffErr != nil {
			return nil, handoffErr
		}
		return ag.Execute(ctx, input)
	}

	agents := make([]agent.Agent, 0, len(agentIDs))
	for _, agentID := range agentIDs {
		ag, err := s.ResolveForOperation(ctx, agentID, AgentOperationExecute)
		if err != nil {
			return nil, err
		}
		agents = append(agents, ag)
	}

	mode := normalizedExecutionMode(req)
	return agentteam.ExecuteAgents(ctx, mode, agents, input)
}

func (s *DefaultAgentService) attachRuntimeHandoffTargets(ctx context.Context, req AgentExecuteRequest, sourceAgentID string) (context.Context, *types.Error) {
	targetIDs, err := handoffAgentIDsFromRequest(req.Context)
	if err != nil {
		return ctx, types.NewInvalidRequestError(err.Error()).WithHTTPStatus(http.StatusBadRequest)
	}
	targetIDs = mergeHandoffAgentIDs(targetIDs, handoffAgentIDsFromConfig(ctx, s, req.AgentID, sourceAgentID))
	if len(targetIDs) == 0 {
		return ctx, nil
	}
	if s.resolver == nil {
		return ctx, types.NewInternalError("handoff_agents requires an agent resolver").WithHTTPStatus(http.StatusNotImplemented)
	}

	sourceAgentID = strings.TrimSpace(sourceAgentID)
	targets := make([]agent.RuntimeHandoffTarget, 0, len(targetIDs))
	seen := make(map[string]struct{}, len(targetIDs))
	for _, targetID := range targetIDs {
		targetID = strings.TrimSpace(targetID)
		if targetID == "" || targetID == sourceAgentID {
			continue
		}
		if _, exists := seen[targetID]; exists {
			continue
		}
		seen[targetID] = struct{}{}
		target, resolveErr := s.resolver(ctx, targetID)
		if resolveErr != nil {
			return ctx, types.NewNotFoundError(fmt.Sprintf("handoff agent %q not found", targetID))
		}
		targets = append(targets, agent.RuntimeHandoffTarget{Agent: target})
	}
	if len(targets) == 0 {
		return ctx, nil
	}
	return agent.WithRuntimeHandoffTargets(ctx, targets), nil
}
