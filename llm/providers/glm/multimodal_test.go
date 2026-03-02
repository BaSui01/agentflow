package glm

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

func TestGLMProvider_MultimodalNotSupported(t *testing.T) {
	p := NewGLMProvider(providers.GLMConfig{}, zap.NewNop())
	ctx := context.Background()

	tests := []struct {
		name string
		fn   func() error
	}{
		{"TranscribeAudio", func() error { _, err := p.TranscribeAudio(ctx, &llm.AudioTranscriptionRequest{}); return err }},
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

func TestGLMProvider_GenerateImage_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/paas/v4/images/generations", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(llm.ImageGenerationResponse{
			Created: 1700000000,
			Data:    []llm.Image{{URL: "https://example.com/img.png"}},
		})
		require.NoError(t, err)
	}))
	t.Cleanup(server.Close)

	p := NewGLMProvider(providers.GLMConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
	}, zap.NewNop())

	resp, err := p.GenerateImage(context.Background(), &llm.ImageGenerationRequest{Prompt: "a cat"})
	require.NoError(t, err)
	require.Len(t, resp.Data, 1)
}

func TestGLMProvider_GenerateAudio_Success(t *testing.T) {
	audioData := []byte("fake-audio")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/paas/v4/audio/speech", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, err := w.Write(audioData)
		require.NoError(t, err)
	}))
	t.Cleanup(server.Close)

	p := NewGLMProvider(providers.GLMConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
	}, zap.NewNop())

	resp, err := p.GenerateAudio(context.Background(), &llm.AudioGenerationRequest{Input: "hello"})
	require.NoError(t, err)
	require.Equal(t, audioData, resp.Audio)
}

func TestGLMProvider_CreateFineTuningJob_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/paas/v4/fine_tuning/jobs", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(llm.FineTuningJob{
			ID:     "ftjob-123",
			Model:  "glm-4",
			Status: "queued",
		})
		require.NoError(t, err)
	}))
	t.Cleanup(server.Close)

	p := NewGLMProvider(providers.GLMConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
	}, zap.NewNop())

	job, err := p.CreateFineTuningJob(context.Background(), &llm.FineTuningJobRequest{
		Model:        "glm-4",
		TrainingFile: "file-1",
	})
	require.NoError(t, err)
	require.Equal(t, "ftjob-123", job.ID)
}

func TestGLMProvider_ListFineTuningJobs_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/paas/v4/fine_tuning/jobs", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(struct {
			Data []llm.FineTuningJob `json:"data"`
		}{
			Data: []llm.FineTuningJob{{ID: "ftjob-1"}, {ID: "ftjob-2"}},
		})
		require.NoError(t, err)
	}))
	t.Cleanup(server.Close)

	p := NewGLMProvider(providers.GLMConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
	}, zap.NewNop())

	jobs, err := p.ListFineTuningJobs(context.Background())
	require.NoError(t, err)
	require.Len(t, jobs, 2)
}

func TestGLMProvider_GetFineTuningJob_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/paas/v4/fine_tuning/jobs/ftjob-123", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(llm.FineTuningJob{
			ID:     "ftjob-123",
			Model:  "glm-4",
			Status: "running",
		})
		require.NoError(t, err)
	}))
	t.Cleanup(server.Close)

	p := NewGLMProvider(providers.GLMConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
	}, zap.NewNop())

	job, err := p.GetFineTuningJob(context.Background(), "ftjob-123")
	require.NoError(t, err)
	require.Equal(t, "ftjob-123", job.ID)
}

func TestGLMProvider_CancelFineTuningJob_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/paas/v4/fine_tuning/jobs/ftjob-123/cancel", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte(`{}`))
		require.NoError(t, err)
	}))
	t.Cleanup(server.Close)

	p := NewGLMProvider(providers.GLMConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
	}, zap.NewNop())

	err := p.CancelFineTuningJob(context.Background(), "ftjob-123")
	require.NoError(t, err)
}
