package grok

import (
	"context"

	providerbase "github.com/BaSui01/agentflow/llm/providers/base"

	"github.com/BaSui01/agentflow/llm"
)

// GenerateImage generates images using xAI Grok Aurora.
// Endpoint: POST /v1/images/generations
// Models: grok-2-image, grok-2-image-latest
func (p *GrokProvider) GenerateImage(ctx context.Context, req *llm.ImageGenerationRequest) (*llm.ImageGenerationResponse, error) {
	return providerbase.GenerateImageOpenAICompat(ctx, p.Client, p.Cfg.BaseURL, p.ResolveAPIKey(ctx), p.Name(), "/v1/images/generations", req, p.ApplyHeaders)
}

// GenerateVideo Grok 不支持视频生成.
func (p *GrokProvider) GenerateVideo(ctx context.Context, req *llm.VideoGenerationRequest) (*llm.VideoGenerationResponse, error) {
	return nil, providerbase.NotSupportedError(p.Name(), "video generation")
}

// GenerateAudio Grok 不支持音频生成.
func (p *GrokProvider) GenerateAudio(ctx context.Context, req *llm.AudioGenerationRequest) (*llm.AudioGenerationResponse, error) {
	return nil, providerbase.NotSupportedError(p.Name(), "audio generation")
}

// TranscribeAudio Grok 不支持音频转录.
func (p *GrokProvider) TranscribeAudio(ctx context.Context, req *llm.AudioTranscriptionRequest) (*llm.AudioTranscriptionResponse, error) {
	return nil, providerbase.NotSupportedError(p.Name(), "audio transcription")
}

// CreateEmbedding creates embeddings using xAI Grok.
// Endpoint: POST /v1/embeddings
// Models: grok-embedding-beta
func (p *GrokProvider) CreateEmbedding(ctx context.Context, req *llm.EmbeddingRequest) (*llm.EmbeddingResponse, error) {
	return providerbase.CreateEmbeddingOpenAICompat(ctx, p.Client, p.Cfg.BaseURL, p.ResolveAPIKey(ctx), p.Name(), "/v1/embeddings", req, p.ApplyHeaders)
}

// CreateFineTuningJob Grok 不支持微调.
func (p *GrokProvider) CreateFineTuningJob(ctx context.Context, req *llm.FineTuningJobRequest) (*llm.FineTuningJob, error) {
	return nil, providerbase.NotSupportedError(p.Name(), "fine-tuning")
}

// ListFineTuningJobs Grok 不支持微调.
func (p *GrokProvider) ListFineTuningJobs(ctx context.Context) ([]llm.FineTuningJob, error) {
	return nil, providerbase.NotSupportedError(p.Name(), "fine-tuning")
}

// GetFineTuningJob Grok 不支持微调.
func (p *GrokProvider) GetFineTuningJob(ctx context.Context, jobID string) (*llm.FineTuningJob, error) {
	return nil, providerbase.NotSupportedError(p.Name(), "fine-tuning")
}

// CancelFineTuningJob Grok 不支持微调.
func (p *GrokProvider) CancelFineTuningJob(ctx context.Context, jobID string) error {
	return providerbase.NotSupportedError(p.Name(), "fine-tuning")
}
