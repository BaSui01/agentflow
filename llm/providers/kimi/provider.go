package kimi

import (
	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/BaSui01/agentflow/llm/providers/openaicompat"
	"go.uber.org/zap"
)

// KimiProvider 实现月之暗面 Kimi LLM 提供者.
// Kimi 使用 OpenAI 兼容的 API 格式.
type KimiProvider struct {
	*openaicompat.Provider
}

// NewKimiProvider 创建新的 Kimi 提供者实例.
func NewKimiProvider(cfg providers.KimiConfig, logger *zap.Logger) *KimiProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.moonshot.cn"
	}

	return &KimiProvider{
		Provider: openaicompat.New(openaicompat.Config{
			ProviderName:  "kimi",
			APIKey:        cfg.APIKey,
			APIKeys:       cfg.APIKeys,
			BaseURL:       cfg.BaseURL,
			DefaultModel:  cfg.Model,
			FallbackModel: "moonshot-v1-32k",
			Timeout:       cfg.Timeout,
			RequestHook:   kimiRequestHook,
		}, logger),
	}
}

// kimiRequestHook handles Kimi-specific request modifications.
// Switches to k1 reasoning model for thinking/extended modes.
func kimiRequestHook(req *llm.ChatRequest, body *providers.OpenAICompatRequest) {
	if req.ReasoningMode == "thinking" || req.ReasoningMode == "extended" {
		if req.Model == "" {
			body.Model = "k1"
		}
	}
}

