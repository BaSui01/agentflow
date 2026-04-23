package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/internal/usecase"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type openAICompatServiceStub struct {
	completeReq    *usecase.ChatRequest
	streamReq      *usecase.ChatRequest
	completeResult *usecase.ChatCompletionResult
	completeErr    *types.Error
	streamChunks   []usecase.ChatStreamEvent
	streamErr      *types.Error
}

func (s *openAICompatServiceStub) Complete(_ context.Context, req *usecase.ChatRequest) (*usecase.ChatCompletionResult, *types.Error) {
	s.completeReq = req
	if s.completeErr != nil {
		return nil, s.completeErr
	}
	return s.completeResult, nil
}

func (s *openAICompatServiceStub) Stream(_ context.Context, req *usecase.ChatRequest) (<-chan usecase.ChatStreamEvent, *types.Error) {
	s.streamReq = req
	if s.streamErr != nil {
		return nil, s.streamErr
	}
	ch := make(chan usecase.ChatStreamEvent, len(s.streamChunks))
	for _, item := range s.streamChunks {
		ch <- item
	}
	close(ch)
	return ch, nil
}

func (s *openAICompatServiceStub) SupportedRoutePolicies() []string {
	return []string{"balanced", "cost_first", "health_first", "latency_first"}
}

func (s *openAICompatServiceStub) DefaultRoutePolicy() string {
	return "balanced"
}

func TestChatHandler_OpenAICompatChatCompletions(t *testing.T) {
	svc := &openAICompatServiceStub{
		completeResult: &usecase.ChatCompletionResult{
			Response: &usecase.ChatResponse{
				ID:    "chatcmpl_test",
				Model: "gpt-5.2",
				Choices: []usecase.ChatChoice{
					{
						Index:        0,
						FinishReason: "stop",
						Message: usecase.Message{
							Role:    "assistant",
							Content: "ok",
						},
					},
				},
				Usage: usecase.ChatUsage{
					PromptTokens:     10,
					CompletionTokens: 5,
					TotalTokens:      15,
				},
				CreatedAt: time.Unix(1700000000, 0),
			},
		},
	}
	handler := NewChatHandler(svc, zap.NewNop())

	body := []byte(`{
		"model":"gpt-5.2",
		"messages":[{"role":"user","content":"hello"}],
		"response_format":{"type":"json_object"},
		"reasoning_effort":"low",
		"parallel_tool_calls":true,
		"service_tier":"auto",
		"user":"compat-user-1",
		"max_completion_tokens":256,
		"store":true,
		"previous_response_id":"resp_prev_123",
		"conversation_id":"conv_chat_123",
		"web_search_options":{
			"search_context_size":"high",
			"user_location":{
				"type":"approximate",
				"approximate":{"country":"US","city":"San Francisco","timezone":"America/Los_Angeles"}
			}
		},
		"provider":"openai",
		"endpoint_mode":"chat_completions"
	}`)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")

	handler.HandleOpenAICompatChatCompletions(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, svc.completeReq)
	assert.Equal(t, "openai", svc.completeReq.Provider)
	assert.Equal(t, "chat_completions", svc.completeReq.EndpointMode)
	require.NotNil(t, svc.completeReq.ResponseFormat)
	assert.Equal(t, "json_object", svc.completeReq.ResponseFormat.Type)
	assert.Equal(t, "low", svc.completeReq.ReasoningEffort)
	require.NotNil(t, svc.completeReq.ParallelToolCalls)
	assert.True(t, *svc.completeReq.ParallelToolCalls)
	require.NotNil(t, svc.completeReq.ServiceTier)
	assert.Equal(t, "auto", *svc.completeReq.ServiceTier)
	assert.Equal(t, "compat-user-1", svc.completeReq.User)
	require.NotNil(t, svc.completeReq.MaxCompletionTokens)
	assert.Equal(t, 256, *svc.completeReq.MaxCompletionTokens)
	require.NotNil(t, svc.completeReq.Store)
	assert.True(t, *svc.completeReq.Store)
	assert.Equal(t, "resp_prev_123", svc.completeReq.PreviousResponseID)
	assert.Equal(t, "conv_chat_123", svc.completeReq.ConversationID)
	require.NotNil(t, svc.completeReq.WebSearchOptions)
	assert.Equal(t, "high", svc.completeReq.WebSearchOptions.SearchContextSize)
	require.NotNil(t, svc.completeReq.WebSearchOptions.UserLocation)
	assert.Equal(t, "approximate", svc.completeReq.WebSearchOptions.UserLocation.Type)
	assert.Equal(t, "US", svc.completeReq.WebSearchOptions.UserLocation.Country)
	assert.Equal(t, "San Francisco", svc.completeReq.WebSearchOptions.UserLocation.City)
	assert.Equal(t, "America/Los_Angeles", svc.completeReq.WebSearchOptions.UserLocation.Timezone)

	var resp openAICompatChatResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "chat.completion", resp.Object)
	assert.Equal(t, "gpt-5.2", resp.Model)
	require.Len(t, resp.Choices, 1)
	assert.Equal(t, "ok", resp.Choices[0].Message.Content)
}

func TestChatHandler_OpenAICompatChatCompletions_Stream(t *testing.T) {
	svc := &openAICompatServiceStub{
		streamChunks: []usecase.ChatStreamEvent{
			{
				Chunk: &usecase.ChatStreamChunk{
					ID:    "chunk_1",
					Model: "gpt-5.2",
					Index: 0,
					Delta: usecase.Message{
						Role:    string(llmcore.RoleAssistant),
						Content: "ok",
					},
					FinishReason: "stop",
				},
			},
		},
	}
	handler := NewChatHandler(svc, zap.NewNop())

	body := []byte(`{
		"model":"gpt-5.2",
		"stream":true,
		"messages":[{"role":"user","content":"hello"}]
	}`)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")

	handler.HandleOpenAICompatChatCompletions(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Body.String(), "chat.completion.chunk")
	assert.Contains(t, w.Body.String(), "data: [DONE]")
}

func TestChatHandler_OpenAICompatResponses(t *testing.T) {
	svc := &openAICompatServiceStub{
		completeResult: &usecase.ChatCompletionResult{
			Response: &usecase.ChatResponse{
				ID:    "resp_test",
				Model: "gpt-5.2",
				Choices: []usecase.ChatChoice{
					{
						Index:        0,
						FinishReason: "stop",
						Message: usecase.Message{
							Role:    "assistant",
							Content: "done",
							ReasoningSummaries: []types.ReasoningSummary{
								{Provider: "openai", ID: "rs_resp_1", Kind: "summary_text", Text: "summary text"},
							},
							OpaqueReasoning: []types.OpaqueReasoning{
								{Provider: "openai", ID: "rs_resp_1", Kind: "encrypted_content", State: "enc_resp"},
							},
						},
					},
				},
				Usage: usecase.ChatUsage{
					PromptTokens:     8,
					CompletionTokens: 3,
					TotalTokens:      11,
				},
				CreatedAt: time.Unix(1700000001, 0),
			},
		},
	}
	handler := NewChatHandler(svc, zap.NewNop())

	body := []byte(`{
		"model":"gpt-5.2",
		"input":"hello from responses",
		"reasoning":{"effort":"medium","summary":"detailed"},
		"text":{"format":{"type":"json_schema","json_schema":{"name":"resp_fmt","schema":{"type":"object"},"strict":true}}},
		"parallel_tool_calls":true,
		"service_tier":"default",
		"user":"resp-user-1",
		"store":true,
		"prompt_cache_key":"route-a",
		"prompt_cache_retention":"24h",
		"previous_response_id":"resp_prev_456",
		"conversation":"conv_resp_456",
		"truncation":"auto",
		"include":["output_text"],
		"tools":[
			{
				"type":"web_search",
				"search_context_size":"low",
				"user_location":{"type":"approximate","country":"CN","city":"Hangzhou"},
				"filters":{"allowed_domains":["example.com","docs.example.com"]}
			}
		],
		"web_search_options":{
			"search_context_size":"high",
			"user_location":{
				"type":"approximate",
				"approximate":{"country":"CN","city":"Shanghai","timezone":"Asia/Shanghai"}
			},
			"allowed_domains":["openai.com"]
		}
	}`)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")

	handler.HandleOpenAICompatResponses(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, svc.completeReq)
	assert.Equal(t, "responses", svc.completeReq.EndpointMode)
	require.Len(t, svc.completeReq.Messages, 1)
	assert.Equal(t, "user", svc.completeReq.Messages[0].Role)
	assert.Equal(t, "hello from responses", svc.completeReq.Messages[0].Content)
	require.NotNil(t, svc.completeReq.ResponseFormat)
	assert.Equal(t, "json_schema", svc.completeReq.ResponseFormat.Type)
	require.NotNil(t, svc.completeReq.ResponseFormat.JSONSchema)
	assert.Equal(t, "resp_fmt", svc.completeReq.ResponseFormat.JSONSchema.Name)
	assert.Equal(t, "medium", svc.completeReq.ReasoningEffort)
	assert.Equal(t, "detailed", svc.completeReq.ReasoningSummary)
	require.NotNil(t, svc.completeReq.ParallelToolCalls)
	assert.True(t, *svc.completeReq.ParallelToolCalls)
	require.NotNil(t, svc.completeReq.ServiceTier)
	assert.Equal(t, "default", *svc.completeReq.ServiceTier)
	assert.Equal(t, "resp-user-1", svc.completeReq.User)
	require.NotNil(t, svc.completeReq.Store)
	assert.True(t, *svc.completeReq.Store)
	assert.Equal(t, "route-a", svc.completeReq.PromptCacheKey)
	assert.Equal(t, "24h", svc.completeReq.PromptCacheRetention)
	assert.Equal(t, "resp_prev_456", svc.completeReq.PreviousResponseID)
	assert.Equal(t, "conv_resp_456", svc.completeReq.ConversationID)
	assert.Equal(t, "auto", svc.completeReq.Truncation)
	assert.Equal(t, []string{"output_text"}, svc.completeReq.Include)
	require.NotNil(t, svc.completeReq.WebSearchOptions)
	assert.Equal(t, "high", svc.completeReq.WebSearchOptions.SearchContextSize)
	require.NotNil(t, svc.completeReq.WebSearchOptions.UserLocation)
	assert.Equal(t, "approximate", svc.completeReq.WebSearchOptions.UserLocation.Type)
	assert.Equal(t, "CN", svc.completeReq.WebSearchOptions.UserLocation.Country)
	assert.Equal(t, "Shanghai", svc.completeReq.WebSearchOptions.UserLocation.City)
	assert.Equal(t, "Asia/Shanghai", svc.completeReq.WebSearchOptions.UserLocation.Timezone)
	assert.Equal(t, []string{"openai.com"}, svc.completeReq.WebSearchOptions.AllowedDomains)

	var resp openAICompatResponsesResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "response", resp.Object)
	assert.Equal(t, "gpt-5.2", resp.Model)
	require.NotEmpty(t, resp.Output)
	assert.Equal(t, "reasoning", resp.Output[0].Type)
	assert.Equal(t, "enc_resp", resp.Output[0].EncryptedContent)
	require.Len(t, resp.Output[0].Summary, 1)
	assert.Equal(t, "summary text", resp.Output[0].Summary[0].Text)
	assert.Equal(t, "message", resp.Output[1].Type)
}

func TestConvertOpenAICompatInboundTools_PreservesStrict(t *testing.T) {
	strict := true
	tools, ws, err := convertOpenAICompatInboundTools([]openAICompatInboundTool{
		{
			Type: "function",
			Function: struct {
				Name        string `json:"name,omitempty"`
				Description string `json:"description,omitempty"`
				Parameters  any    `json:"parameters,omitempty"`
				Strict      *bool  `json:"strict,omitempty"`
			}{
				Name:        "lookup_weather",
				Description: "Lookup weather",
				Parameters:  map[string]any{"type": "object"},
				Strict:      &strict,
			},
		},
	})
	require.Nil(t, err)
	require.Nil(t, ws)
	require.Len(t, tools, 1)
	require.NotNil(t, tools[0].Strict)
	assert.True(t, *tools[0].Strict)
}

func TestConvertOpenAICompatInboundTools_CustomTool(t *testing.T) {
	tools, ws, err := convertOpenAICompatInboundTools([]openAICompatInboundTool{{
		Type: "custom",
		Custom: struct {
			Name        string `json:"name,omitempty"`
			Description string `json:"description,omitempty"`
			Format      any    `json:"format,omitempty"`
		}{
			Name:        "code_exec",
			Description: "Execute code",
			Format: map[string]any{
				"type":       "grammar",
				"syntax":     "lark",
				"definition": "start: WORD",
			},
		},
	}})
	require.Nil(t, err)
	require.Nil(t, ws)
	require.Len(t, tools, 1)
	assert.Equal(t, types.ToolTypeCustom, tools[0].Type)
	require.NotNil(t, tools[0].Format)
	assert.Equal(t, "grammar", tools[0].Format.Type)
}

func TestConvertOpenAICompatResponsesInput_CustomToolCall(t *testing.T) {
	input := []any{
		map[string]any{
			"type":    "custom_tool_call",
			"call_id": "call_custom_1",
			"name":    "code_exec",
			"input":   "print('hi')",
		},
		map[string]any{
			"type":    "custom_tool_call_output",
			"call_id": "call_custom_1",
			"output":  "done",
		},
	}
	msgs, err := convertOpenAICompatResponsesInput(input)
	require.Nil(t, err)
	require.Len(t, msgs, 2)
	require.Len(t, msgs[0].ToolCalls, 1)
	assert.Equal(t, types.ToolTypeCustom, msgs[0].ToolCalls[0].Type)
	assert.Equal(t, "code_exec", msgs[0].ToolCalls[0].Name)
	assert.Equal(t, "print('hi')", msgs[0].ToolCalls[0].Input)
	assert.Equal(t, "call_custom_1", msgs[1].ToolCallID)
}

func TestChatHandler_OpenAICompatResponses_Stream(t *testing.T) {
	svc := &openAICompatServiceStub{
		streamChunks: []usecase.ChatStreamEvent{
			{
				Chunk: &usecase.ChatStreamChunk{
					ID:    "resp_stream_1",
					Model: "gpt-5.2",
					Delta: usecase.Message{
						Role:    string(llmcore.RoleAssistant),
						Content: "hello",
					},
					FinishReason: "stop",
				},
			},
		},
	}
	handler := NewChatHandler(svc, zap.NewNop())

	body := []byte(`{
		"model":"gpt-5.2",
		"stream":true,
		"input":"hello"
	}`)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")

	handler.HandleOpenAICompatResponses(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
	text := w.Body.String()
	assert.True(t, strings.Contains(text, "response.created"))
	assert.True(t, strings.Contains(text, "response.output_text.delta"))
	assert.True(t, strings.Contains(text, "data: [DONE]"))
}

func TestChatHandler_OpenAICompatResponses_WebSearchToolFilters(t *testing.T) {
	svc := &openAICompatServiceStub{
		completeResult: &usecase.ChatCompletionResult{
			Response: &usecase.ChatResponse{
				ID: "resp_filters",
				Choices: []usecase.ChatChoice{
					{
						Index: 0,
						Message: usecase.Message{
							Role:    "assistant",
							Content: "ok",
						},
					},
				},
			},
		},
	}
	handler := NewChatHandler(svc, zap.NewNop())

	body := []byte(`{
		"model":"gpt-5.2",
		"input":"hello",
		"tools":[
			{
				"type":"web_search",
				"search_context_size":"low",
				"user_location":{"type":"approximate","country":"CN","city":"Beijing"},
				"filters":{"allowed_domains":["example.com","docs.example.com"]}
			}
		]
	}`)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")

	handler.HandleOpenAICompatResponses(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, svc.completeReq)
	require.NotNil(t, svc.completeReq.WebSearchOptions)
	assert.Equal(t, "low", svc.completeReq.WebSearchOptions.SearchContextSize)
	require.NotNil(t, svc.completeReq.WebSearchOptions.UserLocation)
	assert.Equal(t, "CN", svc.completeReq.WebSearchOptions.UserLocation.Country)
	assert.Equal(t, "Beijing", svc.completeReq.WebSearchOptions.UserLocation.City)
	assert.Equal(t, []string{"example.com", "docs.example.com"}, svc.completeReq.WebSearchOptions.AllowedDomains)
}

func TestChatHandler_OpenAICompatResponses_InputReasoningRoundTrip(t *testing.T) {
	svc := &openAICompatServiceStub{
		completeResult: &usecase.ChatCompletionResult{
			Response: &usecase.ChatResponse{
				ID:    "resp_test",
				Model: "gpt-5.2",
				Choices: []usecase.ChatChoice{{
					Index: 0,
					Message: usecase.Message{
						Role:    "assistant",
						Content: "done",
					},
				}},
			},
		},
	}
	handler := NewChatHandler(svc, zap.NewNop())

	body := []byte(`{
		"model":"gpt-5.2",
		"input":[
			{"type":"reasoning","id":"rs_prev","status":"completed","summary":[{"type":"summary_text","text":"previous summary"}],"encrypted_content":"enc_prev"},
			{"role":"user","content":"next question"}
		]
	}`)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")

	handler.HandleOpenAICompatResponses(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, svc.completeReq)
	require.Len(t, svc.completeReq.Messages, 2)
	require.NotNil(t, svc.completeReq.Messages[0].ReasoningContent)
	assert.Equal(t, "previous summary", *svc.completeReq.Messages[0].ReasoningContent)
	require.Len(t, svc.completeReq.Messages[0].ReasoningSummaries, 1)
	assert.Equal(t, "summary_text", svc.completeReq.Messages[0].ReasoningSummaries[0].Kind)
	require.Len(t, svc.completeReq.Messages[0].OpaqueReasoning, 1)
	assert.Equal(t, "enc_prev", svc.completeReq.Messages[0].OpaqueReasoning[0].State)
	assert.Equal(t, "next question", svc.completeReq.Messages[1].Content)
}
