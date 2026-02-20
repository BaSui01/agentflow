package minimax

import (
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/BaSui01/agentflow/llm/providers/openaicompat"
	"go.uber.org/zap"
)

// MiniMaxProvider 实现 MiniMax LLM 提供者.
// MiniMax 使用 OpenAI 兼容的 API 格式.
type MiniMaxProvider struct {
	*openaicompat.Provider
}

// NewMiniMaxProvider 创建新的 MiniMax 提供者实例.
func NewMiniMaxProvider(cfg providers.MiniMaxConfig, logger *zap.Logger) *MiniMaxProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.minimax.io"
	}

	return &MiniMaxProvider{
		Provider: openaicompat.New(openaicompat.Config{
			ProviderName:  "minimax",
			APIKey:        cfg.APIKey,
			BaseURL:       cfg.BaseURL,
			DefaultModel:  cfg.Model,
			FallbackModel: "abab6.5s-chat",
			Timeout:       cfg.Timeout,
		}, logger),
	}
}
