package doubao

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

// --- Constructor and defaults ---

func TestNewDoubaoProvider_Defaults(t *testing.T) {
	tests := []struct {
		name            string
		cfg             providers.DoubaoConfig
		expectedBaseURL string
		expectedName    string
	}{
		{
			name:            "empty config uses default BaseURL",
			cfg:             providers.DoubaoConfig{},
			expectedBaseURL: "https://ark.cn-beijing.volces.com",
			expectedName:    "doubao",
		},
		{
			name: "custom BaseURL is preserved",
			cfg: providers.DoubaoConfig{
				BaseProviderConfig: providers.BaseProviderConfig{
					BaseURL: "https://custom.example.com",
				},
			},
			expectedBaseURL: "https://custom.example.com",
			expectedName:    "doubao",
		},
		{
			name: "API key is passed through",
			cfg: providers.DoubaoConfig{
				BaseProviderConfig: providers.BaseProviderConfig{
					APIKey: "test-key-123",
				},
			},
			expectedBaseURL: "https://ark.cn-beijing.volces.com",
			expectedName:    "doubao",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewDoubaoProvider(tt.cfg, zap.NewNop())
			require.NotNil(t, p)
			assert.Equal(t, tt.expectedName, p.Name())
			assert.Equal(t, tt.expectedBaseURL, p.Cfg.BaseURL)
		})
	}
}

func TestDoubaoProvider_EndpointPath(t *testing.T) {
	p := NewDoubaoProvider(providers.DoubaoConfig{}, zap.NewNop())
	assert.Equal(t, "/api/v3/chat/completions", p.Cfg.EndpointPath)
}

func TestDoubaoProvider_FallbackModel(t *testing.T) {
	p := NewDoubaoProvider(providers.DoubaoConfig{}, zap.NewNop())
	assert.Equal(t, "Doubao-1.5-pro-32k", p.Cfg.FallbackModel)
}

func TestDoubaoProvider_SupportsNativeFunctionCalling(t *testing.T) {
	p := NewDoubaoProvider(providers.DoubaoConfig{}, zap.NewNop())
	assert.True(t, p.SupportsNativeFunctionCalling())
}

func TestDoubaoProvider_NilLogger(t *testing.T) {
	p := NewDoubaoProvider(providers.DoubaoConfig{}, nil)
	require.NotNil(t, p)
	assert.Equal(t, "doubao", p.Name())
}

// --- Multimodal not-supported methods ---

func TestDoubaoProvider_NotSupported(t *testing.T) {
	p := NewDoubaoProvider(providers.DoubaoConfig{}, zap.NewNop())
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
			name:    "TranscribeAudio returns not supported",
			callFn:  func() error { _, err := p.TranscribeAudio(ctx, &llm.AudioTranscriptionRequest{}); return err },
			feature: "audio transcription",
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
			llmErr, ok := err.(*llm.Error)
			require.True(t, ok, "error should be *llm.Error")
			assert.Equal(t, llm.ErrInvalidRequest, llmErr.Code)
			assert.Contains(t, llmErr.Message, tt.feature)
			assert.Equal(t, http.StatusNotImplemented, llmErr.HTTPStatus)
			assert.Equal(t, "doubao", llmErr.Provider)
		})
	}
}

// --- Completion via httptest ---

func TestDoubaoProvider_Completion(t *testing.T) {
	var capturedRequest providers.OpenAICompatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v3/chat/completions", r.URL.Path)
		assert.Contains(t, r.Header.Get("Authorization"), "Bearer ")

		json.NewDecoder(r.Body).Decode(&capturedRequest)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(providers.OpenAICompatResponse{
			ID:    "resp-1",
			Model: "Doubao-1.5-pro-32k",
			Choices: []providers.OpenAICompatChoice{
				{
					Index:        0,
					FinishReason: "stop",
					Message: providers.OpenAICompatMessage{
						Role:    "assistant",
						Content: "Hello from Doubao",
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

	cfg := providers.DoubaoConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			APIKey:  "test-key",
			BaseURL: server.URL,
		},
	}
	p := NewDoubaoProvider(cfg, zap.NewNop())

	resp, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "Hi"},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Equal(t, "doubao", resp.Provider)
	assert.Equal(t, "Doubao-1.5-pro-32k", resp.Model)
	require.Len(t, resp.Choices, 1)
	assert.Equal(t, "Hello from Doubao", resp.Choices[0].Message.Content)
	assert.Equal(t, 15, resp.Usage.TotalTokens)

	// Verify fallback model was used (no model in request, no default model in config)
	assert.Equal(t, "Doubao-1.5-pro-32k", capturedRequest.Model)
}

// --- Stream via httptest ---

func TestDoubaoProvider_Stream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v3/chat/completions", r.URL.Path)

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		chunk := providers.OpenAICompatResponse{
			ID:    "stream-1",
			Model: "Doubao-1.5-pro-32k",
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

	cfg := providers.DoubaoConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			APIKey:  "test-key",
			BaseURL: server.URL,
		},
	}
	p := NewDoubaoProvider(cfg, zap.NewNop())

	ch, err := p.Stream(context.Background(), &llm.ChatRequest{
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "Hi"},
		},
	})
	require.NoError(t, err)

	var chunks []llm.StreamChunk
	for c := range ch {
		chunks = append(chunks, c)
	}

	require.Len(t, chunks, 1)
	assert.Equal(t, "Hello", chunks[0].Delta.Content)
	assert.Equal(t, "doubao", chunks[0].Provider)
}
