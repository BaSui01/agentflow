package multiagent

import (
	"context"
	"fmt"
	"testing"
	"time"

	agent "github.com/BaSui01/agentflow/agent/runtime"
)

// mockAgent implements agent.Agent for testing.
type mockAgent struct {
	id     string
	name   string
	output *agent.Output
	err    error
	delay  time.Duration
	execFn func(ctx context.Context, input *agent.Input) (*agent.Output, error)
}

func (m *mockAgent) ID() string                       { return m.id }
func (m *mockAgent) Name() string                     { return m.name }
func (m *mockAgent) Type() agent.AgentType            { return "mock" }
func (m *mockAgent) State() agent.State               { return agent.StateReady }
func (m *mockAgent) Init(_ context.Context) error     { return nil }
func (m *mockAgent) Teardown(_ context.Context) error { return nil }
func (m *mockAgent) Plan(_ context.Context, _ *agent.Input) (*agent.PlanResult, error) {
	return nil, nil
}
func (m *mockAgent) Observe(_ context.Context, _ *agent.Feedback) error { return nil }

func (m *mockAgent) Execute(ctx context.Context, input *agent.Input) (*agent.Output, error) {
	if m.execFn != nil {
		return m.execFn(ctx, input)
	}
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return m.output, m.err
}

// --- Aggregator tests ---

func TestAggregator_MergeAll(t *testing.T) {
	agg := NewAggregator(StrategyMergeAll)
	results := []WorkerResult{
		{AgentID: "a1", Content: "hello", TokensUsed: 10, Cost: 0.01, Duration: time.Second},
		{AgentID: "a2", Content: "world", TokensUsed: 20, Cost: 0.02, Duration: 2 * time.Second},
	}
	out, err := agg.Aggregate(results)
	if err != nil {
		t.Fatal(err)
	}
	if out.SourceCount != 2 {
		t.Errorf("expected 2 sources, got %d", out.SourceCount)
	}
	if out.TokensUsed != 30 {
		t.Errorf("expected 30 tokens, got %d", out.TokensUsed)
	}
	if out.Duration != 2*time.Second {
		t.Errorf("expected 2s duration, got %v", out.Duration)
	}
}

func TestAggregator_BestOfN(t *testing.T) {
	agg := NewAggregator(StrategyBestOfN)
	results := []WorkerResult{
		{AgentID: "a1", Content: "low", Score: 0.3},
		{AgentID: "a2", Content: "high", Score: 0.9},
		{AgentID: "a3", Content: "mid", Score: 0.6},
	}
	out, err := agg.Aggregate(results)
	if err != nil {
		t.Fatal(err)
	}
	if out.Content != "high" {
		t.Errorf("expected 'high', got %q", out.Content)
	}
}

func TestAggregator_VoteMajority(t *testing.T) {
	agg := NewAggregator(StrategyVoteMajority)
	results := []WorkerResult{
		{AgentID: "a1", Content: "yes"},
		{AgentID: "a2", Content: "no"},
		{AgentID: "a3", Content: "yes"},
	}
	out, err := agg.Aggregate(results)
	if err != nil {
		t.Fatal(err)
	}
	if out.Content != "yes" {
		t.Errorf("expected 'yes', got %q", out.Content)
	}
}

func TestAggregator_WeightedMerge(t *testing.T) {
	agg := NewAggregator(StrategyWeightedMerge)
	results := []WorkerResult{
		{AgentID: "a1", Content: "alpha", Weight: 3.0},
		{AgentID: "a2", Content: "beta", Weight: 1.0},
	}
	out, err := agg.Aggregate(results)
	if err != nil {
		t.Fatal(err)
	}
	if out.FinishReason != "weighted_merge" {
		t.Errorf("expected weighted_merge, got %q", out.FinishReason)
	}
}

func TestAggregator_AllFailed(t *testing.T) {
	agg := NewAggregator(StrategyMergeAll)
	results := []WorkerResult{
		{AgentID: "a1", Err: fmt.Errorf("fail1")},
		{AgentID: "a2", Err: fmt.Errorf("fail2")},
	}
	_, err := agg.Aggregate(results)
	if err == nil {
		t.Fatal("expected error when all failed")
	}
}

// --- WorkerPool tests ---

func TestWorkerPool_PartialResult(t *testing.T) {
	pool := NewWorkerPool(DefaultWorkerPoolConfig(), nil)
	tasks := []WorkerTask{
		{AgentID: "ok", Agent: &mockAgent{id: "ok", output: &agent.Output{Content: "done"}}},
		{AgentID: "fail", Agent: &mockAgent{id: "fail", err: fmt.Errorf("boom")}},
	}
	results, err := pool.Execute(context.Background(), tasks)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Err != nil {
		t.Errorf("first result should succeed")
	}
	if results[1].Err == nil {
		t.Errorf("second result should fail")
	}
}

func TestWorkerPool_FailFast(t *testing.T) {
	cfg := DefaultWorkerPoolConfig()
	cfg.FailurePolicy = PolicyFailFast
	pool := NewWorkerPool(cfg, nil)
	tasks := []WorkerTask{
		{AgentID: "fail", Agent: &mockAgent{id: "fail", err: fmt.Errorf("boom")}},
		{AgentID: "slow", Agent: &mockAgent{id: "slow", output: &agent.Output{Content: "done"}, delay: 5 * time.Second}},
	}
	_, err := pool.Execute(context.Background(), tasks)
	if err == nil {
		t.Fatal("expected error with FailFast policy")
	}
}

func TestWorkerPool_RetryFailed(t *testing.T) {
	callCount := 0
	cfg := DefaultWorkerPoolConfig()
	cfg.FailurePolicy = PolicyRetryFailed
	cfg.MaxRetries = 2
	pool := NewWorkerPool(cfg, nil)
	tasks := []WorkerTask{
		{AgentID: "flaky", Agent: &mockAgent{id: "flaky", execFn: func(_ context.Context, _ *agent.Input) (*agent.Output, error) {
			callCount++
			if callCount <= 1 {
				return nil, fmt.Errorf("transient")
			}
			return &agent.Output{Content: "recovered"}, nil
		}}},
	}
	results, err := pool.Execute(context.Background(), tasks)
	if err != nil {
		t.Fatal(err)
	}
	if results[0].Err != nil {
		t.Errorf("expected recovery after retry, got err: %v", results[0].Err)
	}
	if results[0].Content != "recovered" {
		t.Errorf("expected 'recovered', got %q", results[0].Content)
	}
}

// --- Supervisor tests ---

func TestSupervisor_Run(t *testing.T) {
	agents := []agent.Agent{
		&mockAgent{id: "w1", output: &agent.Output{Content: "result1", TokensUsed: 5}},
		&mockAgent{id: "w2", output: &agent.Output{Content: "result2", TokensUsed: 10}},
	}
	splitter := &StaticSplitter{Agents: agents}
	cfg := DefaultSupervisorConfig()
	sup := NewSupervisor(splitter, cfg, nil)

	input := &agent.Input{TraceID: "t1", Content: "test"}
	agg, err := sup.Run(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if agg.SourceCount != 2 {
		t.Errorf("expected 2 sources, got %d", agg.SourceCount)
	}
	if agg.TokensUsed != 15 {
		t.Errorf("expected 15 tokens, got %d", agg.TokensUsed)
	}
}

func TestSupervisor_AllWorkersFail(t *testing.T) {
	agents := []agent.Agent{
		&mockAgent{id: "w1", err: fmt.Errorf("fail1")},
		&mockAgent{id: "w2", err: fmt.Errorf("fail2")},
	}
	splitter := &StaticSplitter{Agents: agents}
	cfg := DefaultSupervisorConfig()
	sup := NewSupervisor(splitter, cfg, nil)

	input := &agent.Input{TraceID: "t1", Content: "test"}
	_, err := sup.Run(context.Background(), input)
	if err == nil {
		t.Fatal("expected error when all workers fail")
	}
}

// --- ModeRegistry tests ---

type testMode struct {
	name string
}

func (m *testMode) Name() string { return m.name }
func (m *testMode) Execute(_ context.Context, _ []agent.Agent, _ *agent.Input) (*agent.Output, error) {
	return &agent.Output{Content: "mode:" + m.name}, nil
}

func TestModeRegistry_RegisterAndExecute(t *testing.T) {
	reg := NewModeRegistry()
	reg.Register(&testMode{name: "reasoning"})
	reg.Register(&testMode{name: "collaboration"})

	names := reg.List()
	if len(names) != 2 {
		t.Errorf("expected 2 modes, got %d", len(names))
	}

	out, err := reg.Execute(context.Background(), "reasoning", nil, &agent.Input{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Content != "mode:reasoning" {
		t.Errorf("expected 'mode:reasoning', got %q", out.Content)
	}

	_, err = reg.Execute(context.Background(), "unknown", nil, &agent.Input{})
	if err == nil {
		t.Fatal("expected error for unknown mode")
	}
}
