package glm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	providerbase "github.com/BaSui01/agentflow/llm/providers/base"

	"github.com/BaSui01/agentflow/types"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// --- Constructor and defaults ---

func TestNewGLMProvider_Defaults(t *testing.T) {
	tests := []struct {
		name            string
		cfg             providers.GLMConfig
		expectedBaseURL string
	}{
		{
			name:            "empty config uses default BaseURL",
			cfg:             providers.GLMConfig{},
			expectedBaseURL: "https://open.bigmodel.cn",
		},
		{
			name: "custom BaseURL is preserved",
			cfg: providers.GLMConfig{
				BaseProviderConfig: providers.BaseProviderConfig{BaseURL: "https://custom.example.com"},
			},
			expectedBaseURL: "https://custom.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newGLMProvider(tt.cfg, zap.NewNop())
			require.NotNil(t, p)
			assert.Equal(t, "glm", p.Name())
			assert.Equal(t, tt.expectedBaseURL, p.Cfg.BaseURL)
		})
	}
}

func TestGLMProvider_FallbackModel(t *testing.T) {
	p := newGLMProvider(providers.GLMConfig{}, zap.NewNop())
	assert.Equal(t, "glm-5.1", p.Cfg.FallbackModel)
}

func TestGLMProvider_EndpointPath(t *testing.T) {
	p := newGLMProvider(providers.GLMConfig{}, zap.NewNop())
	assert.Equal(t, "/api/paas/v4/chat/completions", p.Cfg.EndpointPath)
}

func TestGLMProvider_NilLogger(t *testing.T) {
	p := newGLMProvider(providers.GLMConfig{}, nil)
	require.NotNil(t, p)
	assert.Equal(t, "glm", p.Name())
}

func TestGLMProvider_SupportsNativeFunctionCalling(t *testing.T) {
	p := newGLMProvider(providers.GLMConfig{}, zap.NewNop())
	assert.True(t, p.SupportsNativeFunctionCalling())
}

// --- Completion via httptest ---

func TestGLMProvider_Completion(t *testing.T) {
	var capturedRequest providerbase.OpenAICompatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/paas/v4/chat/completions", r.URL.Path)
		assert.Contains(t, r.Header.Get("Authorization"), "Bearer ")
		err := json.NewDecoder(r.Body).Decode(&capturedRequest)
		require.NoError(t, err)

		w.Header().Set("Content-Type", "application/json")
		err = json.NewEncoder(w).Encode(providerbase.OpenAICompatResponse{
			ID: "resp-1", Model: "glm-5.1",
			Choices: []providerbase.OpenAICompatChoice{
				{Index: 0, FinishReason: "stop", Message: providerbase.OpenAICompatMessage{Role: "assistant", Content: "Hello from GLM"}},
			},
			Usage: &providerbase.OpenAICompatUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		})
		require.NoError(t, err)
	}))
	t.Cleanup(func() { server.Close() })

	p := newGLMProvider(providers.GLMConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
	}, zap.NewNop())

	resp, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "glm", resp.Provider)
	assert.Equal(t, "glm-5.1", resp.Model)
	require.Len(t, resp.Choices, 1)
	assert.Equal(t, "Hello from GLM", resp.Choices[0].Message.Content)
	assert.Equal(t, 15, resp.Usage.TotalTokens)
	assert.Equal(t, "glm-5.1", capturedRequest.Model)
}

func TestGLMProvider_Completion_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, err := w.Write([]byte(`{"error":{"message":"Invalid API key"}}`))
		require.NoError(t, err)
	}))
	t.Cleanup(func() { server.Close() })

	p := newGLMProvider(providers.GLMConfig{
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

func TestGLMProvider_Completion_RateLimited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, err := w.Write([]byte(`{"error":{"message":"Rate limit"}}`))
		require.NoError(t, err)
	}))
	t.Cleanup(func() { server.Close() })

	p := newGLMProvider(providers.GLMConfig{
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

// --- Stream via httptest ---

func TestGLMProvider_Stream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/paas/v4/chat/completions", r.URL.Path)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		chunk := providerbase.OpenAICompatResponse{
			ID: "stream-1", Model: "glm-5.1",
			Choices: []providerbase.OpenAICompatChoice{
				{Index: 0, Delta: &providerbase.OpenAICompatMessage{Role: "assistant", Content: "Hello"}},
			},
		}
		data, _ := json.Marshal(chunk)
		_, err := w.Write([]byte("data: "))
		require.NoError(t, err)
		_, err = w.Write(data)
		require.NoError(t, err)
		_, err = w.Write([]byte("\n\ndata: [DONE]\n\n"))
		require.NoError(t, err)
	}))
	t.Cleanup(func() { server.Close() })

	p := newGLMProvider(providers.GLMConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
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
	assert.Equal(t, "glm", chunks[0].Provider)
}

func TestGLMProvider_Stream_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, err := w.Write([]byte(`{"error":{"message":"Rate limit"}}`))
		require.NoError(t, err)
	}))
	t.Cleanup(func() { server.Close() })

	p := newGLMProvider(providers.GLMConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: server.URL},
	}, zap.NewNop())

	_, err := p.Stream(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.Error(t, err)
	llmErr, ok := err.(*types.Error)
	require.True(t, ok)
	assert.Equal(t, llm.ErrRateLimit, llmErr.Code)
}

// --- Multimodal not-supported methods ---

func TestGLMProvider_NotSupported(t *testing.T) {
	p := newGLMProvider(providers.GLMConfig{}, zap.NewNop())
	ctx := context.Background()

	tests := []struct {
		name    string
		callFn  func() error
		feature string
	}{
		{"TranscribeAudio", func() error { _, err := p.TranscribeAudio(ctx, &llm.AudioTranscriptionRequest{}); return err }, "audio transcription"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.callFn()
			require.Error(t, err)
			llmErr, ok := err.(*types.Error)
			require.True(t, ok)
			assert.Equal(t, llm.ErrInvalidRequest, llmErr.Code)
			assert.Contains(t, llmErr.Message, tt.feature)
			assert.Equal(t, "glm", llmErr.Provider)
		})
	}
}

// --- HealthCheck ---

func TestGLMProvider_HealthCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"object":"list","data":[]}`))
		require.NoError(t, err)
	}))
	t.Cleanup(func() { server.Close() })

	p := newGLMProvider(providers.GLMConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: server.URL},
	}, zap.NewNop())

	status, err := p.HealthCheck(context.Background())
	require.NoError(t, err)
	assert.True(t, status.Healthy)
}
