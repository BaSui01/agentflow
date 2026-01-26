package llm

import (
	"context"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// RoutingStrategy defines the routing strategy
type RoutingStrategy string

const (
	StrategyTagBased    RoutingStrategy = "tag"
	StrategyCostBased   RoutingStrategy = "cost"
	StrategyQPSBased    RoutingStrategy = "qps"
	StrategyHealthBased RoutingStrategy = "health"
	StrategyCanary      RoutingStrategy = "canary"
)

// Router is the legacy single-provider router (DEPRECATED)
// Use MultiProviderRouter for new implementations
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

// RouterOptions configures the router
type RouterOptions struct {
	HealthCheckInterval time.Duration
	HealthCheckTimeout  time.Duration
	Logger              *zap.Logger
}

// ProviderSelection represents a selected provider
type ProviderSelection struct {
	Provider     Provider
	ProviderID   uint
	ProviderCode string
	ModelID      uint
	ModelName    string
	IsCanary     bool
	Strategy     RoutingStrategy
}

// NewRouter creates a legacy router (DEPRECATED)
// Use NewMultiProviderRouter instead
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

// SelectProvider selects a provider (DEPRECATED - minimal implementation)
func (r *Router) SelectProvider(ctx context.Context, req *ChatRequest, strategy RoutingStrategy) (*ProviderSelection, error) {
	// Minimal implementation for backward compatibility
	// Real logic should use MultiProviderRouter
	return nil, &Error{Code: "DEPRECATED", Message: "Use MultiProviderRouter instead"}
}
