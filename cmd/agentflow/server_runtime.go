package main

import (
	"fmt"
	"sync"

	"github.com/BaSui01/agentflow/config"
	"github.com/BaSui01/agentflow/pkg/metrics"
	"github.com/BaSui01/agentflow/pkg/telemetry"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Server 是 AgentFlow 的主服务器
type Server struct {
	cfg        *config.Config
	configPath string
	logger     *zap.Logger

	infra    serverInfraBundle
	ops      serverOpsBundle
	handlers serverHandlerBundle
	text     serverTextRuntimeBundle
	tooling  serverToolingBundle
	workflow serverWorkflowBundle

	wg sync.WaitGroup
}

func NewServer(cfg *config.Config, configPath string, logger *zap.Logger, tp *telemetry.Providers, db *gorm.DB) *Server {
	return &Server{
		cfg:        cfg,
		configPath: configPath,
		logger:     logger,
		infra: serverInfraBundle{
			telemetry: tp,
			db:        db,
		},
	}
}

// Start 启动所有服务
func (s *Server) Start() error {
	s.ops.metricsCollector = metrics.NewCollector("agentflow", s.logger)

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

	s.logStartupSummary()

	s.logger.Info("All servers started",
		zap.Int("http_port", s.cfg.Server.HTTPPort),
		zap.Int("metrics_port", s.cfg.Server.MetricsPort),
		zap.String("metrics_bind_address", s.cfg.Server.MetricsBindAddress),
		zap.Bool("pprof_enabled", s.cfg.Server.EnablePProf),
		zap.Bool("hot_reload_enabled", s.configPath != ""),
	)

	return nil
}
