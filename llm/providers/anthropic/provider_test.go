package claude

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
		json.NewEncoder(w).Encode(claudeResponse{
			ID: "msg_1", Role: "assistant", Model: "claude-opus-4.5-20260105",
			Content: []claudeContent{{Type: "text", Text: "ok"}},
			StopReason: "end_turn",
		})
	}))
	t.Cleanup(func() { server.Close() })

	p := NewClaudeProvider(providers.ClaudeConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "sk-test-123", BaseURL: server.URL},
	}, zap.NewNop())

	_, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.NoError(t, err)
	assert.Equal(t, "sk-test-123", capturedHeaders.Get("x-api-key"))
	assert.Equal(t, "2023-06-01", capturedHeaders.Get("anthropic-version"))
	assert.Equal(t, "application/json", capturedHeaders.Get("Content-Type"))
}

func TestClaudeProvider_Headers_Bearer(t *testing.T) {
	var capturedHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(claudeResponse{
			ID: "msg_1", Role: "assistant", Model: "claude-opus-4.5-20260105",
			Content: []claudeContent{{Type: "text", Text: "ok"}},
		})
	}))
	t.Cleanup(func() { server.Close() })

	p := NewClaudeProvider(providers.ClaudeConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "sk-test", BaseURL: server.URL},
		AuthType:           "bearer",
	}, zap.NewNop())

	_, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.NoError(t, err)
	assert.Equal(t, "Bearer sk-test", capturedHeaders.Get("Authorization"))
	assert.Empty(t, capturedHeaders.Get("x-api-key"))
}

func TestClaudeProvider_Headers_CustomVersion(t *testing.T) {
	var capturedVersion string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedVersion = r.Header.Get("anthropic-version")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(claudeResponse{
			ID: "msg_1", Role: "assistant", Model: "claude-opus-4.5-20260105",
			Content: []claudeContent{{Type: "text", Text: "ok"}},
		})
	}))
	t.Cleanup(func() { server.Close() })

	p := NewClaudeProvider(providers.ClaudeConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "sk-test", BaseURL: server.URL},
		AnthropicVersion:   "2024-01-01",
	}, zap.NewNop())

	_, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.NoError(t, err)
	assert.Equal(t, "2024-01-01", capturedVersion)
}

// --- convertToClaudeMessages ---

func TestConvertToClaudeMessages(t *testing.T) {
	msgs := []llm.Message{
		{Role: llm.RoleSystem, Content: "You are helpful"},
		{Role: llm.RoleUser, Content: "Hello"},
		{Role: llm.RoleAssistant, Content: "Hi there", ToolCalls: []llm.ToolCall{
			{ID: "tc_1", Name: "search", Arguments: json.RawMessage(`{"q":"test"}`)},
		}},
		{Role: llm.RoleTool, ToolCallID: "tc_1", Content: "result data"},
	}

	system, claudeMsgs := convertToClaudeMessages(msgs)
	assert.Equal(t, "You are helpful", system)
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
		{"none string", "none", nil},
		{"empty string", "", nil},
		{"specific tool", "my_tool", &claudeToolChoice{Type: "tool", Name: "my_tool"}},
		{"map form", map[string]any{"type": "tool", "name": "calc"}, &claudeToolChoice{Type: "tool", Name: "calc"}},
		{"unsupported type", 42, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertClaudeToolChoice(tt.input)
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

		json.NewDecoder(r.Body).Decode(&capturedRequest)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(claudeResponse{
			ID:    "msg_01abc",
			Role:  "assistant",
			Model: "claude-opus-4.5-20260105",
			Content: []claudeContent{
				{Type: "text", Text: "Hello from Claude"},
			},
			StopReason: "end_turn",
			Usage:      &claudeUsage{InputTokens: 10, OutputTokens: 5},
		})
	}))
	t.Cleanup(func() { server.Close() })

	p := NewClaudeProvider(providers.ClaudeConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "sk-test", BaseURL: server.URL},
	}, zap.NewNop())

	resp, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []llm.Message{
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
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Search for test"}},
	})
	require.NoError(t, err)
	require.Len(t, resp.Choices, 1)
	assert.Equal(t, "Let me search.", resp.Choices[0].Message.Content)
	require.Len(t, resp.Choices[0].Message.ToolCalls, 1)
	assert.Equal(t, "search", resp.Choices[0].Message.ToolCalls[0].Name)
	assert.Equal(t, "toolu_1", resp.Choices[0].Message.ToolCalls[0].ID)
	assert.Equal(t, "tool_use", resp.Choices[0].FinishReason)
}

func TestClaudeProvider_Completion_ThoughtSignatures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(claudeResponse{
			ID: "msg_ts", Role: "assistant", Model: "claude-opus-4.5-20260105",
			Content:           []claudeContent{{Type: "text", Text: "2+2=4"}},
			StopReason:        "end_turn",
			ThoughtSignatures: []string{"sig_abc"},
		})
	}))
	t.Cleanup(func() { server.Close() })

	p := NewClaudeProvider(providers.ClaudeConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "sk-test", BaseURL: server.URL},
	}, zap.NewNop())

	resp, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages:          []llm.Message{{Role: llm.RoleUser, Content: "2+2?"}},
		ThoughtSignatures: []string{"sig_abc"},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"sig_abc"}, resp.ThoughtSignatures)
}

func TestClaudeProvider_Completion_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"Invalid API key","type":"authentication_error"}}`))
	}))
	t.Cleanup(func() { server.Close() })

	p := NewClaudeProvider(providers.ClaudeConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "bad-key", BaseURL: server.URL},
	}, zap.NewNop())

	_, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.Error(t, err)
	llmErr, ok := err.(*llm.Error)
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

		// message_delta with stop reason
		writeSSEEvent(w, "message_delta", claudeStreamEvent{
			Type:  "message_delta",
			Delta: &claudeDelta{StopReason: "end_turn"},
		})

		// message_stop with usage
		writeSSEEvent(w, "message_stop", claudeStreamEvent{
			Type:  "message_stop",
			Usage: &claudeUsage{InputTokens: 10, OutputTokens: 5},
		})
	}))
	t.Cleanup(func() { server.Close() })

	p := NewClaudeProvider(providers.ClaudeConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "sk-test", BaseURL: server.URL},
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
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Weather?"}},
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
}

func TestClaudeProvider_Stream_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"message":"Rate limit exceeded","type":"rate_limit_error"}}`))
	}))
	t.Cleanup(func() { server.Close() })

	p := NewClaudeProvider(providers.ClaudeConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "sk-test", BaseURL: server.URL},
	}, zap.NewNop())

	_, err := p.Stream(context.Background(), &llm.ChatRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.Error(t, err)
	llmErr, ok := err.(*llm.Error)
	require.True(t, ok)
	assert.Equal(t, llm.ErrRateLimited, llmErr.Code)
	assert.True(t, llmErr.Retryable)
}

// --- HealthCheck ---

func TestClaudeProvider_HealthCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/models", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data":[]}`))
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
	llmErr, ok := err.(*llm.Error)
	require.True(t, ok)
	assert.Equal(t, llm.ErrForbidden, llmErr.Code)
}
