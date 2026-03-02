package main

import (
	"context"
	"fmt"
	"sync"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/agent/evaluation"
	"github.com/BaSui01/agentflow/agent/memory"
	"github.com/BaSui01/agentflow/api/handlers"
	"github.com/BaSui01/agentflow/config"
	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/budget"
	"github.com/BaSui01/agentflow/llm/cache"
	"github.com/BaSui01/agentflow/llm/observability"
	"github.com/BaSui01/agentflow/llm/tools"
	"github.com/BaSui01/agentflow/pkg/metrics"
	mongoclient "github.com/BaSui01/agentflow/pkg/mongodb"
	"github.com/BaSui01/agentflow/pkg/server"
	pkgservice "github.com/BaSui01/agentflow/pkg/service"
	"github.com/BaSui01/agentflow/pkg/telemetry"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Server 是 AgentFlow 的主服务器
type Server struct {
	cfg        *config.Config
	configPath string
	logger     *zap.Logger

	telemetry *telemetry.Providers
	db        *gorm.DB

	mongoClient *mongoclient.Client

	httpManager     *server.Manager
	metricsManager  *server.Manager
	serviceRegistry *pkgservice.Registry

	healthHandler     *handlers.HealthHandler
	chatHandler       *handlers.ChatHandler
	agentHandler      *handlers.AgentHandler
	apiKeyHandler     *handlers.APIKeyHandler
	ragHandler        *handlers.RAGHandler
	workflowHandler   *handlers.WorkflowHandler
	protocolHandler   *handlers.ProtocolHandler
	multimodalHandler *handlers.MultimodalHandler
	multimodalRedis   *redis.Client

	metricsCollector *metrics.Collector

	hotReloadManager *config.HotReloadManager
	configAPIHandler *config.ConfigAPIHandler

	rateLimiterCancel       context.CancelFunc
	tenantRateLimiterCancel context.CancelFunc

	provider llm.Provider

	budgetManager *budget.TokenBudgetManager
	costTracker   *observability.CostTracker
	llmCache      *cache.MultiLevelCache
	llmMetrics    *observability.Metrics

	resolver *agent.CachingResolver

	auditLogger *tools.DefaultAuditLogger
	abTester    *evaluation.ABTester

	enhancedMemory *memory.EnhancedMemorySystem

	wg sync.WaitGroup
}

func NewServer(cfg *config.Config, configPath string, logger *zap.Logger, tp *telemetry.Providers, db *gorm.DB) *Server {
	return &Server{
		cfg:        cfg,
		configPath: configPath,
		logger:     logger,
		telemetry:  tp,
		db:         db,
	}
}

// Start 启动所有服务
func (s *Server) Start() error {
	s.metricsCollector = metrics.NewCollector("agentflow", s.logger)

	if err := s.initMongoDB(); err != nil {
		return fmt.Errorf("failed to init MongoDB: %w", err)
	}
	if err := s.initHandlers(); err != nil {
		return fmt.Errorf("failed to init handlers: %w", err)
	}
	if err := s.initHotReloadManager(); err != nil {
		return fmt.Errorf("failed to init hot reload manager: %w", err)
	}
	if err := s.startHTTPServer(); err != nil {
		return fmt.Errorf("failed to start HTTP server: %w", err)
	}
	if err := s.startMetricsServer(); err != nil {
		return fmt.Errorf("failed to start metrics server: %w", err)
	}
	if err := s.startLifecycleServices(); err != nil {
		return fmt.Errorf("failed to start lifecycle services: %w", err)
	}

	s.logger.Info("All servers started",
		zap.Int("http_port", s.cfg.Server.HTTPPort),
		zap.Int("metrics_port", s.cfg.Server.MetricsPort),
		zap.Bool("hot_reload_enabled", s.configPath != ""),
	)

	return nil
}
