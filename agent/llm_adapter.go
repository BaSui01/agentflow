package agent

import (
	"context"

	"github.com/BaSui01/agentflow/types"
	"github.com/BaSui01/agentflow/llm"
	llmtools "github.com/BaSui01/agentflow/llm/capabilities/tools"
)

// =============================================================================
// LLM Provider Adapter - 将 llm.Provider 适配为 types.ChatProvider
// =============================================================================
// 这个文件提供 agent 层与 llm 层之间的桥接，确保 agent 层只依赖 types 接口。
// llm.Provider 自动满足 types.ChatProvider 接口（duck typing）。
// =============================================================================

// ChatProviderAlias 是 types.ChatProvider 的别名，方便 agent 层使用。
type ChatProvider = types.ChatProvider

// ChatRequestAlias 是 types.ChatRequest 的别名。
type ChatRequest = types.ChatRequest

// ChatResponseAlias 是 types.ChatResponse 的别名。
type ChatResponse = types.ChatResponse

// StreamChunkAlias 是 types.StreamChunk 的别名。
type StreamChunk = types.StreamChunk

// ProviderAdapter 将 llm.Provider 包装为 types.ChatProvider。
// 由于 llm.Provider 已经满足 types.ChatProvider 接口，这个适配器主要用于类型转换。
type ProviderAdapter struct {
	Provider llm.Provider
}

// NewProviderAdapter 创建一个 ProviderAdapter。
func NewProviderAdapter(p llm.Provider) *ProviderAdapter {
	return &ProviderAdapter{Provider: p}
}

// 编译时接口检查：确保 llm.Provider 满足 types.ChatProvider
var _ types.ChatProvider = (llm.Provider)(nil)

// =============================================================================
// Tool Result Adapter - 工具执行结果适配
// =============================================================================

// ToolResultAlias 是 types.ToolResult 的别名。
type ToolResult = types.ToolResult

// ToolExecutorAdapter 适配 llmtools.ToolExecutor 接口。
type ToolExecutorAdapter struct {
	Executor llmtools.ToolExecutor
}

// Execute 批量执行工具调用。
func (a *ToolExecutorAdapter) Execute(ctx context.Context, calls []types.ToolCall) []types.ToolResult {
	if a.Executor == nil {
		return nil
	}
	// llmtools.ToolResult = types.ToolResult（通过类型别名）
	results := a.Executor.Execute(ctx, calls)
	out := make([]types.ToolResult, len(results))
	for i, r := range results {
		out[i] = types.ToolResult{
			ToolCallID: r.ToolCallID,
			Name:       r.Name,
			Result:     r.Result,
			Error:      r.Error,
			Duration:   r.Duration,
			FromCache:  r.FromCache,
		}
	}
	return out
}

// ExecuteOne 执行单个工具调用。
func (a *ToolExecutorAdapter) ExecuteOne(ctx context.Context, call types.ToolCall) types.ToolResult {
	if a.Executor == nil {
		return types.ToolResult{ToolCallID: call.ID, Name: call.Name, Error: "executor not configured"}
	}
	r := a.Executor.ExecuteOne(ctx, call)
	return types.ToolResult{
		ToolCallID: r.ToolCallID,
		Name:       r.Name,
		Result:     r.Result,
		Error:      r.Error,
		Duration:   r.Duration,
		FromCache:  r.FromCache,
	}
}
