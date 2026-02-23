package budget

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func testLogger() *zap.Logger {
	return zap.NewNop()
}

func TestDefaultBudgetConfig(t *testing.T) {
	cfg := DefaultBudgetConfig()
	assert.Equal(t, 100000, cfg.MaxTokensPerRequest)
	assert.Equal(t, 500000, cfg.MaxTokensPerMinute)
	assert.Equal(t, 5000000, cfg.MaxTokensPerHour)
	assert.Equal(t, 50000000, cfg.MaxTokensPerDay)
	assert.Equal(t, 10.0, cfg.MaxCostPerRequest)
	assert.Equal(t, 1000.0, cfg.MaxCostPerDay)
	assert.Equal(t, 0.8, cfg.AlertThreshold)
	assert.True(t, cfg.AutoThrottle)
	assert.Equal(t, time.Second, cfg.ThrottleDelay)
}

func TestNewTokenBudgetManager(t *testing.T) {
	cfg := DefaultBudgetConfig()
	mgr := NewTokenBudgetManager(cfg, testLogger())
	require.NotNil(t, mgr)
	assert.Equal(t, cfg, mgr.config)
}

func TestTokenBudgetManager_CheckBudget_WithinLimits(t *testing.T) {
	cfg := DefaultBudgetConfig()
	mgr := NewTokenBudgetManager(cfg, testLogger())

	err := mgr.CheckBudget(context.Background(), 1000, 0.01)
	assert.NoError(t, err)
}

func TestTokenBudgetManager_CheckBudget_ExceedsPerRequestTokens(t *testing.T) {
	cfg := DefaultBudgetConfig()
	cfg.MaxTokensPerRequest = 1000
	mgr := NewTokenBudgetManager(cfg, testLogger())

	err := mgr.CheckBudget(context.Background(), 2000, 0.01)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "per-request limit")
}

func TestTokenBudgetManager_CheckBudget_ExceedsPerRequestCost(t *testing.T) {
	cfg := DefaultBudgetConfig()
	cfg.MaxCostPerRequest = 1.0
	mgr := NewTokenBudgetManager(cfg, testLogger())

	err := mgr.CheckBudget(context.Background(), 100, 5.0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "per-request limit")
}

func TestTokenBudgetManager_CheckBudget_ExceedsMinuteLimit(t *testing.T) {
	cfg := DefaultBudgetConfig()
	cfg.MaxTokensPerMinute = 1000
	cfg.AutoThrottle = false
	mgr := NewTokenBudgetManager(cfg, testLogger())

	mgr.RecordUsage(UsageRecord{Tokens: 900, Cost: 0.01})

	err := mgr.CheckBudget(context.Background(), 200, 0.01)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "minute token limit")
}

func TestTokenBudgetManager_CheckBudget_ExceedsHourLimit(t *testing.T) {
	cfg := DefaultBudgetConfig()
	cfg.MaxTokensPerHour = 2000
	mgr := NewTokenBudgetManager(cfg, testLogger())

	mgr.RecordUsage(UsageRecord{Tokens: 1900, Cost: 0.01})

	err := mgr.CheckBudget(context.Background(), 200, 0.01)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "hour token limit")
}

func TestTokenBudgetManager_CheckBudget_ExceedsDayLimit(t *testing.T) {
	cfg := DefaultBudgetConfig()
	cfg.MaxTokensPerDay = 3000
	mgr := NewTokenBudgetManager(cfg, testLogger())

	mgr.RecordUsage(UsageRecord{Tokens: 2900, Cost: 0.01})

	err := mgr.CheckBudget(context.Background(), 200, 0.01)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "day token limit")
}

func TestTokenBudgetManager_CheckBudget_ExceedsDailyCost(t *testing.T) {
	cfg := DefaultBudgetConfig()
	cfg.MaxCostPerDay = 10.0
	mgr := NewTokenBudgetManager(cfg, testLogger())

	mgr.RecordUsage(UsageRecord{Tokens: 100, Cost: 9.5})

	err := mgr.CheckBudget(context.Background(), 100, 1.0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "daily cost limit")
}

func TestTokenBudgetManager_CheckBudget_Throttled(t *testing.T) {
	cfg := DefaultBudgetConfig()
	cfg.MaxTokensPerMinute = 100
	cfg.AutoThrottle = true
	cfg.ThrottleDelay = 5 * time.Second
	mgr := NewTokenBudgetManager(cfg, testLogger())

	// Fill up minute budget to trigger throttle
	mgr.RecordUsage(UsageRecord{Tokens: 99, Cost: 0.01})
	_ = mgr.CheckBudget(context.Background(), 10, 0.01) // triggers throttle

	// Next check should be throttled
	err := mgr.CheckBudget(context.Background(), 1, 0.01)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "throttled")
}

func TestTokenBudgetManager_RecordUsage(t *testing.T) {
	cfg := DefaultBudgetConfig()
	mgr := NewTokenBudgetManager(cfg, testLogger())

	mgr.RecordUsage(UsageRecord{Tokens: 500, Cost: 0.05, Model: "gpt-4"})

	status := mgr.GetStatus()
	assert.Equal(t, int64(500), status.TokensUsedMinute)
	assert.Equal(t, int64(500), status.TokensUsedHour)
	assert.Equal(t, int64(500), status.TokensUsedDay)
	assert.InDelta(t, 0.05, status.CostUsedDay, 0.001)
}

func TestTokenBudgetManager_GetStatus(t *testing.T) {
	cfg := DefaultBudgetConfig()
	mgr := NewTokenBudgetManager(cfg, testLogger())

	status := mgr.GetStatus()
	assert.Equal(t, int64(0), status.TokensUsedMinute)
	assert.Equal(t, int64(0), status.TokensUsedHour)
	assert.Equal(t, int64(0), status.TokensUsedDay)
	assert.Equal(t, 0.0, status.CostUsedDay)
	assert.False(t, status.IsThrottled)
	assert.Nil(t, status.ThrottleUntil)
}

func TestTokenBudgetManager_GetStatus_Utilization(t *testing.T) {
	cfg := DefaultBudgetConfig()
	cfg.MaxTokensPerMinute = 1000
	cfg.MaxTokensPerHour = 10000
	cfg.MaxTokensPerDay = 100000
	cfg.MaxCostPerDay = 100.0
	mgr := NewTokenBudgetManager(cfg, testLogger())

	mgr.RecordUsage(UsageRecord{Tokens: 500, Cost: 50.0})

	status := mgr.GetStatus()
	assert.InDelta(t, 0.5, status.MinuteUtilization, 0.01)
	assert.InDelta(t, 0.05, status.HourUtilization, 0.01)
	assert.InDelta(t, 0.005, status.DayUtilization, 0.001)
	assert.InDelta(t, 0.5, status.CostUtilization, 0.01)
}

func TestTokenBudgetManager_OnAlert(t *testing.T) {
	cfg := DefaultBudgetConfig()
	cfg.MaxTokensPerMinute = 100
	cfg.AlertThreshold = 0.8
	mgr := NewTokenBudgetManager(cfg, testLogger())

	var received []Alert
	var mu sync.Mutex
	mgr.OnAlert(func(a Alert) {
		mu.Lock()
		received = append(received, a)
		mu.Unlock()
	})

	// Record usage above threshold (80% of 100 = 80)
	mgr.RecordUsage(UsageRecord{Tokens: 85, Cost: 0.01})

	// Give goroutine time to fire
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	require.GreaterOrEqual(t, len(received), 1)
	found := false
	for _, a := range received {
		if a.Type == AlertTokenMinute {
			found = true
			assert.Equal(t, 0.8, a.Threshold)
		}
	}
	assert.True(t, found, "expected minute alert")
}

func TestTokenBudgetManager_Reset(t *testing.T) {
	cfg := DefaultBudgetConfig()
	mgr := NewTokenBudgetManager(cfg, testLogger())

	mgr.RecordUsage(UsageRecord{Tokens: 1000, Cost: 1.0})
	mgr.Reset()

	status := mgr.GetStatus()
	assert.Equal(t, int64(0), status.TokensUsedMinute)
	assert.Equal(t, int64(0), status.TokensUsedHour)
	assert.Equal(t, int64(0), status.TokensUsedDay)
	assert.Equal(t, 0.0, status.CostUsedDay)
	assert.False(t, status.IsThrottled)
}

func TestTokenBudgetManager_ConcurrentAccess(t *testing.T) {
	cfg := DefaultBudgetConfig()
	mgr := NewTokenBudgetManager(cfg, testLogger())

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = mgr.CheckBudget(context.Background(), 10, 0.001)
			mgr.RecordUsage(UsageRecord{Tokens: 10, Cost: 0.001})
			_ = mgr.GetStatus()
		}()
	}
	wg.Wait()

	status := mgr.GetStatus()
	assert.Equal(t, int64(500), status.TokensUsedMinute)
}

func TestTokenBudgetManager_AlertFiredOnlyOnce(t *testing.T) {
	cfg := DefaultBudgetConfig()
	cfg.MaxTokensPerMinute = 100
	cfg.AlertThreshold = 0.5
	mgr := NewTokenBudgetManager(cfg, testLogger())

	var count int
	var mu sync.Mutex
	mgr.OnAlert(func(a Alert) {
		if a.Type == AlertTokenMinute {
			mu.Lock()
			count++
			mu.Unlock()
		}
	})

	// Record twice above threshold
	mgr.RecordUsage(UsageRecord{Tokens: 60, Cost: 0.01})
	mgr.RecordUsage(UsageRecord{Tokens: 10, Cost: 0.01})

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 1, count, "alert should fire only once per window")
}
