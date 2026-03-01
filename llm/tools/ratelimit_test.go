package tools

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ====== RateLimitManager Tests ======

func TestNewRateLimitManager(t *testing.T) {
	rlm := NewRateLimitManager(nil)
	assert.NotNil(t, rlm)
	assert.NotNil(t, rlm.rules)
	assert.NotNil(t, rlm.limiters)
	assert.NotNil(t, rlm.stats)
}

func TestDefaultRateLimitManager_AddRule(t *testing.T) {
	rlm := NewRateLimitManager(nil)
	rule := CreateGlobalRateLimit("r1", "global-limit", 100, time.Minute)

	err := rlm.AddRule(rule)
	require.NoError(t, err)

	got, ok := rlm.GetRule("r1")
	assert.True(t, ok)
	assert.Equal(t, "global-limit", got.Name)
	assert.False(t, got.CreatedAt.IsZero())
}

func TestDefaultRateLimitManager_AddRule_EmptyID(t *testing.T) {
	rlm := NewRateLimitManager(nil)
	err := rlm.AddRule(&RateLimitRule{Name: "test"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "rule ID is required")
}

func TestDefaultRateLimitManager_RemoveRule(t *testing.T) {
	rlm := NewRateLimitManager(nil)
	require.NoError(t, rlm.AddRule(CreateGlobalRateLimit("r1", "test", 10, time.Second)))

	err := rlm.RemoveRule("r1")
	require.NoError(t, err)

	_, ok := rlm.GetRule("r1")
	assert.False(t, ok)
}

func TestDefaultRateLimitManager_RemoveRule_NotFound(t *testing.T) {
	rlm := NewRateLimitManager(nil)
	err := rlm.RemoveRule("nonexistent")
	assert.Error(t, err)
}

func TestDefaultRateLimitManager_ListRules(t *testing.T) {
	rlm := NewRateLimitManager(nil)
	require.NoError(t, rlm.AddRule(CreateGlobalRateLimit("r1", "rule1", 10, time.Second)))
	require.NoError(t, rlm.AddRule(CreateGlobalRateLimit("r2", "rule2", 20, time.Second)))

	rules := rlm.ListRules()
	assert.Len(t, rules, 2)
}

func TestDefaultRateLimitManager_CheckRateLimit_Allowed(t *testing.T) {
	rlm := NewRateLimitManager(nil)
	rule := CreateGlobalRateLimit("r1", "global", 10, time.Minute)
	require.NoError(t, rlm.AddRule(rule))

	result, err := rlm.CheckRateLimit(context.Background(), &RateLimitContext{
		ToolName: "calc",
	})
	require.NoError(t, err)
	assert.True(t, result.Allowed)
}

func TestDefaultRateLimitManager_CheckRateLimit_Exceeded(t *testing.T) {
	rlm := NewRateLimitManager(nil)
	rule := CreateGlobalRateLimit("r1", "global", 2, time.Minute)
	require.NoError(t, rlm.AddRule(rule))

	ctx := context.Background()
	rlCtx := &RateLimitContext{ToolName: "calc"}

	// First 2 should pass
	for i := 0; i < 2; i++ {
		result, err := rlm.CheckRateLimit(ctx, rlCtx)
		require.NoError(t, err)
		assert.True(t, result.Allowed, "request %d should be allowed", i)
	}

	// Third should be rejected
	result, err := rlm.CheckRateLimit(ctx, rlCtx)
	require.NoError(t, err)
	assert.False(t, result.Allowed)
	assert.Equal(t, RateLimitActionReject, result.Action)
	assert.NotEmpty(t, result.Reason)
}

func TestDefaultRateLimitManager_CheckRateLimit_DisabledRule(t *testing.T) {
	rlm := NewRateLimitManager(nil)
	rule := CreateGlobalRateLimit("r1", "global", 1, time.Minute)
	rule.Enabled = false
	require.NoError(t, rlm.AddRule(rule))

	// Should always be allowed since rule is disabled
	for i := 0; i < 5; i++ {
		result, err := rlm.CheckRateLimit(context.Background(), &RateLimitContext{ToolName: "calc"})
		require.NoError(t, err)
		assert.True(t, result.Allowed)
	}
}

func TestDefaultRateLimitManager_CheckRateLimit_ToolScope(t *testing.T) {
	rlm := NewRateLimitManager(nil)
	rule := CreateToolRateLimit("r1", "tool-limit", "calc*", 2, time.Minute)
	require.NoError(t, rlm.AddRule(rule))

	ctx := context.Background()

	// calc should be rate limited
	for i := 0; i < 2; i++ {
		result, err := rlm.CheckRateLimit(ctx, &RateLimitContext{ToolName: "calculator"})
		require.NoError(t, err)
		assert.True(t, result.Allowed)
	}
	result, err := rlm.CheckRateLimit(ctx, &RateLimitContext{ToolName: "calculator"})
	require.NoError(t, err)
	assert.False(t, result.Allowed)

	// search should not be affected
	result, err = rlm.CheckRateLimit(ctx, &RateLimitContext{ToolName: "search"})
	require.NoError(t, err)
	assert.True(t, result.Allowed)
}

func TestDefaultRateLimitManager_CheckRateLimit_UserScope(t *testing.T) {
	rlm := NewRateLimitManager(nil)
	rule := CreateUserRateLimit("r1", "user-limit", 1, time.Minute)
	require.NoError(t, rlm.AddRule(rule))

	ctx := context.Background()

	// User1 uses their quota
	result, err := rlm.CheckRateLimit(ctx, &RateLimitContext{UserID: "user1", ToolName: "calc"})
	require.NoError(t, err)
	assert.True(t, result.Allowed)

	result, err = rlm.CheckRateLimit(ctx, &RateLimitContext{UserID: "user1", ToolName: "calc"})
	require.NoError(t, err)
	assert.False(t, result.Allowed)

	// User2 should still have quota
	result, err = rlm.CheckRateLimit(ctx, &RateLimitContext{UserID: "user2", ToolName: "calc"})
	require.NoError(t, err)
	assert.True(t, result.Allowed)
}

func TestDefaultRateLimitManager_CheckRateLimit_AgentScope(t *testing.T) {
	rlm := NewRateLimitManager(nil)
	rule := CreateAgentRateLimit("r1", "agent-limit", 1, time.Minute)
	require.NoError(t, rlm.AddRule(rule))

	ctx := context.Background()

	result, err := rlm.CheckRateLimit(ctx, &RateLimitContext{AgentID: "a1", ToolName: "calc"})
	require.NoError(t, err)
	assert.True(t, result.Allowed)

	result, err = rlm.CheckRateLimit(ctx, &RateLimitContext{AgentID: "a1", ToolName: "calc"})
	require.NoError(t, err)
	assert.False(t, result.Allowed)
}

func TestDefaultRateLimitManager_CheckRateLimit_TokenBucket(t *testing.T) {
	rlm := NewRateLimitManager(nil)
	rule := CreateTokenBucketRateLimit("r1", "bucket", 3, 10.0)
	require.NoError(t, rlm.AddRule(rule))

	ctx := context.Background()
	rlCtx := &RateLimitContext{ToolName: "calc"}

	// Should allow burst of 3
	for i := 0; i < 3; i++ {
		result, err := rlm.CheckRateLimit(ctx, rlCtx)
		require.NoError(t, err)
		assert.True(t, result.Allowed)
	}

	// 4th should be rejected
	result, err := rlm.CheckRateLimit(ctx, rlCtx)
	require.NoError(t, err)
	assert.False(t, result.Allowed)
}

func TestDefaultRateLimitManager_CheckRateLimit_FixedWindow(t *testing.T) {
	rlm := NewRateLimitManager(nil)
	rule := &RateLimitRule{
		ID:          "r1",
		Name:        "fixed",
		Scope:       RateLimitScopeGlobal,
		Strategy:    RateLimitStrategyFixedWindow,
		MaxRequests: 2,
		Window:      time.Minute,
		Action:      RateLimitActionReject,
		Enabled:     true,
	}
	require.NoError(t, rlm.AddRule(rule))

	ctx := context.Background()
	rlCtx := &RateLimitContext{ToolName: "calc"}

	for i := 0; i < 2; i++ {
		result, err := rlm.CheckRateLimit(ctx, rlCtx)
		require.NoError(t, err)
		assert.True(t, result.Allowed)
	}

	result, err := rlm.CheckRateLimit(ctx, rlCtx)
	require.NoError(t, err)
	assert.False(t, result.Allowed)
}

func TestDefaultRateLimitManager_GetStats(t *testing.T) {
	rlm := NewRateLimitManager(nil)
	rule := CreateGlobalRateLimit("r1", "global", 10, time.Minute)
	require.NoError(t, rlm.AddRule(rule))

	ctx := context.Background()
	rlCtx := &RateLimitContext{ToolName: "calc"}

	_, err := rlm.CheckRateLimit(ctx, rlCtx)
	require.NoError(t, err)

	// Stats key format: "global:r1"
	stats := rlm.GetStats(RateLimitScopeGlobal, "r1")
	if stats != nil {
		assert.Equal(t, int64(1), stats.TotalRequests)
		assert.Equal(t, int64(1), stats.AllowedRequests)
	}
}

func TestDefaultRateLimitManager_Reset(t *testing.T) {
	rlm := NewRateLimitManager(nil)
	rule := CreateGlobalRateLimit("r1", "global", 1, time.Minute)
	require.NoError(t, rlm.AddRule(rule))

	ctx := context.Background()
	rlCtx := &RateLimitContext{ToolName: "calc"}

	// Use up the quota
	_, err := rlm.CheckRateLimit(ctx, rlCtx)
	require.NoError(t, err)

	result, err := rlm.CheckRateLimit(ctx, rlCtx)
	require.NoError(t, err)
	assert.False(t, result.Allowed)

	// Reset
	err = rlm.Reset(RateLimitScopeGlobal, "r1")
	require.NoError(t, err)
}

// ====== SlidingWindowLimiter Tests ======

func TestSlidingWindowLimiter_Basic(t *testing.T) {
	l := NewSlidingWindowLimiter(3, time.Minute)

	for i := 0; i < 3; i++ {
		assert.True(t, l.Allow(), "request %d should be allowed", i)
	}
	assert.False(t, l.Allow(), "4th request should be rejected")
}

func TestSlidingWindowLimiter_Remaining(t *testing.T) {
	l := NewSlidingWindowLimiter(5, time.Minute)

	assert.Equal(t, 5, l.Remaining())
	l.Allow()
	l.Allow()
	assert.Equal(t, 3, l.Remaining())
}

func TestSlidingWindowLimiter_ResetAt(t *testing.T) {
	l := NewSlidingWindowLimiter(5, time.Minute)

	// Empty limiter
	resetAt := l.ResetAt()
	assert.False(t, resetAt.IsZero())

	l.Allow()
	resetAt = l.ResetAt()
	assert.True(t, resetAt.After(time.Now()))
}

func TestSlidingWindowLimiter_Reset(t *testing.T) {
	l := NewSlidingWindowLimiter(2, time.Minute)
	l.Allow()
	l.Allow()
	assert.False(t, l.Allow())

	l.Reset()
	assert.True(t, l.Allow())
}

// ====== Exported TokenBucketLimiter Tests ======

func TestExportedTokenBucketLimiter_Allow(t *testing.T) {
	l := NewTokenBucketLimiter(3, 1.0)

	for i := 0; i < 3; i++ {
		assert.True(t, l.Allow())
	}
	assert.False(t, l.Allow())
}

func TestExportedTokenBucketLimiter_Remaining(t *testing.T) {
	l := NewTokenBucketLimiter(5, 1.0)
	assert.Equal(t, 5, l.Remaining())

	l.Allow()
	assert.True(t, l.Remaining() >= 4)
}

func TestExportedTokenBucketLimiter_ResetAt(t *testing.T) {
	l := NewTokenBucketLimiter(5, 1.0)

	// Full bucket
	resetAt := l.ResetAt()
	assert.True(t, resetAt.Before(time.Now().Add(time.Second)))

	// Drain some tokens
	for i := 0; i < 5; i++ {
		l.Allow()
	}
	resetAt = l.ResetAt()
	assert.True(t, resetAt.After(time.Now()))
}

func TestExportedTokenBucketLimiter_Reset(t *testing.T) {
	l := NewTokenBucketLimiter(2, 1.0)
	l.Allow()
	l.Allow()
	assert.False(t, l.Allow())

	l.Reset()
	assert.True(t, l.Allow())
}

// ====== FixedWindowLimiter Tests ======

func TestFixedWindowLimiter_Basic(t *testing.T) {
	l := NewFixedWindowLimiter(3, time.Minute)

	for i := 0; i < 3; i++ {
		assert.True(t, l.Allow())
	}
	assert.False(t, l.Allow())
}

func TestFixedWindowLimiter_Remaining(t *testing.T) {
	l := NewFixedWindowLimiter(5, time.Minute)
	assert.Equal(t, 5, l.Remaining())

	l.Allow()
	assert.Equal(t, 4, l.Remaining())
}

func TestFixedWindowLimiter_ResetAt(t *testing.T) {
	l := NewFixedWindowLimiter(5, time.Minute)
	resetAt := l.ResetAt()
	assert.True(t, resetAt.After(time.Now()))
}

func TestFixedWindowLimiter_Reset(t *testing.T) {
	l := NewFixedWindowLimiter(2, time.Minute)
	l.Allow()
	l.Allow()
	assert.False(t, l.Allow())

	l.Reset()
	assert.True(t, l.Allow())
}

// ====== Convenience Function Tests ======

func TestCreateGlobalRateLimit(t *testing.T) {
	rule := CreateGlobalRateLimit("r1", "global", 100, time.Minute)
	assert.Equal(t, RateLimitScopeGlobal, rule.Scope)
	assert.Equal(t, RateLimitStrategySlidingWindow, rule.Strategy)
	assert.Equal(t, 100, rule.MaxRequests)
	assert.True(t, rule.Enabled)
}

func TestCreateToolRateLimit(t *testing.T) {
	rule := CreateToolRateLimit("r1", "tool", "calc*", 50, time.Minute)
	assert.Equal(t, RateLimitScopeTool, rule.Scope)
	assert.Equal(t, "calc*", rule.ToolPattern)
}

func TestCreateUserRateLimit(t *testing.T) {
	rule := CreateUserRateLimit("r1", "user", 10, time.Minute)
	assert.Equal(t, RateLimitScopeUser, rule.Scope)
}

func TestCreateAgentRateLimit(t *testing.T) {
	rule := CreateAgentRateLimit("r1", "agent", 10, time.Minute)
	assert.Equal(t, RateLimitScopeAgent, rule.Scope)
}

func TestCreateTokenBucketRateLimit(t *testing.T) {
	rule := CreateTokenBucketRateLimit("r1", "bucket", 10, 5.0)
	assert.Equal(t, RateLimitStrategyTokenBucket, rule.Strategy)
	assert.Equal(t, 10, rule.BurstSize)
	assert.Equal(t, 5.0, rule.RefillRate)
}

// ====== RateLimitMiddleware Tests ======

func TestRateLimitMiddleware_Allowed(t *testing.T) {
	rlm := NewRateLimitManager(nil)
	require.NoError(t, rlm.AddRule(CreateGlobalRateLimit("r1", "global", 10, time.Minute)))

	innerFn := func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{"ok":true}`), nil
	}

	middleware := RateLimitMiddleware(rlm, nil)
	wrappedFn := middleware(innerFn)

	ctx := WithPermissionContext(context.Background(), &PermissionContext{
		ToolName: "calc",
	})

	result, err := wrappedFn(ctx, nil)
	require.NoError(t, err)
	assert.JSONEq(t, `{"ok":true}`, string(result))
}

func TestRateLimitMiddleware_Rejected(t *testing.T) {
	rlm := NewRateLimitManager(nil)
	require.NoError(t, rlm.AddRule(CreateGlobalRateLimit("r1", "global", 1, time.Minute)))

	callCount := 0
	innerFn := func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
		callCount++
		return json.RawMessage(`{"ok":true}`), nil
	}

	middleware := RateLimitMiddleware(rlm, nil)
	wrappedFn := middleware(innerFn)

	ctx := WithPermissionContext(context.Background(), &PermissionContext{
		ToolName: "calc",
	})

	// First call succeeds
	_, err := wrappedFn(ctx, nil)
	require.NoError(t, err)

	// Second call rejected
	_, err = wrappedFn(ctx, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "rate limit exceeded")
	assert.Equal(t, 1, callCount)
}

