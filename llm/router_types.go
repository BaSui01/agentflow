package llm

import (
	"context"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// 路线战略
type RoutingStrategy string

const (
	StrategyTagBased    RoutingStrategy = "tag"
	StrategyCostBased   RoutingStrategy = "cost"
	StrategyQPSBased    RoutingStrategy = "qps"
	StrategyHealthBased RoutingStrategy = "health"
	StrategyCanary      RoutingStrategy = "canary"
)

// 路由器是遗留的单一提供路由器(DEPRECATED)
// 在新执行中使用多服务路透社
type Router struct {
	db            *gorm.DB
	providers     map[string]Provider
	healthMonitor *HealthMonitor
	canaryConfig  *CanaryConfig
	logger        *zap.Logger

	healthCheckInterval time.Duration
	healthCheckTimeout  time.Duration
	healthCheckCancel   context.CancelFunc
}

// 路由选项配置路由器
type RouterOptions struct {
	HealthCheckInterval time.Duration
	HealthCheckTimeout  time.Duration
	Logger              *zap.Logger
}

// 提供者选择代表选定的提供者
type ProviderSelection struct {
	Provider     Provider
	ProviderID   uint
	ProviderCode string
	ModelID      uint
	ModelName    string
	IsCanary     bool
	Strategy     RoutingStrategy
}

// NewRouter 创建了遗产路由器( DEPRECATED)
// 使用新多维路透器
func NewRouter(db *gorm.DB, providers map[string]Provider, opts RouterOptions) *Router {
	if opts.Logger == nil {
		opts.Logger = zap.NewNop()
	}
	if opts.HealthCheckInterval <= 0 {
		opts.HealthCheckInterval = 60 * time.Second
	}
	if opts.HealthCheckTimeout <= 0 {
		opts.HealthCheckTimeout = 10 * time.Second
	}

	return &Router{
		db:                  db,
		providers:           providers,
		healthMonitor:       NewHealthMonitor(db),
		canaryConfig:        NewCanaryConfig(db),
		logger:              opts.Logger,
		healthCheckInterval: opts.HealthCheckInterval,
		healthCheckTimeout:  opts.HealthCheckTimeout,
	}
}

// 选择提供者( DEPRECATED - 最小执行)
func (r *Router) SelectProvider(ctx context.Context, req *ChatRequest, strategy RoutingStrategy) (*ProviderSelection, error) {
	// 向后兼容最小执行
	// 真正的逻辑应该使用多功能旋转器
	return nil, &Error{Code: "DEPRECATED", Message: "Use MultiProviderRouter instead"}
}
