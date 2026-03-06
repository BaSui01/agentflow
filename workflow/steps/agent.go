package steps

import (
	"context"
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/workflow/core"
)

// AgentStep 通过 AgentExecutor 抽象执行 agent。
type AgentStep struct {
	id          string
	AgentID     string
	AgentModel  string
	AgentPrompt string
	AgentTools  []string
	Agent       core.AgentExecutor
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

	data := make(map[string]any)
	for k, v := range input.Data {
		data[k] = v
	}
	if s.AgentID != "" {
		if _, exists := data["agent_id"]; !exists {
			data["agent_id"] = s.AgentID
		}
	}
	if s.AgentModel != "" {
		data["agent_model"] = s.AgentModel
	}
	if s.AgentPrompt != "" {
		data["agent_prompt"] = s.AgentPrompt
	}
	if len(s.AgentTools) > 0 {
		data["agent_tools"] = s.AgentTools
	}

	result, err := s.Agent.Execute(ctx, data)
	if err != nil {
		return core.StepOutput{}, core.NewStepError(s.id, core.StepTypeAgent, fmt.Errorf("%w: %w", core.ErrStepExecution, err))
	}

	output := core.StepOutput{
		Data: map[string]any{
			"result": result.Content,
		},
		Latency: time.Since(start),
		Agent: &core.AgentExecutionMetadata{
			AgentID:      s.AgentID,
			TokensUsed:   result.TokensUsed,
			Cost:         result.Cost,
			Duration:     result.Duration,
			FinishReason: result.FinishReason,
		},
	}

	return output, nil
}
