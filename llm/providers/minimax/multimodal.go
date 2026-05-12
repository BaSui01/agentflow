package minimax

import (
	"context"

	providerbase "github.com/BaSui01/agentflow/llm/providers/base"

	llm "github.com/BaSui01/agentflow/llm/core"
)

// GenerateAudio 使用 MiniMax 生成音频.
func (p *MiniMaxProvider) GenerateAudio(ctx context.Context, req *llm.AudioGenerationRequest) (*llm.AudioGenerationResponse, error) {
	return providerbase.GenerateAudioOpenAICompat(ctx, providerbase.OpenAICompatParams{Client: p.Client, BaseURL: p.Cfg.BaseURL, APIKey: p.ResolveAPIKey(ctx), ProviderName: p.Name(), Endpoint: "/v1/audio/speech", BuildHeadersFunc: p.ApplyHeaders}, req)
}
