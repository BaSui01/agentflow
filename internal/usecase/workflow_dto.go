package usecase

import (
	"time"

	"github.com/BaSui01/agentflow/types"
	workflow "github.com/BaSui01/agentflow/workflow/core"
)

type WorkflowBuildInput struct {
	DSL     string
	DSLFile string
	DAGJSON string
	DAGYAML string
	DAGFile string
	Source  string
}

type WorkflowExecuteInput struct {
	BuildInput WorkflowBuildInput
	Input      any
}

// WorkflowPlan is a usecase-owned wrapper around the executable workflow
// definition so handlers do not depend on workflow/core concrete types.
type WorkflowPlan struct {
	name string
	dag  *workflow.DAGWorkflow
}

func (p *WorkflowPlan) Name() string {
	if p == nil {
		return ""
	}
	return p.name
}

func newWorkflowPlan(dag *workflow.DAGWorkflow) *WorkflowPlan {
	if dag == nil {
		return nil
	}
	return &WorkflowPlan{
		name: dag.Name(),
		dag:  dag,
	}
}

type WorkflowStreamEventType string

type WorkflowStreamEvent struct {
	Type     WorkflowStreamEventType
	NodeID   string
	NodeName string
	Data     any
	Error    string
}

type WorkflowStreamEmitter func(WorkflowStreamEvent)

type WorkflowNodeEvent struct {
	Type       types.WorkflowNodeEventType
	TraceID    string
	RunID      string
	WorkflowID string
	NodeID     string
	NodeType   string
	LatencyMs  int64
	Error      string
	Timestamp  time.Time
}

type WorkflowNodeEventEmitter func(WorkflowNodeEvent)

type WorkflowDSLValidationResult struct {
	Valid  bool
	Name   string
	Errors []string
}
