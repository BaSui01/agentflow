package glm

import (
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/BaSui01/agentflow/llm/providers/openaicompat"
	"go.uber.org/zap"
)

// GLMProvider 实现智谱 AI GLM LLM 提供者.
// GLM 使用 OpenAI 兼容的 API 格式.
type GLMProvider struct {
	*openaicompat.Provider
}

// NewGLMProvider 创建新的 GLM 提供者实例.
func NewGLMProvider(cfg providers.GLMConfig, logger *zap.Logger) *GLMProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://open.bigmodel.cn"
	}

	return &GLMProvider{
		Provider: openaicompat.New(openaicompat.Config{
			ProviderName:  "glm",
			APIKey:        cfg.APIKey,
			BaseURL:       cfg.BaseURL,
			DefaultModel:  cfg.Model,
			FallbackModel: "glm-4-plus",
			Timeout:       cfg.Timeout,
			EndpointPath:  "/api/paas/v4/chat/completions",
		}, logger),
	}
}
