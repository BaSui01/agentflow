package doubao

import (
	"context"

	providerbase "github.com/BaSui01/agentflow/llm/providers/base"

	"github.com/BaSui01/agentflow/llm"
)

// GenerateImage 使用 Doubao 生成图像.
func (p *DoubaoProvider) GenerateImage(ctx context.Context, req *llm.ImageGenerationRequest) (*llm.ImageGenerationResponse, error) {
	return providerbase.GenerateImageOpenAICompat(ctx, p.Client, p.Cfg.BaseURL, p.ResolveAPIKey(ctx), p.Name(), "/api/v3/images/generations", req, p.ApplyHeaders)
}

// GenerateVideo Doubao 不支持视频生成.
func (p *DoubaoProvider) GenerateVideo(ctx context.Context, req *llm.VideoGenerationRequest) (*llm.VideoGenerationResponse, error) {
	return nil, providerbase.NotSupportedError(p.Name(), "video generation")
}

// GenerateAudio 使用 Doubao 生成音频.
func (p *DoubaoProvider) GenerateAudio(ctx context.Context, req *llm.AudioGenerationRequest) (*llm.AudioGenerationResponse, error) {
	return providerbase.GenerateAudioOpenAICompat(ctx, p.Client, p.Cfg.BaseURL, p.ResolveAPIKey(ctx), p.Name(), "/api/v3/audio/speech", req, p.ApplyHeaders)
}

// TranscribeAudio Doubao 不支持音频转录.
func (p *DoubaoProvider) TranscribeAudio(ctx context.Context, req *llm.AudioTranscriptionRequest) (*llm.AudioTranscriptionResponse, error) {
	return nil, providerbase.NotSupportedError(p.Name(), "audio transcription")
}

// CreateEmbedding 使用 Doubao 创建嵌入.
func (p *DoubaoProvider) CreateEmbedding(ctx context.Context, req *llm.EmbeddingRequest) (*llm.EmbeddingResponse, error) {
	return providerbase.CreateEmbeddingOpenAICompat(ctx, p.Client, p.Cfg.BaseURL, p.ResolveAPIKey(ctx), p.Name(), "/api/v3/embeddings", req, p.ApplyHeaders)
}

// CreateFineTuningJob Doubao 不支持微调.
func (p *DoubaoProvider) CreateFineTuningJob(ctx context.Context, req *llm.FineTuningJobRequest) (*llm.FineTuningJob, error) {
	return nil, providerbase.NotSupportedError(p.Name(), "fine-tuning")
}

// ListFineTuningJobs Doubao 不支持微调.
func (p *DoubaoProvider) ListFineTuningJobs(ctx context.Context) ([]llm.FineTuningJob, error) {
	return nil, providerbase.NotSupportedError(p.Name(), "fine-tuning")
}

// GetFineTuningJob Doubao 不支持微调.
func (p *DoubaoProvider) GetFineTuningJob(ctx context.Context, jobID string) (*llm.FineTuningJob, error) {
	return nil, providerbase.NotSupportedError(p.Name(), "fine-tuning")
}

// CancelFineTuningJob Doubao 不支持微调.
func (p *DoubaoProvider) CancelFineTuningJob(ctx context.Context, jobID string) error {
	return providerbase.NotSupportedError(p.Name(), "fine-tuning")
}
