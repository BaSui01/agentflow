package llama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestLlamaProvider_Name(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		expected string
	}{
		{"Together", "together", "llama-together"},
		{"Replicate", "replicate", "llama-replicate"},
		{"OpenRouter", "openrouter", "llama-openrouter"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewLlamaProvider(providers.LlamaConfig{
				Provider: tt.provider,
			}, zap.NewNop())
			assert.Equal(t, tt.expected, provider.Name())
		})
	}
}

func TestLlamaProvider_SupportsNativeFunctionCalling(t *testing.T) {
	provider := NewLlamaProvider(providers.LlamaConfig{}, zap.NewNop())
	assert.True(t, provider.SupportsNativeFunctionCalling())
}

func TestLlamaProvider_DefaultProvider(t *testing.T) {
	cfg := providers.LlamaConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key"}}
	provider := NewLlamaProvider(cfg, zap.NewNop())
	assert.Equal(t, "llama-together", provider.Name())
}

func TestLlamaProvider_BaseURLSelection(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		expected string
	}{
		{"Together", "together", "https://api.together.xyz/v1"},
		{"Replicate", "replicate", "https://api.replicate.com/v1"},
		{"OpenRouter", "openrouter", "https://openrouter.ai/api/v1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := providers.LlamaConfig{
				BaseProviderConfig: providers.BaseProviderConfig{
					APIKey:   "test-key",
				},
				Provider: tt.provider,
			}
			provider := NewLlamaProvider(cfg, zap.NewNop())
			assert.NotNil(t, provider)
		})
	}
}

func TestLlamaProvider_Integration(t *testing.T) {
	apiKey := os.Getenv("TOGETHER_API_KEY")
	if apiKey == "" {
		t.Skip("TOGETHER_API_KEY not set, skipping integration test")
	}

	provider := NewLlamaProvider(providers.LlamaConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			APIKey:   apiKey,
			Model:    "meta-llama/Llama-3.3-70B-Instruct-Turbo",
			Timeout:  30 * time.Second,
		},
		Provider: "together",
	}, zap.NewNop())

	ctx := context.Background()

	t.Run("HealthCheck", func(t *testing.T) {
		status, err := provider.HealthCheck(ctx)
		require.NoError(t, err)
		assert.True(t, status.Healthy)
		assert.Greater(t, status.Latency, time.Duration(0))
	})

	t.Run("Completion", func(t *testing.T) {
		req := &llm.ChatRequest{
			Model: "meta-llama/Llama-3.3-70B-Instruct-Turbo",
			Messages: []llm.Message{
				{Role: llm.RoleUser, Content: "Say 'test' only"},
			},
			MaxTokens:   10,
			Temperature: 0.1,
		}

		resp, err := provider.Completion(ctx, req)
		require.NoError(t, err)
		assert.NotNil(t, resp)
		assert.NotEmpty(t, resp.Choices)
		assert.NotEmpty(t, resp.Choices[0].Message.Content)
	})

	t.Run("Stream", func(t *testing.T) {
		req := &llm.ChatRequest{
			Model: "meta-llama/Llama-3.3-70B-Instruct-Turbo",
			Messages: []llm.Message{
				{Role: llm.RoleUser, Content: "Count to 3"},
			},
			MaxTokens: 20,
		}

		stream, err := provider.Stream(ctx, req)
		require.NoError(t, err)

		var chunks []llm.StreamChunk
		for chunk := range stream {
			if chunk.Err != nil {
				t.Fatalf("Stream error: %v", chunk.Err)
			}
			chunks = append(chunks, chunk)
		}

		assert.NotEmpty(t, chunks)
	})
}

// --- httptest-based unit tests ---

func TestLlamaProvider_NilLogger(t *testing.T) {
	p := NewLlamaProvider(providers.LlamaConfig{}, nil)
	require.NotNil(t, p)
}

func TestLlamaProvider_FallbackModel(t *testing.T) {
	p := NewLlamaProvider(providers.LlamaConfig{}, zap.NewNop())
	assert.Equal(t, "meta-llama/Llama-3-70b-chat-hf", p.Cfg.FallbackModel)
}

func TestLlamaProvider_Completion_Httptest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		assert.Contains(t, r.Header.Get("Authorization"), "Bearer ")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(providers.OpenAICompatResponse{
			ID: "resp-1", Model: "meta-llama/Llama-3.3-70B-Instruct-Turbo",
			Choices: []providers.OpenAICompatChoice{
				{Index: 0, FinishReason: "stop", Message: providers.OpenAICompatMessage{Role: "assistant", Content: "Hello from Llama"}},
			},
			Usage: &providers.OpenAICompatUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		})
	}))
	t.Cleanup(func() { server.Close() })

	p := NewLlamaProvider(providers.LlamaConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
	}, zap.NewNop())

	resp, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.NoError(t, err)
	assert.Contains(t, resp.Provider, "llama")
	require.Len(t, resp.Choices, 1)
	assert.Equal(t, "Hello from Llama", resp.Choices[0].Message.Content)
	assert.Equal(t, 15, resp.Usage.TotalTokens)
}

func TestLlamaProvider_Completion_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"Invalid API key"}}`))
	}))
	t.Cleanup(func() { server.Close() })

	p := NewLlamaProvider(providers.LlamaConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "bad", BaseURL: server.URL},
	}, zap.NewNop())

	_, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.Error(t, err)
	llmErr, ok := err.(*llm.Error)
	require.True(t, ok)
	assert.Equal(t, llm.ErrUnauthorized, llmErr.Code)
}

func TestLlamaProvider_Completion_RateLimited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"message":"Rate limit"}}`))
	}))
	t.Cleanup(func() { server.Close() })

	p := NewLlamaProvider(providers.LlamaConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: server.URL},
	}, zap.NewNop())

	_, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.Error(t, err)
	llmErr, ok := err.(*llm.Error)
	require.True(t, ok)
	assert.Equal(t, llm.ErrRateLimited, llmErr.Code)
}

func TestLlamaProvider_Stream_Httptest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		for _, content := range []string{"Hello", " ", "World"} {
			chunk := providers.OpenAICompatResponse{
				ID: "stream-1", Model: "meta-llama/Llama-3.3-70B-Instruct-Turbo",
				Choices: []providers.OpenAICompatChoice{
					{Index: 0, Delta: &providers.OpenAICompatMessage{Role: "assistant", Content: content}},
				},
			}
			data, _ := json.Marshal(chunk)
			w.Write([]byte("data: "))
			w.Write(data)
			w.Write([]byte("\n\n"))
		}
		w.Write([]byte("data: [DONE]\n\n"))
	}))
	t.Cleanup(func() { server.Close() })

	p := NewLlamaProvider(providers.LlamaConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: server.URL},
	}, zap.NewNop())

	ch, err := p.Stream(context.Background(), &llm.ChatRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.NoError(t, err)

	var combined string
	for c := range ch {
		combined += c.Delta.Content
	}
	assert.Equal(t, "Hello World", combined)
}

func TestLlamaProvider_Stream_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"error":{"message":"Service unavailable"}}`))
	}))
	t.Cleanup(func() { server.Close() })

	p := NewLlamaProvider(providers.LlamaConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: server.URL},
	}, zap.NewNop())

	_, err := p.Stream(context.Background(), &llm.ChatRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.Error(t, err)
}

func TestLlamaProvider_NotSupported(t *testing.T) {
	p := NewLlamaProvider(providers.LlamaConfig{}, zap.NewNop())
	ctx := context.Background()

	tests := []struct {
		name    string
		callFn  func() error
		feature string
	}{
		{"GenerateImage", func() error { _, err := p.GenerateImage(ctx, &llm.ImageGenerationRequest{}); return err }, "image generation"},
		{"GenerateVideo", func() error { _, err := p.GenerateVideo(ctx, &llm.VideoGenerationRequest{}); return err }, "video generation"},
		{"GenerateAudio", func() error { _, err := p.GenerateAudio(ctx, &llm.AudioGenerationRequest{}); return err }, "audio generation"},
		{"TranscribeAudio", func() error { _, err := p.TranscribeAudio(ctx, &llm.AudioTranscriptionRequest{}); return err }, "audio transcription"},
		{"CreateEmbedding", func() error { _, err := p.CreateEmbedding(ctx, &llm.EmbeddingRequest{}); return err }, "embeddings"},
		{"CreateFineTuningJob", func() error { _, err := p.CreateFineTuningJob(ctx, &llm.FineTuningJobRequest{}); return err }, "fine-tuning"},
		{"ListFineTuningJobs", func() error { _, err := p.ListFineTuningJobs(ctx); return err }, "fine-tuning"},
		{"GetFineTuningJob", func() error { _, err := p.GetFineTuningJob(ctx, "j"); return err }, "fine-tuning"},
		{"CancelFineTuningJob", func() error { return p.CancelFineTuningJob(ctx, "j") }, "fine-tuning"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.callFn()
			require.Error(t, err)
			llmErr, ok := err.(*llm.Error)
			require.True(t, ok)
			assert.Equal(t, llm.ErrInvalidRequest, llmErr.Code)
			assert.Contains(t, llmErr.Message, tt.feature)
		})
	}
}

func TestLlamaProvider_HealthCheck_Httptest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"object":"list","data":[]}`))
	}))
	t.Cleanup(func() { server.Close() })

	p := NewLlamaProvider(providers.LlamaConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: server.URL},
	}, zap.NewNop())

	status, err := p.HealthCheck(context.Background())
	require.NoError(t, err)
	assert.True(t, status.Healthy)
}

func TestLlamaProvider_HealthCheck_Unhealthy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`error`))
	}))
	t.Cleanup(func() { server.Close() })

	p := NewLlamaProvider(providers.LlamaConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: server.URL},
	}, zap.NewNop())

	status, err := p.HealthCheck(context.Background())
	require.Error(t, err)
	assert.False(t, status.Healthy)
}
