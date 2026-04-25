package runtime

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeRunConfigDeepCopiesAndAppliesOverrideSemantics(t *testing.T) {
	baseTimeout := 2 * time.Second
	base := &RunConfig{
		Model:         StringPtr("base-model"),
		Stop:          []string{"stop-a"},
		ToolWhitelist: []string{"search"},
		Timeout:       &baseTimeout,
		Metadata:      map[string]string{"base": "keep"},
		Tags:          []string{"base-tag"},
	}
	override := &RunConfig{
		Provider:     StringPtr(" override-provider "),
		DisableTools: true,
		Metadata:     map[string]string{"override": "yes"},
	}

	merged := MergeRunConfig(base, override)

	require.NotNil(t, merged)
	assert.Equal(t, "base-model", *merged.Model)
	assert.Equal(t, " override-provider ", *merged.Provider)
	assert.True(t, merged.DisableTools)
	assert.Nil(t, merged.ToolWhitelist)
	assert.Equal(t, []string{"stop-a"}, merged.Stop)
	assert.Equal(t, map[string]string{"base": "keep", "override": "yes"}, merged.Metadata)
	assert.Equal(t, []string{"base-tag"}, merged.Tags)

	base.Stop[0] = "mutated"
	base.Metadata["base"] = "mutated"
	override.Metadata["override"] = "mutated"

	assert.Equal(t, []string{"stop-a"}, merged.Stop)
	assert.Equal(t, map[string]string{"base": "keep", "override": "yes"}, merged.Metadata)
}

func TestDefaultExecutionOptionsResolverAppliesContextRunConfigAndInputOverrides(t *testing.T) {
	ctx := types.WithLLMModel(context.Background(), "ctx-model")
	ctx = types.WithLLMProvider(ctx, "ctx-provider")
	ctx = types.WithLLMRoutePolicy(ctx, "balanced")
	ctx = WithRunConfig(ctx, &RunConfig{
		Provider:      StringPtr(" runtime-provider "),
		ToolWhitelist: []string{"search"},
		Metadata:      map[string]string{"source": "ctx"},
	})
	input := &Input{
		Context: map[string]any{
			"disable_planner":           "true",
			"top_level_loop_budget":     json.Number("9"),
			"max_react_iterations":      "4",
			"unsupported_runtime_field": "ignored",
		},
		Overrides: &RunConfig{
			Model: StringPtr("override-model"),
			Tags:  []string{"urgent"},
		},
	}

	options := NewDefaultExecutionOptionsResolver().Resolve(ctx, types.AgentConfig{
		Core: types.CoreConfig{ID: "agent-1", Name: "Agent 1", Type: string(TypeAssistant)},
		LLM:  types.LLMConfig{Model: "base-model", Provider: "base-provider"},
	}, input)

	assert.Equal(t, "override-model", options.Model.Model)
	assert.Equal(t, "runtime-provider", options.Model.Provider)
	assert.Equal(t, "balanced", options.Model.RoutePolicy)
	assert.True(t, options.Control.DisablePlanner)
	assert.Equal(t, 4, options.Control.MaxReActIterations)
	assert.Equal(t, 9, options.Control.MaxLoopIterations)
	assert.Equal(t, []string{"search"}, options.Tools.ToolWhitelist)
	assert.Equal(t, map[string]string{"source": "ctx"}, options.Metadata)
	assert.Equal(t, []string{"urgent"}, options.Tags)
}
