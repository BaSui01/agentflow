package hunyuan

import (
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/BaSui01/agentflow/llm/providers/openaicompat"
	"go.uber.org/zap"
)

// HunyuanProvider 实现腾讯混元 LLM 提供者.
// Hunyuan 使用 OpenAI 兼容的 API 格式.
type HunyuanProvider struct {
	*openaicompat.Provider
}

// NewHunyuanProvider 创建新的 Hunyuan 提供者实例.
func NewHunyuanProvider(cfg providers.HunyuanConfig, logger *zap.Logger) *HunyuanProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.hunyuan.cloud.tencent.com/v1"
	}

	return &HunyuanProvider{
		Provider: openaicompat.New(openaicompat.Config{
			ProviderName:  "hunyuan",
			APIKey:        cfg.APIKey,
			BaseURL:       cfg.BaseURL,
			DefaultModel:  cfg.Model,
			FallbackModel: "hunyuan-pro",
			Timeout:       cfg.Timeout,
		}, logger),
	}
}
