package llm

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
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
