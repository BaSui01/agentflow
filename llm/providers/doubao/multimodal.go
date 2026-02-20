package doubao

import (
	"context"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
)

// 出家偶像不为杜宝所支持.
func (p *DoubaoProvider) GenerateImage(ctx context.Context, req *llm.ImageGenerationRequest) (*llm.ImageGenerationResponse, error) {
	return nil, providers.NotSupportedError(p.Name(), "image generation")
}

// 生成Video不被多宝所支持.
func (p *DoubaoProvider) GenerateVideo(ctx context.Context, req *llm.VideoGenerationRequest) (*llm.VideoGenerationResponse, error) {
	return nil, providers.NotSupportedError(p.Name(), "video generation")
}

// 生成Audio使用多巴奥生成音频.
func (p *DoubaoProvider) GenerateAudio(ctx context.Context, req *llm.AudioGenerationRequest) (*llm.AudioGenerationResponse, error) {
	return providers.GenerateAudioOpenAICompat(ctx, p.client, p.cfg.BaseURL, p.cfg.APIKey, p.Name(), "/api/v3/audio/speech", req, p.buildHeaders)
}

// TrancisAudio 不支持杜宝.
func (p *DoubaoProvider) TranscribeAudio(ctx context.Context, req *llm.AudioTranscriptionRequest) (*llm.AudioTranscriptionResponse, error) {
	return nil, providers.NotSupportedError(p.Name(), "audio transcription")
}

// CreateEmbedding 使用 Doubao 创建嵌入.
func (p *DoubaoProvider) CreateEmbedding(ctx context.Context, req *llm.EmbeddingRequest) (*llm.EmbeddingResponse, error) {
	return providers.CreateEmbeddingOpenAICompat(ctx, p.client, p.cfg.BaseURL, p.cfg.APIKey, p.Name(), "/api/v3/embeddings", req, p.buildHeaders)
}

// CreateFineTuningJob不受多宝支持.
func (p *DoubaoProvider) CreateFineTuningJob(ctx context.Context, req *llm.FineTuningJobRequest) (*llm.FineTuningJob, error) {
	return nil, providers.NotSupportedError(p.Name(), "fine-tuning")
}

// ListFineTuningJobs 不支持杜宝.
func (p *DoubaoProvider) ListFineTuningJobs(ctx context.Context) ([]llm.FineTuningJob, error) {
	return nil, providers.NotSupportedError(p.Name(), "fine-tuning")
}

// Get FineTuningJob 不支持杜宝.
func (p *DoubaoProvider) GetFineTuningJob(ctx context.Context, jobID string) (*llm.FineTuningJob, error) {
	return nil, providers.NotSupportedError(p.Name(), "fine-tuning")
}

// 取消FineTuningJob不支持杜宝.
func (p *DoubaoProvider) CancelFineTuningJob(ctx context.Context, jobID string) error {
	return providers.NotSupportedError(p.Name(), "fine-tuning")
}
