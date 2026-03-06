package steps

import (
	"context"
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/workflow/core"
)

// HumanStep 通过 HumanInputHandler 请求人工输入。
type HumanStep struct {
	id        string
	Prompt    string
	InputType string
	Options   []string
	Timeout   time.Duration
	Handler   core.HumanInputHandler
}

// NewHumanStep 创建人工步骤。
func NewHumanStep(id string, handler core.HumanInputHandler) *HumanStep {
	return &HumanStep{
		id:        id,
		InputType: "text",
		Handler:   handler,
	}
}

func (s *HumanStep) ID() string          { return s.id }
func (s *HumanStep) Type() core.StepType { return core.StepTypeHuman }

func (s *HumanStep) Validate() error {
	if s.Handler == nil {
		return core.NewStepError(s.id, core.StepTypeHuman, core.ErrStepNotConfigured)
	}
	if s.InputType == "" {
		return core.NewStepError(s.id, core.StepTypeHuman, fmt.Errorf("%w: human input type is empty", core.ErrStepValidation))
	}
	return nil
}

func (s *HumanStep) Execute(ctx context.Context, input core.StepInput) (core.StepOutput, error) {
	if s.Handler == nil {
		return core.StepOutput{}, core.NewStepError(s.id, core.StepTypeHuman, core.ErrStepNotConfigured)
	}

	runCtx := ctx
	var cancel context.CancelFunc
	if s.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, s.Timeout)
		defer cancel()
	}

	start := time.Now()

	result, err := s.Handler.RequestInput(runCtx, s.Prompt, s.InputType, s.Options)
	if err != nil {
		if runCtx.Err() != nil {
			return core.StepOutput{}, core.NewStepError(s.id, core.StepTypeHuman, core.ErrStepTimeout)
		}
		return core.StepOutput{}, core.NewStepError(s.id, core.StepTypeHuman, fmt.Errorf("%w: %w", core.ErrStepExecution, err))
	}

	output := core.StepOutput{
		Data: map[string]any{
			"input":     result.Value,
			"option_id": result.OptionID,
		},
		Latency: time.Since(start),
	}

	if len(input.Metadata) > 0 {
		output.Data["metadata"] = input.Metadata
	}

	return output, nil
}
