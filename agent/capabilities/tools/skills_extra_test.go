package skills

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// --- DiscoverSkills ---

func TestDefaultSkillManager_DiscoverSkills_NoMatch(t *testing.T) {
	mgr := NewSkillManager(DefaultSkillManagerConfig(), zap.NewNop())
	ctx := context.Background()

	skills, err := mgr.DiscoverSkills(ctx, "completely unrelated query xyz")
	require.NoError(t, err)
	assert.Empty(t, skills)
}

func TestDefaultSkillManager_DiscoverSkills_WithMatch(t *testing.T) {
	config := DefaultSkillManagerConfig()
	config.AutoLoad = true
	mgr := NewSkillManager(config, zap.NewNop())

	skill, _ := NewSkillBuilder("code-review", "Code Review").
		WithDescription("Review Go code quality and suggest improvements").
		WithInstructions("Review the code carefully").
		WithCategory("coding").
		WithTags("go", "review", "code").
		Build()
	require.NoError(t, mgr.RegisterSkill(skill))

	ctx := context.Background()
	discovered, err := mgr.DiscoverSkills(ctx, "review go code")
	require.NoError(t, err)
	assert.NotEmpty(t, discovered)
	assert.Equal(t, "code-review", discovered[0].ID)
}

// --- UnloadSkill ---

func TestDefaultSkillManager_UnloadSkill_Success(t *testing.T) {
	config := DefaultSkillManagerConfig()
	config.AutoLoad = true
	mgr := NewSkillManager(config, zap.NewNop())
	skill, _ := NewSkillBuilder("unload-test", "Unload Test").
		WithDescription("Test skill for unloading").
		WithInstructions("Do something").
		Build()
	require.NoError(t, mgr.RegisterSkill(skill))

	// Skill should be auto-loaded
	_, loaded := mgr.GetSkill("unload-test")
	assert.True(t, loaded)

	err := mgr.UnloadSkill(context.Background(), "unload-test")
	require.NoError(t, err)

	_, loaded = mgr.GetSkill("unload-test")
	assert.False(t, loaded)
}

func TestDefaultSkillManager_UnloadSkill_NotLoaded(t *testing.T) {
	mgr := NewSkillManager(DefaultSkillManagerConfig(), zap.NewNop())

	err := mgr.UnloadSkill(context.Background(), "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not loaded")
}

// --- UnregisterSkill ---

func TestDefaultSkillManager_UnregisterSkill(t *testing.T) {
	config := DefaultSkillManagerConfig()
	config.AutoLoad = true
	mgr := NewSkillManager(config, zap.NewNop())

	skill, _ := NewSkillBuilder("unreg-test", "Unreg Test").
		WithDescription("Test skill for unregistering").
		WithInstructions("Do something").
		Build()
	require.NoError(t, mgr.RegisterSkill(skill))

	err := mgr.UnregisterSkill("unreg-test")
	require.NoError(t, err)

	// Should not be findable anymore
	_, loaded := mgr.GetSkill("unreg-test")
	assert.False(t, loaded)

	// Index should be empty
	assert.Equal(t, 0, mgr.GetIndexedSkillsCount())
}

// --- GetLoadedSkillsCount ---

func TestDefaultSkillManager_GetLoadedSkillsCount(t *testing.T) {
	config := DefaultSkillManagerConfig()
	config.AutoLoad = true
	mgr := NewSkillManager(config, zap.NewNop())

	assert.Equal(t, 0, mgr.GetLoadedSkillsCount())

	skill, _ := NewSkillBuilder("count-test", "Count Test").
		WithDescription("Test skill").
		WithInstructions("Do something").
		Build()
	require.NoError(t, mgr.RegisterSkill(skill))

	assert.Equal(t, 1, mgr.GetLoadedSkillsCount())
}

// --- GetIndexedSkillsCount ---

func TestDefaultSkillManager_GetIndexedSkillsCount(t *testing.T) {
	mgr := NewSkillManager(DefaultSkillManagerConfig(), zap.NewNop())

	assert.Equal(t, 0, mgr.GetIndexedSkillsCount())

	skill, _ := NewSkillBuilder("idx-test", "Index Test").
		WithDescription("Test skill").
		WithInstructions("Do something").
		Build()
	require.NoError(t, mgr.RegisterSkill(skill))

	assert.Equal(t, 1, mgr.GetIndexedSkillsCount())
}

// --- ClearCache ---

func TestDefaultSkillManager_ClearCache(t *testing.T) {
	config := DefaultSkillManagerConfig()
	config.AutoLoad = true
	mgr := NewSkillManager(config, zap.NewNop())

	skill, _ := NewSkillBuilder("cache-test", "Cache Test").
		WithDescription("Test skill").
		WithInstructions("Do something").
		Build()
	require.NoError(t, mgr.RegisterSkill(skill))

	assert.Equal(t, 1, mgr.GetLoadedSkillsCount())

	mgr.ClearCache()
	assert.Equal(t, 0, mgr.GetLoadedSkillsCount())
	// Index should still be intact
	assert.Equal(t, 1, mgr.GetIndexedSkillsCount())
}
