package steps

import (
	"context"
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/workflow/core"
)

// ToolStep 通过 ToolRegistry 执行工具调用。
type ToolStep struct {
	id       string
	ToolName string
	Params   map[string]any
	Registry core.ToolRegistry
}

// NewToolStep 创建工具步骤。
func NewToolStep(id string, toolName string, registry core.ToolRegistry) *ToolStep {
	return &ToolStep{id: id, ToolName: toolName, Registry: registry}
}

func (s *ToolStep) ID() string          { return s.id }
func (s *ToolStep) Type() core.StepType { return core.StepTypeTool }

func (s *ToolStep) Validate() error {
	if s.Registry == nil {
		return core.NewStepError(s.id, core.StepTypeTool, core.ErrStepNotConfigured)
	}
	if s.ToolName == "" {
		return core.NewStepError(s.id, core.StepTypeTool, fmt.Errorf("%w: tool name is empty", core.ErrStepValidation))
	}
	return nil
}

func (s *ToolStep) Execute(ctx context.Context, input core.StepInput) (core.StepOutput, error) {
	if s.Registry == nil {
		return core.StepOutput{}, core.NewStepError(s.id, core.StepTypeTool, core.ErrStepNotConfigured)
	}

	start := time.Now()

	// 合并静态参数与动态输入
	params := make(map[string]any, len(s.Params)+len(input.Data))
	for k, v := range s.Params {
		params[k] = v
	}
	for k, v := range input.Data {
		if _, exists := params[k]; !exists {
			params[k] = v
		}
	}

	result, err := s.Registry.ExecuteTool(ctx, s.ToolName, params)
	if err != nil {
		return core.StepOutput{}, core.NewStepError(s.id, core.StepTypeTool, fmt.Errorf("%w: %w", core.ErrStepExecution, err))
	}

	return core.StepOutput{
		Data:    map[string]any{"result": result},
		Latency: time.Since(start),
	}, nil
}
