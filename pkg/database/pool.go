package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// =============================================================================
// 🗄️ 数据库连接池管理器
// =============================================================================

// PoolManager 数据库连接池管理器
type PoolManager struct {
	db     *gorm.DB
	sqlDB  *sql.DB
	config PoolConfig
	logger *zap.Logger
	mu     sync.RWMutex
	closed bool
}

// PoolConfig 连接池配置
type PoolConfig struct {
	// 最大空闲连接数
	MaxIdleConns int `yaml:"max_idle_conns" json:"max_idle_conns"`

	// 最大打开连接数
	MaxOpenConns int `yaml:"max_open_conns" json:"max_open_conns"`

	// 连接最大生命周期
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime" json:"conn_max_lifetime"`

	// 连接最大空闲时间
	ConnMaxIdleTime time.Duration `yaml:"conn_max_idle_time" json:"conn_max_idle_time"`

	// 健康检查间隔
	HealthCheckInterval time.Duration `yaml:"health_check_interval" json:"health_check_interval"`
}

// DefaultPoolConfig 返回默认连接池配置
func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		MaxIdleConns:        10,
		MaxOpenConns:        100,
		ConnMaxLifetime:     time.Hour,
		ConnMaxIdleTime:     10 * time.Minute,
		HealthCheckInterval: 30 * time.Second,
	}
}

// NewPoolManager 创建连接池管理器
func NewPoolManager(db *gorm.DB, config PoolConfig, logger *zap.Logger) (*PoolManager, error) {
	if db == nil {
		return nil, fmt.Errorf("db cannot be nil")
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get sql.DB: %w", err)
	}

	// 配置连接池
	sqlDB.SetMaxIdleConns(config.MaxIdleConns)
	sqlDB.SetMaxOpenConns(config.MaxOpenConns)
	sqlDB.SetConnMaxLifetime(config.ConnMaxLifetime)
	sqlDB.SetConnMaxIdleTime(config.ConnMaxIdleTime)

	pm := &PoolManager{
		db:     db,
		sqlDB:  sqlDB,
		config: config,
		logger: logger.With(zap.String("component", "db_pool")),
	}

	// 启动健康检查
	if config.HealthCheckInterval > 0 {
		go pm.healthCheckLoop()
	}

	logger.Info("database pool initialized",
		zap.Int("max_idle_conns", config.MaxIdleConns),
		zap.Int("max_open_conns", config.MaxOpenConns),
		zap.Duration("conn_max_lifetime", config.ConnMaxLifetime),
	)

	return pm, nil
}

// =============================================================================
// 🎯 核心方法
// =============================================================================

// DB 返回 GORM 数据库实例
func (pm *PoolManager) DB() *gorm.DB {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.db
}

// Ping 检查数据库连接
func (pm *PoolManager) Ping(ctx context.Context) error {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	if pm.closed {
		return fmt.Errorf("pool is closed")
	}

	return pm.sqlDB.PingContext(ctx)
}

// Stats 返回连接池统计信息
func (pm *PoolManager) Stats() sql.DBStats {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.sqlDB.Stats()
}

// Close 关闭连接池
func (pm *PoolManager) Close() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.closed {
		return nil
	}

	pm.closed = true
	pm.logger.Info("closing database pool")

	return pm.sqlDB.Close()
}

// =============================================================================
// 🏥 健康检查
// =============================================================================

// healthCheckLoop 健康检查循环
func (pm *PoolManager) healthCheckLoop() {
	ticker := time.NewTicker(pm.config.HealthCheckInterval)
	defer ticker.Stop()

	for range ticker.C {
		pm.mu.RLock()
		if pm.closed {
			pm.mu.RUnlock()
			return
		}
		pm.mu.RUnlock()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := pm.Ping(ctx); err != nil {
			pm.logger.Error("database health check failed", zap.Error(err))
		} else {
			stats := pm.Stats()
			pm.logger.Debug("database health check passed",
				zap.Int("open_connections", stats.OpenConnections),
				zap.Int("in_use", stats.InUse),
				zap.Int("idle", stats.Idle),
			)
		}
		cancel()
	}
}

// =============================================================================
// 📊 统计信息
// =============================================================================

// PoolStats 连接池统计信息（更友好的格式）
type PoolStats struct {
	MaxOpenConnections int           `json:"max_open_connections"`
	OpenConnections    int           `json:"open_connections"`
	InUse              int           `json:"in_use"`
	Idle               int           `json:"idle"`
	WaitCount          int64         `json:"wait_count"`
	WaitDuration       time.Duration `json:"wait_duration"`
	MaxIdleClosed      int64         `json:"max_idle_closed"`
	MaxLifetimeClosed  int64         `json:"max_lifetime_closed"`
}

// GetStats 获取友好格式的统计信息
func (pm *PoolManager) GetStats() PoolStats {
	stats := pm.Stats()
	return PoolStats{
		MaxOpenConnections: stats.MaxOpenConnections,
		OpenConnections:    stats.OpenConnections,
		InUse:              stats.InUse,
		Idle:               stats.Idle,
		WaitCount:          stats.WaitCount,
		WaitDuration:       stats.WaitDuration,
		MaxIdleClosed:      stats.MaxIdleClosed,
		MaxLifetimeClosed:  stats.MaxLifetimeClosed,
	}
}

// =============================================================================
// 🔄 事务管理
// =============================================================================

// TransactionFunc 事务函数类型
type TransactionFunc func(tx *gorm.DB) error

// WithTransaction 在事务中执行函数
func (pm *PoolManager) WithTransaction(ctx context.Context, fn TransactionFunc) error {
	pm.mu.RLock()
	if pm.closed {
		pm.mu.RUnlock()
		return fmt.Errorf("pool is closed")
	}
	db := pm.db
	pm.mu.RUnlock()

	return db.WithContext(ctx).Transaction(fn)
}

// WithTransactionRetry 在事务中执行函数（带重试）
func (pm *PoolManager) WithTransactionRetry(ctx context.Context, maxRetries int, fn TransactionFunc) error {
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		err := pm.WithTransaction(ctx, fn)
		if err == nil {
			return nil
		}

		lastErr = err

		// 检查是否可重试（例如死锁、序列化失败等）
		if !isRetryableError(err) {
			return err
		}

		pm.logger.Warn("transaction failed, retrying",
			zap.Int("attempt", i+1),
			zap.Int("max_retries", maxRetries),
			zap.Error(err),
		)

		// 指数退避
		backoff := time.Duration(1<<uint(i)) * 100 * time.Millisecond
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
	}

	return fmt.Errorf("transaction failed after %d retries: %w", maxRetries, lastErr)
}

// isRetryableError 判断错误是否可重试
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errMsg := strings.ToLower(err.Error())

	// 死锁
	if strings.Contains(errMsg, "deadlock") {
		return true
	}

	// 序列化失败（PostgreSQL SQLSTATE 40001）
	if strings.Contains(errMsg, "serialization failure") || strings.Contains(errMsg, "40001") {
		return true
	}

	// 连接相关错误
	if strings.Contains(errMsg, "connection reset") ||
		strings.Contains(errMsg, "connection refused") ||
		strings.Contains(errMsg, "broken pipe") {
		return true
	}

	// 锁超时
	if strings.Contains(errMsg, "lock timeout") || strings.Contains(errMsg, "lock wait timeout") {
		return true
	}

	// driver: bad connection（Go database/sql 标准错误）
	if strings.Contains(errMsg, "bad connection") {
		return true
	}

	return false
}
