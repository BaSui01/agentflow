package core

import (
	"context"
	"time"
)

// GatewayLike 是 LLM Gateway 的抽象接口。
// workflow 层通过此接口调用 LLM，不直接依赖 llm.Provider。
type GatewayLike interface {
	Invoke(ctx context.Context, req *LLMRequest) (*LLMResponse, error)
}

// LLMRequest 是 workflow 层面的 LLM 请求。
type LLMRequest struct {
	Model       string
	Prompt      string
	Temperature float64
	MaxTokens   int
	Metadata    map[string]string
}

// LLMResponse 是 workflow 层面的 LLM 响应。
type LLMResponse struct {
	Content string
	Model   string
	Usage   *LLMUsage
}

// LLMUsage 记录 token 用量。
type LLMUsage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// ToolRegistry 工具注册表抽象。
// 返回值保留 any 因为工具结果是异构的，不同工具返回不同结构。
type ToolRegistry interface {
	ExecuteTool(ctx context.Context, name string, params map[string]any) (any, error)
}

// HumanInputResult 人工输入的结构化返回值。
type HumanInputResult struct {
	Value    string `json:"value"`
	OptionID string `json:"option_id,omitempty"`
}

// HumanInputHandler 人工输入处理器抽象。
type HumanInputHandler interface {
	RequestInput(ctx context.Context, prompt string, inputType string, options []string) (*HumanInputResult, error)
}

// AgentExecutionOutput carries structured execution results from an agent step,
// eliminating type assertions for metadata extraction.
type AgentExecutionOutput struct {
	Content      string        `json:"content"`
	TokensUsed   int           `json:"tokens_used,omitempty"`
	Cost         float64       `json:"cost,omitempty"`
	Duration     time.Duration `json:"duration,omitempty"`
	FinishReason string        `json:"finish_reason,omitempty"`
}

// AgentExecutor workflow 侧 agent 执行抽象。
type AgentExecutor interface {
	Execute(ctx context.Context, input map[string]any) (*AgentExecutionOutput, error)
}
