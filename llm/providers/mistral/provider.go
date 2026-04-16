package mistral

import (
	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
	providerbase "github.com/BaSui01/agentflow/llm/providers/base"
	"github.com/BaSui01/agentflow/llm/providers/openaicompat"
	"go.uber.org/zap"
)

// MistralProvider 实现 Mistral AI LLM 提供者.
// Mistral AI 使用 OpenAI 兼容的 API 格式.
type MistralProvider struct {
	*openaicompat.Provider
}

// newMistralCapabilityHost 创建 Mistral capability host。
// 它承载 transcription/embedding/fine-tuning 等能力实现，但不是公共 chat 主链入口。
func newMistralCapabilityHost(cfg providers.MistralConfig, logger *zap.Logger) *MistralProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.mistral.ai"
	}

	return &MistralProvider{
		Provider: openaicompat.New(openaicompat.Config{
			ProviderName:  "mistral",
			APIKey:        cfg.APIKey,
			APIKeys:       cfg.APIKeys,
			BaseURL:       cfg.BaseURL,
			DefaultModel:  cfg.Model,
			FallbackModel: "mistral-large-latest",
			Timeout:       cfg.Timeout,
			RequestHook:   mistralRequestHook,
		}, logger),
	}
}

// newMistralProvider 仅供本包测试与能力承载复用；公共 chat 入口统一走 vendor factory。
func newMistralProvider(cfg providers.MistralConfig, logger *zap.Logger) *MistralProvider {
	return newMistralCapabilityHost(cfg, logger)
}

// mistralRequestHook handles Mistral-specific request modifications.
// Switches to magistral-medium-latest reasoning model for thinking/extended modes.
func mistralRequestHook(req *llm.ChatRequest, body *providerbase.OpenAICompatRequest) {
	if req.ReasoningMode == "thinking" || req.ReasoningMode == "extended" {
		if req.Model == "" {
			body.Model = "magistral-medium-latest"
		}
	}
}
