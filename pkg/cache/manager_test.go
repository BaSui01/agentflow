package cache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// =============================================================================
// 🧪 Manager 测试
// =============================================================================

func setupTestRedis(t *testing.T) (*miniredis.Miniredis, *Manager) {
	// 创建 miniredis 实例
	mr, err := miniredis.Run()
	require.NoError(t, err)

	// 创建 Manager
	logger := zap.NewNop()
	config := Config{
		Addr:       mr.Addr(),
		Password:   "",
		DB:         0,
		DefaultTTL: 1 * time.Minute,
	}

	manager, err := NewManager(config, logger)
	require.NoError(t, err)

	return mr, manager
}

func TestNewManager(t *testing.T) {
	mr, manager := setupTestRedis(t)
	defer mr.Close()
	defer manager.Close()

	assert.NotNil(t, manager)
	assert.NotNil(t, manager.redis)
	assert.NotNil(t, manager.logger)
}

func TestManager_SetAndGet(t *testing.T) {
	mr, manager := setupTestRedis(t)
	defer mr.Close()
	defer manager.Close()

	ctx := context.Background()

	// 设置值
	err := manager.Set(ctx, "test-key", "test-value", 1*time.Minute)
	require.NoError(t, err)

	// 获取值
	value, err := manager.Get(ctx, "test-key")
	require.NoError(t, err)
	assert.Equal(t, "test-value", value)
}

func TestManager_GetNonExistent(t *testing.T) {
	mr, manager := setupTestRedis(t)
	defer mr.Close()
	defer manager.Close()

	ctx := context.Background()

	// 获取不存在的键
	value, err := manager.Get(ctx, "non-existent")
	assert.Error(t, err)
	assert.Equal(t, "", value)
}

func TestManager_Delete(t *testing.T) {
	mr, manager := setupTestRedis(t)
	defer mr.Close()
	defer manager.Close()

	ctx := context.Background()

	// 设置值
	err := manager.Set(ctx, "test-key", "test-value", 1*time.Minute)
	require.NoError(t, err)

	// 删除值
	err = manager.Delete(ctx, "test-key")
	require.NoError(t, err)

	// 验证已删除
	_, err = manager.Get(ctx, "test-key")
	assert.Error(t, err)
}

func TestManager_SetJSON(t *testing.T) {
	mr, manager := setupTestRedis(t)
	defer mr.Close()
	defer manager.Close()

	ctx := context.Background()

	type TestData struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	data := TestData{
		Name:  "test",
		Value: 123,
	}

	// 设置 JSON
	err := manager.SetJSON(ctx, "test-json", data, 1*time.Minute)
	require.NoError(t, err)

	// 获取 JSON
	var result TestData
	err = manager.GetJSON(ctx, "test-json", &result)
	require.NoError(t, err)

	assert.Equal(t, data.Name, result.Name)
	assert.Equal(t, data.Value, result.Value)
}

func TestManager_GetJSONNonExistent(t *testing.T) {
	mr, manager := setupTestRedis(t)
	defer mr.Close()
	defer manager.Close()

	ctx := context.Background()

	var result map[string]any
	err := manager.GetJSON(ctx, "non-existent", &result)
	assert.Error(t, err)
}

func TestManager_SetJSONInvalidData(t *testing.T) {
	mr, manager := setupTestRedis(t)
	defer mr.Close()
	defer manager.Close()

	ctx := context.Background()

	// 尝试序列化无法序列化的数据
	invalidData := make(chan int)
	err := manager.SetJSON(ctx, "test-invalid", invalidData, 1*time.Minute)
	assert.Error(t, err)
}

func TestManager_GetJSONInvalidJSON(t *testing.T) {
	mr, manager := setupTestRedis(t)
	defer mr.Close()
	defer manager.Close()

	ctx := context.Background()

	// 设置无效的 JSON 字符串
	err := manager.Set(ctx, "test-invalid-json", "not a json", 1*time.Minute)
	require.NoError(t, err)

	// 尝试获取为 JSON
	var result map[string]any
	err = manager.GetJSON(ctx, "test-invalid-json", &result)
	assert.Error(t, err)
}

func TestManager_TTL(t *testing.T) {
	mr, manager := setupTestRedis(t)
	defer mr.Close()
	defer manager.Close()

	ctx := context.Background()

	// 设置带 TTL 的值
	err := manager.Set(ctx, "test-ttl", "value", 100*time.Millisecond)
	require.NoError(t, err)

	// 立即获取应该成功
	value, err := manager.Get(ctx, "test-ttl")
	require.NoError(t, err)
	assert.Equal(t, "value", value)

	// 快进时间
	mr.FastForward(200 * time.Millisecond)

	// 现在应该过期了
	_, err = manager.Get(ctx, "test-ttl")
	assert.Error(t, err)
}

func TestManager_HealthCheck(t *testing.T) {
	mr, manager := setupTestRedis(t)
	defer mr.Close()
	defer manager.Close()

	ctx := context.Background()

	// Ping 应该成功
	err := manager.Ping(ctx)
	assert.NoError(t, err)
}

func TestManager_HealthCheckFailed(t *testing.T) {
	logger := zap.NewNop()
	config := Config{
		Addr: "localhost:9999", // 不存在的地址
	}

	manager, err := NewManager(config, logger)
	assert.Nil(t, manager)
	assert.Error(t, err)
}

func TestManager_ConcurrentOperations(t *testing.T) {
	mr, manager := setupTestRedis(t)
	defer mr.Close()
	defer manager.Close()

	ctx := context.Background()

	// 并发写入
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			key := "concurrent-" + string(rune('0'+id))
			err := manager.Set(ctx, key, "value", 1*time.Minute)
			assert.NoError(t, err)
			done <- true
		}(i)
	}

	// 等待所有写入完成
	for i := 0; i < 10; i++ {
		<-done
	}

	// 并发读取
	for i := 0; i < 10; i++ {
		go func(id int) {
			key := "concurrent-" + string(rune('0'+id))
			value, err := manager.Get(ctx, key)
			assert.NoError(t, err)
			assert.Equal(t, "value", value)
			done <- true
		}(i)
	}

	// 等待所有读取完成
	for i := 0; i < 10; i++ {
		<-done
	}
}

