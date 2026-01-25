package agent

import (
	"context"

	"github.com/BaSui01/agentflow/llm"
	llmtools "github.com/BaSui01/agentflow/llm/tools"
)

// ToolManager abstracts the "tool list + tool execution" capability for Agent runtime.
//
// Design goals:
// - Avoid pkg/agent directly depending on pkg/agent/tools (eliminate import cycle)
// - Allow different implementations to be injected at application layer (default uses tools.ToolManager)
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
