package providerbase

import (
	"context"
	"net/http"

	llm "github.com/BaSui01/agentflow/llm/core"
)

// MultimodalAdapterConfig configures the default provider multimodal adapter.
type MultimodalAdapterConfig struct {
	ProviderName string
}

// MultimodalAdapter provides default multimodal/fine-tuning behavior for
// providers that only support a subset of capabilities. Providers embed or
// delegate to this adapter and override supported methods only.
type MultimodalAdapter struct {
	providerName string
}

// NewMultimodalAdapter creates a default multimodal adapter.
func NewMultimodalAdapter(config MultimodalAdapterConfig) *MultimodalAdapter {
	return &MultimodalAdapter{providerName: config.ProviderName}
}

func (a *MultimodalAdapter) name() string {
	if a == nil || a.providerName == "" {
		return "provider"
	}
	return a.providerName
}

func (a *MultimodalAdapter) unsupported(feature string) *llm.Error {
	err := NotSupportedError(a.name(), feature)
	err.HTTPStatus = http.StatusNotImplemented
	return err
}

// GenerateImage returns unsupported by default.
func (a *MultimodalAdapter) GenerateImage(context.Context, *llm.ImageGenerationRequest) (*llm.ImageGenerationResponse, error) {
	return nil, a.unsupported("image generation")
}

// GenerateVideo returns unsupported by default.
func (a *MultimodalAdapter) GenerateVideo(context.Context, *llm.VideoGenerationRequest) (*llm.VideoGenerationResponse, error) {
	return nil, a.unsupported("video generation")
}

// GenerateAudio returns unsupported by default.
func (a *MultimodalAdapter) GenerateAudio(context.Context, *llm.AudioGenerationRequest) (*llm.AudioGenerationResponse, error) {
	return nil, a.unsupported("audio generation")
}

// TranscribeAudio returns unsupported by default.
func (a *MultimodalAdapter) TranscribeAudio(context.Context, *llm.AudioTranscriptionRequest) (*llm.AudioTranscriptionResponse, error) {
	return nil, a.unsupported("audio transcription")
}

// CreateEmbedding returns unsupported by default.
func (a *MultimodalAdapter) CreateEmbedding(context.Context, *llm.EmbeddingRequest) (*llm.EmbeddingResponse, error) {
	return nil, a.unsupported("embeddings")
}

// CreateFineTuningJob returns unsupported by default.
func (a *MultimodalAdapter) CreateFineTuningJob(context.Context, *llm.FineTuningJobRequest) (*llm.FineTuningJob, error) {
	return nil, a.unsupported("fine-tuning")
}

// ListFineTuningJobs returns unsupported by default.
func (a *MultimodalAdapter) ListFineTuningJobs(context.Context) ([]llm.FineTuningJob, error) {
	return nil, a.unsupported("fine-tuning")
}

// GetFineTuningJob returns unsupported by default.
func (a *MultimodalAdapter) GetFineTuningJob(context.Context, string) (*llm.FineTuningJob, error) {
	return nil, a.unsupported("fine-tuning")
}

// CancelFineTuningJob returns unsupported by default.
func (a *MultimodalAdapter) CancelFineTuningJob(context.Context, string) error {
	return a.unsupported("fine-tuning")
}
