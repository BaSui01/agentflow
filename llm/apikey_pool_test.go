//go:build cgo
// +build cgo

package llm

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&LLMProviderAPIKey{})
	require.NoError(t, err)

	return db
}

func TestAPIKeyPool_SelectKey(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	// 插入测试数据
	keys := []*LLMProviderAPIKey{
		{ProviderID: 1, APIKey: "key1", Label: "主账号", Priority: 10, Weight: 100, Enabled: true},
		{ProviderID: 1, APIKey: "key2", Label: "备用1", Priority: 50, Weight: 80, Enabled: true},
		{ProviderID: 1, APIKey: "key3", Label: "备用2", Priority: 100, Weight: 50, Enabled: true},
		{ProviderID: 1, APIKey: "key4", Label: "禁用", Priority: 200, Weight: 10, Enabled: false},
	}

	for _, key := range keys {
		require.NoError(t, db.Create(key).Error)
	}

	tests := []struct {
		name     string
		strategy APIKeySelectionStrategy
		wantErr  bool
	}{
		{"RoundRobin", StrategyRoundRobin, false},
		{"WeightedRandom", StrategyWeightedRandom, false},
		{"Priority", StrategyPriority, false},
		{"LeastUsed", StrategyLeastUsed, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := NewAPIKeyPool(db, 1, tt.strategy, zap.NewNop())
			err := pool.LoadKeys(ctx)
			require.NoError(t, err)

			key, err := pool.SelectKey(ctx)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, key)
				assert.True(t, key.Enabled)
			}
		})
	}
}

func TestAPIKeyPool_RoundRobin(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	keys := []*LLMProviderAPIKey{
		{ProviderID: 1, APIKey: "key1", Priority: 10, Weight: 100, Enabled: true},
		{ProviderID: 1, APIKey: "key2", Priority: 20, Weight: 100, Enabled: true},
		{ProviderID: 1, APIKey: "key3", Priority: 30, Weight: 100, Enabled: true},
	}

	for _, key := range keys {
		require.NoError(t, db.Create(key).Error)
	}

	pool := NewAPIKeyPool(db, 1, StrategyRoundRobin, zap.NewNop())
	require.NoError(t, pool.LoadKeys(ctx))

	// 轮询应该依次返回不同的 key
	selectedKeys := make(map[string]int)
	for i := 0; i < 9; i++ {
		key, err := pool.SelectKey(ctx)
		require.NoError(t, err)
		selectedKeys[key.APIKey]++
	}

	// 每个 key 应该被选中 3 次
	assert.Equal(t, 3, selectedKeys["key1"])
	assert.Equal(t, 3, selectedKeys["key2"])
	assert.Equal(t, 3, selectedKeys["key3"])
}

func TestAPIKeyPool_Priority(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	keys := []*LLMProviderAPIKey{
		{ProviderID: 1, APIKey: "key1", Priority: 100, Weight: 100, Enabled: true},
		{ProviderID: 1, APIKey: "key2", Priority: 10, Weight: 100, Enabled: true}, // 最高优先级
		{ProviderID: 1, APIKey: "key3", Priority: 50, Weight: 100, Enabled: true},
	}

	for _, key := range keys {
		require.NoError(t, db.Create(key).Error)
	}

	pool := NewAPIKeyPool(db, 1, StrategyPriority, zap.NewNop())
	require.NoError(t, pool.LoadKeys(ctx))

	// 应该总是返回优先级最高的 key
	for i := 0; i < 5; i++ {
		key, err := pool.SelectKey(ctx)
		require.NoError(t, err)
		assert.Equal(t, "key2", key.APIKey)
	}
}

func TestAPIKeyPool_RateLimiting(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	now := time.Now()
	key := &LLMProviderAPIKey{
		ProviderID:   1,
		APIKey:       "key1",
		Priority:     10,
		Weight:       100,
		Enabled:      true,
		RateLimitRPM: 10,
		CurrentRPM:   10, // 已达限制
		RPMResetAt:   now.Add(time.Minute),
	}

	require.NoError(t, db.Create(key).Error)

	pool := NewAPIKeyPool(db, 1, StrategyPriority, zap.NewNop())
	require.NoError(t, pool.LoadKeys(ctx))

	// 应该返回错误，因为所有 key 都被限流
	_, err := pool.SelectKey(ctx)
	assert.ErrorIs(t, err, ErrAllKeysRateLimited)
}

func TestAPIKeyPool_RecordUsage(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	key := &LLMProviderAPIKey{
		ProviderID: 1,
		APIKey:     "key1",
		Priority:   10,
		Weight:     100,
		Enabled:    true,
	}

	require.NoError(t, db.Create(key).Error)

	pool := NewAPIKeyPool(db, 1, StrategyPriority, zap.NewNop())
	require.NoError(t, pool.LoadKeys(ctx))

	// 记录成功
	err := pool.RecordSuccess(ctx, key.ID)
	assert.NoError(t, err)

	// 记录失败
	err = pool.RecordFailure(ctx, key.ID, "test error")
	assert.NoError(t, err)

	// 验证统计信息
	time.Sleep(100 * time.Millisecond) // 等待异步更新
	stats := pool.GetStats()
	assert.Contains(t, stats, key.ID)
	assert.Equal(t, int64(2), stats[key.ID].TotalRequests)
	assert.Equal(t, int64(1), stats[key.ID].FailedRequests)
}

func TestLLMProviderAPIKey_IsHealthy(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name string
		key  *LLMProviderAPIKey
		want bool
	}{
		{
			name: "Healthy",
			key: &LLMProviderAPIKey{
				Enabled:        true,
				TotalRequests:  100,
				FailedRequests: 10,
			},
			want: true,
		},
		{
			name: "Disabled",
			key: &LLMProviderAPIKey{
				Enabled: false,
			},
			want: false,
		},
		{
			name: "RPM Limited",
			key: &LLMProviderAPIKey{
				Enabled:      true,
				RateLimitRPM: 10,
				CurrentRPM:   10,
				RPMResetAt:   now.Add(time.Minute),
			},
			want: false,
		},
		{
			name: "High Fail Rate",
			key: &LLMProviderAPIKey{
				Enabled:        true,
				TotalRequests:  100,
				FailedRequests: 60,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.key.IsHealthy()
			assert.Equal(t, tt.want, got)
		})
	}
}
