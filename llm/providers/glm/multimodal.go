package glm

import (
	"context"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
)

// GenerateImage 使用 GLM CogView 生成图像.
func (p *GLMProvider) GenerateImage(ctx context.Context, req *llm.ImageGenerationRequest) (*llm.ImageGenerationResponse, error) {
	return providers.GenerateImageOpenAICompat(ctx, p.Client, p.Cfg.BaseURL, p.Cfg.APIKey, p.Name(), "/api/paas/v4/images/generations", req, providers.BearerTokenHeaders)
}

// GenerateVideo 使用 GLM CogVideo 生成视频.
func (p *GLMProvider) GenerateVideo(ctx context.Context, req *llm.VideoGenerationRequest) (*llm.VideoGenerationResponse, error) {
	return providers.GenerateVideoOpenAICompat(ctx, p.Client, p.Cfg.BaseURL, p.Cfg.APIKey, p.Name(), "/api/paas/v4/videos/generations", req, providers.BearerTokenHeaders)
}

// GenerateAudio GLM 不支持音频生成.
func (p *GLMProvider) GenerateAudio(ctx context.Context, req *llm.AudioGenerationRequest) (*llm.AudioGenerationResponse, error) {
	return nil, providers.NotSupportedError(p.Name(), "audio generation")
}

// TranscribeAudio GLM 不支持音频转录.
func (p *GLMProvider) TranscribeAudio(ctx context.Context, req *llm.AudioTranscriptionRequest) (*llm.AudioTranscriptionResponse, error) {
	return nil, providers.NotSupportedError(p.Name(), "audio transcription")
}

// CreateEmbedding 使用 GLM 创建嵌入.
func (p *GLMProvider) CreateEmbedding(ctx context.Context, req *llm.EmbeddingRequest) (*llm.EmbeddingResponse, error) {
	return providers.CreateEmbeddingOpenAICompat(ctx, p.Client, p.Cfg.BaseURL, p.Cfg.APIKey, p.Name(), "/api/paas/v4/embeddings", req, providers.BearerTokenHeaders)
}

// CreateFineTuningJob GLM 不支持微调.
func (p *GLMProvider) CreateFineTuningJob(ctx context.Context, req *llm.FineTuningJobRequest) (*llm.FineTuningJob, error) {
	return nil, providers.NotSupportedError(p.Name(), "fine-tuning")
}

// ListFineTuningJobs GLM 不支持微调.
func (p *GLMProvider) ListFineTuningJobs(ctx context.Context) ([]llm.FineTuningJob, error) {
	return nil, providers.NotSupportedError(p.Name(), "fine-tuning")
}

// GetFineTuningJob GLM 不支持微调.
func (p *GLMProvider) GetFineTuningJob(ctx context.Context, jobID string) (*llm.FineTuningJob, error) {
	return nil, providers.NotSupportedError(p.Name(), "fine-tuning")
}

// CancelFineTuningJob GLM 不支持微调.
func (p *GLMProvider) CancelFineTuningJob(ctx context.Context, jobID string) error {
	return providers.NotSupportedError(p.Name(), "fine-tuning")
}
