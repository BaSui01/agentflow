package core

import (
	"context"
	"fmt"
)

// Facade 是 workflow 对外统一执行入口。
// API/handler 层应通过该入口执行 workflow.Workflow，而不是直接操作具体 executor。
type Facade struct {
	executor *DAGExecutor
}

// NewFacade 创建 workflow 执行门面。
func NewFacade(executor *DAGExecutor) *Facade {
	return &Facade{executor: executor}
}

// ExecuteDAG 执行 DAG workflow，作为 workflow 对外统一执行入口。
func (f *Facade) ExecuteDAG(ctx context.Context, wf *DAGWorkflow, input any) (any, error) {
	if f == nil || f.executor == nil {
		return nil, fmt.Errorf("workflow facade executor is not configured")
	}
	if wf == nil {
		return nil, fmt.Errorf("dag workflow is nil")
	}
	wf.SetExecutor(f.executor)
	return wf.Execute(ctx, input)
}
