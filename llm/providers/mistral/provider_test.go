package mistral

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

// --- Constructor ---

func TestNewMistralProvider_Defaults(t *testing.T) {
	tests := []struct {
		name            string
		cfg             providers.MistralConfig
		expectedBaseURL string
	}{
		{
			name:            "empty config uses default BaseURL",
			cfg:             providers.MistralConfig{},
			expectedBaseURL: "https://api.mistral.ai",
		},
		{
			name: "custom BaseURL is preserved",
			cfg: providers.MistralConfig{
				BaseProviderConfig: providers.BaseProviderConfig{
					BaseURL: "https://custom.example.com",
				},
			},
			expectedBaseURL: "https://custom.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewMistralProvider(tt.cfg, zap.NewNop())
			require.NotNil(t, p)
			assert.Equal(t, "mistral", p.Name())
			assert.Equal(t, tt.expectedBaseURL, p.Cfg.BaseURL)
		})
	}
}

func TestMistralProvider_FallbackModel(t *testing.T) {
	p := NewMistralProvider(providers.MistralConfig{}, zap.NewNop())
	assert.Equal(t, "mistral-large-latest", p.Cfg.FallbackModel)
}

func TestMistralProvider_NilLogger(t *testing.T) {
	p := NewMistralProvider(providers.MistralConfig{}, nil)
	require.NotNil(t, p)
	assert.Equal(t, "mistral", p.Name())
}

// --- Completion via httptest ---

func TestMistralProvider_Completion(t *testing.T) {
	var capturedRequest providers.OpenAICompatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		assert.Contains(t, r.Header.Get("Authorization"), "Bearer ")

		json.NewDecoder(r.Body).Decode(&capturedRequest)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(providers.OpenAICompatResponse{
			ID:    "resp-1",
			Model: "mistral-large-latest",
			Choices: []providers.OpenAICompatChoice{
				{
					Index:        0,
					FinishReason: "stop",
					Message: providers.OpenAICompatMessage{
						Role:    "assistant",
						Content: "Hello from Mistral",
					},
				},
			},
			Usage: &providers.OpenAICompatUsage{
				PromptTokens:     8,
				CompletionTokens: 4,
				TotalTokens:      12,
			},
		})
	}))
	t.Cleanup(func() { server.Close() })

	cfg := providers.MistralConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			APIKey:  "test-key",
			BaseURL: server.URL,
		},
	}
	p := NewMistralProvider(cfg, zap.NewNop())

	resp, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "Hi"},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Equal(t, "mistral", resp.Provider)
	assert.Equal(t, "mistral-large-latest", resp.Model)
	require.Len(t, resp.Choices, 1)
	assert.Equal(t, "Hello from Mistral", resp.Choices[0].Message.Content)
	assert.Equal(t, 12, resp.Usage.TotalTokens)
	assert.Equal(t, "mistral-large-latest", capturedRequest.Model)
}

func TestMistralProvider_Completion_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"message": "Invalid API key",
				"type":    "authentication_error",
			},
		})
	}))
	t.Cleanup(func() { server.Close() })

	p := NewMistralProvider(providers.MistralConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			APIKey:  "bad-key",
			BaseURL: server.URL,
		},
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

func TestMistralProvider_Stream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		chunk := providers.OpenAICompatResponse{
			ID:    "stream-1",
			Model: "mistral-large-latest",
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

	p := NewMistralProvider(providers.MistralConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			APIKey:  "test-key",
			BaseURL: server.URL,
		},
	}, zap.NewNop())

	ch, err := p.Stream(context.Background(), &llm.ChatRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.NoError(t, err)

	var chunks []llm.StreamChunk
	for c := range ch {
		chunks = append(chunks, c)
	}

	require.Len(t, chunks, 1)
	assert.Equal(t, "Hello", chunks[0].Delta.Content)
	assert.Equal(t, "mistral", chunks[0].Provider)
}

func TestMistralProvider_Stream_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"message":"Rate limit exceeded"}}`))
	}))
	t.Cleanup(func() { server.Close() })

	p := NewMistralProvider(providers.MistralConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			APIKey:  "test-key",
			BaseURL: server.URL,
		},
	}, zap.NewNop())

	_, err := p.Stream(context.Background(), &llm.ChatRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.Error(t, err)
	llmErr, ok := err.(*llm.Error)
	require.True(t, ok)
	assert.Equal(t, llm.ErrRateLimited, llmErr.Code)
}
