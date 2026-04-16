package glm

import (
	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
	providerbase "github.com/BaSui01/agentflow/llm/providers/base"
	"github.com/BaSui01/agentflow/llm/providers/openaicompat"
	"go.uber.org/zap"
)

// GLMProvider 实现智谱 AI GLM LLM 提供者.
// GLM 使用 OpenAI 兼容的 API 格式.
type GLMProvider struct {
	*openaicompat.Provider
}

// newGLMCapabilityHost 创建 GLM capability host。
// 它承载 image/video/audio/embedding/fine-tuning 等能力实现，但不是公共 chat 主链入口。
func newGLMCapabilityHost(cfg providers.GLMConfig, logger *zap.Logger) *GLMProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://open.bigmodel.cn"
	}

	return &GLMProvider{
		Provider: openaicompat.New(openaicompat.Config{
			ProviderName:  "glm",
			APIKey:        cfg.APIKey,
			APIKeys:       cfg.APIKeys,
			BaseURL:       cfg.BaseURL,
			DefaultModel:  cfg.Model,
			FallbackModel: "glm-4-plus",
			Timeout:       cfg.Timeout,
			EndpointPath:  "/api/paas/v4/chat/completions",
			RequestHook:   glmRequestHook,
		}, logger),
	}
}

// newGLMProvider 仅供本包测试与能力承载复用；公共 chat 入口统一走 vendor factory。
func newGLMProvider(cfg providers.GLMConfig, logger *zap.Logger) *GLMProvider {
	return newGLMCapabilityHost(cfg, logger)
}

// glmRequestHook handles GLM-specific request modifications.
// Switches to glm-z1-flash reasoning model for thinking/extended modes.
func glmRequestHook(req *llm.ChatRequest, body *providerbase.OpenAICompatRequest) {
	if req.ReasoningMode == "thinking" || req.ReasoningMode == "extended" {
		if req.Model == "" {
			body.Model = "glm-z1-flash"
		}
	}
}
