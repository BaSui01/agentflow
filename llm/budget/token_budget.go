// 一揽子预算提供象征性的预算管理和成本控制。
package budget

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// 预算Config 配置符号预算管理 。
type BudgetConfig struct {
	MaxTokensPerRequest int           `json:"max_tokens_per_request"`
	MaxTokensPerMinute  int           `json:"max_tokens_per_minute"`
	MaxTokensPerHour    int           `json:"max_tokens_per_hour"`
	MaxTokensPerDay     int           `json:"max_tokens_per_day"`
	MaxCostPerRequest   float64       `json:"max_cost_per_request"`
	MaxCostPerDay       float64       `json:"max_cost_per_day"`
	AlertThreshold      float64       `json:"alert_threshold"` // 0.0-1.0, alert when usage exceeds this
	AutoThrottle        bool          `json:"auto_throttle"`
	ThrottleDelay       time.Duration `json:"throttle_delay"`
}

// 默认预览返回合理的默认值 。
func DefaultBudgetConfig() BudgetConfig {
	return BudgetConfig{
		MaxTokensPerRequest: 100000,
		MaxTokensPerMinute:  500000,
		MaxTokensPerHour:    5000000,
		MaxTokensPerDay:     50000000,
		MaxCostPerRequest:   10.0,
		MaxCostPerDay:       1000.0,
		AlertThreshold:      0.8,
		AutoThrottle:        true,
		ThrottleDelay:       time.Second,
	}
}

// 用法记录代表单一使用记录.
type UsageRecord struct {
	Timestamp time.Time `json:"timestamp"`
	Tokens    int       `json:"tokens"`
	Cost      float64   `json:"cost"`
	Model     string    `json:"model"`
	RequestID string    `json:"request_id"`
	UserID    string    `json:"user_id,omitempty"`
	AgentID   string    `json:"agent_id,omitempty"`
}

// 预算状况是目前的预算状况。
type BudgetStatus struct {
	TokensUsedMinute  int64      `json:"tokens_used_minute"`
	TokensUsedHour    int64      `json:"tokens_used_hour"`
	TokensUsedDay     int64      `json:"tokens_used_day"`
	CostUsedDay       float64    `json:"cost_used_day"`
	MinuteUtilization float64    `json:"minute_utilization"`
	HourUtilization   float64    `json:"hour_utilization"`
	DayUtilization    float64    `json:"day_utilization"`
	CostUtilization   float64    `json:"cost_utilization"`
	IsThrottled       bool       `json:"is_throttled"`
	ThrottleUntil     *time.Time `json:"throttle_until,omitempty"`
}

// 提醒Type代表预算提醒的类型.
type AlertType string

const (
	AlertTokenMinute AlertType = "token_minute_threshold"
	AlertTokenHour   AlertType = "token_hour_threshold"
	AlertTokenDay    AlertType = "token_day_threshold"
	AlertCostDay     AlertType = "cost_day_threshold"
	AlertLimitHit    AlertType = "limit_hit"
)

// 警报代表预算警报。
type Alert struct {
	Type      AlertType `json:"type"`
	Message   string    `json:"message"`
	Threshold float64   `json:"threshold"`
	Current   float64   `json:"current"`
	Timestamp time.Time `json:"timestamp"`
}

// 警报汉德勒处理预算警报.
type AlertHandler func(alert Alert)

// TokenBudgetManager管理符名预算并强制执行限制.
type TokenBudgetManager struct {
	config        BudgetConfig
	logger        *zap.Logger
	alertHandlers []AlertHandler

	// 用于线程安全更新的原子计数器
	tokensMinute int64
	tokensHour   int64
	tokensDay    int64
	costDay      int64 // stored as cost * 1000000 for atomic ops

	// 时间窗口
	minuteStart time.Time
	hourStart   time.Time
	dayStart    time.Time

	// 调弦
	throttleUntil time.Time
	mu            sync.RWMutex

	// 警报跟踪
	alertedMinute bool
	alertedHour   bool
	alertedDay    bool
	alertedCost   bool
}

// NewTokenBudgetManager 创建了新的代币预算管理器.
func NewTokenBudgetManager(config BudgetConfig, logger *zap.Logger) *TokenBudgetManager {
	now := time.Now()
	return &TokenBudgetManager{
		config:      config,
		logger:      logger,
		minuteStart: now,
		hourStart:   now,
		dayStart:    now.Truncate(24 * time.Hour),
	}
}

// OnAlert登记了一个警报处理器。
func (m *TokenBudgetManager) OnAlert(handler AlertHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.alertHandlers = append(m.alertHandlers, handler)
}

// 检查预算是否在预算范围内 。
func (m *TokenBudgetManager) CheckBudget(ctx context.Context, estimatedTokens int, estimatedCost float64) error {
	m.resetWindowsIfNeeded()

	// 检查节奏
	m.mu.RLock()
	if time.Now().Before(m.throttleUntil) {
		m.mu.RUnlock()
		return fmt.Errorf("throttled until %s", m.throttleUntil.Format(time.RFC3339))
	}
	m.mu.RUnlock()

	// 检查每个请求的限制
	if estimatedTokens > m.config.MaxTokensPerRequest {
		return fmt.Errorf("estimated tokens %d exceeds per-request limit %d",
			estimatedTokens, m.config.MaxTokensPerRequest)
	}
	if estimatedCost > m.config.MaxCostPerRequest {
		return fmt.Errorf("estimated cost %.2f exceeds per-request limit %.2f",
			estimatedCost, m.config.MaxCostPerRequest)
	}

	// 检查窗口限制
	currentMinute := atomic.LoadInt64(&m.tokensMinute)
	if int(currentMinute)+estimatedTokens > m.config.MaxTokensPerMinute {
		m.applyThrottle()
		return fmt.Errorf("would exceed minute token limit")
	}

	currentHour := atomic.LoadInt64(&m.tokensHour)
	if int(currentHour)+estimatedTokens > m.config.MaxTokensPerHour {
		return fmt.Errorf("would exceed hour token limit")
	}

	currentDay := atomic.LoadInt64(&m.tokensDay)
	if int(currentDay)+estimatedTokens > m.config.MaxTokensPerDay {
		return fmt.Errorf("would exceed day token limit")
	}

	currentCost := float64(atomic.LoadInt64(&m.costDay)) / 1000000
	if currentCost+estimatedCost > m.config.MaxCostPerDay {
		return fmt.Errorf("would exceed daily cost limit")
	}

	return nil
}

// 记录Usage记录符和成本使用.
func (m *TokenBudgetManager) RecordUsage(record UsageRecord) {
	m.resetWindowsIfNeeded()

	// 更新计数器
	atomic.AddInt64(&m.tokensMinute, int64(record.Tokens))
	atomic.AddInt64(&m.tokensHour, int64(record.Tokens))
	atomic.AddInt64(&m.tokensDay, int64(record.Tokens))
	atomic.AddInt64(&m.costDay, int64(record.Cost*1000000))

	// 检查提示
	m.checkAlerts()

	m.logger.Debug("usage recorded",
		zap.Int("tokens", record.Tokens),
		zap.Float64("cost", record.Cost),
		zap.String("model", record.Model))
}

// Get Status 返回当前预算状况 。
func (m *TokenBudgetManager) GetStatus() BudgetStatus {
	m.resetWindowsIfNeeded()

	tokensMinute := atomic.LoadInt64(&m.tokensMinute)
	tokensHour := atomic.LoadInt64(&m.tokensHour)
	tokensDay := atomic.LoadInt64(&m.tokensDay)
	costDay := float64(atomic.LoadInt64(&m.costDay)) / 1000000

	status := BudgetStatus{
		TokensUsedMinute:  tokensMinute,
		TokensUsedHour:    tokensHour,
		TokensUsedDay:     tokensDay,
		CostUsedDay:       costDay,
		MinuteUtilization: float64(tokensMinute) / float64(m.config.MaxTokensPerMinute),
		HourUtilization:   float64(tokensHour) / float64(m.config.MaxTokensPerHour),
		DayUtilization:    float64(tokensDay) / float64(m.config.MaxTokensPerDay),
		CostUtilization:   costDay / m.config.MaxCostPerDay,
	}

	m.mu.RLock()
	if time.Now().Before(m.throttleUntil) {
		status.IsThrottled = true
		status.ThrottleUntil = &m.throttleUntil
	}
	m.mu.RUnlock()

	return status
}

func (m *TokenBudgetManager) resetWindowsIfNeeded() {
	now := time.Now()

	m.mu.Lock()
	defer m.mu.Unlock()

	// 重置分钟窗口
	if now.Sub(m.minuteStart) >= time.Minute {
		atomic.StoreInt64(&m.tokensMinute, 0)
		m.minuteStart = now
		m.alertedMinute = false
	}

	// 重置小时窗口
	if now.Sub(m.hourStart) >= time.Hour {
		atomic.StoreInt64(&m.tokensHour, 0)
		m.hourStart = now
		m.alertedHour = false
	}

	// 重设日窗口
	dayStart := now.Truncate(24 * time.Hour)
	if dayStart.After(m.dayStart) {
		atomic.StoreInt64(&m.tokensDay, 0)
		atomic.StoreInt64(&m.costDay, 0)
		m.dayStart = dayStart
		m.alertedDay = false
		m.alertedCost = false
	}
}

func (m *TokenBudgetManager) applyThrottle() {
	if !m.config.AutoThrottle {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.throttleUntil = time.Now().Add(m.config.ThrottleDelay)
	m.logger.Warn("throttling applied", zap.Time("until", m.throttleUntil))
}

func (m *TokenBudgetManager) checkAlerts() {
	m.mu.Lock()
	defer m.mu.Unlock()

	threshold := m.config.AlertThreshold

	// 检查分钟阈值
	minuteUtil := float64(atomic.LoadInt64(&m.tokensMinute)) / float64(m.config.MaxTokensPerMinute)
	if minuteUtil >= threshold && !m.alertedMinute {
		m.alertedMinute = true
		m.fireAlert(Alert{
			Type:      AlertTokenMinute,
			Message:   "Minute token usage threshold exceeded",
			Threshold: threshold,
			Current:   minuteUtil,
			Timestamp: time.Now(),
		})
	}

	// 检查小时阈值
	hourUtil := float64(atomic.LoadInt64(&m.tokensHour)) / float64(m.config.MaxTokensPerHour)
	if hourUtil >= threshold && !m.alertedHour {
		m.alertedHour = true
		m.fireAlert(Alert{
			Type:      AlertTokenHour,
			Message:   "Hour token usage threshold exceeded",
			Threshold: threshold,
			Current:   hourUtil,
			Timestamp: time.Now(),
		})
	}

	// 检查日阈值
	dayUtil := float64(atomic.LoadInt64(&m.tokensDay)) / float64(m.config.MaxTokensPerDay)
	if dayUtil >= threshold && !m.alertedDay {
		m.alertedDay = true
		m.fireAlert(Alert{
			Type:      AlertTokenDay,
			Message:   "Day token usage threshold exceeded",
			Threshold: threshold,
			Current:   dayUtil,
			Timestamp: time.Now(),
		})
	}

	// 检查费用门槛值
	costUtil := float64(atomic.LoadInt64(&m.costDay)) / 1000000 / m.config.MaxCostPerDay
	if costUtil >= threshold && !m.alertedCost {
		m.alertedCost = true
		m.fireAlert(Alert{
			Type:      AlertCostDay,
			Message:   "Daily cost threshold exceeded",
			Threshold: threshold,
			Current:   costUtil,
			Timestamp: time.Now(),
		})
	}
}

func (m *TokenBudgetManager) fireAlert(alert Alert) {
	m.logger.Warn("budget alert",
		zap.String("type", string(alert.Type)),
		zap.String("message", alert.Message),
		zap.Float64("threshold", alert.Threshold),
		zap.Float64("current", alert.Current))

	for _, handler := range m.alertHandlers {
		go handler(alert)
	}
}

// 重置所有计数器(用于测试).
func (m *TokenBudgetManager) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	atomic.StoreInt64(&m.tokensMinute, 0)
	atomic.StoreInt64(&m.tokensHour, 0)
	atomic.StoreInt64(&m.tokensDay, 0)
	atomic.StoreInt64(&m.costDay, 0)

	now := time.Now()
	m.minuteStart = now
	m.hourStart = now
	m.dayStart = now.Truncate(24 * time.Hour)
	m.throttleUntil = time.Time{}

	m.alertedMinute = false
	m.alertedHour = false
	m.alertedDay = false
	m.alertedCost = false
}
