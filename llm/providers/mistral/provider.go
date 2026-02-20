package mistral

import (
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/BaSui01/agentflow/llm/providers/openaicompat"
	"go.uber.org/zap"
)

// MistralProvider 实现 Mistral AI LLM 提供者.
// Mistral AI 使用 OpenAI 兼容的 API 格式.
type MistralProvider struct {
	*openaicompat.Provider
}

// NewMistralProvider 创建新的 Mistral 提供者实例.
func NewMistralProvider(cfg providers.MistralConfig, logger *zap.Logger) *MistralProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.mistral.ai"
	}

	return &MistralProvider{
		Provider: openaicompat.New(openaicompat.Config{
			ProviderName:  "mistral",
			APIKey:        cfg.APIKey,
			BaseURL:       cfg.BaseURL,
			DefaultModel:  cfg.Model,
			FallbackModel: "mistral-large-latest",
			Timeout:       cfg.Timeout,
		}, logger),
	}
}
