package policy

import (
	"context"
	"time"

	"github.com/BaSui01/agentflow/types"
)

// BlockingRateLimiter 定义阻塞式限流接口。
type BlockingRateLimiter interface {
	Wait(ctx context.Context) error
}

// ManagerConfig 定义策略管理器依赖。
type ManagerConfig struct {
	Budget      *TokenBudgetManager
	RetryPolicy *RetryPolicy
	RateLimiter BlockingRateLimiter
}

// Manager 聚合预算、限流和重试策略。
type Manager struct {
	budget      *TokenBudgetManager
	retryPolicy *RetryPolicy
	rateLimiter BlockingRateLimiter
}

// NewManager 创建策略管理器。
func NewManager(cfg ManagerConfig) *Manager {
	retryPolicy := cfg.RetryPolicy
	if retryPolicy == nil {
		retryPolicy = DefaultRetryPolicy()
	}
	return &Manager{
		budget:      cfg.Budget,
		retryPolicy: retryPolicy,
		rateLimiter: cfg.RateLimiter,
	}
}

// PreCheck 执行请求前策略检查。
func (m *Manager) PreCheck(ctx context.Context, estimatedTokens int, estimatedCostUSD float64) error {
	if m == nil {
		return nil
	}
	if m.rateLimiter != nil {
		if err := m.rateLimiter.Wait(ctx); err != nil {
			return types.NewRateLimitError(err.Error()).WithCause(err)
		}
	}
	if m.budget != nil {
		if err := m.budget.CheckBudget(ctx, estimatedTokens, estimatedCostUSD); err != nil {
			return types.NewError(types.ErrQuotaExceeded, err.Error()).
				WithHTTPStatus(402).
				WithRetryable(false).
				WithCause(err)
		}
	}
	return nil
}

// RecordUsage 记录请求后预算消耗。
func (m *Manager) RecordUsage(record UsageRecord) {
	if m == nil || m.budget == nil {
		return
	}
	if record.Timestamp.IsZero() {
		record.Timestamp = time.Now()
	}
	m.budget.RecordUsage(record)
}

// Retry 返回统一重试策略。
func (m *Manager) Retry() *RetryPolicy {
	if m == nil || m.retryPolicy == nil {
		return DefaultRetryPolicy()
	}
	return m.retryPolicy
}

// Budget 返回预算管理器引用。
func (m *Manager) Budget() *TokenBudgetManager {
	if m == nil {
		return nil
	}
	return m.budget
}
