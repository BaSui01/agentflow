package hunyuan

import (
	"context"

	providerbase "github.com/BaSui01/agentflow/llm/providers/base"

	"github.com/BaSui01/agentflow/llm"
)

// GenerateImage Hunyuan 不支持图像生成.
func (p *HunyuanProvider) GenerateImage(ctx context.Context, req *llm.ImageGenerationRequest) (*llm.ImageGenerationResponse, error) {
	return nil, providerbase.NotSupportedError(p.Name(), "image generation")
}

// GenerateVideo Hunyuan 不支持视频生成.
func (p *HunyuanProvider) GenerateVideo(ctx context.Context, req *llm.VideoGenerationRequest) (*llm.VideoGenerationResponse, error) {
	return nil, providerbase.NotSupportedError(p.Name(), "video generation")
}

// GenerateAudio Hunyuan 不支持音频生成.
func (p *HunyuanProvider) GenerateAudio(ctx context.Context, req *llm.AudioGenerationRequest) (*llm.AudioGenerationResponse, error) {
	return nil, providerbase.NotSupportedError(p.Name(), "audio generation")
}

// TranscribeAudio Hunyuan 不支持音频转录.
func (p *HunyuanProvider) TranscribeAudio(ctx context.Context, req *llm.AudioTranscriptionRequest) (*llm.AudioTranscriptionResponse, error) {
	return nil, providerbase.NotSupportedError(p.Name(), "audio transcription")
}

// CreateEmbedding Hunyuan 不支持嵌入.
func (p *HunyuanProvider) CreateEmbedding(ctx context.Context, req *llm.EmbeddingRequest) (*llm.EmbeddingResponse, error) {
	return nil, providerbase.NotSupportedError(p.Name(), "embeddings")
}

// CreateFineTuningJob Hunyuan 不支持微调.
func (p *HunyuanProvider) CreateFineTuningJob(ctx context.Context, req *llm.FineTuningJobRequest) (*llm.FineTuningJob, error) {
	return nil, providerbase.NotSupportedError(p.Name(), "fine-tuning")
}

// ListFineTuningJobs Hunyuan 不支持微调.
func (p *HunyuanProvider) ListFineTuningJobs(ctx context.Context) ([]llm.FineTuningJob, error) {
	return nil, providerbase.NotSupportedError(p.Name(), "fine-tuning")
}

// GetFineTuningJob Hunyuan 不支持微调.
func (p *HunyuanProvider) GetFineTuningJob(ctx context.Context, jobID string) (*llm.FineTuningJob, error) {
	return nil, providerbase.NotSupportedError(p.Name(), "fine-tuning")
}

// CancelFineTuningJob Hunyuan 不支持微调.
func (p *HunyuanProvider) CancelFineTuningJob(ctx context.Context, jobID string) error {
	return providerbase.NotSupportedError(p.Name(), "fine-tuning")
}
