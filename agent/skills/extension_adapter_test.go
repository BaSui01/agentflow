package skills

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestSkillsExtensionAdapter_ListSkills(t *testing.T) {
	mgr := NewSkillManager(DefaultSkillManagerConfig(), zap.NewNop())
	s1, _ := NewSkillBuilder("s1", "Alpha").WithInstructions("do").Build()
	s2, _ := NewSkillBuilder("s2", "Beta").WithInstructions("do").Build()
	require.NoError(t, mgr.RegisterSkill(s1))
	require.NoError(t, mgr.RegisterSkill(s2))

	adapter := NewSkillsExtensionAdapter(mgr, nil)
	names := adapter.ListSkills()
	assert.Len(t, names, 2)
	assert.Contains(t, names, "Alpha")
	assert.Contains(t, names, "Beta")
}

func TestSkillsExtensionAdapter_LoadSkill_ByName(t *testing.T) {
	config := DefaultSkillManagerConfig()
	config.AutoLoad = false
	mgr := NewSkillManager(config, zap.NewNop())
	s1, _ := NewSkillBuilder("s1", "Alpha").WithInstructions("do").Build()
	require.NoError(t, mgr.RegisterSkill(s1))

	adapter := NewSkillsExtensionAdapter(mgr, nil)
	require.NoError(t, adapter.LoadSkill(context.Background(), "Alpha"))

	_, ok := mgr.GetSkill("s1")
	assert.True(t, ok)
}

func TestSkillsExtensionAdapter_LoadSkill_NotFound(t *testing.T) {
	mgr := NewSkillManager(DefaultSkillManagerConfig(), zap.NewNop())
	adapter := NewSkillsExtensionAdapter(mgr, nil)
	err := adapter.LoadSkill(context.Background(), "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestSkillsExtensionAdapter_ExecuteSkill_ViaRegistry(t *testing.T) {
	mgr := NewSkillManager(DefaultSkillManagerConfig(), zap.NewNop())
	reg := NewRegistry(zap.NewNop())

	handler := func(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
		return []byte(`{"result":"executed"}`), nil
	}
	require.NoError(t, reg.Register(&SkillDefinition{ID: "s1", Name: "Alpha", Category: CategoryCoding}, handler))

	adapter := NewSkillsExtensionAdapter(mgr, reg)
	result, err := adapter.ExecuteSkill(context.Background(), "Alpha", map[string]string{"key": "val"})
	require.NoError(t, err)

	resultMap, ok := result.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "executed", resultMap["result"])
}

func TestSkillsExtensionAdapter_ExecuteSkill_FallbackToInstructions(t *testing.T) {
	mgr := NewSkillManager(DefaultSkillManagerConfig(), zap.NewNop())
	s1, _ := NewSkillBuilder("s1", "Alpha").
		WithInstructions("Do the thing").
		Build()
	require.NoError(t, mgr.RegisterSkill(s1))

	adapter := NewSkillsExtensionAdapter(mgr, nil)
	result, err := adapter.ExecuteSkill(context.Background(), "s1", "input")
	require.NoError(t, err)

	resultMap, ok := result.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "Do the thing", resultMap["instructions"])
}

func TestSkillsExtensionAdapter_ExecuteSkill_NotFound(t *testing.T) {
	mgr := NewSkillManager(DefaultSkillManagerConfig(), zap.NewNop())
	adapter := NewSkillsExtensionAdapter(mgr, nil)
	_, err := adapter.ExecuteSkill(context.Background(), "nonexistent", nil)
	assert.Error(t, err)
}

