package claude

import (
	"github.com/BaSui01/agentflow/types"
	"context"
	"testing"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestClaudeProvider_MultimodalNotSupported(t *testing.T) {
	p := NewClaudeProvider(providers.ClaudeConfig{}, zap.NewNop())
	ctx := context.Background()

	tests := []struct {
		name string
		fn   func() error
	}{
		{"GenerateImage", func() error { _, err := p.GenerateImage(ctx, &llm.ImageGenerationRequest{}); return err }},
		{"GenerateVideo", func() error { _, err := p.GenerateVideo(ctx, &llm.VideoGenerationRequest{}); return err }},
		{"GenerateAudio", func() error { _, err := p.GenerateAudio(ctx, &llm.AudioGenerationRequest{}); return err }},
		{"TranscribeAudio", func() error { _, err := p.TranscribeAudio(ctx, &llm.AudioTranscriptionRequest{}); return err }},
		{"CreateEmbedding", func() error { _, err := p.CreateEmbedding(ctx, &llm.EmbeddingRequest{}); return err }},
		{"CreateFineTuningJob", func() error { _, err := p.CreateFineTuningJob(ctx, &llm.FineTuningJobRequest{}); return err }},
		{"ListFineTuningJobs", func() error { _, err := p.ListFineTuningJobs(ctx); return err }},
		{"GetFineTuningJob", func() error { _, err := p.GetFineTuningJob(ctx, "job-1"); return err }},
		{"CancelFineTuningJob", func() error { return p.CancelFineTuningJob(ctx, "job-1") }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fn()
			require.Error(t, err)
			llmErr, ok := err.(*types.Error)
			require.True(t, ok)
			assert.Equal(t, llm.ErrInvalidRequest, llmErr.Code)
			assert.Contains(t, llmErr.Message, "not supported")
		})
	}
}


