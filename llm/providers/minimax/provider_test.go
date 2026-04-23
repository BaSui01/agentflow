package minimax

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	providerbase "github.com/BaSui01/agentflow/llm/providers/base"

	"github.com/BaSui01/agentflow/types"

	llm "github.com/BaSui01/agentflow/llm/core"
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
			p := newMiniMaxProvider(tt.cfg, zap.NewNop())
			require.NotNil(t, p)
			assert.Equal(t, "minimax", p.Name())
			assert.Equal(t, tt.expectedBaseURL, p.Cfg.BaseURL)
		})
	}
}

func TestMiniMaxProvider_FallbackModel(t *testing.T) {
	p := newMiniMaxProvider(providers.MiniMaxConfig{}, zap.NewNop())
	assert.Equal(t, "MiniMax-M2.7", p.Cfg.FallbackModel)
}

func TestMiniMaxProvider_NilLogger(t *testing.T) {
	p := newMiniMaxProvider(providers.MiniMaxConfig{}, nil)
	require.NotNil(t, p)
	assert.Equal(t, "minimax", p.Name())
}

// --- SupportsNativeFunctionCalling ---

func TestMiniMaxProvider_LegacyModel_NoNativeFunctionCalling(t *testing.T) {
	p := newMiniMaxProvider(providers.MiniMaxConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			Model: "abab6.5s-chat",
		},
	}, zap.NewNop())

	assert.False(t, p.SupportsNativeFunctionCalling(),
		"旧模型 abab 系列应不支持原生函数调用")
}

func TestMiniMaxProvider_NewModel_SupportsNativeFunctionCalling(t *testing.T) {
	p := newMiniMaxProvider(providers.MiniMaxConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			Model: "MiniMax-M2.7",
		},
	}, zap.NewNop())

	assert.True(t, p.SupportsNativeFunctionCalling(),
		"新模型应支持原生函数调用")
}

func TestMiniMaxProvider_EmptyModel_SupportsNativeFunctionCalling(t *testing.T) {
	p := newMiniMaxProvider(providers.MiniMaxConfig{}, zap.NewNop())

	assert.True(t, p.SupportsNativeFunctionCalling(),
		"未指定模型时应默认支持原生函数调用")
}

// --- Completion via httptest ---

func TestMiniMaxProvider_Completion(t *testing.T) {
	var capturedRequest providerbase.OpenAICompatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		assert.Contains(t, r.Header.Get("Authorization"), "Bearer ")

		json.NewDecoder(r.Body).Decode(&capturedRequest)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(providerbase.OpenAICompatResponse{
			ID:    "resp-1",
			Model: "abab6.5s-chat",
			Choices: []providerbase.OpenAICompatChoice{
				{
					Index:        0,
					FinishReason: "stop",
					Message: providerbase.OpenAICompatMessage{
						Role:    "assistant",
						Content: "Hello from MiniMax",
					},
				},
			},
			Usage: &providerbase.OpenAICompatUsage{
				PromptTokens:     10,
				CompletionTokens: 5,
				TotalTokens:      15,
			},
		})
	}))
	t.Cleanup(func() { server.Close() })

	p := newMiniMaxProvider(providers.MiniMaxConfig{
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
	assert.Equal(t, "MiniMax-M2.7", capturedRequest.Model)
}

func TestMiniMaxProvider_Completion_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"Invalid API key","type":"auth_error"}}`))
	}))
	t.Cleanup(func() { server.Close() })

	p := newMiniMaxProvider(providers.MiniMaxConfig{
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

		chunk := providerbase.OpenAICompatResponse{
			ID:    "stream-1",
			Model: "abab6.5s-chat",
			Choices: []providerbase.OpenAICompatChoice{
				{
					Index: 0,
					Delta: &providerbase.OpenAICompatMessage{
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

	p := newMiniMaxProvider(providers.MiniMaxConfig{
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

// TestMiniMaxProvider_Stream_LegacyModel_NoProviderLevelXMLParsing
// 旧模型的 XML 工具调用解析已迁移到框架级 XMLToolCallProvider，
// MiniMax Provider 自身不再做 XML 解析，内容原样透传。
func TestMiniMaxProvider_Stream_LegacyModel_NoProviderLevelXMLParsing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		chunk := providerbase.OpenAICompatResponse{
			ID:    "stream-tc",
			Model: "abab6.5s-chat",
			Choices: []providerbase.OpenAICompatChoice{
				{
					Index: 0,
					Delta: &providerbase.OpenAICompatMessage{
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

	// 旧模型 → SupportsNativeFunctionCalling() == false
	p := newMiniMaxProvider(providers.MiniMaxConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			APIKey:  "test-key",
			BaseURL: server.URL,
			Model:   "abab6.5s-chat",
		},
	}, zap.NewNop())

	assert.False(t, p.SupportsNativeFunctionCalling())

	ch, err := p.Stream(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "What is the weather?"}},
	})
	require.NoError(t, err)

	var chunks []llm.StreamChunk
	for c := range ch {
		chunks = append(chunks, c)
	}

	require.Len(t, chunks, 1)
	// Provider 不再做 XML 解析，内容原样透传
	// XML 解析由框架级 XMLToolCallProvider 负责
	assert.Contains(t, chunks[0].Delta.Content, "<tool_calls>")
	assert.Empty(t, chunks[0].Delta.ToolCalls, "Provider 自身不应解析 XML 工具调用")
}

func TestMiniMaxProvider_Stream_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"error":{"message":"Service unavailable"}}`))
	}))
	t.Cleanup(func() { server.Close() })

	p := newMiniMaxProvider(providers.MiniMaxConfig{
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
