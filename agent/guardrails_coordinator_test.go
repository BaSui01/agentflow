package agent

import (
	"testing"

	"github.com/BaSui01/agentflow/agent/guardrails"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewGuardrailsCoordinator_NilConfig(t *testing.T) {
	gc := NewGuardrailsCoordinator(nil, zap.NewNop())
	require.NotNil(t, gc)
	assert.False(t, gc.Enabled())
	assert.Nil(t, gc.GetConfig())
	assert.Nil(t, gc.GetInputValidatorChain())
	assert.Nil(t, gc.GetOutputValidator())
	assert.Equal(t, 0, gc.InputValidatorCount())
}

func TestGuardrailsCoordinator_SetEnabled(t *testing.T) {
	gc := NewGuardrailsCoordinator(nil, zap.NewNop())
	gc.SetEnabled(true)
	assert.True(t, gc.Enabled())
	gc.SetEnabled(false)
	assert.False(t, gc.Enabled())
}

func TestGuardrailsCoordinator_WithConfig(t *testing.T) {
	cfg := &guardrails.GuardrailsConfig{MaxInputLength: 100}
	gc := NewGuardrailsCoordinator(cfg, zap.NewNop())
	assert.True(t, gc.Enabled())
	assert.Equal(t, cfg, gc.GetConfig())
	assert.NotNil(t, gc.GetInputValidatorChain())
	assert.NotNil(t, gc.GetOutputValidator())
}

func TestGuardrailsCoordinator_AddOutputValidator_NilInit(t *testing.T) {
	gc := NewGuardrailsCoordinator(nil, zap.NewNop())
	assert.Nil(t, gc.GetOutputValidator())

	// Adding a validator should auto-create the output validator
	gc.AddOutputValidator(guardrails.NewLengthValidator(&guardrails.LengthValidatorConfig{
		MaxLength: 100,
		Action:    guardrails.LengthActionReject,
	}))
	assert.NotNil(t, gc.GetOutputValidator())
	assert.True(t, gc.Enabled())
}

func TestGuardrailsCoordinator_AddOutputFilter_NilInit(t *testing.T) {
	gc := NewGuardrailsCoordinator(nil, zap.NewNop())

	// Adding a filter should auto-create the output validator
	filter, err := guardrails.NewContentFilter(nil)
	require.NoError(t, err)
	gc.AddOutputFilter(filter)
	assert.NotNil(t, gc.GetOutputValidator())
	assert.True(t, gc.Enabled())
}

func TestGuardrailsCoordinator_GetInputValidatorChain_WithConfig(t *testing.T) {
	cfg := &guardrails.GuardrailsConfig{}
	gc := NewGuardrailsCoordinator(cfg, zap.NewNop())
	assert.NotNil(t, gc.GetInputValidatorChain())
}
