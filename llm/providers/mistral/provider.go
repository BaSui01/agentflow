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

// NewMistralProvider 创建新的 Mistral 提供者实例.
func NewMistralProvider(cfg providers.MistralConfig, logger *zap.Logger) *MistralProvider {
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

// mistralRequestHook handles Mistral-specific request modifications.
// Switches to magistral-medium-latest reasoning model for thinking/extended modes.
func mistralRequestHook(req *llm.ChatRequest, body *providerbase.OpenAICompatRequest) {
	if req.ReasoningMode == "thinking" || req.ReasoningMode == "extended" {
		if req.Model == "" {
			body.Model = "magistral-medium-latest"
		}
	}
}
