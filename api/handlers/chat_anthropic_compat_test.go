package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/internal/usecase"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestChatHandler_AnthropicCompatMessages(t *testing.T) {
	svc := &openAICompatServiceStub{
		completeResult: &usecase.ChatCompletionResult{
			Response: &usecase.ChatResponse{
				ID:    "msg_test_1",
				Model: "claude-sonnet-4-20250514",
				Choices: []usecase.ChatChoice{
					{
						Index:        0,
						FinishReason: "tool_calls",
						Message: usecase.Message{
							Role:    "assistant",
							Content: "让我先查一下",
							ToolCalls: []types.ToolCall{
								{
									ID:        "toolu_1",
									Type:      types.ToolTypeFunction,
									Name:      "get_weather",
									Arguments: json.RawMessage(`{"city":"Shanghai"}`),
								},
							},
						},
					},
				},
				Usage: usecase.ChatUsage{
					PromptTokens:     12,
					CompletionTokens: 7,
					TotalTokens:      19,
				},
				CreatedAt: time.Unix(1700000100, 0),
			},
		},
	}
	handler := NewChatHandler(svc, zap.NewNop())

	body := []byte(`{
		"model":"claude-sonnet-4-20250514",
		"max_tokens":256,
		"system":[{"type":"text","text":"Be helpful"}],
		"messages":[{"role":"user","content":"天气怎么样？"}],
		"tools":[{"name":"get_weather","description":"Get weather","input_schema":{"type":"object","properties":{"city":{"type":"string"}}}}],
		"tool_choice":{"type":"tool","name":"get_weather"},
		"thinking":{"type":"adaptive","display":"summarized","budget_tokens":1024},
		"metadata":{"user_id":"anthropic-user-1"},
		"top_k":50,
		"inference_speed":"fast"
	}`)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")

	handler.HandleAnthropicCompatMessages(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, svc.completeReq)
	require.Len(t, svc.completeReq.Messages, 2)
	assert.Equal(t, "system", svc.completeReq.Messages[0].Role)
	assert.Equal(t, "Be helpful", svc.completeReq.Messages[0].Content)
	assert.Equal(t, "user", svc.completeReq.Messages[1].Role)
	assert.Equal(t, "天气怎么样？", svc.completeReq.Messages[1].Content)
	require.Len(t, svc.completeReq.Tools, 1)
	assert.Equal(t, "get_weather", svc.completeReq.Tools[0].Name)
	choice, ok := svc.completeReq.ToolChoice.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "tool", choice["type"])
	assert.Equal(t, "get_weather", choice["name"])
	assert.Equal(t, "anthropic-user-1", svc.completeReq.User)
	assert.Equal(t, "summarized", svc.completeReq.ReasoningDisplay)
	assert.Equal(t, "fast", svc.completeReq.InferenceSpeed)
	require.NotNil(t, svc.completeReq.Metadata)
	assert.Equal(t, "adaptive", svc.completeReq.Metadata["reasoning_mode"])
	assert.Equal(t, "50", svc.completeReq.Metadata["anthropic_top_k"])
	assert.Equal(t, "1024", svc.completeReq.Metadata["anthropic_thinking_budget_tokens"])

	var resp anthropicCompatMessageResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "message", resp.Type)
	assert.Equal(t, "assistant", resp.Role)
	assert.Equal(t, "claude-sonnet-4-20250514", resp.Model)
	assert.Equal(t, "tool_use", resp.StopReason)
	assert.Equal(t, 12, resp.Usage.InputTokens)
	assert.Equal(t, 7, resp.Usage.OutputTokens)
	require.Len(t, resp.Content, 2)
	assert.Equal(t, "text", resp.Content[0].Type)
	assert.Equal(t, "让我先查一下", resp.Content[0].Text)
	assert.Equal(t, "tool_use", resp.Content[1].Type)
	assert.Equal(t, "toolu_1", resp.Content[1].ID)
	assert.Equal(t, "get_weather", resp.Content[1].Name)
}

func TestBuildAPIChatRequestFromAnthropicMessages_ToolRoundTrip(t *testing.T) {
	req := anthropicCompatMessagesRequest{
		Model:     "claude-sonnet-4-20250514",
		MaxTokens: 128,
		Messages: []anthropicCompatInboundMessage{
			{
				Role: "assistant",
				Content: []any{
					map[string]any{
						"type": "tool_use",
						"id":   "toolu_1",
						"name": "lookup_weather",
						"input": map[string]any{
							"city": "Hangzhou",
						},
					},
				},
			},
			{
				Role: "user",
				Content: []any{
					map[string]any{
						"type":        "tool_result",
						"tool_use_id": "toolu_1",
						"content": map[string]any{
							"temperature": 22,
						},
					},
				},
			},
		},
	}

	apiReq, err := buildAPIChatRequestFromAnthropicMessages(req)
	require.Nil(t, err)
	require.Len(t, apiReq.Messages, 2)
	assert.Equal(t, "assistant", apiReq.Messages[0].Role)
	require.Len(t, apiReq.Messages[0].ToolCalls, 1)
	assert.Equal(t, "lookup_weather", apiReq.Messages[0].ToolCalls[0].Name)
	assert.JSONEq(t, `{"city":"Hangzhou"}`, string(apiReq.Messages[0].ToolCalls[0].Arguments))
	assert.Equal(t, "tool", apiReq.Messages[1].Role)
	assert.Equal(t, "toolu_1", apiReq.Messages[1].ToolCallID)
	assert.JSONEq(t, `{"temperature":22}`, apiReq.Messages[1].Content)
}

func TestAnthropicCompatMessagesToolCallRoundTrip(t *testing.T) {
	req := anthropicCompatMessagesRequest{
		Model:     "claude-sonnet-4-20250514",
		MaxTokens: 64,
		Messages: []anthropicCompatInboundMessage{
			{
				Role: "assistant",
				Content: []any{
					map[string]any{
						"type": "tool_use",
						"id":   "toolu_2",
						"name": "lookup_weather",
						"input": map[string]any{
							"city": "Beijing",
						},
					},
				},
			},
		},
	}

	apiReq, err := buildAPIChatRequestFromAnthropicMessages(req)
	require.Nil(t, err)
	require.Len(t, apiReq.Messages, 1)
	require.Len(t, apiReq.Messages[0].ToolCalls, 1)
	assert.Equal(t, "lookup_weather", apiReq.Messages[0].ToolCalls[0].Name)
	assert.JSONEq(t, `{"city":"Beijing"}`, string(apiReq.Messages[0].ToolCalls[0].Arguments))
}

func TestAnthropicCompatMessagesToolResultRoundTrip(t *testing.T) {
	req := anthropicCompatMessagesRequest{
		Model:     "claude-sonnet-4-20250514",
		MaxTokens: 64,
		Messages: []anthropicCompatInboundMessage{
			{
				Role: "user",
				Content: []any{
					map[string]any{
						"type":        "tool_result",
						"tool_use_id": "toolu_2",
						"content": map[string]any{
							"temperature": 26,
						},
					},
				},
			},
		},
	}

	apiReq, err := buildAPIChatRequestFromAnthropicMessages(req)
	require.Nil(t, err)
	require.Len(t, apiReq.Messages, 1)
	assert.Equal(t, "tool", apiReq.Messages[0].Role)
	assert.Equal(t, "toolu_2", apiReq.Messages[0].ToolCallID)
	assert.JSONEq(t, `{"temperature":26}`, apiReq.Messages[0].Content)
}

func TestChatHandler_AnthropicCompatMessages_Stream(t *testing.T) {
	svc := &openAICompatServiceStub{
		streamChunks: []usecase.ChatStreamEvent{
			{
				Chunk: &usecase.ChatStreamChunk{
					ID:    "chunk_1",
					Model: "claude-sonnet-4-20250514",
					Index: 0,
					Delta: usecase.Message{
						Role:    "assistant",
						Content: "hello",
					},
				},
			},
			{
				Chunk: &usecase.ChatStreamChunk{
					ID:           "chunk_2",
					Model:        "claude-sonnet-4-20250514",
					Index:        0,
					FinishReason: "stop",
					Usage: &usecase.ChatUsage{
						PromptTokens:     10,
						CompletionTokens: 5,
						TotalTokens:      15,
					},
				},
			},
		},
	}
	handler := NewChatHandler(svc, zap.NewNop())

	body := []byte(`{
		"model":"claude-sonnet-4-20250514",
		"max_tokens":128,
		"stream":true,
		"messages":[{"role":"user","content":"hello"}]
	}`)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")

	handler.HandleAnthropicCompatMessages(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
	text := w.Body.String()
	assert.Contains(t, text, "event: message_start")
	assert.Contains(t, text, "event: content_block_delta")
	assert.Contains(t, text, "event: message_delta")
	assert.Contains(t, text, "event: message_stop")
}

func TestChatHandler_AnthropicCompatMessages_Error(t *testing.T) {
	handler := NewChatHandler(&openAICompatServiceStub{}, zap.NewNop())

	body := []byte(`{
		"model":"claude-sonnet-4-20250514",
		"messages":[{"role":"user","content":"hello"}]
	}`)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")

	handler.HandleAnthropicCompatMessages(w, r)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var resp anthropicCompatErrorEnvelope
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "error", resp.Type)
	assert.Equal(t, "invalid_request_error", resp.Error.Type)
	assert.True(t, strings.Contains(resp.Error.Message, "max_tokens"))
}
