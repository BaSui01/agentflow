package doubao

import (
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/BaSui01/agentflow/llm/providers/openaicompat"
	"go.uber.org/zap"
)

// DoubaoProvider 实现字节跳动豆包 LLM 提供者.
// Doubao 使用 OpenAI 兼容的 API 格式.
type DoubaoProvider struct {
	*openaicompat.Provider
}

// NewDoubaoProvider 创建新的 Doubao 提供者实例.
func NewDoubaoProvider(cfg providers.DoubaoConfig, logger *zap.Logger) *DoubaoProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://ark.cn-beijing.volces.com"
	}

	return &DoubaoProvider{
		Provider: openaicompat.New(openaicompat.Config{
			ProviderName:  "doubao",
			APIKey:        cfg.APIKey,
			BaseURL:       cfg.BaseURL,
			DefaultModel:  cfg.Model,
			FallbackModel: "Doubao-1.5-pro-32k",
			Timeout:       cfg.Timeout,
			EndpointPath:  "/api/v3/chat/completions",
		}, logger),
	}
}
