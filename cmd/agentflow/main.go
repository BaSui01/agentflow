// =============================================================================
// AgentFlow 主入口
// =============================================================================
// 完整服务入口点，包含 HTTP/gRPC 服务、健康检查、Prometheus 指标
//
// 使用方法:
//
//	agentflow serve                       # 启动服务
//	agentflow serve --config config.yaml  # 指定配置文件
//	agentflow version                     # 显示版本信息
//	agentflow health                      # 健康检查
//	agentflow migrate up                  # 运行数据库迁移
//	agentflow migrate down                # 回滚最后一次迁移
//	agentflow migrate status              # 查看迁移状态
// =============================================================================

// @title AgentFlow API
// @version 1.0.0
// @description AgentFlow is a production-ready Go framework for building AI agents with multi-provider LLM support.
// @description
// @description ## Features
// @description - Multi-provider LLM routing (OpenAI, Claude, Gemini, DeepSeek, etc.)
// @description - A2A (Agent-to-Agent) protocol support
// @description - MCP (Model Context Protocol) support
// @description - Streaming responses via SSE
// @description - Health monitoring and metrics

// @contact.name AgentFlow Team
// @contact.url https://github.com/BaSui01/agentflow

// @license.name MIT
// @license.url https://opensource.org/licenses/MIT

// @host localhost:8080
// @BasePath /
// @schemes http https

// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name X-API-Key
// @description API key for authentication

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/BaSui01/agentflow/config"
)

// =============================================================================
// 📦 版本信息（构建时注入）
// =============================================================================

var (
	Version   = "dev"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

// =============================================================================
// 🎯 主函数
// =============================================================================

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "serve":
		runServe(os.Args[2:])
	case "migrate":
		runMigrate(os.Args[2:])
	case "version":
		printVersion()
	case "health":
		runHealthCheck(os.Args[2:])
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

// =============================================================================
// 🖥️ serve 命令
// =============================================================================

func runServe(args []string) {
	// 解析命令行参数
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	configPath := fs.String("config", "", "Path to config file")
	fs.Parse(args)

	// 加载配置
	loader := config.NewLoader()
	if *configPath != "" {
		loader = loader.WithConfigPath(*configPath)
	}

	cfg, err := loader.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// 验证配置
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "Invalid config: %v\n", err)
		os.Exit(1)
	}

	// 初始化日志
	logger := initLogger(cfg.Log)
	defer logger.Sync()

	logger.Info("Starting AgentFlow",
		zap.String("version", Version),
		zap.String("build_time", BuildTime),
		zap.String("git_commit", GitCommit),
	)

	// 创建服务器（传入配置文件路径以支持热更新）
	server := NewServer(cfg, *configPath, logger)

	// 启动服务器
	if err := server.Start(); err != nil {
		logger.Fatal("Failed to start server", zap.Error(err))
	}

	// 等待关闭信号
	server.WaitForShutdown()

	logger.Info("AgentFlow stopped")
}

// =============================================================================
// 🏥 健康检查命令
// =============================================================================

func runHealthCheck(args []string) {
	fs := flag.NewFlagSet("health", flag.ExitOnError)
	addr := fs.String("addr", "http://localhost:8080", "Server address")
	fs.Parse(args)

	resp, err := http.Get(*addr + "/health")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Health check failed: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "Health check failed: status %d\n", resp.StatusCode)
		os.Exit(1)
	}

	fmt.Println("OK")
}

// =============================================================================
// 📋 版本和帮助
// =============================================================================

func printVersion() {
	fmt.Printf("AgentFlow %s\n", Version)
	fmt.Printf("  Build Time: %s\n", BuildTime)
	fmt.Printf("  Git Commit: %s\n", GitCommit)
}

func printUsage() {
	fmt.Println(`AgentFlow - AI Agent Framework

Usage:
  agentflow <command> [options]

Commands:
  serve     Start the AgentFlow server
  migrate   Database migration commands
  version   Show version information
  health    Check server health
  help      Show this help message

Options for 'serve':
  --config <path>   Path to configuration file (YAML)

Migration subcommands:
  migrate up        Apply all pending migrations
  migrate down      Rollback the last migration
  migrate status    Show migration status
  migrate version   Show current migration version
  migrate goto <v>  Migrate to a specific version
  migrate force <v> Force set migration version
  migrate reset     Rollback all migrations

Examples:
  agentflow serve
  agentflow serve --config /etc/agentflow/config.yaml
  agentflow migrate up
  agentflow migrate status
  agentflow health --addr http://localhost:8080
  agentflow version`)
}

// =============================================================================
// 🔧 日志初始化
// =============================================================================

func initLogger(cfg config.LogConfig) *zap.Logger {
	// 解析日志级别
	var level zapcore.Level
	switch cfg.Level {
	case "debug":
		level = zapcore.DebugLevel
	case "info":
		level = zapcore.InfoLevel
	case "warn":
		level = zapcore.WarnLevel
	case "error":
		level = zapcore.ErrorLevel
	default:
		level = zapcore.InfoLevel
	}

	// 配置编码器
	var encoderConfig zapcore.EncoderConfig
	if cfg.Format == "console" {
		encoderConfig = zap.NewDevelopmentEncoderConfig()
		encoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	} else {
		encoderConfig = zap.NewProductionEncoderConfig()
		encoderConfig.TimeKey = "timestamp"
		encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	}

	// 构建配置
	zapConfig := zap.Config{
		Level:            zap.NewAtomicLevelAt(level),
		Development:      cfg.Format == "console",
		Encoding:         cfg.Format,
		EncoderConfig:    encoderConfig,
		OutputPaths:      cfg.OutputPaths,
		ErrorOutputPaths: []string{"stderr"},
	}

	if cfg.Format == "console" {
		zapConfig.Encoding = "console"
	} else {
		zapConfig.Encoding = "json"
	}

	// 构建 logger
	logger, err := zapConfig.Build(
		zap.AddCaller(),
		zap.AddStacktrace(zapcore.ErrorLevel),
	)
	if err != nil {
		// 回退到基本 logger
		logger, _ = zap.NewProduction()
	}

	return logger
}

// =============================================================================
// 🖥️ Server 结构
// =============================================================================

// Server 是 AgentFlow 的主服务器
type Server struct {
	cfg        *config.Config
	configPath string
	logger     *zap.Logger

	httpServer    *http.Server
	metricsServer *http.Server

	// Hot reload manager
	hotReloadManager *config.HotReloadManager
	configAPIHandler *config.ConfigAPIHandler

	shutdownChan chan struct{}
	wg           sync.WaitGroup
}

// NewServer 创建新的服务器实例
func NewServer(cfg *config.Config, configPath string, logger *zap.Logger) *Server {
	return &Server{
		cfg:          cfg,
		configPath:   configPath,
		logger:       logger,
		shutdownChan: make(chan struct{}),
	}
}

// Start 启动所有服务
func (s *Server) Start() error {
	// 初始化热更新管理器
	if err := s.initHotReloadManager(); err != nil {
		return fmt.Errorf("failed to init hot reload manager: %w", err)
	}

	// 启动 HTTP 服务器
	if err := s.startHTTPServer(); err != nil {
		return fmt.Errorf("failed to start HTTP server: %w", err)
	}

	// 启动 Metrics 服务器
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
		// 更新服务器配置引用
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

// startHTTPServer 启动 HTTP 服务器
func (s *Server) startHTTPServer() error {
	mux := http.NewServeMux()

	// 健康检查端点
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/ready", s.handleReady)
	mux.HandleFunc("/readyz", s.handleReady)

	// 版本信息端点
	mux.HandleFunc("/version", s.handleVersion)

	// API 路由（占位符，后续扩展）
	mux.HandleFunc("/api/v1/agents", s.handleAgents)

	// 配置管理 API
	if s.configAPIHandler != nil {
		s.configAPIHandler.RegisterRoutes(mux)
		s.logger.Info("Configuration API registered",
			zap.String("get_config", "GET /api/v1/config"),
			zap.String("update_config", "PUT /api/v1/config"),
			zap.String("reload_config", "POST /api/v1/config/reload"),
			zap.String("get_fields", "GET /api/v1/config/fields"),
			zap.String("get_changes", "GET /api/v1/config/changes"),
		)
	}

	// 构建中间件链
	skipAuthPaths := []string{"/health", "/healthz", "/ready", "/readyz", "/version", "/metrics"}
	handler := Chain(mux,
		Recovery(s.logger),
		RequestLogger(s.logger),
		CORS(s.cfg.Server.CORSAllowedOrigins),
		RateLimiter(float64(s.cfg.Server.RateLimitRPS), s.cfg.Server.RateLimitBurst, s.logger),
		APIKeyAuth(s.cfg.Server.APIKeys, skipAuthPaths, s.logger),
	)

	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.cfg.Server.HTTPPort),
		Handler:      handler,
		ReadTimeout:  s.cfg.Server.ReadTimeout,
		WriteTimeout: s.cfg.Server.WriteTimeout,
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.logger.Info("HTTP server starting", zap.Int("port", s.cfg.Server.HTTPPort))
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("HTTP server error", zap.Error(err))
		}
	}()

	return nil
}

// startMetricsServer 启动 Metrics 服务器
func (s *Server) startMetricsServer() error {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	s.metricsServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.cfg.Server.MetricsPort),
		Handler: mux,
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.logger.Info("Metrics server starting", zap.Int("port", s.cfg.Server.MetricsPort))
		if err := s.metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("Metrics server error", zap.Error(err))
		}
	}()

	return nil
}

// WaitForShutdown 等待关闭信号并优雅关闭
func (s *Server) WaitForShutdown() {
	// 监听系统信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 等待信号
	sig := <-sigChan
	s.logger.Info("Received shutdown signal", zap.String("signal", sig.String()))

	// 开始优雅关闭
	s.Shutdown()
}

// Shutdown 优雅关闭所有服务
func (s *Server) Shutdown() {
	s.logger.Info("Starting graceful shutdown...")

	ctx, cancel := context.WithTimeout(context.Background(), s.cfg.Server.ShutdownTimeout)
	defer cancel()

	// 停止热更新管理器
	if s.hotReloadManager != nil {
		if err := s.hotReloadManager.Stop(); err != nil {
			s.logger.Error("Hot reload manager shutdown error", zap.Error(err))
		}
	}

	// 关闭 HTTP 服务器
	if s.httpServer != nil {
		if err := s.httpServer.Shutdown(ctx); err != nil {
			s.logger.Error("HTTP server shutdown error", zap.Error(err))
		}
	}

	// 关闭 Metrics 服务器
	if s.metricsServer != nil {
		if err := s.metricsServer.Shutdown(ctx); err != nil {
			s.logger.Error("Metrics server shutdown error", zap.Error(err))
		}
	}

	// 等待所有 goroutine 完成
	s.wg.Wait()

	close(s.shutdownChan)
	s.logger.Info("Graceful shutdown completed")
}

// =============================================================================
// 🌐 HTTP 处理器
// =============================================================================

// HealthResponse 健康检查响应
// @Description 健康检查响应结构
type HealthResponse struct {
	Status    string `json:"status" example:"healthy"`    // 服务状态
	Timestamp string `json:"timestamp" example:"2024-01-01T00:00:00Z"` // 时间戳
	Version   string `json:"version" example:"1.0.0"`     // 版本号
}

// handleHealth 处理健康检查请求
// @Summary 健康检查
// @Description 返回服务的健康状态
// @Tags Health
// @Accept json
// @Produce json
// @Success 200 {object} HealthResponse "服务健康"
// @Failure 503 {object} HealthResponse "服务不健康"
// @Router /health [get]
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	resp := HealthResponse{
		Status:    "healthy",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Version:   Version,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// handleReady 处理就绪检查请求
// @Summary 就绪检查
// @Description 返回服务是否准备好接受请求
// @Tags Health
// @Accept json
// @Produce json
// @Success 200 {object} HealthResponse "服务就绪"
// @Failure 503 {object} HealthResponse "服务未就绪"
// @Router /ready [get]
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	// NOTE: 依赖服务健康检查将在 v2.0 中实现，当前仅检查进程状态
	resp := HealthResponse{
		Status:    "ready",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Version:   Version,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// VersionResponse 版本信息响应
// @Description 版本信息响应结构
type VersionResponse struct {
	Version   string `json:"version" example:"1.0.0"`      // 版本号
	BuildTime string `json:"build_time" example:"2024-01-01T00:00:00Z"` // 构建时间
	GitCommit string `json:"git_commit" example:"abc1234"` // Git 提交哈希
}

// handleVersion 处理版本信息请求
// @Summary 获取版本信息
// @Description 返回服务的版本信息
// @Tags Health
// @Accept json
// @Produce json
// @Success 200 {object} VersionResponse "版本信息"
// @Router /version [get]
func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	resp := VersionResponse{
		Version:   Version,
		BuildTime: BuildTime,
		GitCommit: GitCommit,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// AgentListResponse Agent 列表响应
// @Description Agent 列表响应结构
type AgentListResponse struct {
	Agents []interface{} `json:"agents"` // Agent 列表
	Total  int           `json:"total"`  // 总数
}

// ErrorResponse 错误响应
// @Description 错误响应结构
type ErrorResponse struct {
	Error string `json:"error" example:"not implemented"` // 错误信息
}

// handleAgents 处理 Agent API 请求
// @Summary 获取 Agent 列表
// @Description 返回所有 Agent 的列表
// @Tags Agents
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {object} AgentListResponse "Agent 列表"
// @Failure 401 {object} ErrorResponse "未授权"
// @Failure 501 {object} ErrorResponse "未实现"
// @Router /api/v1/agents [get]
func (s *Server) handleAgents(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// NOTE: Agent 列表功能依赖运行时 registry，将在 CLI 完善时实现
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"agents": []interface{}{},
			"total":  0,
		})
	case http.MethodPost:
		// NOTE: Agent 创建功能依赖运行时 registry，将在 CLI 完善时实现
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotImplemented)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "not implemented",
		})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
