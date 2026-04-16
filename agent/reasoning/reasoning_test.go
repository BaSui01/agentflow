package reasoning

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/types"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/capabilities/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// --- mock provider (satisfies llm.Provider) ---

type testProvider struct {
	completionFn func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error)
}

func (p *testProvider) Completion(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	if p.completionFn != nil {
		return p.completionFn(ctx, req)
	}
	return &llm.ChatResponse{
		Choices: []llm.ChatChoice{{Message: types.Message{Content: "mock"}}},
	}, nil
}
func (p *testProvider) Stream(context.Context, *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	return nil, nil
}
func (p *testProvider) HealthCheck(context.Context) (*llm.HealthStatus, error) {
	return &llm.HealthStatus{Healthy: true}, nil
}
func (p *testProvider) Name() string                        { return "test" }
func (p *testProvider) SupportsNativeFunctionCalling() bool { return false }
func (p *testProvider) ListModels(context.Context) ([]llm.Model, error) {
	return nil, nil
}
func (p *testProvider) Endpoints() llm.ProviderEndpoints {
	return llm.ProviderEndpoints{}
}

// --- mock tool executor (satisfies tools.ToolExecutor) ---

type testToolExecutor struct {
	executeFn func(ctx context.Context, calls []types.ToolCall) []tools.ToolResult
}

func (e *testToolExecutor) Execute(ctx context.Context, calls []types.ToolCall) []tools.ToolResult {
	if e.executeFn != nil {
		return e.executeFn(ctx, calls)
	}
	var results []tools.ToolResult
	for _, c := range calls {
		results = append(results, tools.ToolResult{
			ToolCallID: c.ID, Result: json.RawMessage(`"ok"`),
		})
	}
	return results
}

func (e *testToolExecutor) ExecuteOne(ctx context.Context, call types.ToolCall) tools.ToolResult {
	results := e.Execute(ctx, []types.ToolCall{call})
	if len(results) > 0 {
		return results[0]
	}
	return tools.ToolResult{}
}

// --- helper function tests ---

func TestExtractJSON(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, input, expected string
	}{
		{"plain array", `[{"id":"1"}]`, `[{"id":"1"}]`},
		{"with prefix", `Here: [{"id":"1"}]`, `[{"id":"1"}]`},
		{"no brackets", `no json here`, `no json here`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, extractJSON(tt.input))
		})
	}
}

func TestExtractJSONObject(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, input, expected string
	}{
		{"plain object", `{"key":"val"}`, `{"key":"val"}`},
		{"with prefix", `Result: {"key":"val"}`, `{"key":"val"}`},
		{"no braces", `no json`, `no json`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, extractJSONObject(tt.input))
		})
	}
}

func TestExtractJSONFromContent(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, input, expected string
	}{
		{"simple", `{"score": 0.8}`, `{"score": 0.8}`},
		{"nested", `text {"o": {"i": 1}} more`, `{"o": {"i": 1}}`},
		{"no json", `plain text`, `plain text`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, extractJSONFromContent(tt.input))
		})
	}
}

func TestTruncate(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "short", truncate("short", 10))
	assert.Equal(t, "hel...", truncate("hello world", 3))
	assert.Equal(t, "", truncate("", 5))
}

func TestContainsCompletionIndicator(t *testing.T) {
	t.Parallel()
	assert.True(t, containsCompletionIndicator("The Final Answer is here"))
	assert.True(t, containsCompletionIndicator("In summary, done"))
	assert.False(t, containsCompletionIndicator("still working"))
}

func TestToLower(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "hello world", toLower("Hello World"))
	assert.Equal(t, "", toLower(""))
}

func TestJoinStrings(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "", joinStrings(nil, ","))
	assert.Equal(t, "a", joinStrings([]string{"a"}, ","))
	assert.Equal(t, "a,b", joinStrings([]string{"a", "b"}, ","))
}

func TestContainsString(t *testing.T) {
	t.Parallel()
	assert.True(t, containsString("hello world", "world"))
	assert.False(t, containsString("hello", "world"))
	assert.False(t, containsString("ab", "abc"))
}

// --- ReWOO tests ---

func TestReWOO_Name(t *testing.T) {
	t.Parallel()
	r := NewReWOO(nil, nil, nil, DefaultReWOOConfig(), nil)
	assert.Equal(t, "rewoo", r.Name())
}

func TestReWOO_ExtractDependencies(t *testing.T) {
	t.Parallel()
	r := NewReWOO(nil, nil, nil, DefaultReWOOConfig(), nil)

	deps := r.extractDependencies("use #E1 and #E2, also #E1 again")
	assert.Equal(t, []string{"#E1", "#E2"}, deps)

	deps = r.extractDependencies("no dependencies")
	assert.Empty(t, deps)
}

func TestReWOO_ParsePlanManually(t *testing.T) {
	t.Parallel()
	r := NewReWOO(nil, nil, nil, DefaultReWOOConfig(), nil)

	content := "#E1 = search[golang concurrency]\n#E2 = analyze[#E1 results]"
	plan := r.parsePlanManually(content)
	require.Len(t, plan, 2)
	assert.Equal(t, "#E1", plan[0].ID)
	assert.Equal(t, "search", plan[0].Tool)
	assert.Equal(t, "golang concurrency", plan[0].Arguments)
}

func TestReWOO_ExecuteSteps_CircularDependency(t *testing.T) {
	t.Parallel()
	r := NewReWOO(nil, &testToolExecutor{}, nil, DefaultReWOOConfig(), zap.NewNop())

	plan := []PlanStep{
		{ID: "#E1", Tool: "s", Arguments: "#E2", Dependencies: []string{"#E2"}},
		{ID: "#E2", Tool: "s", Arguments: "#E1", Dependencies: []string{"#E1"}},
	}
	observations, _ := r.executeSteps(context.Background(), plan)
	assert.Empty(t, observations)
}

func TestReWOO_ExecuteTool(t *testing.T) {
	t.Parallel()
	executor := &testToolExecutor{
		executeFn: func(_ context.Context, calls []types.ToolCall) []tools.ToolResult {
			return []tools.ToolResult{{ToolCallID: calls[0].ID, Result: json.RawMessage(`"found"`)}}
		},
	}
	r := NewReWOO(nil, executor, nil, DefaultReWOOConfig(), zap.NewNop())
	assert.Equal(t, `"found"`, r.executeTool(context.Background(), "search", "q"))
}

func TestReWOO_ExecuteTool_Error(t *testing.T) {
	t.Parallel()
	executor := &testToolExecutor{
		executeFn: func(_ context.Context, calls []types.ToolCall) []tools.ToolResult {
			return []tools.ToolResult{{ToolCallID: calls[0].ID, Error: "fail"}}
		},
	}
	r := NewReWOO(nil, executor, nil, DefaultReWOOConfig(), zap.NewNop())
	assert.Contains(t, r.executeTool(context.Background(), "s", "q"), "Error")
}

func TestReWOO_ExecuteTool_NoResult(t *testing.T) {
	t.Parallel()
	executor := &testToolExecutor{
		executeFn: func(_ context.Context, _ []types.ToolCall) []tools.ToolResult { return nil },
	}
	r := NewReWOO(nil, executor, nil, DefaultReWOOConfig(), zap.NewNop())
	assert.Equal(t, "No result", r.executeTool(context.Background(), "s", "q"))
}

// --- Config defaults ---

func TestDefaultReWOOConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultReWOOConfig()
	assert.Equal(t, 10, cfg.MaxPlanSteps)
	assert.Equal(t, 120*time.Second, cfg.Timeout)
}

func TestDefaultTreeOfThoughtConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultTreeOfThoughtConfig()
	assert.Equal(t, 3, cfg.BranchingFactor)
	assert.Equal(t, 5, cfg.MaxDepth)
	assert.Equal(t, 0.3, cfg.PruneThreshold)
}

func TestDefaultReflexionConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultReflexionConfig()
	assert.Equal(t, 5, cfg.MaxTrials)
	assert.Equal(t, 0.8, cfg.SuccessThreshold)
}

func TestDefaultDynamicPlannerConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultDynamicPlannerConfig()
	assert.Equal(t, 5, cfg.MaxBacktracks)
	assert.Equal(t, 20, cfg.MaxPlanDepth)
}

func TestDefaultIterativeDeepeningConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultIterativeDeepeningConfig()
	assert.Equal(t, 3, cfg.Breadth)
	assert.Equal(t, 3, cfg.MaxDepth)
}

func TestDefaultPlanExecuteConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultPlanExecuteConfig()
	assert.Equal(t, 15, cfg.MaxPlanSteps)
	assert.True(t, cfg.AdaptivePlanning)
}

// --- TreeOfThought tests ---

func TestTreeOfThought_Name(t *testing.T) {
	t.Parallel()
	tot := NewTreeOfThought(nil, nil, DefaultTreeOfThoughtConfig(), nil)
	assert.Equal(t, "tree_of_thought", tot.Name())
}

func TestTreeOfThought_SelectTopBranches(t *testing.T) {
	t.Parallel()
	cfg := DefaultTreeOfThoughtConfig()
	cfg.PruneThreshold = 0.3
	tot := NewTreeOfThought(nil, nil, cfg, nil)

	thoughts := []ReasoningStep{
		{StepID: "a", Score: 0.9},
		{StepID: "b", Score: 0.1},
		{StepID: "c", Score: 0.7},
		{StepID: "d", Score: 0.5},
	}
	selected := tot.selectTopBranches(thoughts, 2)
	require.Len(t, selected, 2)
	assert.Equal(t, "a", selected[0].StepID)
	assert.Equal(t, "c", selected[1].StepID)
}

func TestTreeOfThought_SelectTopBranches_AllBelowThreshold(t *testing.T) {
	t.Parallel()
	cfg := DefaultTreeOfThoughtConfig()
	cfg.PruneThreshold = 0.9
	tot := NewTreeOfThought(nil, nil, cfg, nil)

	thoughts := []ReasoningStep{{StepID: "a", Score: 0.5}}
	assert.Empty(t, tot.selectTopBranches(thoughts, 2))
}

// --- ReflexionExecutor tests ---

func TestReflexionExecutor_Name(t *testing.T) {
	t.Parallel()
	r := NewReflexionExecutor(nil, nil, nil, DefaultReflexionConfig(), nil)
	assert.Equal(t, "reflexion", r.Name())
}

// --- DynamicPlanner tests ---

func TestDynamicPlanner_Name(t *testing.T) {
	t.Parallel()
	dp := NewDynamicPlanner(nil, nil, nil, DefaultDynamicPlannerConfig(), nil)
	assert.Equal(t, "dynamic_planner", dp.Name())
}

func TestDynamicPlanner_NextNodeID(t *testing.T) {
	t.Parallel()
	dp := NewDynamicPlanner(nil, nil, nil, DefaultDynamicPlannerConfig(), nil)
	assert.Equal(t, "node_1", dp.nextNodeID())
	assert.Equal(t, "node_2", dp.nextNodeID())
}

func TestDynamicPlanner_FindNextNodeRecursive(t *testing.T) {
	t.Parallel()
	dp := NewDynamicPlanner(nil, nil, nil, DefaultDynamicPlannerConfig(), nil)

	root := &PlanNode{
		ID: "root", Status: NodeStatusCompleted,
		Children: []*PlanNode{
			{ID: "c1", Status: NodeStatusCompleted},
			{ID: "c2", Status: NodeStatusPending},
		},
	}
	found := dp.findNextNodeRecursive(root)
	require.NotNil(t, found)
	assert.Equal(t, "c2", found.ID)

	assert.Nil(t, dp.findNextNodeRecursive(nil))
}

func TestDynamicPlanner_FindParent(t *testing.T) {
	t.Parallel()
	dp := NewDynamicPlanner(nil, nil, nil, DefaultDynamicPlannerConfig(), nil)

	root := &PlanNode{
		ID: "root",
		Children: []*PlanNode{
			{ID: "c1", Children: []*PlanNode{{ID: "gc1"}}},
			{ID: "c2"},
		},
	}
	assert.Equal(t, "c1", dp.findParent(root, "gc1").ID)
	assert.Equal(t, "root", dp.findParent(root, "c2").ID)
	assert.Nil(t, dp.findParent(root, "missing"))
	assert.Nil(t, dp.findParent(nil, "any"))
}

func TestDynamicPlanner_FindNodeByID(t *testing.T) {
	t.Parallel()
	dp := NewDynamicPlanner(nil, nil, nil, DefaultDynamicPlannerConfig(), nil)

	root := &PlanNode{
		ID:       "root",
		Children: []*PlanNode{{ID: "c1", Children: []*PlanNode{{ID: "gc1"}}}},
	}
	assert.Equal(t, "gc1", dp.findNodeByID(root, "gc1").ID)
	assert.Equal(t, "root", dp.findNodeByID(root, "root").ID)
	assert.Nil(t, dp.findNodeByID(root, "missing"))
	assert.Nil(t, dp.findNodeByID(nil, "any"))
}

func TestDynamicPlanner_CollectSteps(t *testing.T) {
	t.Parallel()
	dp := NewDynamicPlanner(nil, nil, nil, DefaultDynamicPlannerConfig(), nil)

	root := &PlanNode{
		ID: "root", Action: "search", Description: "find", Confidence: 0.9,
		Status: NodeStatusCompleted,
		Children: []*PlanNode{
			{ID: "c1", Action: "think", Description: "analyze", Status: NodeStatusCompleted},
			{ID: "c2", Action: "act", Description: "do", Status: NodeStatusBacktrack},
		},
	}
	steps := dp.collectSteps(root)
	require.Len(t, steps, 3)
	assert.Equal(t, "action", steps[0].Type)
	assert.Equal(t, "thought", steps[1].Type)
	assert.Equal(t, "backtrack", steps[2].Type)
}

func TestDynamicPlanner_CollectResults(t *testing.T) {
	t.Parallel()
	dp := NewDynamicPlanner(nil, nil, nil, DefaultDynamicPlannerConfig(), nil)

	root := &PlanNode{
		ID: "root", Description: "root", Status: NodeStatusCompleted, Result: "r",
		Children: []*PlanNode{
			{ID: "c1", Description: "child", Status: NodeStatusCompleted, Result: "cr"},
			{ID: "c2", Description: "fail", Status: NodeStatusFailed},
		},
	}
	var results []string
	dp.collectResults(root, &results)
	assert.Len(t, results, 2)
}

// --- IterativeDeepening tests ---

func TestIterativeDeepening_Name(t *testing.T) {
	t.Parallel()
	id := NewIterativeDeepening(nil, nil, DefaultIterativeDeepeningConfig(), nil)
	assert.Equal(t, "iterative_deepening", id.Name())
}

func TestIterativeDeepening_CalculateConfidence(t *testing.T) {
	t.Parallel()
	id := NewIterativeDeepening(nil, nil, DefaultIterativeDeepeningConfig(), nil)

	assert.Equal(t, 0.0, id.calculateConfidence(nil))

	findings := []researchFinding{{Relevance: 0.9}, {Relevance: 0.8}}
	conf := id.calculateConfidence(findings)
	assert.Greater(t, conf, 0.0)
	assert.LessOrEqual(t, conf, 1.0)
}

func TestIterativeDeepening_CalculateConfidence_MoreFindingsHigherConfidence(t *testing.T) {
	t.Parallel()
	id := NewIterativeDeepening(nil, nil, DefaultIterativeDeepeningConfig(), nil)

	few := []researchFinding{{Relevance: 0.8}}
	many := make([]researchFinding, 20)
	for i := range many {
		many[i] = researchFinding{Relevance: 0.8}
	}
	assert.Greater(t, id.calculateConfidence(many), id.calculateConfidence(few))
}

// --- PlanAndExecute tests ---

func TestPlanAndExecute_Name(t *testing.T) {
	t.Parallel()
	pe := NewPlanAndExecute(nil, nil, nil, DefaultPlanExecuteConfig(), nil)
	assert.Equal(t, "plan_and_execute", pe.Name())
}

// --- ReWOO full Execute with mock ---

func TestReWOO_Execute_Success(t *testing.T) {
	t.Parallel()

	callCount := 0
	provider := &testProvider{
		completionFn: func(_ context.Context, _ *llm.ChatRequest) (*llm.ChatResponse, error) {
			callCount++
			switch {
			case callCount == 1:
				// Plan generation
				return &llm.ChatResponse{
					Choices: []llm.ChatChoice{{Message: types.Message{
						Content: `[{"id":"#E1","tool":"search","arguments":"golang","reasoning":"find info"}]`,
					}}},
					Usage: llm.ChatUsage{TotalTokens: 50},
				}, nil
			default:
				// Synthesis
				return &llm.ChatResponse{
					Choices: []llm.ChatChoice{{Message: types.Message{Content: "final answer"}}},
					Usage:   llm.ChatUsage{TotalTokens: 30},
				}, nil
			}
		},
	}
	executor := &testToolExecutor{
		executeFn: func(_ context.Context, calls []types.ToolCall) []tools.ToolResult {
			return []tools.ToolResult{{ToolCallID: calls[0].ID, Result: json.RawMessage(`"search result"`)}}
		},
	}

	r := NewReWOO(provider, executor, nil, DefaultReWOOConfig(), zap.NewNop())
	result, err := r.Execute(context.Background(), "find golang info")
	require.NoError(t, err)
	assert.Equal(t, "rewoo", result.Pattern)
	assert.Equal(t, "final answer", result.FinalAnswer)
	assert.Greater(t, result.TotalTokens, 0)
}

// --- TreeOfThought Execute tests ---

func TestTreeOfThought_Execute_HighScoreEarlyReturn(t *testing.T) {
	t.Parallel()

	callCount := 0
	provider := &testProvider{
		completionFn: func(_ context.Context, _ *llm.ChatRequest) (*llm.ChatResponse, error) {
			callCount++
			if callCount == 1 {
				// generateThoughts: return thoughts as JSON
				return &llm.ChatResponse{
					Choices: []llm.ChatChoice{{Message: types.Message{
						Content: `[{"thought":"approach A","reasoning":"good"},{"thought":"approach B","reasoning":"also good"}]`,
					}}},
					Usage: llm.ChatUsage{TotalTokens: 20},
				}, nil
			}
			// evaluateSingle: return high score
			return &llm.ChatResponse{
				Choices: []llm.ChatChoice{{Message: types.Message{Content: "0.95"}}},
				Usage:   llm.ChatUsage{TotalTokens: 5},
			}, nil
		},
	}

	cfg := DefaultTreeOfThoughtConfig()
	cfg.BranchingFactor = 2
	cfg.MaxDepth = 3
	cfg.PruneThreshold = 0.3
	cfg.Timeout = 10 * time.Second
	tot := NewTreeOfThought(provider, nil, cfg, zap.NewNop())

	result, err := tot.Execute(context.Background(), "solve problem")
	require.NoError(t, err)
	assert.Equal(t, "tree_of_thought", result.Pattern)
	assert.NotEmpty(t, result.FinalAnswer)
	assert.GreaterOrEqual(t, result.Confidence, 0.9)
}

func TestTreeOfThought_Execute_MaxDepthReached(t *testing.T) {
	t.Parallel()

	provider := &testProvider{
		completionFn: func(_ context.Context, _ *llm.ChatRequest) (*llm.ChatResponse, error) {
			return &llm.ChatResponse{
				Choices: []llm.ChatChoice{{Message: types.Message{Content: "0.5"}}},
				Usage:   llm.ChatUsage{TotalTokens: 5},
			}, nil
		},
	}

	cfg := DefaultTreeOfThoughtConfig()
	cfg.BranchingFactor = 1
	cfg.MaxDepth = 1
	cfg.PruneThreshold = 0.1
	cfg.Timeout = 10 * time.Second
	tot := NewTreeOfThought(provider, nil, cfg, zap.NewNop())

	result, err := tot.Execute(context.Background(), "solve problem")
	require.NoError(t, err)
	assert.Equal(t, "tree_of_thought", result.Pattern)
	assert.True(t, result.Metadata["max_depth_reached"].(bool))
}

func TestTreeOfThought_GenerateThoughts_WithParent(t *testing.T) {
	t.Parallel()

	provider := &testProvider{
		completionFn: func(_ context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
			return &llm.ChatResponse{
				Choices: []llm.ChatChoice{{Message: types.Message{
					Content: `[{"thought":"child thought","reasoning":"follows parent"}]`,
				}}},
				Usage: llm.ChatUsage{TotalTokens: 10},
			}, nil
		},
	}

	cfg := DefaultTreeOfThoughtConfig()
	cfg.Timeout = 10 * time.Second
	tot := NewTreeOfThought(provider, nil, cfg, zap.NewNop())

	parent := &ReasoningStep{Content: "parent step"}
	thoughts, tokens, err := tot.generateThoughts(context.Background(), "task", parent, 1)
	require.NoError(t, err)
	assert.Len(t, thoughts, 1)
	assert.Greater(t, tokens, 0)
}

func TestTreeOfThought_GenerateThoughts_FallbackOnBadJSON(t *testing.T) {
	t.Parallel()

	provider := &testProvider{
		completionFn: func(_ context.Context, _ *llm.ChatRequest) (*llm.ChatResponse, error) {
			return &llm.ChatResponse{
				Choices: []llm.ChatChoice{{Message: types.Message{Content: "not valid json"}}},
				Usage:   llm.ChatUsage{TotalTokens: 10},
			}, nil
		},
	}

	cfg := DefaultTreeOfThoughtConfig()
	cfg.Timeout = 10 * time.Second
	tot := NewTreeOfThought(provider, nil, cfg, zap.NewNop())

	thoughts, _, err := tot.generateThoughts(context.Background(), "task", nil, 2)
	require.NoError(t, err)
	require.Len(t, thoughts, 1)
	assert.Equal(t, "not valid json", thoughts[0].Content)
}

func TestTreeOfThought_EvaluateSequential(t *testing.T) {
	t.Parallel()

	provider := &testProvider{
		completionFn: func(_ context.Context, _ *llm.ChatRequest) (*llm.ChatResponse, error) {
			return &llm.ChatResponse{
				Choices: []llm.ChatChoice{{Message: types.Message{Content: "0.75"}}},
				Usage:   llm.ChatUsage{TotalTokens: 5},
			}, nil
		},
	}

	cfg := DefaultTreeOfThoughtConfig()
	cfg.ParallelEval = false
	tot := NewTreeOfThought(provider, nil, cfg, zap.NewNop())

	thoughts := []ReasoningStep{{StepID: "a"}, {StepID: "b"}}
	evaluated, tokens := tot.evaluateSequential(context.Background(), "task", thoughts)
	assert.Len(t, evaluated, 2)
	assert.InDelta(t, 0.75, evaluated[0].Score, 0.01)
	assert.Greater(t, tokens, 0)
}

func TestTreeOfThought_EvaluateSingle_Error(t *testing.T) {
	t.Parallel()

	provider := &testProvider{
		completionFn: func(_ context.Context, _ *llm.ChatRequest) (*llm.ChatResponse, error) {
			return nil, assert.AnError
		},
	}

	cfg := DefaultTreeOfThoughtConfig()
	tot := NewTreeOfThought(provider, nil, cfg, zap.NewNop())

	score, tokens := tot.evaluateSingle(context.Background(), "task", ReasoningStep{})
	assert.Equal(t, 0.5, score)
	assert.Equal(t, 0, tokens)
}

func TestTreeOfThought_EvaluateSingle_OutOfRange(t *testing.T) {
	t.Parallel()

	provider := &testProvider{
		completionFn: func(_ context.Context, _ *llm.ChatRequest) (*llm.ChatResponse, error) {
			return &llm.ChatResponse{
				Choices: []llm.ChatChoice{{Message: types.Message{Content: "5.0"}}},
				Usage:   llm.ChatUsage{TotalTokens: 5},
			}, nil
		},
	}

	cfg := DefaultTreeOfThoughtConfig()
	tot := NewTreeOfThought(provider, nil, cfg, zap.NewNop())

	score, _ := tot.evaluateSingle(context.Background(), "task", ReasoningStep{})
	assert.Equal(t, 0.5, score) // out of range defaults to 0.5
}

// --- ReflexionExecutor Execute tests ---

func TestReflexionExecutor_Execute_SuccessOnFirstTrial(t *testing.T) {
	t.Parallel()

	callCount := 0
	provider := &testProvider{
		completionFn: func(_ context.Context, _ *llm.ChatRequest) (*llm.ChatResponse, error) {
			callCount++
			if callCount == 1 {
				// executeTrial: action
				return &llm.ChatResponse{
					Choices: []llm.ChatChoice{{Message: types.Message{Content: "solution"}}},
					Usage:   llm.ChatUsage{TotalTokens: 20},
				}, nil
			}
			// evaluateTrial: high score
			return &llm.ChatResponse{
				Choices: []llm.ChatChoice{{Message: types.Message{Content: `{"score": 0.9}`}}},
				Usage:   llm.ChatUsage{TotalTokens: 10},
			}, nil
		},
	}

	cfg := DefaultReflexionConfig()
	cfg.MaxTrials = 3
	cfg.SuccessThreshold = 0.8
	cfg.Timeout = 10 * time.Second
	r := NewReflexionExecutor(provider, &testToolExecutor{}, nil, cfg, zap.NewNop())

	result, err := r.Execute(context.Background(), "solve this")
	require.NoError(t, err)
	assert.Equal(t, "reflexion", result.Pattern)
	assert.Equal(t, "solution", result.FinalAnswer)
}

func TestReflexionExecutor_Execute_MultipleTrials(t *testing.T) {
	t.Parallel()

	callCount := 0
	provider := &testProvider{
		completionFn: func(_ context.Context, _ *llm.ChatRequest) (*llm.ChatResponse, error) {
			callCount++
			switch callCount {
			case 1:
				// Trial 1 action
				return &llm.ChatResponse{
					Choices: []llm.ChatChoice{{Message: types.Message{Content: "attempt 1"}}},
					Usage:   llm.ChatUsage{TotalTokens: 10},
				}, nil
			case 2:
				// Trial 1 evaluation: low score
				return &llm.ChatResponse{
					Choices: []llm.ChatChoice{{Message: types.Message{Content: `{"score": 0.3}`}}},
					Usage:   llm.ChatUsage{TotalTokens: 5},
				}, nil
			case 3:
				// Trial 1 reflection
				return &llm.ChatResponse{
					Choices: []llm.ChatChoice{{Message: types.Message{
						Content: `{"analysis":"needs improvement","mistakes":["wrong approach"],"next_strategy":"try harder"}`,
					}}},
					Usage: llm.ChatUsage{TotalTokens: 10},
				}, nil
			case 4:
				// Trial 2 action
				return &llm.ChatResponse{
					Choices: []llm.ChatChoice{{Message: types.Message{Content: "attempt 2"}}},
					Usage:   llm.ChatUsage{TotalTokens: 10},
				}, nil
			default:
				// Trial 2 evaluation: high score
				return &llm.ChatResponse{
					Choices: []llm.ChatChoice{{Message: types.Message{Content: `{"score": 0.9}`}}},
					Usage:   llm.ChatUsage{TotalTokens: 5},
				}, nil
			}
		},
	}

	cfg := DefaultReflexionConfig()
	cfg.MaxTrials = 5
	cfg.SuccessThreshold = 0.8
	cfg.Timeout = 10 * time.Second
	r := NewReflexionExecutor(provider, &testToolExecutor{}, nil, cfg, zap.NewNop())

	result, err := r.Execute(context.Background(), "solve this")
	require.NoError(t, err)
	assert.Equal(t, "attempt 2", result.FinalAnswer)
	assert.Greater(t, len(result.Steps), 1)
}

func TestReflexionExecutor_Execute_WithToolCalls(t *testing.T) {
	t.Parallel()

	callCount := 0
	provider := &testProvider{
		completionFn: func(_ context.Context, _ *llm.ChatRequest) (*llm.ChatResponse, error) {
			callCount++
			if callCount == 1 {
				// Return tool calls
				return &llm.ChatResponse{
					Choices: []llm.ChatChoice{{Message: types.Message{
						ToolCalls: []types.ToolCall{{ID: "tc1", Name: "search", Arguments: json.RawMessage(`{"q":"test"}`)}},
					}}},
					Usage: llm.ChatUsage{TotalTokens: 15},
				}, nil
			}
			// Evaluation: high score
			return &llm.ChatResponse{
				Choices: []llm.ChatChoice{{Message: types.Message{Content: `{"score": 0.85}`}}},
				Usage:   llm.ChatUsage{TotalTokens: 5},
			}, nil
		},
	}

	executor := &testToolExecutor{
		executeFn: func(_ context.Context, calls []types.ToolCall) []tools.ToolResult {
			return []tools.ToolResult{{ToolCallID: calls[0].ID, Result: json.RawMessage(`"tool result"`)}}
		},
	}

	cfg := DefaultReflexionConfig()
	cfg.MaxTrials = 2
	cfg.SuccessThreshold = 0.8
	cfg.Timeout = 10 * time.Second
	r := NewReflexionExecutor(provider, executor, nil, cfg, zap.NewNop())

	result, err := r.Execute(context.Background(), "use tools")
	require.NoError(t, err)
	assert.Contains(t, result.FinalAnswer, "tool result")
}

// --- PlanAndExecute Execute tests ---

func TestPlanAndExecute_Execute_Success(t *testing.T) {
	t.Parallel()

	callCount := 0
	provider := &testProvider{
		completionFn: func(_ context.Context, _ *llm.ChatRequest) (*llm.ChatResponse, error) {
			callCount++
			switch callCount {
			case 1:
				// createPlan
				return &llm.ChatResponse{
					Choices: []llm.ChatChoice{{Message: types.Message{
						Content: `{"goal":"solve","steps":[{"id":"step_1","description":"do thing","tool":"search","arguments":"query"}]}`,
					}}},
					Usage: llm.ChatUsage{TotalTokens: 30},
				}, nil
			default:
				// synthesizeAnswer
				return &llm.ChatResponse{
					Choices: []llm.ChatChoice{{Message: types.Message{Content: "final synthesis"}}},
					Usage:   llm.ChatUsage{TotalTokens: 20},
				}, nil
			}
		},
	}

	executor := &testToolExecutor{
		executeFn: func(_ context.Context, calls []types.ToolCall) []tools.ToolResult {
			return []tools.ToolResult{{ToolCallID: calls[0].ID, Result: json.RawMessage(`"tool done"`)}}
		},
	}

	cfg := DefaultPlanExecuteConfig()
	cfg.Timeout = 10 * time.Second
	pe := NewPlanAndExecute(provider, executor, nil, cfg, zap.NewNop())

	result, err := pe.Execute(context.Background(), "do something")
	require.NoError(t, err)
	assert.Equal(t, "plan_and_execute", result.Pattern)
	assert.Equal(t, "final synthesis", result.FinalAnswer)
	assert.Equal(t, 0.8, result.Confidence)
	assert.Equal(t, "completed", result.Metadata["final_status"])
}

func TestPlanAndExecute_Execute_LLMStep(t *testing.T) {
	t.Parallel()

	callCount := 0
	provider := &testProvider{
		completionFn: func(_ context.Context, _ *llm.ChatRequest) (*llm.ChatResponse, error) {
			callCount++
			switch callCount {
			case 1:
				// createPlan: step without tool
				return &llm.ChatResponse{
					Choices: []llm.ChatChoice{{Message: types.Message{
						Content: `{"goal":"think","steps":[{"id":"step_1","description":"analyze the problem"}]}`,
					}}},
					Usage: llm.ChatUsage{TotalTokens: 20},
				}, nil
			case 2:
				// executeLLMStep
				return &llm.ChatResponse{
					Choices: []llm.ChatChoice{{Message: types.Message{Content: "analysis result"}}},
					Usage:   llm.ChatUsage{TotalTokens: 15},
				}, nil
			default:
				// synthesizeAnswer
				return &llm.ChatResponse{
					Choices: []llm.ChatChoice{{Message: types.Message{Content: "final answer"}}},
					Usage:   llm.ChatUsage{TotalTokens: 10},
				}, nil
			}
		},
	}

	cfg := DefaultPlanExecuteConfig()
	cfg.Timeout = 10 * time.Second
	pe := NewPlanAndExecute(provider, &testToolExecutor{}, nil, cfg, zap.NewNop())

	result, err := pe.Execute(context.Background(), "analyze")
	require.NoError(t, err)
	assert.Equal(t, "final answer", result.FinalAnswer)
}

func TestPlanAndExecute_Execute_ToolFailure_Replan(t *testing.T) {
	t.Parallel()

	callCount := 0
	provider := &testProvider{
		completionFn: func(_ context.Context, _ *llm.ChatRequest) (*llm.ChatResponse, error) {
			callCount++
			switch callCount {
			case 1:
				// createPlan: two steps, first will fail
				return &llm.ChatResponse{
					Choices: []llm.ChatChoice{{Message: types.Message{
						Content: `{"goal":"do","steps":[{"id":"step_1","description":"fail step","tool":"bad_tool","arguments":"x"},{"id":"step_2","description":"next","tool":"good_tool","arguments":"y"}]}`,
					}}},
					Usage: llm.ChatUsage{TotalTokens: 20},
				}, nil
			case 2:
				// replan: new plan with one step
				return &llm.ChatResponse{
					Choices: []llm.ChatChoice{{Message: types.Message{
						Content: `{"goal":"retry","steps":[{"id":"step_r1","description":"retry step","tool":"good_tool","arguments":"z"}]}`,
					}}},
					Usage: llm.ChatUsage{TotalTokens: 15},
				}, nil
			default:
				// synthesizeAnswer
				return &llm.ChatResponse{
					Choices: []llm.ChatChoice{{Message: types.Message{Content: "recovered answer"}}},
					Usage:   llm.ChatUsage{TotalTokens: 10},
				}, nil
			}
		},
	}

	firstCall := true
	executor := &testToolExecutor{
		executeFn: func(_ context.Context, calls []types.ToolCall) []tools.ToolResult {
			if firstCall {
				firstCall = false
				return []tools.ToolResult{{ToolCallID: calls[0].ID, Error: "tool broken"}}
			}
			return []tools.ToolResult{{ToolCallID: calls[0].ID, Result: json.RawMessage(`"ok"`)}}
		},
	}

	cfg := DefaultPlanExecuteConfig()
	cfg.Timeout = 10 * time.Second
	cfg.AdaptivePlanning = true
	cfg.MaxReplanAttempts = 2
	pe := NewPlanAndExecute(provider, executor, nil, cfg, zap.NewNop())

	result, err := pe.Execute(context.Background(), "do something")
	require.NoError(t, err)
	assert.Equal(t, "recovered answer", result.FinalAnswer)
	assert.Equal(t, 1, result.Metadata["replan_attempts"])
}

func TestPlanAndExecute_Execute_PlanFailed(t *testing.T) {
	t.Parallel()

	callCount := 0
	provider := &testProvider{
		completionFn: func(_ context.Context, _ *llm.ChatRequest) (*llm.ChatResponse, error) {
			callCount++
			if callCount == 1 {
				// createPlan
				return &llm.ChatResponse{
					Choices: []llm.ChatChoice{{Message: types.Message{
						Content: `{"goal":"do","steps":[{"id":"step_1","description":"fail","tool":"bad","arguments":"x"}]}`,
					}}},
					Usage: llm.ChatUsage{TotalTokens: 20},
				}, nil
			}
			return nil, assert.AnError
		},
	}

	executor := &testToolExecutor{
		executeFn: func(_ context.Context, calls []types.ToolCall) []tools.ToolResult {
			return []tools.ToolResult{{ToolCallID: calls[0].ID, Error: "broken"}}
		},
	}

	cfg := DefaultPlanExecuteConfig()
	cfg.Timeout = 10 * time.Second
	cfg.AdaptivePlanning = true
	cfg.MaxReplanAttempts = 1
	pe := NewPlanAndExecute(provider, executor, nil, cfg, zap.NewNop())

	result, err := pe.Execute(context.Background(), "do something")
	require.NoError(t, err)
	assert.Equal(t, 0.2, result.Confidence)
	assert.Equal(t, "failed", result.Metadata["final_status"])
}

func TestPlanAndExecute_CreatePlan_BadJSON(t *testing.T) {
	t.Parallel()

	callCount := 0
	provider := &testProvider{
		completionFn: func(_ context.Context, _ *llm.ChatRequest) (*llm.ChatResponse, error) {
			callCount++
			if callCount == 1 {
				// createPlan: bad JSON, should fallback to minimal plan
				return &llm.ChatResponse{
					Choices: []llm.ChatChoice{{Message: types.Message{Content: "not json"}}},
					Usage:   llm.ChatUsage{TotalTokens: 10},
				}, nil
			}
			if callCount == 2 {
				// executeLLMStep for the fallback step
				return &llm.ChatResponse{
					Choices: []llm.ChatChoice{{Message: types.Message{Content: "direct result"}}},
					Usage:   llm.ChatUsage{TotalTokens: 10},
				}, nil
			}
			// synthesizeAnswer
			return &llm.ChatResponse{
				Choices: []llm.ChatChoice{{Message: types.Message{Content: "synthesized"}}},
				Usage:   llm.ChatUsage{TotalTokens: 10},
			}, nil
		},
	}

	cfg := DefaultPlanExecuteConfig()
	cfg.Timeout = 10 * time.Second
	pe := NewPlanAndExecute(provider, &testToolExecutor{}, nil, cfg, zap.NewNop())

	result, err := pe.Execute(context.Background(), "do something")
	require.NoError(t, err)
	assert.Equal(t, "synthesized", result.FinalAnswer)
}

// --- DynamicPlanner Execute test ---

func TestDynamicPlanner_Execute_Success(t *testing.T) {
	t.Parallel()

	callCount := 0
	provider := &testProvider{
		completionFn: func(_ context.Context, _ *llm.ChatRequest) (*llm.ChatResponse, error) {
			callCount++
			switch callCount {
			case 1:
				// generateNextSteps: initial plan
				return &llm.ChatResponse{
					Choices: []llm.ChatChoice{{Message: types.Message{
						Content: `[{"action":"search","description":"find info","confidence":0.8}]`,
					}}},
					Usage: llm.ChatUsage{TotalTokens: 20},
				}, nil
			case 2:
				// executeLLMNode
				return &llm.ChatResponse{
					Choices: []llm.ChatChoice{{Message: types.Message{Content: "Final Answer: the result is 42"}}},
					Usage:   llm.ChatUsage{TotalTokens: 15},
				}, nil
			default:
				// synthesizeFinalAnswer
				return &llm.ChatResponse{
					Choices: []llm.ChatChoice{{Message: types.Message{Content: "42"}}},
					Usage:   llm.ChatUsage{TotalTokens: 10},
				}, nil
			}
		},
	}

	cfg := DefaultDynamicPlannerConfig()
	cfg.Timeout = 10 * time.Second
	dp := NewDynamicPlanner(provider, &testToolExecutor{}, nil, cfg, zap.NewNop())

	result, err := dp.Execute(context.Background(), "what is the answer")
	require.NoError(t, err)
	assert.Equal(t, "dynamic_planner", result.Pattern)
	assert.NotEmpty(t, result.FinalAnswer)
}

// --- IterativeDeepening Execute test ---

func TestIterativeDeepening_Execute_Success(t *testing.T) {
	t.Parallel()

	callCount := 0
	provider := &testProvider{
		completionFn: func(_ context.Context, _ *llm.ChatRequest) (*llm.ChatResponse, error) {
			callCount++
			switch {
			case callCount == 1:
				// analyzeQuery
				return &llm.ChatResponse{
					Choices: []llm.ChatChoice{{Message: types.Message{
						Content: `{"aspects":["aspect1"],"initial_queries":["query1"],"depth_strategy":"breadth_first"}`,
					}}},
					Usage: llm.ChatUsage{TotalTokens: 20},
				}, nil
			case callCount == 2:
				// generateDirections
				return &llm.ChatResponse{
					Choices: []llm.ChatChoice{{Message: types.Message{
						Content: `[{"direction":"dir1","query":"q1","priority":0.9}]`,
					}}},
					Usage: llm.ChatUsage{TotalTokens: 10},
				}, nil
			case callCount == 3:
				// executeQueries -> generateQueries
				return &llm.ChatResponse{
					Choices: []llm.ChatChoice{{Message: types.Message{
						Content: `[{"finding":"found something","relevance":0.9,"source":"test"}]`,
					}}},
					Usage: llm.ChatUsage{TotalTokens: 15},
				}, nil
			default:
				// synthesize
				return &llm.ChatResponse{
					Choices: []llm.ChatChoice{{Message: types.Message{Content: "synthesized answer"}}},
					Usage:   llm.ChatUsage{TotalTokens: 10},
				}, nil
			}
		},
	}

	cfg := DefaultIterativeDeepeningConfig()
	cfg.Timeout = 10 * time.Second
	cfg.MaxDepth = 1
	cfg.Breadth = 1
	cfg.MinConfidence = 0.0 // low threshold so it finishes quickly
	id := NewIterativeDeepening(provider, nil, cfg, zap.NewNop())

	result, err := id.Execute(context.Background(), "research topic")
	require.NoError(t, err)
	assert.Equal(t, "iterative_deepening", result.Pattern)
	assert.NotEmpty(t, result.FinalAnswer)
}
