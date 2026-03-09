package grok

import (
	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
	providerbase "github.com/BaSui01/agentflow/llm/providers/base"
	"github.com/BaSui01/agentflow/llm/providers/openaicompat"
	"go.uber.org/zap"
)

const defaultGrokBaseURL = "https://api.x.ai"

// GrokProvider 实现 xAI Grok LLM 提供者.
// Grok 使用 OpenAI 兼容的 API 格式；默认 Base URL https://api.x.ai
type GrokProvider struct {
	*openaicompat.Provider
}

// NewGrokProvider 创建新的 Grok 提供者实例.
func NewGrokProvider(cfg providers.GrokConfig, logger *zap.Logger) *GrokProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultGrokBaseURL
	}

	return &GrokProvider{
		Provider: openaicompat.New(openaicompat.Config{
			ProviderName:  "grok",
			APIKey:        cfg.APIKey,
			APIKeys:       cfg.APIKeys,
			BaseURL:       cfg.BaseURL,
			DefaultModel:  cfg.Model,
			FallbackModel: "grok-3",
			Timeout:       cfg.Timeout,
			RequestHook:   grokRequestHook,
		}, logger),
	}
}

// grokRequestHook handles Grok-specific request modifications.
// Switches to grok-3-mini reasoning model for thinking/extended modes.
func grokRequestHook(req *llm.ChatRequest, body *providerbase.OpenAICompatRequest) {
	if req.ReasoningMode == "thinking" || req.ReasoningMode == "extended" {
		if req.Model == "" {
			body.Model = "grok-3-mini"
		}
	}
}
