package workflow

import (
	"context"
)

// Runnable is the common execution interface shared by workflow executable nodes.
// It represents any unit of work that can be executed with input and produce output.
type Runnable interface {
	Execute(ctx context.Context, input any) (any, error)
}

// Workflow 工作流接口
// Workflow 是预定义的步骤序列，提供可预测和一致的执行
type Workflow interface {
	Runnable
	// Name 返回工作流名称
	Name() string
	// Description 返回工作流描述
	Description() string
}

// Step 工作流步骤接口
type Step interface {
	Runnable
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

// =============================================================================
// Workflow Streaming
// =============================================================================

// WorkflowStreamEventType defines the type of workflow stream event.
type WorkflowStreamEventType string

const (
	// WorkflowEventNodeStart is emitted before a DAG node begins execution.
	WorkflowEventNodeStart WorkflowStreamEventType = "node_start"
	// WorkflowEventNodeComplete is emitted after a DAG node finishes successfully.
	WorkflowEventNodeComplete WorkflowStreamEventType = "node_complete"
	// WorkflowEventNodeError is emitted when a DAG node fails.
	WorkflowEventNodeError WorkflowStreamEventType = "node_error"
	// WorkflowEventStepProgress is emitted for intermediate step progress.
	WorkflowEventStepProgress WorkflowStreamEventType = "step_progress"
	// WorkflowEventToken is emitted for streaming token output from LLM steps.
	WorkflowEventToken WorkflowStreamEventType = "token"
)

// WorkflowStreamEvent carries information about a workflow execution event.
type WorkflowStreamEvent struct {
	Type     WorkflowStreamEventType `json:"type"`
	NodeID   string                  `json:"node_id,omitempty"`
	NodeName string                  `json:"node_name,omitempty"`
	Data     any                     `json:"data,omitempty"`
	Error    error                   `json:"-"`
}

// WorkflowStreamEmitter is a callback that receives workflow stream events.
type WorkflowStreamEmitter func(WorkflowStreamEvent)

// workflowStreamEmitterKey is the context key for WorkflowStreamEmitter.
type workflowStreamEmitterKey struct{}

// WithWorkflowStreamEmitter stores a WorkflowStreamEmitter in the context.
func WithWorkflowStreamEmitter(ctx context.Context, emitter WorkflowStreamEmitter) context.Context {
	if emitter == nil {
		return ctx
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, workflowStreamEmitterKey{}, emitter)
}

// workflowStreamEmitterFromContext retrieves the WorkflowStreamEmitter from context.
func workflowStreamEmitterFromContext(ctx context.Context) (WorkflowStreamEmitter, bool) {
	if ctx == nil {
		return nil, false
	}
	v := ctx.Value(workflowStreamEmitterKey{})
	if v == nil {
		return nil, false
	}
	emit, ok := v.(WorkflowStreamEmitter)
	return emit, ok && emit != nil
}

// WorkflowStreamEmitterFromContext exposes the workflow stream emitter lookup to
// subpackages such as workflow/steps without leaking the private context key.
func WorkflowStreamEmitterFromContext(ctx context.Context) (WorkflowStreamEmitter, bool) {
	return workflowStreamEmitterFromContext(ctx)
}
