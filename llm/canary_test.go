package llm

import (
	"testing"

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

	// We can't call SetDeployment with nil DB (it would panic on db.Table)
	// but we can test GetAllDeployments and RemoveDeployment
	assert.Empty(t, cc.GetAllDeployments())

	cc.RemoveDeployment(1) // no-op, should not panic
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

func TestCanaryStageConstants(t *testing.T) {
	assert.Equal(t, CanaryStage("init"), CanaryStageInit)
	assert.Equal(t, CanaryStage("10pct"), CanaryStage10Pct)
	assert.Equal(t, CanaryStage("50pct"), CanaryStage50Pct)
	assert.Equal(t, CanaryStage("100pct"), CanaryStage100Pct)
	assert.Equal(t, CanaryStage("rollback"), CanaryStageRollback)
}

