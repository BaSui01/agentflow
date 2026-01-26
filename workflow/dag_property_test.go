package workflow

import (
	"context"
	"errors"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// Feature: agent-framework-enhancements, Property 11: Conditional Routing Correctness
// Validates: Requirements 2.2
func TestProperty_ConditionalRoutingCorrectness(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	properties.Property("condition nodes route to correct branch based on condition result", prop.ForAll(
		func(conditionResult bool, inputValue int) bool {
			ctx := context.Background()

			// Track which branch was executed
			trueBranchExecuted := false
			falseBranchExecuted := false

			// Build workflow with conditional routing
			workflow, err := NewDAGBuilder("conditional-test").
				AddNode("start", NodeTypeAction).
				WithStep(&testStep{name: "start", result: inputValue}).
				Done().
				AddNode("condition", NodeTypeCondition).
				WithCondition(func(ctx context.Context, input interface{}) (bool, error) {
					return conditionResult, nil
				}).
				WithOnTrue("true_branch").
				WithOnFalse("false_branch").
				Done().
				AddNode("true_branch", NodeTypeAction).
				WithStep(&testStep{name: "true_branch", callback: func() { trueBranchExecuted = true }}).
				Done().
				AddNode("false_branch", NodeTypeAction).
				WithStep(&testStep{name: "false_branch", callback: func() { falseBranchExecuted = true }}).
				Done().
				AddEdge("start", "condition").
				SetEntry("start").
				Build()

			if err != nil {
				t.Logf("Build failed: %v", err)
				return false
			}

			// Execute workflow
			_, err = workflow.Execute(ctx, inputValue)
			if err != nil {
				t.Logf("Execute failed: %v", err)
				return false
			}

			// Verify correct branch was executed
			if conditionResult {
				if !trueBranchExecuted {
					t.Logf("True branch should have been executed")
					return false
				}
				if falseBranchExecuted {
					t.Logf("False branch should not have been executed")
					return false
				}
			} else {
				if trueBranchExecuted {
					t.Logf("True branch should not have been executed")
					return false
				}
				if !falseBranchExecuted {
					t.Logf("False branch should have been executed")
					return false
				}
			}

			return true
		},
		gen.Bool(),
		gen.Int(),
	))

	properties.TestingRun(t)
}

// Feature: agent-framework-enhancements, Property 12: Loop Termination
// Validates: Requirements 2.3
func TestProperty_LoopTermination(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	properties.Property("loops terminate within max iterations", prop.ForAll(
		func(maxIterations int) bool {
			if maxIterations <= 0 || maxIterations > 20 {
				return true // Skip invalid values
			}

			ctx := context.Background()
			iterationCount := 0

			// Build workflow with for loop
			workflow, err := NewDAGBuilder("loop-test").
				AddNode("loop", NodeTypeLoop).
				WithLoop(LoopConfig{
					Type:          LoopTypeFor,
					MaxIterations: maxIterations,
				}).
				Done().
				AddNode("body", NodeTypeAction).
				WithStep(&testStep{name: "body", callback: func() { iterationCount++ }}).
				Done().
				AddEdge("loop", "body").
				SetEntry("loop").
				Build()

			if err != nil {
				t.Logf("Build failed: %v", err)
				return false
			}

			// Execute workflow
			_, err = workflow.Execute(ctx, nil)
			if err != nil {
				t.Logf("Execute failed: %v", err)
				return false
			}

			// Verify loop terminated at max iterations
			if iterationCount != maxIterations {
				t.Logf("Expected %d iterations, got %d", maxIterations, iterationCount)
				return false
			}

			return true
		},
		gen.IntRange(1, 20),
	))

	properties.TestingRun(t)
}

// Feature: agent-framework-enhancements, Property 16: Dependency Ordering
// Validates: Requirements 2.7
func TestProperty_DependencyOrdering(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	properties.Property("nodes execute in dependency order", prop.ForAll(
		func(nodeCount int) bool {
			if nodeCount < 2 || nodeCount > 10 {
				return true
			}

			ctx := context.Background()
			executionOrder := make([]string, 0, nodeCount)

			// Build linear workflow
			builder := NewDAGBuilder("dependency-test")

			// Add nodes
			for i := 0; i < nodeCount; i++ {
				nodeID := string(rune('a' + i))
				builder.AddNode(nodeID, NodeTypeAction).
					WithStep(&testStep{
						name: nodeID,
						callback: func(id string) func() {
							return func() { executionOrder = append(executionOrder, id) }
						}(nodeID),
					}).
					Done()
			}

			// Add edges (linear chain)
			for i := 0; i < nodeCount-1; i++ {
				fromID := string(rune('a' + i))
				toID := string(rune('a' + i + 1))
				builder.AddEdge(fromID, toID)
			}

			builder.SetEntry("a")

			workflow, err := builder.Build()
			if err != nil {
				t.Logf("Build failed: %v", err)
				return false
			}

			// Execute workflow
			_, err = workflow.Execute(ctx, nil)
			if err != nil {
				t.Logf("Execute failed: %v", err)
				return false
			}

			// Verify execution order matches dependency order
			if len(executionOrder) != nodeCount {
				t.Logf("Expected %d nodes executed, got %d", nodeCount, len(executionOrder))
				return false
			}

			for i := 0; i < nodeCount; i++ {
				expectedID := string(rune('a' + i))
				if executionOrder[i] != expectedID {
					t.Logf("Expected node %s at position %d, got %s", expectedID, i, executionOrder[i])
					return false
				}
			}

			return true
		},
		gen.IntRange(2, 10),
	))

	properties.TestingRun(t)
}

// Feature: agent-framework-enhancements, Property 18: Cycle Detection
// Validates: Requirements 2.10
func TestProperty_CycleDetection(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	properties.Property("cycles are detected during build", prop.ForAll(
		func(nodeCount int) bool {
			if nodeCount < 2 || nodeCount > 10 {
				return true
			}

			// Build workflow with cycle (last node points back to first)
			builder := NewDAGBuilder("cycle-test")

			// Add nodes
			for i := 0; i < nodeCount; i++ {
				nodeID := string(rune('a' + i))
				builder.AddNode(nodeID, NodeTypeAction).
					WithStep(&testStep{name: nodeID}).
					Done()
			}

			// Add edges (linear chain)
			for i := 0; i < nodeCount-1; i++ {
				fromID := string(rune('a' + i))
				toID := string(rune('a' + i + 1))
				builder.AddEdge(fromID, toID)
			}

			// Add cycle edge (last -> first)
			lastID := string(rune('a' + nodeCount - 1))
			builder.AddEdge(lastID, "a")
			builder.SetEntry("a")

			// Build should fail due to cycle
			_, err := builder.Build()
			if err == nil {
				t.Logf("Expected cycle detection error, got nil")
				return false
			}

			return true
		},
		gen.IntRange(2, 10),
	))

	properties.TestingRun(t)
}

// testStep is a simple step implementation for property testing
type testStep struct {
	name     string
	result   interface{}
	err      error
	callback func()
}

func (s *testStep) Execute(ctx context.Context, input interface{}) (interface{}, error) {
	if s.callback != nil {
		s.callback()
	}
	if s.err != nil {
		return nil, s.err
	}
	if s.result != nil {
		return s.result, nil
	}
	return input, nil
}

func (s *testStep) Name() string {
	return s.name
}

// Feature: agent-framework-enhancements, Property 17: Error Handling Strategy Application
// Validates: Requirements 2.8
func TestProperty_ErrorHandlingStrategy(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 50

	properties := gopter.NewProperties(parameters)

	properties.Property("errors propagate correctly from failed nodes", prop.ForAll(
		func(failAtNode int) bool {
			// Ensure failAtNode is in valid range
			failAtNode = failAtNode % 3
			if failAtNode < 0 {
				failAtNode = -failAtNode
			}

			ctx := context.Background()
			expectedError := errors.New("test error")

			// Build workflow with potential failure
			builder := NewDAGBuilder("error-test")

			for i := 0; i < 3; i++ {
				nodeID := string(rune('a' + i))
				var step *testStep
				if i == failAtNode {
					step = &testStep{name: nodeID, err: expectedError}
				} else {
					step = &testStep{name: nodeID}
				}
				builder.AddNode(nodeID, NodeTypeAction).
					WithStep(step).
					Done()
			}

			builder.AddEdge("a", "b").AddEdge("b", "c").SetEntry("a")

			workflow, err := builder.Build()
			if err != nil {
				t.Logf("Build failed: %v", err)
				return false
			}

			// Execute workflow
			_, err = workflow.Execute(ctx, nil)

			// Should fail with error
			if err == nil {
				t.Logf("Expected error, got nil")
				return false
			}

			return true
		},
		gen.Int(),
	))

	properties.TestingRun(t)
}

// Feature: agent-framework-enhancements, Property 19: Execution Path Recording
// Validates: Requirements 6.1
func TestProperty_ExecutionPathRecording(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 50

	properties := gopter.NewProperties(parameters)

	properties.Property("execution history records all executed nodes", prop.ForAll(
		func(nodeCount int) bool {
			if nodeCount < 1 || nodeCount > 5 {
				return true
			}

			ctx := context.Background()

			// Build linear workflow
			builder := NewDAGBuilder("history-test")

			for i := 0; i < nodeCount; i++ {
				nodeID := string(rune('a' + i))
				builder.AddNode(nodeID, NodeTypeAction).
					WithStep(&testStep{name: nodeID}).
					Done()
			}

			for i := 0; i < nodeCount-1; i++ {
				fromID := string(rune('a' + i))
				toID := string(rune('a' + i + 1))
				builder.AddEdge(fromID, toID)
			}

			builder.SetEntry("a")

			workflow, err := builder.Build()
			if err != nil {
				t.Logf("Build failed: %v", err)
				return false
			}

			// Execute workflow
			executor := NewDAGExecutor(nil, nil)
			_, err = executor.Execute(ctx, workflow.Graph(), nil)
			if err != nil {
				t.Logf("Execute failed: %v", err)
				return false
			}

			// Verify history records all nodes
			history := executor.GetHistory()
			if history == nil {
				t.Logf("History is nil")
				return false
			}

			nodes := history.GetNodes()
			if len(nodes) != nodeCount {
				t.Logf("Expected %d nodes in history, got %d", nodeCount, len(nodes))
				return false
			}

			// Verify all nodes are recorded
			for i := 0; i < nodeCount; i++ {
				expectedID := string(rune('a' + i))
				found := false
				for _, node := range nodes {
					if node.NodeID == expectedID {
						found = true
						break
					}
				}
				if !found {
					t.Logf("Node %s not found in history", expectedID)
					return false
				}
			}

			return true
		},
		gen.IntRange(1, 5),
	))

	properties.TestingRun(t)
}

// Feature: agent-framework-enhancements, Property 22: Execution History Query Accuracy
// Validates: Requirements 6.4
func TestProperty_ExecutionHistoryQueryAccuracy(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 50

	properties := gopter.NewProperties(parameters)

	properties.Property("history store queries return correct results", prop.ForAll(
		func(executionCount int) bool {
			if executionCount < 1 || executionCount > 5 {
				return true
			}

			ctx := context.Background()
			store := NewExecutionHistoryStore()

			// Build simple workflow
			workflow, err := NewDAGBuilder("query-test").
				AddNode("a", NodeTypeAction).
				WithStep(&testStep{name: "a"}).
				Done().
				SetEntry("a").
				Build()

			if err != nil {
				t.Logf("Build failed: %v", err)
				return false
			}

			// Execute multiple times with shared store
			executor := NewDAGExecutor(nil, nil)
			executor.SetHistoryStore(store)

			for i := 0; i < executionCount; i++ {
				_, _ = executor.Execute(ctx, workflow.Graph(), nil)
			}

			// Query by status
			completed := store.ListByStatus(ExecutionStatusCompleted)
			if len(completed) != executionCount {
				t.Logf("Expected %d completed executions, got %d", executionCount, len(completed))
				return false
			}

			return true
		},
		gen.IntRange(1, 5),
	))

	properties.TestingRun(t)
}
