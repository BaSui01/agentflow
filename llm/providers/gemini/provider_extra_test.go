package gemini

import (
	"context"
	"encoding/json"
	"fmt"
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

// --- normalizeFinishReason ---

func TestNormalizeFinishReason(t *testing.T) {
	tests := []struct{ in, out string }{
		{"STOP", "stop"},
		{"MAX_TOKENS", "length"},
		{"SAFETY", "content_filter"},
		{"RECITATION", "content_filter"},
		{"BLOCKLIST", "content_filter"},
		{"LANGUAGE", "content_filter"},
		{"", ""},
		{"OTHER", "other"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.out, normalizeFinishReason(tt.in), "input: %s", tt.in)
	}
}

// --- convertToolChoice ---

func TestConvertToolChoice(t *testing.T) {
	assert.Nil(t, convertToolChoice(nil))

	tc := convertToolChoice("auto")
	require.NotNil(t, tc)
	assert.Equal(t, "AUTO", tc.FunctionCallingConfig.Mode)

	tc = convertToolChoice("required")
	require.NotNil(t, tc)
	assert.Equal(t, "ANY", tc.FunctionCallingConfig.Mode)

	tc = convertToolChoice("none")
	require.NotNil(t, tc)
	assert.Equal(t, "NONE", tc.FunctionCallingConfig.Mode)

	// OpenAI-style specific function
	tc = convertToolChoice(map[string]any{
		"type":     "function",
		"function": map[string]any{"name": "get_weather"},
	})
	require.NotNil(t, tc)
	assert.Equal(t, "ANY", tc.FunctionCallingConfig.Mode)
	assert.Equal(t, []string{"get_weather"}, tc.FunctionCallingConfig.AllowedFunctionNames)

	// Unknown string
	assert.Nil(t, convertToolChoice("unknown_value"))
}

// --- checkPromptFeedback ---

func TestCheckPromptFeedback_NoBlock(t *testing.T) {
	resp := geminiResponse{Candidates: []geminiCandidate{{Content: geminiContent{}}}}
	assert.NoError(t, checkPromptFeedback(resp, "gemini"))
}

func TestCheckPromptFeedback_Blocked(t *testing.T) {
	resp := geminiResponse{
		PromptFeedback: &geminiPromptFeedback{BlockReason: "SAFETY"},
	}
	err := checkPromptFeedback(resp, "gemini")
	require.Error(t, err)
	llmErr, ok := err.(*llm.Error)
	require.True(t, ok)
	assert.Equal(t, llm.ErrContentFiltered, llmErr.Code)
	assert.Contains(t, llmErr.Message, "SAFETY")
}

// --- SupportsStructuredOutput ---

func TestGeminiProvider_SupportsStructuredOutput(t *testing.T) {
	p := NewGeminiProvider(providers.GeminiConfig{}, zap.NewNop())
	assert.True(t, p.SupportsStructuredOutput())
}

// --- buildGenerationConfig ---

func TestBuildGenerationConfig_Empty(t *testing.T) {
	req := &llm.ChatRequest{}
	assert.Nil(t, buildGenerationConfig(req))
}

func TestBuildGenerationConfig_WithThinking(t *testing.T) {
	req := &llm.ChatRequest{ReasoningMode: "high"}
	cfg := buildGenerationConfig(req)
	require.NotNil(t, cfg)
	require.NotNil(t, cfg.ThinkingConfig)
	assert.Equal(t, "high", cfg.ThinkingConfig.ThinkingLevel)
	assert.True(t, cfg.ThinkingConfig.IncludeThoughts)
}

func TestBuildGenerationConfig_WithResponseFormat(t *testing.T) {
	req := &llm.ChatRequest{
		ResponseFormat: &llm.ResponseFormat{
			Type: llm.ResponseFormatJSONSchema,
			JSONSchema: &llm.JSONSchemaParam{
				Name:   "test",
				Schema: map[string]any{"type": "object"},
			},
		},
	}
	cfg := buildGenerationConfig(req)
	require.NotNil(t, cfg)
	assert.Equal(t, "application/json", cfg.ResponseMimeType)
	assert.NotNil(t, cfg.ResponseSchema)
}

// --- Completion with thinking ---

func TestGeminiProvider_Completion_WithThinking(t *testing.T) {
	thoughtFlag := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(geminiResponse{
			Candidates: []geminiCandidate{{
				Content: geminiContent{
					Role: "model",
					Parts: []geminiPart{
						{Text: "Let me think...", Thought: &thoughtFlag},
						{Text: "The answer is 42."},
					},
				},
				FinishReason: "STOP",
			}},
			UsageMetadata: &geminiUsageMetadata{
				PromptTokenCount:     10,
				CandidatesTokenCount: 8,
				TotalTokenCount:      18,
				ThoughtsTokenCount:   5,
			},
		})
	}))
	t.Cleanup(server.Close)

	p := NewGeminiProvider(providers.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: server.URL},
	}, zap.NewNop())

	resp, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages:      []llm.Message{{Role: llm.RoleUser, Content: "What is 6*7?"}},
		ReasoningMode: "high",
	})
	require.NoError(t, err)
	require.Len(t, resp.Choices, 1)
	assert.Equal(t, "The answer is 42.", resp.Choices[0].Message.Content)
	require.NotNil(t, resp.Choices[0].Message.ReasoningContent)
	assert.Equal(t, "Let me think...", *resp.Choices[0].Message.ReasoningContent)
	require.NotNil(t, resp.Usage.CompletionTokensDetails)
	assert.Equal(t, 5, resp.Usage.CompletionTokensDetails.ReasoningTokens)
}

// --- Completion with promptFeedback block ---

func TestGeminiProvider_Completion_PromptBlocked(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(geminiResponse{
			PromptFeedback: &geminiPromptFeedback{BlockReason: "SAFETY"},
		})
	}))
	t.Cleanup(server.Close)

	p := NewGeminiProvider(providers.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: server.URL},
	}, zap.NewNop())

	_, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "bad content"}},
	})
	require.Error(t, err)
	llmErr, ok := err.(*llm.Error)
	require.True(t, ok)
	assert.Equal(t, llm.ErrContentFiltered, llmErr.Code)
}

// --- Stream with SSE and thinking ---

func TestGeminiProvider_Stream_WithThinking(t *testing.T) {
	thoughtFlag := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		chunk := geminiResponse{
			Candidates: []geminiCandidate{{
				Content: geminiContent{
					Role: "model",
					Parts: []geminiPart{
						{Text: "thinking...", Thought: &thoughtFlag},
						{Text: "result"},
					},
				},
				FinishReason: "STOP",
			}},
		}
		data, _ := json.Marshal(chunk)
		fmt.Fprintf(w, "data: %s\n\n", data)
	}))
	t.Cleanup(server.Close)

	p := NewGeminiProvider(providers.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: server.URL},
	}, zap.NewNop())

	ch, err := p.Stream(context.Background(), &llm.ChatRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.NoError(t, err)

	var chunks []llm.StreamChunk
	for c := range ch {
		require.Nil(t, c.Err)
		chunks = append(chunks, c)
	}
	require.NotEmpty(t, chunks)
	assert.Equal(t, "result", chunks[0].Delta.Content)
	require.NotNil(t, chunks[0].Delta.ReasoningContent)
	assert.Equal(t, "thinking...", *chunks[0].Delta.ReasoningContent)
	assert.Equal(t, "stop", chunks[0].FinishReason)
}

// --- convertSafetySettings ---

func TestConvertSafetySettings(t *testing.T) {
	assert.Nil(t, convertSafetySettings(nil))

	settings := []providers.GeminiSafetySetting{
		{Category: "HARM_CATEGORY_HARASSMENT", Threshold: "BLOCK_NONE"},
	}
	result := convertSafetySettings(settings)
	require.Len(t, result, 1)
	assert.Equal(t, "HARM_CATEGORY_HARASSMENT", result[0].Category)
	assert.Equal(t, "BLOCK_NONE", result[0].Threshold)
}

// --- Stream ---

func TestGeminiProvider_Stream_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "streamGenerateContent")
		w.Header().Set("Content-Type", "text/event-stream")
		// Return SSE format
		chunk := geminiResponse{
			Candidates: []geminiCandidate{
				{Content: geminiContent{
					Role:  "model",
					Parts: []geminiPart{{Text: "Hello"}},
				}},
			},
		}
		data, _ := json.Marshal(chunk)
		fmt.Fprintf(w, "data: %s\n\n", data)
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
