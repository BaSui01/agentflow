// =============================================================================
// AgentFlow 主入口
// =============================================================================
// 完整服务入口点，包含 HTTP 服务、健康检查、Prometheus 指标
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
// @version 1.8.11
// @description AgentFlow is a production-ready Go framework for building AI agents with multi-provider LLM support.
// @description
// @description ## Features
// @description - Multi-provider LLM routing (OpenAI, Claude, Gemini, DeepSeek, etc.)
// @description - Runtime config management API (hot reload, history, rollback)
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
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"go.uber.org/zap"

	"github.com/BaSui01/agentflow/internal/app/bootstrap"
)

// =============================================================================
// 📦 版本信息（构建时注入）
// =============================================================================

var (
	Version   = "1.8.11"
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
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse serve flags: %v\n", err)
		os.Exit(1)
	}

	runtime, err := bootstrap.InitializeServeRuntime(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid config: %v\n", err)
		os.Exit(1)
	}

	// 初始化日志
	logger := runtime.Logger
	defer func() {
		_ = logger.Sync()
	}()

	logger.Info("Starting AgentFlow",
		zap.String("version", Version),
		zap.String("build_time", BuildTime),
		zap.String("git_commit", GitCommit),
	)

	// 创建服务器（传入配置文件路径以支持热更新）
	server := NewServer(runtime.Config, *configPath, logger, runtime.Telemetry, runtime.DB)

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
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse health flags: %v\n", err)
		os.Exit(1)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(*addr + "/health")
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
