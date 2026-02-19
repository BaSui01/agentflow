package hunyuan

import (
	"context"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
)

// GenerateImage is not supported by Hunyuan.
func (p *HunyuanProvider) GenerateImage(ctx context.Context, req *llm.ImageGenerationRequest) (*llm.ImageGenerationResponse, error) {
	return nil, providers.NotSupportedError(p.Name(), "image generation")
}

// GenerateVideo is not supported by Hunyuan.
func (p *HunyuanProvider) GenerateVideo(ctx context.Context, req *llm.VideoGenerationRequest) (*llm.VideoGenerationResponse, error) {
	return nil, providers.NotSupportedError(p.Name(), "video generation")
}

// GenerateAudio is not supported by Hunyuan.
func (p *HunyuanProvider) GenerateAudio(ctx context.Context, req *llm.AudioGenerationRequest) (*llm.AudioGenerationResponse, error) {
	return nil, providers.NotSupportedError(p.Name(), "audio generation")
}

// TranscribeAudio is not supported by Hunyuan.
func (p *HunyuanProvider) TranscribeAudio(ctx context.Context, req *llm.AudioTranscriptionRequest) (*llm.AudioTranscriptionResponse, error) {
	return nil, providers.NotSupportedError(p.Name(), "audio transcription")
}

// CreateEmbedding is not supported by Hunyuan.
func (p *HunyuanProvider) CreateEmbedding(ctx context.Context, req *llm.EmbeddingRequest) (*llm.EmbeddingResponse, error) {
	return nil, providers.NotSupportedError(p.Name(), "embeddings")
}

// CreateFineTuningJob is not supported by Hunyuan.
func (p *HunyuanProvider) CreateFineTuningJob(ctx context.Context, req *llm.FineTuningJobRequest) (*llm.FineTuningJob, error) {
	return nil, providers.NotSupportedError(p.Name(), "fine-tuning")
}

// ListFineTuningJobs is not supported by Hunyuan.
func (p *HunyuanProvider) ListFineTuningJobs(ctx context.Context) ([]llm.FineTuningJob, error) {
	return nil, providers.NotSupportedError(p.Name(), "fine-tuning")
}

// GetFineTuningJob is not supported by Hunyuan.
func (p *HunyuanProvider) GetFineTuningJob(ctx context.Context, jobID string) (*llm.FineTuningJob, error) {
	return nil, providers.NotSupportedError(p.Name(), "fine-tuning")
}

// CancelFineTuningJob is not supported by Hunyuan.
func (p *HunyuanProvider) CancelFineTuningJob(ctx context.Context, jobID string) error {
	return providers.NotSupportedError(p.Name(), "fine-tuning")
}
