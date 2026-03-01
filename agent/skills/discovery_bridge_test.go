package skills

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type mockRegistrar struct {
	registered   map[string]*CapabilityDescriptor
	unregistered []string
	failRegister bool
}

func newMockRegistrar() *mockRegistrar {
	return &mockRegistrar{registered: make(map[string]*CapabilityDescriptor)}
}

func (m *mockRegistrar) RegisterCapability(ctx context.Context, desc *CapabilityDescriptor) error {
	if m.failRegister {
		return fmt.Errorf("register failed")
	}
	m.registered[desc.Name] = desc
	return nil
}

func (m *mockRegistrar) UnregisterCapability(ctx context.Context, agentID string, capName string) error {
	m.unregistered = append(m.unregistered, capName)
	return nil
}

func TestSkillDiscoveryBridge_RegisterSkillAsCapability(t *testing.T) {
	mgr := NewSkillManager(DefaultSkillManagerConfig(), zap.NewNop())
	reg := newMockRegistrar()
	bridge := NewSkillDiscoveryBridge(mgr, reg, "agent-1", nil)

	skill := &Skill{ID: "s1", Name: "Test", Description: "desc", Version: "1.0.0", Category: "coding"}
	require.NoError(t, bridge.RegisterSkillAsCapability(context.Background(), skill))

	desc, ok := reg.registered["s1"]
	assert.True(t, ok)
	assert.Equal(t, "agent-1", desc.AgentID)
	assert.Equal(t, "task", desc.Category) // coding -> task
}

func TestSkillDiscoveryBridge_RegisterSkillAsCapability_NilSkill(t *testing.T) {
	bridge := NewSkillDiscoveryBridge(nil, newMockRegistrar(), "a1", nil)
	err := bridge.RegisterSkillAsCapability(context.Background(), nil)
	assert.Error(t, err)
}

func TestSkillDiscoveryBridge_RegisterSkillAsCapability_RegistrarFails(t *testing.T) {
	reg := newMockRegistrar()
	reg.failRegister = true
	bridge := NewSkillDiscoveryBridge(nil, reg, "a1", nil)

	err := bridge.RegisterSkillAsCapability(context.Background(), &Skill{ID: "s1"})
	assert.Error(t, err)
}

func TestSkillDiscoveryBridge_UnregisterSkill(t *testing.T) {
	reg := newMockRegistrar()
	bridge := NewSkillDiscoveryBridge(nil, reg, "a1", nil)

	require.NoError(t, bridge.UnregisterSkill(context.Background(), "s1"))
	assert.Contains(t, reg.unregistered, "s1")
}

func TestSkillDiscoveryBridge_SyncAll(t *testing.T) {
	mgr := NewSkillManager(DefaultSkillManagerConfig(), zap.NewNop())
	skill, _ := NewSkillBuilder("s1", "Skill1").
		WithInstructions("do stuff").
		WithCategory("coding").
		Build()
	require.NoError(t, mgr.RegisterSkill(skill))

	reg := newMockRegistrar()
	bridge := NewSkillDiscoveryBridge(mgr, reg, "a1", nil)

	require.NoError(t, bridge.SyncAll(context.Background()))
	assert.Len(t, reg.registered, 1)
}

func TestSkillDiscoveryBridge_SyncAll_NoSkills(t *testing.T) {
	mgr := NewSkillManager(DefaultSkillManagerConfig(), zap.NewNop())
	reg := newMockRegistrar()
	bridge := NewSkillDiscoveryBridge(mgr, reg, "a1", nil)

	require.NoError(t, bridge.SyncAll(context.Background()))
	assert.Empty(t, reg.registered)
}

func TestSkillDiscoveryBridge_SyncAll_PartialFailure(t *testing.T) {
	mgr := NewSkillManager(DefaultSkillManagerConfig(), zap.NewNop())
	s1, _ := NewSkillBuilder("s1", "Skill1").WithInstructions("do").Build()
	s2, _ := NewSkillBuilder("s2", "Skill2").WithInstructions("do").Build()
	require.NoError(t, mgr.RegisterSkill(s1))
	require.NoError(t, mgr.RegisterSkill(s2))

	reg := newMockRegistrar()
	reg.failRegister = true
	bridge := NewSkillDiscoveryBridge(mgr, reg, "a1", nil)

	err := bridge.SyncAll(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "errors")
}

func TestMapSkillCategoryToCapType(t *testing.T) {
	tests := []struct {
		category string
		expected string
	}{
		{"coding", "task"},
		{"automation", "task"},
		{"research", "query"},
		{"data", "query"},
		{"reasoning", "query"},
		{"communication", "stream"},
		{"unknown", "task"},
		{"", "task"},
	}

	for _, tt := range tests {
		t.Run(tt.category, func(t *testing.T) {
			assert.Equal(t, tt.expected, mapSkillCategoryToCapType(tt.category))
		})
	}
}

func TestBuildSkillMetadata(t *testing.T) {
	skill := &Skill{
		ID:       "s1",
		Version:  "1.0.0",
		Category: "coding",
		Author:   "test-author",
	}

	meta := buildSkillMetadata(skill)
	assert.Equal(t, "skills", meta["source"])
	assert.Equal(t, "s1", meta["skill_id"])
	assert.Equal(t, "1.0.0", meta["version"])
	assert.Equal(t, "coding", meta["category"])
	assert.Equal(t, "test-author", meta["author"])
	assert.NotEmpty(t, meta["synced_at"])
}
