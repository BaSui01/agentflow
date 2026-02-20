package qwen

import (
	"context"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
)

// GenerateImage 使用 Qwen Wanx 生成图像.
// Endpoint: POST /compatible-mode/v1/images/generations
// Models: wanx-v1, wanx2.1-t2i-turbo, wanx2.1-t2i-plus
func (p *QwenProvider) GenerateImage(ctx context.Context, req *llm.ImageGenerationRequest) (*llm.ImageGenerationResponse, error) {
	return providers.GenerateImageOpenAICompat(ctx, p.client, p.cfg.BaseURL, p.cfg.APIKey, p.Name(), "/compatible-mode/v1/images/generations", req, p.buildHeaders)
}

// GenerateVideo Qwen 不支持视频生成.
func (p *QwenProvider) GenerateVideo(ctx context.Context, req *llm.VideoGenerationRequest) (*llm.VideoGenerationResponse, error) {
	return nil, providers.NotSupportedError(p.Name(), "video generation")
}

// GenerateAudio 使用 Qwen TTS 生成音频.
// Endpoint: POST /compatible-mode/v1/audio/speech
// Models: cosyvoice-v1, sambert-v1, qwen-tts
func (p *QwenProvider) GenerateAudio(ctx context.Context, req *llm.AudioGenerationRequest) (*llm.AudioGenerationResponse, error) {
	return providers.GenerateAudioOpenAICompat(ctx, p.client, p.cfg.BaseURL, p.cfg.APIKey, p.Name(), "/compatible-mode/v1/audio/speech", req, p.buildHeaders)
}

// TranscribeAudio Qwen 不支持音频转录.
func (p *QwenProvider) TranscribeAudio(ctx context.Context, req *llm.AudioTranscriptionRequest) (*llm.AudioTranscriptionResponse, error) {
	return nil, providers.NotSupportedError(p.Name(), "audio transcription")
}

// CreateEmbedding 使用 Qwen 创建嵌入.
// Endpoint: POST /compatible-mode/v1/embeddings
// Models: text-embedding-v4, text-embedding-v3, text-embedding-v2
func (p *QwenProvider) CreateEmbedding(ctx context.Context, req *llm.EmbeddingRequest) (*llm.EmbeddingResponse, error) {
	return providers.CreateEmbeddingOpenAICompat(ctx, p.client, p.cfg.BaseURL, p.cfg.APIKey, p.Name(), "/compatible-mode/v1/embeddings", req, p.buildHeaders)
}

// CreateFineTuningJob Qwen 不支持微调.
func (p *QwenProvider) CreateFineTuningJob(ctx context.Context, req *llm.FineTuningJobRequest) (*llm.FineTuningJob, error) {
	return nil, providers.NotSupportedError(p.Name(), "fine-tuning")
}

// ListFineTuningJobs Qwen 不支持微调.
func (p *QwenProvider) ListFineTuningJobs(ctx context.Context) ([]llm.FineTuningJob, error) {
	return nil, providers.NotSupportedError(p.Name(), "fine-tuning")
}

// GetFineTuningJob Qwen 不支持微调.
func (p *QwenProvider) GetFineTuningJob(ctx context.Context, jobID string) (*llm.FineTuningJob, error) {
	return nil, providers.NotSupportedError(p.Name(), "fine-tuning")
}

// CancelFineTuningJob Qwen 不支持微调.
func (p *QwenProvider) CancelFineTuningJob(ctx context.Context, jobID string) error {
	return providers.NotSupportedError(p.Name(), "fine-tuning")
}
