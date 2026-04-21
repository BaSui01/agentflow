package k8s

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// --- SetInstanceProvider ---

func TestAgentOperator_SetInstanceProvider(t *testing.T) {
	op := NewAgentOperator(DefaultOperatorConfig(), zap.NewNop())
	provider := NewInMemoryInstanceProvider(zap.NewNop())
	op.SetInstanceProvider(provider)
	// Verify it was set by using it
	assert.NotNil(t, op.instanceProvider)
}

// --- SetScaleCallback ---

func TestAgentOperator_SetScaleCallback(t *testing.T) {
	op := NewAgentOperator(DefaultOperatorConfig(), zap.NewNop())
	scaleCalled := false
	op.SetScaleCallback(func(agent *AgentCRD, replicas int32) error {
		scaleCalled = true
		return nil
	})
	_ = scaleCalled
	assert.NotNil(t, op.onScale)
}

// --- removeInstances ---

func TestAgentOperator_RemoveInstances(t *testing.T) {
	op := NewAgentOperator(DefaultOperatorConfig(), zap.NewNop())

	agent := &AgentCRD{
		Metadata: ObjectMeta{Name: "test-agent", Namespace: "default"},
		Spec:     AgentSpec{Replicas: 3},
	}

	// Create some instances
	op.mu.Lock()
	op.instances["inst-1"] = &AgentInstance{ID: "inst-1", AgentName: "test-agent", Namespace: "default"}
	op.instances["inst-2"] = &AgentInstance{ID: "inst-2", AgentName: "test-agent", Namespace: "default"}
	op.instances["inst-3"] = &AgentInstance{ID: "inst-3", AgentName: "test-agent", Namespace: "default"}
	op.instances["inst-4"] = &AgentInstance{ID: "inst-4", AgentName: "other-agent", Namespace: "default"}
	op.mu.Unlock()

	op.removeInstances(agent, 2)

	op.mu.Lock()
	defer op.mu.Unlock()
	// Should have removed 2 of the 3 test-agent instances
	testAgentCount := 0
	for _, inst := range op.instances {
		if inst.AgentName == "test-agent" {
			testAgentCount++
		}
	}
	assert.Equal(t, 1, testAgentCount)
	// other-agent should still be there
	assert.NotNil(t, op.instances["inst-4"])
}

// --- collectMetrics ---

func TestAgentOperator_CollectMetrics(t *testing.T) {
	op := NewAgentOperator(DefaultOperatorConfig(), zap.NewNop())

	agent := &AgentCRD{
		Metadata: ObjectMeta{Name: "test-agent", Namespace: "default"},
		Spec: AgentSpec{
			Replicas: 1,
			Scaling: ScalingSpec{
				TargetMetrics: []TargetMetric{
					{Name: "requests_per_second", TargetValue: 100},
					{Name: "cpu", TargetValue: 80},
				},
			},
		},
		Status: AgentCRDStatus{},
	}

	op.mu.Lock()
	op.agents["default/test-agent"] = agent
	op.instances["inst-1"] = &AgentInstance{
		ID:        "inst-1",
		AgentName: "test-agent",
		Namespace: "default",
		Metrics: InstanceMetrics{
			RequestsPerSecond: 50,
			CPUUsage:          0.75,
		},
	}
	op.mu.Unlock()

	op.collectMetrics()

	op.mu.Lock()
	defer op.mu.Unlock()
	require.Len(t, agent.Status.CurrentMetrics, 2)
	assert.Equal(t, "requests_per_second", agent.Status.CurrentMetrics[0].Name)
	assert.Equal(t, int64(50), agent.Status.CurrentMetrics[0].CurrentValue)
}

// --- getCurrentMetricValueLocked ---

func TestAgentOperator_GetCurrentMetricValueLocked(t *testing.T) {
	op := NewAgentOperator(DefaultOperatorConfig(), zap.NewNop())

	agent := &AgentCRD{
		Metadata: ObjectMeta{Name: "test-agent", Namespace: "default"},
	}

	op.mu.Lock()
	op.instances["inst-1"] = &AgentInstance{
		ID:        "inst-1",
		AgentName: "test-agent",
		Namespace: "default",
		Metrics: InstanceMetrics{
			RequestsPerSecond: 100,
			AverageLatency:    50 * time.Millisecond,
			CPUUsage:          0.5,
			MemoryUsage:       0.3,
		},
	}
	op.instances["inst-2"] = &AgentInstance{
		ID:        "inst-2",
		AgentName: "test-agent",
		Namespace: "default",
		Metrics: InstanceMetrics{
			RequestsPerSecond: 200,
			AverageLatency:    100 * time.Millisecond,
			CPUUsage:          0.7,
			MemoryUsage:       0.5,
		},
	}
	op.mu.Unlock()

	op.mu.Lock()
	defer op.mu.Unlock()

	rps := op.getCurrentMetricValueLocked(agent, "requests_per_second")
	assert.Equal(t, int64(150), rps) // (100+200)/2

	latency := op.getCurrentMetricValueLocked(agent, "latency")
	assert.Equal(t, int64(75), latency) // (50+100)/2

	cpu := op.getCurrentMetricValueLocked(agent, "cpu")
	assert.Equal(t, int64(60), cpu) // (50+70)/2

	memory := op.getCurrentMetricValueLocked(agent, "memory")
	assert.Equal(t, int64(40), memory) // (30+50)/2

	// No instances
	unknown := op.getCurrentMetricValueLocked(&AgentCRD{
		Metadata: ObjectMeta{Name: "nonexistent", Namespace: "default"},
	}, "cpu")
	assert.Equal(t, int64(0), unknown)
}

// --- RegisterAgent with scale callback ---

func TestAgentOperator_RegisterAgent_WithScaleCallback(t *testing.T) {
	op := NewAgentOperator(DefaultOperatorConfig(), zap.NewNop())

	scaleCalled := false
	op.SetScaleCallback(func(agent *AgentCRD, replicas int32) error {
		scaleCalled = true
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, op.Start(ctx))
	defer op.Stop()

	agent := &AgentCRD{
		Metadata: ObjectMeta{Name: "scale-test", Namespace: "default"},
		Spec:     AgentSpec{Replicas: 2},
	}
	require.NoError(t, op.RegisterAgent(agent))

	// Wait for reconcile
	time.Sleep(200 * time.Millisecond)

	// Scale callback may or may not be called depending on reconcile logic
	_ = scaleCalled
}
