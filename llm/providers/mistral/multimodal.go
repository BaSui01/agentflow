package mistral

import (
	"context"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
)

// GenerateImage Mistral 不支持图像生成.
func (p *MistralProvider) GenerateImage(ctx context.Context, req *llm.ImageGenerationRequest) (*llm.ImageGenerationResponse, error) {
	return nil, providers.NotSupportedError(p.Name(), "image generation")
}

// GenerateVideo Mistral 不支持视频生成.
func (p *MistralProvider) GenerateVideo(ctx context.Context, req *llm.VideoGenerationRequest) (*llm.VideoGenerationResponse, error) {
	return nil, providers.NotSupportedError(p.Name(), "video generation")
}

// GenerateAudio Mistral 不支持音频生成.
func (p *MistralProvider) GenerateAudio(ctx context.Context, req *llm.AudioGenerationRequest) (*llm.AudioGenerationResponse, error) {
	return nil, providers.NotSupportedError(p.Name(), "audio generation")
}

// TranscribeAudio 使用 Voxtral 进行音频转录.
// Endpoint: POST /v1/audio/transcriptions
// Models: voxtral-mini-transcribe-2-2602
func (p *MistralProvider) TranscribeAudio(ctx context.Context, req *llm.AudioTranscriptionRequest) (*llm.AudioTranscriptionResponse, error) {
	// 如果未指定模型，使用 Voxtral 默认模型
	if req.Model == "" {
		req.Model = "voxtral-mini-transcribe-2-2602"
	}
	resp, err := p.OpenAIProvider.TranscribeAudio(ctx, req)
	if err != nil {
		if llmErr, ok := err.(*llm.Error); ok {
			llmErr.Provider = p.Name()
			return nil, llmErr
		}
		return nil, err
	}
	return resp, nil
}

// CreateEmbedding 使用 Mistral 创建嵌入（委托给 OpenAI 兼容实现）.
// Endpoint: POST /v1/embeddings
// Models: mistral-embed, codestral-embed-2505
func (p *MistralProvider) CreateEmbedding(ctx context.Context, req *llm.EmbeddingRequest) (*llm.EmbeddingResponse, error) {
	resp, err := p.OpenAIProvider.CreateEmbedding(ctx, req)
	if err != nil {
		if llmErr, ok := err.(*llm.Error); ok {
			llmErr.Provider = p.Name()
			return nil, llmErr
		}
		return nil, err
	}
	return resp, nil
}

// CreateFineTuningJob Mistral 不支持微调.
func (p *MistralProvider) CreateFineTuningJob(ctx context.Context, req *llm.FineTuningJobRequest) (*llm.FineTuningJob, error) {
	return nil, providers.NotSupportedError(p.Name(), "fine-tuning")
}

// ListFineTuningJobs Mistral 不支持微调.
func (p *MistralProvider) ListFineTuningJobs(ctx context.Context) ([]llm.FineTuningJob, error) {
	return nil, providers.NotSupportedError(p.Name(), "fine-tuning")
}

// GetFineTuningJob Mistral 不支持微调.
func (p *MistralProvider) GetFineTuningJob(ctx context.Context, jobID string) (*llm.FineTuningJob, error) {
	return nil, providers.NotSupportedError(p.Name(), "fine-tuning")
}

// CancelFineTuningJob Mistral 不支持微调.
func (p *MistralProvider) CancelFineTuningJob(ctx context.Context, jobID string) error {
	return providers.NotSupportedError(p.Name(), "fine-tuning")
}
