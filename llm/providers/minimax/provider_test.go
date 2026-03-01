package minimax

import (
	"github.com/BaSui01/agentflow/types"
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

// --- Constructor ---

func TestNewMiniMaxProvider_Defaults(t *testing.T) {
	tests := []struct {
		name            string
		cfg             providers.MiniMaxConfig
		expectedBaseURL string
	}{
		{
			name:            "empty config uses default BaseURL",
			cfg:             providers.MiniMaxConfig{},
			expectedBaseURL: "https://api.minimax.io",
		},
		{
			name: "custom BaseURL is preserved",
			cfg: providers.MiniMaxConfig{
				BaseProviderConfig: providers.BaseProviderConfig{
					BaseURL: "https://custom.example.com",
				},
			},
			expectedBaseURL: "https://custom.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewMiniMaxProvider(tt.cfg, zap.NewNop())
			require.NotNil(t, p)
			assert.Equal(t, "minimax", p.Name())
			assert.Equal(t, tt.expectedBaseURL, p.Cfg.BaseURL)
		})
	}
}

func TestMiniMaxProvider_FallbackModel(t *testing.T) {
	p := NewMiniMaxProvider(providers.MiniMaxConfig{}, zap.NewNop())
	assert.Equal(t, "MiniMax-Text-01", p.Cfg.FallbackModel)
}

func TestMiniMaxProvider_NilLogger(t *testing.T) {
	p := NewMiniMaxProvider(providers.MiniMaxConfig{}, nil)
	require.NotNil(t, p)
	assert.Equal(t, "minimax", p.Name())
}

// --- parseXMLToolCall ---

func TestParseXMLToolCall(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		wantOK   bool
		wantName string
	}{
		{
			name:     "valid XML tool call",
			content:  "<tool_calls>\n{\"name\":\"get_weather\",\"arguments\":{\"city\":\"Beijing\"}}\n</tool_calls>",
			wantOK:   true,
			wantName: "get_weather",
		},
		{
			name:   "no XML tags",
			content: "Hello, how can I help?",
			wantOK: false,
		},
		{
			name:   "empty between tags",
			content: "<tool_calls>\n\n</tool_calls>",
			wantOK: false,
		},
		{
			name:   "invalid JSON between tags",
			content: "<tool_calls>\nnot-json\n</tool_calls>",
			wantOK: false,
		},
		{
			name:   "missing close tag",
			content: "<tool_calls>{\"name\":\"test\"}",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tc, ok := parseXMLToolCall(tt.content)
			assert.Equal(t, tt.wantOK, ok)
			if tt.wantOK {
				assert.Equal(t, tt.wantName, tc.Name)
				assert.NotEmpty(t, tc.ID)
			}
		})
	}
}

// --- Completion via httptest ---

func TestMiniMaxProvider_Completion(t *testing.T) {
	var capturedRequest providers.OpenAICompatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		assert.Contains(t, r.Header.Get("Authorization"), "Bearer ")

		json.NewDecoder(r.Body).Decode(&capturedRequest)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(providers.OpenAICompatResponse{
			ID:    "resp-1",
			Model: "abab6.5s-chat",
			Choices: []providers.OpenAICompatChoice{
				{
					Index:        0,
					FinishReason: "stop",
					Message: providers.OpenAICompatMessage{
						Role:    "assistant",
						Content: "Hello from MiniMax",
					},
				},
			},
			Usage: &providers.OpenAICompatUsage{
				PromptTokens:     10,
				CompletionTokens: 5,
				TotalTokens:      15,
			},
		})
	}))
	t.Cleanup(func() { server.Close() })

	p := NewMiniMaxProvider(providers.MiniMaxConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			APIKey:  "test-key",
			BaseURL: server.URL,
		},
	}, zap.NewNop())

	resp, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Equal(t, "minimax", resp.Provider)
	assert.Equal(t, "abab6.5s-chat", resp.Model)
	require.Len(t, resp.Choices, 1)
	assert.Equal(t, "Hello from MiniMax", resp.Choices[0].Message.Content)
	assert.Equal(t, 15, resp.Usage.TotalTokens)
	assert.Equal(t, "MiniMax-Text-01", capturedRequest.Model)
}

func TestMiniMaxProvider_Completion_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"Invalid API key","type":"auth_error"}}`))
	}))
	t.Cleanup(func() { server.Close() })

	p := NewMiniMaxProvider(providers.MiniMaxConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			APIKey:  "bad-key",
			BaseURL: server.URL,
		},
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

func TestMiniMaxProvider_Stream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		chunk := providers.OpenAICompatResponse{
			ID:    "stream-1",
			Model: "abab6.5s-chat",
			Choices: []providers.OpenAICompatChoice{
				{
					Index: 0,
					Delta: &providers.OpenAICompatMessage{
						Role:    "assistant",
						Content: "Hello",
					},
				},
			},
		}
		data, _ := json.Marshal(chunk)
		w.Write([]byte("data: "))
		w.Write(data)
		w.Write([]byte("\n\ndata: [DONE]\n\n"))
	}))
	t.Cleanup(func() { server.Close() })

	p := NewMiniMaxProvider(providers.MiniMaxConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			APIKey:  "test-key",
			BaseURL: server.URL,
		},
	}, zap.NewNop())

	ch, err := p.Stream(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.NoError(t, err)

	var chunks []llm.StreamChunk
	for c := range ch {
		chunks = append(chunks, c)
	}

	require.Len(t, chunks, 1)
	assert.Equal(t, "Hello", chunks[0].Delta.Content)
	assert.Equal(t, "minimax", chunks[0].Provider)
}

func TestMiniMaxProvider_Stream_XMLToolCall(t *testing.T) {
	// MiniMax Stream overrides the parent to parse XML tool calls from content
	// This only applies to legacy abab models
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		chunk := providers.OpenAICompatResponse{
			ID:    "stream-tc",
			Model: "abab6.5s-chat",
			Choices: []providers.OpenAICompatChoice{
				{
					Index: 0,
					Delta: &providers.OpenAICompatMessage{
						Role:    "assistant",
						Content: "<tool_calls>\n{\"name\":\"get_weather\",\"arguments\":{\"city\":\"Beijing\"}}\n</tool_calls>",
					},
				},
			},
		}
		data, _ := json.Marshal(chunk)
		w.Write([]byte("data: "))
		w.Write(data)
		w.Write([]byte("\n\ndata: [DONE]\n\n"))
	}))
	t.Cleanup(func() { server.Close() })

	// Use a legacy abab model to trigger XML tool call parsing
	p := NewMiniMaxProvider(providers.MiniMaxConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			APIKey:  "test-key",
			BaseURL: server.URL,
			Model:   "abab6.5s-chat",
		},
	}, zap.NewNop())

	ch, err := p.Stream(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "What is the weather?"}},
	})
	require.NoError(t, err)

	var chunks []llm.StreamChunk
	for c := range ch {
		chunks = append(chunks, c)
	}

	require.Len(t, chunks, 1)
	// Content should be cleared, tool call should be extracted
	assert.Empty(t, chunks[0].Delta.Content)
	require.Len(t, chunks[0].Delta.ToolCalls, 1)
	assert.Equal(t, "get_weather", chunks[0].Delta.ToolCalls[0].Name)
}

func TestMiniMaxProvider_Stream_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"error":{"message":"Service unavailable"}}`))
	}))
	t.Cleanup(func() { server.Close() })

	p := NewMiniMaxProvider(providers.MiniMaxConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			APIKey:  "test-key",
			BaseURL: server.URL,
		},
	}, zap.NewNop())

	_, err := p.Stream(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.Error(t, err)
	llmErr, ok := err.(*types.Error)
	require.True(t, ok)
	assert.Equal(t, llm.ErrUpstreamError, llmErr.Code)
	assert.True(t, llmErr.Retryable)
}


