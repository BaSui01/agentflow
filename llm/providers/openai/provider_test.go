package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	providerbase "github.com/BaSui01/agentflow/llm/providers/base"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewOpenAIProvider_Defaults(t *testing.T) {
	p := NewOpenAIProvider(providers.OpenAIConfig{}, zap.NewNop())
	require.NotNil(t, p)
	assert.Equal(t, "openai", p.Name())
	assert.Equal(t, "https://api.openai.com", p.Cfg.BaseURL)
	assert.Equal(t, "gpt-5.2", p.Cfg.FallbackModel)
}

func TestWithPreviousResponseID(t *testing.T) {
	ctx := WithPreviousResponseID(context.Background(), "resp_abc")
	id, ok := PreviousResponseIDFromContext(ctx)
	require.True(t, ok)
	assert.Equal(t, "resp_abc", id)
}

func TestOpenAIProvider_Endpoints(t *testing.T) {
	p := NewOpenAIProvider(providers.OpenAIConfig{UseResponsesAPI: true}, zap.NewNop())
	ep := p.Endpoints()
	assert.Contains(t, ep.Completion, "/v1/responses")
}

func TestOpenAIProvider_HealthCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/models", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"gpt-5.2","object":"model","created":1700000000,"owned_by":"openai"}]}`))
	}))
	defer server.Close()

	p := NewOpenAIProvider(providers.OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: server.URL},
	}, zap.NewNop())

	status, err := p.HealthCheck(context.Background())
	require.NoError(t, err)
	assert.True(t, status.Healthy)
}

func TestOpenAIProvider_ListModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/models", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"gpt-5.2","object":"model","created":1700000000,"owned_by":"openai"}]}`))
	}))
	defer server.Close()

	p := NewOpenAIProvider(providers.OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: server.URL},
	}, zap.NewNop())

	models, err := p.ListModels(context.Background())
	require.NoError(t, err)
	require.Len(t, models, 1)
	assert.Equal(t, "gpt-5.2", models[0].ID)
	assert.Equal(t, "model", models[0].Object)
	assert.Equal(t, int64(1700000000), models[0].Created)
	assert.Equal(t, "openai", models[0].OwnedBy)
}

func TestOpenAIProvider_ListModels_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"message":"Forbidden"}}`))
	}))
	defer server.Close()

	p := NewOpenAIProvider(providers.OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: server.URL},
	}, zap.NewNop())

	_, err := p.ListModels(context.Background())
	require.Error(t, err)
	llmErr, ok := err.(*types.Error)
	require.True(t, ok)
	assert.Equal(t, llm.ErrForbidden, llmErr.Code)
}

func TestOpenAIProvider_Completion_Standard(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		require.NoError(t, json.NewEncoder(w).Encode(providerbase.OpenAICompatResponse{
			ID:    "chatcmpl-1",
			Model: "gpt-5.2",
			Choices: []providerbase.OpenAICompatChoice{{
				Index: 0, FinishReason: "stop",
				Message: providerbase.OpenAICompatMessage{Role: "assistant", Content: "Hello"},
			}},
			Usage: &providerbase.OpenAICompatUsage{PromptTokens: 5, CompletionTokens: 3, TotalTokens: 8},
		}))
	}))
	defer server.Close()

	p := NewOpenAIProvider(providers.OpenAIConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: server.URL}}, zap.NewNop())
	resp, err := p.Completion(context.Background(), &llm.ChatRequest{Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}}})
	require.NoError(t, err)
	require.Len(t, resp.Choices, 1)
	assert.Equal(t, "Hello", resp.Choices[0].Message.Content)
	assert.Equal(t, 8, resp.Usage.TotalTokens)
}

func TestOpenAIProvider_Completion_ResponsesAPI(t *testing.T) {
	var reqBody openAIResponsesRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/responses", r.URL.Path)
		require.NoError(t, json.NewDecoder(r.Body).Decode(&reqBody))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
          "id":"resp_abc",
          "object":"response",
          "created_at":1700000000,
          "status":"completed",
          "model":"gpt-5.2",
          "output":[{"type":"message","id":"msg_1","status":"completed","role":"assistant","content":[{"type":"output_text","text":"Hello from Responses API"}]}],
          "usage":{"input_tokens":8,"output_tokens":6,"total_tokens":14}
        }`))
	}))
	defer server.Close()

	storeTrue := true
	p := NewOpenAIProvider(providers.OpenAIConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: server.URL}, UseResponsesAPI: true}, zap.NewNop())
	resp, err := p.Completion(context.Background(), &llm.ChatRequest{Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}}, Store: &storeTrue})
	require.NoError(t, err)
	require.Len(t, resp.Choices, 1)
	assert.True(t, *reqBody.Store)
	assert.Equal(t, "Hello from Responses API", resp.Choices[0].Message.Content)
	assert.Equal(t, 14, resp.Usage.TotalTokens)
}

func TestOpenAIProvider_Completion_ResponsesAPI_MapsReasoningAndTools(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
          "id":"resp_reasoning",
          "object":"response",
          "created_at":1700000000,
          "status":"completed",
          "model":"gpt-5.2",
          "output":[
            {"type":"reasoning","id":"rs_1","status":"completed","summary":[{"type":"summary_text","text":"Investigating request"}],"encrypted_content":"enc_123"},
            {"type":"message","id":"msg_1","status":"completed","role":"assistant","content":[{"type":"output_text","text":"done"}]},
            {"type":"function_call","id":"fc_1","call_id":"call_1","name":"get_weather","arguments":"{\"city\":\"NYC\"}"},
            {"type":"custom_tool_call","id":"ct_1","call_id":"call_custom_1","name":"code_exec","input":"print('hi')"}
          ]
        }`))
	}))
	defer server.Close()

	p := NewOpenAIProvider(providers.OpenAIConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: server.URL}, UseResponsesAPI: true}, zap.NewNop())
	resp, err := p.Completion(context.Background(), &llm.ChatRequest{Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}}, ReasoningEffort: "medium"})
	require.NoError(t, err)
	require.Len(t, resp.Choices, 1)
	msg := resp.Choices[0].Message
	require.NotNil(t, msg.ReasoningContent)
	assert.Equal(t, "Investigating request", *msg.ReasoningContent)
	require.Len(t, msg.ReasoningSummaries, 1)
	require.Len(t, msg.OpaqueReasoning, 1)
	require.Len(t, msg.ToolCalls, 2)
}

func TestOpenAIProvider_Completion_ResponsesAPI_MapsWebSearchAndToolChoice(t *testing.T) {
	var raw map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&raw))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_ws","object":"response","created_at":1700000000,"status":"completed","model":"gpt-5.2","output":[{"type":"message","id":"msg_1","role":"assistant","content":[{"type":"output_text","text":"ok"}]}]}`))
	}))
	defer server.Close()

	parallel := true
	p := NewOpenAIProvider(providers.OpenAIConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: server.URL}, UseResponsesAPI: true}, zap.NewNop())
	_, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages:          []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
		ToolChoice:        &types.ToolChoice{Mode: types.ToolChoiceModeRequired},
		ParallelToolCalls: &parallel,
		Tools:             []types.ToolSchema{{Name: "web_search"}, {Name: "get_weather", Parameters: json.RawMessage(`{"type":"object"}`)}},
		WebSearchOptions:  &llm.WebSearchOptions{SearchContextSize: "high"},
	})
	require.NoError(t, err)
	toolChoice, ok := raw["tool_choice"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "allowed_tools", toolChoice["type"])
	assert.Equal(t, "required", toolChoice["mode"])
	assert.Equal(t, true, raw["parallel_tool_calls"])
	tools, ok := raw["tools"].([]any)
	require.True(t, ok)
	require.Len(t, tools, 2)
}

func TestOpenAIProvider_Stream_ResponsesAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprintf(w, "event: response.created\n")
		_, _ = fmt.Fprintf(w, "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_1\",\"model\":\"gpt-5.2\"}}\n\n")
		_, _ = fmt.Fprintf(w, "event: response.output_text.delta\n")
		_, _ = fmt.Fprintf(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"Hello \"}\n\n")
		_, _ = fmt.Fprintf(w, "event: response.output_item.added\n")
		_, _ = fmt.Fprintf(w, "data: {\"type\":\"response.output_item.added\",\"item\":{\"type\":\"function_call\",\"id\":\"fc_1\",\"call_id\":\"call_1\",\"name\":\"get_weather\"}}\n\n")
		_, _ = fmt.Fprintf(w, "event: response.function_call_arguments.delta\n")
		_, _ = fmt.Fprintf(w, "data: {\"type\":\"response.function_call_arguments.delta\",\"item_id\":\"fc_1\",\"delta\":\"{\\\"city\\\":\"}\n\n")
		_, _ = fmt.Fprintf(w, "event: response.function_call_arguments.delta\n")
		_, _ = fmt.Fprintf(w, "data: {\"type\":\"response.function_call_arguments.delta\",\"item_id\":\"fc_1\",\"delta\":\"\\\"NYC\\\"}\"}\n\n")
		_, _ = fmt.Fprintf(w, "event: response.function_call_arguments.done\n")
		_, _ = fmt.Fprintf(w, "data: {\"type\":\"response.function_call_arguments.done\",\"item_id\":\"fc_1\"}\n\n")
		_, _ = fmt.Fprintf(w, "event: response.completed\n")
		_, _ = fmt.Fprintf(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"model\":\"gpt-5.2\",\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":5,\"output_tokens\":3,\"total_tokens\":8}}}\n\n")
	}))
	defer server.Close()

	p := NewOpenAIProvider(providers.OpenAIConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: server.URL}, UseResponsesAPI: true}, zap.NewNop())
	ch, err := p.Stream(context.Background(), &llm.ChatRequest{Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}}})
	require.NoError(t, err)
	var sawText, sawTool, sawUsage bool
	for c := range ch {
		if c.Delta.Content != "" {
			sawText = true
		}
		if len(c.Delta.ToolCalls) > 0 {
			sawTool = true
		}
		if c.Usage != nil {
			sawUsage = true
		}
	}
	assert.True(t, sawText)
	assert.True(t, sawTool)
	assert.True(t, sawUsage)
}
