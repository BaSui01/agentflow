package runtime

import (
	"strings"
	"testing"

	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConsumeDirectStreamChunkAccumulatesContentReasoningAndProviderState(t *testing.T) {
	reasoning := "think"
	events := make([]RuntimeStreamEvent, 0, 2)
	result := &directStreamingAttemptResult{}
	var reasoningBuf strings.Builder

	consumeDirectStreamChunk(func(event RuntimeStreamEvent) {
		events = append(events, event)
	}, result, &reasoningBuf, types.StreamChunk{
		ID:           "chunk-1",
		Provider:     "openai",
		Model:        "gpt-5.4",
		FinishReason: "stop",
		Usage:        &types.ChatUsage{PromptTokens: 1, CompletionTokens: 2, TotalTokens: 3},
		Delta: types.Message{
			Content:          "hello",
			ReasoningContent: &reasoning,
			ReasoningSummaries: []types.ReasoningSummary{{
				Provider: "openai",
				Text:     "summary",
			}},
			OpaqueReasoning: []types.OpaqueReasoning{{
				Provider: "openai",
				Kind:     "encrypted",
				State:    "state",
			}},
			ThinkingBlocks: []types.ThinkingBlock{{
				Thinking:  "hidden",
				Signature: "sig",
			}},
		},
	})

	assert.Equal(t, "chunk-1", result.lastID)
	assert.Equal(t, "openai", result.lastProvider)
	assert.Equal(t, "gpt-5.4", result.lastModel)
	assert.Equal(t, "stop", result.lastFinishReason)
	require.NotNil(t, result.lastUsage)
	assert.Equal(t, 3, result.lastUsage.TotalTokens)
	assert.Equal(t, "hello", result.assembled.Content)
	assert.Equal(t, "think", reasoningBuf.String())
	require.Len(t, result.assembled.ReasoningSummaries, 1)
	require.Len(t, result.assembled.OpaqueReasoning, 1)
	require.Len(t, result.assembled.ThinkingBlocks, 1)
	require.Len(t, events, 2)
	assert.Equal(t, RuntimeStreamToken, events[0].Type)
	assert.Equal(t, RuntimeStreamReasoning, events[1].Type)
}

func TestFinalizeDirectStreamingResponseEmitsMessageAndLoopStop(t *testing.T) {
	events := make([]RuntimeStreamEvent, 0, 3)
	resp := finalizeDirectStreamingResponse(func(event RuntimeStreamEvent) {
		events = append(events, event)
	}, &directStreamingAttemptResult{
		assembled:        types.Message{Content: "done"},
		lastID:           "chatcmpl-1",
		lastProvider:     "openai",
		lastModel:        "gpt-5.4",
		lastFinishReason: "stop",
		reasoning:        "reason",
	}, types.ChatUsage{TotalTokens: 7})

	require.NotNil(t, resp)
	assert.Equal(t, "chatcmpl-1", resp.ID)
	assert.Equal(t, "openai", resp.Provider)
	assert.Equal(t, "gpt-5.4", resp.Model)
	assert.Equal(t, 7, resp.Usage.TotalTokens)
	require.Len(t, resp.Choices, 1)
	assert.Equal(t, types.RoleAssistant, resp.Choices[0].Message.Role)
	assert.Equal(t, "done", resp.Choices[0].Message.Content)
	require.NotNil(t, resp.Choices[0].Message.ReasoningContent)
	assert.Equal(t, "reason", *resp.Choices[0].Message.ReasoningContent)

	require.Len(t, events, 3)
	assert.Equal(t, SDKMessageOutputCreated, events[0].SDKEventName)
	assert.Equal(t, "completion_judge_decision", events[1].Data.(map[string]any)["status"])
	assert.Equal(t, "loop_stopped", events[2].Data.(map[string]any)["status"])
}
