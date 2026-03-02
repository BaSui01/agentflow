package tools

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"go.uber.org/zap"
)

// TestGetUsage_KeyMatchesBuildUsageKey verifies that GetUsage correctly
// aggregates usage recorded via RecordCost / CheckBudget, which use
// buildUsageKey internally. This was broken when GetUsage built a key
// with an empty budgetID ("scope:scopeID::periodKey") that never matched
// the buildUsageKey format ("scope:scopeID:budgetID:periodKey").
func TestGetUsage_KeyMatchesBuildUsageKey(t *testing.T) {
	cc := NewCostController(zap.NewNop())

	budget := CreateAgentBudget("b1", "agent budget", "agent-1", 1000, BudgetPeriodDaily)
	if err := cc.AddBudget(budget); err != nil {
		t.Fatalf("AddBudget: %v", err)
	}

	// Record some cost
	err := cc.RecordCost(&CostRecord{
		AgentID:  "agent-1",
		ToolName: "tool-a",
		Cost:     42.0,
		Unit:     CostUnitCredits,
	})
	if err != nil {
		t.Fatalf("RecordCost: %v", err)
	}

	// GetUsage must return the recorded cost
	usage := cc.GetUsage(BudgetScopeAgent, "agent-1", BudgetPeriodDaily)
	if usage != 42.0 {
		t.Errorf("GetUsage = %v, want 42.0", usage)
	}
}

// TestGetUsage_MultipleBudgetsSameScope verifies that GetUsage sums
// usage across multiple budgets with the same scope and scopeID.
func TestGetUsage_MultipleBudgetsSameScope(t *testing.T) {
	cc := NewCostController(zap.NewNop())

	b1 := CreateAgentBudget("b1", "budget 1", "agent-1", 1000, BudgetPeriodDaily)
	b2 := CreateAgentBudget("b2", "budget 2", "agent-1", 500, BudgetPeriodDaily)
	if err := cc.AddBudget(b1); err != nil {
		t.Fatalf("AddBudget b1: %v", err)
	}
	if err := cc.AddBudget(b2); err != nil {
		t.Fatalf("AddBudget b2: %v", err)
	}

	if err := cc.RecordCost(&CostRecord{AgentID: "agent-1", ToolName: "t1", Cost: 10.0, Unit: CostUnitCredits}); err != nil {
		t.Fatalf("RecordCost t1: %v", err)
	}
	if err := cc.RecordCost(&CostRecord{AgentID: "agent-1", ToolName: "t2", Cost: 20.0, Unit: CostUnitCredits}); err != nil {
		t.Fatalf("RecordCost t2: %v", err)
	}

	// Both budgets match agent-1, so each gets 30.0 of usage.
	// GetUsage should sum across both budget keys = 60.0
	usage := cc.GetUsage(BudgetScopeAgent, "agent-1", BudgetPeriodDaily)
	if usage != 60.0 {
		t.Errorf("GetUsage = %v, want 60.0", usage)
	}
}

// TestResetUsageIfNeeded_CalendarBased verifies that resetUsageIfNeeded
// uses calendar-based period keys rather than duration-based checks.
// We simulate a period change by directly injecting an old-period key.
func TestResetUsageIfNeeded_CalendarBased(t *testing.T) {
	cc := NewCostController(zap.NewNop())

	budget := &Budget{
		ID:      "b1",
		Name:    "test",
		Scope:   BudgetScopeAgent,
		ScopeID: "agent-1",
		Limit:   1000,
		Period:  BudgetPeriodDaily,
		Enabled: true,
	}
	if err := cc.AddBudget(budget); err != nil {
		t.Fatalf("AddBudget: %v", err)
	}

	// Inject an old-period usage key (yesterday)
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	oldKey := "agent:agent-1:b1:" + yesterday
	cc.mu.Lock()
	cc.usage[oldKey] = 100.0
	cc.mu.Unlock()

	// Also add current period usage
	currentKey := cc.buildUsageKey(budget)
	cc.mu.Lock()
	cc.usage[currentKey] = 50.0
	cc.mu.Unlock()

	// resetUsageIfNeeded should remove the old key but keep the current one
	cc.mu.Lock()
	cc.resetUsageIfNeeded(budget)
	cc.mu.Unlock()

	cc.mu.RLock()
	defer cc.mu.RUnlock()

	if _, exists := cc.usage[oldKey]; exists {
		t.Error("old period key should have been deleted")
	}
	if cc.usage[currentKey] != 50.0 {
		t.Errorf("current period usage = %v, want 50.0", cc.usage[currentKey])
	}
}

// TestResetUsageIfNeeded_TotalPeriodNeverResets verifies that budgets
// with BudgetPeriodTotal are never reset.
func TestResetUsageIfNeeded_TotalPeriodNeverResets(t *testing.T) {
	cc := NewCostController(zap.NewNop())

	budget := &Budget{
		ID:      "b1",
		Name:    "total budget",
		Scope:   BudgetScopeGlobal,
		Limit:   10000,
		Period:  BudgetPeriodTotal,
		Enabled: true,
	}
	if err := cc.AddBudget(budget); err != nil {
		t.Fatalf("AddBudget: %v", err)
	}

	key := cc.buildUsageKey(budget)
	cc.mu.Lock()
	cc.usage[key] = 5000.0
	cc.resetUsageIfNeeded(budget)
	cc.mu.Unlock()

	cc.mu.RLock()
	defer cc.mu.RUnlock()
	if cc.usage[key] != 5000.0 {
		t.Errorf("total period usage = %v, want 5000.0 (should not reset)", cc.usage[key])
	}
}

// TestCheckBudget_ExceedsBudget verifies that CheckBudget correctly
// blocks calls that would exceed the budget limit.
func TestCheckBudget_ExceedsBudget(t *testing.T) {
	cc := NewCostController(zap.NewNop())

	budget := CreateAgentBudget("b1", "agent budget", "agent-1", 100, BudgetPeriodDaily)
	if err := cc.AddBudget(budget); err != nil {
		t.Fatalf("AddBudget: %v", err)
	}

	// Record 90 credits of usage
	if err := cc.RecordCost(&CostRecord{AgentID: "agent-1", ToolName: "t1", Cost: 90.0, Unit: CostUnitCredits}); err != nil {
		t.Fatalf("RecordCost: %v", err)
	}

	// Trying to spend 20 more should be denied (90 + 20 > 100)
	result, err := cc.CheckBudget(context.Background(), "agent-1", "", "", "t1", 20.0)
	if err != nil {
		t.Fatalf("CheckBudget: %v", err)
	}
	if result.Allowed {
		t.Error("expected budget to be exceeded, but was allowed")
	}

	// Trying to spend 10 should be allowed (90 + 10 = 100)
	result, err = cc.CheckBudget(context.Background(), "agent-1", "", "", "t1", 10.0)
	if err != nil {
		t.Fatalf("CheckBudget: %v", err)
	}
	if !result.Allowed {
		t.Errorf("expected budget check to pass, got reason: %s", result.Reason)
	}
}

// TestCalculateCost_DefaultAndConfigured verifies cost calculation
// for both configured and unconfigured tools.
func TestCalculateCost_DefaultAndConfigured(t *testing.T) {
	cc := NewCostController(zap.NewNop())

	// Unconfigured tool should return default cost of 1.0
	cost, err := cc.CalculateCost("unknown-tool", nil)
	if err != nil {
		t.Fatalf("CalculateCost: %v", err)
	}
	if cost != 1.0 {
		t.Errorf("default cost = %v, want 1.0", cost)
	}

	// Configured tool with base cost only
	if err := cc.SetToolCost(&ToolCost{ToolName: "my-tool", BaseCost: 5.0, Unit: CostUnitCredits}); err != nil {
		t.Fatalf("SetToolCost my-tool: %v", err)
	}
	cost, err = cc.CalculateCost("my-tool", nil)
	if err != nil {
		t.Fatalf("CalculateCost: %v", err)
	}
	if cost != 5.0 {
		t.Errorf("configured cost = %v, want 5.0", cost)
	}

	// Configured tool with per-unit cost (tokens)
	if err := cc.SetToolCost(&ToolCost{ToolName: "token-tool", BaseCost: 1.0, CostPerUnit: 0.01, Unit: CostUnitTokens}); err != nil {
		t.Fatalf("SetToolCost token-tool: %v", err)
	}
	args := json.RawMessage(`{"prompt": "hello world"}`)
	cost, err = cc.CalculateCost("token-tool", args)
	if err != nil {
		t.Fatalf("CalculateCost: %v", err)
	}
	// base(1.0) + len(args)/4 * 0.01
	expectedExtra := float64(len(args)) / 4.0 * 0.01
	if cost != 1.0+expectedExtra {
		t.Errorf("token cost = %v, want %v", cost, 1.0+expectedExtra)
	}
}

