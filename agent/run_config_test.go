package agent

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunConfig_WithRunConfig_GetRunConfig(t *testing.T) {
	tests := []struct {
		name string
		rc   *RunConfig
	}{
		{
			name: "round-trip with full config",
			rc: &RunConfig{
				Model:              StringPtr("gpt-4o"),
				Temperature:        Float32Ptr(0.7),
				MaxTokens:          IntPtr(2048),
				TopP:               Float32Ptr(0.9),
				Stop:               []string{"\n"},
				ToolChoice:         StringPtr("auto"),
				Timeout:            DurationPtr(30 * time.Second),
				MaxReActIterations: IntPtr(5),
				MaxLoopIterations:  IntPtr(7),
				Metadata:           map[string]string{"env": "test"},
				Tags:               []string{"unit-test"},
			},
		},
		{
			name: "round-trip with partial config",
			rc: &RunConfig{
				Model: StringPtr("claude-3"),
			},
		},
		{
			name: "round-trip with nil config",
			rc:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			ctx = WithRunConfig(ctx, tt.rc)
			got := GetRunConfig(ctx)
			assert.Equal(t, tt.rc, got)
		})
	}
}

func TestRunConfig_GetRunConfig_NoConfig(t *testing.T) {
	ctx := context.Background()
	got := GetRunConfig(ctx)
	assert.Nil(t, got)
}

func TestRunConfig_ApplyToRequest(t *testing.T) {
	baseCfg := types.AgentConfig{
		Core: types.CoreConfig{ID: "t", Name: "t", Type: string(TypeGeneric)},
		LLM: types.LLMConfig{
			Model:       "base-model",
			MaxTokens:   1024,
			Temperature: 0.5,
		},
	}

	tests := []struct {
		name     string
		rc       *RunConfig
		initial  llm.ChatRequest
		expected llm.ChatRequest
	}{
		{
			name: "override all fields",
			rc: &RunConfig{
				Model:             StringPtr("override-model"),
				Temperature:       Float32Ptr(0.9),
				MaxTokens:         IntPtr(4096),
				TopP:              Float32Ptr(0.95),
				Stop:              []string{"STOP"},
				ToolChoice:        StringPtr("none"),
				Timeout:           DurationPtr(60 * time.Second),
				MaxLoopIterations: IntPtr(6),
				Metadata:          map[string]string{"key": "val"},
				Tags:              []string{"tag1"},
			},
			initial: llm.ChatRequest{
				Model:       "base-model",
				MaxTokens:   1024,
				Temperature: 0.5,
			},
			expected: llm.ChatRequest{
				Model:       "override-model",
				MaxTokens:   4096,
				Temperature: 0.9,
				TopP:        0.95,
				Stop:        []string{"STOP"},
				ToolChoice:  "none",
				Timeout:     60 * time.Second,
				Metadata:    map[string]string{"key": "val", "max_loop_iterations": "6"},
				Tags:        []string{"tag1"},
			},
		},
		{
			name: "partial override keeps defaults",
			rc: &RunConfig{
				Temperature: Float32Ptr(0.1),
			},
			initial: llm.ChatRequest{
				Model:       "base-model",
				MaxTokens:   1024,
				Temperature: 0.5,
			},
			expected: llm.ChatRequest{
				Model:       "base-model",
				MaxTokens:   1024,
				Temperature: 0.1,
			},
		},
		{
			name: "nil RunConfig is no-op",
			rc:   nil,
			initial: llm.ChatRequest{
				Model:       "base-model",
				MaxTokens:   1024,
				Temperature: 0.5,
			},
			expected: llm.ChatRequest{
				Model:       "base-model",
				MaxTokens:   1024,
				Temperature: 0.5,
			},
		},
		{
			name: "metadata merges with existing",
			rc: &RunConfig{
				Metadata: map[string]string{"new": "value"},
			},
			initial: llm.ChatRequest{
				Model:    "base-model",
				Metadata: map[string]string{"existing": "keep"},
			},
			expected: llm.ChatRequest{
				Model:    "base-model",
				Metadata: map[string]string{"existing": "keep", "new": "value"},
			},
		},
		{
			name: "metadata creates map when nil",
			rc: &RunConfig{
				Metadata: map[string]string{"key": "val"},
			},
			initial: llm.ChatRequest{
				Model: "base-model",
			},
			expected: llm.ChatRequest{
				Model:    "base-model",
				Metadata: map[string]string{"key": "val"},
			},
		},
		{
			name: "max loop iterations override wins over metadata duplicate",
			rc: &RunConfig{
				MaxLoopIterations: IntPtr(4),
				Metadata:          map[string]string{"max_loop_iterations": "999", "key": "val"},
			},
			initial: llm.ChatRequest{
				Model: "base-model",
			},
			expected: llm.ChatRequest{
				Model:    "base-model",
				Metadata: map[string]string{"max_loop_iterations": "4", "key": "val"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.initial
			tt.rc.ApplyToRequest(&req, baseCfg)
			assert.Equal(t, tt.expected, req)
		})
	}
}

func TestRunConfig_ApplyToRequest_NilRequest(t *testing.T) {
	rc := &RunConfig{Model: StringPtr("test")}
	// Should not panic
	assert.NotPanics(t, func() {
		rc.ApplyToRequest(nil, types.AgentConfig{})
	})
}

func TestRunConfig_EffectiveMaxReActIterations(t *testing.T) {
	tests := []struct {
		name       string
		rc         *RunConfig
		defaultVal int
		expected   int
	}{
		{
			name:       "nil RunConfig returns default",
			rc:         nil,
			defaultVal: 10,
			expected:   10,
		},
		{
			name:       "RunConfig without override returns default",
			rc:         &RunConfig{},
			defaultVal: 10,
			expected:   10,
		},
		{
			name:       "RunConfig with override returns override",
			rc:         &RunConfig{MaxReActIterations: IntPtr(3)},
			defaultVal: 10,
			expected:   3,
		},
		{
			name:       "RunConfig with zero override returns zero",
			rc:         &RunConfig{MaxReActIterations: IntPtr(0)},
			defaultVal: 10,
			expected:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.rc.EffectiveMaxReActIterations(tt.defaultVal)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestRunConfig_EffectiveMaxLoopIterations(t *testing.T) {
	tests := []struct {
		name       string
		rc         *RunConfig
		defaultVal int
		expected   int
	}{
		{
			name:       "nil RunConfig returns default",
			rc:         nil,
			defaultVal: 4,
			expected:   4,
		},
		{
			name:       "RunConfig without override returns default",
			rc:         &RunConfig{},
			defaultVal: 4,
			expected:   4,
		},
		{
			name:       "RunConfig with override returns override",
			rc:         &RunConfig{MaxLoopIterations: IntPtr(8)},
			defaultVal: 4,
			expected:   8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.rc.EffectiveMaxLoopIterations(tt.defaultVal)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestMergeRunConfig(t *testing.T) {
	base := &RunConfig{
		Model:         StringPtr("base-model"),
		Metadata:      map[string]string{"tenant": "t1"},
		Tags:          []string{"existing"},
		ToolWhitelist: []string{"tool-a"},
	}
	override := &RunConfig{
		MaxTokens:         IntPtr(2048),
		MaxLoopIterations: IntPtr(9),
		Metadata:          map[string]string{"route": "balanced"},
		DisableTools:      true,
		ToolWhitelist:     []string{"tool-b"},
	}

	merged := MergeRunConfig(base, override)
	require.NotNil(t, merged)
	assert.Equal(t, "base-model", *merged.Model)
	assert.Equal(t, 2048, *merged.MaxTokens)
	assert.Equal(t, 9, *merged.MaxLoopIterations)
	assert.Equal(t, map[string]string{"tenant": "t1", "route": "balanced"}, merged.Metadata)
	assert.Equal(t, []string{"existing"}, merged.Tags)
	assert.Equal(t, []string{"tool-b"}, merged.ToolWhitelist)
	assert.False(t, merged.DisableTools)
}

func TestRunConfigFromInputContext(t *testing.T) {
	rc := RunConfigFromInputContext(map[string]any{
		"max_react_iterations": 2,
		"max_loop_iterations":  5,
	})
	require.NotNil(t, rc)
	require.NotNil(t, rc.MaxReActIterations)
	require.NotNil(t, rc.MaxLoopIterations)
	assert.Equal(t, 2, *rc.MaxReActIterations)
	assert.Equal(t, 5, *rc.MaxLoopIterations)
}

func TestRunConfigFromInputContext_ParsesFlexibleNumberTypes(t *testing.T) {
	rc := RunConfigFromInputContext(map[string]any{
		"max_react_iterations": "3",
		"max_loop_iterations":  json.Number("6"),
	})
	require.NotNil(t, rc)
	require.NotNil(t, rc.MaxReActIterations)
	require.NotNil(t, rc.MaxLoopIterations)
	assert.Equal(t, 3, *rc.MaxReActIterations)
	assert.Equal(t, 6, *rc.MaxLoopIterations)
}

func TestRunConfigFromInputContext_UsesSingleTrackLoopBudgetKey(t *testing.T) {
	assert.Nil(t, RunConfigFromInputContext(map[string]any{"loop_max_iterations": 5}))
}

func TestResolveRunConfig_MergesContextInputContextAndOverrides(t *testing.T) {
	ctx := WithRunConfig(context.Background(), &RunConfig{
		MaxLoopIterations: IntPtr(2),
		MaxTokens:         IntPtr(512),
	})
	input := &Input{
		Context: map[string]any{
			"max_loop_iterations": 3,
		},
		Overrides: &RunConfig{
			MaxLoopIterations:  IntPtr(4),
			MaxReActIterations: IntPtr(6),
		},
	}

	resolved := ResolveRunConfig(ctx, input)
	require.NotNil(t, resolved)
	require.NotNil(t, resolved.MaxLoopIterations)
	require.NotNil(t, resolved.MaxReActIterations)
	require.NotNil(t, resolved.MaxTokens)
	assert.Equal(t, 4, *resolved.MaxLoopIterations)
	assert.Equal(t, 6, *resolved.MaxReActIterations)
	assert.Equal(t, 512, *resolved.MaxTokens)
}

func TestRunConfigFromInputContext_Empty(t *testing.T) {
	assert.Nil(t, RunConfigFromInputContext(nil))
	assert.Nil(t, RunConfigFromInputContext(map[string]any{"tenant": "t1"}))
}

func TestRunConfig_HelperFunctions(t *testing.T) {
	t.Run("StringPtr", func(t *testing.T) {
		p := StringPtr("hello")
		require.NotNil(t, p)
		assert.Equal(t, "hello", *p)
	})

	t.Run("Float32Ptr", func(t *testing.T) {
		p := Float32Ptr(0.7)
		require.NotNil(t, p)
		assert.InDelta(t, float32(0.7), *p, 0.001)
	})

	t.Run("IntPtr", func(t *testing.T) {
		p := IntPtr(42)
		require.NotNil(t, p)
		assert.Equal(t, 42, *p)
	})

	t.Run("DurationPtr", func(t *testing.T) {
		p := DurationPtr(5 * time.Second)
		require.NotNil(t, p)
		assert.Equal(t, 5*time.Second, *p)
	})
}
