package gemini

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BaSui01/agentflow/types"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestGeminiProvider_MultimodalNotSupported(t *testing.T) {
	p := NewGeminiProvider(providers.GeminiConfig{}, zap.NewNop())
	ctx := context.Background()

	t.Run("TranscribeAudio", func(t *testing.T) {
		_, err := p.TranscribeAudio(ctx, &llm.AudioTranscriptionRequest{})
		require.Error(t, err)
		llmErr, ok := err.(*types.Error)
		require.True(t, ok)
		assert.Equal(t, llm.ErrInvalidRequest, llmErr.Code)
		assert.Contains(t, llmErr.Message, "not supported")
	})

	t.Run("CreateFineTuningJob", func(t *testing.T) {
		_, err := p.CreateFineTuningJob(ctx, &llm.FineTuningJobRequest{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not supported")
	})

	t.Run("ListFineTuningJobs", func(t *testing.T) {
		_, err := p.ListFineTuningJobs(ctx)
		require.Error(t, err)
	})

	t.Run("GetFineTuningJob", func(t *testing.T) {
		_, err := p.GetFineTuningJob(ctx, "job-123")
		require.Error(t, err)
	})

	t.Run("CancelFineTuningJob", func(t *testing.T) {
		err := p.CancelFineTuningJob(ctx, "job-123")
		require.Error(t, err)
	})
}

func TestGeminiProvider_GenerateImage_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/v1beta/models/imagen-4.0-generate-001:predict")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(llm.ImageGenerationResponse{
			Created: 1700000000,
			Data:    []llm.Image{{URL: "https://example.com/img.png"}},
		})
	}))
	t.Cleanup(server.Close)

	p := NewGeminiProvider(providers.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
	}, zap.NewNop())

	resp, err := p.GenerateImage(context.Background(), &llm.ImageGenerationRequest{Prompt: "a cat"})
	require.NoError(t, err)
	require.Len(t, resp.Data, 1)
}

func TestGeminiProvider_GenerateVideo_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "predictLongRunning")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(llm.VideoGenerationResponse{ID: "vid-1"})
	}))
	t.Cleanup(server.Close)

	p := NewGeminiProvider(providers.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
	}, zap.NewNop())

	resp, err := p.GenerateVideo(context.Background(), &llm.VideoGenerationRequest{Prompt: "a sunset"})
	require.NoError(t, err)
	assert.Equal(t, "vid-1", resp.ID)
}

func TestGeminiProvider_GenerateAudio_Success(t *testing.T) {
	audioData := []byte("fake-audio")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "generateContent")
		w.WriteHeader(http.StatusOK)
		w.Write(audioData)
	}))
	t.Cleanup(server.Close)

	p := NewGeminiProvider(providers.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
	}, zap.NewNop())

	resp, err := p.GenerateAudio(context.Background(), &llm.AudioGenerationRequest{Input: "hello"})
	require.NoError(t, err)
	assert.Equal(t, audioData, resp.Audio)
}

func TestGeminiProvider_CreateEmbedding_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "embedContent")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(llm.EmbeddingResponse{
			Object: "list",
			Data:   []llm.Embedding{{Embedding: []float64{0.1, 0.2}}},
		})
	}))
	t.Cleanup(server.Close)

	p := NewGeminiProvider(providers.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
	}, zap.NewNop())

	resp, err := p.CreateEmbedding(context.Background(), &llm.EmbeddingRequest{Input: []string{"hello"}})
	require.NoError(t, err)
	require.Len(t, resp.Data, 1)
}
