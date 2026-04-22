package tools

import (
	"context"
	"testing"

	"github.com/BaSui01/agentflow/agent/execution/protocol/a2a"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// --- Composer: RegisterResourceRequirement, GetDependencies, GetExclusiveGroups, ClearDependencies, ClearExclusiveGroups ---

func TestCapabilityComposer_ResourceRequirement(t *testing.T) {
	registry := newCovTestRegistry(t)
	matcher := newCovTestMatcher(registry)
	composer := NewCapabilityComposer(registry, matcher, nil, nil)

	composer.RegisterResourceRequirement(&ResourceRequirement{
		CapabilityName:     "gpu_task",
		CPUCores:           4,
		MemoryMB:           8192,
		GPURequired:        true,
		ExclusiveResources: []string{"gpu0"},
	})

	composer.RegisterResourceRequirement(&ResourceRequirement{
		CapabilityName:     "gpu_task2",
		ExclusiveResources: []string{"gpu0"},
	})

	conflicts, err := composer.DetectConflicts(context.Background(), []string{"gpu_task", "gpu_task2"})
	require.NoError(t, err)
	assert.NotEmpty(t, conflicts)
	assert.Equal(t, ConflictTypeResource, conflicts[0].Type)
}

func TestCapabilityComposer_GetDependencies(t *testing.T) {
	composer := NewCapabilityComposer(newCovTestRegistry(t), newCovTestMatcher(nil), nil, nil)

	t.Run("no deps", func(t *testing.T) {
		deps := composer.GetDependencies("nonexistent")
		assert.Nil(t, deps)
	})

	t.Run("with deps", func(t *testing.T) {
		composer.RegisterDependency("capA", []string{"capB", "capC"})
		deps := composer.GetDependencies("capA")
		assert.Equal(t, []string{"capB", "capC"}, deps)
	})
}
func TestCapabilityComposer_GetExclusiveGroups(t *testing.T) {
	composer := NewCapabilityComposer(newCovTestRegistry(t), newCovTestMatcher(nil), nil, nil)

	t.Run("empty", func(t *testing.T) {
		groups := composer.GetExclusiveGroups()
		assert.Empty(t, groups)
	})

	t.Run("with groups", func(t *testing.T) {
		composer.RegisterExclusiveGroup([]string{"a", "b"})
		groups := composer.GetExclusiveGroups()
		assert.Len(t, groups, 1)
		assert.Equal(t, []string{"a", "b"}, groups[0])
	})
}

func TestCapabilityComposer_ClearDependencies(t *testing.T) {
	composer := NewCapabilityComposer(newCovTestRegistry(t), newCovTestMatcher(nil), nil, nil)
	composer.RegisterDependency("a", []string{"b"})
	assert.NotNil(t, composer.GetDependencies("a"))

	composer.ClearDependencies()
	assert.Nil(t, composer.GetDependencies("a"))
}

func TestCapabilityComposer_ClearExclusiveGroups(t *testing.T) {
	composer := NewCapabilityComposer(newCovTestRegistry(t), newCovTestMatcher(nil), nil, nil)
	composer.RegisterExclusiveGroup([]string{"a", "b"})
	assert.NotEmpty(t, composer.GetExclusiveGroups())

	composer.ClearExclusiveGroups()
	assert.Empty(t, composer.GetExclusiveGroups())
}

func TestCapabilityComposer_Contains(t *testing.T) {
	composer := NewCapabilityComposer(newCovTestRegistry(t), newCovTestMatcher(nil), nil, nil)

	assert.True(t, composer.contains([]string{"A", "B"}, "a"))
	assert.True(t, composer.contains([]string{"A", "B"}, "B"))
	assert.False(t, composer.contains([]string{"A", "B"}, "C"))
	assert.False(t, composer.contains([]string{}, "A"))
}

func TestCapabilityComposer_CountCapabilitiesForAgent(t *testing.T) {
	composer := NewCapabilityComposer(newCovTestRegistry(t), newCovTestMatcher(nil), nil, nil)

	capMap := map[string]string{
		"cap1": "agent1",
		"cap2": "agent1",
		"cap3": "agent2",
	}
	assert.Equal(t, 2, composer.countCapabilitiesForAgent(capMap, "agent1"))
	assert.Equal(t, 1, composer.countCapabilitiesForAgent(capMap, "agent2"))
	assert.Equal(t, 0, composer.countCapabilitiesForAgent(capMap, "agent3"))
}

func TestCapabilityComposer_SelectBestCapability(t *testing.T) {
	composer := NewCapabilityComposer(newCovTestRegistry(t), newCovTestMatcher(nil), nil, nil)

	t.Run("empty", func(t *testing.T) {
		result := composer.selectBestCapability(nil)
		assert.Nil(t, result)
	})

	t.Run("sorts by score then load", func(t *testing.T) {
		caps := []CapabilityInfo{
			{AgentID: "a1", Score: 80, Load: 0.5},
			{AgentID: "a2", Score: 90, Load: 0.8},
			{AgentID: "a3", Score: 90, Load: 0.2},
		}
		best := composer.selectBestCapability(caps)
		assert.Equal(t, "a3", best.AgentID) // highest score, lowest load
	})
}

func TestCapabilityComposer_HasCircularDependency(t *testing.T) {
	composer := NewCapabilityComposer(newCovTestRegistry(t), newCovTestMatcher(nil), nil, nil)

	t.Run("no cycle", func(t *testing.T) {
		composer.RegisterDependency("a", []string{"b"})
		composer.RegisterDependency("b", []string{"c"})
		assert.False(t, composer.hasCircularDependency("a", make(map[string]bool)))
	})

	t.Run("with cycle", func(t *testing.T) {
		composer.ClearDependencies()
		composer.RegisterDependency("a", []string{"b"})
		composer.RegisterDependency("b", []string{"a"})
		assert.True(t, composer.hasCircularDependency("a", make(map[string]bool)))
	})
}

// --- Integration: FindAgentsForTask, reportLoads ---

func TestAgentDiscoveryIntegration_FindAgentsForTask(t *testing.T) {
	logger := zap.NewNop()
	service := NewDiscoveryService(nil, logger)
	integration := NewAgentDiscoveryIntegration(service, nil, logger)

	ctx := context.Background()

	// Register an agent
	agent := &mockCapabilityProvider{
		id:   "agent1",
		name: "Agent 1",
		caps: []a2a.Capability{
			{Name: "search", Description: "search", Type: a2a.CapabilityTypeQuery},
		},
	}
	require.NoError(t, integration.RegisterAgent(ctx, agent))

	results, err := integration.FindAgentsForTask(ctx, &MatchRequest{
		RequiredCapabilities: []string{"search"},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, results)
}

// --- mockCapabilityProvider ---

type mockCapabilityProvider struct {
	id   string
	name string
	caps []a2a.Capability
	card *a2a.AgentCard
}

func (m *mockCapabilityProvider) ID() string                        { return m.id }
func (m *mockCapabilityProvider) Name() string                      { return m.name }
func (m *mockCapabilityProvider) GetCapabilities() []a2a.Capability { return m.caps }
func (m *mockCapabilityProvider) GetAgentCard() *a2a.AgentCard      { return m.card }

// --- helpers ---

func newCovTestRegistry(t *testing.T) *CapabilityRegistry {
	t.Helper()
	config := DefaultRegistryConfig()
	config.EnableHealthCheck = false
	return NewCapabilityRegistry(config, zap.NewNop())
}

func newCovTestMatcher(registry Registry) *CapabilityMatcher {
	return NewCapabilityMatcher(registry, nil, zap.NewNop())
}
