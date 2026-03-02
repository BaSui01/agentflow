package hunyuan

import (
	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
	providerbase "github.com/BaSui01/agentflow/llm/providers/base"
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
		cfg.BaseURL = "https://api.hunyuan.cloud.tencent.com"
	}

	return &HunyuanProvider{
		Provider: openaicompat.New(openaicompat.Config{
			ProviderName:  "hunyuan",
			APIKey:        cfg.APIKey,
			APIKeys:       cfg.APIKeys,
			BaseURL:       cfg.BaseURL,
			DefaultModel:  cfg.Model,
			FallbackModel: "hunyuan-turbos-latest",
			Timeout:       cfg.Timeout,
			RequestHook:   hunyuanRequestHook,
		}, logger),
	}
}

// hunyuanRequestHook handles Hunyuan-specific request modifications.
// - Switches to hunyuan-t1 reasoning model for thinking/extended modes.
// - Routes to hunyuan-functioncall when tools are present and no specific model is set.
func hunyuanRequestHook(req *llm.ChatRequest, body *providerbase.OpenAICompatRequest) {
	// Reasoning mode: switch to hunyuan-t1
	if req.ReasoningMode == "thinking" || req.ReasoningMode == "extended" {
		if req.Model == "" {
			body.Model = "hunyuan-t1"
		}
		return
	}
	// Function calling routing: when tools are present and user didn't specify a model,
	// route to the dedicated function calling model.
	if len(body.Tools) > 0 && req.Model == "" &&
		body.Model != "hunyuan-functioncall" && body.Model != "hunyuan-t1" {
		body.Model = "hunyuan-functioncall"
	}
}
