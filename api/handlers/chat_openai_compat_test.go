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

	"github.com/BaSui01/agentflow/api"
	"github.com/BaSui01/agentflow/internal/usecase"
	"github.com/BaSui01/agentflow/llm"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type openAICompatServiceStub struct {
	completeReq    *api.ChatRequest
	streamReq      *api.ChatRequest
	completeResult *usecase.ChatCompletionResult
	completeErr    *types.Error
	streamChunks   []llmcore.UnifiedChunk
	streamErr      *types.Error
}

func (s *openAICompatServiceStub) Complete(_ context.Context, req *api.ChatRequest) (*usecase.ChatCompletionResult, *types.Error) {
	s.completeReq = req
	if s.completeErr != nil {
		return nil, s.completeErr
	}
	return s.completeResult, nil
}

func (s *openAICompatServiceStub) Stream(_ context.Context, req *api.ChatRequest) (<-chan llmcore.UnifiedChunk, *types.Error) {
	s.streamReq = req
	if s.streamErr != nil {
		return nil, s.streamErr
	}
	ch := make(chan llmcore.UnifiedChunk, len(s.streamChunks))
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
			Response: &api.ChatResponse{
				ID:    "chatcmpl_test",
				Model: "gpt-5.2",
				Choices: []api.ChatChoice{
					{
						Index:        0,
						FinishReason: "stop",
						Message: api.Message{
							Role:    "assistant",
							Content: "ok",
						},
					},
				},
				Usage: api.ChatUsage{
					PromptTokens:     10,
					CompletionTokens: 5,
					TotalTokens:      15,
				},
				CreatedAt: time.Unix(1700000000, 0),
			},
		},
	}
	handler := NewChatHandlerWithService(svc, zap.NewNop())

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
		streamChunks: []llmcore.UnifiedChunk{
			{
				Output: &llm.StreamChunk{
					ID:    "chunk_1",
					Model: "gpt-5.2",
					Index: 0,
					Delta: types.Message{
						Role:    llm.RoleAssistant,
						Content: "ok",
					},
					FinishReason: "stop",
				},
			},
		},
	}
	handler := NewChatHandlerWithService(svc, zap.NewNop())

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
			Response: &api.ChatResponse{
				ID:    "resp_test",
				Model: "gpt-5.2",
				Choices: []api.ChatChoice{
					{
						Index:        0,
						FinishReason: "stop",
						Message: api.Message{
							Role:    "assistant",
							Content: "done",
						},
					},
				},
				Usage: api.ChatUsage{
					PromptTokens:     8,
					CompletionTokens: 3,
					TotalTokens:      11,
				},
				CreatedAt: time.Unix(1700000001, 0),
			},
		},
	}
	handler := NewChatHandlerWithService(svc, zap.NewNop())

	body := []byte(`{
		"model":"gpt-5.2",
		"input":"hello from responses",
		"reasoning":{"effort":"medium"},
		"text":{"format":{"type":"json_schema","json_schema":{"name":"resp_fmt","schema":{"type":"object"},"strict":true}}},
		"parallel_tool_calls":true,
		"service_tier":"default",
		"user":"resp-user-1",
		"store":true,
		"previous_response_id":"resp_prev_456",
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
	require.NotNil(t, svc.completeReq.ParallelToolCalls)
	assert.True(t, *svc.completeReq.ParallelToolCalls)
	require.NotNil(t, svc.completeReq.ServiceTier)
	assert.Equal(t, "default", *svc.completeReq.ServiceTier)
	assert.Equal(t, "resp-user-1", svc.completeReq.User)
	require.NotNil(t, svc.completeReq.Store)
	assert.True(t, *svc.completeReq.Store)
	assert.Equal(t, "resp_prev_456", svc.completeReq.PreviousResponseID)
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
	assert.Equal(t, "message", resp.Output[0].Type)
}

func TestChatHandler_OpenAICompatResponses_Stream(t *testing.T) {
	svc := &openAICompatServiceStub{
		streamChunks: []llmcore.UnifiedChunk{
			{
				Output: &llm.StreamChunk{
					ID:    "resp_stream_1",
					Model: "gpt-5.2",
					Delta: types.Message{
						Role:    llm.RoleAssistant,
						Content: "hello",
					},
					FinishReason: "stop",
				},
			},
		},
	}
	handler := NewChatHandlerWithService(svc, zap.NewNop())

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
			Response: &api.ChatResponse{
				ID: "resp_filters",
				Choices: []api.ChatChoice{
					{
						Index: 0,
						Message: api.Message{
							Role:    "assistant",
							Content: "ok",
						},
					},
				},
			},
		},
	}
	handler := NewChatHandlerWithService(svc, zap.NewNop())

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
