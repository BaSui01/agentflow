package gemini

import (
	"context"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
)

// =============================================================================
// 🖼️ Image Generation
// =============================================================================

// GenerateImage generates an image using Gemini Imagen.
func (p *GeminiProvider) GenerateImage(ctx context.Context, req *llm.ImageGenerationRequest) (*llm.ImageGenerationResponse, error) {
	return providers.GenerateImageOpenAICompat(ctx, p.client, p.cfg.BaseURL, p.cfg.APIKey, p.Name(), "/v1beta/models/imagen-4:predict", req, p.buildHeaders)
}

// =============================================================================
// 🎬 Video Generation
// =============================================================================

// GenerateVideo generates a video using Gemini Veo.
func (p *GeminiProvider) GenerateVideo(ctx context.Context, req *llm.VideoGenerationRequest) (*llm.VideoGenerationResponse, error) {
	return providers.GenerateVideoOpenAICompat(ctx, p.client, p.cfg.BaseURL, p.cfg.APIKey, p.Name(), "/v1beta/models/veo-3.1:predict", req, p.buildHeaders)
}

// =============================================================================
// 🎵 Audio Generation & Transcription
// =============================================================================

// GenerateAudio generates audio using Gemini.
func (p *GeminiProvider) GenerateAudio(ctx context.Context, req *llm.AudioGenerationRequest) (*llm.AudioGenerationResponse, error) {
	return providers.GenerateAudioOpenAICompat(ctx, p.client, p.cfg.BaseURL, p.cfg.APIKey, p.Name(), "/v1beta/models/gemini-2.5-flash-live:generateContent", req, p.buildHeaders)
}

// TranscribeAudio is not supported by Gemini.
func (p *GeminiProvider) TranscribeAudio(ctx context.Context, req *llm.AudioTranscriptionRequest) (*llm.AudioTranscriptionResponse, error) {
	return nil, providers.NotSupportedError(p.Name(), "audio transcription")
}

// =============================================================================
// 📝 Embeddings
// =============================================================================

// CreateEmbedding creates embeddings using Gemini.
func (p *GeminiProvider) CreateEmbedding(ctx context.Context, req *llm.EmbeddingRequest) (*llm.EmbeddingResponse, error) {
	return providers.CreateEmbeddingOpenAICompat(ctx, p.client, p.cfg.BaseURL, p.cfg.APIKey, p.Name(), "/v1beta/models/text-embedding-004:embedContent", req, p.buildHeaders)
}

// =============================================================================
// 🔄 Fine-Tuning
// =============================================================================

// CreateFineTuningJob is not supported by Gemini.
func (p *GeminiProvider) CreateFineTuningJob(ctx context.Context, req *llm.FineTuningJobRequest) (*llm.FineTuningJob, error) {
	return nil, providers.NotSupportedError(p.Name(), "fine-tuning")
}

// ListFineTuningJobs is not supported by Gemini.
func (p *GeminiProvider) ListFineTuningJobs(ctx context.Context) ([]llm.FineTuningJob, error) {
	return nil, providers.NotSupportedError(p.Name(), "fine-tuning")
}

// GetFineTuningJob is not supported by Gemini.
func (p *GeminiProvider) GetFineTuningJob(ctx context.Context, jobID string) (*llm.FineTuningJob, error) {
	return nil, providers.NotSupportedError(p.Name(), "fine-tuning")
}

// CancelFineTuningJob is not supported by Gemini.
func (p *GeminiProvider) CancelFineTuningJob(ctx context.Context, jobID string) error {
	return providers.NotSupportedError(p.Name(), "fine-tuning")
}
