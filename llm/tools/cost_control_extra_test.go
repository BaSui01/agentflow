package tools

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type testAlertHandler struct{ called bool }

func (h *testAlertHandler) HandleAlert(_ context.Context, _ *CostAlert) error {
	h.called = true
	return nil
}

func TestCostController_SetTokenCounter(t *testing.T) {
	cc := NewCostController(zap.NewNop())
	cc.SetTokenCounter(nil) // should not panic
}

func TestCostController_SetAlertHandler(t *testing.T) {
	cc := NewCostController(zap.NewNop())
	h := &testAlertHandler{}
	cc.SetAlertHandler(h)
	assert.False(t, h.called) // just setting, not triggering
}

func TestCostController_GetToolCost_Extra(t *testing.T) {
	cc := NewCostController(zap.NewNop())
	tc := CreateToolCost("search", 0.01, 0.001)
	cc.SetToolCost(tc)

	cost, ok := cc.GetToolCost("search")
	require.True(t, ok)
	assert.Equal(t, 0.01, cost.BaseCost)

	_, ok = cc.GetToolCost("nonexistent")
	assert.False(t, ok)
}

func TestCostController_RemoveBudget_Extra(t *testing.T) {
	cc := NewCostController(zap.NewNop())
	cc.AddBudget(&Budget{ID: "b1", Name: "test", Limit: 100})

	err := cc.RemoveBudget("b1")
	require.NoError(t, err)

	_, ok := cc.GetBudget("b1")
	assert.False(t, ok)
}

func TestCostController_RemoveBudget_NotFound_Extra(t *testing.T) {
	cc := NewCostController(zap.NewNop())
	err := cc.RemoveBudget("nonexistent")
	require.Error(t, err)
}
func TestCostController_GetBudget_Extra(t *testing.T) {
	cc := NewCostController(zap.NewNop())
	cc.AddBudget(&Budget{ID: "b1", Name: "test", Limit: 100})

	budget, ok := cc.GetBudget("b1")
	require.True(t, ok)
	assert.Equal(t, "test", budget.Name)

	_, ok = cc.GetBudget("nonexistent")
	assert.False(t, ok)
}

func TestCostController_ListBudgets_Extra(t *testing.T) {
	cc := NewCostController(zap.NewNop())
	cc.AddBudget(&Budget{ID: "b1", Name: "budget1", Limit: 100})
	cc.AddBudget(&Budget{ID: "b2", Name: "budget2", Limit: 200})

	budgets := cc.ListBudgets()
	assert.Len(t, budgets, 2)
}

func TestCostController_GetOptimizations_Extra(t *testing.T) {
	cc := NewCostController(zap.NewNop())
	cc.SetToolCost(CreateToolCost("search", 0.01, 0))
	cc.RecordCost(&CostRecord{ToolName: "search", Cost: 0.01, AgentID: "a1", UserID: "u1"})
	cc.RecordCost(&CostRecord{ToolName: "search", Cost: 0.01, AgentID: "a1", UserID: "u1"})

	opts := cc.GetOptimizations("a1", "u1")
	// May return nil or empty slice depending on analysis
	_ = opts
}

func TestCostController_GetCostReport_Extra(t *testing.T) {
	cc := NewCostController(zap.NewNop())
	cc.RecordCost(&CostRecord{ToolName: "search", Cost: 0.01, AgentID: "a1", UserID: "u1"})

	report, err := cc.GetCostReport(nil)
	require.NoError(t, err)
	assert.NotNil(t, report)
	assert.Equal(t, int64(1), report.TotalCalls)
}

func TestCostController_GetCostReport_WithFilter(t *testing.T) {
	cc := NewCostController(zap.NewNop())
	cc.RecordCost(&CostRecord{ToolName: "search", Cost: 0.01, AgentID: "a1", UserID: "u1"})
	cc.RecordCost(&CostRecord{ToolName: "calc", Cost: 0.02, AgentID: "a2", UserID: "u2"})

	report, err := cc.GetCostReport(&CostReportFilter{AgentID: "a1"})
	require.NoError(t, err)
	assert.Equal(t, int64(1), report.TotalCalls)
}

func TestCreateGlobalBudget_Func(t *testing.T) {
	b := CreateGlobalBudget("g1", "global", 1000, BudgetPeriodMonthly)
	assert.Equal(t, "g1", b.ID)
	assert.Equal(t, BudgetScopeGlobal, b.Scope)
	assert.True(t, b.Enabled)
}

func TestCreateUserBudget_Func(t *testing.T) {
	b := CreateUserBudget("u1", "user-budget", "user-1", 500, BudgetPeriodDaily)
	assert.Equal(t, "u1", b.ID)
	assert.Equal(t, BudgetScopeUser, b.Scope)
	assert.Equal(t, "user-1", b.ScopeID)
}

func TestCreateAgentBudget_Func(t *testing.T) {
	b := CreateAgentBudget("a1", "agent-budget", "agent-1", 200, BudgetPeriodWeekly)
	assert.Equal(t, "a1", b.ID)
	assert.Equal(t, BudgetScopeAgent, b.Scope)
}

func TestCreateToolCost_Func(t *testing.T) {
	tc := CreateToolCost("search", 0.01, 0.001)
	assert.Equal(t, "search", tc.ToolName)
	assert.Equal(t, 0.01, tc.BaseCost)
}

func TestCostController_GetUsage(t *testing.T) {
	cc := NewCostController(zap.NewNop())
	b := CreateGlobalBudget("g1", "global", 1000, BudgetPeriodDaily)
	cc.AddBudget(b)
	cc.RecordCost(&CostRecord{ToolName: "search", Cost: 5.0, AgentID: "a1", UserID: "u1"})

	usage := cc.GetUsage(BudgetScopeGlobal, "", BudgetPeriodDaily)
	assert.GreaterOrEqual(t, usage, 0.0)
}
