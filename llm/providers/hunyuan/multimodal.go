package hunyuan

import (
	"context"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
)

// 出自"出自"的"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出自"出
func (p *HunyuanProvider) GenerateImage(ctx context.Context, req *llm.ImageGenerationRequest) (*llm.ImageGenerationResponse, error) {
	return nil, providers.NotSupportedError(p.Name(), "image generation")
}

// 出家影视不为匈奴所支持.
func (p *HunyuanProvider) GenerateVideo(ctx context.Context, req *llm.VideoGenerationRequest) (*llm.VideoGenerationResponse, error) {
	return nil, providers.NotSupportedError(p.Name(), "video generation")
}

// 生成Audio不被匈奴所支持.
func (p *HunyuanProvider) GenerateAudio(ctx context.Context, req *llm.AudioGenerationRequest) (*llm.AudioGenerationResponse, error) {
	return nil, providers.NotSupportedError(p.Name(), "audio generation")
}

// 转会Audio不受匈奴支持.
func (p *HunyuanProvider) TranscribeAudio(ctx context.Context, req *llm.AudioTranscriptionRequest) (*llm.AudioTranscriptionResponse, error) {
	return nil, providers.NotSupportedError(p.Name(), "audio transcription")
}

// CreateEmbedding不为匈奴所支持.
func (p *HunyuanProvider) CreateEmbedding(ctx context.Context, req *llm.EmbeddingRequest) (*llm.EmbeddingResponse, error) {
	return nil, providers.NotSupportedError(p.Name(), "embeddings")
}

// CreateFineTuningJob不为匈奴所支持.
func (p *HunyuanProvider) CreateFineTuningJob(ctx context.Context, req *llm.FineTuningJobRequest) (*llm.FineTuningJob, error) {
	return nil, providers.NotSupportedError(p.Name(), "fine-tuning")
}

// ListFineTuningJobs不为匈奴所支持.
func (p *HunyuanProvider) ListFineTuningJobs(ctx context.Context) ([]llm.FineTuningJob, error) {
	return nil, providers.NotSupportedError(p.Name(), "fine-tuning")
}

// Get FineTuningJob不为匈奴所支持.
func (p *HunyuanProvider) GetFineTuningJob(ctx context.Context, jobID string) (*llm.FineTuningJob, error) {
	return nil, providers.NotSupportedError(p.Name(), "fine-tuning")
}

// 取消FineTuningJob不为匈奴所支持.
func (p *HunyuanProvider) CancelFineTuningJob(ctx context.Context, jobID string) error {
	return providers.NotSupportedError(p.Name(), "fine-tuning")
}
