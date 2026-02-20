package workflow

import (
	"context"
	"fmt"
)

// Workflow 工作流接口
// Workflow 是预定义的步骤序列，提供可预测和一致的执行
type Workflow interface {
	// Execute 执行工作流
	Execute(ctx context.Context, input any) (any, error)
	// Name 返回工作流名称
	Name() string
	// Description 返回工作流描述
	Description() string
}

// Step 工作流步骤接口
type Step interface {
	// Execute 执行步骤
	Execute(ctx context.Context, input any) (any, error)
	// Name 返回步骤名称
	Name() string
}

// StepFunc 步骤函数类型
type StepFunc func(ctx context.Context, input any) (any, error)

// FuncStep 函数步骤实现
type FuncStep struct {
	name string
	fn   StepFunc
}

// NewFuncStep 创建函数步骤
func NewFuncStep(name string, fn StepFunc) *FuncStep {
	return &FuncStep{
		name: name,
		fn:   fn,
	}
}

func (s *FuncStep) Execute(ctx context.Context, input any) (any, error) {
	return s.fn(ctx, input)
}

func (s *FuncStep) Name() string {
	return s.name
}

// ChainWorkflow 提示词链工作流
// 将任务分解为固定的步骤序列，每个步骤处理前一步的输出
type ChainWorkflow struct {
	name        string
	description string
	steps       []Step
}

// NewChainWorkflow 创建提示词链工作流
func NewChainWorkflow(name, description string, steps ...Step) *ChainWorkflow {
	return &ChainWorkflow{
		name:        name,
		description: description,
		steps:       steps,
	}
}

// Execute 执行提示词链
// 按顺序执行每个步骤，将前一步的输出作为下一步的输入
func (w *ChainWorkflow) Execute(ctx context.Context, input any) (any, error) {
	current := input

	for i, step := range w.steps {
		// 检查上下文是否已取消
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// 执行步骤
		result, err := step.Execute(ctx, current)
		if err != nil {
			return nil, fmt.Errorf("step %d (%s) failed: %w", i+1, step.Name(), err)
		}

		current = result
	}

	return current, nil
}

func (w *ChainWorkflow) Name() string {
	return w.name
}

func (w *ChainWorkflow) Description() string {
	return w.description
}

// AddStep 添加步骤
func (w *ChainWorkflow) AddStep(step Step) {
	w.steps = append(w.steps, step)
}

// Steps 返回所有步骤
func (w *ChainWorkflow) Steps() []Step {
	return w.steps
}
