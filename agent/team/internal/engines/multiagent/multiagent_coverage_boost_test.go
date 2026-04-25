package multiagent

import (
	"context"
	"fmt"
	"testing"
	"time"

	agent "github.com/BaSui01/agentflow/agent/runtime"

	llm "github.com/BaSui01/agentflow/llm/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ---------------------------------------------------------------------------
// safeStubProvider coverage
// ---------------------------------------------------------------------------

func TestSafeStubProvider_Completion(t *testing.T) {
	sp := safeStubProvider{}
	resp, err := sp.Completion(context.Background(), &llm.ChatRequest{Model: "test"})
	require.NoError(t, err)
	assert.Equal(t, "test", resp.Model)
	assert.Contains(t, resp.Choices[0].Message.Content, "stub")
}

func TestSafeStubProvider_Stream(t *testing.T) {
	sp := safeStubProvider{}
	ch, err := sp.Stream(context.Background(), &llm.ChatRequest{Model: "test"})
	require.NoError(t, err)
	chunk := <-ch
	assert.Equal(t, "test", chunk.Model)
	assert.Contains(t, chunk.Delta.Content, "stub")
}

func TestSafeStubProvider_HealthCheck(t *testing.T) {
	sp := safeStubProvider{}
	status, err := sp.HealthCheck(context.Background())
	require.NoError(t, err)
	assert.True(t, status.Healthy)
}

func TestSafeStubProvider_Name(t *testing.T) {
	sp := safeStubProvider{}
	assert.Equal(t, "safe-stub", sp.Name())
}

func TestSafeStubProvider_SupportsNativeFunctionCalling(t *testing.T) {
	sp := safeStubProvider{}
	assert.False(t, sp.SupportsNativeFunctionCalling())
}

func TestSafeStubProvider_ListModels(t *testing.T) {
	sp := safeStubProvider{}
	models, err := sp.ListModels(context.Background())
	require.NoError(t, err)
	assert.Nil(t, models)
}

func TestSafeStubProvider_Endpoints(t *testing.T) {
	sp := safeStubProvider{}
	ep := sp.Endpoints()
	assert.Empty(t, ep.Completion)
	assert.Empty(t, ep.Models)
	assert.Empty(t, ep.BaseURL)
}

// ---------------------------------------------------------------------------
// crew mode coverage
// ---------------------------------------------------------------------------

func TestCrewModeStrategy_Execute(t *testing.T) {
	agents := []agent.Agent{
		&mockAgent{id: "a1", name: "one", output: &agent.Output{Content: "first", Duration: time.Millisecond}},
		&mockAgent{id: "a2", name: "two", output: &agent.Output{Content: "second", Duration: time.Millisecond}},
	}
	strategy := newCrewModeStrategy(zap.NewNop())
	out, err := strategy.Execute(context.Background(), agents, &agent.Input{TraceID: "trace-1", Content: "start"})
	require.NoError(t, err)
	assert.Contains(t, out.Content, "[one] first")
	assert.Contains(t, out.Content, "[two] second")
	assert.Equal(t, "trace-1", out.TraceID)
	assert.Equal(t, ModeCrew, out.Metadata["mode"])
}

// ---------------------------------------------------------------------------
// ScopedStores coverage
// ---------------------------------------------------------------------------

func TestScopedStores_NewAndScopedKey(t *testing.T) {
	inner := agent.NewPersistenceStores(zap.NewNop())
	ss := NewScopedStores(inner, "sub-agent-1", zap.NewNop())
	require.NotNil(t, ss)
	assert.Equal(t, "sub-agent-1/mykey", ss.scopedKey("mykey"))
}

func TestScopedStores_RecordRun(t *testing.T) {
	inner := agent.NewPersistenceStores(zap.NewNop())
	// No run store set, so RecordRun returns ""
	ss := NewScopedStores(inner, "sub-1", zap.NewNop())
	runID := ss.RecordRun(context.Background(), "tenant-1", "trace-1", "input", time.Now())
	assert.Empty(t, runID)
}

func TestScopedStores_UpdateRunStatus(t *testing.T) {
	inner := agent.NewPersistenceStores(zap.NewNop())
	ss := NewScopedStores(inner, "sub-1", zap.NewNop())
	err := ss.UpdateRunStatus(context.Background(), "", "completed", nil, "")
	require.NoError(t, err)
}

func TestScopedStores_PersistConversation(t *testing.T) {
	inner := agent.NewPersistenceStores(zap.NewNop())
	ss := NewScopedStores(inner, "sub-1", zap.NewNop())
	// No conversation store set, should not panic
	ss.PersistConversation(context.Background(), "conv-1", "tenant-1", "user-1", "hello", "world")
}

func TestScopedStores_RestoreConversation(t *testing.T) {
	inner := agent.NewPersistenceStores(zap.NewNop())
	ss := NewScopedStores(inner, "sub-1", zap.NewNop())
	// No conversation store set, returns empty
	msgs := ss.RestoreConversation(context.Background(), "conv-1")
	assert.Empty(t, msgs)
}

func TestScopedStores_LoadPrompt(t *testing.T) {
	inner := agent.NewPersistenceStores(zap.NewNop())
	ss := NewScopedStores(inner, "sub-1", zap.NewNop())
	// No prompt store set, returns nil
	doc := ss.LoadPrompt(context.Background(), "generic", "default", "tenant-1")
	assert.Nil(t, doc)
}

// ---------------------------------------------------------------------------
// ModeRegistry edge cases
// ---------------------------------------------------------------------------

func TestModeRegistry_Get_NotFound(t *testing.T) {
	reg := NewModeRegistry()
	_, err := reg.Get("nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not registered")
}

func TestModeRegistry_RegisterOverwrite(t *testing.T) {
	reg := NewModeRegistry()
	reg.Register(&testMode{name: "m1"})
	out1, _ := reg.Execute(context.Background(), "m1", nil, &agent.Input{})
	assert.Equal(t, "mode:m1", out1.Content)

	// Overwrite with a different implementation
	reg.Register(&testModeV2{name: "m1"})
	out2, _ := reg.Execute(context.Background(), "m1", nil, &agent.Input{})
	assert.Equal(t, "v2:m1", out2.Content)
}

type testModeV2 struct{ name string }

func (m *testModeV2) Name() string { return m.name }
func (m *testModeV2) Execute(_ context.Context, _ []agent.Agent, _ *agent.Input) (*agent.Output, error) {
	return &agent.Output{Content: "v2:" + m.name}, nil
}

func TestRegisterDefaultModes_NilRegistry(t *testing.T) {
	err := RegisterDefaultModes(nil, zap.NewNop())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil")
}

func TestRegisterDefaultModes_NilLogger(t *testing.T) {
	reg := NewModeRegistry()
	err := RegisterDefaultModes(reg, nil)
	require.NoError(t, err)
	assert.True(t, len(reg.List()) >= 8)
}

// ---------------------------------------------------------------------------
// WorkerPool edge cases
// ---------------------------------------------------------------------------

func TestWorkerPool_EmptyTasks(t *testing.T) {
	pool := NewWorkerPool(DefaultWorkerPoolConfig(), nil)
	results, err := pool.Execute(context.Background(), nil)
	require.NoError(t, err)
	assert.Nil(t, results)
}

func TestWorkerPool_NilLogger(t *testing.T) {
	pool := NewWorkerPool(DefaultWorkerPoolConfig(), nil)
	require.NotNil(t, pool)
}

func TestWorkerPool_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	pool := NewWorkerPool(DefaultWorkerPoolConfig(), zap.NewNop())
	tasks := []WorkerTask{
		{AgentID: "a1", Agent: &mockAgent{id: "a1", delay: 5 * time.Second}},
	}
	results, err := pool.Execute(ctx, tasks)
	require.NoError(t, err) // PartialResult policy
	assert.Len(t, results, 1)
	assert.Error(t, results[0].Err)
}

func TestWorkerPool_TaskTimeout(t *testing.T) {
	cfg := WorkerPoolConfig{
		FailurePolicy: PolicyPartialResult,
		TaskTimeout:   1 * time.Millisecond,
	}
	pool := NewWorkerPool(cfg, zap.NewNop())
	tasks := []WorkerTask{
		{AgentID: "slow", Agent: &mockAgent{id: "slow", delay: 5 * time.Second}},
	}
	results, err := pool.Execute(context.Background(), tasks)
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Error(t, results[0].Err)
}

func TestWorkerPool_RetryAllSucceedOnSecondAttempt(t *testing.T) {
	callCount := 0
	cfg := WorkerPoolConfig{
		FailurePolicy: PolicyRetryFailed,
		MaxRetries:    3,
		TaskTimeout:   5 * time.Second,
	}
	pool := NewWorkerPool(cfg, zap.NewNop())
	tasks := []WorkerTask{
		{AgentID: "flaky", Agent: &mockAgent{id: "flaky", execFn: func(_ context.Context, _ *agent.Input) (*agent.Output, error) {
			callCount++
			if callCount <= 1 {
				return nil, fmt.Errorf("transient error")
			}
			return &agent.Output{Content: "ok"}, nil
		}}},
	}
	results, err := pool.Execute(context.Background(), tasks)
	require.NoError(t, err)
	assert.Nil(t, results[0].Err)
	assert.Equal(t, "ok", results[0].Content)
}

func TestWorkerPool_RetryExhausted(t *testing.T) {
	cfg := WorkerPoolConfig{
		FailurePolicy: PolicyRetryFailed,
		MaxRetries:    2,
		TaskTimeout:   5 * time.Second,
	}
	pool := NewWorkerPool(cfg, zap.NewNop())
	tasks := []WorkerTask{
		{AgentID: "always-fail", Agent: &mockAgent{id: "always-fail", err: fmt.Errorf("permanent")}},
	}
	results, err := pool.Execute(context.Background(), tasks)
	require.NoError(t, err)
	assert.Error(t, results[0].Err)
}

// ---------------------------------------------------------------------------
// Supervisor edge cases
// ---------------------------------------------------------------------------

func TestSupervisor_SplitterError(t *testing.T) {
	splitter := &errorSplitter{err: fmt.Errorf("split failed")}
	sup := NewSupervisor(splitter, DefaultSupervisorConfig(), zap.NewNop())
	_, err := sup.Run(context.Background(), &agent.Input{TraceID: "t1", Content: "test"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "split failed")
}

type errorSplitter struct{ err error }

func (s *errorSplitter) Split(_ context.Context, _ *agent.Input) ([]WorkerTask, error) {
	return nil, s.err
}

func TestSupervisor_EmptyTasks(t *testing.T) {
	splitter := &emptySplitter{}
	sup := NewSupervisor(splitter, DefaultSupervisorConfig(), zap.NewNop())
	_, err := sup.Run(context.Background(), &agent.Input{TraceID: "t1", Content: "test"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "zero tasks")
}

type emptySplitter struct{}

func (s *emptySplitter) Split(_ context.Context, _ *agent.Input) ([]WorkerTask, error) {
	return nil, nil
}

func TestSupervisor_NilLogger(t *testing.T) {
	agents := []agent.Agent{
		&mockAgent{id: "w1", output: &agent.Output{Content: "ok"}},
	}
	splitter := &StaticSplitter{Agents: agents}
	sup := NewSupervisor(splitter, DefaultSupervisorConfig(), nil)
	result, err := sup.Run(context.Background(), &agent.Input{TraceID: "t1", Content: "test"})
	require.NoError(t, err)
	assert.Equal(t, 1, result.SourceCount)
}

func TestSupervisor_WithWeights(t *testing.T) {
	agents := []agent.Agent{
		&mockAgent{id: "w1", output: &agent.Output{Content: "r1", TokensUsed: 5}},
		&mockAgent{id: "w2", output: &agent.Output{Content: "r2", TokensUsed: 10}},
	}
	splitter := &StaticSplitter{
		Agents:  agents,
		Weights: map[string]float64{"w1": 2.0, "w2": 1.0},
	}
	cfg := DefaultSupervisorConfig()
	cfg.AggregationStrategy = StrategyWeightedMerge
	sup := NewSupervisor(splitter, cfg, zap.NewNop())
	result, err := sup.Run(context.Background(), &agent.Input{TraceID: "t1", Content: "test"})
	require.NoError(t, err)
	assert.Equal(t, 2, result.SourceCount)
	assert.Equal(t, "weighted_merge", result.FinishReason)
}

// ---------------------------------------------------------------------------
// primaryModeStrategy edge cases
// ---------------------------------------------------------------------------

func TestPrimaryMode_NoAgents(t *testing.T) {
	reg := NewModeRegistry()
	require.NoError(t, RegisterDefaultModes(reg, zap.NewNop()))
	_, err := reg.Execute(context.Background(), ModeReasoning, nil, &agent.Input{Content: "hi"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least one agent")
}

func TestPrimaryMode_NilMetadata(t *testing.T) {
	reg := NewModeRegistry()
	require.NoError(t, RegisterDefaultModes(reg, zap.NewNop()))
	ag := &mockAgent{id: "a1", output: &agent.Output{Content: "ok", Metadata: nil}}
	out, err := reg.Execute(context.Background(), ModeReasoning, []agent.Agent{ag}, &agent.Input{Content: "hi"})
	require.NoError(t, err)
	assert.Equal(t, ModeReasoning, out.Metadata["mode"])
}

// ---------------------------------------------------------------------------
// collaborationModeStrategy edge cases
// ---------------------------------------------------------------------------

func TestCollaborationMode_TooFewAgents(t *testing.T) {
	reg := NewModeRegistry()
	require.NoError(t, RegisterDefaultModes(reg, zap.NewNop()))
	_, err := reg.Execute(context.Background(), ModeCollaboration, []agent.Agent{
		&mockAgent{id: "a1", output: &agent.Output{Content: "ok"}},
	}, &agent.Input{Content: "hi"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least two agents")
}

func TestCollaborationMode_NilInput(t *testing.T) {
	// collaborationPatternFromInput with nil input
	pattern := collaborationPatternFromInput(nil)
	assert.NotEmpty(t, string(pattern))
}

func TestCollaborationMode_NilContext(t *testing.T) {
	pattern := collaborationPatternFromInput(&agent.Input{Content: "hi", Context: nil})
	assert.NotEmpty(t, string(pattern))
}

func TestParallelMode_NoAgents(t *testing.T) {
	reg := NewModeRegistry()
	require.NoError(t, RegisterDefaultModes(reg, zap.NewNop()))
	_, err := reg.Execute(context.Background(), ModeParallel, nil, &agent.Input{Content: "hi"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least one agent")
}

func TestAggregationStrategyFromInput_DefaultsToMergeAll(t *testing.T) {
	assert.Equal(t, StrategyMergeAll, aggregationStrategyFromInput(nil))
	assert.Equal(t, StrategyMergeAll, aggregationStrategyFromInput(&agent.Input{Context: nil}))
	assert.Equal(t, StrategyMergeAll, aggregationStrategyFromInput(&agent.Input{Context: map[string]any{"aggregation_strategy": "unknown"}}))
}

func TestAggregationStrategyFromInput_RecognizesSupportedValues(t *testing.T) {
	assert.Equal(t, StrategyBestOfN, aggregationStrategyFromInput(&agent.Input{Context: map[string]any{"aggregation_strategy": "best_of_n"}}))
	assert.Equal(t, StrategyVoteMajority, aggregationStrategyFromInput(&agent.Input{Context: map[string]any{"aggregation_strategy": "vote_majority"}}))
	assert.Equal(t, StrategyWeightedMerge, aggregationStrategyFromInput(&agent.Input{Context: map[string]any{"aggregation_strategy": "weighted_merge"}}))
}

// ---------------------------------------------------------------------------
// loopModeStrategy edge cases
// ---------------------------------------------------------------------------

func TestLoopMode_NoAgents(t *testing.T) {
	reg := NewModeRegistry()
	require.NoError(t, RegisterDefaultModes(reg, zap.NewNop()))
	_, err := reg.Execute(context.Background(), ModeLoop, nil, &agent.Input{Content: "hi"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least one agent")
}

func TestLoopMode_DefaultMaxIterations(t *testing.T) {
	callCount := 0
	ag := &mockAgent{id: "a1", execFn: func(_ context.Context, _ *agent.Input) (*agent.Output, error) {
		callCount++
		return &agent.Output{Content: "working"}, nil
	}}
	reg := NewModeRegistry()
	require.NoError(t, RegisterDefaultModes(reg, zap.NewNop()))
	out, err := reg.Execute(context.Background(), ModeLoop, []agent.Agent{ag}, &agent.Input{Content: "hi"})
	require.NoError(t, err)
	assert.Equal(t, defaultLoopMaxIterations, callCount)
	assert.Equal(t, ModeLoop, out.Metadata["mode"])
}

func TestLoopMode_CustomStopKeyword(t *testing.T) {
	callCount := 0
	ag := &mockAgent{id: "a1", execFn: func(_ context.Context, _ *agent.Input) (*agent.Output, error) {
		callCount++
		if callCount >= 2 {
			return &agent.Output{Content: "DONE_NOW"}, nil
		}
		return &agent.Output{Content: "working"}, nil
	}}
	reg := NewModeRegistry()
	require.NoError(t, RegisterDefaultModes(reg, zap.NewNop()))
	out, err := reg.Execute(context.Background(), ModeLoop, []agent.Agent{ag}, &agent.Input{
		Content: "hi",
		Context: map[string]any{"stop_keyword": "DONE_NOW", "max_iterations": 10},
	})
	require.NoError(t, err)
	assert.Equal(t, 2, callCount)
	assert.Equal(t, "DONE_NOW", out.Content)
}

func TestLoopMode_StopInReasoningContent(t *testing.T) {
	callCount := 0
	ag := &mockAgent{id: "a1", execFn: func(_ context.Context, _ *agent.Input) (*agent.Output, error) {
		callCount++
		if callCount >= 2 {
			rc := "I think LOOP_COMPLETE is appropriate"
			return &agent.Output{
				Content:          "still thinking",
				ReasoningContent: &rc,
			}, nil
		}
		return &agent.Output{Content: "working"}, nil
	}}
	reg := NewModeRegistry()
	require.NoError(t, RegisterDefaultModes(reg, zap.NewNop()))
	out, err := reg.Execute(context.Background(), ModeLoop, []agent.Agent{ag}, &agent.Input{
		Content: "hi",
		Context: map[string]any{"max_iterations": 10},
	})
	require.NoError(t, err)
	assert.Equal(t, 2, callCount)
	assert.Equal(t, ModeLoop, out.Metadata["mode"])
	assert.Equal(t, "still thinking", out.Content)
}

func TestLoopMode_AgentFailsFirstIterationNoFallback(t *testing.T) {
	ag := &mockAgent{id: "a1", err: fmt.Errorf("boom")}
	reg := NewModeRegistry()
	require.NoError(t, RegisterDefaultModes(reg, zap.NewNop()))
	_, err := reg.Execute(context.Background(), ModeLoop, []agent.Agent{ag}, &agent.Input{
		Content: "hi",
		Context: map[string]any{"max_iterations": 3},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed at iteration 1")
}

func TestLoopMode_AgentFailsLaterIterationFallsBack(t *testing.T) {
	callCount := 0
	ag := &mockAgent{id: "a1", execFn: func(_ context.Context, _ *agent.Input) (*agent.Output, error) {
		callCount++
		if callCount >= 2 {
			return nil, fmt.Errorf("transient")
		}
		return &agent.Output{Content: "first-ok"}, nil
	}}
	reg := NewModeRegistry()
	require.NoError(t, RegisterDefaultModes(reg, zap.NewNop()))
	out, err := reg.Execute(context.Background(), ModeLoop, []agent.Agent{ag}, &agent.Input{
		Content: "hi",
		Context: map[string]any{"max_iterations": 5},
	})
	require.NoError(t, err)
	assert.Equal(t, "first-ok", out.Content)
}

func TestLoopMode_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ag := &mockAgent{id: "a1", output: &agent.Output{Content: "ok"}}
	reg := NewModeRegistry()
	require.NoError(t, RegisterDefaultModes(reg, zap.NewNop()))
	_, err := reg.Execute(ctx, ModeLoop, []agent.Agent{ag}, &agent.Input{
		Content: "hi",
		Context: map[string]any{"max_iterations": 5},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cancelled")
}

func TestLoopMode_MultipleAgentsRoundRobin(t *testing.T) {
	var ids []string
	makeAgent := func(id string) *mockAgent {
		return &mockAgent{id: id, execFn: func(_ context.Context, _ *agent.Input) (*agent.Output, error) {
			ids = append(ids, id)
			return &agent.Output{Content: "working"}, nil
		}}
	}
	reg := NewModeRegistry()
	require.NoError(t, RegisterDefaultModes(reg, zap.NewNop()))
	_, err := reg.Execute(context.Background(), ModeLoop, []agent.Agent{
		makeAgent("a1"), makeAgent("a2"),
	}, &agent.Input{
		Content: "hi",
		Context: map[string]any{"max_iterations": 4},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"a1", "a2", "a1", "a2"}, ids)
}

// ---------------------------------------------------------------------------
// Aggregator edge cases
// ---------------------------------------------------------------------------

func TestAggregator_UnknownStrategy(t *testing.T) {
	agg := NewAggregator(AggregationStrategy("unknown"))
	results := []WorkerResult{
		{AgentID: "a1", Content: "hello"},
	}
	out, err := agg.Aggregate(results)
	require.NoError(t, err)
	// falls back to mergeAll
	assert.Equal(t, "hello", out.Content)
}

func TestAggregator_WeightedMerge_ZeroWeights(t *testing.T) {
	agg := NewAggregator(StrategyWeightedMerge)
	results := []WorkerResult{
		{AgentID: "a1", Content: "alpha", Weight: 0},
		{AgentID: "a2", Content: "beta", Weight: 0},
	}
	out, err := agg.Aggregate(results)
	require.NoError(t, err)
	// zero total weight falls back to mergeAll
	assert.Equal(t, "merge_all", out.FinishReason)
}

func TestAggregator_MixedSuccessAndFailure(t *testing.T) {
	agg := NewAggregator(StrategyMergeAll)
	results := []WorkerResult{
		{AgentID: "a1", Content: "ok", TokensUsed: 5},
		{AgentID: "a2", Err: fmt.Errorf("fail")},
	}
	out, err := agg.Aggregate(results)
	require.NoError(t, err)
	assert.Equal(t, 1, out.SourceCount)
	assert.Equal(t, 1, out.FailedCount)
	assert.Equal(t, "ok", out.Content)
}

// ---------------------------------------------------------------------------
// DefaultWorkerPoolConfig / DefaultSupervisorConfig
// ---------------------------------------------------------------------------

func TestDefaultWorkerPoolConfig(t *testing.T) {
	cfg := DefaultWorkerPoolConfig()
	assert.Equal(t, PolicyPartialResult, cfg.FailurePolicy)
	assert.Equal(t, 2, cfg.MaxRetries)
	assert.Equal(t, 5*time.Minute, cfg.TaskTimeout)
}

func TestDefaultSupervisorConfig(t *testing.T) {
	cfg := DefaultSupervisorConfig()
	assert.Equal(t, StrategyMergeAll, cfg.AggregationStrategy)
	assert.Equal(t, PolicyPartialResult, cfg.FailurePolicy)
	assert.Equal(t, 2, cfg.MaxRetries)
	assert.Equal(t, 5*time.Minute, cfg.TaskTimeout)
}

// ---------------------------------------------------------------------------
// StaticSplitter
// ---------------------------------------------------------------------------

func TestStaticSplitter_NoWeights(t *testing.T) {
	agents := []agent.Agent{
		&mockAgent{id: "a1"},
		&mockAgent{id: "a2"},
	}
	splitter := &StaticSplitter{Agents: agents}
	tasks, err := splitter.Split(context.Background(), &agent.Input{Content: "test"})
	require.NoError(t, err)
	assert.Len(t, tasks, 2)
	assert.Equal(t, 1.0, tasks[0].Weight)
	assert.Equal(t, 1.0, tasks[1].Weight)
}

func TestStaticSplitter_WithWeights(t *testing.T) {
	agents := []agent.Agent{
		&mockAgent{id: "a1"},
		&mockAgent{id: "a2"},
	}
	splitter := &StaticSplitter{
		Agents:  agents,
		Weights: map[string]float64{"a1": 3.0},
	}
	tasks, err := splitter.Split(context.Background(), &agent.Input{Content: "test"})
	require.NoError(t, err)
	assert.Equal(t, 3.0, tasks[0].Weight)
	assert.Equal(t, 1.0, tasks[1].Weight) // default
}
