package router

import (
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

// Router 是 MultiProviderRouter 的基础结构体，提供 DB、健康监控等基础设施。
type Router struct {
	db            *gorm.DB
	providers     map[string]Provider
	healthMonitor *HealthMonitor
	canaryConfig  *CanaryConfig
	logger        *zap.Logger

	healthCheckInterval time.Duration
	healthCheckTimeout  time.Duration
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

// NewRouter 创建基础路由器（仅供 MultiProviderRouter 内部使用）
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
		canaryConfig:        NewCanaryConfig(db, opts.Logger),
		logger:              opts.Logger,
		healthCheckInterval: opts.HealthCheckInterval,
		healthCheckTimeout:  opts.HealthCheckTimeout,
	}
}
