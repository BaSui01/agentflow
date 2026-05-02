package runtime

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type asyncPolicyAgent struct {
	id            string
	active        *int32
	maxSeen       *int32
	sleepDuration time.Duration
}

func (a *asyncPolicyAgent) ID() string                 { return a.id }
func (a *asyncPolicyAgent) Name() string               { return a.id }
func (a *asyncPolicyAgent) Type() AgentType            { return TypeGeneric }
func (a *asyncPolicyAgent) State() State               { return StateReady }
func (a *asyncPolicyAgent) Init(context.Context) error { return nil }
func (a *asyncPolicyAgent) Teardown(context.Context) error { return nil }
func (a *asyncPolicyAgent) Plan(context.Context, *Input) (*PlanResult, error) { return &PlanResult{}, nil }
func (a *asyncPolicyAgent) Observe(context.Context, *Feedback) error { return nil }
func (a *asyncPolicyAgent) Execute(ctx context.Context, input *Input) (*Output, error) {
	cur := atomic.AddInt32(a.active, 1)
	for {
		prev := atomic.LoadInt32(a.maxSeen)
		if cur <= prev || atomic.CompareAndSwapInt32(a.maxSeen, prev, cur) {
			break
		}
	}
	defer atomic.AddInt32(a.active, -1)

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(a.sleepDuration):
	}
	return &Output{TraceID: input.TraceID, Content: a.id}, nil
}

func TestAsyncExecutor_ExecuteWithSubagents_RespectsMaxParallelism(t *testing.T) {
	var active int32
	var maxSeen int32
	root := &asyncPolicyAgent{id: "root", active: &active, maxSeen: &maxSeen, sleepDuration: 5 * time.Millisecond}
	exec := NewAsyncExecutor(root, zap.NewNop())
	defer exec.manager.Close()
	exec.SetMaxParallelism(2)

	subagents := []Agent{
		&asyncPolicyAgent{id: "a1", active: &active, maxSeen: &maxSeen, sleepDuration: 20 * time.Millisecond},
		&asyncPolicyAgent{id: "a2", active: &active, maxSeen: &maxSeen, sleepDuration: 20 * time.Millisecond},
		&asyncPolicyAgent{id: "a3", active: &active, maxSeen: &maxSeen, sleepDuration: 20 * time.Millisecond},
		&asyncPolicyAgent{id: "a4", active: &active, maxSeen: &maxSeen, sleepDuration: 20 * time.Millisecond},
	}

	out, err := exec.ExecuteWithSubagents(context.Background(), &Input{TraceID: "trace-1", Content: "task"}, subagents)
	require.NoError(t, err)
	require.NotNil(t, out)
	assert.LessOrEqual(t, atomic.LoadInt32(&maxSeen), int32(2))
}

func TestAsyncExecutor_ExecuteWithSubagents_RejectsWhenMaxDepthReached(t *testing.T) {
	var active int32
	var maxSeen int32
	root := &asyncPolicyAgent{id: "root", active: &active, maxSeen: &maxSeen, sleepDuration: 1 * time.Millisecond}
	exec := NewAsyncExecutor(root, zap.NewNop())
	defer exec.manager.Close()

	ctx := types.WithSubagentDepth(context.Background(), 2)
	ctx = WithRunConfig(ctx, &RunConfig{SubagentMaxParallelism: IntPtr(2)})
	input := &Input{
		TraceID: "trace-depth",
		Content: "task",
		Context: map[string]any{
			"subagent_max_depth": 2,
		},
	}

	_, err := exec.ExecuteWithSubagents(ctx, input, []Agent{
		&asyncPolicyAgent{id: "a1", active: &active, maxSeen: &maxSeen, sleepDuration: 1 * time.Millisecond},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "subagent max depth reached")
}

func TestRealtimeCoordinator_CoordinateSubagents_RejectsWhenMaxDepthReached(t *testing.T) {
	var active int32
	var maxSeen int32
	manager := NewSubagentManager(zap.NewNop())
	defer manager.Close()
	coord := NewRealtimeCoordinator(manager, nil, zap.NewNop())

	ctx := types.WithSubagentDepth(context.Background(), 2)
	ctx = WithRunConfig(ctx, &RunConfig{SubagentMaxDepth: IntPtr(2), SubagentMaxParallelism: IntPtr(2)})

	_, err := coord.CoordinateSubagents(ctx, []Agent{
		&asyncPolicyAgent{id: "a1", active: &active, maxSeen: &maxSeen, sleepDuration: 1 * time.Millisecond},
	}, &Input{TraceID: "trace-coord", Content: "task"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "subagent max depth reached")
}
