package qwen

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

func TestQwenProvider_MultimodalNotSupported(t *testing.T) {
	p := NewQwenProvider(providers.QwenConfig{}, zap.NewNop())
	ctx := context.Background()

	tests := []struct {
		name string
		fn   func() error
	}{
		{"TranscribeAudio", func() error { _, err := p.TranscribeAudio(ctx, &llm.AudioTranscriptionRequest{}); return err }},
		{"CreateFineTuningJob", func() error { _, err := p.CreateFineTuningJob(ctx, &llm.FineTuningJobRequest{}); return err }},
		{"ListFineTuningJobs", func() error { _, err := p.ListFineTuningJobs(ctx); return err }},
		{"GetFineTuningJob", func() error { _, err := p.GetFineTuningJob(ctx, "j1"); return err }},
		{"CancelFineTuningJob", func() error { return p.CancelFineTuningJob(ctx, "j1") }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fn()
			require.Error(t, err)
			llmErr, ok := err.(*types.Error)
			require.True(t, ok)
			assert.Equal(t, llm.ErrInvalidRequest, llmErr.Code)
		})
	}
}

func TestQwenProvider_GenerateImage_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/compatible-mode/v1/images/generations", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(llm.ImageGenerationResponse{
			Created: 1700000000,
			Data:    []llm.Image{{URL: "https://example.com/img.png"}},
		})
		require.NoError(t, err)
	}))
	t.Cleanup(server.Close)

	p := NewQwenProvider(providers.QwenConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
	}, zap.NewNop())

	resp, err := p.GenerateImage(context.Background(), &llm.ImageGenerationRequest{Prompt: "a cat"})
	require.NoError(t, err)
	require.Len(t, resp.Data, 1)
}

func TestQwenProvider_GenerateAudio_Success(t *testing.T) {
	audioData := []byte("fake-audio")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/compatible-mode/v1/audio/speech", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, err := w.Write(audioData)
		require.NoError(t, err)
	}))
	t.Cleanup(server.Close)

	p := NewQwenProvider(providers.QwenConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
	}, zap.NewNop())

	resp, err := p.GenerateAudio(context.Background(), &llm.AudioGenerationRequest{Input: "hello"})
	require.NoError(t, err)
	assert.Equal(t, audioData, resp.Audio)
}

func TestQwenProvider_CreateEmbedding_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/compatible-mode/v1/embeddings", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(llm.EmbeddingResponse{
			Object: "list",
			Data:   []llm.Embedding{{Embedding: []float64{0.1, 0.2}}},
		})
		require.NoError(t, err)
	}))
	t.Cleanup(server.Close)

	p := NewQwenProvider(providers.QwenConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
	}, zap.NewNop())

	resp, err := p.CreateEmbedding(context.Background(), &llm.EmbeddingRequest{Input: []string{"hello"}})
	require.NoError(t, err)
	require.Len(t, resp.Data, 1)
}
