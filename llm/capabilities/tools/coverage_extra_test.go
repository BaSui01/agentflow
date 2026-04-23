package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	llmpkg "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// --- CostController: getPeriodKey / getAlertLevel / findApplicableBudgets ---

func TestCostController_CalculateCost_WithTokenCounter(t *testing.T) {
	cc := NewCostController(zap.NewNop())
	cc.SetTokenCounter(&fakeTokenCounter{count: 10})
	require.NoError(t, cc.SetToolCost(&ToolCost{ToolName: "search", BaseCost: 0.01, CostPerUnit: 0.001, Unit: CostUnitTokens}))

	cost, err := cc.CalculateCost("search", json.RawMessage(`{"q":"hello"}`))
	require.NoError(t, err)
	assert.InDelta(t, 0.01+10*0.001, cost, 0.0001)
}

func TestCostController_CalculateCost_Credits(t *testing.T) {
	cc := NewCostController(zap.NewNop())
	require.NoError(t, cc.SetToolCost(&ToolCost{ToolName: "calc", BaseCost: 0.05, CostPerUnit: 0.01, Unit: CostUnitCredits}))

	args := json.RawMessage(`{"x":1}`)
	cost, err := cc.CalculateCost("calc", args)
	require.NoError(t, err)
	expected := 0.05 + float64(len(args))/100.0*0.01
	assert.InDelta(t, expected, cost, 0.0001)
}

func TestCostController_CalculateCost_Dollars(t *testing.T) {
	cc := NewCostController(zap.NewNop())
	require.NoError(t, cc.SetToolCost(&ToolCost{ToolName: "api", BaseCost: 0.1, CostPerUnit: 0.02, Unit: CostUnitDollars}))

	args := json.RawMessage(`{"data":"test"}`)
	cost, err := cc.CalculateCost("api", args)
	require.NoError(t, err)
	assert.Greater(t, cost, 0.1)
}

func TestCostController_CalculateCost_TokensFallback(t *testing.T) {
	cc := NewCostController(zap.NewNop())
	// No token counter set, so it falls back to char-based estimation
	require.NoError(t, cc.SetToolCost(&ToolCost{ToolName: "tok", BaseCost: 0.0, CostPerUnit: 0.001, Unit: CostUnitTokens}))

	args := json.RawMessage(`{"text":"hello world"}`)
	cost, err := cc.CalculateCost("tok", args)
	require.NoError(t, err)
	expected := float64(len(args)) / 4.0 * 0.001
	assert.InDelta(t, expected, cost, 0.0001)
}

func TestCostController_CalculateCost_Unknown(t *testing.T) {
	cc := NewCostController(zap.NewNop())
	cost, err := cc.CalculateCost("unknown_tool", json.RawMessage(`{}`))
	require.NoError(t, err)
	assert.Equal(t, 1.0, cost) // default cost
}

func TestCostController_FindApplicableBudgets_AllScopes(t *testing.T) {
	cc := NewCostController(zap.NewNop())
	require.NoError(t, cc.AddBudget(&Budget{ID: "g1", Scope: BudgetScopeGlobal, Limit: 100, Enabled: true}))
	require.NoError(t, cc.AddBudget(&Budget{ID: "a1", Scope: BudgetScopeAgent, ScopeID: "agent-1", Limit: 50, Enabled: true}))
	require.NoError(t, cc.AddBudget(&Budget{ID: "u1", Scope: BudgetScopeUser, ScopeID: "user-1", Limit: 30, Enabled: true}))
	require.NoError(t, cc.AddBudget(&Budget{ID: "s1", Scope: BudgetScopeSession, ScopeID: "sess-1", Limit: 20, Enabled: true}))
	require.NoError(t, cc.AddBudget(&Budget{ID: "t1", Scope: BudgetScopeTool, ScopeID: "search", Limit: 10, Enabled: true}))

	// CheckBudget exercises findApplicableBudgets internally
	result, err := cc.CheckBudget(context.Background(), "agent-1", "user-1", "sess-1", "search", 1.0)
	require.NoError(t, err)
	assert.True(t, result.Allowed)
}

func TestCostController_CheckBudget_Exceeded(t *testing.T) {
	cc := NewCostController(zap.NewNop())
	require.NoError(t, cc.AddBudget(&Budget{
		ID: "g1", Scope: BudgetScopeGlobal, Limit: 5, Period: BudgetPeriodTotal,
		AlertThresholds: []float64{80, 100}, Enabled: true,
	}))
	// Record enough cost to exceed budget
	require.NoError(t, cc.RecordCost(&CostRecord{ToolName: "t", Cost: 4.5, AgentID: "a", UserID: "u"}))

	result, err := cc.CheckBudget(context.Background(), "a", "u", "", "t", 2.0)
	require.NoError(t, err)
	assert.False(t, result.Allowed)
}

func TestCostController_CheckBudget_AlertTriggered(t *testing.T) {
	h := &testAlertHandler{}
	cc := NewCostController(zap.NewNop())
	cc.SetAlertHandler(h)
	require.NoError(t, cc.AddBudget(&Budget{
		ID: "g1", Scope: BudgetScopeGlobal, Limit: 10, Period: BudgetPeriodTotal,
		AlertThresholds: []float64{50, 80, 100}, Enabled: true,
	}))
	// Usage at 4.5 (45%), then adding 1.0 brings it to 5.5 (55%) -- crosses 50% threshold
	require.NoError(t, cc.RecordCost(&CostRecord{ToolName: "t", Cost: 4.5, AgentID: "a", UserID: "u"}))

	result, err := cc.CheckBudget(context.Background(), "a", "u", "", "t", 1.0)
	require.NoError(t, err)
	assert.True(t, result.Allowed)
	assert.NotNil(t, result.Alert)
}

// --- CostControlMiddleware ---

func TestCostControlMiddleware_Allowed(t *testing.T) {
	cc := NewCostController(zap.NewNop())
	require.NoError(t, cc.SetToolCost(&ToolCost{ToolName: "echo", BaseCost: 0.01}))
	require.NoError(t, cc.AddBudget(&Budget{ID: "g1", Scope: BudgetScopeGlobal, Limit: 100, Period: BudgetPeriodTotal, Enabled: true}))

	inner := func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`"ok"`), nil
	}

	mw := CostControlMiddleware(cc, nil)
	wrapped := mw(inner)

	ctx := WithPermissionContext(context.Background(), &PermissionContext{
		AgentID: "a1", UserID: "u1", ToolName: "echo",
	})
	resp, err := wrapped(ctx, json.RawMessage(`{}`))
	require.NoError(t, err)
	assert.Equal(t, json.RawMessage(`"ok"`), resp)
}

func TestCostControlMiddleware_BudgetExceeded(t *testing.T) {
	cc := NewCostController(zap.NewNop())
	require.NoError(t, cc.AddBudget(&Budget{ID: "g1", Scope: BudgetScopeGlobal, Limit: 0.5, Period: BudgetPeriodTotal, Enabled: true}))
	// Default cost is 1.0 for unknown tools, which exceeds 0.5

	inner := func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`"ok"`), nil
	}

	mw := CostControlMiddleware(cc, nil)
	wrapped := mw(inner)

	_, err := wrapped(context.Background(), json.RawMessage(`{}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "budget exceeded")
}

// --- MemoryAuditBackend.Close ---

func TestMemoryAuditBackend_Close(t *testing.T) {
	backend := NewMemoryAuditBackend(100)
	err := backend.Close()
	assert.NoError(t, err)
}

// --- RateLimitManager: SetQueueHandler / SetDegradeHandler ---

type fakeQueueHandler struct{}

func (f *fakeQueueHandler) Enqueue(_ context.Context, _ *RateLimitContext) error { return nil }
func (f *fakeQueueHandler) Dequeue(_ context.Context) (*RateLimitContext, error) { return nil, nil }

type fakeDegradeHandler struct{}

func (f *fakeDegradeHandler) GetDegradedResponse(_ context.Context, _ *RateLimitContext) (json.RawMessage, error) {
	return json.RawMessage(`"degraded"`), nil
}

func TestRateLimitManager_SetQueueHandler(t *testing.T) {
	rlm := NewRateLimitManager(zap.NewNop())
	rlm.SetQueueHandler(&fakeQueueHandler{})
}

func TestRateLimitManager_SetDegradeHandler(t *testing.T) {
	rlm := NewRateLimitManager(zap.NewNop())
	rlm.SetDegradeHandler(&fakeDegradeHandler{})
}

// --- ToolCallChain ---

func TestNewToolCallChain(t *testing.T) {
	reg := NewDefaultRegistry(zap.NewNop())
	re := NewResilientExecutor(reg, DefaultFallbackConfig(), zap.NewNop())
	chain := NewToolCallChain(re, zap.NewNop())
	require.NotNil(t, chain)
}

func TestToolCallChain_ExecuteChain_Success(t *testing.T) {
	reg := NewDefaultRegistry(zap.NewNop())
	require.NoError(t, reg.Register("step1", func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{"value":"from-step1"}`), nil
	}, ToolMetadata{Description: "step1"}))
	require.NoError(t, reg.Register("step2", func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{"result":"done"}`), nil
	}, ToolMetadata{Description: "step2"}))

	re := NewResilientExecutor(reg, DefaultFallbackConfig(), zap.NewNop())
	chain := NewToolCallChain(re, zap.NewNop())

	calls := []llmpkg.ToolCall{
		{ID: "call1", Name: "step1", Arguments: json.RawMessage(`{"input":"start"}`)},
		{ID: "call2", Name: "step2", Arguments: json.RawMessage(`{"ref":"${call1.value}"}`)},
	}

	results, err := chain.ExecuteChain(context.Background(), calls)
	require.NoError(t, err)
	assert.Len(t, results, 2)
	assert.Empty(t, results[0].Error)
	assert.Empty(t, results[1].Error)
}

func TestToolCallChain_ExecuteChain_ErrorStopsChain(t *testing.T) {
	reg := NewDefaultRegistry(zap.NewNop())
	require.NoError(t, reg.Register("fail", func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
		return nil, fmt.Errorf("step failed")
	}, ToolMetadata{Description: "fail"}))
	require.NoError(t, reg.Register("step2", func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`"ok"`), nil
	}, ToolMetadata{Description: "step2"}))

	cfg := DefaultFallbackConfig()
	cfg.MaxRetries = 0
	re := NewResilientExecutor(reg, cfg, zap.NewNop())
	chain := NewToolCallChain(re, zap.NewNop())

	calls := []llmpkg.ToolCall{
		{ID: "call1", Name: "fail", Arguments: json.RawMessage(`{}`)},
		{ID: "call2", Name: "step2", Arguments: json.RawMessage(`{}`)},
	}

	results, err := chain.ExecuteChain(context.Background(), calls)
	require.NoError(t, err)
	assert.Len(t, results, 1) // chain stopped after first error
	assert.NotEmpty(t, results[0].Error)
}

// --- RegisterStreaming ---

func TestDefaultRegistry_RegisterStreaming(t *testing.T) {
	reg := NewDefaultRegistry(zap.NewNop())
	fn := func(ctx context.Context, args json.RawMessage, emit ToolProgressEmitter) (json.RawMessage, error) {
		emit(ToolStreamEvent{Type: "progress", Data: "50%"})
		return json.RawMessage(`"done"`), nil
	}

	err := reg.RegisterStreaming("stream-tool", fn, ToolMetadata{Description: "streaming tool"})
	require.NoError(t, err)

	// Verify it's registered as both normal and streaming
	_, _, err2 := reg.Get("stream-tool")
	assert.NoError(t, err2)

	sfn, found := reg.GetStreaming("stream-tool")
	assert.True(t, found)
	assert.NotNil(t, sfn)
}

// --- fakeTokenCounter ---

type fakeTokenCounter struct{ count int }

func (f *fakeTokenCounter) CountTokens(_ string) int { return f.count }

// Ensure it satisfies the interface
var _ types.TokenCounter = (*fakeTokenCounter)(nil)
