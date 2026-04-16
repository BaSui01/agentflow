package llm

import (
	"context"
	"time"

	"github.com/BaSui01/agentflow/types"
)

// =============================================================================
// Provider 接口 - 实现 types.ChatProvider
// =============================================================================
// llm.Provider 扩展 types.ChatProvider，增加健康检查、模型列表等方法。
// 核心类型（ChatRequest、ChatResponse、StreamChunk）定义在 types 层。
// =============================================================================

// Provider 定义了统一的 LLM 适配器接口.
// 扩展 types.ChatProvider，增加健康检查、模型列表等方法。
type Provider interface {
	// 继承 types.ChatProvider 的核心方法
	types.ChatProvider

	// HealthCheck 执行轻量级健康检查。
	HealthCheck(ctx context.Context) (*HealthStatus, error)

	// SupportsNativeFunctionCalling 返回是否支持原生函数调用。
	SupportsNativeFunctionCalling() bool

	// ListModels 返回提供者支持的可用模型列表。
	// 如果提供者不支持列出模型，则返回 nil。
	ListModels(ctx context.Context) ([]Model, error)

	// Endpoints 返回该提供者使用的所有 API 端点完整 URL，用于调试和配置验证。
	Endpoints() ProviderEndpoints
}

// 编译时接口检查：确保 Provider 满足 types.ChatProvider
var _ types.ChatProvider = (Provider)(nil)

// =============================================================================
// Provider 特有类型（不在 types 层）
// =============================================================================

// HealthStatus 表示提供者的健康检查结果。
// 这是 Provider 级别的统一健康状态类型，同时被 llm.Provider 和 llm/embedding.Provider 使用。
type HealthStatus struct {
	Healthy   bool          `json:"healthy"`
	Latency   time.Duration `json:"latency"`
	ErrorRate float64       `json:"error_rate"`
	Message   string        `json:"message,omitempty"`
}

// Model 表示提供者支持的一个模型.
type Model struct {
	ID              string   `json:"id"`                          // 模型 ID（API 调用时使用）
	Object          string   `json:"object"`                      // 对象类型（通常是 "model"）
	Created         int64    `json:"created"`                     // 创建时间戳
	OwnedBy         string   `json:"owned_by"`                    // 所属组织
	Permissions     []string `json:"permissions"`                 // 权限列表
	Root            string   `json:"root"`                        // 根模型
	Parent          string   `json:"parent"`                      // 父模型
	MaxInputTokens  int      `json:"max_input_tokens,omitempty"`  // 最大输入 token 数
	MaxOutputTokens int      `json:"max_output_tokens,omitempty"` // 最大输出 token 数
	Capabilities    []string `json:"capabilities,omitempty"`      // 模型能力列表
}

// ProviderEndpoints 描述提供者使用的所有 API 端点。
type ProviderEndpoints struct {
	Completion string `json:"completion"`       // 聊天补全端点
	Stream     string `json:"stream,omitempty"` // 流式端点（如果与 Completion 不同）
	Models     string `json:"models"`           // 模型列表端点
	Health     string `json:"health,omitempty"` // 健康检查端点（如果与 Models 不同）
	BaseURL    string `json:"base_url"`         // 基础 URL
}
