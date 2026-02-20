package mistral

import (
	"context"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
)

// 生成图像不被Mistral支持.
func (p *MistralProvider) GenerateImage(ctx context.Context, req *llm.ImageGenerationRequest) (*llm.ImageGenerationResponse, error) {
	return nil, providers.NotSupportedError(p.Name(), "image generation")
}

// 生成Video不被Mistral支持.
func (p *MistralProvider) GenerateVideo(ctx context.Context, req *llm.VideoGenerationRequest) (*llm.VideoGenerationResponse, error) {
	return nil, providers.NotSupportedError(p.Name(), "video generation")
}

// 生成Audio不被Mistral支持.
func (p *MistralProvider) GenerateAudio(ctx context.Context, req *llm.AudioGenerationRequest) (*llm.AudioGenerationResponse, error) {
	return nil, providers.NotSupportedError(p.Name(), "audio generation")
}

// TrancisAudio不被米斯特拉尔支持.
func (p *MistralProvider) TranscribeAudio(ctx context.Context, req *llm.AudioTranscriptionRequest) (*llm.AudioTranscriptionResponse, error) {
	return nil, providers.NotSupportedError(p.Name(), "audio transcription")
}

// CreateEmbedding 创建了使用Mistral(授权到 OpenAI 执行)的嵌入.
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

// CreateFineTuningJob 不为米斯特拉尔所支持.
func (p *MistralProvider) CreateFineTuningJob(ctx context.Context, req *llm.FineTuningJobRequest) (*llm.FineTuningJob, error) {
	return nil, providers.NotSupportedError(p.Name(), "fine-tuning")
}

// ListFineTuningJobs 不为米斯特拉尔所支持.
func (p *MistralProvider) ListFineTuningJobs(ctx context.Context) ([]llm.FineTuningJob, error) {
	return nil, providers.NotSupportedError(p.Name(), "fine-tuning")
}

// Get FineTuningJob 不支持米斯特拉尔.
func (p *MistralProvider) GetFineTuningJob(ctx context.Context, jobID string) (*llm.FineTuningJob, error) {
	return nil, providers.NotSupportedError(p.Name(), "fine-tuning")
}

// 取消FineTuningJob不被Mistral支持.
func (p *MistralProvider) CancelFineTuningJob(ctx context.Context, jobID string) error {
	return providers.NotSupportedError(p.Name(), "fine-tuning")
}
