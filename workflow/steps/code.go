package steps

import (
	"context"
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/workflow/core"
)

// CodeHandler CodeStep 执行函数签名。
type CodeHandler func(ctx context.Context, input core.StepInput) (map[string]any, error)

// CodeStep 执行自定义代码逻辑。
type CodeStep struct {
	id      string
	Handler CodeHandler
}

// NewCodeStep 创建代码步骤。
func NewCodeStep(id string, handler CodeHandler) *CodeStep {
	return &CodeStep{id: id, Handler: handler}
}

func (s *CodeStep) ID() string          { return s.id }
func (s *CodeStep) Type() core.StepType { return core.StepTypeCode }

func (s *CodeStep) Validate() error {
	if s.Handler == nil {
		return core.NewStepError(s.id, core.StepTypeCode, core.ErrStepNotConfigured)
	}
	return nil
}

func (s *CodeStep) Execute(ctx context.Context, input core.StepInput) (core.StepOutput, error) {
	if s.Handler == nil {
		return core.StepOutput{}, core.NewStepError(s.id, core.StepTypeCode, core.ErrStepNotConfigured)
	}

	start := time.Now()

	data, err := s.Handler(ctx, input)
	if err != nil {
		return core.StepOutput{}, core.NewStepError(s.id, core.StepTypeCode, fmt.Errorf("%w: %w", core.ErrStepExecution, err))
	}
	if data == nil {
		data = map[string]any{}
	}

	return core.StepOutput{
		Data:    data,
		Latency: time.Since(start),
	}, nil
}
