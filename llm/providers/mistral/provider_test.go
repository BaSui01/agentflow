package mistral

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// --- Constructor and defaults ---

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

func TestMistralProvider_SupportsNativeFunctionCalling(t *testing.T) {
	p := NewMistralProvider(providers.MistralConfig{}, zap.NewNop())
	assert.True(t, p.SupportsNativeFunctionCalling())
}

func TestMistralProvider_SupportsStructuredOutput(t *testing.T) {
	p := NewMistralProvider(providers.MistralConfig{}, zap.NewNop())
	assert.True(t, p.SupportsStructuredOutput())
}

func TestMistralProvider_EndpointPath(t *testing.T) {
	p := NewMistralProvider(providers.MistralConfig{}, zap.NewNop())
	assert.Equal(t, "/v1/chat/completions", p.Cfg.EndpointPath)
}

// --- Completion via httptest ---

func TestMistralProvider_Completion(t *testing.T) {
	var capturedRequest providers.OpenAICompatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		assert.Contains(t, r.Header.Get("Authorization"), "Bearer ")

		err := json.NewDecoder(r.Body).Decode(&capturedRequest)
		require.NoError(t, err)

		w.Header().Set("Content-Type", "application/json")
		err = json.NewEncoder(w).Encode(providers.OpenAICompatResponse{
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
		require.NoError(t, err)
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
		Messages: []types.Message{
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

func TestMistralProvider_Completion_WithCustomModel(t *testing.T) {
	var capturedRequest providers.OpenAICompatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := json.NewDecoder(r.Body).Decode(&capturedRequest)
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/json")
		err = json.NewEncoder(w).Encode(providers.OpenAICompatResponse{
			ID:    "resp-2",
			Model: "mistral-small-latest",
			Choices: []providers.OpenAICompatChoice{
				{Index: 0, FinishReason: "stop", Message: providers.OpenAICompatMessage{Role: "assistant", Content: "ok"}},
			},
		})
		require.NoError(t, err)
	}))
	t.Cleanup(func() { server.Close() })

	p := NewMistralProvider(providers.MistralConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: server.URL},
	}, zap.NewNop())

	resp, err := p.Completion(context.Background(), &llm.ChatRequest{
		Model:    "mistral-small-latest",
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.NoError(t, err)
	assert.Equal(t, "mistral-small-latest", capturedRequest.Model)
	assert.Equal(t, "mistral-small-latest", resp.Model)
}

func TestMistralProvider_Completion_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		err := json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"message": "Invalid API key", "type": "authentication_error"},
		})
		require.NoError(t, err)
	}))
	t.Cleanup(func() { server.Close() })

	p := NewMistralProvider(providers.MistralConfig{
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

func TestMistralProvider_Completion_RateLimited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, err := w.Write([]byte(`{"error":{"message":"Rate limit exceeded"}}`))
		require.NoError(t, err)
	}))
	t.Cleanup(func() { server.Close() })

	p := NewMistralProvider(providers.MistralConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: server.URL},
	}, zap.NewNop())

	_, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.Error(t, err)
	llmErr, ok := err.(*types.Error)
	require.True(t, ok)
	assert.Equal(t, llm.ErrRateLimit, llmErr.Code)
	assert.True(t, llmErr.Retryable)
}

func TestMistralProvider_Completion_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, err := w.Write([]byte(`{"error":{"message":"Service unavailable"}}`))
		require.NoError(t, err)
	}))
	t.Cleanup(func() { server.Close() })

	p := NewMistralProvider(providers.MistralConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: server.URL},
	}, zap.NewNop())

	_, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.Error(t, err)
	llmErr, ok := err.(*types.Error)
	require.True(t, ok)
	assert.Equal(t, llm.ErrUpstreamError, llmErr.Code)
	assert.True(t, llmErr.Retryable)
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
				{Index: 0, Delta: &providers.OpenAICompatMessage{Role: "assistant", Content: "Hello"}},
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

	p := NewMistralProvider(providers.MistralConfig{
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
	assert.Equal(t, "mistral", chunks[0].Provider)
}

func TestMistralProvider_Stream_MultipleChunks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		for _, content := range []string{"Hello", " ", "World"} {
			chunk := providers.OpenAICompatResponse{
				ID:    "stream-2",
				Model: "mistral-large-latest",
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

	p := NewMistralProvider(providers.MistralConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: server.URL},
	}, zap.NewNop())

	ch, err := p.Stream(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.NoError(t, err)

	var combined string
	for c := range ch {
		combined += c.Delta.Content
	}
	assert.Equal(t, "Hello World", combined)
}

func TestMistralProvider_Stream_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"message":"Rate limit exceeded"}}`))
	}))
	t.Cleanup(func() { server.Close() })

	p := NewMistralProvider(providers.MistralConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
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

func TestMistralProvider_NotSupported(t *testing.T) {
	p := NewMistralProvider(providers.MistralConfig{}, zap.NewNop())
	ctx := context.Background()

	tests := []struct {
		name    string
		callFn  func() error
		feature string
	}{
		{
			name:    "GenerateImage returns not supported",
			callFn:  func() error { _, err := p.GenerateImage(ctx, &llm.ImageGenerationRequest{}); return err },
			feature: "image generation",
		},
		{
			name:    "GenerateVideo returns not supported",
			callFn:  func() error { _, err := p.GenerateVideo(ctx, &llm.VideoGenerationRequest{}); return err },
			feature: "video generation",
		},
		{
			name:    "GenerateAudio returns not supported",
			callFn:  func() error { _, err := p.GenerateAudio(ctx, &llm.AudioGenerationRequest{}); return err },
			feature: "audio generation",
		},
		{
			name:    "CreateFineTuningJob returns not supported",
			callFn:  func() error { _, err := p.CreateFineTuningJob(ctx, &llm.FineTuningJobRequest{}); return err },
			feature: "fine-tuning",
		},
		{
			name:    "ListFineTuningJobs returns not supported",
			callFn:  func() error { _, err := p.ListFineTuningJobs(ctx); return err },
			feature: "fine-tuning",
		},
		{
			name:    "GetFineTuningJob returns not supported",
			callFn:  func() error { _, err := p.GetFineTuningJob(ctx, "job-1"); return err },
			feature: "fine-tuning",
		},
		{
			name:    "CancelFineTuningJob returns not supported",
			callFn:  func() error { return p.CancelFineTuningJob(ctx, "job-1") },
			feature: "fine-tuning",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.callFn()
			require.Error(t, err)
			llmErr, ok := err.(*types.Error)
			require.True(t, ok, "error should be *types.Error")
			assert.Equal(t, llm.ErrInvalidRequest, llmErr.Code)
			assert.Contains(t, llmErr.Message, tt.feature)
			assert.Equal(t, http.StatusNotImplemented, llmErr.HTTPStatus)
			assert.Equal(t, "mistral", llmErr.Provider)
		})
	}
}

// --- TranscribeAudio via httptest ---

func TestMistralProvider_TranscribeAudio(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/v1/audio/transcriptions")
		assert.Contains(t, r.Header.Get("Content-Type"), "multipart/form-data")

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(llm.AudioTranscriptionResponse{
			Text: "Hello world",
		})
	}))
	t.Cleanup(func() { server.Close() })

	p := NewMistralProvider(providers.MistralConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
	}, zap.NewNop())

	resp, err := p.TranscribeAudio(context.Background(), &llm.AudioTranscriptionRequest{
		File: []byte("fake-audio-data"),
	})
	require.NoError(t, err)
	assert.Equal(t, "Hello world", resp.Text)
}

func TestMistralProvider_TranscribeAudio_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"message":"Invalid file"}}`))
	}))
	t.Cleanup(func() { server.Close() })

	p := NewMistralProvider(providers.MistralConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: server.URL},
	}, zap.NewNop())

	_, err := p.TranscribeAudio(context.Background(), &llm.AudioTranscriptionRequest{
		File: []byte("bad"),
	})
	require.Error(t, err)
}

// --- CreateEmbedding via httptest ---

func TestMistralProvider_CreateEmbedding(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/v1/embeddings")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(llm.EmbeddingResponse{
			Data: []llm.Embedding{{Index: 0, Embedding: []float64{0.1, 0.2, 0.3}}},
		})
	}))
	t.Cleanup(func() { server.Close() })

	p := NewMistralProvider(providers.MistralConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: server.URL},
	}, zap.NewNop())

	resp, err := p.CreateEmbedding(context.Background(), &llm.EmbeddingRequest{
		Input: []string{"test text"},
	})
	require.NoError(t, err)
	require.Len(t, resp.Data, 1)
	assert.Equal(t, 0, resp.Data[0].Index)
}

func TestMistralProvider_CreateEmbedding_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"Unauthorized"}}`))
	}))
	t.Cleanup(func() { server.Close() })

	p := NewMistralProvider(providers.MistralConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "bad", BaseURL: server.URL},
	}, zap.NewNop())

	_, err := p.CreateEmbedding(context.Background(), &llm.EmbeddingRequest{Input: []string{"test"}})
	require.Error(t, err)
}

// --- HealthCheck via httptest ---

func TestMistralProvider_HealthCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/models", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"object":"list","data":[]}`))
	}))
	t.Cleanup(func() { server.Close() })

	p := NewMistralProvider(providers.MistralConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: server.URL},
	}, zap.NewNop())

	status, err := p.HealthCheck(context.Background())
	require.NoError(t, err)
	assert.True(t, status.Healthy)
	assert.GreaterOrEqual(t, status.Latency, time.Duration(0))
}

func TestMistralProvider_HealthCheck_Unhealthy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`error`))
	}))
	t.Cleanup(func() { server.Close() })

	p := NewMistralProvider(providers.MistralConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: server.URL},
	}, zap.NewNop())

	status, err := p.HealthCheck(context.Background())
	require.Error(t, err)
	assert.False(t, status.Healthy)
}
