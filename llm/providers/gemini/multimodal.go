package gemini

import (
	"context"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
)

// =============================================================================
// QQ 图像生成
// =============================================================================

// 生成图像使用双子座图像生成图像.
func (p *GeminiProvider) GenerateImage(ctx context.Context, req *llm.ImageGenerationRequest) (*llm.ImageGenerationResponse, error) {
	return providers.GenerateImageOpenAICompat(ctx, p.client, p.cfg.BaseURL, p.cfg.APIKey, p.Name(), "/v1beta/models/imagen-4:predict", req, p.buildHeaders)
}

// =============================================================================
// 视频生成
// =============================================================================

// 生成Video使用双子座Veo生成视频.
func (p *GeminiProvider) GenerateVideo(ctx context.Context, req *llm.VideoGenerationRequest) (*llm.VideoGenerationResponse, error) {
	return providers.GenerateVideoOpenAICompat(ctx, p.client, p.cfg.BaseURL, p.cfg.APIKey, p.Name(), "/v1beta/models/veo-3.1:predict", req, p.buildHeaders)
}

// =============================================================================
// QQ 音频生成和转录
// =============================================================================

// 生成Audio使用双子座生成音频.
func (p *GeminiProvider) GenerateAudio(ctx context.Context, req *llm.AudioGenerationRequest) (*llm.AudioGenerationResponse, error) {
	return providers.GenerateAudioOpenAICompat(ctx, p.client, p.cfg.BaseURL, p.cfg.APIKey, p.Name(), "/v1beta/models/gemini-2.5-flash-live:generateContent", req, p.buildHeaders)
}

// TrancisAudio不支持双子座.
func (p *GeminiProvider) TranscribeAudio(ctx context.Context, req *llm.AudioTranscriptionRequest) (*llm.AudioTranscriptionResponse, error) {
	return nil, providers.NotSupportedError(p.Name(), "audio transcription")
}

// =============================================================================
// * 嵌入物
// =============================================================================

// CreateEmbedding使用双子座创建嵌入.
func (p *GeminiProvider) CreateEmbedding(ctx context.Context, req *llm.EmbeddingRequest) (*llm.EmbeddingResponse, error) {
	return providers.CreateEmbeddingOpenAICompat(ctx, p.client, p.cfg.BaseURL, p.cfg.APIKey, p.Name(), "/v1beta/models/text-embedding-004:embedContent", req, p.buildHeaders)
}

// =============================================================================
// {\fn黑体\fs22\bord1\shad0\3aHBE\4aH00\fscx67\fscy66\2cHFFFFFF\3cH808080}好图宁 {\fn黑体\fs22\bord1\shad0\3aHBE\4aH00\fscx67\fscy66\2cHFFFFFF\3cH808080}好图宁
// =============================================================================

// CreateFineTuningJob 不支持双子座.
func (p *GeminiProvider) CreateFineTuningJob(ctx context.Context, req *llm.FineTuningJobRequest) (*llm.FineTuningJob, error) {
	return nil, providers.NotSupportedError(p.Name(), "fine-tuning")
}

// ListFineTuningJobs 不支持双子座.
func (p *GeminiProvider) ListFineTuningJobs(ctx context.Context) ([]llm.FineTuningJob, error) {
	return nil, providers.NotSupportedError(p.Name(), "fine-tuning")
}

// Get FineTuningJob 不支持双子座.
func (p *GeminiProvider) GetFineTuningJob(ctx context.Context, jobID string) (*llm.FineTuningJob, error) {
	return nil, providers.NotSupportedError(p.Name(), "fine-tuning")
}

// 取消FineTuningJob不支持双子座.
func (p *GeminiProvider) CancelFineTuningJob(ctx context.Context, jobID string) error {
	return providers.NotSupportedError(p.Name(), "fine-tuning")
}
