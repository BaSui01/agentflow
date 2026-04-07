package claude

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BaSui01/agentflow/types"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// --- Constructor ---

func TestNewClaudeProvider_Defaults(t *testing.T) {
	tests := []struct {
		name            string
		cfg             providers.ClaudeConfig
		expectedBaseURL string
	}{
		{
			name:            "empty config uses default BaseURL",
			cfg:             providers.ClaudeConfig{},
			expectedBaseURL: "https://api.anthropic.com",
		},
		{
			name: "custom BaseURL is preserved",
			cfg: providers.ClaudeConfig{
				BaseProviderConfig: providers.BaseProviderConfig{
					BaseURL: "https://custom.example.com",
				},
			},
			expectedBaseURL: "https://custom.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewClaudeProvider(tt.cfg, zap.NewNop())
			require.NotNil(t, p)
			assert.Equal(t, "claude", p.Name())
			assert.Equal(t, tt.expectedBaseURL, p.cfg.BaseURL)
		})
	}
}

func TestClaudeProvider_SupportsNativeFunctionCalling(t *testing.T) {
	p := NewClaudeProvider(providers.ClaudeConfig{}, zap.NewNop())
	assert.True(t, p.SupportsNativeFunctionCalling())
}

func TestClaudeProvider_Endpoints(t *testing.T) {
	p := NewClaudeProvider(providers.ClaudeConfig{
		BaseProviderConfig: providers.BaseProviderConfig{BaseURL: "https://api.anthropic.com"},
	}, zap.NewNop())
	ep := p.Endpoints()
	assert.Equal(t, "https://api.anthropic.com/v1/messages", ep.Completion)
	assert.Equal(t, "https://api.anthropic.com/v1/models", ep.Models)
}

// --- Headers ---

func TestClaudeProvider_Headers_APIKey(t *testing.T) {
	var capturedHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(claudeResponse{
			ID: "msg_1", Role: "assistant", Model: "claude-opus-4.5-20260105",
			Content:    []claudeContent{{Type: "text", Text: "ok"}},
			StopReason: "end_turn",
		})
		require.NoError(t, err)
	}))
	t.Cleanup(func() { server.Close() })

	p := NewClaudeProvider(providers.ClaudeConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "sk-test-123", BaseURL: server.URL},
	}, zap.NewNop())

	_, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.NoError(t, err)
	assert.Equal(t, "sk-test-123", capturedHeaders.Get("x-api-key"))
	assert.Equal(t, "2025-04-14", capturedHeaders.Get("anthropic-version"))
	assert.Equal(t, "application/json", capturedHeaders.Get("Content-Type"))
}

func TestClaudeProvider_Headers_CustomVersion(t *testing.T) {
	var capturedVersion string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedVersion = r.Header.Get("anthropic-version")
		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(claudeResponse{
			ID: "msg_1", Role: "assistant", Model: "claude-opus-4.5-20260105",
			Content: []claudeContent{{Type: "text", Text: "ok"}},
		})
		require.NoError(t, err)
	}))
	t.Cleanup(func() { server.Close() })

	p := NewClaudeProvider(providers.ClaudeConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "sk-test", BaseURL: server.URL},
		AnthropicVersion:   "2024-01-01",
	}, zap.NewNop())

	_, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.NoError(t, err)
	assert.Equal(t, "2024-01-01", capturedVersion)
}

// --- convertToClaudeMessages ---

func TestConvertToClaudeMessages(t *testing.T) {
	msgs := []types.Message{
		{Role: llm.RoleSystem, Content: "You are helpful"},
		{Role: llm.RoleDeveloper, Content: "Always answer in JSON"},
		{Role: llm.RoleUser, Content: "Hello"},
		{Role: llm.RoleAssistant, Content: "Hi there", ToolCalls: []types.ToolCall{
			{ID: "tc_1", Name: "search", Arguments: json.RawMessage(`{"q":"test"}`)},
		}},
		{Role: llm.RoleTool, ToolCallID: "tc_1", Content: "result data"},
	}

	system, claudeMsgs := convertToClaudeMessages(msgs)
	assert.Equal(t, "You are helpful\n\nAlways answer in JSON", system)
	require.Len(t, claudeMsgs, 3)

	// User message
	assert.Equal(t, "user", claudeMsgs[0].Role)
	require.Len(t, claudeMsgs[0].Content, 1)
	assert.Equal(t, "text", claudeMsgs[0].Content[0].Type)

	// Assistant with tool_use
	assert.Equal(t, "assistant", claudeMsgs[1].Role)
	require.Len(t, claudeMsgs[1].Content, 2)
	assert.Equal(t, "text", claudeMsgs[1].Content[0].Type)
	assert.Equal(t, "tool_use", claudeMsgs[1].Content[1].Type)
	assert.Equal(t, "search", claudeMsgs[1].Content[1].Name)

	// Tool result wrapped as user
	assert.Equal(t, "user", claudeMsgs[2].Role)
	assert.Equal(t, "tool_result", claudeMsgs[2].Content[0].Type)
	assert.Equal(t, "tc_1", claudeMsgs[2].Content[0].ToolUseID)
}

// --- convertClaudeToolChoice ---

func TestConvertClaudeToolChoice(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected *claudeToolChoice
	}{
		{"nil", nil, nil},
		{"auto string", "auto", &claudeToolChoice{Type: "auto"}},
		{"any string", "any", &claudeToolChoice{Type: "any"}},
		{"required string", "required", &claudeToolChoice{Type: "any"}},
		{"none string", "none", &claudeToolChoice{Type: "none"}},
		{"empty string", "", nil},
		{"specific tool", "my_tool", &claudeToolChoice{Type: "tool", Name: "my_tool"}},
		{"map form", map[string]any{"type": "tool", "name": "calc"}, &claudeToolChoice{Type: "tool", Name: "calc"}},
		{"unsupported type", 42, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertClaudeToolChoice(tt.input, nil, false)
			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Equal(t, tt.expected.Type, result.Type)
				assert.Equal(t, tt.expected.Name, result.Name)
			}
		})
	}
}

func TestConvertClaudeToolChoice_DisablesParallelToolUse(t *testing.T) {
	parallel := false
	result := convertClaudeToolChoice(nil, &parallel, true)
	require.NotNil(t, result)
	assert.Equal(t, "auto", result.Type)
	require.NotNil(t, result.DisableParallelToolUse)
	assert.True(t, *result.DisableParallelToolUse)
}

// --- chooseMaxTokens ---

func TestChooseMaxTokens(t *testing.T) {
	assert.Equal(t, 4096, chooseMaxTokens(nil))
	assert.Equal(t, 4096, chooseMaxTokens(&llm.ChatRequest{}))
	assert.Equal(t, 100, chooseMaxTokens(&llm.ChatRequest{MaxTokens: 100}))
}

// --- Completion via httptest ---

func TestClaudeProvider_Completion(t *testing.T) {
	var capturedRequest claudeRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/messages", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)

		err := json.NewDecoder(r.Body).Decode(&capturedRequest)
		require.NoError(t, err)

		w.Header().Set("Content-Type", "application/json")
		err = json.NewEncoder(w).Encode(claudeResponse{
			ID:    "msg_01abc",
			Role:  "assistant",
			Model: "claude-opus-4.5-20260105",
			Content: []claudeContent{
				{Type: "text", Text: "Hello from Claude"},
			},
			StopReason: "end_turn",
			Usage:      &claudeUsage{InputTokens: 10, OutputTokens: 5},
		})
		require.NoError(t, err)
	}))
	t.Cleanup(func() { server.Close() })

	p := NewClaudeProvider(providers.ClaudeConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "sk-test", BaseURL: server.URL},
	}, zap.NewNop())

	resp, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{
			{Role: llm.RoleSystem, Content: "Be helpful"},
			{Role: llm.RoleUser, Content: "Hi"},
		},
		MaxTokens: 100,
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Equal(t, "msg_01abc", resp.ID)
	assert.Equal(t, "claude", resp.Provider)
	assert.Equal(t, "claude-opus-4.5-20260105", resp.Model)
	require.Len(t, resp.Choices, 1)
	assert.Equal(t, "Hello from Claude", resp.Choices[0].Message.Content)
	assert.Equal(t, "end_turn", resp.Choices[0].FinishReason)
	assert.Equal(t, 10, resp.Usage.PromptTokens)
	assert.Equal(t, 5, resp.Usage.CompletionTokens)
	assert.Equal(t, 15, resp.Usage.TotalTokens)

	// Verify request body
	assert.Equal(t, "Be helpful", capturedRequest.System)
	assert.Equal(t, 100, capturedRequest.MaxTokens)
}

func TestClaudeProvider_Completion_WithToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(claudeResponse{
			ID: "msg_tc", Role: "assistant", Model: "claude-opus-4.5-20260105",
			Content: []claudeContent{
				{Type: "text", Text: "Let me search."},
				{Type: "tool_use", ID: "toolu_1", Name: "search", Input: json.RawMessage(`{"query":"test"}`)},
			},
			StopReason: "tool_use",
		})
	}))
	t.Cleanup(func() { server.Close() })

	p := NewClaudeProvider(providers.ClaudeConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "sk-test", BaseURL: server.URL},
	}, zap.NewNop())

	resp, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Search for test"}},
	})
	require.NoError(t, err)
	require.Len(t, resp.Choices, 1)
	assert.Equal(t, "Let me search.", resp.Choices[0].Message.Content)
	require.Len(t, resp.Choices[0].Message.ToolCalls, 1)
	assert.Equal(t, "search", resp.Choices[0].Message.ToolCalls[0].Name)
	assert.Equal(t, "toolu_1", resp.Choices[0].Message.ToolCalls[0].ID)
	assert.Equal(t, "tool_use", resp.Choices[0].FinishReason)
}

func TestClaudeProvider_Completion_TolerantStringContentAndUsageAliases(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":         "msg_compat_1",
			"type":       "message",
			"role":       "assistant",
			"model":      "glm-5",
			"content":    "compat-ok",
			"stopReason": "end_turn",
			"usage": map[string]any{
				"prompt_tokens":     11,
				"completion_tokens": 7,
				"total_tokens":      18,
			},
		})
	}))
	t.Cleanup(func() { server.Close() })

	p := NewClaudeProvider(providers.ClaudeConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "sk-test", BaseURL: server.URL},
	}, zap.NewNop())

	resp, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "compat?"}},
	})
	require.NoError(t, err)
	require.Len(t, resp.Choices, 1)
	assert.Equal(t, "compat-ok", resp.Choices[0].Message.Content)
	assert.Equal(t, "end_turn", resp.Choices[0].FinishReason)
	assert.Equal(t, 11, resp.Usage.PromptTokens)
	assert.Equal(t, 7, resp.Usage.CompletionTokens)
	assert.Equal(t, 18, resp.Usage.TotalTokens)
}

func TestClaudeProvider_Completion_ThinkingContentBlock(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(claudeResponse{
			ID: "msg_ts", Role: "assistant", Model: "claude-opus-4.5-20260105",
			Content: []claudeContent{
				{Type: "thinking", Thinking: "Let me think step by step...", Signature: "sig_abc"},
				{Type: "text", Text: "2+2=4"},
			},
			StopReason: "end_turn",
		})
	}))
	t.Cleanup(func() { server.Close() })

	p := NewClaudeProvider(providers.ClaudeConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "sk-test", BaseURL: server.URL},
	}, zap.NewNop())

	resp, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "2+2?"}},
	})
	require.NoError(t, err)
	assert.Equal(t, "2+2=4", resp.Choices[0].Message.Content)
	require.NotNil(t, resp.Choices[0].Message.ReasoningContent)
	assert.Equal(t, "Let me think step by step...", *resp.Choices[0].Message.ReasoningContent)
	require.Len(t, resp.Choices[0].Message.ThinkingBlocks, 1)
	assert.Equal(t, "sig_abc", resp.Choices[0].Message.ThinkingBlocks[0].Signature)
	assert.Equal(t, []string{"sig_abc"}, resp.ThoughtSignatures)
}

func TestClaudeProvider_Completion_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, err := w.Write([]byte(`{"error":{"message":"Invalid API key","type":"authentication_error"}}`))
		require.NoError(t, err)
	}))
	t.Cleanup(func() { server.Close() })

	p := NewClaudeProvider(providers.ClaudeConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "bad-key", BaseURL: server.URL},
	}, zap.NewNop())

	_, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.Error(t, err)
	llmErr, ok := err.(*types.Error)
	require.True(t, ok)
	assert.Equal(t, llm.ErrUnauthorized, llmErr.Code)
}

// --- Stream via httptest ---

func writeSSEEvent(w http.ResponseWriter, eventType string, data any) {
	jsonData, _ := json.Marshal(data)
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, string(jsonData))
}

func TestClaudeProvider_Stream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/messages", r.URL.Path)

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		// message_start
		writeSSEEvent(w, "message_start", claudeStreamEvent{
			Type: "message_start",
			Message: &claudeResponse{
				ID: "msg_stream", Model: "claude-opus-4.5-20260105",
			},
		})

		// content_block_delta with text
		writeSSEEvent(w, "content_block_delta", claudeStreamEvent{
			Type:  "content_block_delta",
			Index: 0,
			Delta: &claudeDelta{Type: "text_delta", Text: "Hello "},
		})

		writeSSEEvent(w, "content_block_delta", claudeStreamEvent{
			Type:  "content_block_delta",
			Index: 0,
			Delta: &claudeDelta{Type: "text_delta", Text: "world"},
		})

		// message_delta with stop reason and usage (Bug2 fix: usage is on message_delta, not message_stop)
		writeSSEEvent(w, "message_delta", claudeStreamEvent{
			Type:  "message_delta",
			Delta: &claudeDelta{StopReason: "end_turn"},
			Usage: &claudeUsage{InputTokens: 10, OutputTokens: 5},
		})

		// message_stop (no usage data per API spec)
		writeSSEEvent(w, "message_stop", claudeStreamEvent{
			Type: "message_stop",
		})
	}))
	t.Cleanup(func() { server.Close() })

	p := NewClaudeProvider(providers.ClaudeConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "sk-test", BaseURL: server.URL},
	}, zap.NewNop())

	ch, err := p.Stream(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.NoError(t, err)

	var chunks []llm.StreamChunk
	for c := range ch {
		require.Nil(t, c.Err)
		chunks = append(chunks, c)
	}

	// Should have: 2 text deltas + 1 message_delta (stop) + 1 message_stop (usage)
	require.GreaterOrEqual(t, len(chunks), 3)

	// Verify text content
	var content string
	for _, c := range chunks {
		content += c.Delta.Content
	}
	assert.Equal(t, "Hello world", content)

	// Verify provider and model
	for _, c := range chunks {
		if c.Provider != "" {
			assert.Equal(t, "claude", c.Provider)
		}
	}

	// Last chunk should have usage
	lastChunk := chunks[len(chunks)-1]
	require.NotNil(t, lastChunk.Usage)
	assert.Equal(t, 10, lastChunk.Usage.PromptTokens)
	assert.Equal(t, 5, lastChunk.Usage.CompletionTokens)
}

func TestClaudeProvider_Stream_ToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		writeSSEEvent(w, "message_start", claudeStreamEvent{
			Type:    "message_start",
			Message: &claudeResponse{ID: "msg_tc", Model: "claude-opus-4.5-20260105"},
		})

		// content_block_start for tool_use
		writeSSEEvent(w, "content_block_start", claudeStreamEvent{
			Type:  "content_block_start",
			Index: 0,
			ContentBlock: &claudeContent{
				Type: "tool_use", ID: "toolu_1", Name: "get_weather",
			},
		})

		// input_json_delta
		writeSSEEvent(w, "content_block_delta", claudeStreamEvent{
			Type:  "content_block_delta",
			Index: 0,
			Delta: &claudeDelta{Type: "input_json_delta", PartialJSON: `{"city":`},
		})
		writeSSEEvent(w, "content_block_delta", claudeStreamEvent{
			Type:  "content_block_delta",
			Index: 0,
			Delta: &claudeDelta{Type: "input_json_delta", PartialJSON: `"NYC"}`},
		})

		// content_block_stop
		writeSSEEvent(w, "content_block_stop", claudeStreamEvent{
			Type: "content_block_stop", Index: 0,
		})

		writeSSEEvent(w, "message_stop", claudeStreamEvent{Type: "message_stop"})
	}))
	t.Cleanup(func() { server.Close() })

	p := NewClaudeProvider(providers.ClaudeConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "sk-test", BaseURL: server.URL},
	}, zap.NewNop())

	ch, err := p.Stream(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Weather?"}},
	})
	require.NoError(t, err)

	var toolCallChunks []llm.StreamChunk
	for c := range ch {
		if len(c.Delta.ToolCalls) > 0 {
			toolCallChunks = append(toolCallChunks, c)
		}
	}

	// Should have at least one chunk with the completed tool call
	require.NotEmpty(t, toolCallChunks)
	tc := toolCallChunks[len(toolCallChunks)-1].Delta.ToolCalls[0]
	assert.Equal(t, "get_weather", tc.Name)
	assert.Equal(t, "toolu_1", tc.ID)

	// Bug1 fix: verify Arguments is valid JSON (not corrupted by initial "{}")
	assert.JSONEq(t, `{"city":"NYC"}`, string(tc.Arguments))
}

func TestClaudeProvider_Stream_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, err := w.Write([]byte(`{"error":{"message":"Rate limit exceeded","type":"rate_limit_error"}}`))
		require.NoError(t, err)
	}))
	t.Cleanup(func() { server.Close() })

	p := NewClaudeProvider(providers.ClaudeConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "sk-test", BaseURL: server.URL},
	}, zap.NewNop())

	_, err := p.Stream(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.Error(t, err)
	llmErr, ok := err.(*types.Error)
	require.True(t, ok)
	assert.Equal(t, llm.ErrRateLimit, llmErr.Code)
	assert.True(t, llmErr.Retryable)
}

// --- HealthCheck ---

func TestClaudeProvider_HealthCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/models", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte(`{"data":[]}`))
		require.NoError(t, err)
	}))
	t.Cleanup(func() { server.Close() })

	p := NewClaudeProvider(providers.ClaudeConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "sk-test", BaseURL: server.URL},
	}, zap.NewNop())

	status, err := p.HealthCheck(context.Background())
	require.NoError(t, err)
	assert.True(t, status.Healthy)
}

func TestClaudeProvider_HealthCheck_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":{"message":"Internal error"}}`))
	}))
	t.Cleanup(func() { server.Close() })

	p := NewClaudeProvider(providers.ClaudeConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "sk-test", BaseURL: server.URL},
	}, zap.NewNop())

	status, err := p.HealthCheck(context.Background())
	require.Error(t, err)
	assert.False(t, status.Healthy)
}

// --- ListModels ---

func TestClaudeProvider_ListModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/models", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"id":"claude-3-opus","display_name":"Claude 3 Opus","created_at":"2024-02-29T00:00:00Z","type":"model"}]}`))
	}))
	t.Cleanup(func() { server.Close() })

	p := NewClaudeProvider(providers.ClaudeConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "sk-test", BaseURL: server.URL},
	}, zap.NewNop())

	models, err := p.ListModels(context.Background())
	require.NoError(t, err)
	require.Len(t, models, 1)
	assert.Equal(t, "claude-3-opus", models[0].ID)
	assert.Equal(t, "anthropic", models[0].OwnedBy)
}

func TestClaudeProvider_ListModels_TolerantModelsEnvelope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/models", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":[{"name":"models/glm-5","owned_by":"openai-compatible"}]}`))
	}))
	t.Cleanup(func() { server.Close() })

	p := NewClaudeProvider(providers.ClaudeConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "sk-test", BaseURL: server.URL},
	}, zap.NewNop())

	models, err := p.ListModels(context.Background())
	require.NoError(t, err)
	require.Len(t, models, 1)
	assert.Equal(t, "glm-5", models[0].ID)
	assert.Equal(t, "openai-compatible", models[0].OwnedBy)
}

func TestClaudeProvider_ListModels_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error":{"message":"Forbidden"}}`))
	}))
	t.Cleanup(func() { server.Close() })

	p := NewClaudeProvider(providers.ClaudeConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "sk-test", BaseURL: server.URL},
	}, zap.NewNop())

	_, err := p.ListModels(context.Background())
	require.Error(t, err)
	llmErr, ok := err.(*types.Error)
	require.True(t, ok)
	assert.Equal(t, llm.ErrForbidden, llmErr.Code)
}

// --- Bug A: buildThinking budget_tokens constraint ---

func TestBuildThinking_BudgetConstraints(t *testing.T) {
	tests := []struct {
		name      string
		maxTokens int
		mode      string
		wantNil   bool
		wantBudge int
	}{
		{"no reasoning", 4096, "", true, 0},
		{"normal case", 4096, "extended", false, 3072},
		{"max_tokens too small", 500, "extended", true, 0},
		{"max_tokens exactly 1024", 1024, "extended", true, 0},
		{"max_tokens 1025", 1025, "extended", false, 1024},
		{"max_tokens 2000", 2000, "extended", false, 1500},
		{"budget must be < max_tokens", 1366, "extended", false, 1024},
		{"unknown mode", 4096, "fast", true, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &llm.ChatRequest{MaxTokens: tt.maxTokens, ReasoningMode: tt.mode}
			result := buildThinking(req)
			if tt.wantNil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Equal(t, "enabled", result.Type)
				assert.Equal(t, tt.wantBudge, result.BudgetTokens)
				assert.Less(t, result.BudgetTokens, tt.maxTokens, "budget_tokens must be < max_tokens")
			}
		})
	}
}

// --- Bug B: HealthCheck/ListModels use resolveAPIKey ---

func TestClaudeProvider_HealthCheck_MultiKey(t *testing.T) {
	var capturedKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedKey = r.Header.Get("x-api-key")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data":[]}`))
	}))
	t.Cleanup(func() { server.Close() })

	p := NewClaudeProvider(providers.ClaudeConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			BaseURL: server.URL,
			APIKeys: []providers.APIKeyEntry{{Key: "sk-multi-1"}},
		},
	}, zap.NewNop())

	status, err := p.HealthCheck(context.Background())
	require.NoError(t, err)
	assert.True(t, status.Healthy)
	assert.Equal(t, "sk-multi-1", capturedKey)
}

func TestClaudeProvider_ListModels_MultiKey(t *testing.T) {
	var capturedKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedKey = r.Header.Get("x-api-key")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[]}`))
	}))
	t.Cleanup(func() { server.Close() })

	p := NewClaudeProvider(providers.ClaudeConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			BaseURL: server.URL,
			APIKeys: []providers.APIKeyEntry{{Key: "sk-multi-2"}},
		},
	}, zap.NewNop())

	_, err := p.ListModels(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "sk-multi-2", capturedKey)
}

// --- Bug C: detectImageMediaType ---

func TestDetectImageMediaType(t *testing.T) {
	tests := []struct {
		name     string
		header   []byte
		expected string
	}{
		{"PNG", []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, "image/png"},
		{"JPEG", []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10}, "image/jpeg"},
		{"GIF", []byte{0x47, 0x49, 0x46, 0x38, 0x39, 0x61}, "image/gif"},
		{"WebP", []byte{0x52, 0x49, 0x46, 0x46, 0x00, 0x00, 0x00, 0x00, 0x57, 0x45, 0x42, 0x50}, "image/webp"},
		{"unknown fallback", []byte{0x00, 0x00, 0x00, 0x00}, "image/png"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b64 := base64.StdEncoding.EncodeToString(tt.header)
			assert.Equal(t, tt.expected, detectImageMediaType(b64))
		})
	}
}

// --- Bug D: input_json_delta should not emit empty chunks ---

func TestClaudeProvider_Stream_NoEmptyChunksForJsonDelta(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		writeSSEEvent(w, "message_start", claudeStreamEvent{
			Type:    "message_start",
			Message: &claudeResponse{ID: "msg_1", Model: "claude-opus-4.5-20260105"},
		})
		writeSSEEvent(w, "content_block_start", claudeStreamEvent{
			Type: "content_block_start", Index: 0,
			ContentBlock: &claudeContent{Type: "tool_use", ID: "t1", Name: "calc"},
		})
		// These should NOT produce chunks
		writeSSEEvent(w, "content_block_delta", claudeStreamEvent{
			Type: "content_block_delta", Index: 0,
			Delta: &claudeDelta{Type: "input_json_delta", PartialJSON: `{"x":1}`},
		})
		writeSSEEvent(w, "content_block_stop", claudeStreamEvent{
			Type: "content_block_stop", Index: 0,
		})
		writeSSEEvent(w, "message_stop", claudeStreamEvent{Type: "message_stop"})
	}))
	t.Cleanup(func() { server.Close() })

	p := NewClaudeProvider(providers.ClaudeConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "sk-test", BaseURL: server.URL},
	}, zap.NewNop())

	ch, err := p.Stream(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "calc"}},
	})
	require.NoError(t, err)

	var emptyChunks int
	for c := range ch {
		// A chunk with no content, no tool calls, no reasoning, no finish reason, no usage
		if c.Delta.Content == "" && len(c.Delta.ToolCalls) == 0 &&
			c.Delta.ReasoningContent == nil && c.FinishReason == "" && c.Usage == nil {
			emptyChunks++
		}
	}
	assert.Equal(t, 0, emptyChunks, "should not emit empty chunks for input_json_delta")
}

// --- Bug E: message_start input_tokens merged into message_delta ---

func TestClaudeProvider_Stream_InputTokensFromMessageStart(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		// message_start with input_tokens
		writeSSEEvent(w, "message_start", claudeStreamEvent{
			Type: "message_start",
			Message: &claudeResponse{
				ID: "msg_1", Model: "claude-opus-4.5-20260105",
				Usage: &claudeUsage{InputTokens: 25, OutputTokens: 1},
			},
		})
		writeSSEEvent(w, "content_block_delta", claudeStreamEvent{
			Type: "content_block_delta", Index: 0,
			Delta: &claudeDelta{Type: "text_delta", Text: "Hi"},
		})
		// message_delta with only output_tokens (basic case per API docs)
		writeSSEEvent(w, "message_delta", claudeStreamEvent{
			Type:  "message_delta",
			Delta: &claudeDelta{StopReason: "end_turn"},
			Usage: &claudeUsage{OutputTokens: 15},
		})
		writeSSEEvent(w, "message_stop", claudeStreamEvent{Type: "message_stop"})
	}))
	t.Cleanup(func() { server.Close() })

	p := NewClaudeProvider(providers.ClaudeConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "sk-test", BaseURL: server.URL},
	}, zap.NewNop())

	ch, err := p.Stream(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.NoError(t, err)

	var lastUsage *llm.ChatUsage
	for c := range ch {
		if c.Usage != nil {
			lastUsage = c.Usage
		}
	}
	require.NotNil(t, lastUsage, "should have usage")
	assert.Equal(t, 25, lastUsage.PromptTokens, "input_tokens from message_start should be merged")
	assert.Equal(t, 15, lastUsage.CompletionTokens)
}

// --- Bug G: multiple system messages concatenation ---

func TestConvertToClaudeMessages_MultipleSystem(t *testing.T) {
	msgs := []types.Message{
		{Role: llm.RoleSystem, Content: "You are helpful."},
		{Role: llm.RoleSystem, Content: "Be concise."},
		{Role: llm.RoleUser, Content: "Hi"},
	}
	system, claudeMsgs := convertToClaudeMessages(msgs)
	assert.Equal(t, "You are helpful.\n\nBe concise.", system)
	require.Len(t, claudeMsgs, 1)
}

// --- Bug H: multiple thinking blocks concatenation ---

func TestToClaudeChatResponse_MultipleThinkingBlocks(t *testing.T) {
	cr := claudeResponse{
		ID: "msg_1", Role: "assistant", Model: "test",
		Content: []claudeContent{
			{Type: "thinking", Thinking: "Step 1: analyze", Signature: "sig1"},
			{Type: "thinking", Thinking: "Step 2: conclude", Signature: "sig2"},
			{Type: "text", Text: "Answer"},
		},
		StopReason: "end_turn",
	}
	resp := toClaudeChatResponse(cr, "claude")
	require.NotNil(t, resp.Choices[0].Message.ReasoningContent)
	assert.Equal(t, "Step 1: analyze\n\nStep 2: conclude", *resp.Choices[0].Message.ReasoningContent)
	require.Len(t, resp.Choices[0].Message.ThinkingBlocks, 2)
	assert.Equal(t, []string{"sig1", "sig2"}, resp.ThoughtSignatures)
}

func TestToClaudeChatResponse_RedactedThinking(t *testing.T) {
	cr := claudeResponse{
		ID: "msg_redacted", Role: "assistant", Model: "test",
		Content: []claudeContent{
			{Type: "redacted_thinking", Data: "opaque_blob"},
			{Type: "text", Text: "Answer"},
		},
		StopReason: "end_turn",
	}

	resp := toClaudeChatResponse(cr, "claude")
	require.Len(t, resp.Choices, 1)
	require.Len(t, resp.Choices[0].Message.OpaqueReasoning, 1)
	assert.Equal(t, "anthropic", resp.Choices[0].Message.OpaqueReasoning[0].Provider)
	assert.Equal(t, "redacted_thinking", resp.Choices[0].Message.OpaqueReasoning[0].Kind)
	assert.Equal(t, "opaque_blob", resp.Choices[0].Message.OpaqueReasoning[0].State)
}

// =============================================================================
// Web Search Tests
// =============================================================================

func TestConvertToClaudeTools_WithWebSearch(t *testing.T) {
	// 1. wsOpts 触发注入
	tools := []types.ToolSchema{
		{Name: "get_weather", Description: "Get weather", Parameters: json.RawMessage(`{"type":"object"}`)},
	}
	wsOpts := &llm.WebSearchOptions{
		AllowedDomains: []string{"example.com"},
		BlockedDomains: []string{"spam.com"},
		MaxUses:        5,
		UserLocation: &llm.WebSearchLocation{
			Type:    "approximate",
			Country: "US",
		},
	}

	result := convertToClaudeTools(tools, wsOpts)
	require.Len(t, result, 2) // 1 function tool + 1 web_search tool

	// 验证普通工具
	var funcTool map[string]any
	require.NoError(t, json.Unmarshal(result[0], &funcTool))
	assert.Equal(t, "get_weather", funcTool["name"])

	// 验证 web_search 工具
	var wsTool map[string]any
	require.NoError(t, json.Unmarshal(result[1], &wsTool))
	assert.Equal(t, "web_search_20260209", wsTool["type"])
	assert.Equal(t, "web_search", wsTool["name"])
	assert.Equal(t, float64(5), wsTool["max_uses"])
	assert.Equal(t, []any{"example.com"}, wsTool["allowed_domains"])
	assert.Equal(t, []any{"spam.com"}, wsTool["blocked_domains"])
	loc := wsTool["user_location"].(map[string]any)
	assert.Equal(t, "US", loc["country"])
}

func TestConvertToClaudeTools_WithWebSearch_FromToolName(t *testing.T) {
	// 2. 工具列表中有 web_search 占位工具时，自动替换为 server tool
	tools := []types.ToolSchema{
		{Name: "web_search", Description: "Search the web", Parameters: json.RawMessage(`{"type":"object"}`)},
		{Name: "calculator", Description: "Calc", Parameters: json.RawMessage(`{"type":"object"}`)},
	}

	result := convertToClaudeTools(tools, nil)
	require.Len(t, result, 2) // calculator + web_search_20260209

	var names []string
	for _, raw := range result {
		var m map[string]any
		json.Unmarshal(raw, &m)
		if n, ok := m["name"].(string); ok {
			names = append(names, n)
		}
	}
	assert.Contains(t, names, "calculator")
	assert.Contains(t, names, "web_search")
}

func TestConvertToClaudeTools_NilWebSearchOpts(t *testing.T) {
	// 3. 无 web search 时不注入
	tools := []types.ToolSchema{
		{Name: "calc", Description: "Calc", Parameters: json.RawMessage(`{"type":"object"}`)},
	}
	result := convertToClaudeTools(tools, nil)
	require.Len(t, result, 1)
}

func TestConvertToClaudeTools_PreservesStrict(t *testing.T) {
	strict := true
	result := convertToClaudeTools([]types.ToolSchema{{
		Name:        "calc",
		Description: "Calc",
		Parameters:  json.RawMessage(`{"type":"object"}`),
		Strict:      &strict,
	}}, nil)
	require.Len(t, result, 1)
	var tool map[string]any
	require.NoError(t, json.Unmarshal(result[0], &tool))
	assert.Equal(t, true, tool["strict"])
}

func TestToClaudeChatResponse_WithWebSearch(t *testing.T) {
	cr := claudeResponse{
		ID: "msg_ws", Role: "assistant", Model: "claude-opus-4.5-20260105",
		Content: []claudeContent{
			{
				Type: "server_tool_use",
				ID:   "srvtoolu_1",
				Name: "web_search",
			},
			{
				Type: "web_search_tool_result",
				ID:   "srvtoolu_1",
			},
			{
				Type: "text",
				Text: "Based on my search, here's the answer.",
				Citations: []claudeCitation{
					{
						Type:       "web_search_result_location",
						URL:        "https://example.com/article",
						Title:      "Example Article",
						StartIndex: 0,
						EndIndex:   38,
					},
				},
			},
		},
		StopReason: "end_turn",
		Usage:      &claudeUsage{InputTokens: 50, OutputTokens: 30},
	}

	resp := toClaudeChatResponse(cr, "claude")
	require.Len(t, resp.Choices, 1)
	msg := resp.Choices[0].Message

	// 验证文本内容
	assert.Equal(t, "Based on my search, here's the answer.", msg.Content)

	// 验证 annotations（citations → url_citation）
	require.Len(t, msg.Annotations, 1)
	assert.Equal(t, "url_citation", msg.Annotations[0].Type)
	assert.Equal(t, "https://example.com/article", msg.Annotations[0].URL)
	assert.Equal(t, "Example Article", msg.Annotations[0].Title)
	assert.Equal(t, 0, msg.Annotations[0].StartIndex)
	assert.Equal(t, 38, msg.Annotations[0].EndIndex)

	// 验证 web search blocks 保存到 metadata 用于 round-trip
	meta, ok := msg.Metadata.(map[string]any)
	require.True(t, ok)
	blocks, ok := meta["claude_web_search_blocks"].([]json.RawMessage)
	require.True(t, ok)
	assert.Len(t, blocks, 2) // server_tool_use + web_search_tool_result
}

func TestConvertToClaudeMessages_WebSearchRoundTrip(t *testing.T) {
	// 模拟 assistant 消息带有 web search metadata
	wsBlocks := []json.RawMessage{
		json.RawMessage(`{"type":"server_tool_use","id":"srvtoolu_1","name":"web_search"}`),
		json.RawMessage(`{"type":"web_search_tool_result","id":"srvtoolu_1","encrypted_content":"enc123"}`),
	}
	msgs := []types.Message{
		{Role: llm.RoleUser, Content: "search for Go 1.22"},
		{
			Role:    llm.RoleAssistant,
			Content: "Here's what I found.",
			Metadata: map[string]any{
				"claude_web_search_blocks": wsBlocks,
			},
		},
		{Role: llm.RoleUser, Content: "Tell me more"},
	}

	_, claudeMsgs := convertToClaudeMessages(msgs)
	require.Len(t, claudeMsgs, 3)

	// assistant 消息应包含 text + 2 个 web search blocks
	assistantMsg := claudeMsgs[1]
	assert.Equal(t, "assistant", assistantMsg.Role)
	require.GreaterOrEqual(t, len(assistantMsg.Content), 3)

	// 验证 web search blocks 被正确回传
	assert.Equal(t, "text", assistantMsg.Content[0].Type)
	assert.Equal(t, "server_tool_use", assistantMsg.Content[1].Type)
	assert.Equal(t, "web_search_tool_result", assistantMsg.Content[2].Type)
}

func TestClaudeProvider_Stream_WebSearch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		writeSSEEvent(w, "message_start", claudeStreamEvent{
			Type: "message_start",
			Message: &claudeResponse{
				ID: "msg_ws", Model: "claude-opus-4.5-20260105",
				Usage: &claudeUsage{InputTokens: 20},
			},
		})

		// server_tool_use block
		writeSSEEvent(w, "content_block_start", claudeStreamEvent{
			Type:  "content_block_start",
			Index: 0,
			ContentBlock: &claudeContent{
				Type: "server_tool_use", ID: "srvtoolu_1", Name: "web_search",
			},
		})
		writeSSEEvent(w, "content_block_stop", claudeStreamEvent{
			Type: "content_block_stop", Index: 0,
		})

		// web_search_tool_result block
		writeSSEEvent(w, "content_block_start", claudeStreamEvent{
			Type:  "content_block_start",
			Index: 1,
			ContentBlock: &claudeContent{
				Type: "web_search_tool_result", ID: "srvtoolu_1",
			},
		})
		writeSSEEvent(w, "content_block_stop", claudeStreamEvent{
			Type: "content_block_stop", Index: 1,
		})

		// text block with content
		writeSSEEvent(w, "content_block_start", claudeStreamEvent{
			Type:         "content_block_start",
			Index:        2,
			ContentBlock: &claudeContent{Type: "text"},
		})
		writeSSEEvent(w, "content_block_delta", claudeStreamEvent{
			Type: "content_block_delta", Index: 2,
			Delta: &claudeDelta{Type: "text_delta", Text: "Search results say hello"},
		})
		writeSSEEvent(w, "content_block_stop", claudeStreamEvent{
			Type: "content_block_stop", Index: 2,
		})

		// message_delta with stop reason
		writeSSEEvent(w, "message_delta", claudeStreamEvent{
			Type:  "message_delta",
			Delta: &claudeDelta{StopReason: "end_turn"},
			Usage: &claudeUsage{OutputTokens: 15},
		})

		writeSSEEvent(w, "message_stop", claudeStreamEvent{Type: "message_stop"})
	}))
	t.Cleanup(func() { server.Close() })

	p := NewClaudeProvider(providers.ClaudeConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "sk-test", BaseURL: server.URL},
	}, zap.NewNop())

	ch, err := p.Stream(context.Background(), &llm.ChatRequest{
		Messages:         []types.Message{{Role: llm.RoleUser, Content: "search something"}},
		WebSearchOptions: &llm.WebSearchOptions{},
	})
	require.NoError(t, err)

	var chunks []llm.StreamChunk
	for c := range ch {
		require.Nil(t, c.Err)
		chunks = append(chunks, c)
	}

	// 应该有: text delta + message_delta (stop+usage)，server_tool_use 和 web_search_tool_result 块被静默跳过
	require.GreaterOrEqual(t, len(chunks), 2)

	// 验证文本内容
	var content string
	for _, c := range chunks {
		content += c.Delta.Content
	}
	assert.Equal(t, "Search results say hello", content)

	// 不应有空 chunk（web search 块应被静默跳过）
	for _, c := range chunks {
		hasContent := c.Delta.Content != "" || len(c.Delta.ToolCalls) > 0 ||
			c.Delta.ReasoningContent != nil || c.FinishReason != "" ||
			c.Usage != nil || len(c.Delta.Annotations) > 0
		assert.True(t, hasContent, "should not emit empty chunks for web search blocks")
	}
}

func TestClaudeProvider_Stream_ThinkingBlockPreservesSignature(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		writeSSEEvent(w, "message_start", claudeStreamEvent{
			Type: "message_start",
			Message: &claudeResponse{
				ID: "msg_thinking", Model: "claude-opus-4.5-20260105",
			},
		})
		writeSSEEvent(w, "content_block_start", claudeStreamEvent{
			Type:  "content_block_start",
			Index: 0,
			ContentBlock: &claudeContent{
				Type: "thinking",
			},
		})
		writeSSEEvent(w, "content_block_delta", claudeStreamEvent{
			Type:  "content_block_delta",
			Index: 0,
			Delta: &claudeDelta{Type: "thinking_delta", Thinking: "thought text"},
		})
		writeSSEEvent(w, "content_block_delta", claudeStreamEvent{
			Type:  "content_block_delta",
			Index: 0,
			Delta: &claudeDelta{Type: "signature_delta", Signature: "sig_stream"},
		})
		writeSSEEvent(w, "content_block_stop", claudeStreamEvent{
			Type:  "content_block_stop",
			Index: 0,
		})
		writeSSEEvent(w, "message_delta", claudeStreamEvent{
			Type:  "message_delta",
			Delta: &claudeDelta{StopReason: "end_turn"},
		})
		writeSSEEvent(w, "message_stop", claudeStreamEvent{Type: "message_stop"})
	}))
	t.Cleanup(func() { server.Close() })

	p := NewClaudeProvider(providers.ClaudeConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "sk-test", BaseURL: server.URL},
	}, zap.NewNop())

	ch, err := p.Stream(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "think"}},
	})
	require.NoError(t, err)

	var (
		reasoningDelta string
		thinkingChunk  *llm.StreamChunk
	)
	for c := range ch {
		require.Nil(t, c.Err)
		if c.Delta.ReasoningContent != nil && reasoningDelta == "" {
			reasoningDelta = *c.Delta.ReasoningContent
		}
		if len(c.Delta.ThinkingBlocks) > 0 {
			copied := c
			thinkingChunk = &copied
		}
	}

	assert.Equal(t, "thought text", reasoningDelta)
	require.NotNil(t, thinkingChunk)
	require.Len(t, thinkingChunk.Delta.ThinkingBlocks, 1)
	assert.Equal(t, "sig_stream", thinkingChunk.Delta.ThinkingBlocks[0].Signature)
}
