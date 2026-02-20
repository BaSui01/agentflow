package deepseek

import (
	"context"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
)

// GenerateImage DeepSeek 不支持图像生成.
func (p *DeepSeekProvider) GenerateImage(ctx context.Context, req *llm.ImageGenerationRequest) (*llm.ImageGenerationResponse, error) {
	return nil, providers.NotSupportedError(p.Name(), "image generation")
}

// GenerateVideo DeepSeek 不支持视频生成.
func (p *DeepSeekProvider) GenerateVideo(ctx context.Context, req *llm.VideoGenerationRequest) (*llm.VideoGenerationResponse, error) {
	return nil, providers.NotSupportedError(p.Name(), "video generation")
}

// GenerateAudio DeepSeek 不支持音频生成.
func (p *DeepSeekProvider) GenerateAudio(ctx context.Context, req *llm.AudioGenerationRequest) (*llm.AudioGenerationResponse, error) {
	return nil, providers.NotSupportedError(p.Name(), "audio generation")
}

// TranscribeAudio DeepSeek 不支持音频转录.
func (p *DeepSeekProvider) TranscribeAudio(ctx context.Context, req *llm.AudioTranscriptionRequest) (*llm.AudioTranscriptionResponse, error) {
	return nil, providers.NotSupportedError(p.Name(), "audio transcription")
}

// CreateEmbedding DeepSeek 不支持嵌入.
func (p *DeepSeekProvider) CreateEmbedding(ctx context.Context, req *llm.EmbeddingRequest) (*llm.EmbeddingResponse, error) {
	return nil, providers.NotSupportedError(p.Name(), "embeddings")
}

// CreateFineTuningJob DeepSeek 不支持微调.
func (p *DeepSeekProvider) CreateFineTuningJob(ctx context.Context, req *llm.FineTuningJobRequest) (*llm.FineTuningJob, error) {
	return nil, providers.NotSupportedError(p.Name(), "fine-tuning")
}

// ListFineTuningJobs DeepSeek 不支持微调.
func (p *DeepSeekProvider) ListFineTuningJobs(ctx context.Context) ([]llm.FineTuningJob, error) {
	return nil, providers.NotSupportedError(p.Name(), "fine-tuning")
}

// GetFineTuningJob DeepSeek 不支持微调.
func (p *DeepSeekProvider) GetFineTuningJob(ctx context.Context, jobID string) (*llm.FineTuningJob, error) {
	return nil, providers.NotSupportedError(p.Name(), "fine-tuning")
}

// CancelFineTuningJob DeepSeek 不支持微调.
func (p *DeepSeekProvider) CancelFineTuningJob(ctx context.Context, jobID string) error {
	return providers.NotSupportedError(p.Name(), "fine-tuning")
}
