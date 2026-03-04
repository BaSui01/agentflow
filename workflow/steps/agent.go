package steps

import (
	"context"
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/workflow/core"
)

// AgentStep 通过 AgentExecutor 抽象执行 agent。
type AgentStep struct {
	id    string
	Agent core.AgentExecutor
}

// NewAgentStep 创建 agent 步骤。
func NewAgentStep(id string, agent core.AgentExecutor) *AgentStep {
	return &AgentStep{id: id, Agent: agent}
}

func (s *AgentStep) ID() string          { return s.id }
func (s *AgentStep) Type() core.StepType { return core.StepTypeAgent }

func (s *AgentStep) Validate() error {
	if s.Agent == nil {
		return core.NewStepError(s.id, core.StepTypeAgent, core.ErrStepNotConfigured)
	}
	return nil
}

func (s *AgentStep) Execute(ctx context.Context, input core.StepInput) (core.StepOutput, error) {
	if s.Agent == nil {
		return core.StepOutput{}, core.NewStepError(s.id, core.StepTypeAgent, core.ErrStepNotConfigured)
	}

	start := time.Now()

	result, err := s.Agent.Execute(ctx, input.Data)
	if err != nil {
		return core.StepOutput{}, core.NewStepError(s.id, core.StepTypeAgent, fmt.Errorf("%w: %w", core.ErrStepExecution, err))
	}

	return core.StepOutput{
		Data: map[string]any{
			"result": result,
		},
		Latency: time.Since(start),
	}, nil
}
