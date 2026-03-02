package observability

import (
	"context"
	"errors"
	"testing"

	"github.com/BaSui01/agentflow/types"
)

func TestEmitNodeStart(t *testing.T) {
	var captured NodeEvent
	emitter := func(ev NodeEvent) { captured = ev }

	ctx := context.Background()
	ctx = types.WithTraceID(ctx, "t-1")
	ctx = types.WithRunID(ctx, "r-1")
	ctx = WithNodeEventEmitter(ctx, emitter)

	EmitNodeStart(ctx, "wf-1", "node-a", "action")

	if captured.Type != NodeStart {
		t.Fatalf("expected type node_start, got %s", captured.Type)
	}
	if captured.TraceID != "t-1" {
		t.Fatalf("expected trace_id t-1, got %s", captured.TraceID)
	}
	if captured.RunID != "r-1" {
		t.Fatalf("expected run_id r-1, got %s", captured.RunID)
	}
	if captured.WorkflowID != "wf-1" {
		t.Fatalf("expected workflow_id wf-1, got %s", captured.WorkflowID)
	}
	if captured.NodeID != "node-a" {
		t.Fatalf("expected node_id node-a, got %s", captured.NodeID)
	}
}

func TestEmitNodeComplete(t *testing.T) {
	var captured NodeEvent
	emitter := func(ev NodeEvent) { captured = ev }

	ctx := WithNodeEventEmitter(context.Background(), emitter)
	EmitNodeComplete(ctx, "wf-2", "node-b", "condition", 42)

	if captured.Type != NodeComplete {
		t.Fatalf("expected type node_complete, got %s", captured.Type)
	}
	if captured.LatencyMs != 42 {
		t.Fatalf("expected latency_ms 42, got %d", captured.LatencyMs)
	}
}

func TestEmitNodeError(t *testing.T) {
	var captured NodeEvent
	emitter := func(ev NodeEvent) { captured = ev }

	ctx := WithNodeEventEmitter(context.Background(), emitter)
	EmitNodeError(ctx, "wf-3", "node-c", "action", 100, errors.New("boom"))

	if captured.Type != NodeError {
		t.Fatalf("expected type node_error, got %s", captured.Type)
	}
	if captured.Error != "boom" {
		t.Fatalf("expected error boom, got %s", captured.Error)
	}
	if captured.LatencyMs != 100 {
		t.Fatalf("expected latency_ms 100, got %d", captured.LatencyMs)
	}
}

func TestEmitWithoutEmitter(t *testing.T) {
	// Should not panic when no emitter is set.
	ctx := context.Background()
	EmitNodeStart(ctx, "wf", "n", "action")
	EmitNodeComplete(ctx, "wf", "n", "action", 0)
	EmitNodeError(ctx, "wf", "n", "action", 0, errors.New("x"))
}

func TestNodeEventEmitterFromContext_Nil(t *testing.T) {
	_, ok := NodeEventEmitterFromContext(context.Background())
	if ok {
		t.Fatal("expected no emitter from empty context")
	}
}
