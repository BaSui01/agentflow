package idempotency

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// IdempotencyKey 幂等键结构
// 用于缓存相同请求的响应，避免重复处理
type IdempotencyKey struct {
	Key       string          // 幂等键（SHA256 hash）
	Result    json.RawMessage // 缓存的响应结果
	ExpiresAt time.Time       // 过期时间
}

// Manager 幂等性管理器接口
// 提供幂等键的生成、存储和查询能力
type Manager interface {
	// GenerateKey 根据输入生成幂等键
	GenerateKey(inputs ...any) (string, error)

	// Get 获取缓存的结果
	Get(ctx context.Context, key string) (json.RawMessage, bool, error)

	// Set 设置缓存结果
	Set(ctx context.Context, key string, result any, ttl time.Duration) error

	// Delete 删除缓存
	Delete(ctx context.Context, key string) error

	// Exists 检查幂等键是否存在
	Exists(ctx context.Context, key string) (bool, error)
}

// redisManager 基于 Redis 的幂等性管理器实现
type redisManager struct {
	redis  *redis.Client
	prefix string // Redis key 前缀
	logger *zap.Logger
}

// NewRedisManager 创建基于 Redis 的幂等性管理器
func NewRedisManager(redis *redis.Client, prefix string, logger *zap.Logger) Manager {
	if prefix == "" {
		prefix = "idempotency:"
	}

	return &redisManager{
		redis:  redis,
		prefix: prefix,
		logger: logger,
	}
}

// GenerateKey 实现 Manager.GenerateKey
// 使用 SHA256 生成幂等键，确保相同输入生成相同的键
func (m *redisManager) GenerateKey(inputs ...any) (string, error) {
	if len(inputs) == 0 {
		return "", errors.New("至少需要一个输入参数")
	}

	// 将输入序列化为 JSON
	data, err := json.Marshal(inputs)
	if err != nil {
		return "", fmt.Errorf("序列化输入失败: %w", err)
	}

	// 计算 SHA256 哈希
	hash := sha256.Sum256(data)
	key := hex.EncodeToString(hash[:])

	m.logger.Debug("生成幂等键",
		zap.String("key", key),
		zap.Int("inputs_count", len(inputs)),
	)

	return key, nil
}

// Get 实现 Manager.Get
func (m *redisManager) Get(ctx context.Context, key string) (json.RawMessage, bool, error) {
	redisKey := m.prefix + key

	// 从 Redis 获取
	data, err := m.redis.Get(ctx, redisKey).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			// 键不存在
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("从 Redis 获取失败: %w", err)
	}

	m.logger.Debug("幂等键命中",
		zap.String("key", key),
		zap.Int("data_size", len(data)),
	)

	return data, true, nil
}

// Set 实现 Manager.Set
func (m *redisManager) Set(ctx context.Context, key string, result any, ttl time.Duration) error {
	redisKey := m.prefix + key

	// 序列化结果
	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("序列化结果失败: %w", err)
	}

	// 存储到 Redis，设置 TTL
	if ttl <= 0 {
		ttl = 1 * time.Hour // 默认 1 小时
	}

	err = m.redis.Set(ctx, redisKey, data, ttl).Err()
	if err != nil {
		return fmt.Errorf("存储到 Redis 失败: %w", err)
	}

	m.logger.Debug("幂等键已存储",
		zap.String("key", key),
		zap.Duration("ttl", ttl),
		zap.Int("data_size", len(data)),
	)

	return nil
}

// Delete 实现 Manager.Delete
func (m *redisManager) Delete(ctx context.Context, key string) error {
	redisKey := m.prefix + key

	err := m.redis.Del(ctx, redisKey).Err()
	if err != nil {
		return fmt.Errorf("从 Redis 删除失败: %w", err)
	}

	m.logger.Debug("幂等键已删除",
		zap.String("key", key),
	)

	return nil
}

// Exists 实现 Manager.Exists
func (m *redisManager) Exists(ctx context.Context, key string) (bool, error) {
	redisKey := m.prefix + key

	count, err := m.redis.Exists(ctx, redisKey).Result()
	if err != nil {
		return false, fmt.Errorf("检查 Redis 键失败: %w", err)
	}

	return count > 0, nil
}

// memoryManager 基于内存的幂等性管理器实现（用于测试）
type memoryManager struct {
	cache           map[string]*cacheEntry
	mu              sync.RWMutex
	logger          *zap.Logger
	stopCh          chan struct{}
	cleanupInterval time.Duration
}

type cacheEntry struct {
	Data      json.RawMessage
	ExpiresAt time.Time
}

// NewMemoryManager 创建基于内存的幂等性管理器（仅用于测试）
func NewMemoryManager(logger *zap.Logger) Manager {
	m := &memoryManager{
		cache:           make(map[string]*cacheEntry),
		logger:          logger,
		stopCh:          make(chan struct{}),
		cleanupInterval: 5 * time.Minute,
	}
	// 启动后台清理 goroutine
	go m.cleanupLoop()
	return m
}

// NewMemoryManagerWithCleanup 创建带自定义清理间隔的内存管理器
func NewMemoryManagerWithCleanup(logger *zap.Logger, cleanupInterval time.Duration) Manager {
	m := &memoryManager{
		cache:           make(map[string]*cacheEntry),
		logger:          logger,
		stopCh:          make(chan struct{}),
		cleanupInterval: cleanupInterval,
	}
	go m.cleanupLoop()
	return m
}

// cleanupLoop 定期清理过期条目
func (m *memoryManager) cleanupLoop() {
	ticker := time.NewTicker(m.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.cleanup()
		case <-m.stopCh:
			return
		}
	}
}

// cleanup 清理所有过期条目
func (m *memoryManager) cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	expired := 0
	for key, entry := range m.cache {
		if now.After(entry.ExpiresAt) {
			delete(m.cache, key)
			expired++
		}
	}

	if expired > 0 && m.logger != nil {
		m.logger.Debug("cleaned up expired idempotency entries",
			zap.Int("expired", expired),
			zap.Int("remaining", len(m.cache)))
	}
}

// Close 停止清理 goroutine
func (m *memoryManager) Close() {
	close(m.stopCh)
}

// GenerateKey 实现 Manager.GenerateKey（同 redisManager）
func (m *memoryManager) GenerateKey(inputs ...any) (string, error) {
	if len(inputs) == 0 {
		return "", errors.New("至少需要一个输入参数")
	}

	data, err := json.Marshal(inputs)
	if err != nil {
		return "", fmt.Errorf("序列化输入失败: %w", err)
	}

	hash := sha256.Sum256(data)
	key := hex.EncodeToString(hash[:])

	return key, nil
}

// Get 实现 Manager.Get
func (m *memoryManager) Get(ctx context.Context, key string) (json.RawMessage, bool, error) {
	m.mu.RLock()
	entry, exists := m.cache[key]
	m.mu.RUnlock()

	if !exists {
		return nil, false, nil
	}

	// 检查是否过期
	if time.Now().After(entry.ExpiresAt) {
		m.mu.Lock()
		delete(m.cache, key)
		m.mu.Unlock()
		return nil, false, nil
	}

	return entry.Data, true, nil
}

// Set 实现 Manager.Set
func (m *memoryManager) Set(ctx context.Context, key string, result any, ttl time.Duration) error {
	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("序列化结果失败: %w", err)
	}

	if ttl <= 0 {
		ttl = 1 * time.Hour
	}

	m.mu.Lock()
	m.cache[key] = &cacheEntry{
		Data:      data,
		ExpiresAt: time.Now().Add(ttl),
	}
	m.mu.Unlock()

	return nil
}

// Delete 实现 Manager.Delete
func (m *memoryManager) Delete(ctx context.Context, key string) error {
	m.mu.Lock()
	delete(m.cache, key)
	m.mu.Unlock()
	return nil
}

// Exists 实现 Manager.Exists
func (m *memoryManager) Exists(ctx context.Context, key string) (bool, error) {
	m.mu.RLock()
	entry, exists := m.cache[key]
	m.mu.RUnlock()

	if !exists {
		return false, nil
	}

	// 检查是否过期
	if time.Now().After(entry.ExpiresAt) {
		m.mu.Lock()
		delete(m.cache, key)
		m.mu.Unlock()
		return false, nil
	}

	return true, nil
}
