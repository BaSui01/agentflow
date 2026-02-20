package agent

import (
	"context"

	"github.com/BaSui01/agentflow/llm"
	llmtools "github.com/BaSui01/agentflow/llm/tools"
)

// ToolManager为Agent运行时间摘要了"工具列表+工具执行"的能力.
//
// 设计目标:
// - 直接根据pkg/剂/工具避免pkg/剂(取消进口周期)
// - 允许在应用程序层注入不同的执行(默认使用工具)。 工具管理器)
type ToolManager interface {
	GetAllowedTools(agentID string) []llm.ToolSchema
	ExecuteForAgent(ctx context.Context, agentID string, calls []llm.ToolCall) []llmtools.ToolResult
}

func filterToolSchemasByWhitelist(all []llm.ToolSchema, whitelist []string) []llm.ToolSchema {
	if len(whitelist) == 0 {
		return all
	}
	allowed := make(map[string]struct{}, len(whitelist))
	for _, name := range whitelist {
		if name == "" {
			continue
		}
		allowed[name] = struct{}{}
	}
	out := make([]llm.ToolSchema, 0, len(all))
	for _, s := range all {
		if _, ok := allowed[s.Name]; ok {
			out = append(out, s)
		}
	}
	return out
}
