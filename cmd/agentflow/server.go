// Package main provides the AgentFlow server implementation.
package main

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/BaSui01/agentflow/api/handlers"
	"github.com/BaSui01/agentflow/config"
	"github.com/BaSui01/agentflow/internal/metrics"
	"github.com/BaSui01/agentflow/internal/server"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

// =============================================================================
// 🖥️ Server 结构（重构版）
// =============================================================================

// Server 是 AgentFlow 的主服务器
type Server struct {
	cfg        *config.Config
	configPath string
	logger     *zap.Logger

	// 服务器管理器
	httpManager    *server.Manager
	metricsManager *server.Manager

	// Handlers
	healthHandler *handlers.HealthHandler
	// TODO: 添加更多 handlers
	// chatHandler   *handlers.ChatHandler
	// agentHandler  *handlers.AgentHandler

	// 指标收集器
	metricsCollector *metrics.Collector

	// 热更新管理器
	hotReloadManager *config.HotReloadManager
	configAPIHandler *config.ConfigAPIHandler

	wg sync.WaitGroup
}

// NewServer 创建新的服务器实例
func NewServer(cfg *config.Config, configPath string, logger *zap.Logger) *Server {
	return &Server{
		cfg:        cfg,
		configPath: configPath,
		logger:     logger,
	}
}

// =============================================================================
// 🚀 启动流程
// =============================================================================

// Start 启动所有服务
func (s *Server) Start() error {
	// 1. 初始化指标收集器
	s.metricsCollector = metrics.NewCollector("agentflow", s.logger)

	// 2. 初始化 Handlers
	if err := s.initHandlers(); err != nil {
		return fmt.Errorf("failed to init handlers: %w", err)
	}

	// 3. 初始化热更新管理器
	if err := s.initHotReloadManager(); err != nil {
		return fmt.Errorf("failed to init hot reload manager: %w", err)
	}

	// 4. 启动 HTTP 服务器
	if err := s.startHTTPServer(); err != nil {
		return fmt.Errorf("failed to start HTTP server: %w", err)
	}

	// 5. 启动 Metrics 服务器
	if err := s.startMetricsServer(); err != nil {
		return fmt.Errorf("failed to start metrics server: %w", err)
	}

	s.logger.Info("All servers started",
		zap.Int("http_port", s.cfg.Server.HTTPPort),
		zap.Int("metrics_port", s.cfg.Server.MetricsPort),
		zap.Bool("hot_reload_enabled", s.configPath != ""),
	)

	return nil
}

// =============================================================================
// 🔧 初始化方法
// =============================================================================

// initHandlers 初始化所有 handlers
func (s *Server) initHandlers() error {
	// 健康检查 handler
	s.healthHandler = handlers.NewHealthHandler(s.logger)

	// TODO: 初始化其他 handlers
	// 需要先初始化 Provider 和 Registry
	// s.chatHandler = handlers.NewChatHandler(provider, s.logger)
	// s.agentHandler = handlers.NewAgentHandler(registry, s.logger)

	s.logger.Info("Handlers initialized")
	return nil
}

// initHotReloadManager 初始化热更新管理器
func (s *Server) initHotReloadManager() error {
	opts := []config.HotReloadOption{
		config.WithHotReloadLogger(s.logger),
	}

	if s.configPath != "" {
		opts = append(opts, config.WithConfigPath(s.configPath))
	}

	s.hotReloadManager = config.NewHotReloadManager(s.cfg, opts...)

	// 注册配置变更回调
	s.hotReloadManager.OnChange(func(change config.ConfigChange) {
		s.logger.Info("Configuration changed",
			zap.String("path", change.Path),
			zap.String("source", change.Source),
			zap.Bool("requires_restart", change.RequiresRestart),
		)
	})

	// 注册配置重载回调
	s.hotReloadManager.OnReload(func(oldConfig, newConfig *config.Config) {
		s.logger.Info("Configuration reloaded")
		s.cfg = newConfig
	})

	// 启动热更新管理器
	ctx := context.Background()
	if err := s.hotReloadManager.Start(ctx); err != nil {
		return fmt.Errorf("failed to start hot reload manager: %w", err)
	}

	// 创建配置 API 处理器
	s.configAPIHandler = config.NewConfigAPIHandler(s.hotReloadManager)

	return nil
}

// =============================================================================
// 🌐 HTTP 服务器
// =============================================================================

// startHTTPServer 启动 HTTP 服务器（使用新的 handlers）
func (s *Server) startHTTPServer() error {
	mux := http.NewServeMux()

	// ========================================
	// 健康检查端点（使用新的 HealthHandler）
	// ========================================
	mux.HandleFunc("/health", s.healthHandler.HandleHealth)
	mux.HandleFunc("/healthz", s.healthHandler.HandleHealthz)
	mux.HandleFunc("/ready", s.healthHandler.HandleReady)
	mux.HandleFunc("/readyz", s.healthHandler.HandleReady)

	// 版本信息端点
	mux.HandleFunc("/version", s.healthHandler.HandleVersion(Version, BuildTime, GitCommit))

	// ========================================
	// API 路由（TODO: 使用新的 handlers）
	// ========================================
	// mux.HandleFunc("/v1/chat/completions", s.chatHandler.HandleCompletion)
	// mux.HandleFunc("/v1/chat/completions/stream", s.chatHandler.HandleStream)
	// mux.HandleFunc("/v1/agents", s.agentHandler.HandleListAgents)
	// mux.HandleFunc("/v1/agents/execute", s.agentHandler.HandleExecuteAgent)

	// ========================================
	// 配置管理 API
	// ========================================
	if s.configAPIHandler != nil {
		s.configAPIHandler.RegisterRoutes(mux)
		s.logger.Info("Configuration API registered")
	}

	// ========================================
	// 构建中间件链
	// ========================================
	skipAuthPaths := []string{"/health", "/healthz", "/ready", "/readyz", "/version", "/metrics"}
	handler := Chain(mux,
		Recovery(s.logger),
		RequestLogger(s.logger),
		CORS(s.cfg.Server.CORSAllowedOrigins),
		RateLimiter(float64(s.cfg.Server.RateLimitRPS), s.cfg.Server.RateLimitBurst, s.logger),
		APIKeyAuth(s.cfg.Server.APIKeys, skipAuthPaths, s.logger),
	)

	// ========================================
	// 使用 internal/server.Manager
	// ========================================
	serverConfig := server.Config{
		Addr:            fmt.Sprintf(":%d", s.cfg.Server.HTTPPort),
		ReadTimeout:     s.cfg.Server.ReadTimeout,
		WriteTimeout:    s.cfg.Server.WriteTimeout,
		IdleTimeout:     120 * s.cfg.Server.ReadTimeout, // 2x ReadTimeout
		MaxHeaderBytes:  1 << 20,                        // 1 MB
		ShutdownTimeout: s.cfg.Server.ShutdownTimeout,
	}

	s.httpManager = server.NewManager(handler, serverConfig, s.logger)

	// 启动服务器（非阻塞）
	if err := s.httpManager.Start(); err != nil {
		return err
	}

	s.logger.Info("HTTP server started", zap.Int("port", s.cfg.Server.HTTPPort))
	return nil
}

// =============================================================================
// 📊 Metrics 服务器
// =============================================================================

// startMetricsServer 启动 Metrics 服务器
func (s *Server) startMetricsServer() error {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	serverConfig := server.Config{
		Addr:            fmt.Sprintf(":%d", s.cfg.Server.MetricsPort),
		ReadTimeout:     s.cfg.Server.ReadTimeout,
		WriteTimeout:    s.cfg.Server.WriteTimeout,
		ShutdownTimeout: s.cfg.Server.ShutdownTimeout,
	}

	s.metricsManager = server.NewManager(mux, serverConfig, s.logger)

	// 启动服务器（非阻塞）
	if err := s.metricsManager.Start(); err != nil {
		return err
	}

	s.logger.Info("Metrics server started", zap.Int("port", s.cfg.Server.MetricsPort))
	return nil
}

// =============================================================================
// 🛑 关闭流程
// =============================================================================

// WaitForShutdown 等待关闭信号并优雅关闭
func (s *Server) WaitForShutdown() {
	// 使用 httpManager 的 WaitForShutdown（它会监听信号）
	if s.httpManager != nil {
		s.httpManager.WaitForShutdown()
	}

	// 执行清理
	s.Shutdown()
}

// Shutdown 优雅关闭所有服务
func (s *Server) Shutdown() {
	s.logger.Info("Starting graceful shutdown...")

	ctx := context.Background()

	// 1. 停止热更新管理器
	if s.hotReloadManager != nil {
		if err := s.hotReloadManager.Stop(); err != nil {
			s.logger.Error("Hot reload manager shutdown error", zap.Error(err))
		}
	}

	// 2. 关闭 HTTP 服务器
	if s.httpManager != nil {
		if err := s.httpManager.Shutdown(ctx); err != nil {
			s.logger.Error("HTTP server shutdown error", zap.Error(err))
		}
	}

	// 3. 关闭 Metrics 服务器
	if s.metricsManager != nil {
		if err := s.metricsManager.Shutdown(ctx); err != nil {
			s.logger.Error("Metrics server shutdown error", zap.Error(err))
		}
	}

	// 4. 等待所有 goroutine 完成
	s.wg.Wait()

	s.logger.Info("Graceful shutdown completed")
}
