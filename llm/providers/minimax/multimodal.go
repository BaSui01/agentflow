package minimax

import (
	"context"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
)

// GenerateImage MiniMax 不支持图像生成.
func (p *MiniMaxProvider) GenerateImage(ctx context.Context, req *llm.ImageGenerationRequest) (*llm.ImageGenerationResponse, error) {
	return nil, providers.NotSupportedError(p.Name(), "image generation")
}

// GenerateVideo MiniMax 不支持视频生成.
func (p *MiniMaxProvider) GenerateVideo(ctx context.Context, req *llm.VideoGenerationRequest) (*llm.VideoGenerationResponse, error) {
	return nil, providers.NotSupportedError(p.Name(), "video generation")
}

// GenerateAudio 使用 MiniMax 生成音频.
func (p *MiniMaxProvider) GenerateAudio(ctx context.Context, req *llm.AudioGenerationRequest) (*llm.AudioGenerationResponse, error) {
	return providers.GenerateAudioOpenAICompat(ctx, p.client, p.cfg.BaseURL, p.cfg.APIKey, p.Name(), "/v1/audio/speech", req, p.buildHeaders)
}

// TranscribeAudio MiniMax 不支持音频转录.
func (p *MiniMaxProvider) TranscribeAudio(ctx context.Context, req *llm.AudioTranscriptionRequest) (*llm.AudioTranscriptionResponse, error) {
	return nil, providers.NotSupportedError(p.Name(), "audio transcription")
}

// CreateEmbedding MiniMax 不支持嵌入.
func (p *MiniMaxProvider) CreateEmbedding(ctx context.Context, req *llm.EmbeddingRequest) (*llm.EmbeddingResponse, error) {
	return nil, providers.NotSupportedError(p.Name(), "embeddings")
}

// CreateFineTuningJob MiniMax 不支持微调.
func (p *MiniMaxProvider) CreateFineTuningJob(ctx context.Context, req *llm.FineTuningJobRequest) (*llm.FineTuningJob, error) {
	return nil, providers.NotSupportedError(p.Name(), "fine-tuning")
}

// ListFineTuningJobs MiniMax 不支持微调.
func (p *MiniMaxProvider) ListFineTuningJobs(ctx context.Context) ([]llm.FineTuningJob, error) {
	return nil, providers.NotSupportedError(p.Name(), "fine-tuning")
}

// GetFineTuningJob MiniMax 不支持微调.
func (p *MiniMaxProvider) GetFineTuningJob(ctx context.Context, jobID string) (*llm.FineTuningJob, error) {
	return nil, providers.NotSupportedError(p.Name(), "fine-tuning")
}

// CancelFineTuningJob MiniMax 不支持微调.
func (p *MiniMaxProvider) CancelFineTuningJob(ctx context.Context, jobID string) error {
	return providers.NotSupportedError(p.Name(), "fine-tuning")
}
