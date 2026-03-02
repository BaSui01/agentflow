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
	// All previously not-supported capabilities are now implemented.
	// This test is kept as a placeholder for future not-supported checks.
}

func TestGeminiProvider_TranscribeAudio_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "generateContent")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"candidates": []map[string]any{
				{"content": map[string]any{"parts": []map[string]any{{"text": "Hello world"}}}},
			},
		})
	}))
	t.Cleanup(server.Close)

	p := NewGeminiProvider(providers.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
	}, zap.NewNop())

	resp, err := p.TranscribeAudio(context.Background(), &llm.AudioTranscriptionRequest{
		File: []byte("fake-audio"), Language: "en",
	})
	require.NoError(t, err)
	assert.Equal(t, "Hello world", resp.Text)
}

func TestGeminiProvider_TranscribeAudio_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"message":"Bad request"}}`))
	}))
	t.Cleanup(server.Close)

	p := NewGeminiProvider(providers.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
	}, zap.NewNop())

	_, err := p.TranscribeAudio(context.Background(), &llm.AudioTranscriptionRequest{File: []byte("audio")})
	require.Error(t, err)
}

func TestGeminiProvider_CreateFineTuningJob_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1beta/tunedModels", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"name": "tunedModels/test-123",
			"metadata": map[string]any{"totalSteps": 100},
		})
	}))
	t.Cleanup(server.Close)

	p := NewGeminiProvider(providers.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
	}, zap.NewNop())

	job, err := p.CreateFineTuningJob(context.Background(), &llm.FineTuningJobRequest{Model: "gemini-2.5-flash"})
	require.NoError(t, err)
	assert.Equal(t, "tunedModels/test-123", job.ID)
}

func TestGeminiProvider_CreateFineTuningJob_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error":{"message":"Forbidden"}}`))
	}))
	t.Cleanup(server.Close)

	p := NewGeminiProvider(providers.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
	}, zap.NewNop())

	_, err := p.CreateFineTuningJob(context.Background(), &llm.FineTuningJobRequest{})
	require.Error(t, err)
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
