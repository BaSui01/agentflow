package qwen

import (
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/BaSui01/agentflow/llm/providers/openaicompat"
	"go.uber.org/zap"
)

// QwenProvider 实现阿里巴巴通义千问 LLM 提供者.
// Qwen 使用 OpenAI 兼容的 API 格式.
type QwenProvider struct {
	*openaicompat.Provider
}

// NewQwenProvider 创建新的 Qwen 提供者实例.
func NewQwenProvider(cfg providers.QwenConfig, logger *zap.Logger) *QwenProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://dashscope.aliyuncs.com"
	}

	return &QwenProvider{
		Provider: openaicompat.New(openaicompat.Config{
			ProviderName:  "qwen",
			APIKey:        cfg.APIKey,
			BaseURL:       cfg.BaseURL,
			DefaultModel:  cfg.Model,
			FallbackModel: "qwen3-235b-a22b",
			Timeout:       cfg.Timeout,
			EndpointPath:  "/compatible-mode/v1/chat/completions",
		}, logger),
	}
}
