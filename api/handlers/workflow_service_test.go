package handlers

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/types"
	"github.com/BaSui01/agentflow/workflow"
	"github.com/BaSui01/agentflow/workflow/dsl"
	workflowobs "github.com/BaSui01/agentflow/workflow/observability"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type workflowExecutorStub struct {
	executeFn func(ctx context.Context, wf *workflow.DAGWorkflow, input any) (any, error)
}

func (s *workflowExecutorStub) ExecuteDAG(ctx context.Context, wf *workflow.DAGWorkflow, input any) (any, error) {
	if s.executeFn != nil {
		return s.executeFn(ctx, wf, input)
	}
	return nil, nil
}

func TestWorkflowService_BuildDAGWorkflow_FromDSL_Success(t *testing.T) {
	svc := newDefaultWorkflowService(&workflowExecutorStub{}, dsl.NewParser())

	wf, source, err := svc.BuildDAGWorkflow(workflowExecuteRequest{
		DSL: `
version: "1.0"
name: "test-workflow"
steps:
  s1:
    type: "passthrough"
workflow:
  entry: "n1"
  nodes:
    - id: "n1"
      type: "action"
      step: "s1"
`,
	})
	require.Nil(t, err)
	require.NotNil(t, wf)
	assert.Equal(t, "dsl", source)
	assert.Equal(t, "test-workflow", wf.Name())
}

func TestWorkflowService_BuildDAGWorkflow_InvalidDAGFileExtension(t *testing.T) {
	svc := newDefaultWorkflowService(&workflowExecutorStub{}, dsl.NewParser())

	wf, _, err := svc.BuildDAGWorkflow(workflowExecuteRequest{
		DAGFile: "workflow.txt",
	})
	require.Nil(t, wf)
	require.NotNil(t, err)
	assert.Equal(t, types.ErrInvalidRequest, err.Code)
	assert.Equal(t, http.StatusBadRequest, err.HTTPStatus)
	assert.Contains(t, err.Error(), "dag_file must be .json/.yml/.yaml")
}

func TestWorkflowService_BuildDAGWorkflow_SourceMismatch(t *testing.T) {
	svc := newDefaultWorkflowService(&workflowExecutorStub{}, dsl.NewParser())

	wf, _, err := svc.BuildDAGWorkflow(workflowExecuteRequest{
		Source: "dag_json",
		DSL: `
version: "1.0"
name: "test-workflow"
steps:
  s1:
    type: "passthrough"
workflow:
  entry: "n1"
  nodes:
    - id: "n1"
      type: "action"
      step: "s1"
`,
	})
	require.Nil(t, wf)
	require.NotNil(t, err)
	assert.Equal(t, types.ErrInvalidRequest, err.Code)
	assert.Contains(t, err.Error(), "source=dag_json requires dag_json")
}

func TestWorkflowService_BuildDAGWorkflow_AutoSourceConflict(t *testing.T) {
	svc := newDefaultWorkflowService(&workflowExecutorStub{}, dsl.NewParser())

	wf, _, err := svc.BuildDAGWorkflow(workflowExecuteRequest{
		DSL:     "version: \"1.0\"\nname: \"wf\"\nworkflow:\n  entry: \"n1\"\n  nodes: []\n",
		DAGJSON: `{"name":"wf","entry":"n1","nodes":[]}`,
	})
	require.Nil(t, wf)
	require.NotNil(t, err)
	assert.Equal(t, types.ErrInvalidRequest, err.Code)
	assert.Contains(t, err.Error(), "multiple workflow sources provided")
}

func TestWorkflowService_ValidateDSL_InvalidYAML(t *testing.T) {
	svc := newDefaultWorkflowService(&workflowExecutorStub{}, dsl.NewParser())

	result := svc.ValidateDSL("not: [valid")
	assert.False(t, result.Valid)
	require.NotEmpty(t, result.Errors)
	assert.Contains(t, result.Errors[0], "invalid YAML:")
}

func TestWorkflowService_Execute_ExecutorNotConfigured(t *testing.T) {
	svc := newDefaultWorkflowService(nil, dsl.NewParser())

	out, err := svc.Execute(context.Background(), &workflow.DAGWorkflow{}, "input", nil, nil)
	require.Nil(t, out)
	require.NotNil(t, err)
	assert.Equal(t, types.ErrInternalError, err.Code)
	assert.Equal(t, http.StatusNotImplemented, err.HTTPStatus)
}

func TestWorkflowService_Execute_InjectsNodeEmitter(t *testing.T) {
	var emitted bool
	executor := &workflowExecutorStub{
		executeFn: func(ctx context.Context, _ *workflow.DAGWorkflow, _ any) (any, error) {
			nodeEmitter, ok := workflowobs.NodeEventEmitterFromContext(ctx)
			if !ok {
				return nil, errors.New("node emitter missing")
			}
			nodeEmitter(workflowobs.NodeEvent{
				Type:      workflowobs.NodeStart,
				NodeID:    "n1",
				Timestamp: time.Now(),
			})
			return "ok", nil
		},
	}
	svc := newDefaultWorkflowService(executor, dsl.NewParser())

	out, err := svc.Execute(context.Background(), &workflow.DAGWorkflow{}, "input", nil, func(event workflowobs.NodeEvent) {
		emitted = event.NodeID == "n1"
	})
	require.Nil(t, err)
	assert.Equal(t, "ok", out)
	assert.True(t, emitted)
}
