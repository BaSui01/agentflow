package vendor

import (
	"testing"

	"github.com/BaSui01/agentflow/llm"
	providerbase "github.com/BaSui01/agentflow/llm/providers/base"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQwenRequestHook_ThinkingModeUsesLatestQwen3AndCompatFlags(t *testing.T) {
	req := &llm.ChatRequest{ReasoningMode: "thinking"}
	body := &providerbase.OpenAICompatRequest{Model: "qwen3-235b-a22b"}

	qwenRequestHook(req, body)

	assert.Equal(t, "qwen3-max-2026-01-23", body.Model)
	require.NotNil(t, body.EnableThinking)
	assert.True(t, *body.EnableThinking)
	require.NotNil(t, body.IncrementalOutput)
	assert.True(t, *body.IncrementalOutput)
}

func TestQwenRequestHook_ThinkingModeDefaultsToSnapshotWhenModelEmpty(t *testing.T) {
	req := &llm.ChatRequest{ReasoningMode: "extended"}
	body := &providerbase.OpenAICompatRequest{}

	qwenRequestHook(req, body)

	assert.Equal(t, "qwen3-max-2026-01-23", body.Model)
}

func TestGrokRequestHook_ReasoningModeUsesLatestReasoningModelAndStripsUnsupportedFields(t *testing.T) {
	body := &providerbase.OpenAICompatRequest{}

	grokRequestHook(&llm.ChatRequest{ReasoningMode: "thinking"}, body)

	assert.Equal(t, "grok-4.20-reasoning", body.Model)
}

func TestKimiRequestHook_ThinkingModeUsesK25AndThinkingObject(t *testing.T) {
	body := &providerbase.OpenAICompatRequest{}

	kimiRequestHook(&llm.ChatRequest{ReasoningMode: "thinking"}, body)

	assert.Equal(t, "kimi-k2.5", body.Model)
	require.NotNil(t, body.Thinking)
	assert.Equal(t, "enabled", body.Thinking.Type)
}

func TestKimiRequestHook_DisabledReasoningUsesK25DisabledThinking(t *testing.T) {
	body := &providerbase.OpenAICompatRequest{}

	kimiRequestHook(&llm.ChatRequest{ReasoningMode: "disabled"}, body)

	assert.Equal(t, "kimi-k2.5", body.Model)
	require.NotNil(t, body.Thinking)
	assert.Equal(t, "disabled", body.Thinking.Type)
}

func TestValidateQwenRequest_RejectsStructuredOutputInThinkingMode(t *testing.T) {
	err := validateQwenRequest(&llm.ChatRequest{
		ReasoningMode: "thinking",
		ResponseFormat: &llm.ResponseFormat{
			Type: llm.ResponseFormatJSONObject,
		},
	}, &providerbase.OpenAICompatRequest{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "structured JSON response_format")
}

func TestValidateGrokRequest_RejectsUnsupportedReasoningFields(t *testing.T) {
	freq := float32(0.5)
	err := validateGrokRequest(&llm.ChatRequest{
		Model:            "grok-4.20-reasoning",
		FrequencyPenalty: &freq,
	}, &providerbase.OpenAICompatRequest{Model: "grok-4.20-reasoning"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "frequency_penalty")
}

func TestValidateKimiRequest_RejectsToolChoiceAndSamplingInThinkingMode(t *testing.T) {
	n := 2
	err := validateKimiRequest(&llm.ChatRequest{
		ReasoningMode: "thinking",
		Temperature:   0.7,
		N:             &n,
		ToolChoice:    &types.ToolChoice{Mode: types.ToolChoiceModeRequired},
	}, &providerbase.OpenAICompatRequest{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "tool_choice auto")
}
