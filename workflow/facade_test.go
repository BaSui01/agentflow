package workflow

import (
	"context"
	"testing"
)

func TestFacadeExecute(t *testing.T) {
	graph := NewDAGGraph()
	graph.AddNode(&DAGNode{ID: "n1", Type: NodeTypeAction, Step: NewFuncStep("echo", func(ctx context.Context, input any) (any, error) {
		return "ok", nil
	})})
	graph.SetEntry("n1")

	wf := NewDAGWorkflow("wf", "desc", graph)
	facade := NewFacade(NewDAGExecutor(nil, nil))

	out, err := facade.ExecuteDAG(context.Background(), wf, "in")
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if out != "ok" {
		t.Fatalf("unexpected output: %v", out)
	}
}

func TestFacadeExecuteErrors(t *testing.T) {
	facade := NewFacade(nil)
	if _, err := facade.ExecuteDAG(context.Background(), &DAGWorkflow{}, nil); err == nil {
		t.Fatal("expected error for nil executor")
	}

	facade = NewFacade(NewDAGExecutor(nil, nil))
	if _, err := facade.ExecuteDAG(context.Background(), nil, nil); err == nil {
		t.Fatal("expected error for nil dag workflow")
	}
}
