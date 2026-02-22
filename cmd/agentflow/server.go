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
	llmfactory "github.com/BaSui01/agentflow/llm/factory"
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
	chatHandler   *handlers.ChatHandler
	// TODO: agentHandler depends on agent registry, kept as future work
	// agentHandler  *handlers.AgentHandler

	// 指标收集器
	metricsCollector *metrics.Collector

	// 热更新管理器
	hotReloadManager *config.HotReloadManager
	configAPIHandler *config.ConfigAPIHandler

	// Rate limiter 生命周期管理
	rateLimiterCancel context.CancelFunc

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

	// 初始化 LLM Provider（使用工厂函数）
	if s.cfg.LLM.APIKey != "" {
		provider, err := llmfactory.NewProviderFromConfig(s.cfg.LLM.DefaultProvider, llmfactory.ProviderConfig{
			APIKey:  s.cfg.LLM.APIKey,
			BaseURL: s.cfg.LLM.BaseURL,
			Timeout: s.cfg.LLM.Timeout,
		}, s.logger)
		if err != nil {
			s.logger.Warn("Failed to create LLM provider, chat endpoints disabled",
				zap.String("provider", s.cfg.LLM.DefaultProvider),
				zap.Error(err))
		} else {
			s.chatHandler = handlers.NewChatHandler(provider, s.logger)
			s.logger.Info("Chat handler initialized",
				zap.String("provider", s.cfg.LLM.DefaultProvider))
		}
	} else {
		s.logger.Info("LLM API key not configured, chat endpoints disabled")
	}

	// TODO: agentHandler initialization requires agent registry (OP8)

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
	// API 路由
	// ========================================
	if s.chatHandler != nil {
		mux.HandleFunc("/api/v1/chat/completions", s.chatHandler.HandleCompletion)
		mux.HandleFunc("/api/v1/chat/completions/stream", s.chatHandler.HandleStream)
		s.logger.Info("Chat API routes registered")
	}
	// TODO: Agent routes depend on agent registry (OP8)
	// mux.HandleFunc("/api/v1/agents", s.agentHandler.HandleListAgents)
	// mux.HandleFunc("/api/v1/agents/execute", s.agentHandler.HandleExecuteAgent)

	// ========================================
	// 配置管理 API（需要独立认证保护）
	// 安全修复：配置 API 是敏感的管理端点，必须经过认证中间件保护，
	// 不依赖全局中间件链的顺序，而是显式包装认证检查。
	// ========================================
	if s.configAPIHandler != nil {
		configAuth := config.NewConfigAPIMiddleware(s.configAPIHandler, s.getFirstAPIKey())
		mux.HandleFunc("/api/v1/config", configAuth.RequireAuth(s.configAPIHandler.HandleConfig))
		mux.HandleFunc("/api/v1/config/reload", configAuth.RequireAuth(s.configAPIHandler.HandleReload))
		mux.HandleFunc("/api/v1/config/fields", configAuth.RequireAuth(s.configAPIHandler.HandleFields))
		mux.HandleFunc("/api/v1/config/changes", configAuth.RequireAuth(s.configAPIHandler.HandleChanges))
		s.logger.Info("Configuration API registered with authentication")
	}

	// ========================================
	// 构建中间件链
	// ========================================
	skipAuthPaths := []string{"/health", "/healthz", "/ready", "/readyz", "/version", "/metrics"}
	rateLimiterCtx, rateLimiterCancel := context.WithCancel(context.Background())
	s.rateLimiterCancel = rateLimiterCancel
	handler := Chain(mux,
		Recovery(s.logger),
		RequestID(),
		SecurityHeaders(),
		RequestLogger(s.logger),
		CORS(s.cfg.Server.CORSAllowedOrigins),
		RateLimiter(rateLimiterCtx, float64(s.cfg.Server.RateLimitRPS), s.cfg.Server.RateLimitBurst, s.logger),
		APIKeyAuth(s.cfg.Server.APIKeys, skipAuthPaths, s.cfg.Server.AllowQueryAPIKey, s.logger),
	)

	// ========================================
	// 使用 internal/server.Manager
	// ========================================
	serverConfig := server.Config{
		Addr:            fmt.Sprintf(":%d", s.cfg.Server.HTTPPort),
		ReadTimeout:     s.cfg.Server.ReadTimeout,
		WriteTimeout:    s.cfg.Server.WriteTimeout,
		IdleTimeout:     2 * s.cfg.Server.ReadTimeout, // 2x ReadTimeout
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

// getFirstAPIKey 返回配置中的第一个 API Key，用于配置 API 的独立认证。
// 如果未配置任何 API Key，返回空字符串（ConfigAPIMiddleware 会跳过认证检查）。
func (s *Server) getFirstAPIKey() string {
	if len(s.cfg.Server.APIKeys) > 0 {
		return s.cfg.Server.APIKeys[0]
	}
	return ""
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

	// 0. 停止 rate limiter 清理 goroutine
	if s.rateLimiterCancel != nil {
		s.rateLimiterCancel()
	}

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
