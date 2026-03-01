package database

import (
	"context"
	"errors"
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
// isRetryableError tests
// =============================================================================

func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, false},
		{"deadlock", errors.New("ERROR: deadlock detected"), true},
		{"serialization failure", errors.New("serialization failure"), true},
		{"sqlstate 40001", errors.New("pq: could not serialize access due to 40001"), true},
		{"connection reset", errors.New("connection reset by peer"), true},
		{"connection refused", errors.New("dial tcp: connection refused"), true},
		{"broken pipe", errors.New("write: broken pipe"), true},
		{"lock timeout", errors.New("lock timeout exceeded"), true},
		{"lock wait timeout", errors.New("lock wait timeout exceeded"), true},
		{"bad connection", errors.New("driver: bad connection"), true},
		{"generic error", errors.New("some random error"), false},
		{"unique violation", errors.New("duplicate key value violates unique constraint"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, isRetryableError(tt.err))
		})
	}
}

// =============================================================================
// NewPoolManager edge cases
// =============================================================================

func TestNewPoolManager_NilDB(t *testing.T) {
	_, err := NewPoolManager(nil, DefaultPoolConfig(), zap.NewNop())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db cannot be nil")
}

// =============================================================================
// Ping on closed pool
// =============================================================================

func TestPoolManager_Ping_Closed(t *testing.T) {
	mockDB, mock, gormDB := setupTestDB(t)
	defer mockDB.Close()

	pm, err := NewPoolManager(gormDB, PoolConfig{}, zap.NewNop())
	require.NoError(t, err)

	mock.ExpectClose()
	require.NoError(t, pm.Close())

	err = pm.Ping(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pool is closed")
}

// =============================================================================
// Close idempotent
// =============================================================================

func TestPoolManager_Close_Idempotent(t *testing.T) {
	mockDB, mock, gormDB := setupTestDB(t)
	defer mockDB.Close()

	pm, err := NewPoolManager(gormDB, PoolConfig{}, zap.NewNop())
	require.NoError(t, err)

	mock.ExpectClose()
	require.NoError(t, pm.Close())

	// Second close should be no-op (returns nil)
	require.NoError(t, pm.Close())
}

// =============================================================================
// WithTransaction on closed pool
// =============================================================================

func TestPoolManager_WithTransaction_Closed(t *testing.T) {
	mockDB, mock, gormDB := setupTestDB(t)
	defer mockDB.Close()

	pm, err := NewPoolManager(gormDB, PoolConfig{}, zap.NewNop())
	require.NoError(t, err)

	mock.ExpectClose()
	require.NoError(t, pm.Close())

	err = pm.WithTransaction(context.Background(), func(tx *gorm.DB) error {
		return nil
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pool is closed")
}

// =============================================================================
// WithTransactionRetry — success on first try
// =============================================================================

func TestPoolManager_WithTransactionRetry_SuccessFirstTry(t *testing.T) {
	mockDB, mock, gormDB := setupTestDB(t)
	defer mockDB.Close()

	pm, err := NewPoolManager(gormDB, PoolConfig{MaxOpenConns: 10, MaxIdleConns: 5}, zap.NewNop())
	require.NoError(t, err)

	mock.ExpectBegin()
	mock.ExpectCommit()

	callCount := 0
	err = pm.WithTransactionRetry(context.Background(), 3, func(tx *gorm.DB) error {
		callCount++
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, 1, callCount)
}

// =============================================================================
// WithTransactionRetry — non-retryable error stops immediately
// =============================================================================

func TestPoolManager_WithTransactionRetry_NonRetryableError(t *testing.T) {
	mockDB, mock, gormDB := setupTestDB(t)
	defer mockDB.Close()

	pm, err := NewPoolManager(gormDB, PoolConfig{MaxOpenConns: 10, MaxIdleConns: 5}, zap.NewNop())
	require.NoError(t, err)

	mock.ExpectBegin()
	mock.ExpectRollback()

	callCount := 0
	err = pm.WithTransactionRetry(context.Background(), 3, func(tx *gorm.DB) error {
		callCount++
		return errors.New("unique constraint violation")
	})

	require.Error(t, err)
	assert.Equal(t, 1, callCount, "should not retry non-retryable errors")
	assert.Contains(t, err.Error(), "unique constraint violation")
}

// setupTestDBForRetry creates a mock DB suitable for retry tests.
// Uses MonitorPingsOption and MatchExpectationsInOrder(false) for flexibility.
func setupTestDBForRetry(t *testing.T) (sqlmock.Sqlmock, *PoolManager) {
	t.Helper()
	mockDB, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	require.NoError(t, err)
	t.Cleanup(func() { mockDB.Close() })

	mock.MatchExpectationsInOrder(false)

	dialector := postgres.New(postgres.Config{Conn: mockDB})
	mock.ExpectPing()
	gormDB, err := gorm.Open(dialector, &gorm.Config{})
	require.NoError(t, err)

	pm, err := NewPoolManager(gormDB, PoolConfig{MaxOpenConns: 10, MaxIdleConns: 5}, zap.NewNop())
	require.NoError(t, err)

	return mock, pm
}

// =============================================================================
// WithTransactionRetry — retryable error retries then succeeds
// =============================================================================

func TestPoolManager_WithTransactionRetry_RetryThenSuccess(t *testing.T) {
	mock, pm := setupTestDBForRetry(t)

	mock.ExpectBegin()
	mock.ExpectRollback()
	mock.ExpectBegin()
	mock.ExpectCommit()

	callCount := 0
	err := pm.WithTransactionRetry(context.Background(), 3, func(tx *gorm.DB) error {
		callCount++
		if callCount == 1 {
			return errors.New("deadlock detected")
		}
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, 2, callCount)
}

// =============================================================================
// WithTransactionRetry — exhausts all retries
// =============================================================================

func TestPoolManager_WithTransactionRetry_ExhaustsRetries(t *testing.T) {
	mock, pm := setupTestDBForRetry(t)

	mock.ExpectBegin()
	mock.ExpectRollback()

	callCount := 0
	err := pm.WithTransactionRetry(context.Background(), 1, func(tx *gorm.DB) error {
		callCount++
		return errors.New("deadlock detected")
	})

	require.Error(t, err)
	assert.Equal(t, 1, callCount)
	assert.Contains(t, err.Error(), "transaction failed after 1 retries")
	_ = mock
}

// =============================================================================
// WithTransactionRetry — context cancellation during backoff
// =============================================================================

func TestPoolManager_WithTransactionRetry_ContextCancelled(t *testing.T) {
	mock, pm := setupTestDBForRetry(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	mock.ExpectBegin()
	mock.ExpectRollback()

	err := pm.WithTransactionRetry(ctx, 5, func(tx *gorm.DB) error {
		return errors.New("deadlock detected")
	})

	require.Error(t, err)
	_ = mock
}

// =============================================================================
// Stats returns valid data
// =============================================================================

func TestPoolManager_Stats(t *testing.T) {
	mockDB, _, gormDB := setupTestDB(t)
	defer mockDB.Close()

	pm, err := NewPoolManager(gormDB, PoolConfig{
		MaxOpenConns: 50,
		MaxIdleConns: 10,
	}, zap.NewNop())
	require.NoError(t, err)

	stats := pm.Stats()
	assert.GreaterOrEqual(t, stats.MaxOpenConnections, 0)
}

// =============================================================================
// DefaultPoolConfig values
// =============================================================================

func TestDefaultPoolConfig_Values(t *testing.T) {
	cfg := DefaultPoolConfig()
	assert.Equal(t, 10, cfg.MaxIdleConns)
	assert.Equal(t, 100, cfg.MaxOpenConns)
	assert.Equal(t, time.Hour, cfg.ConnMaxLifetime)
	assert.Equal(t, 10*time.Minute, cfg.ConnMaxIdleTime)
	assert.Equal(t, 30*time.Second, cfg.HealthCheckInterval)
}

// =============================================================================
// PoolStats JSON fields
// =============================================================================

func TestPoolStats_Fields(t *testing.T) {
	stats := PoolStats{
		MaxOpenConnections: 100,
		OpenConnections:    10,
		InUse:              5,
		Idle:               5,
		WaitCount:          3,
		WaitDuration:       time.Millisecond * 50,
		MaxIdleClosed:      1,
		MaxLifetimeClosed:  2,
	}
	assert.Equal(t, 100, stats.MaxOpenConnections)
	assert.Equal(t, 10, stats.OpenConnections)
	assert.Equal(t, 5, stats.InUse)
	assert.Equal(t, 5, stats.Idle)
	assert.Equal(t, int64(3), stats.WaitCount)
	assert.Equal(t, int64(1), stats.MaxIdleClosed)
	assert.Equal(t, int64(2), stats.MaxLifetimeClosed)
}

