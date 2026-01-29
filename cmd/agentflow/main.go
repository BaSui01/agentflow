// =============================================================================
// 🚀 AgentFlow 主入口
// =============================================================================
// 完整服务入口点，包含 HTTP/gRPC 服务、健康检查、Prometheus 指标
//
// 使用方法:
//
//	agentflow serve                    # 启动服务
//	agentflow serve --config config.yaml  # 指定配置文件
//	agentflow version                  # 显示版本信息
//	agentflow health                   # 健康检查
// =============================================================================
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

	// 创建服务器
	server := NewServer(cfg, logger)

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
  version   Show version information
  health    Check server health
  help      Show this help message

Options for 'serve':
  --config <path>   Path to configuration file (YAML)

Examples:
  agentflow serve
  agentflow serve --config /etc/agentflow/config.yaml
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
	cfg    *config.Config
	logger *zap.Logger

	httpServer    *http.Server
	metricsServer *http.Server

	shutdownChan chan struct{}
	wg           sync.WaitGroup
}

// NewServer 创建新的服务器实例
func NewServer(cfg *config.Config, logger *zap.Logger) *Server {
	return &Server{
		cfg:          cfg,
		logger:       logger,
		shutdownChan: make(chan struct{}),
	}
}

// Start 启动所有服务
func (s *Server) Start() error {
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
	)

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

	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.cfg.Server.HTTPPort),
		Handler:      mux,
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
type HealthResponse struct {
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
	Version   string `json:"version"`
}

// handleHealth 处理健康检查请求
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
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	// TODO: 检查依赖服务（Redis、PostgreSQL、Qdrant）的连接状态
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
type VersionResponse struct {
	Version   string `json:"version"`
	BuildTime string `json:"build_time"`
	GitCommit string `json:"git_commit"`
}

// handleVersion 处理版本信息请求
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

// handleAgents 处理 Agent API 请求（占位符）
func (s *Server) handleAgents(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// TODO: 列出所有 Agent
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"agents": []interface{}{},
			"total":  0,
		})
	case http.MethodPost:
		// TODO: 创建新 Agent
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotImplemented)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "not implemented",
		})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
