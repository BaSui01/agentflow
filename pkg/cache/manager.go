package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/BaSui01/agentflow/pkg/tlsutil"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// =============================================================================
// 💾 缓存管理器
// =============================================================================

// Manager 缓存管理器
type Manager struct {
	redis  *redis.Client
	config Config
	logger *zap.Logger
	mu     sync.RWMutex
	closed bool
}

// Config 缓存配置
type Config struct {
	// Redis 地址
	Addr string `yaml:"addr" json:"addr"`

	// 密码
	Password string `yaml:"password" json:"password"`

	// 数据库编号
	DB int `yaml:"db" json:"db"`

	// 默认过期时间
	DefaultTTL time.Duration `yaml:"default_ttl" json:"default_ttl"`

	// 最大重试次数
	MaxRetries int `yaml:"max_retries" json:"max_retries"`

	// 连接池大小
	PoolSize int `yaml:"pool_size" json:"pool_size"`

	// 最小空闲连接数
	MinIdleConns int `yaml:"min_idle_conns" json:"min_idle_conns"`

	// 是否启用 TLS
	TLSEnabled bool `yaml:"tls_enabled" json:"tls_enabled"`

	// 健康检查间隔
	HealthCheckInterval time.Duration `yaml:"health_check_interval" json:"health_check_interval"`
}

// DefaultConfig 返回默认缓存配置
func DefaultConfig() Config {
	return Config{
		Addr:                "localhost:6379",
		Password:            "",
		DB:                  0,
		DefaultTTL:          5 * time.Minute,
		MaxRetries:          3,
		PoolSize:            10,
		MinIdleConns:        2,
		HealthCheckInterval: 30 * time.Second,
	}
}

// NewManager 创建缓存管理器
func NewManager(config Config, logger *zap.Logger) (*Manager, error) {
	opts := &redis.Options{
		Addr:         config.Addr,
		Password:     config.Password,
		DB:           config.DB,
		MaxRetries:   config.MaxRetries,
		PoolSize:     config.PoolSize,
		MinIdleConns: config.MinIdleConns,
	}
	if config.TLSEnabled {
		opts.TLSConfig = tlsutil.DefaultTLSConfig()
	}
	client := redis.NewClient(opts)

	// 测试连接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	m := &Manager{
		redis:  client,
		config: config,
		logger: logger.With(zap.String("component", "cache")),
	}

	// 启动健康检查
	if config.HealthCheckInterval > 0 {
		go m.healthCheckLoop()
	}

	logger.Info("cache manager initialized",
		zap.String("addr", config.Addr),
		zap.Int("pool_size", config.PoolSize),
	)

	return m, nil
}

// =============================================================================
// 🎯 核心方法
// =============================================================================

// Get 获取缓存值
func (m *Manager) Get(ctx context.Context, key string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return "", fmt.Errorf("cache manager is closed")
	}

	val, err := m.redis.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", ErrCacheMiss
	}
	if err != nil {
		m.logger.Error("cache get failed", zap.String("key", key), zap.Error(err))
		return "", fmt.Errorf("cache get failed: %w", err)
	}

	return val, nil
}

// Set 设置缓存值
func (m *Manager) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return fmt.Errorf("cache manager is closed")
	}

	if ttl == 0 {
		ttl = m.config.DefaultTTL
	}

	err := m.redis.Set(ctx, key, value, ttl).Err()
	if err != nil {
		m.logger.Error("cache set failed", zap.String("key", key), zap.Error(err))
		return fmt.Errorf("cache set failed: %w", err)
	}

	return nil
}

// GetJSON 获取 JSON 缓存值
func (m *Manager) GetJSON(ctx context.Context, key string, dest any) error {
	val, err := m.Get(ctx, key)
	if err != nil {
		return err
	}

	if err := json.Unmarshal([]byte(val), dest); err != nil {
		return fmt.Errorf("failed to unmarshal cache value: %w", err)
	}

	return nil
}

// SetJSON 设置 JSON 缓存值
func (m *Manager) SetJSON(ctx context.Context, key string, value any, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal cache value: %w", err)
	}

	return m.Set(ctx, key, string(data), ttl)
}

// Delete 删除缓存值
func (m *Manager) Delete(ctx context.Context, keys ...string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return fmt.Errorf("cache manager is closed")
	}

	if len(keys) == 0 {
		return nil
	}

	err := m.redis.Del(ctx, keys...).Err()
	if err != nil {
		m.logger.Error("cache delete failed", zap.Strings("keys", keys), zap.Error(err))
		return fmt.Errorf("cache delete failed: %w", err)
	}

	return nil
}

// Exists 检查键是否存在
func (m *Manager) Exists(ctx context.Context, keys ...string) (int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return 0, fmt.Errorf("cache manager is closed")
	}

	count, err := m.redis.Exists(ctx, keys...).Result()
	if err != nil {
		return 0, fmt.Errorf("cache exists check failed: %w", err)
	}

	return count, nil
}

// Expire 设置键的过期时间
func (m *Manager) Expire(ctx context.Context, key string, ttl time.Duration) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return fmt.Errorf("cache manager is closed")
	}

	err := m.redis.Expire(ctx, key, ttl).Err()
	if err != nil {
		return fmt.Errorf("cache expire failed: %w", err)
	}

	return nil
}

// Ping 检查 Redis 连接
func (m *Manager) Ping(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return fmt.Errorf("cache manager is closed")
	}

	return m.redis.Ping(ctx).Err()
}

// Close 关闭缓存管理器
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil
	}

	m.closed = true
	m.logger.Info("closing cache manager")

	return m.redis.Close()
}

// =============================================================================
// 🏥 健康检查
// =============================================================================

// healthCheckLoop 健康检查循环
func (m *Manager) healthCheckLoop() {
	ticker := time.NewTicker(m.config.HealthCheckInterval)
	defer ticker.Stop()

	for range ticker.C {
		m.mu.RLock()
		if m.closed {
			m.mu.RUnlock()
			return
		}
		m.mu.RUnlock()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := m.Ping(ctx); err != nil {
			m.logger.Error("cache health check failed", zap.Error(err))
		} else {
			m.logger.Debug("cache health check passed")
		}
		cancel()
	}
}

// =============================================================================
// 📊 统计信息
// =============================================================================

// Stats 缓存统计信息
type Stats struct {
	Hits        uint64 `json:"hits"`
	Misses      uint64 `json:"misses"`
	Keys        int64  `json:"keys"`
	UsedMemory  int64  `json:"used_memory"`
	MaxMemory   int64  `json:"max_memory"`
	Connections int    `json:"connections"`
}

// GetStats 获取缓存统计信息
func (m *Manager) GetStats(ctx context.Context) (*Stats, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return nil, fmt.Errorf("cache manager is closed")
	}

	_, err := m.redis.Info(ctx, "stats", "memory", "clients").Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get redis info: %w", err)
	}

	// 解析 Redis INFO 输出（简化版）
	stats := &Stats{}

	// TODO: 解析 info 字符串提取统计信息
	// 这里只是示例，实际需要解析 Redis INFO 输出

	return stats, nil
}

// =============================================================================
// 🔧 辅助函数
// =============================================================================

// ErrCacheMiss 缓存未命中错误
var ErrCacheMiss = errors.New("cache miss")

// IsCacheMiss 判断是否为缓存未命中错误
func IsCacheMiss(err error) bool {
	return errors.Is(err, ErrCacheMiss)
}
