package llm

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func TestCanaryConfig_NilDB(t *testing.T) {
	// With nil DB, loadFromDB should be a no-op
	cc := NewCanaryConfig(nil, zap.NewNop())
	t.Cleanup(cc.Stop)

	assert.NotNil(t, cc)
	assert.Empty(t, cc.GetAllDeployments())
}

func TestCanaryConfig_GetSetDeployment_NilDB(t *testing.T) {
	cc := NewCanaryConfig(nil, nil) // nil logger defaults to nop
	t.Cleanup(cc.Stop)

	// No deployment initially
	assert.Nil(t, cc.GetDeployment(1))

	err := cc.SetDeployment(&CanaryDeployment{
		ProviderID:     1,
		CanaryVersion:  "v2",
		StableVersion:  "v1",
		TrafficPercent: 10,
		Stage:          CanaryStage10Pct,
	})
	require.NoError(t, err)

	assert.NotNil(t, cc.GetDeployment(1))
	assert.Len(t, cc.GetAllDeployments(), 1)

	err = cc.UpdateStage(1, CanaryStage50Pct)
	require.NoError(t, err)
	assert.Equal(t, CanaryStage50Pct, cc.GetDeployment(1).Stage)

	err = cc.TriggerRollback(1, "nil-db rollback")
	require.NoError(t, err)
	assert.Equal(t, CanaryStageRollback, cc.GetDeployment(1).Stage)

	cc.RemoveDeployment(1)
	assert.Nil(t, cc.GetDeployment(1))
}

func TestCanaryConfig_SetDeployment_DoesNotMutateMemoryOnDBError(t *testing.T) {
	db := setupBrokenCanaryDB(t)
	cc := NewCanaryConfig(db, zap.NewNop())
	t.Cleanup(cc.Stop)

	require.NoError(t, db.Exec("DROP TABLE sc_llm_canary_deployments").Error)

	deployment := &CanaryDeployment{
		ProviderID:     1,
		CanaryVersion:  "v2",
		StableVersion:  "v1",
		TrafficPercent: 10,
		Stage:          CanaryStage10Pct,
		StartTime:      time.Now(),
	}
	err := cc.SetDeployment(deployment)
	require.Error(t, err)
	assert.Nil(t, cc.GetDeployment(1))
	assert.Zero(t, deployment.ID)
}

func TestCanaryConfig_UpdateStage_DoesNotMutateMemoryOnDBError(t *testing.T) {
	db := setupBrokenCanaryDB(t)
	cc := NewCanaryConfig(db, zap.NewNop())
	t.Cleanup(cc.Stop)

	require.NoError(t, cc.SetDeployment(&CanaryDeployment{
		ProviderID:     1,
		CanaryVersion:  "v2",
		StableVersion:  "v1",
		TrafficPercent: 10,
		Stage:          CanaryStage10Pct,
		StartTime:      time.Now(),
	}))
	require.NoError(t, db.Exec("DROP TABLE sc_llm_canary_deployments").Error)

	err := cc.UpdateStage(1, CanaryStage50Pct)
	require.Error(t, err)
	deployment := cc.GetDeployment(1)
	require.NotNil(t, deployment)
	assert.Equal(t, CanaryStage10Pct, deployment.Stage)
	assert.Equal(t, 10, deployment.TrafficPercent)
}

func TestCanaryConfig_TriggerRollback_DoesNotMutateMemoryOnDBError(t *testing.T) {
	db := setupBrokenCanaryDB(t)
	cc := NewCanaryConfig(db, zap.NewNop())
	t.Cleanup(cc.Stop)

	require.NoError(t, cc.SetDeployment(&CanaryDeployment{
		ProviderID:     1,
		CanaryVersion:  "v2",
		StableVersion:  "v1",
		TrafficPercent: 50,
		Stage:          CanaryStage50Pct,
		StartTime:      time.Now(),
	}))
	require.NoError(t, db.Exec("DROP TABLE sc_audit_logs").Error)

	err := cc.TriggerRollback(1, "db unavailable")
	require.Error(t, err)
	deployment := cc.GetDeployment(1)
	require.NotNil(t, deployment)
	assert.Equal(t, CanaryStage50Pct, deployment.Stage)
	assert.Equal(t, 50, deployment.TrafficPercent)
	assert.Empty(t, deployment.RollbackReason)
}

func TestCanaryConfig_UpdateStage_NotFound(t *testing.T) {
	cc := NewCanaryConfig(nil, zap.NewNop())
	t.Cleanup(cc.Stop)

	err := cc.UpdateStage(999, CanaryStage10Pct)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no canary deployment found")
}

func TestCanaryConfig_TriggerRollback_NotFound(t *testing.T) {
	cc := NewCanaryConfig(nil, zap.NewNop())
	t.Cleanup(cc.Stop)

	err := cc.TriggerRollback(999, "test reason")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no canary deployment found")
}

func TestCanaryConfig_RemoveDeployment(t *testing.T) {
	cc := NewCanaryConfig(nil, zap.NewNop())
	t.Cleanup(cc.Stop)

	// Manually inject a deployment for testing
	cc.mu.Lock()
	cc.deployments[1] = &CanaryDeployment{ID: 1, ProviderID: 1}
	cc.mu.Unlock()

	assert.NotNil(t, cc.GetDeployment(1))
	cc.RemoveDeployment(1)
	assert.Nil(t, cc.GetDeployment(1))
}

func TestCanaryConfig_GetAllDeployments(t *testing.T) {
	cc := NewCanaryConfig(nil, zap.NewNop())
	t.Cleanup(cc.Stop)

	cc.mu.Lock()
	cc.deployments[1] = &CanaryDeployment{ID: 1, ProviderID: 1}
	cc.deployments[2] = &CanaryDeployment{ID: 2, ProviderID: 2}
	cc.mu.Unlock()

	all := cc.GetAllDeployments()
	assert.Len(t, all, 2)
}

func TestCanaryMonitor_NewAndStop(t *testing.T) {
	cc := NewCanaryConfig(nil, zap.NewNop())
	t.Cleanup(cc.Stop)

	monitor := NewCanaryMonitor(nil, cc, nil) // nil logger defaults to nop
	assert.NotNil(t, monitor)
	monitor.Stop() // should not panic
}

func TestCanaryMonitor_GetProviderStats_NilDB(t *testing.T) {
	cc := NewCanaryConfig(nil, zap.NewNop())
	t.Cleanup(cc.Stop)

	monitor := NewCanaryMonitor(nil, cc, zap.NewNop())
	stats := monitor.getProviderStats(1, "demo", time.Minute)
	assert.Equal(t, ProviderStats{}, stats)
}

func TestCanaryStageConstants(t *testing.T) {
	assert.Equal(t, CanaryStage("init"), CanaryStageInit)
	assert.Equal(t, CanaryStage("10pct"), CanaryStage10Pct)
	assert.Equal(t, CanaryStage("50pct"), CanaryStage50Pct)
	assert.Equal(t, CanaryStage("100pct"), CanaryStage100Pct)
	assert.Equal(t, CanaryStage("rollback"), CanaryStageRollback)
}

func setupBrokenCanaryDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE sc_llm_canary_deployments (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		provider_id INTEGER,
		canary_version TEXT,
		stable_version TEXT,
		traffic_percent INTEGER,
		stage TEXT,
		max_error_rate REAL,
		max_latency_p95_ms INTEGER,
		auto_rollback BOOLEAN,
		started_at DATETIME,
		completed_at DATETIME,
		rollback_reason TEXT,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE sc_audit_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		tenant_id INTEGER,
		user_id INTEGER,
		action TEXT,
		resource_type TEXT,
		resource_id TEXT,
		details TEXT,
		created_at DATETIME
	)`).Error)

	return db
}
