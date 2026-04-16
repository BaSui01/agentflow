package qwen

import (
	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
	providerbase "github.com/BaSui01/agentflow/llm/providers/base"
	"github.com/BaSui01/agentflow/llm/providers/openaicompat"
	"go.uber.org/zap"
)

// QwenProvider 实现阿里巴巴通义千问 LLM 提供者.
// Qwen 使用 OpenAI 兼容的 API 格式.
type QwenProvider struct {
	*openaicompat.Provider
}

// newQwenCapabilityHost 创建 Qwen capability host。
// 它承载 image/video/audio/embedding 等厂商能力实现，但不是公共 chat 主链入口。
func newQwenCapabilityHost(cfg providers.QwenConfig, logger *zap.Logger) *QwenProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://dashscope.aliyuncs.com"
	}

	return &QwenProvider{
		Provider: openaicompat.New(openaicompat.Config{
			ProviderName:  "qwen",
			APIKey:        cfg.APIKey,
			APIKeys:       cfg.APIKeys,
			BaseURL:       cfg.BaseURL,
			DefaultModel:  cfg.Model,
			FallbackModel: "qwen3-235b-a22b",
			Timeout:       cfg.Timeout,
			EndpointPath:  "/compatible-mode/v1/chat/completions",
			RequestHook:   qwenRequestHook,
		}, logger),
	}
}

// newQwenProvider 仅供本包测试与能力承载复用；公共 chat 入口统一走 vendor factory。
func newQwenProvider(cfg providers.QwenConfig, logger *zap.Logger) *QwenProvider {
	return newQwenCapabilityHost(cfg, logger)
}

// qwenRequestHook handles Qwen-specific request modifications.
// Switches to qwen3-max for thinking/extended reasoning modes.
// Qwen3 series natively supports thinking mode; qwen3-max enables it by default.
func qwenRequestHook(req *llm.ChatRequest, body *providerbase.OpenAICompatRequest) {
	if req.ReasoningMode == "thinking" || req.ReasoningMode == "extended" {
		if req.Model == "" {
			body.Model = "qwen3-max"
		}
	}
}
