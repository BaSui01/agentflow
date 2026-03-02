package qwen

import (
	"context"

	providerbase "github.com/BaSui01/agentflow/llm/providers/base"

	"github.com/BaSui01/agentflow/llm"
)

// GenerateImage 使用 Qwen Wanx 生成图像.
// Endpoint: POST /compatible-mode/v1/images/generations
// Models: wanx-v1, wanx2.1-t2i-turbo, wanx2.1-t2i-plus
func (p *QwenProvider) GenerateImage(ctx context.Context, req *llm.ImageGenerationRequest) (*llm.ImageGenerationResponse, error) {
	return providerbase.GenerateImageOpenAICompat(ctx, p.Client, p.Cfg.BaseURL, p.ResolveAPIKey(ctx), p.Name(), "/compatible-mode/v1/images/generations", req, p.ApplyHeaders)
}

// GenerateVideo Qwen 不支持视频生成.
func (p *QwenProvider) GenerateVideo(ctx context.Context, req *llm.VideoGenerationRequest) (*llm.VideoGenerationResponse, error) {
	return nil, providerbase.NotSupportedError(p.Name(), "video generation")
}

// GenerateAudio 使用 Qwen TTS 生成音频.
// Endpoint: POST /compatible-mode/v1/audio/speech
// Models: cosyvoice-v1, sambert-v1, qwen-tts
func (p *QwenProvider) GenerateAudio(ctx context.Context, req *llm.AudioGenerationRequest) (*llm.AudioGenerationResponse, error) {
	return providerbase.GenerateAudioOpenAICompat(ctx, p.Client, p.Cfg.BaseURL, p.ResolveAPIKey(ctx), p.Name(), "/compatible-mode/v1/audio/speech", req, p.ApplyHeaders)
}

// TranscribeAudio Qwen 不支持音频转录.
func (p *QwenProvider) TranscribeAudio(ctx context.Context, req *llm.AudioTranscriptionRequest) (*llm.AudioTranscriptionResponse, error) {
	return nil, providerbase.NotSupportedError(p.Name(), "audio transcription")
}

// CreateEmbedding 使用 Qwen 创建嵌入.
// Endpoint: POST /compatible-mode/v1/embeddings
// Models: text-embedding-v4, text-embedding-v3, text-embedding-v2
func (p *QwenProvider) CreateEmbedding(ctx context.Context, req *llm.EmbeddingRequest) (*llm.EmbeddingResponse, error) {
	return providerbase.CreateEmbeddingOpenAICompat(ctx, p.Client, p.Cfg.BaseURL, p.ResolveAPIKey(ctx), p.Name(), "/compatible-mode/v1/embeddings", req, p.ApplyHeaders)
}

// CreateFineTuningJob Qwen 不支持微调.
func (p *QwenProvider) CreateFineTuningJob(ctx context.Context, req *llm.FineTuningJobRequest) (*llm.FineTuningJob, error) {
	return nil, providerbase.NotSupportedError(p.Name(), "fine-tuning")
}

// ListFineTuningJobs Qwen 不支持微调.
func (p *QwenProvider) ListFineTuningJobs(ctx context.Context) ([]llm.FineTuningJob, error) {
	return nil, providerbase.NotSupportedError(p.Name(), "fine-tuning")
}

// GetFineTuningJob Qwen 不支持微调.
func (p *QwenProvider) GetFineTuningJob(ctx context.Context, jobID string) (*llm.FineTuningJob, error) {
	return nil, providerbase.NotSupportedError(p.Name(), "fine-tuning")
}

// CancelFineTuningJob Qwen 不支持微调.
func (p *QwenProvider) CancelFineTuningJob(ctx context.Context, jobID string) error {
	return providerbase.NotSupportedError(p.Name(), "fine-tuning")
}
