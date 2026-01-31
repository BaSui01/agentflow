// Package tools provides cost control for tool execution in enterprise AI Agent frameworks.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// CostAlertLevel represents the severity of a cost alert.
type CostAlertLevel string

const (
	CostAlertLevelInfo     CostAlertLevel = "info"
	CostAlertLevelWarning  CostAlertLevel = "warning"
	CostAlertLevelCritical CostAlertLevel = "critical"
)

// CostUnit represents the unit of cost measurement.
type CostUnit string

const (
	CostUnitCredits CostUnit = "credits"
	CostUnitDollars CostUnit = "dollars"
	CostUnitTokens  CostUnit = "tokens"
)

// ToolCost defines the cost configuration for a tool.
type ToolCost struct {
	ToolName    string   `json:"tool_name"`
	BaseCost    float64  `json:"base_cost"`              // Base cost per call
	CostPerUnit float64  `json:"cost_per_unit"`          // Cost per unit (e.g., per token)
	Unit        CostUnit `json:"unit"`                   // Cost unit
	Description string   `json:"description,omitempty"`
}

// Budget defines a budget configuration.
type Budget struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Scope       BudgetScope    `json:"scope"`
	ScopeID     string         `json:"scope_id,omitempty"` // Agent ID, User ID, etc.
	Limit       float64        `json:"limit"`              // Budget limit
	Unit        CostUnit       `json:"unit"`
	Period      BudgetPeriod   `json:"period"`
	AlertThresholds []float64  `json:"alert_thresholds,omitempty"` // Percentages (e.g., 50, 80, 100)
	Enabled     bool           `json:"enabled"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// BudgetScope defines the scope of a budget.
type BudgetScope string

const (
	BudgetScopeGlobal  BudgetScope = "global"
	BudgetScopeAgent   BudgetScope = "agent"
	BudgetScopeUser    BudgetScope = "user"
	BudgetScopeSession BudgetScope = "session"
	BudgetScopeTool    BudgetScope = "tool"
)

// BudgetPeriod defines the period of a budget.
type BudgetPeriod string

const (
	BudgetPeriodHourly  BudgetPeriod = "hourly"
	BudgetPeriodDaily   BudgetPeriod = "daily"
	BudgetPeriodWeekly  BudgetPeriod = "weekly"
	BudgetPeriodMonthly BudgetPeriod = "monthly"
	BudgetPeriodTotal   BudgetPeriod = "total" // No reset
)

// CostRecord represents a single cost record.
type CostRecord struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	AgentID   string    `json:"agent_id"`
	UserID    string    `json:"user_id"`
	SessionID string    `json:"session_id,omitempty"`
	ToolName  string    `json:"tool_name"`
	Cost      float64   `json:"cost"`
	Unit      CostUnit  `json:"unit"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// CostAlert represents a cost alert.
type CostAlert struct {
	ID        string         `json:"id"`
	Timestamp time.Time      `json:"timestamp"`
	Level     CostAlertLevel `json:"level"`
	BudgetID  string         `json:"budget_id"`
	Message   string         `json:"message"`
	Current   float64        `json:"current"`
	Limit     float64        `json:"limit"`
	Percentage float64       `json:"percentage"`
}

// CostCheckResult contains the result of a cost check.
type CostCheckResult struct {
	Allowed     bool           `json:"allowed"`
	Cost        float64        `json:"cost"`
	Budget      *Budget        `json:"budget,omitempty"`
	CurrentUsage float64       `json:"current_usage"`
	Remaining   float64        `json:"remaining"`
	Alert       *CostAlert     `json:"alert,omitempty"`
	Reason      string         `json:"reason,omitempty"`
}

// CostOptimization represents a cost optimization suggestion.
type CostOptimization struct {
	Type        string  `json:"type"`
	Description string  `json:"description"`
	Savings     float64 `json:"savings,omitempty"`
	Priority    int     `json:"priority"`
}

// CostController manages cost control.
type CostController interface {
	// CalculateCost calculates the cost of a tool call.
	CalculateCost(toolName string, args json.RawMessage) (float64, error)

	// CheckBudget checks if a tool call is within budget.
	CheckBudget(ctx context.Context, agentID, userID, sessionID, toolName string, cost float64) (*CostCheckResult, error)

	// RecordCost records a cost.
	RecordCost(record *CostRecord) error

	// SetToolCost sets the cost configuration for a tool.
	SetToolCost(cost *ToolCost) error

	// GetToolCost gets the cost configuration for a tool.
	GetToolCost(toolName string) (*ToolCost, bool)

	// AddBudget adds a budget.
	AddBudget(budget *Budget) error

	// RemoveBudget removes a budget.
	RemoveBudget(budgetID string) error

	// GetBudget gets a budget by ID.
	GetBudget(budgetID string) (*Budget, bool)

	// ListBudgets lists all budgets.
	ListBudgets() []*Budget

	// GetUsage gets current usage for a scope.
	GetUsage(scope BudgetScope, scopeID string, period BudgetPeriod) float64

	// GetOptimizations gets cost optimization suggestions.
	GetOptimizations(agentID, userID string) []*CostOptimization

	// GetCostReport generates a cost report.
	GetCostReport(filter *CostReportFilter) (*CostReport, error)
}

// CostReportFilter defines filters for cost reports.
type CostReportFilter struct {
	AgentID   string     `json:"agent_id,omitempty"`
	UserID    string     `json:"user_id,omitempty"`
	ToolName  string     `json:"tool_name,omitempty"`
	StartTime *time.Time `json:"start_time,omitempty"`
	EndTime   *time.Time `json:"end_time,omitempty"`
	GroupBy   string     `json:"group_by,omitempty"` // agent, user, tool, day, hour
}

// CostReport represents a cost report.
type CostReport struct {
	TotalCost    float64                `json:"total_cost"`
	TotalCalls   int64                  `json:"total_calls"`
	AverageCost  float64                `json:"average_cost"`
	ByTool       map[string]float64     `json:"by_tool,omitempty"`
	ByAgent      map[string]float64     `json:"by_agent,omitempty"`
	ByUser       map[string]float64     `json:"by_user,omitempty"`
	ByDay        map[string]float64     `json:"by_day,omitempty"`
	TopTools     []ToolCostSummary      `json:"top_tools,omitempty"`
	GeneratedAt  time.Time              `json:"generated_at"`
}

// ToolCostSummary summarizes costs for a tool.
type ToolCostSummary struct {
	ToolName   string  `json:"tool_name"`
	TotalCost  float64 `json:"total_cost"`
	CallCount  int64   `json:"call_count"`
	AvgCost    float64 `json:"avg_cost"`
}

// DefaultCostController is the default implementation of CostController.
type DefaultCostController struct {
	toolCosts    map[string]*ToolCost
	budgets      map[string]*Budget
	records      []*CostRecord
	usage        map[string]float64 // key -> usage
	alertHandler CostAlertHandler
	logger       *zap.Logger
	mu           sync.RWMutex
}

// CostAlertHandler handles cost alerts.
type CostAlertHandler interface {
	HandleAlert(ctx context.Context, alert *CostAlert) error
}

// NewCostController creates a new cost controller.
func NewCostController(logger *zap.Logger) *DefaultCostController {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &DefaultCostController{
		toolCosts: make(map[string]*ToolCost),
		budgets:   make(map[string]*Budget),
		records:   make([]*CostRecord, 0),
		usage:     make(map[string]float64),
		logger:    logger.With(zap.String("component", "cost_controller")),
	}
}

// SetAlertHandler sets the alert handler.
func (cc *DefaultCostController) SetAlertHandler(handler CostAlertHandler) {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	cc.alertHandler = handler
}

// CalculateCost calculates the cost of a tool call.
func (cc *DefaultCostController) CalculateCost(toolName string, args json.RawMessage) (float64, error) {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	toolCost, ok := cc.toolCosts[toolName]
	if !ok {
		// Default cost if not configured
		return 1.0, nil
	}

	cost := toolCost.BaseCost

	// Calculate additional cost based on arguments
	if toolCost.CostPerUnit > 0 && len(args) > 0 {
		// Simple estimation based on argument size
		units := float64(len(args)) / 100.0
		cost += units * toolCost.CostPerUnit
	}

	return cost, nil
}

// CheckBudget checks if a tool call is within budget.
func (cc *DefaultCostController) CheckBudget(ctx context.Context, agentID, userID, sessionID, toolName string, cost float64) (*CostCheckResult, error) {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	result := &CostCheckResult{
		Allowed: true,
		Cost:    cost,
	}

	// Find applicable budgets
	applicableBudgets := cc.findApplicableBudgets(agentID, userID, sessionID, toolName)

	for _, budget := range applicableBudgets {
		if !budget.Enabled {
			continue
		}

		key := cc.buildUsageKey(budget)
		currentUsage := cc.usage[key]
		newUsage := currentUsage + cost

		result.CurrentUsage = currentUsage
		result.Remaining = budget.Limit - currentUsage

		// Check if over budget
		if newUsage > budget.Limit {
			result.Allowed = false
			result.Budget = budget
			result.Reason = fmt.Sprintf("budget exceeded: %s (%.2f/%.2f)", budget.Name, newUsage, budget.Limit)

			cc.logger.Warn("budget exceeded",
				zap.String("budget_id", budget.ID),
				zap.Float64("current", currentUsage),
				zap.Float64("cost", cost),
				zap.Float64("limit", budget.Limit),
			)

			return result, nil
		}

		// Check alert thresholds
		percentage := (newUsage / budget.Limit) * 100
		for _, threshold := range budget.AlertThresholds {
			prevPercentage := (currentUsage / budget.Limit) * 100
			if percentage >= threshold && prevPercentage < threshold {
				alert := &CostAlert{
					ID:         fmt.Sprintf("alert_%d", time.Now().UnixNano()),
					Timestamp:  time.Now(),
					Level:      cc.getAlertLevel(threshold),
					BudgetID:   budget.ID,
					Message:    fmt.Sprintf("Budget %s reached %.0f%% (%.2f/%.2f)", budget.Name, percentage, newUsage, budget.Limit),
					Current:    newUsage,
					Limit:      budget.Limit,
					Percentage: percentage,
				}
				result.Alert = alert

				if cc.alertHandler != nil {
					go cc.alertHandler.HandleAlert(ctx, alert)
				}

				cc.logger.Warn("budget alert triggered",
					zap.String("budget_id", budget.ID),
					zap.Float64("threshold", threshold),
					zap.Float64("percentage", percentage),
				)
			}
		}
	}

	return result, nil
}

// findApplicableBudgets finds budgets applicable to the context.
func (cc *DefaultCostController) findApplicableBudgets(agentID, userID, sessionID, toolName string) []*Budget {
	var budgets []*Budget

	for _, budget := range cc.budgets {
		switch budget.Scope {
		case BudgetScopeGlobal:
			budgets = append(budgets, budget)
		case BudgetScopeAgent:
			if budget.ScopeID == agentID || budget.ScopeID == "" {
				budgets = append(budgets, budget)
			}
		case BudgetScopeUser:
			if budget.ScopeID == userID || budget.ScopeID == "" {
				budgets = append(budgets, budget)
			}
		case BudgetScopeSession:
			if budget.ScopeID == sessionID || budget.ScopeID == "" {
				budgets = append(budgets, budget)
			}
		case BudgetScopeTool:
			if budget.ScopeID == toolName || budget.ScopeID == "" {
				budgets = append(budgets, budget)
			}
		}
	}

	return budgets
}

// buildUsageKey builds a unique key for usage tracking.
func (cc *DefaultCostController) buildUsageKey(budget *Budget) string {
	periodKey := cc.getPeriodKey(budget.Period)
	return fmt.Sprintf("%s:%s:%s:%s", budget.Scope, budget.ScopeID, budget.ID, periodKey)
}

// getPeriodKey gets the period key for the current time.
func (cc *DefaultCostController) getPeriodKey(period BudgetPeriod) string {
	now := time.Now()
	switch period {
	case BudgetPeriodHourly:
		return now.Format("2006-01-02-15")
	case BudgetPeriodDaily:
		return now.Format("2006-01-02")
	case BudgetPeriodWeekly:
		year, week := now.ISOWeek()
		return fmt.Sprintf("%d-W%02d", year, week)
	case BudgetPeriodMonthly:
		return now.Format("2006-01")
	case BudgetPeriodTotal:
		return "total"
	}
	return "unknown"
}

// getAlertLevel determines the alert level based on threshold.
func (cc *DefaultCostController) getAlertLevel(threshold float64) CostAlertLevel {
	if threshold >= 100 {
		return CostAlertLevelCritical
	} else if threshold >= 80 {
		return CostAlertLevelWarning
	}
	return CostAlertLevelInfo
}

// RecordCost records a cost.
func (cc *DefaultCostController) RecordCost(record *CostRecord) error {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	if record.ID == "" {
		record.ID = fmt.Sprintf("cost_%d", time.Now().UnixNano())
	}
	if record.Timestamp.IsZero() {
		record.Timestamp = time.Now()
	}

	cc.records = append(cc.records, record)

	// Update usage for all applicable budgets
	applicableBudgets := cc.findApplicableBudgets(record.AgentID, record.UserID, record.SessionID, record.ToolName)
	for _, budget := range applicableBudgets {
		key := cc.buildUsageKey(budget)
		cc.usage[key] += record.Cost
	}

	cc.logger.Debug("cost recorded",
		zap.String("record_id", record.ID),
		zap.String("tool", record.ToolName),
		zap.Float64("cost", record.Cost),
	)

	return nil
}

// SetToolCost sets the cost configuration for a tool.
func (cc *DefaultCostController) SetToolCost(cost *ToolCost) error {
	if cost.ToolName == "" {
		return fmt.Errorf("tool name is required")
	}

	cc.mu.Lock()
	defer cc.mu.Unlock()

	cc.toolCosts[cost.ToolName] = cost

	cc.logger.Info("tool cost set",
		zap.String("tool", cost.ToolName),
		zap.Float64("base_cost", cost.BaseCost),
	)

	return nil
}

// GetToolCost gets the cost configuration for a tool.
func (cc *DefaultCostController) GetToolCost(toolName string) (*ToolCost, bool) {
	cc.mu.RLock()
	defer cc.mu.RUnlock()
	cost, ok := cc.toolCosts[toolName]
	return cost, ok
}

// AddBudget adds a budget.
func (cc *DefaultCostController) AddBudget(budget *Budget) error {
	if budget.ID == "" {
		return fmt.Errorf("budget ID is required")
	}

	cc.mu.Lock()
	defer cc.mu.Unlock()

	budget.CreatedAt = time.Now()
	budget.UpdatedAt = time.Now()
	cc.budgets[budget.ID] = budget

	cc.logger.Info("budget added",
		zap.String("budget_id", budget.ID),
		zap.String("name", budget.Name),
		zap.Float64("limit", budget.Limit),
	)

	return nil
}

// RemoveBudget removes a budget.
func (cc *DefaultCostController) RemoveBudget(budgetID string) error {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	if _, ok := cc.budgets[budgetID]; !ok {
		return fmt.Errorf("budget not found: %s", budgetID)
	}

	delete(cc.budgets, budgetID)
	cc.logger.Info("budget removed", zap.String("budget_id", budgetID))
	return nil
}

// GetBudget gets a budget by ID.
func (cc *DefaultCostController) GetBudget(budgetID string) (*Budget, bool) {
	cc.mu.RLock()
	defer cc.mu.RUnlock()
	budget, ok := cc.budgets[budgetID]
	return budget, ok
}

// ListBudgets lists all budgets.
func (cc *DefaultCostController) ListBudgets() []*Budget {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	budgets := make([]*Budget, 0, len(cc.budgets))
	for _, budget := range cc.budgets {
		budgets = append(budgets, budget)
	}
	return budgets
}

// GetUsage gets current usage for a scope.
func (cc *DefaultCostController) GetUsage(scope BudgetScope, scopeID string, period BudgetPeriod) float64 {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	periodKey := cc.getPeriodKey(period)
	key := fmt.Sprintf("%s:%s::%s", scope, scopeID, periodKey)

	var total float64
	for k, v := range cc.usage {
		if len(k) >= len(key) && k[:len(key)] == key {
			total += v
		}
	}
	return total
}

// GetOptimizations gets cost optimization suggestions.
func (cc *DefaultCostController) GetOptimizations(agentID, userID string) []*CostOptimization {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	var optimizations []*CostOptimization

	// Analyze usage patterns
	toolUsage := make(map[string]int)
	toolCosts := make(map[string]float64)

	for _, record := range cc.records {
		if (agentID == "" || record.AgentID == agentID) &&
			(userID == "" || record.UserID == userID) {
			toolUsage[record.ToolName]++
			toolCosts[record.ToolName] += record.Cost
		}
	}

	// Find high-cost tools
	for tool, cost := range toolCosts {
		if cost > 100 { // Threshold for high cost
			optimizations = append(optimizations, &CostOptimization{
				Type:        "high_cost_tool",
				Description: fmt.Sprintf("Tool '%s' has high total cost (%.2f). Consider caching results or reducing usage.", tool, cost),
				Savings:     cost * 0.2, // Estimated 20% savings
				Priority:    1,
			})
		}
	}

	// Find frequently used tools
	for tool, count := range toolUsage {
		if count > 100 { // Threshold for high frequency
			optimizations = append(optimizations, &CostOptimization{
				Type:        "high_frequency_tool",
				Description: fmt.Sprintf("Tool '%s' is called frequently (%d times). Consider batching requests.", tool, count),
				Savings:     float64(count) * 0.1, // Estimated savings
				Priority:    2,
			})
		}
	}

	// Check for budget utilization
	for _, budget := range cc.budgets {
		key := cc.buildUsageKey(budget)
		usage := cc.usage[key]
		utilization := (usage / budget.Limit) * 100

		if utilization < 20 {
			optimizations = append(optimizations, &CostOptimization{
				Type:        "underutilized_budget",
				Description: fmt.Sprintf("Budget '%s' is underutilized (%.1f%%). Consider reducing the limit.", budget.Name, utilization),
				Priority:    3,
			})
		}
	}

	return optimizations
}

// GetCostReport generates a cost report.
func (cc *DefaultCostController) GetCostReport(filter *CostReportFilter) (*CostReport, error) {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	report := &CostReport{
		ByTool:      make(map[string]float64),
		ByAgent:     make(map[string]float64),
		ByUser:      make(map[string]float64),
		ByDay:       make(map[string]float64),
		GeneratedAt: time.Now(),
	}

	toolCalls := make(map[string]int64)

	for _, record := range cc.records {
		// Apply filters
		if filter.AgentID != "" && record.AgentID != filter.AgentID {
			continue
		}
		if filter.UserID != "" && record.UserID != filter.UserID {
			continue
		}
		if filter.ToolName != "" && record.ToolName != filter.ToolName {
			continue
		}
		if filter.StartTime != nil && record.Timestamp.Before(*filter.StartTime) {
			continue
		}
		if filter.EndTime != nil && record.Timestamp.After(*filter.EndTime) {
			continue
		}

		// Aggregate data
		report.TotalCost += record.Cost
		report.TotalCalls++

		report.ByTool[record.ToolName] += record.Cost
		toolCalls[record.ToolName]++

		if record.AgentID != "" {
			report.ByAgent[record.AgentID] += record.Cost
		}
		if record.UserID != "" {
			report.ByUser[record.UserID] += record.Cost
		}

		day := record.Timestamp.Format("2006-01-02")
		report.ByDay[day] += record.Cost
	}

	// Calculate average
	if report.TotalCalls > 0 {
		report.AverageCost = report.TotalCost / float64(report.TotalCalls)
	}

	// Build top tools list
	for tool, cost := range report.ByTool {
		calls := toolCalls[tool]
		avgCost := float64(0)
		if calls > 0 {
			avgCost = cost / float64(calls)
		}
		report.TopTools = append(report.TopTools, ToolCostSummary{
			ToolName:  tool,
			TotalCost: cost,
			CallCount: calls,
			AvgCost:   avgCost,
		})
	}

	// Sort top tools by total cost (descending)
	for i := 0; i < len(report.TopTools)-1; i++ {
		for j := i + 1; j < len(report.TopTools); j++ {
			if report.TopTools[j].TotalCost > report.TopTools[i].TotalCost {
				report.TopTools[i], report.TopTools[j] = report.TopTools[j], report.TopTools[i]
			}
		}
	}

	// Limit to top 10
	if len(report.TopTools) > 10 {
		report.TopTools = report.TopTools[:10]
	}

	return report, nil
}

// ====== Cost Control Middleware ======

// CostControlMiddleware creates a middleware that enforces cost control.
func CostControlMiddleware(cc CostController, auditLogger AuditLogger) func(ToolFunc) ToolFunc {
	return func(next ToolFunc) ToolFunc {
		return func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
			// Extract context information
			permCtx, _ := GetPermissionContext(ctx)

			var agentID, userID, sessionID, toolName string
			if permCtx != nil {
				agentID = permCtx.AgentID
				userID = permCtx.UserID
				sessionID = permCtx.SessionID
				toolName = permCtx.ToolName
			}

			// Calculate cost
			cost, err := cc.CalculateCost(toolName, args)
			if err != nil {
				return nil, fmt.Errorf("failed to calculate cost: %w", err)
			}

			// Check budget
			result, err := cc.CheckBudget(ctx, agentID, userID, sessionID, toolName, cost)
			if err != nil {
				return nil, fmt.Errorf("budget check failed: %w", err)
			}

			if !result.Allowed {
				// Log cost alert
				if auditLogger != nil {
					LogCostAlert(auditLogger, agentID, userID, cost, "budget_exceeded")
				}
				return nil, fmt.Errorf("budget exceeded: %s", result.Reason)
			}

			// Execute the tool
			response, execErr := next(ctx, args)

			// Record the cost
			record := &CostRecord{
				AgentID:   agentID,
				UserID:    userID,
				SessionID: sessionID,
				ToolName:  toolName,
				Cost:      cost,
				Unit:      CostUnitCredits,
			}
			cc.RecordCost(record)

			return response, execErr
		}
	}
}

// ====== Convenience Functions ======

// CreateGlobalBudget creates a global budget.
func CreateGlobalBudget(id, name string, limit float64, period BudgetPeriod) *Budget {
	return &Budget{
		ID:              id,
		Name:            name,
		Scope:           BudgetScopeGlobal,
		Limit:           limit,
		Unit:            CostUnitCredits,
		Period:          period,
		AlertThresholds: []float64{50, 80, 100},
		Enabled:         true,
	}
}

// CreateAgentBudget creates an agent-specific budget.
func CreateAgentBudget(id, name, agentID string, limit float64, period BudgetPeriod) *Budget {
	return &Budget{
		ID:              id,
		Name:            name,
		Scope:           BudgetScopeAgent,
		ScopeID:         agentID,
		Limit:           limit,
		Unit:            CostUnitCredits,
		Period:          period,
		AlertThresholds: []float64{50, 80, 100},
		Enabled:         true,
	}
}

// CreateUserBudget creates a user-specific budget.
func CreateUserBudget(id, name, userID string, limit float64, period BudgetPeriod) *Budget {
	return &Budget{
		ID:              id,
		Name:            name,
		Scope:           BudgetScopeUser,
		ScopeID:         userID,
		Limit:           limit,
		Unit:            CostUnitCredits,
		Period:          period,
		AlertThresholds: []float64{50, 80, 100},
		Enabled:         true,
	}
}

// CreateToolCost creates a tool cost configuration.
func CreateToolCost(toolName string, baseCost, costPerUnit float64) *ToolCost {
	return &ToolCost{
		ToolName:    toolName,
		BaseCost:    baseCost,
		CostPerUnit: costPerUnit,
		Unit:        CostUnitCredits,
	}
}
