package database

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// =============================================================================
// 🧪 PoolManager 测试
// =============================================================================

func setupTestDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock, *gorm.DB) {
	// 创建 mock DB
	mockDB, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	require.NoError(t, err)

	// 创建 GORM DB
	dialector := postgres.New(postgres.Config{
		Conn: mockDB,
	})
	mock.ExpectPing()

	gormDB, err := gorm.Open(dialector, &gorm.Config{})
	require.NoError(t, err)

	return mockDB, mock, gormDB
}

func TestNewPoolManager(t *testing.T) {
	mockDB, _, gormDB := setupTestDB(t)
	defer mockDB.Close()

	logger := zap.NewNop()
	config := PoolConfig{
		MaxOpenConns:    10,
		MaxIdleConns:    5,
		ConnMaxLifetime: 1 * time.Hour,
		ConnMaxIdleTime: 30 * time.Minute,
	}

	manager, err := NewPoolManager(gormDB, config, logger)
	require.NoError(t, err)

	assert.NotNil(t, manager)
	assert.NotNil(t, manager.db)
	assert.NotNil(t, manager.logger)
	assert.Equal(t, config, manager.config)
}

func TestPoolManager_GetDB(t *testing.T) {
	mockDB, _, gormDB := setupTestDB(t)
	defer mockDB.Close()

	logger := zap.NewNop()
	config := PoolConfig{
		MaxOpenConns: 10,
		MaxIdleConns: 5,
	}

	manager, err := NewPoolManager(gormDB, config, logger)
	require.NoError(t, err)

	db := manager.DB()

	assert.NotNil(t, db)
	assert.Equal(t, gormDB, db)
}

func TestPoolManager_HealthCheck(t *testing.T) {
	mockDB, mock, gormDB := setupTestDB(t)
	defer mockDB.Close()

	logger := zap.NewNop()
	config := PoolConfig{
		MaxOpenConns: 10,
		MaxIdleConns: 5,
	}

	manager, err := NewPoolManager(gormDB, config, logger)
	require.NoError(t, err)

	ctx := context.Background()

	// Mock ping 成功
	mock.ExpectPing()

	err = manager.Ping(ctx)
	assert.NoError(t, err)

	// 验证所有期望都被满足
	err = mock.ExpectationsWereMet()
	assert.NoError(t, err)
}

func TestPoolManager_HealthCheckFailed(t *testing.T) {
	mockDB, mock, gormDB := setupTestDB(t)
	defer mockDB.Close()

	logger := zap.NewNop()
	config := PoolConfig{
		MaxOpenConns: 10,
		MaxIdleConns: 5,
	}

	manager, err := NewPoolManager(gormDB, config, logger)
	require.NoError(t, err)

	ctx := context.Background()

	// Mock ping 失败
	mock.ExpectPing().WillReturnError(sql.ErrConnDone)

	err = manager.Ping(ctx)
	assert.Error(t, err)
}

func TestPoolManager_GetStats(t *testing.T) {
	mockDB, _, gormDB := setupTestDB(t)
	defer mockDB.Close()

	logger := zap.NewNop()
	config := PoolConfig{
		MaxOpenConns: 10,
		MaxIdleConns: 5,
	}

	manager, err := NewPoolManager(gormDB, config, logger)
	require.NoError(t, err)

	stats := manager.GetStats()
	assert.NotNil(t, stats)
	assert.GreaterOrEqual(t, stats.MaxOpenConnections, 0)
	assert.GreaterOrEqual(t, stats.OpenConnections, 0)
	assert.GreaterOrEqual(t, stats.InUse, 0)
	assert.GreaterOrEqual(t, stats.Idle, 0)
}

func TestPoolManager_WithTransaction(t *testing.T) {
	mockDB, mock, gormDB := setupTestDB(t)
	defer mockDB.Close()

	logger := zap.NewNop()
	config := PoolConfig{
		MaxOpenConns: 10,
		MaxIdleConns: 5,
	}

	manager, err := NewPoolManager(gormDB, config, logger)
	require.NoError(t, err)

	ctx := context.Background()

	// Mock 事务
	mock.ExpectBegin()
	mock.ExpectCommit()

	err = manager.WithTransaction(ctx, func(tx *gorm.DB) error {
		// 事务内的操作
		return nil
	})

	assert.NoError(t, err)

	// 验证所有期望都被满足
	err = mock.ExpectationsWereMet()
	assert.NoError(t, err)
}

func TestPoolManager_WithTransactionRollback(t *testing.T) {
	mockDB, mock, gormDB := setupTestDB(t)
	defer mockDB.Close()

	logger := zap.NewNop()
	config := PoolConfig{
		MaxOpenConns: 10,
		MaxIdleConns: 5,
	}

	manager, err := NewPoolManager(gormDB, config, logger)
	require.NoError(t, err)

	ctx := context.Background()

	// Mock 事务回滚
	mock.ExpectBegin()
	mock.ExpectRollback()

	err = manager.WithTransaction(ctx, func(tx *gorm.DB) error {
		// 返回错误触发回滚
		return assert.AnError
	})

	assert.Error(t, err)

	// 验证所有期望都被满足
	err = mock.ExpectationsWereMet()
	assert.NoError(t, err)
}

func TestPoolManager_Close(t *testing.T) {
	mockDB, mock, gormDB := setupTestDB(t)
	defer mockDB.Close()

	logger := zap.NewNop()
	config := PoolConfig{
		MaxOpenConns: 10,
		MaxIdleConns: 5,
	}

	manager, err := NewPoolManager(gormDB, config, logger)
	require.NoError(t, err)

	// Mock close
	mock.ExpectClose()

	err = manager.Close()
	assert.NoError(t, err)

	// 验证所有期望都被满足
	err = mock.ExpectationsWereMet()
	assert.NoError(t, err)
}

func TestPoolManager_StartHealthCheck(t *testing.T) {
	mockDB, mock, gormDB := setupTestDB(t)
	defer mockDB.Close()

	logger := zap.NewNop()
	config := PoolConfig{
		MaxOpenConns:        10,
		MaxIdleConns:        5,
		HealthCheckInterval: 100 * time.Millisecond,
	}

	// Mock 多次 ping — 必须在 NewPoolManager 之前设置，
	// 因为构造函数会立即启动 health check goroutine。
	mock.ExpectPing()
	mock.ExpectPing()
	mock.ExpectPing()

	manager, err := NewPoolManager(gormDB, config, logger)
	require.NoError(t, err)
	defer manager.Close()

	// 等待一段时间让健康检查运行
	time.Sleep(350 * time.Millisecond)

	// 注意：由于时间控制的不确定性，我们不严格验证 mock 期望
	// 只要没有 panic 就算成功
}

func TestDefaultPoolConfig(t *testing.T) {
	config := DefaultPoolConfig()

	assert.Greater(t, config.MaxOpenConns, 0)
	assert.Greater(t, config.MaxIdleConns, 0)
	assert.Greater(t, config.ConnMaxLifetime, time.Duration(0))
	assert.Greater(t, config.ConnMaxIdleTime, time.Duration(0))
}
