package bootstrap

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/BaSui01/agentflow/config"
	"github.com/BaSui01/agentflow/pkg/telemetry"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// ServeRuntime holds dependencies required by cmd/agentflow serve runtime.
type ServeRuntime struct {
	Config    *config.Config
	Logger    *zap.Logger
	Telemetry *telemetry.Providers
	DB        *gorm.DB
}

// LoadAndValidateConfig loads application config from defaults, file, and env,
// then validates the final result.
func LoadAndValidateConfig(configPath string) (*config.Config, error) {
	if configPath == "" {
		cfg, err := config.LoadFromEnv()
		if err != nil {
			return nil, fmt.Errorf("failed to load config: %w", err)
		}
		if err := cfg.Validate(); err != nil {
			return nil, fmt.Errorf("invalid config: %w", err)
		}
		return cfg, nil
	}

	cfg, err := config.NewLoader().
		WithConfigPath(configPath).
		WithEnvPrefix("AGENTFLOW").
		WithValidator(func(c *config.Config) error { return c.Validate() }).
		Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}
	return cfg, nil
}

// NewLogger creates the application logger from config.
// It always returns a usable logger; on build failure it falls back to stderr JSON logger.
func NewLogger(cfg config.LogConfig) *zap.Logger {
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

	var encoderConfig zapcore.EncoderConfig
	if cfg.Format == "console" {
		encoderConfig = zap.NewDevelopmentEncoderConfig()
		encoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	} else {
		encoderConfig = zap.NewProductionEncoderConfig()
		encoderConfig.TimeKey = "timestamp"
		encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	}

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

	logger, err := zapConfig.Build(
		zap.AddCaller(),
		zap.AddStacktrace(zapcore.ErrorLevel),
	)
	if err != nil {
		fallbackCore := zapcore.NewCore(
			zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
			zapcore.Lock(os.Stderr),
			zap.NewAtomicLevelAt(level),
		)
		logger = zap.New(
			fallbackCore,
			zap.AddCaller(),
			zap.AddStacktrace(zapcore.ErrorLevel),
		)
		logger.Warn("failed to initialize configured logger, using stderr fallback", zap.Error(err))
	}

	return logger
}

// OpenDatabase opens database connection based on config.
func OpenDatabase(dbCfg config.DatabaseConfig, logger *zap.Logger) (*gorm.DB, error) {
	if dbCfg.Driver == "" {
		return nil, fmt.Errorf("database driver not configured")
	}

	var dialector gorm.Dialector
	switch dbCfg.Driver {
	case "postgres":
		dialector = postgres.Open(dbCfg.DSN())
	case "mysql":
		dialector = mysql.Open(dbCfg.DSN())
	case "sqlite":
		dialector = sqlite.Open(dbCfg.DSN())
	default:
		return nil, fmt.Errorf("unsupported database driver: %s (supported: postgres, mysql, sqlite)", dbCfg.Driver)
	}

	db, err := gorm.Open(dialector, &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to access sql database handle: %w", err)
	}
	applyDatabasePoolConfig(sqlDB, dbCfg)

	pingCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(pingCtx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	logger.Info("Database connected",
		zap.String("driver", dbCfg.Driver),
		zap.Int("max_open_conns", dbCfg.MaxOpenConns),
		zap.Int("max_idle_conns", dbCfg.MaxIdleConns),
		zap.Duration("conn_max_lifetime", dbCfg.ConnMaxLifetime),
	)
	return db, nil
}

func applyDatabasePoolConfig(sqlDB interface {
	SetMaxOpenConns(int)
	SetMaxIdleConns(int)
	SetConnMaxLifetime(time.Duration)
}, dbCfg config.DatabaseConfig) {
	if dbCfg.MaxOpenConns > 0 {
		sqlDB.SetMaxOpenConns(dbCfg.MaxOpenConns)
	}
	if dbCfg.MaxIdleConns > 0 {
		sqlDB.SetMaxIdleConns(dbCfg.MaxIdleConns)
	}
	if dbCfg.ConnMaxLifetime > 0 {
		sqlDB.SetConnMaxLifetime(dbCfg.ConnMaxLifetime)
	}
}

// InitializeServeRuntime centralizes startup bootstrapping for the serve command:
// config loading/validation, logger creation, telemetry init, and DB connection.
func InitializeServeRuntime(configPath string) (*ServeRuntime, error) {
	cfg, err := LoadAndValidateConfig(configPath)
	if err != nil {
		return nil, err
	}

	logger := NewLogger(cfg.Log)

	otelProviders, err := telemetry.Init(cfg.Telemetry, logger)
	if err != nil {
		logger.Warn("failed to initialize telemetry", zap.Error(err))
	}

	db, err := OpenDatabase(cfg.Database, logger)
	if err != nil {
		return nil, fmt.Errorf("database is required for serve startup: %w", err)
	}

	return &ServeRuntime{
		Config:    cfg,
		Logger:    logger,
		Telemetry: otelProviders,
		DB:        db,
	}, nil
}
