package agent

import (
	"context"

	"github.com/yourusername/agentflow/llm"
	llmtools "github.com/yourusername/agentflow/llm/tools"
)

// ToolManager 抽象 Agent 运行时的"工具列表 + 工具执行"能力。
//
// 设计目标：
// - 避免 pkg/agent 直接依赖 pkg/agent/tools（消除 import cycle）
// - 允许在 application 层注入不同实现（默认使用 tools.ToolManager）
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
