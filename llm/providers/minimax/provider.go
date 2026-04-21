package minimax

import (
	"strings"

	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/BaSui01/agentflow/llm/providers/openaicompat"
	"go.uber.org/zap"
)

// MiniMaxProvider 实现 MiniMax LLM 提供者.
// MiniMax 使用 OpenAI 兼容的 API 格式.
//
// 旧模型（abab 系列）不支持原生函数调用，框架会自动降级为 XML 工具调用模式。
// 新模型（MiniMax-Text-01, M1, M2 等）支持标准 JSON tool calling。
type MiniMaxProvider struct {
	*openaicompat.Provider
}

// newMiniMaxCapabilityHost 创建 MiniMax capability host。
// 它承载 audio 等厂商能力实现，同时复用 compat transport 基础设施。
// 注意：这不是对外 chat 主链入口；chat 主链统一走 vendor.NewChatProviderFromConfig。
func newMiniMaxCapabilityHost(cfg providers.MiniMaxConfig, logger *zap.Logger) *MiniMaxProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.minimax.io"
	}

	// 旧模型不支持原生工具调用，框架会自动降级到 XML 模式
	supportsTools := !isLegacyModel(cfg.Model)

	return &MiniMaxProvider{
		Provider: openaicompat.New(openaicompat.Config{
			ProviderName:  "minimax",
			APIKey:        cfg.APIKey,
			APIKeys:       cfg.APIKeys,
			BaseURL:       cfg.BaseURL,
			DefaultModel:  cfg.Model,
			FallbackModel: "MiniMax-M2.7",
			Timeout:       cfg.Timeout,
			SupportsTools: &supportsTools,
		}, logger),
	}
}

// newMiniMaxProvider 创建新的 MiniMax 提供者实例。
// 仅供本包测试与能力承载复用；公共 chat 入口已收敛到 vendor factory。
func newMiniMaxProvider(cfg providers.MiniMaxConfig, logger *zap.Logger) *MiniMaxProvider {
	return newMiniMaxCapabilityHost(cfg, logger)
}

// isLegacyModel returns true for old MiniMax models (abab series) that use XML tool call format.
func isLegacyModel(model string) bool {
	return strings.HasPrefix(model, "abab")
}
