package gemini

import (
	"context"
	"fmt"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
)

// =============================================================================
// 图像生成
// =============================================================================

// GenerateImage generates images using Gemini Imagen 4.
// Endpoint: POST /v1beta/models/{model}:predict
// Models: imagen-4.0-generate-001 (standard), imagen-4.0-ultra-generate-001 (ultra), imagen-4.0-fast-generate-001 (fast)
// Also supports native generation via gemini-2.5-flash-image, gemini-3-pro-image-preview
func (p *GeminiProvider) GenerateImage(ctx context.Context, req *llm.ImageGenerationRequest) (*llm.ImageGenerationResponse, error) {
	model := req.Model
	if model == "" {
		model = "imagen-4.0-generate-001"
	}
	endpoint := fmt.Sprintf("/v1beta/models/%s:predict", model)
	return providers.GenerateImageOpenAICompat(ctx, p.client, p.cfg.BaseURL, p.cfg.APIKey, p.Name(), endpoint, req, p.buildHeaders)
}

// =============================================================================
// 视频生成
// =============================================================================

// GenerateVideo 使用 Gemini Veo 生成视频.
// Endpoint: POST /v1beta/models/{model}:predictLongRunning
// Models: veo-3.1-generate-preview (standard), veo-3.1-fast-generate-preview (fast)
func (p *GeminiProvider) GenerateVideo(ctx context.Context, req *llm.VideoGenerationRequest) (*llm.VideoGenerationResponse, error) {
	model := req.Model
	if model == "" {
		model = "veo-3.1-generate-preview"
	}
	endpoint := fmt.Sprintf("/v1beta/models/%s:predictLongRunning", model)
	return providers.GenerateVideoOpenAICompat(ctx, p.client, p.cfg.BaseURL, p.cfg.APIKey, p.Name(), endpoint, req, p.buildHeaders)
}

// =============================================================================
// 音频生成和转录
// =============================================================================

// GenerateAudio 使用 Gemini TTS 生成音频.
// Endpoint: POST /v1beta/models/{model}:generateContent
// Models: gemini-2.5-flash-preview-tts, gemini-2.5-pro-preview-tts
// Supports 30+ voices including Kore, Charon, Fenrir, Aoede, Puck, etc.
func (p *GeminiProvider) GenerateAudio(ctx context.Context, req *llm.AudioGenerationRequest) (*llm.AudioGenerationResponse, error) {
	model := req.Model
	if model == "" {
		model = "gemini-2.5-flash-preview-tts"
	}
	endpoint := fmt.Sprintf("/v1beta/models/%s:generateContent", model)
	return providers.GenerateAudioOpenAICompat(ctx, p.client, p.cfg.BaseURL, p.cfg.APIKey, p.Name(), endpoint, req, p.buildHeaders)
}

// TranscribeAudio Gemini 不支持音频转录.
func (p *GeminiProvider) TranscribeAudio(ctx context.Context, req *llm.AudioTranscriptionRequest) (*llm.AudioTranscriptionResponse, error) {
	return nil, providers.NotSupportedError(p.Name(), "audio transcription")
}

// =============================================================================
// 嵌入
// =============================================================================

// CreateEmbedding creates embeddings using Gemini.
// Endpoint: POST /v1beta/models/{model}:embedContent
// Models: gemini-embedding-001 (latest, MRL, 128-3072 dims), text-embedding-004 (legacy)
func (p *GeminiProvider) CreateEmbedding(ctx context.Context, req *llm.EmbeddingRequest) (*llm.EmbeddingResponse, error) {
	model := req.Model
	if model == "" {
		model = "gemini-embedding-001"
	}
	endpoint := fmt.Sprintf("/v1beta/models/%s:embedContent", model)
	return providers.CreateEmbeddingOpenAICompat(ctx, p.client, p.cfg.BaseURL, p.cfg.APIKey, p.Name(), endpoint, req, p.buildHeaders)
}

// =============================================================================
// 微调
// =============================================================================

// CreateFineTuningJob Gemini 暂不支持微调.
func (p *GeminiProvider) CreateFineTuningJob(ctx context.Context, req *llm.FineTuningJobRequest) (*llm.FineTuningJob, error) {
	return nil, providers.NotSupportedError(p.Name(), "fine-tuning")
}

// ListFineTuningJobs Gemini 暂不支持微调.
func (p *GeminiProvider) ListFineTuningJobs(ctx context.Context) ([]llm.FineTuningJob, error) {
	return nil, providers.NotSupportedError(p.Name(), "fine-tuning")
}

// GetFineTuningJob Gemini 暂不支持微调.
func (p *GeminiProvider) GetFineTuningJob(ctx context.Context, jobID string) (*llm.FineTuningJob, error) {
	return nil, providers.NotSupportedError(p.Name(), "fine-tuning")
}

// CancelFineTuningJob Gemini 暂不支持微调.
func (p *GeminiProvider) CancelFineTuningJob(ctx context.Context, jobID string) error {
	return providers.NotSupportedError(p.Name(), "fine-tuning")
}
