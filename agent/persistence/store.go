package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// 常见错误
var (
	ErrNotFound      = errors.New("not found")
	ErrAlreadyExists = errors.New("already exists")
	ErrStoreClosed   = errors.New("store is closed")
	ErrInvalidInput  = errors.New("invalid input")
)

// StoreType 代表存储后端的类型
type StoreType string

const (
	StoreTypeMemory StoreType = "memory"
	StoreTypeFile   StoreType = "file"
	StoreTypeRedis  StoreType = "redis"
)

// RetryConfig 定义消息发送的再试行为
type RetryConfig struct {
	// Max Retries 是重试尝试的最大次数( 默认 3)
	MaxRetries int `json:"max_retries" yaml:"max_retries"`

	// 初始备份是初始备份期限( 默认:1s)
	InitialBackoff time.Duration `json:"initial_backoff" yaml:"initial_backoff"`

	// MaxBackoff 是最大后退持续时间( 默认: 30s)
	MaxBackoff time.Duration `json:"max_backoff" yaml:"max_backoff"`

	// 后置倍数是指数后置的乘数( 默认: 2. 0)
	BackoffMultiplier float64 `json:"backoff_multiplier" yaml:"backoff_multiplier"`
}

// 默认重试Config 返回默认重试配置
// 保守策略:最大3个回推,以指数后置 1s/2s/4s
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:        3,
		InitialBackoff:    1 * time.Second,
		MaxBackoff:        30 * time.Second,
		BackoffMultiplier: 2.0,
	}
}

// 计算 Backoff 计算给定重试的后退持续时间
func (c RetryConfig) CalculateBackoff(attempt int) time.Duration {
	if attempt <= 0 {
		return c.InitialBackoff
	}

	backoff := c.InitialBackoff
	for i := 0; i < attempt; i++ {
		backoff = time.Duration(float64(backoff) * c.BackoffMultiplier)
		if backoff > c.MaxBackoff {
			return c.MaxBackoff
		}
	}
	return backoff
}

// 清理Config 为已完成的任务和旧消息定义清理行为
type CleanupConfig struct {
	// 启用后确定是否启用自动清理
	Enabled bool `json:"enabled" yaml:"enabled"`

	// 间断是清理运行的频率( 默认:1 h)
	Interval time.Duration `json:"interval" yaml:"interval"`

	// 保留信件是保留已确认信件的时间( 默认为 1h)
	MessageRetention time.Duration `json:"message_retention" yaml:"message_retention"`

	// 任务保留是保存已完成任务的时间( 默认: 24h)
	TaskRetention time.Duration `json:"task_retention" yaml:"task_retention"`
}

// 默认CleanupConfig 返回默认清理配置
func DefaultCleanupConfig() CleanupConfig {
	return CleanupConfig{
		Enabled:          true,
		Interval:         1 * time.Hour,
		MessageRetention: 1 * time.Hour,
		TaskRetention:    24 * time.Hour,
	}
}

// StoreConfig 是所有存储执行的基础配置
type StoreConfig struct {
	// 类型是存储后端类型
	Type StoreType `json:"type" yaml:"type"`

	// BaseDir 是基于文件存储的基础目录
	BaseDir string `json:"base_dir" yaml:"base_dir"`

	// Redis 配置( 仅在类型为 “ redis” 时使用)
	Redis RedisStoreConfig `json:"redis" yaml:"redis"`

	// 重试配置
	Retry RetryConfig `json:"retry" yaml:"retry"`

	// 清理配置
	Cleanup CleanupConfig `json:"cleanup" yaml:"cleanup"`
}

// RedisStore Config 包含 Redis 特定配置
type RedisStoreConfig struct {
	// 主机是 Redis 服务器主机
	Host string `json:"host" yaml:"host"`

	// 端口是 Redis 服务器端口
	Port int `json:"port" yaml:"port"`

	// 密码是 Redis 密码( 可选)
	Password string `json:"password" yaml:"password"`

	// DB 为 Redis 数据库编号
	DB int `json:"db" yaml:"db"`

	// PoolSize 是连接池大小
	PoolSize int `json:"pool_size" yaml:"pool_size"`

	// 密钥前缀是所有 Redis 密钥的前缀
	KeyPrefix string `json:"key_prefix" yaml:"key_prefix"`

	// TLSEnabled 是否启用 TLS 连接
	TLSEnabled bool `json:"tls_enabled" yaml:"tls_enabled"`
}

// 默认StoreConfig 返回默认存储配置
func DefaultStoreConfig() StoreConfig {
	return StoreConfig{
		Type:    StoreTypeMemory,
		BaseDir: "./data/persistence",
		Redis: RedisStoreConfig{
			Host:      "localhost",
			Port:      6379,
			DB:        0,
			PoolSize:  10,
			KeyPrefix: "agentflow:",
		},
		Retry:   DefaultRetryConfig(),
		Cleanup: DefaultCleanupConfig(),
	}
}

// 可序列化是可被序列化为JSON的对象的接口
type Serializable interface {
	// 元帅JSON 返回对象的 JSON 编码
	json.Marshaler
	// UnmarshalJSON 解析 JSON 编码数据
	json.Unmarshaler
}

// 存储是所有持久存储的基础界面
type Store interface {
	// 关闭存储并释放资源
	Close() error

	// 平平检查,如果商店是健康的
	Ping(ctx context.Context) error
}
