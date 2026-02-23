package glm

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

func TestGLMProvider_MultimodalNotSupported(t *testing.T) {
	p := NewGLMProvider(providers.GLMConfig{}, zap.NewNop())
	ctx := context.Background()

	tests := []struct {
		name string
		fn   func() error
	}{
		{"GenerateAudio", func() error { _, err := p.GenerateAudio(ctx, &llm.AudioGenerationRequest{}); return err }},
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
			llmErr, ok := err.(*llm.Error)
			require.True(t, ok)
			assert.Equal(t, llm.ErrInvalidRequest, llmErr.Code)
		})
	}
}

func TestGLMProvider_GenerateImage_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/paas/v4/images/generations", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(llm.ImageGenerationResponse{
			Created: 1700000000,
			Data:    []llm.Image{{URL: "https://example.com/img.png"}},
		})
	}))
	t.Cleanup(server.Close)

	p := NewGLMProvider(providers.GLMConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
	}, zap.NewNop())

	resp, err := p.GenerateImage(context.Background(), &llm.ImageGenerationRequest{Prompt: "a cat"})
	require.NoError(t, err)
	require.Len(t, resp.Data, 1)
}
