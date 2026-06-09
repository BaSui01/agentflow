package core

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestDAGExecutor_TopologicalJoinWaitsForAllParents(t *testing.T) {
	var mu sync.Mutex
	seen := make(map[string]any)

	step := func(id string, delay time.Duration) Step {
		return &mockStep{id: id, exec: func(ctx context.Context, input any) (any, error) {
			if delay > 0 {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(delay):
				}
			}
			mu.Lock()
			seen[id] = input
			mu.Unlock()
			return id, nil
		}}
	}

	graph := NewDAGGraph()
	graph.AddNode(&DAGNode{ID: "start", Type: NodeTypeAction, Step: step("start", 0)})
	graph.AddNode(&DAGNode{ID: "fast", Type: NodeTypeAction, Step: step("fast", 0)})
	graph.AddNode(&DAGNode{ID: "slow", Type: NodeTypeAction, Step: step("slow", 40*time.Millisecond)})
	graph.AddNode(&DAGNode{ID: "join", Type: NodeTypeAction, Step: step("join", 0)})
	graph.AddEdge("start", "fast")
	graph.AddEdge("start", "slow")
	graph.AddEdge("fast", "join")
	graph.AddEdge("slow", "join")
	graph.SetEntry("start")

	result, err := NewDAGExecutor(nil, nil).Execute(context.Background(), graph, "input")
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if result != "join" {
		t.Fatalf("expected join result, got %#v", result)
	}

	mu.Lock()
	joinInput := seen["join"]
	mu.Unlock()

	inputs, ok := joinInput.(map[string]any)
	if !ok {
		t.Fatalf("join should receive all parent outputs as map, got %T %#v", joinInput, joinInput)
	}
	if inputs["fast"] != "fast" || inputs["slow"] != "slow" {
		t.Fatalf("join missing parent outputs: %#v", inputs)
	}
}

func TestDAGExecutor_TopologicalIndependentParentsRunConcurrently(t *testing.T) {
	step := func(id string, delay time.Duration) Step {
		return &mockStep{id: id, exec: func(ctx context.Context, input any) (any, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
			return fmt.Sprintf("%s:%v", id, input), nil
		}}
	}

	graph := NewDAGGraph()
	graph.AddNode(&DAGNode{ID: "start", Type: NodeTypeAction, Step: &PassthroughStep{}})
	graph.AddNode(&DAGNode{ID: "a", Type: NodeTypeAction, Step: step("a", 80*time.Millisecond)})
	graph.AddNode(&DAGNode{ID: "b", Type: NodeTypeAction, Step: step("b", 80*time.Millisecond)})
	graph.AddNode(&DAGNode{ID: "join", Type: NodeTypeAction, Step: &PassthroughStep{}})
	graph.AddEdge("start", "a")
	graph.AddEdge("start", "b")
	graph.AddEdge("a", "join")
	graph.AddEdge("b", "join")
	graph.SetEntry("start")

	started := time.Now()
	_, err := NewDAGExecutor(nil, nil).Execute(context.Background(), graph, "input")
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if elapsed := time.Since(started); elapsed >= 140*time.Millisecond {
		t.Fatalf("independent parents should run concurrently, elapsed=%v", elapsed)
	}
}
