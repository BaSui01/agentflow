package grok

import (
	llm "github.com/BaSui01/agentflow/llm/core"
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

// newGrokCapabilityHost 创建 Grok capability host。
// 它承载 image/video/embedding 等厂商能力实现，但不是公共 chat 主链入口。
func newGrokCapabilityHost(cfg providers.GrokConfig, logger *zap.Logger) *GrokProvider {
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
			FallbackModel: "grok-4.20",
			Timeout:       cfg.Timeout,
			RequestHook:   grokRequestHook,
		}, logger),
	}
}

// newGrokProvider 仅供本包测试与能力承载复用；公共 chat 入口统一走 vendor factory。
func newGrokProvider(cfg providers.GrokConfig, logger *zap.Logger) *GrokProvider {
	return newGrokCapabilityHost(cfg, logger)
}

// grokRequestHook handles Grok-specific request modifications.
// Switches to grok-4.20 reasoning model for thinking/extended modes.
func grokRequestHook(req *llm.ChatRequest, body *providerbase.OpenAICompatRequest) {
	if req.ReasoningMode == "thinking" || req.ReasoningMode == "extended" {
		if req.Model == "" {
			body.Model = "grok-4.20-reasoning"
		}
	}
}
