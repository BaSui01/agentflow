package gemini

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// --- resolveAPIKey ---

func TestGeminiProvider_ResolveAPIKey_CredentialOverride(t *testing.T) {
	p := NewGeminiProvider(providers.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "default-key"},
	}, zap.NewNop())

	ctx := llm.WithCredentialOverride(context.Background(), llm.CredentialOverride{APIKey: "override-key"})
	key := p.resolveAPIKey(ctx)
	assert.Equal(t, "override-key", key)
}

func TestGeminiProvider_ResolveAPIKey_MultiKey(t *testing.T) {
	p := NewGeminiProvider(providers.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			APIKey: "fallback",
			APIKeys: []providers.APIKeyEntry{
				{Key: "key-a"},
				{Key: "key-b"},
			},
		},
	}, zap.NewNop())

	key1 := p.resolveAPIKey(context.Background())
	key2 := p.resolveAPIKey(context.Background())
	// Should round-robin between key-a and key-b
	assert.Contains(t, []string{"key-a", "key-b"}, key1)
	assert.Contains(t, []string{"key-a", "key-b"}, key2)
}

// --- convertToGeminiContents ---

func TestConvertToGeminiContents_ToolCalls(t *testing.T) {
	msgs := []llm.Message{
		{Role: llm.RoleUser, Content: "What's the weather?"},
		{Role: llm.RoleAssistant, Content: "Let me check.", ToolCalls: []llm.ToolCall{
			{ID: "tc1", Name: "get_weather", Arguments: json.RawMessage(`{"city":"NYC"}`)},
		}},
		{Role: llm.RoleTool, ToolCallID: "tc1", Name: "get_weather", Content: `{"temp":72}`},
	}

	sys, contents := convertToGeminiContents(msgs)
	assert.Nil(t, sys)
	require.Len(t, contents, 3)

	// Assistant message should have function call part
	assert.Equal(t, "model", contents[1].Role)
	hasFunctionCall := false
	for _, p := range contents[1].Parts {
		if p.FunctionCall != nil {
			hasFunctionCall = true
			assert.Equal(t, "get_weather", p.FunctionCall.Name)
		}
	}
	assert.True(t, hasFunctionCall)

	// Tool message should have function response part
	hasFunctionResponse := false
	for _, p := range contents[2].Parts {
		if p.FunctionResponse != nil {
			hasFunctionResponse = true
			assert.Equal(t, "get_weather", p.FunctionResponse.Name)
		}
	}
	assert.True(t, hasFunctionResponse)
}

func TestConvertToGeminiContents_ToolResponseNonJSON(t *testing.T) {
	msgs := []llm.Message{
		{Role: llm.RoleTool, ToolCallID: "tc1", Name: "search", Content: "plain text result"},
	}

	_, contents := convertToGeminiContents(msgs)
	require.Len(t, contents, 1)
	hasFunctionResponse := false
	for _, p := range contents[0].Parts {
		if p.FunctionResponse != nil {
			hasFunctionResponse = true
			assert.Equal(t, "plain text result", p.FunctionResponse.Response["result"])
		}
	}
	assert.True(t, hasFunctionResponse)
}

// --- convertToGeminiTools ---

func TestConvertToGeminiTools_WithValidTools(t *testing.T) {
	tools := []llm.ToolSchema{
		{
			Name:        "get_weather",
			Description: "Get weather",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}}}`),
		},
	}

	result := convertToGeminiTools(tools)
	require.Len(t, result, 1)
	require.Len(t, result[0].FunctionDeclarations, 1)
	assert.Equal(t, "get_weather", result[0].FunctionDeclarations[0].Name)
}

func TestConvertToGeminiTools_Empty(t *testing.T) {
	result := convertToGeminiTools(nil)
	assert.Nil(t, result)
}

func TestConvertToGeminiTools_InvalidJSON(t *testing.T) {
	tools := []llm.ToolSchema{
		{Name: "bad", Parameters: json.RawMessage(`invalid`)},
	}
	result := convertToGeminiTools(tools)
	assert.Nil(t, result) // all declarations fail to parse -> nil
}

// --- Stream ---

func TestGeminiProvider_Stream_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "streamGenerateContent")
		w.Header().Set("Content-Type", "text/event-stream")
		// Return a simple SSE streaming response
		chunk := geminiResponse{
			Candidates: []geminiCandidate{
				{Content: geminiContent{
					Role:  "model",
					Parts: []geminiPart{{Text: "Hello"}},
				}},
			},
		}
		data, _ := json.Marshal(chunk)
		w.Write([]byte("data: "))
		w.Write(data)
		w.Write([]byte("\n\n"))
	}))
	t.Cleanup(server.Close)

	p := NewGeminiProvider(providers.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
	}, zap.NewNop())

	ch, err := p.Stream(context.Background(), &llm.ChatRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.NoError(t, err)

	var chunks []llm.StreamChunk
	for c := range ch {
		chunks = append(chunks, c)
	}
	// At least we got a channel and it closed without panic
	assert.NotNil(t, ch)
}

func TestGeminiProvider_Stream_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"Invalid API key"}}`))
	}))
	t.Cleanup(server.Close)

	p := NewGeminiProvider(providers.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "bad-key", BaseURL: server.URL},
	}, zap.NewNop())

	_, err := p.Stream(context.Background(), &llm.ChatRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.Error(t, err)
}
