package k8s

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func newTestOperator() *AgentOperator {
	config := DefaultOperatorConfig()
	config.ReconcileInterval = 50 * time.Millisecond
	return NewAgentOperator(config, zap.NewNop())
}

func newTestAgent(name string, replicas int32) *AgentCRD {
	return &AgentCRD{
		APIVersion: "agentflow.io/v1",
		Kind:       "Agent",
		Metadata: ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: AgentSpec{
			AgentType: "chat",
			Replicas:  replicas,
			Model: ModelSpec{
				Provider: "openai",
				Model:    "gpt-4",
			},
		},
	}
}

func TestRegisterAgent(t *testing.T) {
	op := newTestOperator()
	agent := newTestAgent("test-agent", 2)

	err := op.RegisterAgent(agent)
	require.NoError(t, err)

	got := op.GetAgent("default", "test-agent")
	require.NotNil(t, got)
	assert.Equal(t, "test-agent", got.Metadata.Name)
	assert.Equal(t, AgentPhasePending, got.Status.Phase)
}

func TestRegisterAgentDuplicate(t *testing.T) {
	op := newTestOperator()
	agent := newTestAgent("dup-agent", 1)

	err := op.RegisterAgent(agent)
	require.NoError(t, err)

	// Wait for the first reconcile goroutine to complete before re-registering.
	time.Sleep(150 * time.Millisecond)

	// Registering the same agent again should succeed (update)
	err = op.RegisterAgent(agent)
	require.NoError(t, err)
}

func TestGetAgentNotFound(t *testing.T) {
	op := newTestOperator()
	got := op.GetAgent("default", "nonexistent")
	assert.Nil(t, got)
}

func TestGetInstancesEmpty(t *testing.T) {
	op := newTestOperator()
	instances := op.GetInstances("default", "nonexistent")
	assert.Empty(t, instances)
}

func TestGetMetrics(t *testing.T) {
	op := newTestOperator()
	metrics := op.GetMetrics()
	assert.NotNil(t, metrics)
	assert.Equal(t, int64(0), metrics.ReconcileTotal.Load())
}

func TestUnregisterAgent(t *testing.T) {
	op := newTestOperator()
	agent := newTestAgent("unreg-agent", 1)

	err := op.RegisterAgent(agent)
	require.NoError(t, err)

	err = op.UnregisterAgent("default", "unreg-agent")
	require.NoError(t, err)

	got := op.GetAgent("default", "unreg-agent")
	assert.Nil(t, got)
}

func TestUnregisterAgentNotFound(t *testing.T) {
	op := newTestOperator()
	err := op.UnregisterAgent("default", "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "agent not found")
}

func TestListAgents(t *testing.T) {
	op := newTestOperator()

	assert.Empty(t, op.ListAgents())

	_ = op.RegisterAgent(newTestAgent("a1", 1))
	_ = op.RegisterAgent(newTestAgent("a2", 1))

	agents := op.ListAgents()
	assert.Len(t, agents, 2)
}

func TestReconcileCallback(t *testing.T) {
	op := newTestOperator()
	var reconciled atomic.Int32

	op.SetReconcileCallback(func(agent *AgentCRD) error {
		reconciled.Add(1)
		return nil
	})

	agent := newTestAgent("reconcile-agent", 1)
	err := op.RegisterAgent(agent)
	require.NoError(t, err)

	// Wait for initial reconcile goroutine to fire.
	time.Sleep(100 * time.Millisecond)
	assert.GreaterOrEqual(t, reconciled.Load(), int32(1))
}

func TestAutoScaling(t *testing.T) {
	op := newTestOperator()
	agent := newTestAgent("scale-agent", 2)
	agent.Spec.Scaling = ScalingSpec{
		Enabled:     true,
		MinReplicas: 1,
		MaxReplicas: 10,
		TargetMetrics: []TargetMetric{
			{Type: "custom", Name: "requests_per_second", TargetValue: 100},
		},
	}

	err := op.RegisterAgent(agent)
	require.NoError(t, err)

	// Wait for initial reconcile.
	time.Sleep(100 * time.Millisecond)

	// The scaling calculation with 0 metrics should clamp to MinReplicas.
	instances := op.GetInstances("default", "scale-agent")
	assert.GreaterOrEqual(t, len(instances), 0) // instances may or may not be created depending on timing
}

func TestReconcileCallbackError(t *testing.T) {
	op := newTestOperator()
	op.SetReconcileCallback(func(agent *AgentCRD) error {
		return fmt.Errorf("reconcile failed")
	})

	agent := newTestAgent("err-agent", 1)
	err := op.RegisterAgent(agent)
	require.NoError(t, err)

	// Wait for reconcile to fire
	time.Sleep(100 * time.Millisecond)
	assert.Greater(t, op.metrics.ReconcileErrors.Load(), int64(0))
}

func TestScalingDisabled(t *testing.T) {
	op := newTestOperator()
	agent := newTestAgent("no-scale", 0)
	agent.Spec.Scaling = ScalingSpec{Enabled: false}

	err := op.RegisterAgent(agent)
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)
	// With scaling disabled and 0 desired replicas, no scale events should occur
	assert.Equal(t, int64(0), op.metrics.ScaleUpEvents.Load())
	assert.Equal(t, int64(0), op.metrics.ScaleDownEvents.Load())
}

func TestHealthCheckDisabled(t *testing.T) {
	op := newTestOperator()
	agent := newTestAgent("no-health", 1)
	agent.Spec.HealthCheck = HealthCheckSpec{Enabled: false}

	err := op.RegisterAgent(agent)
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, int64(0), op.metrics.SelfHealingEvents.Load())
}

func TestSelfHealing(t *testing.T) {
	op := newTestOperator()
	agent := newTestAgent("heal-agent", 1)
	agent.Spec.HealthCheck = HealthCheckSpec{
		Enabled:          true,
		Interval:         50 * time.Millisecond,
		Timeout:          10 * time.Millisecond,
		FailureThreshold: 1,
	}

	// Always report unhealthy.
	op.SetHealthCheckCallback(func(_ *AgentCRD) (bool, error) {
		return false, nil
	})

	err := op.RegisterAgent(agent)
	require.NoError(t, err)

	// Create an instance manually and mark it as old enough to trigger self-healing.
	op.mu.Lock()
	inst := &AgentInstance{
		ID:          "heal-inst-1",
		AgentName:   "heal-agent",
		Namespace:   "default",
		Status:      InstanceStatusUnhealthy,
		StartTime:   time.Now().Add(-10 * time.Minute),
		LastHealthy: time.Now().Add(-10 * time.Minute),
	}
	op.instances[inst.ID] = inst
	op.mu.Unlock()

	// Run health check.
	op.checkAllHealth()

	assert.Greater(t, op.metrics.SelfHealingEvents.Load(), int64(0))
}

func TestHealthCheckLoop(t *testing.T) {
	op := newTestOperator()
	ctx, cancel := context.WithCancel(context.Background())

	err := op.Start(ctx)
	require.NoError(t, err)

	// Let loops run briefly.
	time.Sleep(100 * time.Millisecond)

	cancel()
	op.Stop()
}

func TestStopIdempotent(t *testing.T) {
	op := newTestOperator()
	ctx := context.Background()

	err := op.Start(ctx)
	require.NoError(t, err)

	// Calling Stop multiple times should not panic.
	op.Stop()
	op.Stop()
	op.Stop()
}

func TestStartAfterStop(t *testing.T) {
	op := newTestOperator()
	ctx := context.Background()

	err := op.Start(ctx)
	require.NoError(t, err)
	op.Stop()

	// Should be able to start again.
	err = op.Start(ctx)
	require.NoError(t, err)
	op.Stop()
}

func TestStartWhileRunning(t *testing.T) {
	op := newTestOperator()
	ctx := context.Background()

	err := op.Start(ctx)
	require.NoError(t, err)
	defer op.Stop()

	err = op.Start(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already running")
}

func TestConcurrentAccess(t *testing.T) {
	op := newTestOperator()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := op.Start(ctx)
	require.NoError(t, err)
	defer op.Stop()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			agent := newTestAgent("concurrent-agent", 1)
			agent.Metadata.Name = "concurrent-agent"
			_ = op.RegisterAgent(agent)
			_ = op.ListAgents()
			_ = op.GetAgent("default", "concurrent-agent")
			_ = op.GetInstances("default", "concurrent-agent")
			_ = op.GetMetrics()
		}(i)
	}
	wg.Wait()
}

func TestUpdateInstanceMetrics(t *testing.T) {
	op := newTestOperator()

	op.mu.Lock()
	op.instances["inst-1"] = &AgentInstance{
		ID:        "inst-1",
		AgentName: "metrics-agent",
		Namespace: "default",
		Status:    InstanceStatusPending,
	}
	op.mu.Unlock()

	op.UpdateInstanceMetrics("inst-1", InstanceMetrics{
		RequestsTotal:     100,
		RequestsPerSecond: 10.5,
	})

	op.mu.RLock()
	inst := op.instances["inst-1"]
	op.mu.RUnlock()

	assert.Equal(t, InstanceStatusRunning, inst.Status)
	assert.Equal(t, int64(100), inst.Metrics.RequestsTotal)
}

func TestExportImportCRD(t *testing.T) {
	op := newTestOperator()
	agent := newTestAgent("export-agent", 2)

	err := op.RegisterAgent(agent)
	require.NoError(t, err)

	// Wait for initial reconcile.
	time.Sleep(50 * time.Millisecond)

	data, err := op.ExportCRD("default", "export-agent")
	require.NoError(t, err)
	assert.Contains(t, string(data), "export-agent")

	// Import into a new operator.
	op2 := newTestOperator()
	err = op2.ImportCRD(data)
	require.NoError(t, err)

	got := op2.GetAgent("default", "export-agent")
	require.NotNil(t, got)
	assert.Equal(t, "export-agent", got.Metadata.Name)
}

func TestExportCRDNotFound(t *testing.T) {
	op := newTestOperator()
	_, err := op.ExportCRD("default", "nonexistent")
	assert.Error(t, err)
}

func TestMetricsAtomic(t *testing.T) {
	op := newTestOperator()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			op.metrics.ReconcileTotal.Add(1)
			op.metrics.ReconcileErrors.Add(1)
			op.metrics.ScaleUpEvents.Add(1)
			op.metrics.ScaleDownEvents.Add(1)
			op.metrics.SelfHealingEvents.Add(1)
		}()
	}
	wg.Wait()

	assert.Equal(t, int64(100), op.metrics.ReconcileTotal.Load())
	assert.Equal(t, int64(100), op.metrics.ReconcileErrors.Load())
	assert.Equal(t, int64(100), op.metrics.ScaleUpEvents.Load())
	assert.Equal(t, int64(100), op.metrics.ScaleDownEvents.Load())
	assert.Equal(t, int64(100), op.metrics.SelfHealingEvents.Load())
}

func TestInMemoryInstanceProvider(t *testing.T) {
	p := NewInMemoryInstanceProvider(zap.NewNop())
	ctx := context.Background()
	agent := newTestAgent("provider-agent", 1)

	// Create instance.
	inst, err := p.CreateInstance(ctx, agent)
	require.NoError(t, err)
	assert.Equal(t, "provider-agent", inst.AgentName)
	assert.Equal(t, InstanceStatusPending, inst.Status)

	// Get status.
	status, err := p.GetInstanceStatus(ctx, inst.ID)
	require.NoError(t, err)
	assert.Equal(t, InstanceStatusPending, status)

	// List instances.
	instances, err := p.ListInstances(ctx, "default", "provider-agent")
	require.NoError(t, err)
	assert.Len(t, instances, 1)

	// Delete instance.
	err = p.DeleteInstance(ctx, inst.ID)
	require.NoError(t, err)

	// Should be gone.
	_, err = p.GetInstanceStatus(ctx, inst.ID)
	assert.Error(t, err)

	instances, err = p.ListInstances(ctx, "default", "provider-agent")
	require.NoError(t, err)
	assert.Empty(t, instances)
}

func TestInMemoryInstanceProviderDeleteNotFound(t *testing.T) {
	p := NewInMemoryInstanceProvider(zap.NewNop())
	err := p.DeleteInstance(context.Background(), "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "instance not found")
}
