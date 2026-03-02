package policy

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

type testBlockingLimiter struct {
	err error
}

func (t *testBlockingLimiter) Wait(context.Context) error {
	return t.err
}

func TestManager_PreCheck_RateLimited(t *testing.T) {
	m := NewManager(ManagerConfig{
		RateLimiter: &testBlockingLimiter{err: errors.New("too many requests")},
	})

	err := m.PreCheck(context.Background(), 0, 0)
	assert.Error(t, err)
	assert.True(t, types.IsErrorCode(err, types.ErrRateLimit))
}

func TestManager_PreCheck_BudgetExceeded(t *testing.T) {
	b := NewTokenBudgetManager(BudgetConfig{
		MaxTokensPerRequest: 10,
		MaxCostPerRequest:   1,
		MaxTokensPerMinute:  100,
		MaxTokensPerHour:    1000,
		MaxTokensPerDay:     10000,
		MaxCostPerDay:       1000,
		AlertThreshold:      0.8,
		AutoThrottle:        true,
		ThrottleDelay:       time.Second,
	}, zap.NewNop())

	m := NewManager(ManagerConfig{Budget: b})
	err := m.PreCheck(context.Background(), 20, 0.5)
	assert.Error(t, err)
	assert.True(t, types.IsErrorCode(err, types.ErrQuotaExceeded))
}

func TestManager_RecordUsage(t *testing.T) {
	b := NewTokenBudgetManager(DefaultBudgetConfig(), zap.NewNop())
	m := NewManager(ManagerConfig{Budget: b})

	m.RecordUsage(UsageRecord{Tokens: 100, Cost: 0.1, Model: "gpt-test"})
	status := b.GetStatus()
	assert.Equal(t, int64(100), status.TokensUsedMinute)
	assert.InDelta(t, 0.1, status.CostUsedDay, 0.001)
}
