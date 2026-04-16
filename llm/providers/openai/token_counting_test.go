package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestOpenAIProvider_CountTokens(t *testing.T) {
	var capturedPath string
	var capturedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		require.NoError(t, json.NewDecoder(r.Body).Decode(&capturedBody))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"input_tokens": 42})
	}))
	t.Cleanup(server.Close)

	p := NewOpenAIProvider(providers.OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
		UseResponsesAPI:    true,
	}, zap.NewNop())

	resp, err := p.CountTokens(context.Background(), &llm.ChatRequest{
		Model: "gpt-5.2",
		Messages: []types.Message{
			{Role: llm.RoleSystem, Content: "You are helpful."},
			{Role: llm.RoleUser, Content: "hello"},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "/v1/responses/input_tokens", capturedPath)
	assert.Equal(t, float64(42), float64(resp.InputTokens))
	assert.Equal(t, "You are helpful.", capturedBody["instructions"])
}

func TestOpenAIProvider_CountTokens_MapsToolChoiceParallelAndWebSearch(t *testing.T) {
	var capturedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&capturedBody))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"input_tokens": 21})
	}))
	t.Cleanup(server.Close)

	parallel := true
	p := NewOpenAIProvider(providers.OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
		UseResponsesAPI:    true,
	}, zap.NewNop())

	resp, err := p.CountTokens(context.Background(), &llm.ChatRequest{
		Model:             "gpt-5.2",
		ToolChoice:        "required",
		ParallelToolCalls: &parallel,
		Messages: []types.Message{
			{Role: llm.RoleUser, Content: "find docs then search web"},
		},
		Tools: []types.ToolSchema{
			{Name: "web_search_preview"},
			{Name: "lookup_docs", Parameters: json.RawMessage(`{"type":"object"}`)},
		},
		WebSearchOptions: &llm.WebSearchOptions{
			SearchContextSize: "high",
			AllowedDomains:    []string{"openai.com"},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 21, resp.InputTokens)
	assert.Equal(t, "required", capturedBody["tool_choice"])
	assert.Equal(t, true, capturedBody["parallel_tool_calls"])

	toolsAny, ok := capturedBody["tools"].([]any)
	require.True(t, ok)
	require.Len(t, toolsAny, 2)
}

func TestOpenAIProvider_CountTokens_SpecificMapToolChoicePreservesName(t *testing.T) {
	var capturedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&capturedBody))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"input_tokens": 9})
	}))
	t.Cleanup(server.Close)

	p := NewOpenAIProvider(providers.OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
		UseResponsesAPI:    true,
	}, zap.NewNop())

	_, err := p.CountTokens(context.Background(), &llm.ChatRequest{
		Model: "gpt-5.2",
		Messages: []types.Message{
			{Role: llm.RoleUser, Content: "weather"},
		},
		ToolChoice: map[string]any{
			"type": "function",
			"function": map[string]any{
				"name": "get_weather",
			},
		},
		Tools: []types.ToolSchema{
			{Name: "get_weather", Parameters: json.RawMessage(`{"type":"object"}`)},
			{Name: "web_search"},
		},
	})
	require.NoError(t, err)

	toolChoice, ok := capturedBody["tool_choice"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "function", toolChoice["type"])
	assert.Equal(t, "get_weather", toolChoice["name"])
}
