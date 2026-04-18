package types

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentConfigExecutionOptions(t *testing.T) {
	cfg := AgentConfig{
		Core: CoreConfig{ID: "agent-1", Name: "Agent", Type: "assistant"},
		LLM: LLMConfig{
			Model:       "gpt-4o",
			Provider:    "openai",
			MaxTokens:   2048,
			Temperature: 0.4,
			TopP:        0.9,
			Stop:        []string{"STOP"},
		},
		Runtime: RuntimeConfig{
			SystemPrompt:       "You are helpful.",
			Tools:              []string{"search", "calc"},
			Handoffs:           []string{"reviewer"},
			MaxReActIterations: 5,
			MaxLoopIterations:  3,
			ToolModel:          "gpt-4o-mini",
		},
		Context: &ContextConfig{
			Enabled:          true,
			MaxContextTokens: 1234,
		},
		Features: FeaturesConfig{
			Reflection:     &ReflectionConfig{Enabled: true, MaxIterations: 2},
			Guardrails:     &GuardrailsConfig{Enabled: true, MaxRetries: 4},
			Memory:         &MemoryConfig{Enabled: true, ShortTermTTL: time.Minute},
			ToolSelection:  &ToolSelectionConfig{Enabled: true, MaxTools: 2},
			PromptEnhancer: &PromptEnhancerConfig{Enabled: true, Mode: "basic"},
		},
		Metadata: map[string]string{"tenant": "t1"},
	}

	options := cfg.ExecutionOptions()
	require.Equal(t, "agent-1", options.Core.ID)
	assert.Equal(t, "openai", options.Model.Provider)
	assert.Equal(t, "gpt-4o", options.Model.Model)
	assert.Equal(t, 2048, options.Model.MaxTokens)
	assert.Equal(t, "You are helpful.", options.Control.SystemPrompt)
	assert.Equal(t, 5, options.Control.MaxReActIterations)
	assert.Equal(t, 3, options.Control.MaxLoopIterations)
	assert.Equal(t, []string{"search", "calc"}, options.Tools.AllowedTools)
	assert.Equal(t, []string{"reviewer"}, options.Tools.Handoffs)
	assert.Equal(t, "gpt-4o-mini", options.Tools.ToolModel)
	require.NotNil(t, options.Control.Context)
	assert.Equal(t, 1234, options.Control.Context.MaxContextTokens)
	require.NotNil(t, options.Control.Reflection)
	assert.Equal(t, 2, options.Control.Reflection.MaxIterations)
	assert.Equal(t, map[string]string{"tenant": "t1"}, options.Metadata)

	cfg.LLM.Stop[0] = "MUTATED"
	cfg.Runtime.Tools[0] = "mutated"
	cfg.Metadata["tenant"] = "mutated"
	assert.Equal(t, []string{"STOP"}, options.Model.Stop)
	assert.Equal(t, []string{"search", "calc"}, options.Tools.AllowedTools)
	assert.Equal(t, map[string]string{"tenant": "t1"}, options.Metadata)
}

func TestParseToolChoiceString(t *testing.T) {
	t.Run("blank returns nil", func(t *testing.T) {
		assert.Nil(t, ParseToolChoiceString(" "))
	})

	t.Run("auto", func(t *testing.T) {
		choice := ParseToolChoiceString("auto")
		require.NotNil(t, choice)
		assert.Equal(t, ToolChoiceModeAuto, choice.Mode)
	})

	t.Run("required", func(t *testing.T) {
		choice := ParseToolChoiceString("required")
		require.NotNil(t, choice)
		assert.Equal(t, ToolChoiceModeRequired, choice.Mode)
	})

	t.Run("specific tool", func(t *testing.T) {
		choice := ParseToolChoiceString("search")
		require.NotNil(t, choice)
		assert.Equal(t, ToolChoiceModeSpecific, choice.Mode)
		assert.Equal(t, "search", choice.ToolName)
	})
}

func TestAgentConfigExecutionOptions_PrefersFormalMainFace(t *testing.T) {
	cfg := AgentConfig{
		Core: CoreConfig{ID: "agent-1", Name: "Agent", Type: "assistant"},
		Model: ModelOptions{
			Model:       "formal-model",
			Provider:    "formal-provider",
			MaxTokens:   99,
			Temperature: 0.2,
		},
		Control: AgentControlOptions{
			SystemPrompt:       "formal prompt",
			MaxReActIterations: 7,
		},
		Tools: ToolProtocolOptions{
			AllowedTools: []string{"formal-tool"},
			ToolModel:    "tool-model",
		},
		LLM: LLMConfig{
			Model:       "legacy-model",
			Provider:    "legacy-provider",
			MaxTokens:   10,
			Temperature: 0.9,
		},
		Runtime: RuntimeConfig{
			SystemPrompt: "legacy prompt",
			Tools:        []string{"legacy-tool"},
		},
	}

	options := cfg.ExecutionOptions()
	assert.Equal(t, "formal-model", options.Model.Model)
	assert.Equal(t, "formal-provider", options.Model.Provider)
	assert.Equal(t, 99, options.Model.MaxTokens)
	assert.Equal(t, "formal prompt", options.Control.SystemPrompt)
	assert.Equal(t, 7, options.Control.MaxReActIterations)
	assert.Equal(t, []string{"formal-tool"}, options.Tools.AllowedTools)
	assert.Equal(t, "tool-model", options.Tools.ToolModel)
}
