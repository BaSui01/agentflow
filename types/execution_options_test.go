package types

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentConfigExecutionOptions(t *testing.T) {
	approvalPolicy := "on-request"
	sandboxMode := "workspace-write"
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
			ApprovalPolicy:     approvalPolicy,
			SandboxMode:        sandboxMode,
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
	assert.Equal(t, approvalPolicy, options.Control.ApprovalPolicy)
	assert.Equal(t, sandboxMode, options.Control.SandboxMode)
	assert.Equal(t, []string{"search", "calc"}, options.Tools.AllowedTools)
	assert.Equal(t, []string{"reviewer"}, options.Tools.Handoffs)
	assert.Equal(t, "gpt-4o-mini", options.Tools.ToolModel)
	require.NotNil(t, options.Tools.Subagents)
	require.NotNil(t, options.Tools.Subagents.AllowHandoffs)
	assert.True(t, *options.Tools.Subagents.AllowHandoffs)
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

func TestAgentConfigExecutionOptions_MapsMemoryExternalContextPolicy(t *testing.T) {
	cfg := AgentConfig{
		Core: CoreConfig{ID: "agent-1", Name: "Agent", Type: "assistant"},
		LLM:  LLMConfig{Model: "gpt-4o"},
		Features: FeaturesConfig{
			Memory: &MemoryConfig{
				Enabled:                        true,
				DisableOnExternalContext:       true,
				DisableRecallOnExternalContext: true,
				DisableWriteOnExternalContext:  true,
			},
		},
	}

	options := cfg.ExecutionOptions()
	require.NotNil(t, options.Control.MemoryExternalContext)
	assert.True(t, options.Control.MemoryExternalContext.DisableAllOnExternalContext)
	assert.True(t, options.Control.MemoryExternalContext.DisableRecallOnExternalContext)
	assert.True(t, options.Control.MemoryExternalContext.DisableWriteOnExternalContext)
}

func TestAgentConfigExecutionOptions_FormalPoliciesOverrideLegacyRuntime(t *testing.T) {
	allowHandoffs := false
	cfg := AgentConfig{
		Core: CoreConfig{ID: "agent-1", Name: "Agent", Type: "assistant"},
		LLM:  LLMConfig{Model: "legacy-model"},
		Runtime: RuntimeConfig{
			ApprovalPolicy: "on-request",
			SandboxMode:    "workspace-write",
			Handoffs:       []string{"legacy-reviewer"},
		},
		Control: AgentControlOptions{
			ApprovalPolicy: "never",
			SandboxMode:    "danger-full-access",
		},
		Tools: ToolProtocolOptions{
			Handoffs:  []string{"formal-reviewer"},
			Subagents: &SubagentExecutionPolicy{AllowHandoffs: &allowHandoffs, MaxDepth: 1, MaxParallelism: 1},
		},
	}

	options := cfg.ExecutionOptions()
	assert.Equal(t, "never", options.Control.ApprovalPolicy)
	assert.Equal(t, "danger-full-access", options.Control.SandboxMode)
	assert.Equal(t, []string{"formal-reviewer"}, options.Tools.Handoffs)
	require.NotNil(t, options.Tools.Subagents)
	require.NotNil(t, options.Tools.Subagents.AllowHandoffs)
	assert.False(t, *options.Tools.Subagents.AllowHandoffs)
	assert.Equal(t, 1, options.Tools.Subagents.MaxDepth)
	assert.Equal(t, 1, options.Tools.Subagents.MaxParallelism)
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

func TestAgentConfigHasFormalMainFaceUsesGenericFormalSurfaceDetection(t *testing.T) {
	source, err := os.ReadFile("execution_options.go")
	require.NoError(t, err)

	body := string(source)
	start := strings.Index(body, "func (c AgentConfig) hasFormalMainFace() bool")
	require.NotEqual(t, -1, start)
	end := strings.Index(body[start:], "\nfunc mergeModelOptions")
	require.NotEqual(t, -1, end)
	fn := body[start : start+end]

	assert.Contains(t, fn, "formalSurfaceHasValues")
	assert.NotContains(t, fn, "c.Model.")
	assert.NotContains(t, fn, "c.Control.")
	assert.NotContains(t, fn, "c.Tools.")
}

func TestAgentConfigHasFormalMainFaceDetectsRepresentativeFormalFields(t *testing.T) {
	assert.False(t, (AgentConfig{}).hasFormalMainFace())
	assert.True(t, (AgentConfig{Model: ModelOptions{Model: "gpt-5.4"}}).hasFormalMainFace())
	assert.True(t, (AgentConfig{Control: AgentControlOptions{Timeout: 5 * time.Second}}).hasFormalMainFace())
	assert.True(t, (AgentConfig{Tools: ToolProtocolOptions{DisableTools: true}}).hasFormalMainFace())
}

func TestAgentConfigExecutionOptions_FormalModelFieldsAreMergedAndCloned(t *testing.T) {
	maxCompletionTokens := 2048
	frequencyPenalty := float32(0.2)
	presencePenalty := float32(0.3)
	repetitionPenalty := float32(1.1)
	n := 2
	logProbs := true
	topLogProbs := 3
	serviceTier := "priority"
	store := true
	thinkingBudget := int32(-1)
	includeThoughts := true
	cfg := AgentConfig{
		Core: CoreConfig{ID: "agent-1", Name: "Agent", Type: "assistant"},
		LLM:  LLMConfig{Model: "legacy-model"},
		Model: ModelOptions{
			Model:                "gpt-5.4",
			MaxCompletionTokens:  &maxCompletionTokens,
			FrequencyPenalty:     &frequencyPenalty,
			PresencePenalty:      &presencePenalty,
			RepetitionPenalty:    &repetitionPenalty,
			N:                    &n,
			LogProbs:             &logProbs,
			TopLogProbs:          &topLogProbs,
			User:                 "user-1",
			StreamOptions:        &StreamOptions{IncludeUsage: true},
			ServiceTier:          &serviceTier,
			ReasoningMode:        "thinking",
			ThinkingType:         "adaptive",
			ThinkingLevel:        "high",
			ThinkingBudget:       &thinkingBudget,
			IncludeThoughts:      &includeThoughts,
			MediaResolution:      "media_resolution_high",
			Store:                &store,
			Modalities:           []string{"text", "audio"},
			PromptCacheKey:       "cache-key",
			PromptCacheRetention: "24h",
			CacheControl:         &CacheControl{Type: "ephemeral", TTL: "5m"},
			CachedContent:        "cachedContents/session-1",
			Include:              []string{"reasoning.encrypted_content"},
			Truncation:           "auto",
			PreviousResponseID:   "resp_prev_1",
			ConversationID:       "conv_1",
			ThoughtSignatures:    []string{"sig-1"},
			Verbosity:            "low",
			Phase:                "commentary",
		},
	}

	options := cfg.ExecutionOptions()
	require.NotNil(t, options.Model.MaxCompletionTokens)
	assert.Equal(t, 2048, *options.Model.MaxCompletionTokens)
	require.NotNil(t, options.Model.FrequencyPenalty)
	assert.Equal(t, float32(0.2), *options.Model.FrequencyPenalty)
	require.NotNil(t, options.Model.PresencePenalty)
	assert.Equal(t, float32(0.3), *options.Model.PresencePenalty)
	require.NotNil(t, options.Model.RepetitionPenalty)
	assert.Equal(t, float32(1.1), *options.Model.RepetitionPenalty)
	require.NotNil(t, options.Model.N)
	assert.Equal(t, 2, *options.Model.N)
	require.NotNil(t, options.Model.LogProbs)
	assert.True(t, *options.Model.LogProbs)
	require.NotNil(t, options.Model.TopLogProbs)
	assert.Equal(t, 3, *options.Model.TopLogProbs)
	assert.Equal(t, "user-1", options.Model.User)
	require.NotNil(t, options.Model.StreamOptions)
	assert.True(t, options.Model.StreamOptions.IncludeUsage)
	require.NotNil(t, options.Model.ServiceTier)
	assert.Equal(t, "priority", *options.Model.ServiceTier)
	assert.Equal(t, "thinking", options.Model.ReasoningMode)
	assert.Equal(t, "adaptive", options.Model.ThinkingType)
	assert.Equal(t, "high", options.Model.ThinkingLevel)
	require.NotNil(t, options.Model.ThinkingBudget)
	assert.Equal(t, int32(-1), *options.Model.ThinkingBudget)
	require.NotNil(t, options.Model.IncludeThoughts)
	assert.True(t, *options.Model.IncludeThoughts)
	assert.Equal(t, "media_resolution_high", options.Model.MediaResolution)
	require.NotNil(t, options.Model.Store)
	assert.True(t, *options.Model.Store)
	assert.Equal(t, []string{"text", "audio"}, options.Model.Modalities)
	assert.Equal(t, "cache-key", options.Model.PromptCacheKey)
	assert.Equal(t, "24h", options.Model.PromptCacheRetention)
	require.NotNil(t, options.Model.CacheControl)
	assert.Equal(t, "ephemeral", options.Model.CacheControl.Type)
	assert.Equal(t, "cachedContents/session-1", options.Model.CachedContent)
	assert.Equal(t, []string{"reasoning.encrypted_content"}, options.Model.Include)
	assert.Equal(t, "auto", options.Model.Truncation)
	assert.Equal(t, "resp_prev_1", options.Model.PreviousResponseID)
	assert.Equal(t, "conv_1", options.Model.ConversationID)
	assert.Equal(t, []string{"sig-1"}, options.Model.ThoughtSignatures)
	assert.Equal(t, "low", options.Model.Verbosity)
	assert.Equal(t, "commentary", options.Model.Phase)

	*cfg.Model.MaxCompletionTokens = 1
	*cfg.Model.FrequencyPenalty = 0.9
	*cfg.Model.PresencePenalty = 0.9
	*cfg.Model.RepetitionPenalty = 0.9
	*cfg.Model.N = 9
	*cfg.Model.LogProbs = false
	*cfg.Model.TopLogProbs = 9
	cfg.Model.StreamOptions.IncludeUsage = false
	*cfg.Model.ServiceTier = "mutated"
	*cfg.Model.Store = false
	cfg.Model.Modalities[0] = "mutated"
	cfg.Model.CacheControl.Type = "mutated"
	cfg.Model.Include[0] = "mutated"
	cfg.Model.ThoughtSignatures[0] = "mutated"
	*cfg.Model.ThinkingBudget = 999
	*cfg.Model.IncludeThoughts = false

	assert.Equal(t, 2048, *options.Model.MaxCompletionTokens)
	assert.Equal(t, float32(0.2), *options.Model.FrequencyPenalty)
	assert.Equal(t, float32(0.3), *options.Model.PresencePenalty)
	assert.Equal(t, float32(1.1), *options.Model.RepetitionPenalty)
	assert.Equal(t, 2, *options.Model.N)
	assert.True(t, *options.Model.LogProbs)
	assert.Equal(t, 3, *options.Model.TopLogProbs)
	assert.True(t, options.Model.StreamOptions.IncludeUsage)
	assert.Equal(t, "priority", *options.Model.ServiceTier)
	assert.True(t, *options.Model.Store)
	assert.Equal(t, []string{"text", "audio"}, options.Model.Modalities)
	assert.Equal(t, "ephemeral", options.Model.CacheControl.Type)
	assert.Equal(t, []string{"reasoning.encrypted_content"}, options.Model.Include)
	assert.Equal(t, []string{"sig-1"}, options.Model.ThoughtSignatures)
	assert.Equal(t, int32(-1), *options.Model.ThinkingBudget)
	assert.True(t, *options.Model.IncludeThoughts)
}

func TestAgentConfigJSONUnmarshalNormalizesLegacyRuntimeSurface(t *testing.T) {
	payload := []byte(`{
		"core":{"id":"agent-1","name":"Agent","type":"assistant"},
		"llm":{"provider":"openai","model":"legacy-model","max_tokens":123,"temperature":0.3,"stop":["STOP"]},
		"runtime":{"system_prompt":"legacy prompt","tools":["search"],"handoffs":["reviewer"],"max_react_iterations":4,"tool_model":"tool-model"},
		"features":{"memory":{"enabled":true,"disable_on_external_context":true}},
		"metadata":{"tenant":"t1"}
	}`)

	var cfg AgentConfig
	require.NoError(t, json.Unmarshal(payload, &cfg))

	assert.Equal(t, "openai", cfg.Model.Provider)
	assert.Equal(t, "legacy-model", cfg.Model.Model)
	assert.Equal(t, 123, cfg.Model.MaxTokens)
	assert.Equal(t, float32(0.3), cfg.Model.Temperature)
	assert.Equal(t, []string{"STOP"}, cfg.Model.Stop)
	assert.Equal(t, "legacy prompt", cfg.Control.SystemPrompt)
	assert.Equal(t, 4, cfg.Control.MaxReActIterations)
	require.NotNil(t, cfg.Control.MemoryExternalContext)
	assert.True(t, cfg.Control.MemoryExternalContext.DisableAllOnExternalContext)
	assert.Equal(t, []string{"search"}, cfg.Tools.AllowedTools)
	assert.Equal(t, []string{"reviewer"}, cfg.Tools.Handoffs)
	assert.Equal(t, "tool-model", cfg.Tools.ToolModel)
	require.NotNil(t, cfg.Tools.Subagents)
	require.NotNil(t, cfg.Tools.Subagents.AllowHandoffs)
	assert.True(t, *cfg.Tools.Subagents.AllowHandoffs)

	options := cfg.ExecutionOptions()
	assert.Equal(t, cfg.Model, options.Model)
	assert.Equal(t, cfg.Control.SystemPrompt, options.Control.SystemPrompt)
	assert.Equal(t, cfg.Tools.AllowedTools, options.Tools.AllowedTools)
}

func TestAgentConfigJSONUnmarshalFormalSurfaceOverridesLegacyRuntimeSurface(t *testing.T) {
	payload := []byte(`{
		"core":{"id":"agent-1","name":"Agent","type":"assistant"},
		"model":{"provider":"formal-provider","model":"formal-model","max_tokens":456},
		"control":{"system_prompt":"formal prompt","max_react_iterations":7},
		"tools":{"allowed_tools":["formal-tool"],"tool_model":"formal-tool-model"},
		"llm":{"provider":"legacy-provider","model":"legacy-model","max_tokens":123},
		"runtime":{"system_prompt":"legacy prompt","tools":["legacy-tool"],"max_react_iterations":4,"tool_model":"legacy-tool-model"}
	}`)

	var cfg AgentConfig
	require.NoError(t, json.Unmarshal(payload, &cfg))

	assert.Equal(t, "formal-provider", cfg.Model.Provider)
	assert.Equal(t, "formal-model", cfg.Model.Model)
	assert.Equal(t, 456, cfg.Model.MaxTokens)
	assert.Equal(t, "formal prompt", cfg.Control.SystemPrompt)
	assert.Equal(t, 7, cfg.Control.MaxReActIterations)
	assert.Equal(t, []string{"formal-tool"}, cfg.Tools.AllowedTools)
	assert.Equal(t, "formal-tool-model", cfg.Tools.ToolModel)
}
