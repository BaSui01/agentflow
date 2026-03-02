package llama

import (
	"context"

	providerbase "github.com/BaSui01/agentflow/llm/providers/base"

	"github.com/BaSui01/agentflow/llm"
)

// GenerateImage Llama 不支持图像生成.
func (p *LlamaProvider) GenerateImage(ctx context.Context, req *llm.ImageGenerationRequest) (*llm.ImageGenerationResponse, error) {
	return nil, providerbase.NotSupportedError(p.Name(), "image generation")
}

// GenerateVideo Llama 不支持视频生成.
func (p *LlamaProvider) GenerateVideo(ctx context.Context, req *llm.VideoGenerationRequest) (*llm.VideoGenerationResponse, error) {
	return nil, providerbase.NotSupportedError(p.Name(), "video generation")
}

// GenerateAudio Llama 不支持音频生成.
func (p *LlamaProvider) GenerateAudio(ctx context.Context, req *llm.AudioGenerationRequest) (*llm.AudioGenerationResponse, error) {
	return nil, providerbase.NotSupportedError(p.Name(), "audio generation")
}

// TranscribeAudio Llama 不支持音频转录.
func (p *LlamaProvider) TranscribeAudio(ctx context.Context, req *llm.AudioTranscriptionRequest) (*llm.AudioTranscriptionResponse, error) {
	return nil, providerbase.NotSupportedError(p.Name(), "audio transcription")
}

// CreateEmbedding Llama 不支持嵌入.
func (p *LlamaProvider) CreateEmbedding(ctx context.Context, req *llm.EmbeddingRequest) (*llm.EmbeddingResponse, error) {
	return nil, providerbase.NotSupportedError(p.Name(), "embeddings")
}

// CreateFineTuningJob Llama 不支持微调.
func (p *LlamaProvider) CreateFineTuningJob(ctx context.Context, req *llm.FineTuningJobRequest) (*llm.FineTuningJob, error) {
	return nil, providerbase.NotSupportedError(p.Name(), "fine-tuning")
}

// ListFineTuningJobs Llama 不支持微调.
func (p *LlamaProvider) ListFineTuningJobs(ctx context.Context) ([]llm.FineTuningJob, error) {
	return nil, providerbase.NotSupportedError(p.Name(), "fine-tuning")
}

// GetFineTuningJob Llama 不支持微调.
func (p *LlamaProvider) GetFineTuningJob(ctx context.Context, jobID string) (*llm.FineTuningJob, error) {
	return nil, providerbase.NotSupportedError(p.Name(), "fine-tuning")
}

// CancelFineTuningJob Llama 不支持微调.
func (p *LlamaProvider) CancelFineTuningJob(ctx context.Context, jobID string) error {
	return providerbase.NotSupportedError(p.Name(), "fine-tuning")
}
