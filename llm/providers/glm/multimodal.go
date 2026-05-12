package glm

import (
	"context"

	providerbase "github.com/BaSui01/agentflow/llm/providers/base"

	llm "github.com/BaSui01/agentflow/llm/core"
)

// TranscribeAudio GLM 不支持音频转录.
func (p *GLMProvider) TranscribeAudio(ctx context.Context, req *llm.AudioTranscriptionRequest) (*llm.AudioTranscriptionResponse, error) {
	return p.multimodal.TranscribeAudio(ctx, req)
}

// GenerateImage 使用 GLM CogView 生成图像.
func (p *GLMProvider) GenerateImage(ctx context.Context, req *llm.ImageGenerationRequest) (*llm.ImageGenerationResponse, error) {
	return providerbase.GenerateImageOpenAICompat(ctx, providerbase.OpenAICompatParams{Client: p.Client, BaseURL: p.Cfg.BaseURL, APIKey: p.ResolveAPIKey(ctx), ProviderName: p.Name(), Endpoint: "/api/paas/v4/images/generations", BuildHeadersFunc: p.ApplyHeaders}, req)
}

// GenerateVideo 使用 GLM CogVideo 生成视频.
func (p *GLMProvider) GenerateVideo(ctx context.Context, req *llm.VideoGenerationRequest) (*llm.VideoGenerationResponse, error) {
	return providerbase.GenerateVideoOpenAICompat(ctx, providerbase.OpenAICompatParams{Client: p.Client, BaseURL: p.Cfg.BaseURL, APIKey: p.ResolveAPIKey(ctx), ProviderName: p.Name(), Endpoint: "/api/paas/v4/videos/generations", BuildHeadersFunc: p.ApplyHeaders}, req)
}

// GenerateAudio 使用 GLM 生成音频。
func (p *GLMProvider) GenerateAudio(ctx context.Context, req *llm.AudioGenerationRequest) (*llm.AudioGenerationResponse, error) {
	return providerbase.GenerateAudioOpenAICompat(ctx, providerbase.OpenAICompatParams{Client: p.Client, BaseURL: p.Cfg.BaseURL, APIKey: p.ResolveAPIKey(ctx), ProviderName: p.Name(), Endpoint: "/api/paas/v4/audio/speech", BuildHeadersFunc: p.ApplyHeaders}, req)
}

// CreateEmbedding 使用 GLM 创建嵌入.
func (p *GLMProvider) CreateEmbedding(ctx context.Context, req *llm.EmbeddingRequest) (*llm.EmbeddingResponse, error) {
	return providerbase.CreateEmbeddingOpenAICompat(ctx, providerbase.OpenAICompatParams{Client: p.Client, BaseURL: p.Cfg.BaseURL, APIKey: p.ResolveAPIKey(ctx), ProviderName: p.Name(), Endpoint: "/api/paas/v4/embeddings", BuildHeadersFunc: p.ApplyHeaders}, req)
}
