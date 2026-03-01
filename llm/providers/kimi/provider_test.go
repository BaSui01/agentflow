package kimi

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

func TestNewKimiProvider_Defaults(t *testing.T) {
	tests := []struct {
		name            string
		cfg             providers.KimiConfig
		expectedBaseURL string
	}{
		{"empty config uses default", providers.KimiConfig{}, "https://api.moonshot.cn"},
		{"custom BaseURL preserved", providers.KimiConfig{
			BaseProviderConfig: providers.BaseProviderConfig{BaseURL: "https://custom.example.com"},
		}, "https://custom.example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewKimiProvider(tt.cfg, zap.NewNop())
			require.NotNil(t, p)
			assert.Equal(t, "kimi", p.Name())
			assert.Equal(t, tt.expectedBaseURL, p.Cfg.BaseURL)
		})
	}
}

func TestKimiProvider_FallbackModel(t *testing.T) {
	p := NewKimiProvider(providers.KimiConfig{}, zap.NewNop())
	assert.Equal(t, "moonshot-v1-32k", p.Cfg.FallbackModel)
}

func TestKimiProvider_NilLogger(t *testing.T) {
	p := NewKimiProvider(providers.KimiConfig{}, nil)
	require.NotNil(t, p)
	assert.Equal(t, "kimi", p.Name())
}

func TestKimiProvider_SupportsNativeFunctionCalling(t *testing.T) {
	p := NewKimiProvider(providers.KimiConfig{}, zap.NewNop())
	assert.True(t, p.SupportsNativeFunctionCalling())
}

func TestKimiProvider_Completion(t *testing.T) {
	var capturedRequest providers.OpenAICompatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		json.NewDecoder(r.Body).Decode(&capturedRequest)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(providers.OpenAICompatResponse{
			ID: "resp-1", Model: "moonshot-v1-8k",
			Choices: []providers.OpenAICompatChoice{
				{Index: 0, FinishReason: "stop", Message: providers.OpenAICompatMessage{Role: "assistant", Content: "Hello from Kimi"}},
			},
			Usage: &providers.OpenAICompatUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		})
	}))
	t.Cleanup(func() { server.Close() })

	p := NewKimiProvider(providers.KimiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
	}, zap.NewNop())

	resp, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.NoError(t, err)
	assert.Equal(t, "kimi", resp.Provider)
	assert.Equal(t, "moonshot-v1-8k", resp.Model)
	require.Len(t, resp.Choices, 1)
	assert.Equal(t, "Hello from Kimi", resp.Choices[0].Message.Content)
	assert.Equal(t, 15, resp.Usage.TotalTokens)
}

func TestKimiProvider_Completion_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"Invalid API key"}}`))
	}))
	t.Cleanup(func() { server.Close() })

	p := NewKimiProvider(providers.KimiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "bad", BaseURL: server.URL},
	}, zap.NewNop())

	_, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.Error(t, err)
	llmErr, ok := err.(*types.Error)
	require.True(t, ok)
	assert.Equal(t, llm.ErrUnauthorized, llmErr.Code)
}

func TestKimiProvider_Completion_RateLimited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"message":"Rate limit"}}`))
	}))
	t.Cleanup(func() { server.Close() })

	p := NewKimiProvider(providers.KimiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: server.URL},
	}, zap.NewNop())

	_, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.Error(t, err)
	llmErr, ok := err.(*types.Error)
	require.True(t, ok)
	assert.Equal(t, llm.ErrRateLimit, llmErr.Code)
}

func TestKimiProvider_Stream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		chunk := providers.OpenAICompatResponse{
			ID: "stream-1", Model: "moonshot-v1-8k",
			Choices: []providers.OpenAICompatChoice{
				{Index: 0, Delta: &providers.OpenAICompatMessage{Role: "assistant", Content: "Hello"}},
			},
		}
		data, _ := json.Marshal(chunk)
		w.Write([]byte("data: "))
		w.Write(data)
		w.Write([]byte("\n\ndata: [DONE]\n\n"))
	}))
	t.Cleanup(func() { server.Close() })

	p := NewKimiProvider(providers.KimiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: server.URL},
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
	assert.Equal(t, "kimi", chunks[0].Provider)
}

func TestKimiProvider_Stream_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"message":"Rate limit"}}`))
	}))
	t.Cleanup(func() { server.Close() })

	p := NewKimiProvider(providers.KimiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: server.URL},
	}, zap.NewNop())

	_, err := p.Stream(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.Error(t, err)
}

func TestKimiProvider_NotSupported(t *testing.T) {
	p := NewKimiProvider(providers.KimiConfig{}, zap.NewNop())
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
			llmErr, ok := err.(*types.Error)
			require.True(t, ok)
			assert.Equal(t, llm.ErrInvalidRequest, llmErr.Code)
			assert.Contains(t, llmErr.Message, tt.feature)
			assert.Equal(t, "kimi", llmErr.Provider)
		})
	}
}

func TestKimiProvider_HealthCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"object":"list","data":[]}`))
	}))
	t.Cleanup(func() { server.Close() })

	p := NewKimiProvider(providers.KimiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: server.URL},
	}, zap.NewNop())

	status, err := p.HealthCheck(context.Background())
	require.NoError(t, err)
	assert.True(t, status.Healthy)
}


