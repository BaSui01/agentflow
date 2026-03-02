package core

import "context"

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
type ToolRegistry interface {
	ExecuteTool(ctx context.Context, name string, params map[string]any) (any, error)
}

// HumanInputHandler 人工输入处理器抽象。
type HumanInputHandler interface {
	RequestInput(ctx context.Context, prompt string, inputType string, options []string) (any, error)
}
