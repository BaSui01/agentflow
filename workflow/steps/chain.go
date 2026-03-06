package steps

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/llm/capabilities/tools"
	"github.com/BaSui01/agentflow/workflow/core"
)

type ChainStep struct {
	id       string
	chain    tools.ToolChain
	executor *tools.ChainExecutor
}

func NewChainStep(id string, chain tools.ToolChain, executor *tools.ChainExecutor) *ChainStep {
	return &ChainStep{id: id, chain: chain, executor: executor}
}

func (s *ChainStep) ID() string          { return s.id }
func (s *ChainStep) Type() core.StepType { return core.StepTypeChain }

func (s *ChainStep) Validate() error {
	if s.executor == nil {
		return core.NewStepError(s.id, core.StepTypeChain, core.ErrStepNotConfigured)
	}
	if len(s.chain.Steps) == 0 {
		return core.NewStepError(s.id, core.StepTypeChain, fmt.Errorf("%w: chain steps is empty", core.ErrStepValidation))
	}
	return nil
}

func (s *ChainStep) Execute(ctx context.Context, input core.StepInput) (core.StepOutput, error) {
	if s.executor == nil {
		return core.StepOutput{}, core.NewStepError(s.id, core.StepTypeChain, core.ErrStepNotConfigured)
	}

	start := time.Now()
	initialInput := make(map[string]any)
	if input.Data != nil {
		for k, v := range input.Data {
			initialInput[k] = v
		}
	}

	result, err := s.executor.ExecuteChain(ctx, s.chain, initialInput)
	if err != nil {
		return core.StepOutput{}, core.NewStepError(s.id, core.StepTypeChain, fmt.Errorf("%w: %w", core.ErrStepExecution, err))
	}

	out := make(map[string]any)
	if result.FinalOutput != nil {
		var v any
		if err := json.Unmarshal(result.FinalOutput, &v); err == nil {
			out["result"] = v
		} else {
			out["result"] = string(result.FinalOutput)
		}
	}
	out["steps"] = result.Steps

	return core.StepOutput{
		Data:    out,
		Latency: time.Since(start),
	}, nil
}
