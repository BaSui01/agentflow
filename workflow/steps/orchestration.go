package steps

import (
	"context"
	"fmt"
	"time"

	agent "github.com/BaSui01/agentflow/agent/runtime"
	"github.com/BaSui01/agentflow/agent/team"
	"github.com/BaSui01/agentflow/workflow/core"
	"go.uber.org/zap"
)

type AgentResolver interface {
	ResolveAgent(ctx context.Context, agentID string) (agent.Agent, error)
}

type OrchestrationStep struct {
	id        string
	Mode      string
	AgentIDs  []string
	MaxRounds int
	Timeout   time.Duration
	resolver  AgentResolver
	executor  team.ModeExecutor
	logger    *zap.Logger
}

func NewOrchestrationStep(id string, resolver AgentResolver, executor team.ModeExecutor, logger *zap.Logger) *OrchestrationStep {
	if logger == nil {
		logger = zap.NewNop()
	}
	if executor == nil {
		executor = team.GlobalModeExecutor()
	}
	return &OrchestrationStep{
		id:       id,
		resolver: resolver,
		executor: executor,
		logger:   logger,
	}
}

func (s *OrchestrationStep) ID() string          { return s.id }
func (s *OrchestrationStep) Type() core.StepType { return core.StepTypeOrchestration }

func (s *OrchestrationStep) Validate() error {
	if s.resolver == nil {
		return core.NewStepError(s.id, core.StepTypeOrchestration, core.ErrStepNotConfigured)
	}
	if s.Mode == "" {
		return core.NewStepError(s.id, core.StepTypeOrchestration, fmt.Errorf("%w: mode is empty", core.ErrStepValidation))
	}
	if len(s.AgentIDs) == 0 {
		return core.NewStepError(s.id, core.StepTypeOrchestration, fmt.Errorf("%w: agent_ids is empty", core.ErrStepValidation))
	}
	return nil
}

func (s *OrchestrationStep) Execute(ctx context.Context, input core.StepInput) (core.StepOutput, error) {
	if s.resolver == nil {
		return core.StepOutput{}, core.NewStepError(s.id, core.StepTypeOrchestration, core.ErrStepNotConfigured)
	}

	agents := make([]agent.Agent, 0, len(s.AgentIDs))
	for _, id := range s.AgentIDs {
		a, err := s.resolver.ResolveAgent(ctx, id)
		if err != nil {
			return core.StepOutput{}, core.NewStepError(s.id, core.StepTypeOrchestration, fmt.Errorf("%w: resolve agent %q: %w", core.ErrStepExecution, id, err))
		}
		agents = append(agents, a)
	}

	content := ""
	if input.Data != nil {
		if c, ok := input.Data["content"].(string); ok {
			content = c
		} else if c, ok := input.Data["input"].(string); ok {
			content = c
		} else if c, ok := input.Data["result"].(string); ok {
			content = c
		} else if c, ok := input.Data["prompt"].(string); ok {
			content = c
		}
	}
	agentInput := &agent.Input{
		TraceID: input.Metadata["trace_id"],
		Content: content,
		Context: map[string]any{},
	}
	if s.MaxRounds > 0 {
		agentInput.Context["max_rounds"] = s.MaxRounds
	}
	if input.Data != nil {
		for k, v := range input.Data {
			if k != "content" && k != "input" && k != "result" && k != "prompt" {
				agentInput.Context[k] = v
			}
		}
	}

	if s.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.Timeout)
		defer cancel()
	}

	start := time.Now()
	out, err := s.executor.Execute(ctx, s.Mode, agents, agentInput)
	if err != nil {
		return core.StepOutput{}, core.NewStepError(s.id, core.StepTypeOrchestration, fmt.Errorf("%w: %w", core.ErrStepExecution, err))
	}

	return core.StepOutput{
		Data: map[string]any{
			"result":   out.Content,
			"metadata": out.Metadata,
		},
		Latency: time.Since(start),
	}, nil
}
