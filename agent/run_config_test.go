package agent

import (
	"context"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/llm"
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
	baseCfg := Config{
		Model:       "base-model",
		MaxTokens:   1024,
		Temperature: 0.5,
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
				Model:       StringPtr("override-model"),
				Temperature: Float32Ptr(0.9),
				MaxTokens:   IntPtr(4096),
				TopP:        Float32Ptr(0.95),
				Stop:        []string{"STOP"},
				ToolChoice:  StringPtr("none"),
				Timeout:     DurationPtr(60 * time.Second),
				Metadata:    map[string]string{"key": "val"},
				Tags:        []string{"tag1"},
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
				Metadata:    map[string]string{"key": "val"},
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
		rc.ApplyToRequest(nil, Config{})
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
