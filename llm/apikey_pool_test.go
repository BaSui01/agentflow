package llm

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	
	// 自动迁移
	err = db.AutoMigrate(&LLMProviderAPIKey{})
	require.NoError(t, err)
	
	return db
}

func TestAPIKeyPool_LoadKeys(t *testing.T) {
	db := setupTestDB(t)
	logger := zaptest.NewLogger(t)
	
	// 准备测试数据
	keys := []*LLMProviderAPIKey{
		{ProviderID: 1, APIKey: "key1", Label: "主账号", Priority: 1, Weight: 100, Enabled: true},
		{ProviderID: 1, APIKey: "key2", Label: "备用账号", Priority: 2, Weight: 50, Enabled: true},
		{ProviderID: 1, APIKey: "key3", Label: "禁用账号", Priority: 3, Weight: 100, Enabled: false},
	}
	
	for _, key := range keys {
		require.NoError(t, db.Create(key).Error)
	}
	
	// 测试加载
	pool := NewAPIKeyPool(db, 1, StrategyWeightedRandom, logger)
	err := pool.LoadKeys(context.Background())
	require.NoError(t, err)
	
	// 验证只加载了启用的 Keys
	assert.Len(t, pool.keys, 2)
	assert.Equal(t, "key1", pool.keys[0].APIKey)
	assert.Equal(t, "key2", pool.keys[1].APIKey)
}

func TestAPIKeyPool_SelectKey_RoundRobin(t *testing.T) {
	db := setupTestDB(t)
	logger := zaptest.NewLogger(t)
	
	keys := []*LLMProviderAPIKey{
		{ProviderID: 1, APIKey: "key1", Priority: 1, Weight: 100, Enabled: true},
		{ProviderID: 1, APIKey: "key2", Priority: 2, Weight: 100, Enabled: true},
		{ProviderID: 1, APIKey: "key3", Priority: 3, Weight: 100, Enabled: true},
	}
	
	for _, key := range keys {
		require.NoError(t, db.Create(key).Error)
	}
	
	pool := NewAPIKeyPool(db, 1, StrategyRoundRobin, logger)
	require.NoError(t, pool.LoadKeys(context.Background()))
	
	// 测试轮询
	ctx := context.Background()
	selected1, err := pool.SelectKey(ctx)
	require.NoError(t, err)
	assert.Equal(t, "key1", selected1.APIKey)
	
	selected2, err := pool.SelectKey(ctx)
	require.NoError(t, err)
	assert.Equal(t, "key2", selected2.APIKey)
	
	selected3, err := pool.SelectKey(ctx)
	require.NoError(t, err)
	assert.Equal(t, "key3", selected3.APIKey)
	
	// 循环回到第一个
	selected4, err := pool.SelectKey(ctx)
	require.NoError(t, err)
	assert.Equal(t, "key1", selected4.APIKey)
}

func TestAPIKeyPool_SelectKey_Priority(t *testing.T) {
	db := setupTestDB(t)
	logger := zaptest.NewLogger(t)
	
	keys := []*LLMProviderAPIKey{
		{ProviderID: 1, APIKey: "key-low", Priority: 100, Weight: 100, Enabled: true},
		{ProviderID: 1, APIKey: "key-high", Priority: 1, Weight: 100, Enabled: true},
		{ProviderID: 1, APIKey: "key-mid", Priority: 50, Weight: 100, Enabled: true},
	}
	
	for _, key := range keys {
		require.NoError(t, db.Create(key).Error)
	}
	
	pool := NewAPIKeyPool(db, 1, StrategyPriority, logger)
	require.NoError(t, pool.LoadKeys(context.Background()))
	
	// 应该总是选择优先级最高的（数字最小）
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		selected, err := pool.SelectKey(ctx)
		require.NoError(t, err)
		assert.Equal(t, "key-high", selected.APIKey)
	}
}

func TestAPIKeyPool_SelectKey_RateLimited(t *testing.T) {
	db := setupTestDB(t)
	logger := zaptest.NewLogger(t)
	
	now := time.Now()
	keys := []*LLMProviderAPIKey{
		{
			ProviderID:   1,
			APIKey:       "key-limited",
			Priority:     1,
			Weight:       100,
			Enabled:      true,
			RateLimitRPM: 10,
			CurrentRPM:   10, // 已达限制
			RPMResetAt:   now.Add(time.Minute),
		},
		{
			ProviderID: 1,
			APIKey:     "key-available",
			Priority:   2,
			Weight:     100,
			Enabled:    true,
		},
	}
	
	for _, key := range keys {
		require.NoError(t, db.Create(key).Error)
	}
	
	pool := NewAPIKeyPool(db, 1, StrategyPriority, logger)
	require.NoError(t, pool.LoadKeys(context.Background()))
	
	// 应该跳过限流的 Key，选择可用的
	ctx := context.Background()
	selected, err := pool.SelectKey(ctx)
	require.NoError(t, err)
	assert.Equal(t, "key-available", selected.APIKey)
}

func TestAPIKeyPool_SelectKey_AllRateLimited(t *testing.T) {
	db := setupTestDB(t)
	logger := zaptest.NewLogger(t)
	
	now := time.Now()
	keys := []*LLMProviderAPIKey{
		{
			ProviderID:   1,
			APIKey:       "key1",
			RateLimitRPM: 10,
			CurrentRPM:   10,
			RPMResetAt:   now.Add(time.Minute),
			Enabled:      true,
		},
		{
			ProviderID:   1,
			APIKey:       "key2",
			RateLimitRPM: 10,
			CurrentRPM:   10,
			RPMResetAt:   now.Add(time.Minute),
			Enabled:      true,
		},
	}
	
	for _, key := range keys {
		require.NoError(t, db.Create(key).Error)
	}
	
	pool := NewAPIKeyPool(db, 1, StrategyRoundRobin, logger)
	require.NoError(t, pool.LoadKeys(context.Background()))
	
	// 所有 Key 都限流，应该返回错误
	ctx := context.Background()
	_, err := pool.SelectKey(ctx)
	assert.ErrorIs(t, err, ErrAllKeysRateLimited)
}

func TestAPIKeyPool_RecordSuccess(t *testing.T) {
	db := setupTestDB(t)
	logger := zaptest.NewLogger(t)
	
	key := &LLMProviderAPIKey{
		ProviderID: 1,
		APIKey:     "test-key",
		Enabled:    true,
	}
	require.NoError(t, db.Create(key).Error)
	
	pool := NewAPIKeyPool(db, 1, StrategyRoundRobin, logger)
	require.NoError(t, pool.LoadKeys(context.Background()))
	
	ctx := context.Background()
	selected, err := pool.SelectKey(ctx)
	require.NoError(t, err)
	
	// 记录成功
	err = pool.RecordSuccess(ctx, selected.ID)
	require.NoError(t, err)
	
	// 验证统计信息
	time.Sleep(100 * time.Millisecond) // 等待异步更新
	stats := pool.GetStats()
	assert.Equal(t, int64(1), stats[selected.ID].TotalRequests)
	assert.Equal(t, int64(0), stats[selected.ID].FailedRequests)
	assert.Equal(t, 1.0, stats[selected.ID].SuccessRate)
}

func TestAPIKeyPool_RecordFailure(t *testing.T) {
	db := setupTestDB(t)
	logger := zaptest.NewLogger(t)
	
	key := &LLMProviderAPIKey{
		ProviderID: 1,
		APIKey:     "test-key",
		Enabled:    true,
	}
	require.NoError(t, db.Create(key).Error)
	
	pool := NewAPIKeyPool(db, 1, StrategyRoundRobin, logger)
	require.NoError(t, pool.LoadKeys(context.Background()))
	
	ctx := context.Background()
	selected, err := pool.SelectKey(ctx)
	require.NoError(t, err)
	
	// 记录失败
	err = pool.RecordFailure(ctx, selected.ID, "rate limit exceeded")
	require.NoError(t, err)
	
	// 验证统计信息
	time.Sleep(100 * time.Millisecond)
	stats := pool.GetStats()
	assert.Equal(t, int64(1), stats[selected.ID].TotalRequests)
	assert.Equal(t, int64(1), stats[selected.ID].FailedRequests)
	assert.Equal(t, 0.0, stats[selected.ID].SuccessRate)
	assert.Equal(t, "rate limit exceeded", stats[selected.ID].LastError)
}

func TestAPIKeyPool_WeightedRandom(t *testing.T) {
	db := setupTestDB(t)
	logger := zaptest.NewLogger(t)
	
	keys := []*LLMProviderAPIKey{
		{ProviderID: 1, APIKey: "key-heavy", Weight: 90, Enabled: true},  // 90% 概率
		{ProviderID: 1, APIKey: "key-light", Weight: 10, Enabled: true},  // 10% 概率
	}
	
	for _, key := range keys {
		require.NoError(t, db.Create(key).Error)
	}
	
	pool := NewAPIKeyPool(db, 1, StrategyWeightedRandom, logger)
	require.NoError(t, pool.LoadKeys(context.Background()))
	
	// 多次选择，统计分布
	ctx := context.Background()
	counts := make(map[string]int)
	iterations := 1000
	
	for i := 0; i < iterations; i++ {
		selected, err := pool.SelectKey(ctx)
		require.NoError(t, err)
		counts[selected.APIKey]++
	}
	
	// 验证分布接近权重比例（允许 20% 误差）
	heavyRatio := float64(counts["key-heavy"]) / float64(iterations)
	assert.InDelta(t, 0.9, heavyRatio, 0.2)
}

func TestLLMProviderAPIKey_IsHealthy(t *testing.T) {
	tests := []struct {
		name     string
		key      *LLMProviderAPIKey
		expected bool
	}{
		{
			name: "健康的 Key",
			key: &LLMProviderAPIKey{
				Enabled:        true,
				TotalRequests:  100,
				FailedRequests: 10,
			},
			expected: true,
		},
		{
			name: "禁用的 Key",
			key: &LLMProviderAPIKey{
				Enabled: false,
			},
			expected: false,
		},
		{
			name: "RPM 限流",
			key: &LLMProviderAPIKey{
				Enabled:      true,
				RateLimitRPM: 10,
				CurrentRPM:   10,
				RPMResetAt:   time.Now().Add(time.Minute),
			},
			expected: false,
		},
		{
			name: "RPM 已重置",
			key: &LLMProviderAPIKey{
				Enabled:      true,
				RateLimitRPM: 10,
				CurrentRPM:   10,
				RPMResetAt:   time.Now().Add(-time.Minute), // 已过期
			},
			expected: true,
		},
		{
			name: "高错误率",
			key: &LLMProviderAPIKey{
				Enabled:        true,
				TotalRequests:  100,
				FailedRequests: 60, // 60% 失败率
			},
			expected: false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.key.IsHealthy()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestLLMProviderAPIKey_IncrementUsage(t *testing.T) {
	key := &LLMProviderAPIKey{
		RateLimitRPM: 100,
		RateLimitRPD: 1000,
		RPMResetAt:   time.Now().Add(-time.Second), // 已过期
		RPDResetAt:   time.Now().Add(-time.Second),
	}
	
	// 成功请求
	key.IncrementUsage(true)
	assert.Equal(t, int64(1), key.TotalRequests)
	assert.Equal(t, int64(0), key.FailedRequests)
	assert.Equal(t, 1, key.CurrentRPM)
	assert.Equal(t, 1, key.CurrentRPD)
	assert.NotNil(t, key.LastUsedAt)
	
	// 失败请求
	key.IncrementUsage(false)
	assert.Equal(t, int64(2), key.TotalRequests)
	assert.Equal(t, int64(1), key.FailedRequests)
	assert.NotNil(t, key.LastErrorAt)
}

func BenchmarkAPIKeyPool_SelectKey(b *testing.B) {
	db := setupTestDB(&testing.T{})
	logger := zaptest.NewLogger(&testing.T{})
	
	// 准备 100 个 Keys
	for i := 0; i < 100; i++ {
		key := &LLMProviderAPIKey{
			ProviderID: 1,
			APIKey:     "key-" + string(rune(i)),
			Weight:     100,
			Enabled:    true,
		}
		db.Create(key)
	}
	
	pool := NewAPIKeyPool(db, 1, StrategyWeightedRandom, logger)
	pool.LoadKeys(context.Background())
	
	ctx := context.Background()
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		_, _ = pool.SelectKey(ctx)
	}
}
