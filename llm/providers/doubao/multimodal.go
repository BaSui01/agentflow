package doubao

import (
	"context"

	providerbase "github.com/BaSui01/agentflow/llm/providers/base"

	llm "github.com/BaSui01/agentflow/llm/core"
)

// GenerateImage 使用 Doubao 生成图像.
func (p *DoubaoProvider) GenerateImage(ctx context.Context, req *llm.ImageGenerationRequest) (*llm.ImageGenerationResponse, error) {
	return providerbase.GenerateImageOpenAICompat(ctx, providerbase.OpenAICompatParams{Client: p.Client, BaseURL: p.Cfg.BaseURL, APIKey: p.ResolveAPIKey(ctx), ProviderName: p.Name(), Endpoint: "/api/v3/images/generations", BuildHeadersFunc: p.ApplyHeaders}, req)
}

// GenerateAudio 使用 Doubao 生成音频.
func (p *DoubaoProvider) GenerateAudio(ctx context.Context, req *llm.AudioGenerationRequest) (*llm.AudioGenerationResponse, error) {
	return providerbase.GenerateAudioOpenAICompat(ctx, providerbase.OpenAICompatParams{Client: p.Client, BaseURL: p.Cfg.BaseURL, APIKey: p.ResolveAPIKey(ctx), ProviderName: p.Name(), Endpoint: "/api/v3/audio/speech", BuildHeadersFunc: p.ApplyHeaders}, req)
}

// CreateEmbedding 使用 Doubao 创建嵌入.
func (p *DoubaoProvider) CreateEmbedding(ctx context.Context, req *llm.EmbeddingRequest) (*llm.EmbeddingResponse, error) {
	return providerbase.CreateEmbeddingOpenAICompat(ctx, providerbase.OpenAICompatParams{Client: p.Client, BaseURL: p.Cfg.BaseURL, APIKey: p.ResolveAPIKey(ctx), ProviderName: p.Name(), Endpoint: "/api/v3/embeddings", BuildHeadersFunc: p.ApplyHeaders}, req)
}
