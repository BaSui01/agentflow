package qwen

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

func TestNewQwenProvider_Defaults(t *testing.T) {
	tests := []struct {
		name            string
		cfg             providers.QwenConfig
		expectedBaseURL string
	}{
		{"empty config uses default", providers.QwenConfig{}, "https://dashscope.aliyuncs.com"},
		{"custom BaseURL preserved", providers.QwenConfig{
			BaseProviderConfig: providers.BaseProviderConfig{BaseURL: "https://custom.example.com"},
		}, "https://custom.example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewQwenProvider(tt.cfg, zap.NewNop())
			require.NotNil(t, p)
			assert.Equal(t, "qwen", p.Name())
			assert.Equal(t, tt.expectedBaseURL, p.Cfg.BaseURL)
		})
	}
}

func TestQwenProvider_FallbackModel(t *testing.T) {
	p := NewQwenProvider(providers.QwenConfig{}, zap.NewNop())
	assert.Equal(t, "qwen3-235b-a22b", p.Cfg.FallbackModel)
}

func TestQwenProvider_EndpointPath(t *testing.T) {
	p := NewQwenProvider(providers.QwenConfig{}, zap.NewNop())
	assert.Equal(t, "/compatible-mode/v1/chat/completions", p.Cfg.EndpointPath)
}

func TestQwenProvider_NilLogger(t *testing.T) {
	p := NewQwenProvider(providers.QwenConfig{}, nil)
	require.NotNil(t, p)
	assert.Equal(t, "qwen", p.Name())
}

func TestQwenProvider_SupportsNativeFunctionCalling(t *testing.T) {
	p := NewQwenProvider(providers.QwenConfig{}, zap.NewNop())
	assert.True(t, p.SupportsNativeFunctionCalling())
}

func TestQwenProvider_Completion(t *testing.T) {
	var capturedRequest providers.OpenAICompatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/compatible-mode/v1/chat/completions", r.URL.Path)
		assert.Contains(t, r.Header.Get("Authorization"), "Bearer ")
		json.NewDecoder(r.Body).Decode(&capturedRequest)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(providers.OpenAICompatResponse{
			ID: "resp-1", Model: "qwen3-235b-a22b",
			Choices: []providers.OpenAICompatChoice{
				{Index: 0, FinishReason: "stop", Message: providers.OpenAICompatMessage{Role: "assistant", Content: "Hello from Qwen"}},
			},
			Usage: &providers.OpenAICompatUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		})
	}))
	t.Cleanup(func() { server.Close() })

	p := NewQwenProvider(providers.QwenConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
	}, zap.NewNop())

	resp, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.NoError(t, err)
	assert.Equal(t, "qwen", resp.Provider)
	assert.Equal(t, "qwen3-235b-a22b", resp.Model)
	require.Len(t, resp.Choices, 1)
	assert.Equal(t, "Hello from Qwen", resp.Choices[0].Message.Content)
	assert.Equal(t, 15, resp.Usage.TotalTokens)
}

func TestQwenProvider_Completion_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"Invalid API key"}}`))
	}))
	t.Cleanup(func() { server.Close() })

	p := NewQwenProvider(providers.QwenConfig{
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

func TestQwenProvider_Completion_RateLimited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"message":"Rate limit"}}`))
	}))
	t.Cleanup(func() { server.Close() })

	p := NewQwenProvider(providers.QwenConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: server.URL},
	}, zap.NewNop())

	_, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.Error(t, err)
	llmErr, ok := err.(*llm.Error)
	require.True(t, ok)
	assert.Equal(t, llm.ErrRateLimited, llmErr.Code)
	assert.True(t, llmErr.Retryable)
}

func TestQwenProvider_Stream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/compatible-mode/v1/chat/completions", r.URL.Path)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		chunk := providers.OpenAICompatResponse{
			ID: "stream-1", Model: "qwen3-235b-a22b",
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

	p := NewQwenProvider(providers.QwenConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: server.URL},
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
	assert.Equal(t, "qwen", chunks[0].Provider)
}

func TestQwenProvider_Stream_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"message":"Rate limit"}}`))
	}))
	t.Cleanup(func() { server.Close() })

	p := NewQwenProvider(providers.QwenConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: server.URL},
	}, zap.NewNop())

	_, err := p.Stream(context.Background(), &llm.ChatRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.Error(t, err)
}

func TestQwenProvider_NotSupported(t *testing.T) {
	p := NewQwenProvider(providers.QwenConfig{}, zap.NewNop())
	ctx := context.Background()

	tests := []struct {
		name    string
		callFn  func() error
		feature string
	}{
		{"GenerateVideo", func() error { _, err := p.GenerateVideo(ctx, &llm.VideoGenerationRequest{}); return err }, "video generation"},
		{"TranscribeAudio", func() error { _, err := p.TranscribeAudio(ctx, &llm.AudioTranscriptionRequest{}); return err }, "audio transcription"},
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
			assert.Equal(t, "qwen", llmErr.Provider)
		})
	}
}

func TestQwenProvider_HealthCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"object":"list","data":[]}`))
	}))
	t.Cleanup(func() { server.Close() })

	p := NewQwenProvider(providers.QwenConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: server.URL},
	}, zap.NewNop())

	status, err := p.HealthCheck(context.Background())
	require.NoError(t, err)
	assert.True(t, status.Healthy)
}
