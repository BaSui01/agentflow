package deepseek

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

func TestNewDeepSeekProvider_Defaults(t *testing.T) {
	tests := []struct {
		name            string
		cfg             providers.DeepSeekConfig
		expectedBaseURL string
	}{
		{"empty config uses default", providers.DeepSeekConfig{}, "https://api.deepseek.com"},
		{"custom BaseURL preserved", providers.DeepSeekConfig{
			BaseProviderConfig: providers.BaseProviderConfig{BaseURL: "https://custom.example.com"},
		}, "https://custom.example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewDeepSeekProvider(tt.cfg, zap.NewNop())
			require.NotNil(t, p)
			assert.Equal(t, "deepseek", p.Name())
			assert.Equal(t, tt.expectedBaseURL, p.Cfg.BaseURL)
		})
	}
}

func TestDeepSeekProvider_FallbackModel(t *testing.T) {
	p := NewDeepSeekProvider(providers.DeepSeekConfig{}, zap.NewNop())
	assert.Equal(t, "deepseek-chat", p.Cfg.FallbackModel)
}

func TestDeepSeekProvider_EndpointPath(t *testing.T) {
	p := NewDeepSeekProvider(providers.DeepSeekConfig{}, zap.NewNop())
	assert.Equal(t, "/chat/completions", p.Cfg.EndpointPath)
}

func TestDeepSeekProvider_NilLogger(t *testing.T) {
	p := NewDeepSeekProvider(providers.DeepSeekConfig{}, nil)
	require.NotNil(t, p)
	assert.Equal(t, "deepseek", p.Name())
}

func TestDeepSeekProvider_SupportsNativeFunctionCalling(t *testing.T) {
	p := NewDeepSeekProvider(providers.DeepSeekConfig{}, zap.NewNop())
	assert.True(t, p.SupportsNativeFunctionCalling())
}

func TestDeepSeekProvider_Completion(t *testing.T) {
	var capturedRequest providers.OpenAICompatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/chat/completions", r.URL.Path)
		assert.Contains(t, r.Header.Get("Authorization"), "Bearer ")
		err := json.NewDecoder(r.Body).Decode(&capturedRequest)
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/json")
		err = json.NewEncoder(w).Encode(providers.OpenAICompatResponse{
			ID: "resp-1", Model: "deepseek-chat",
			Choices: []providers.OpenAICompatChoice{
				{Index: 0, FinishReason: "stop", Message: providers.OpenAICompatMessage{Role: "assistant", Content: "Hello from DeepSeek"}},
			},
			Usage: &providers.OpenAICompatUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		})
		require.NoError(t, err)
	}))
	t.Cleanup(func() { server.Close() })

	p := NewDeepSeekProvider(providers.DeepSeekConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
	}, zap.NewNop())

	resp, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.NoError(t, err)
	assert.Equal(t, "deepseek", resp.Provider)
	assert.Equal(t, "deepseek-chat", resp.Model)
	require.Len(t, resp.Choices, 1)
	assert.Equal(t, "Hello from DeepSeek", resp.Choices[0].Message.Content)
	assert.Equal(t, 15, resp.Usage.TotalTokens)
}

func TestDeepSeekProvider_Completion_ReasoningMode(t *testing.T) {
	var capturedRequest providers.OpenAICompatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedRequest)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(providers.OpenAICompatResponse{
			ID: "resp-2", Model: "deepseek-reasoner",
			Choices: []providers.OpenAICompatChoice{
				{Index: 0, FinishReason: "stop", Message: providers.OpenAICompatMessage{Role: "assistant", Content: "Thinking..."}},
			},
		})
	}))
	t.Cleanup(func() { server.Close() })

	p := NewDeepSeekProvider(providers.DeepSeekConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: server.URL},
	}, zap.NewNop())

	resp, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages:      []types.Message{{Role: llm.RoleUser, Content: "Think about this"}},
		ReasoningMode: "thinking",
	})
	require.NoError(t, err)
	assert.Equal(t, "deepseek-reasoner", capturedRequest.Model)
	assert.NotNil(t, resp)
}

func TestDeepSeekProvider_Completion_ReasoningMode_Extended(t *testing.T) {
	var capturedRequest providers.OpenAICompatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedRequest)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(providers.OpenAICompatResponse{
			ID: "resp-3", Model: "deepseek-reasoner",
			Choices: []providers.OpenAICompatChoice{
				{Index: 0, FinishReason: "stop", Message: providers.OpenAICompatMessage{Role: "assistant", Content: "ok"}},
			},
		})
	}))
	t.Cleanup(func() { server.Close() })

	p := NewDeepSeekProvider(providers.DeepSeekConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: server.URL},
	}, zap.NewNop())

	_, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages:      []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
		ReasoningMode: "extended",
	})
	require.NoError(t, err)
	assert.Equal(t, "deepseek-reasoner", capturedRequest.Model)
}

func TestDeepSeekProvider_Completion_ReasoningMode_WithExplicitModel(t *testing.T) {
	var capturedRequest providers.OpenAICompatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedRequest)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(providers.OpenAICompatResponse{
			ID: "resp-4", Model: "deepseek-chat",
			Choices: []providers.OpenAICompatChoice{
				{Index: 0, FinishReason: "stop", Message: providers.OpenAICompatMessage{Role: "assistant", Content: "ok"}},
			},
		})
	}))
	t.Cleanup(func() { server.Close() })

	p := NewDeepSeekProvider(providers.DeepSeekConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: server.URL},
	}, zap.NewNop())

	// When model is explicitly set, the hook should NOT override it
	_, err := p.Completion(context.Background(), &llm.ChatRequest{
		Model:         "deepseek-chat",
		Messages:      []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
		ReasoningMode: "thinking",
	})
	require.NoError(t, err)
	assert.Equal(t, "deepseek-chat", capturedRequest.Model)
}

func TestDeepSeekProvider_Completion_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"Invalid API key"}}`))
	}))
	t.Cleanup(func() { server.Close() })

	p := NewDeepSeekProvider(providers.DeepSeekConfig{
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

func TestDeepSeekProvider_Stream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/chat/completions", r.URL.Path)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		chunk := providers.OpenAICompatResponse{
			ID: "stream-1", Model: "deepseek-chat",
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

	p := NewDeepSeekProvider(providers.DeepSeekConfig{
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
	assert.Equal(t, "deepseek", chunks[0].Provider)
}

func TestDeepSeekProvider_Stream_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"message":"Rate limit"}}`))
	}))
	t.Cleanup(func() { server.Close() })

	p := NewDeepSeekProvider(providers.DeepSeekConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: server.URL},
	}, zap.NewNop())

	_, err := p.Stream(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.Error(t, err)
}

func TestDeepSeekProvider_NotSupported(t *testing.T) {
	p := NewDeepSeekProvider(providers.DeepSeekConfig{}, zap.NewNop())
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
			assert.Equal(t, "deepseek", llmErr.Provider)
		})
	}
}

func TestDeepSeekProvider_HealthCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"object":"list","data":[]}`))
	}))
	t.Cleanup(func() { server.Close() })

	p := NewDeepSeekProvider(providers.DeepSeekConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: server.URL},
	}, zap.NewNop())

	status, err := p.HealthCheck(context.Background())
	require.NoError(t, err)
	assert.True(t, status.Healthy)
}

// --- deepseekRequestHook unit test ---

func TestDeepseekRequestHook(t *testing.T) {
	tests := []struct {
		name          string
		reqModel      string
		reasoningMode string
		expectedModel string
	}{
		{"thinking mode no model", "", "thinking", "deepseek-reasoner"},
		{"extended mode no model", "", "extended", "deepseek-reasoner"},
		{"thinking mode with model", "deepseek-chat", "thinking", "deepseek-chat"},
		{"no reasoning mode", "", "", "deepseek-chat"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &llm.ChatRequest{Model: tt.reqModel, ReasoningMode: tt.reasoningMode}
			body := &providers.OpenAICompatRequest{Model: "deepseek-chat"}
			deepseekRequestHook(req, body)
			assert.Equal(t, tt.expectedModel, body.Model)
		})
	}
}


