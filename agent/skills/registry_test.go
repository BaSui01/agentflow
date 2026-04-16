package skills

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// --- Registry tests ---

func TestRegistry_Register_And_Get(t *testing.T) {
	reg := NewRegistry(zap.NewNop())
	def := &SkillDefinition{ID: "s1", Name: "Skill1", Category: CategoryCoding}
	handler := func(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
		return []byte(`"ok"`), nil
	}
	require.NoError(t, reg.Register(def, handler))

	inst, ok := reg.Get("s1")
	assert.True(t, ok)
	assert.Equal(t, "Skill1", inst.Definition.Name)
	assert.True(t, inst.Enabled)
}

func TestRegistry_Register_GeneratesID(t *testing.T) {
	reg := NewRegistry(zap.NewNop())
	def := &SkillDefinition{Name: "NoID"}
	require.NoError(t, reg.Register(def, nil))
	assert.NotEmpty(t, def.ID)
}

func TestRegistry_Unregister(t *testing.T) {
	reg := NewRegistry(zap.NewNop())
	def := &SkillDefinition{ID: "s1", Name: "Skill1", Category: CategoryCoding}
	require.NoError(t, reg.Register(def, nil))

	require.NoError(t, reg.Unregister("s1"))
	_, ok := reg.Get("s1")
	assert.False(t, ok)
}

func TestRegistry_Unregister_NotFound(t *testing.T) {
	reg := NewRegistry(zap.NewNop())
	err := reg.Unregister("nonexistent")
	assert.Error(t, err)
}

func TestRegistry_GetByName(t *testing.T) {
	reg := NewRegistry(zap.NewNop())
	require.NoError(t, reg.Register(&SkillDefinition{ID: "s1", Name: "Alpha"}, nil))
	require.NoError(t, reg.Register(&SkillDefinition{ID: "s2", Name: "Beta"}, nil))

	inst, ok := reg.GetByName("Beta")
	assert.True(t, ok)
	assert.Equal(t, "s2", inst.Definition.ID)

	_, ok = reg.GetByName("Gamma")
	assert.False(t, ok)
}

func TestRegistry_ListByCategory(t *testing.T) {
	reg := NewRegistry(zap.NewNop())
	require.NoError(t, reg.Register(&SkillDefinition{ID: "s1", Category: CategoryCoding}, nil))
	require.NoError(t, reg.Register(&SkillDefinition{ID: "s2", Category: CategoryResearch}, nil))
	require.NoError(t, reg.Register(&SkillDefinition{ID: "s3", Category: CategoryCoding}, nil))

	coding := reg.ListByCategory(CategoryCoding)
	assert.Len(t, coding, 2)

	research := reg.ListByCategory(CategoryResearch)
	assert.Len(t, research, 1)
}

func TestRegistry_ListAll(t *testing.T) {
	reg := NewRegistry(zap.NewNop())
	require.NoError(t, reg.Register(&SkillDefinition{ID: "s1"}, nil))
	require.NoError(t, reg.Register(&SkillDefinition{ID: "s2"}, nil))

	all := reg.ListAll()
	assert.Len(t, all, 2)
}

func TestRegistry_Search_ByQuery(t *testing.T) {
	reg := NewRegistry(zap.NewNop())
	require.NoError(t, reg.Register(&SkillDefinition{ID: "s1", Name: "Code Review", Description: "Reviews code"}, nil))
	require.NoError(t, reg.Register(&SkillDefinition{ID: "s2", Name: "Data Analysis", Description: "Analyzes data"}, nil))

	results := reg.Search("Code", nil)
	assert.Len(t, results, 1)
	assert.Equal(t, "s1", results[0].Definition.ID)
}

func TestRegistry_Search_ByTags(t *testing.T) {
	reg := NewRegistry(zap.NewNop())
	require.NoError(t, reg.Register(&SkillDefinition{ID: "s1", Tags: []string{"go", "review"}}, nil))
	require.NoError(t, reg.Register(&SkillDefinition{ID: "s2", Tags: []string{"python"}}, nil))

	results := reg.Search("", []string{"go"})
	assert.Len(t, results, 1)
	assert.Equal(t, "s1", results[0].Definition.ID)
}

func TestRegistry_Invoke_Success(t *testing.T) {
	reg := NewRegistry(zap.NewNop())
	handler := func(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
		return []byte(`{"result":"done"}`), nil
	}
	require.NoError(t, reg.Register(&SkillDefinition{ID: "s1"}, handler))

	result, err := reg.Invoke(context.Background(), "s1", []byte(`{}`))
	require.NoError(t, err)
	assert.Contains(t, string(result), "done")
}

func TestRegistry_Invoke_NotFound(t *testing.T) {
	reg := NewRegistry(zap.NewNop())
	_, err := reg.Invoke(context.Background(), "nope", nil)
	assert.Error(t, err)
}

func TestRegistry_Invoke_Disabled(t *testing.T) {
	reg := NewRegistry(zap.NewNop())
	require.NoError(t, reg.Register(&SkillDefinition{ID: "s1"}, func(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
		return nil, nil
	}))
	require.NoError(t, reg.Disable("s1"))

	_, err := reg.Invoke(context.Background(), "s1", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "disabled")
}

func TestRegistry_Invoke_UpdatesStats(t *testing.T) {
	reg := NewRegistry(zap.NewNop())
	handler := func(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
		return []byte(`"ok"`), nil
	}
	require.NoError(t, reg.Register(&SkillDefinition{ID: "s1"}, handler))

	_, err := reg.Invoke(context.Background(), "s1", []byte(`{}`))
	require.NoError(t, err)
	_, err = reg.Invoke(context.Background(), "s1", []byte(`{}`))
	require.NoError(t, err)

	inst, _ := reg.Get("s1")
	assert.Equal(t, int64(2), inst.Stats.Invocations)
	assert.Equal(t, int64(2), inst.Stats.Successes)
	assert.NotNil(t, inst.Stats.LastInvoked)
}

func TestRegistry_Enable_Disable(t *testing.T) {
	reg := NewRegistry(zap.NewNop())
	require.NoError(t, reg.Register(&SkillDefinition{ID: "s1"}, nil))

	require.NoError(t, reg.Disable("s1"))
	inst, _ := reg.Get("s1")
	assert.False(t, inst.Enabled)

	require.NoError(t, reg.Enable("s1"))
	inst, _ = reg.Get("s1")
	assert.True(t, inst.Enabled)
}

func TestRegistry_Enable_NotFound(t *testing.T) {
	reg := NewRegistry(zap.NewNop())
	assert.Error(t, reg.Enable("nope"))
	assert.Error(t, reg.Disable("nope"))
}

func TestRegistry_Export_Import(t *testing.T) {
	reg := NewRegistry(zap.NewNop())
	require.NoError(t, reg.Register(&SkillDefinition{ID: "s1", Name: "Skill1", Category: CategoryCoding}, nil))
	require.NoError(t, reg.Register(&SkillDefinition{ID: "s2", Name: "Skill2", Category: CategoryResearch}, nil))

	data, err := reg.Export()
	require.NoError(t, err)

	reg2 := NewRegistry(zap.NewNop())
	require.NoError(t, reg2.Import(data))

	all := reg2.ListAll()
	assert.Len(t, all, 2)
	// Imported skills should be disabled (no handler)
	for _, inst := range all {
		assert.False(t, inst.Enabled)
	}
}

func TestRegistry_Import_InvalidJSON(t *testing.T) {
	reg := NewRegistry(zap.NewNop())
	err := reg.Import([]byte(`not json`))
	assert.Error(t, err)
}
