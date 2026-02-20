package deepseek

import (
	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/BaSui01/agentflow/llm/providers/openaicompat"
	"go.uber.org/zap"
)

// DeepSeekProvider 实现 DeepSeek LLM 提供者.
// DeepSeek 使用 OpenAI 兼容的 API 格式.
type DeepSeekProvider struct {
	*openaicompat.Provider
}

// NewDeepSeekProvider 创建新的 DeepSeek 提供者实例.
func NewDeepSeekProvider(cfg providers.DeepSeekConfig, logger *zap.Logger) *DeepSeekProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.deepseek.com"
	}

	return &DeepSeekProvider{
		Provider: openaicompat.New(openaicompat.Config{
			ProviderName:  "deepseek",
			APIKey:        cfg.APIKey,
			BaseURL:       cfg.BaseURL,
			DefaultModel:  cfg.Model,
			FallbackModel: "deepseek-chat",
			Timeout:       cfg.Timeout,
			EndpointPath:  "/chat/completions",
			RequestHook:   deepseekRequestHook,
		}, logger),
	}
}

// deepseekRequestHook handles DeepSeek-specific request modifications.
// Automatically selects deepseek-reasoner model for thinking/extended reasoning modes.
func deepseekRequestHook(req *llm.ChatRequest, body *providers.OpenAICompatRequest) {
	if req.ReasoningMode == "thinking" || req.ReasoningMode == "extended" {
		if req.Model == "" {
			body.Model = "deepseek-reasoner"
		}
	}
}
