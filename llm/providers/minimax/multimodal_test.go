package minimax

import (
	"context"
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

func TestMiniMaxProvider_MultimodalNotSupported(t *testing.T) {
	p := NewMiniMaxProvider(providers.MiniMaxConfig{}, zap.NewNop())
	ctx := context.Background()

	tests := []struct {
		name string
		fn   func() error
	}{
		{"GenerateImage", func() error { _, err := p.GenerateImage(ctx, &llm.ImageGenerationRequest{}); return err }},
		{"GenerateVideo", func() error { _, err := p.GenerateVideo(ctx, &llm.VideoGenerationRequest{}); return err }},
		{"TranscribeAudio", func() error { _, err := p.TranscribeAudio(ctx, &llm.AudioTranscriptionRequest{}); return err }},
		{"CreateEmbedding", func() error { _, err := p.CreateEmbedding(ctx, &llm.EmbeddingRequest{}); return err }},
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

func TestMiniMaxProvider_GenerateAudio_Success(t *testing.T) {
	audioData := []byte("fake-audio-data")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/audio/speech", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		w.Write(audioData)
	}))
	t.Cleanup(server.Close)

	p := NewMiniMaxProvider(providers.MiniMaxConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
	}, zap.NewNop())

	resp, err := p.GenerateAudio(context.Background(), &llm.AudioGenerationRequest{
		Model: "tts-1", Input: "hello",
	})
	require.NoError(t, err)
	assert.Equal(t, audioData, resp.Audio)
}
