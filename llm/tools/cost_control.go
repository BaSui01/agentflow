// 包工具为企业AI代理框架中的工具执行提供成本控制.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// CostAlert Level代表着成本警报的严重性.
type CostAlertLevel string

const (
	CostAlertLevelInfo     CostAlertLevel = "info"
	CostAlertLevelWarning  CostAlertLevel = "warning"
	CostAlertLevelCritical CostAlertLevel = "critical"
)

// 成本单位代表成本计量单位.
type CostUnit string

const (
	CostUnitCredits CostUnit = "credits"
	CostUnitDollars CostUnit = "dollars"
	CostUnitTokens  CostUnit = "tokens"
)

// ToolCost定义了工具的成本配置.
type ToolCost struct {
	ToolName    string   `json:"tool_name"`
	BaseCost    float64  `json:"base_cost"`              // Base cost per call
	CostPerUnit float64  `json:"cost_per_unit"`          // Cost per unit (e.g., per token)
	Unit        CostUnit `json:"unit"`                   // Cost unit
	Description string   `json:"description,omitempty"`
}

// 预算界定了预算配置。
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

// 预算范围界定了预算的范围。
type BudgetScope string

const (
	BudgetScopeGlobal  BudgetScope = "global"
	BudgetScopeAgent   BudgetScope = "agent"
	BudgetScopeUser    BudgetScope = "user"
	BudgetScopeSession BudgetScope = "session"
	BudgetScopeTool    BudgetScope = "tool"
)

// 预算执行情况界定了预算的期限。
type BudgetPeriod string

const (
	BudgetPeriodHourly  BudgetPeriod = "hourly"
	BudgetPeriodDaily   BudgetPeriod = "daily"
	BudgetPeriodWeekly  BudgetPeriod = "weekly"
	BudgetPeriodMonthly BudgetPeriod = "monthly"
	BudgetPeriodTotal   BudgetPeriod = "total" // No reset
)

// 成本记录是单一成本记录。
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

// CostAlert代表成本警报.
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

// CostCheckResult包含成本检查的结果.
type CostCheckResult struct {
	Allowed     bool           `json:"allowed"`
	Cost        float64        `json:"cost"`
	Budget      *Budget        `json:"budget,omitempty"`
	CurrentUsage float64       `json:"current_usage"`
	Remaining   float64        `json:"remaining"`
	Alert       *CostAlert     `json:"alert,omitempty"`
	Reason      string         `json:"reason,omitempty"`
}

// 成本优化是一个成本优化建议。
type CostOptimization struct {
	Type        string  `json:"type"`
	Description string  `json:"description"`
	Savings     float64 `json:"savings,omitempty"`
	Priority    int     `json:"priority"`
}

// 成本控制员管理成本控制。
type CostController interface {
	// 计算成本计算工具调用的成本 。
	CalculateCost(toolName string, args json.RawMessage) (float64, error)

	// 检查预算是否在预算范围内 。
	CheckBudget(ctx context.Context, agentID, userID, sessionID, toolName string, cost float64) (*CostCheckResult, error)

	// 记录成本记录成本。
	RecordCost(record *CostRecord) error

	// SetToolCost 设置工具的成本配置 。
	SetToolCost(cost *ToolCost) error

	// GetTooLCost 获得工具的成本配置.
	GetToolCost(toolName string) (*ToolCost, bool)

	// 增加预算。
	AddBudget(budget *Budget) error

	// 删除预算会删除预算 。
	RemoveBudget(budgetID string) error

	// GetBudget通过身份证获得预算.
	GetBudget(budgetID string) (*Budget, bool)

	// 预算列表列出所有预算.
	ListBudgets() []*Budget

	// GetUsage 获得当前用于一个瞄准镜的用法 。
	GetUsage(scope BudgetScope, scopeID string, period BudgetPeriod) float64

	// Get Optimizations获得成本优化建议.
	GetOptimizations(agentID, userID string) []*CostOptimization

	// GetCostReport 生成成本报告 。
	GetCostReport(filter *CostReportFilter) (*CostReport, error)
}

// CostReportFilter为成本报告定义了过滤器.
type CostReportFilter struct {
	AgentID   string     `json:"agent_id,omitempty"`
	UserID    string     `json:"user_id,omitempty"`
	ToolName  string     `json:"tool_name,omitempty"`
	StartTime *time.Time `json:"start_time,omitempty"`
	EndTime   *time.Time `json:"end_time,omitempty"`
	GroupBy   string     `json:"group_by,omitempty"` // agent, user, tool, day, hour
}

// 成本报告是一种成本报告。
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

// 工具成本汇总总结 工具的成本。
type ToolCostSummary struct {
	ToolName   string  `json:"tool_name"`
	TotalCost  float64 `json:"total_cost"`
	CallCount  int64   `json:"call_count"`
	AvgCost    float64 `json:"avg_cost"`
}

// 默认成本控制器是成本控制器的默认执行 。
type DefaultCostController struct {
	toolCosts       map[string]*ToolCost
	budgets         map[string]*Budget
	records         []*CostRecord
	usage           map[string]float64 // key -> usage
	usageResetTimes map[string]time.Time
	tokenCounter    TokenCounter
	alertHandler    CostAlertHandler
	logger          *zap.Logger
	mu              sync.RWMutex
}

// CostAlertHandler处理成本警报.
type CostAlertHandler interface {
	HandleAlert(ctx context.Context, alert *CostAlert) error
}

// TokenCounter 可选的 token 计数器接口
type TokenCounter interface {
	CountTokens(text string) (int, error)
}

// 新建成本控制器 。
func NewCostController(logger *zap.Logger) *DefaultCostController {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &DefaultCostController{
		toolCosts:       make(map[string]*ToolCost),
		budgets:         make(map[string]*Budget),
		records:         make([]*CostRecord, 0),
		usage:           make(map[string]float64),
		usageResetTimes: make(map[string]time.Time),
		logger:          logger.With(zap.String("component", "cost_controller")),
	}
}

// SetAlertHandler)设置了警报处理器.
func (cc *DefaultCostController) SetAlertHandler(handler CostAlertHandler) {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	cc.alertHandler = handler
}

// 计算成本计算工具调用的成本 。
func (cc *DefaultCostController) CalculateCost(toolName string, args json.RawMessage) (float64, error) {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	toolCost, ok := cc.toolCosts[toolName]
	if !ok {
		return 1.0, nil // 默认成本
	}

	cost := toolCost.BaseCost

	if toolCost.CostPerUnit > 0 && len(args) > 0 {
		switch toolCost.Unit {
		case CostUnitTokens:
			// 如果有 token 计数器，精确计算
			if cc.tokenCounter != nil {
				tokens, err := cc.tokenCounter.CountTokens(string(args))
				if err == nil {
					cost += float64(tokens) * toolCost.CostPerUnit
					break
				}
			}
			// fallback: 按字符数估算（1 token ≈ 4 字符）
			cost += float64(len(args)) / 4.0 * toolCost.CostPerUnit
		case CostUnitCredits, CostUnitDollars:
			cost += float64(len(args)) / 100.0 * toolCost.CostPerUnit
		}
	}

	return cost, nil
}

// resetUsageIfNeeded 检查并重置过期的预算周期
func (cc *DefaultCostController) resetUsageIfNeeded(budget *Budget) {
	key := cc.buildUsageKey(budget)
	resetKey := key + ":reset_at"

	lastReset, exists := cc.usageResetTimes[resetKey]
	if !exists {
		cc.usageResetTimes[resetKey] = time.Now()
		return
	}

	var shouldReset bool
	switch budget.Period {
	case BudgetPeriodHourly:
		shouldReset = time.Since(lastReset) >= time.Hour
	case BudgetPeriodDaily:
		shouldReset = time.Since(lastReset) >= 24*time.Hour
	case BudgetPeriodWeekly:
		shouldReset = time.Since(lastReset) >= 7*24*time.Hour
	case BudgetPeriodMonthly:
		shouldReset = time.Since(lastReset) >= 30*24*time.Hour
	case BudgetPeriodTotal:
		shouldReset = false
	}

	if shouldReset {
		cc.usage[key] = 0
		cc.usageResetTimes[resetKey] = time.Now()
		cc.logger.Info("budget usage reset", zap.String("budget_id", budget.ID), zap.String("period", string(budget.Period)))
	}
}

// 检查预算是否在预算范围内 。
func (cc *DefaultCostController) CheckBudget(ctx context.Context, agentID, userID, sessionID, toolName string, cost float64) (*CostCheckResult, error) {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	result := &CostCheckResult{
		Allowed: true,
		Cost:    cost,
	}

	// 寻找适用预算
	applicableBudgets := cc.findApplicableBudgets(agentID, userID, sessionID, toolName)

	for _, budget := range applicableBudgets {
		if !budget.Enabled {
			continue
		}

		// 惰性检查并重置过期的预算周期
		cc.resetUsageIfNeeded(budget)

		key := cc.buildUsageKey(budget)
		currentUsage := cc.usage[key]
		newUsage := currentUsage + cost

		result.CurrentUsage = currentUsage
		result.Remaining = budget.Limit - currentUsage

		// 检查是否超出预算
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

		// 检查警戒阈值
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

// 可应用的预算为适用于上下文的预算。
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

// 构建 UsageKey 为使用跟踪构建了独特的密钥.
func (cc *DefaultCostController) buildUsageKey(budget *Budget) string {
	periodKey := cc.getPeriodKey(budget.Period)
	return fmt.Sprintf("%s:%s:%s:%s", budget.Scope, budget.ScopeID, budget.ID, periodKey)
}

// 获取 PeriodKey 获得当前时间的周期密钥 。
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

// getAlert Level根据阈值确定警戒级别.
func (cc *DefaultCostController) getAlertLevel(threshold float64) CostAlertLevel {
	if threshold >= 100 {
		return CostAlertLevelCritical
	} else if threshold >= 80 {
		return CostAlertLevelWarning
	}
	return CostAlertLevelInfo
}

// 记录成本记录成本。
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

	// 所有适用预算的最新使用情况
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

// SetToolCost 设置工具的成本配置 。
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

// GetTooLCost 获得工具的成本配置.
func (cc *DefaultCostController) GetToolCost(toolName string) (*ToolCost, bool) {
	cc.mu.RLock()
	defer cc.mu.RUnlock()
	cost, ok := cc.toolCosts[toolName]
	return cost, ok
}

// 增加预算。
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

// 删除预算会删除预算 。
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

// GetBudget通过身份证获得预算.
func (cc *DefaultCostController) GetBudget(budgetID string) (*Budget, bool) {
	cc.mu.RLock()
	defer cc.mu.RUnlock()
	budget, ok := cc.budgets[budgetID]
	return budget, ok
}

// 预算列表列出所有预算.
func (cc *DefaultCostController) ListBudgets() []*Budget {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	budgets := make([]*Budget, 0, len(cc.budgets))
	for _, budget := range cc.budgets {
		budgets = append(budgets, budget)
	}
	return budgets
}

// GetUsage 获得当前用于一个瞄准镜的用法 。
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

// Get Optimizations获得成本优化建议.
func (cc *DefaultCostController) GetOptimizations(agentID, userID string) []*CostOptimization {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	var opts []*CostOptimization

	// 分析高频工具
	toolCalls := make(map[string]int)
	toolCosts := make(map[string]float64)
	for _, rec := range cc.records {
		if agentID != "" && rec.AgentID != agentID {
			continue
		}
		if userID != "" && rec.UserID != userID {
			continue
		}
		toolCalls[rec.ToolName]++
		toolCosts[rec.ToolName] += rec.Cost
	}

	for tool, count := range toolCalls {
		cost := toolCosts[tool]
		avgCost := cost / float64(count)
		// 高频高成本工具建议缓存
		if count > 100 && avgCost > 5.0 {
			opts = append(opts, &CostOptimization{
				Type:        "cache",
				Description: fmt.Sprintf("Tool '%s' called %d times with avg cost %.2f. Consider caching results.", tool, count, avgCost),
				Savings:     cost * 0.3,
				Priority:    1,
			})
		}
		// 低使用率高成本工具建议替换
		if count < 10 && cost > 100 {
			opts = append(opts, &CostOptimization{
				Type:        "replace",
				Description: fmt.Sprintf("Tool '%s' has high total cost (%.2f) with low usage (%d calls). Consider cheaper alternative.", tool, cost, count),
				Savings:     cost * 0.5,
				Priority:    2,
			})
		}
	}

	return opts
}

// GetCostReport 生成成本报告 。
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

	for _, rec := range cc.records {
		// 应用过滤器
		if filter != nil {
			if filter.AgentID != "" && rec.AgentID != filter.AgentID {
				continue
			}
			if filter.UserID != "" && rec.UserID != filter.UserID {
				continue
			}
			if filter.ToolName != "" && rec.ToolName != filter.ToolName {
				continue
			}
			if filter.StartTime != nil && rec.Timestamp.Before(*filter.StartTime) {
				continue
			}
			if filter.EndTime != nil && rec.Timestamp.After(*filter.EndTime) {
				continue
			}
		}

		report.TotalCost += rec.Cost
		report.TotalCalls++
		report.ByTool[rec.ToolName] += rec.Cost
		report.ByAgent[rec.AgentID] += rec.Cost
		report.ByUser[rec.UserID] += rec.Cost
		day := rec.Timestamp.Format("2006-01-02")
		report.ByDay[day] += rec.Cost
	}

	if report.TotalCalls > 0 {
		report.AverageCost = report.TotalCost / float64(report.TotalCalls)
	}

	return report, nil
}

// 成本控制中间软件

// Cost ControlMiddleware 创建了一个执行成本控制的中间软件.
func CostControlMiddleware(cc CostController, auditLogger AuditLogger) func(ToolFunc) ToolFunc {
	return func(next ToolFunc) ToolFunc {
		return func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
			// 摘录上下文信息
			permCtx, _ := GetPermissionContext(ctx)

			var agentID, userID, sessionID, toolName string
			if permCtx != nil {
				agentID = permCtx.AgentID
				userID = permCtx.UserID
				sessionID = permCtx.SessionID
				toolName = permCtx.ToolName
			}

			// 计算成本
			cost, err := cc.CalculateCost(toolName, args)
			if err != nil {
				return nil, fmt.Errorf("failed to calculate cost: %w", err)
			}

			// 检查预算
			result, err := cc.CheckBudget(ctx, agentID, userID, sessionID, toolName, cost)
			if err != nil {
				return nil, fmt.Errorf("budget check failed: %w", err)
			}

			if !result.Allowed {
				// 日志费用提示
				if auditLogger != nil {
					LogCostAlert(auditLogger, agentID, userID, cost, "budget_exceeded")
				}
				return nil, fmt.Errorf("budget exceeded: %s", result.Reason)
			}

			// 执行工具
			response, execErr := next(ctx, args)

			// 记录成本
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

// • 方便功能

// 创建全球预算创建全球预算。
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

// Create AgentBudget 创建针对特定代理的预算.
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

// Create UserBudget 创建针对用户的预算.
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

// Create ToolCost 创建工具成本配置 。
func CreateToolCost(toolName string, baseCost, costPerUnit float64) *ToolCost {
	return &ToolCost{
		ToolName:    toolName,
		BaseCost:    baseCost,
		CostPerUnit: costPerUnit,
		Unit:        CostUnitCredits,
	}
}
