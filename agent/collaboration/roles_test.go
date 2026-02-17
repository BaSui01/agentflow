package collaboration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ---------------------------------------------------------------------------
// RoleRegistry CRUD
// ---------------------------------------------------------------------------

func TestRoleRegistry_RegisterAndGet(t *testing.T) {
	t.Parallel()

	reg := NewRoleRegistry(zap.NewNop())

	def := &RoleDefinition{
		Type: RoleCollector,
		Name: "Test Collector",
	}
	err := reg.Register(def)
	require.NoError(t, err)

	got, ok := reg.Get(RoleCollector)
	assert.True(t, ok)
	assert.Equal(t, "Test Collector", got.Name)
}

func TestRoleRegistry_GetNotFound(t *testing.T) {
	t.Parallel()

	reg := NewRoleRegistry(zap.NewNop())

	_, ok := reg.Get(RoleCollector)
	assert.False(t, ok)
}

func TestRoleRegistry_List(t *testing.T) {
	t.Parallel()

	reg := NewRoleRegistry(zap.NewNop())

	// Empty registry.
	assert.Empty(t, reg.List())

	_ = reg.Register(&RoleDefinition{Type: RoleCollector, Name: "C"})
	_ = reg.Register(&RoleDefinition{Type: RoleFilter, Name: "F"})

	list := reg.List()
	assert.Len(t, list, 2)
}

func TestRoleRegistry_Unregister(t *testing.T) {
	t.Parallel()

	reg := NewRoleRegistry(zap.NewNop())
	_ = reg.Register(&RoleDefinition{Type: RoleCollector, Name: "C"})

	err := reg.Unregister(RoleCollector)
	require.NoError(t, err)

	_, ok := reg.Get(RoleCollector)
	assert.False(t, ok)
}

func TestRoleRegistry_UnregisterNotFound(t *testing.T) {
	t.Parallel()

	reg := NewRoleRegistry(zap.NewNop())
	err := reg.Unregister(RoleCollector)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRoleRegistry_NilLogger(t *testing.T) {
	t.Parallel()

	reg := NewRoleRegistry(nil)
	assert.NotNil(t, reg)

	// Should not panic on operations.
	err := reg.Register(&RoleDefinition{Type: RoleCollector, Name: "C"})
	assert.NoError(t, err)
}

// ---------------------------------------------------------------------------
// Duplicate registration
// ---------------------------------------------------------------------------

func TestRoleRegistry_RegisterDuplicate(t *testing.T) {
	t.Parallel()

	reg := NewRoleRegistry(zap.NewNop())

	def := &RoleDefinition{Type: RoleFilter, Name: "Filter"}
	require.NoError(t, reg.Register(def))

	err := reg.Register(def)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")
}

// ---------------------------------------------------------------------------
// Predefined research roles
// ---------------------------------------------------------------------------

func TestPredefinedRoles_CorrectTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		factory  func() *RoleDefinition
		wantType RoleType
	}{
		{"Collector", NewResearchCollectorRole, RoleCollector},
		{"Filter", NewResearchFilterRole, RoleFilter},
		{"Generator", NewResearchGeneratorRole, RoleGenerator},
		{"Validator", NewResearchValidatorRole, RoleValidator},
		{"Writer", NewResearchWriterRole, RoleWriter},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			role := tc.factory()
			assert.Equal(t, tc.wantType, role.Type)
			assert.NotEmpty(t, role.Name)
			assert.NotEmpty(t, role.Description)
			assert.NotEmpty(t, role.SystemPrompt)
			assert.NotEmpty(t, role.Capabilities)
		})
	}
}

func TestPredefinedRoles_HaveTimeout(t *testing.T) {
	t.Parallel()

	roles := []*RoleDefinition{
		NewResearchCollectorRole(),
		NewResearchFilterRole(),
		NewResearchGeneratorRole(),
		NewResearchValidatorRole(),
		NewResearchWriterRole(),
	}

	for _, r := range roles {
		assert.Greater(t, r.Timeout, time.Duration(0), "role %s should have a timeout", r.Name)
	}
}

// ---------------------------------------------------------------------------
// RegisterResearchRoles
// ---------------------------------------------------------------------------

func TestRegisterResearchRoles(t *testing.T) {
	t.Parallel()

	reg := NewRoleRegistry(zap.NewNop())
	err := RegisterResearchRoles(reg)
	require.NoError(t, err)

	list := reg.List()
	assert.Len(t, list, 5, "should register exactly 5 research roles")

	// Verify each expected type is present.
	expected := []RoleType{RoleCollector, RoleFilter, RoleGenerator, RoleValidator, RoleWriter}
	for _, rt := range expected {
		_, ok := reg.Get(rt)
		assert.True(t, ok, "expected role %s to be registered", rt)
	}
}

func TestRegisterResearchRoles_DuplicateError(t *testing.T) {
	t.Parallel()

	reg := NewRoleRegistry(zap.NewNop())
	require.NoError(t, RegisterResearchRoles(reg))

	// Registering again should fail.
	err := RegisterResearchRoles(reg)
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// RolePipeline – AddStage and basic structure
// ---------------------------------------------------------------------------

func TestRolePipeline_AddStage(t *testing.T) {
	t.Parallel()

	reg := NewRoleRegistry(zap.NewNop())
	require.NoError(t, RegisterResearchRoles(reg))

	cfg := DefaultPipelineConfig()
	noop := func(_ context.Context, _ *RoleDefinition, input interface{}) (interface{}, error) {
		return input, nil
	}

	pipeline := NewRolePipeline(cfg, reg, noop, zap.NewNop())
	pipeline.AddStage(RoleCollector).
		AddStage(RoleFilter).
		AddStage(RoleGenerator)

	assert.Len(t, pipeline.stages, 3)
}

func TestRolePipeline_NilLogger(t *testing.T) {
	t.Parallel()

	reg := NewRoleRegistry(nil)
	cfg := DefaultPipelineConfig()
	noop := func(_ context.Context, _ *RoleDefinition, input interface{}) (interface{}, error) {
		return input, nil
	}

	pipeline := NewRolePipeline(cfg, reg, noop, nil)
	assert.NotNil(t, pipeline)
}

func TestRolePipeline_Execute_SingleStage(t *testing.T) {
	t.Parallel()

	reg := NewRoleRegistry(zap.NewNop())
	require.NoError(t, RegisterResearchRoles(reg))

	cfg := DefaultPipelineConfig()
	cfg.Timeout = 5 * time.Second

	executeFn := func(_ context.Context, role *RoleDefinition, input interface{}) (interface{}, error) {
		return map[string]interface{}{
			"role":   string(role.Type),
			"result": "done",
		}, nil
	}

	pipeline := NewRolePipeline(cfg, reg, executeFn, zap.NewNop())
	pipeline.AddStage(RoleCollector)

	results, err := pipeline.Execute(context.Background(), "initial input")
	require.NoError(t, err)
	assert.Contains(t, results, RoleCollector)
}

func TestDefaultPipelineConfig(t *testing.T) {
	t.Parallel()

	cfg := DefaultPipelineConfig()
	assert.Equal(t, "default-pipeline", cfg.Name)
	assert.Equal(t, 3, cfg.MaxConcurrency)
	assert.Equal(t, 30*time.Minute, cfg.Timeout)
	assert.True(t, cfg.StopOnFailure)
}
