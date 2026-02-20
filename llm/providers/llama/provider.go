package llama

import (
	"fmt"

	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/BaSui01/agentflow/llm/providers/openaicompat"
	"go.uber.org/zap"
)

// LlamaProvider 通过第三方 OpenAI 兼容 API 实现 Meta Llama 提供者.
// 支持 Together AI、Replicate 和 OpenRouter.
type LlamaProvider struct {
	*openaicompat.Provider
}

// NewLlamaProvider 创建新的 Llama 提供者实例.
func NewLlamaProvider(cfg providers.LlamaConfig, logger *zap.Logger) *LlamaProvider {
	if cfg.Provider == "" {
		cfg.Provider = "together"
	}

	if cfg.BaseURL == "" {
		switch cfg.Provider {
		case "together":
			cfg.BaseURL = "https://api.together.xyz"
		case "replicate":
			cfg.BaseURL = "https://api.replicate.com"
		case "openrouter":
			cfg.BaseURL = "https://openrouter.ai/api"
		default:
			cfg.BaseURL = "https://api.together.xyz"
		}
	}

	return &LlamaProvider{
		Provider: openaicompat.New(openaicompat.Config{
			ProviderName:  fmt.Sprintf("llama-%s", cfg.Provider),
			APIKey:        cfg.APIKey,
			BaseURL:       cfg.BaseURL,
			DefaultModel:  cfg.Model,
			FallbackModel: "meta-llama/Llama-3-70b-chat-hf",
			Timeout:       cfg.Timeout,
		}, logger),
	}
}
