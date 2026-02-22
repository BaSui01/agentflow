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
// @version 1.0.0
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
	"go.uber.org/zap/zapcore"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/BaSui01/agentflow/config"
	"github.com/BaSui01/agentflow/internal/telemetry"
	"github.com/BaSui01/agentflow/llm"
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

	// Initialize OpenTelemetry
	otelProviders, err := telemetry.Init(cfg.Telemetry, logger)
	if err != nil {
		logger.Warn("failed to initialize telemetry", zap.Error(err))
	}

	// 初始化数据库连接
	db, err := openDatabase(cfg.Database, logger)
	if err != nil {
		logger.Warn("Database not available, API key management disabled", zap.Error(err))
	} else {
		// AutoMigrate 确保表结构最新（包括新增的 base_url 列）
		if migrateErr := llm.InitDatabase(db); migrateErr != nil {
			logger.Error("Database auto-migrate failed", zap.Error(migrateErr))
		}
	}

	// 创建服务器（传入配置文件路径以支持热更新）
	server := NewServer(cfg, *configPath, logger, otelProviders, db)

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

// openDatabase 根据配置打开数据库连接
func openDatabase(dbCfg config.DatabaseConfig, logger *zap.Logger) (*gorm.DB, error) {
	if dbCfg.Driver == "" {
		return nil, fmt.Errorf("database driver not configured")
	}

	var dialector gorm.Dialector
	switch dbCfg.Driver {
	case "postgres":
		dialector = postgres.Open(dbCfg.DSN())
	default:
		return nil, fmt.Errorf("unsupported database driver: %s (supported: postgres)", dbCfg.Driver)
	}

	db, err := gorm.Open(dialector, &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect database: %w", err)
	}

	logger.Info("Database connected", zap.String("driver", dbCfg.Driver))
	return db, nil
}
