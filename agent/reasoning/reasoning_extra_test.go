package reasoning

import (
	"github.com/BaSui01/agentflow/types"
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// --- DynamicPlanner: executeNode ---

func TestDynamicPlanner_ExecuteNode_ThinkAction(t *testing.T) {
	provider := &testProvider{
		completionFn: func(_ context.Context, _ *llm.ChatRequest) (*llm.ChatResponse, error) {
			return &llm.ChatResponse{
				Choices: []llm.ChatChoice{{Message: types.Message{Content: "thought result"}}},
				Usage:   llm.ChatUsage{TotalTokens: 10},
			}, nil
		},
	}
	dp := NewDynamicPlanner(provider, nil, nil, DefaultDynamicPlannerConfig(), zap.NewNop())

	node := &PlanNode{ID: "n1", Action: "think", Description: "analyze problem"}
	result, tokens, err := dp.executeNode(context.Background(), node)
	require.NoError(t, err)
	assert.Equal(t, "thought result", result)
	assert.Equal(t, 10, tokens)
}

func TestDynamicPlanner_ExecuteNode_ReasonAction(t *testing.T) {
	provider := &testProvider{
		completionFn: func(_ context.Context, _ *llm.ChatRequest) (*llm.ChatResponse, error) {
			return &llm.ChatResponse{
				Choices: []llm.ChatChoice{{Message: types.Message{Content: "reasoning"}}},
				Usage:   llm.ChatUsage{TotalTokens: 5},
			}, nil
		},
	}
	dp := NewDynamicPlanner(provider, nil, nil, DefaultDynamicPlannerConfig(), zap.NewNop())

	node := &PlanNode{ID: "n1", Action: "reason", Description: "reason about it"}
	result, _, err := dp.executeNode(context.Background(), node)
	require.NoError(t, err)
	assert.Equal(t, "reasoning", result)
}

func TestDynamicPlanner_ExecuteNode_ToolAction(t *testing.T) {
	executor := &testToolExecutor{
		executeFn: func(_ context.Context, calls []types.ToolCall) []tools.ToolResult {
			return []tools.ToolResult{{ToolCallID: calls[0].ID, Result: json.RawMessage(`"tool output"`)}}
		},
	}
	dp := NewDynamicPlanner(nil, executor, nil, DefaultDynamicPlannerConfig(), zap.NewNop())

	node := &PlanNode{ID: "n1", Action: "search", Description: "search for info"}
	result, _, err := dp.executeNode(context.Background(), node)
	require.NoError(t, err)
	assert.Equal(t, `"tool output"`, result)
}

func TestDynamicPlanner_ExecuteNode_ToolError(t *testing.T) {
	executor := &testToolExecutor{
		executeFn: func(_ context.Context, calls []types.ToolCall) []tools.ToolResult {
			return []tools.ToolResult{{ToolCallID: calls[0].ID, Error: "tool failed"}}
		},
	}
	dp := NewDynamicPlanner(nil, executor, nil, DefaultDynamicPlannerConfig(), zap.NewNop())

	node := &PlanNode{ID: "n1", Action: "search", Description: "search"}
	_, _, err := dp.executeNode(context.Background(), node)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "tool error")
}

func TestDynamicPlanner_ExecuteNode_NoToolResult(t *testing.T) {
	executor := &testToolExecutor{
		executeFn: func(_ context.Context, _ []types.ToolCall) []tools.ToolResult {
			return nil
		},
	}
	dp := NewDynamicPlanner(nil, executor, nil, DefaultDynamicPlannerConfig(), zap.NewNop())

	node := &PlanNode{ID: "n1", Action: "search", Description: "search"}
	_, _, err := dp.executeNode(context.Background(), node)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no result")
}

// --- DynamicPlanner: executeLLMNode ---

func TestDynamicPlanner_ExecuteLLMNode_Error(t *testing.T) {
	provider := &testProvider{
		completionFn: func(_ context.Context, _ *llm.ChatRequest) (*llm.ChatResponse, error) {
			return nil, fmt.Errorf("LLM unavailable")
		},
	}
	dp := NewDynamicPlanner(provider, nil, nil, DefaultDynamicPlannerConfig(), zap.NewNop())

	node := &PlanNode{ID: "n1", Action: "think", Description: "think"}
	_, _, err := dp.executeLLMNode(context.Background(), node)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "LLM unavailable")
}

func TestDynamicPlanner_ExecuteLLMNode_NoChoices(t *testing.T) {
	provider := &testProvider{
		completionFn: func(_ context.Context, _ *llm.ChatRequest) (*llm.ChatResponse, error) {
			return &llm.ChatResponse{
				Choices: []llm.ChatChoice{},
				Usage:   llm.ChatUsage{TotalTokens: 5},
			}, nil
		},
	}
	dp := NewDynamicPlanner(provider, nil, nil, DefaultDynamicPlannerConfig(), zap.NewNop())

	node := &PlanNode{ID: "n1", Action: "think", Description: "think"}
	_, _, err := dp.executeLLMNode(context.Background(), node)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no choices")
}

// --- DynamicPlanner: tryAlternativeOrBacktrack ---

func TestDynamicPlanner_TryAlternative_HasAlternative(t *testing.T) {
	dp := NewDynamicPlanner(nil, nil, nil, DefaultDynamicPlannerConfig(), zap.NewNop())

	alt := &PlanNode{ID: "alt1", Status: NodeStatusPending}
	failed := &PlanNode{
		ID:           "n1",
		Status:       NodeStatusFailed,
		Alternatives: []*PlanNode{alt},
	}
	dp.rootNode = &PlanNode{ID: "root", Children: []*PlanNode{failed}}

	result := dp.tryAlternativeOrBacktrack(context.Background(), failed)
	assert.True(t, result)
	assert.Equal(t, NodeStatusSkipped, failed.Status)
}

func TestDynamicPlanner_TryAlternative_NoAlternative_Backtrack(t *testing.T) {
	dp := NewDynamicPlanner(nil, nil, nil, DefaultDynamicPlannerConfig(), zap.NewNop())

	parentAlt := &PlanNode{ID: "palt1", Status: NodeStatusPending}
	failed := &PlanNode{ID: "n1", ParentID: "root", Status: NodeStatusFailed}
	dp.rootNode = &PlanNode{
		ID:           "root",
		Children:     []*PlanNode{failed},
		Alternatives: []*PlanNode{parentAlt},
	}

	result := dp.tryAlternativeOrBacktrack(context.Background(), failed)
	assert.True(t, result)
	assert.Equal(t, 1, dp.backtracks)
}

func TestDynamicPlanner_TryAlternative_MaxBacktracksReached(t *testing.T) {
	cfg := DefaultDynamicPlannerConfig()
	cfg.MaxBacktracks = 0
	dp := NewDynamicPlanner(nil, nil, nil, cfg, zap.NewNop())

	failed := &PlanNode{ID: "n1", Status: NodeStatusFailed}
	dp.rootNode = &PlanNode{ID: "root", Children: []*PlanNode{failed}}

	result := dp.tryAlternativeOrBacktrack(context.Background(), failed)
	assert.False(t, result)
}

// --- DynamicPlanner: replaceNodeWithAlternative ---

func TestDynamicPlanner_ReplaceNodeWithAlternative(t *testing.T) {
	dp := NewDynamicPlanner(nil, nil, nil, DefaultDynamicPlannerConfig(), zap.NewNop())

	original := &PlanNode{ID: "orig", ParentID: "root", Status: NodeStatusFailed}
	alt := &PlanNode{ID: "alt", Status: NodeStatusPending}

	dp.replaceNodeWithAlternative(original, alt)
	assert.Equal(t, NodeStatusSkipped, original.Status)
	assert.Equal(t, "root", alt.ParentID)
}

// --- DynamicPlanner: shouldContinue ---

func TestDynamicPlanner_ShouldContinue_CompletionIndicator(t *testing.T) {
	dp := NewDynamicPlanner(nil, nil, nil, DefaultDynamicPlannerConfig(), zap.NewNop())
	dp.rootNode = &PlanNode{ID: "root"}

	// Long result with completion indicator
	longResult := "This is a long result that exceeds 100 characters. " +
		"It contains many words and details about the analysis. " +
		"The Final Answer is that we should proceed with option A."
	node := &PlanNode{ID: "n1", Result: longResult}

	assert.False(t, dp.shouldContinue(context.Background(), "task", node))
}

func TestDynamicPlanner_ShouldContinue_MaxDepthReached(t *testing.T) {
	cfg := DefaultDynamicPlannerConfig()
	cfg.MaxPlanDepth = 2
	dp := NewDynamicPlanner(nil, nil, nil, cfg, zap.NewNop())

	root := &PlanNode{ID: "root"}
	child := &PlanNode{ID: "c1", ParentID: "root"}
	grandchild := &PlanNode{ID: "gc1", ParentID: "c1"}
	root.Children = []*PlanNode{child}
	child.Children = []*PlanNode{grandchild}
	dp.rootNode = root

	assert.False(t, dp.shouldContinue(context.Background(), "task", grandchild))
}

func TestDynamicPlanner_ShouldContinue_CanContinue(t *testing.T) {
	cfg := DefaultDynamicPlannerConfig()
	cfg.MaxPlanDepth = 10
	dp := NewDynamicPlanner(nil, nil, nil, cfg, zap.NewNop())

	root := &PlanNode{ID: "root"}
	child := &PlanNode{ID: "c1", ParentID: "root", Result: "short"}
	root.Children = []*PlanNode{child}
	dp.rootNode = root

	assert.True(t, dp.shouldContinue(context.Background(), "task", child))
}

// --- DynamicPlanner: getNodeDepth ---

func TestDynamicPlanner_GetNodeDepth(t *testing.T) {
	dp := NewDynamicPlanner(nil, nil, nil, DefaultDynamicPlannerConfig(), zap.NewNop())

	root := &PlanNode{ID: "root"}
	child := &PlanNode{ID: "c1", ParentID: "root"}
	grandchild := &PlanNode{ID: "gc1", ParentID: "c1"}
	root.Children = []*PlanNode{child}
	child.Children = []*PlanNode{grandchild}
	dp.rootNode = root

	assert.Equal(t, 0, dp.getNodeDepth(root))
	assert.Equal(t, 1, dp.getNodeDepth(child))
	assert.Equal(t, 2, dp.getNodeDepth(grandchild))
}

func TestDynamicPlanner_GetNodeDepth_OrphanNode(t *testing.T) {
	dp := NewDynamicPlanner(nil, nil, nil, DefaultDynamicPlannerConfig(), zap.NewNop())
	dp.rootNode = &PlanNode{ID: "root"}

	orphan := &PlanNode{ID: "orphan", ParentID: "nonexistent"}
	depth := dp.getNodeDepth(orphan)
	assert.Equal(t, 1, depth) // finds nonexistent parent, breaks
}


