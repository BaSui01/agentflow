package tools

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	toolregistry "github.com/BaSui01/agentflow/agent/capabilities/tools/registry"
	"go.uber.org/zap"
)

// 能力登记是书记官处接口的默认执行。
// 它为特工提供内存,并提供能力信息
// 支持健康检查和事件通知。
type CapabilityRegistry struct {
	mu sync.RWMutex

	// 特工用身份证储存注册探员
	agents map[string]*AgentInfo

	// 能力 按名称索引快速检索的能力。
	capabilityIndex *toolregistry.CapabilityIndex[*CapabilityInfo]

	// 事件 Handlers 存储事件处理器。
	eventHandlers map[string]DiscoveryEventHandler
	handlerMu     sync.RWMutex

	// 健康检查员定期进行健康检查。
	healthChecker *HealthChecker

	// 配置包含注册配置 。
	config *RegistryConfig

	// store is an optional persistence backend. When non-nil, agent data is
	// also persisted through this store. When nil, the registry operates
	// purely in-memory.
	store RegistryStore

	// logger 是日志实例 。
	logger *zap.Logger

	// 信号关闭了
	done      chan struct{}
	closeOnce sync.Once

	// subscriptionCounter 原子计数器，用于生成唯一订阅 ID
	subscriptionCounter atomic.Uint64

	// panicErrChan 可选，handler panic 时写入，供调用方消费
	panicErrChan chan<- error
}

// 登记册Config拥有能力登记册的配置。
type RegistryConfig struct {
	// 健康检查Interval是健康检查的间隔.
	HealthCheckInterval time.Duration `json:"health_check_interval"`

	// 健康检查 暂停是健康检查的暂停。
	HealthCheckTimeout time.Duration `json:"health_check_timeout"`

	// 体质不健康 阈值是指在标记不健康之前,健康检查失败的次数.
	UnhealthyThreshold int `json:"unhealthy_threshold"`

	// 移除Unhealty 之后是清除不健康剂的期限。
	RemoveUnhealthyAfter time.Duration `json:"remove_unhealthy_after"`

	// 启用健康检查可以定期进行健康检查。
	EnableHealthCheck bool `json:"enable_health_check"`

	// 默认能力分数是新能力的默认分数.
	DefaultCapabilityScore float64 `json:"default_capability_score"`
}

// 默认 RegistryConfig 返回带有合理默认的注册Config 。
func DefaultRegistryConfig() *RegistryConfig {
	return &RegistryConfig{
		HealthCheckInterval:    30 * time.Second,
		HealthCheckTimeout:     5 * time.Second,
		UnhealthyThreshold:     3,
		RemoveUnhealthyAfter:   5 * time.Minute,
		EnableHealthCheck:      true,
		DefaultCapabilityScore: 50.0,
	}
}

// RegistryOption configures a CapabilityRegistry.
type RegistryOption func(*CapabilityRegistry)

// WithStore sets a persistence backend for the registry.
// When set, agent data is persisted through the store in addition to the
// in-memory map. When not set, the registry operates purely in-memory.
func WithStore(store RegistryStore) RegistryOption {
	return func(r *CapabilityRegistry) {
		r.store = store
	}
}

// WithPanicErrorChan 设置 handler panic 时写入的 error channel，供调用方消费
func WithPanicErrorChan(ch chan<- error) RegistryOption {
	return func(r *CapabilityRegistry) {
		r.panicErrChan = ch
	}
}

// SetStore sets the persistence backend after construction.
// This is useful when the store is not available at construction time
// (e.g., when MongoDB stores are initialized after the registry).
func (r *CapabilityRegistry) SetStore(store RegistryStore) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.store = store
}

// 新能力登记系统建立了一个新的能力登记册。
func NewCapabilityRegistry(config *RegistryConfig, logger *zap.Logger, opts ...RegistryOption) *CapabilityRegistry {
	if config == nil {
		config = DefaultRegistryConfig()
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	r := &CapabilityRegistry{
		agents:          make(map[string]*AgentInfo),
		capabilityIndex: toolregistry.NewCapabilityIndex[*CapabilityInfo](),
		eventHandlers:   make(map[string]DiscoveryEventHandler),
		config:          config,
		logger:          logger.With(zap.String("component", "capability_registry")),
		done:            make(chan struct{}),
	}

	// Apply options
	for _, opt := range opts {
		opt(r)
	}

	// 如果启用, 初始化健康检查器
	if config.EnableHealthCheck {
		r.healthChecker = NewHealthChecker(&HealthCheckerConfig{
			Interval:           config.HealthCheckInterval,
			Timeout:            config.HealthCheckTimeout,
			UnhealthyThreshold: config.UnhealthyThreshold,
		}, r, logger)
	}

	return r
}

// 启动登记册背景进程。
func (r *CapabilityRegistry) Start(ctx context.Context) error {
	if r.healthChecker != nil {
		if err := r.healthChecker.Start(ctx); err != nil {
			return fmt.Errorf("failed to start health checker: %w", err)
		}
	}

	r.logger.Info("capability registry started")
	return nil
}

// 代理人对具有其能力的代理人进行登记。
