package agent

import (
	"context"

	llmtools "github.com/BaSui01/agentflow/llm/capabilities/tools"
	"github.com/BaSui01/agentflow/types"
)

// PreparedToolProtocol is the runtime-resolved tool execution bundle consumed
// by completion flows.
type PreparedToolProtocol struct {
	Executor     llmtools.ToolExecutor
	HandoffTools map[string]RuntimeHandoffTarget
	ToolRisks    map[string]string
	AllowedTools []string
}

// ToolProtocolRuntime resolves the tool execution contract for a prepared request.
type ToolProtocolRuntime interface {
	Prepare(owner *BaseAgent, pr *preparedRequest) *PreparedToolProtocol
	Execute(ctx context.Context, prepared *PreparedToolProtocol, calls []types.ToolCall) []types.ToolResult
	ToMessages(results []types.ToolResult) []types.Message
}

// DefaultToolProtocolRuntime preserves the current runtime behavior while
// centralizing handoff + tool manager orchestration behind a single interface.
type DefaultToolProtocolRuntime struct{}

func NewDefaultToolProtocolRuntime() ToolProtocolRuntime {
	return DefaultToolProtocolRuntime{}
}

func (DefaultToolProtocolRuntime) Prepare(owner *BaseAgent, pr *preparedRequest) *PreparedToolProtocol {
	if pr == nil || owner == nil {
		return &PreparedToolProtocol{
			Executor: toolManagerExecutor{},
		}
	}
	allowed := append([]string(nil), pr.options.Tools.AllowedTools...)
	base := newToolManagerExecutor(owner.toolManager, owner.config.Core.ID, allowed, owner.bus)
	executor := llmtools.ToolExecutor(base)
	if len(pr.handoffTools) > 0 {
		targets := make([]RuntimeHandoffTarget, 0, len(pr.handoffTools))
		for _, target := range pr.handoffTools {
			targets = append(targets, target)
		}
		executor = newRuntimeHandoffExecutor(owner, base, targets)
	}
	return &PreparedToolProtocol{
		Executor:     executor,
		HandoffTools: cloneRuntimeHandoffMap(pr.handoffTools),
		ToolRisks:    cloneStringMap(pr.toolRisks),
		AllowedTools: allowed,
	}
}

func (DefaultToolProtocolRuntime) Execute(ctx context.Context, prepared *PreparedToolProtocol, calls []types.ToolCall) []types.ToolResult {
	if prepared == nil || prepared.Executor == nil {
		return nil
	}
	return prepared.Executor.Execute(ctx, calls)
}

func (DefaultToolProtocolRuntime) ToMessages(results []types.ToolResult) []types.Message {
	if len(results) == 0 {
		return nil
	}
	out := make([]types.Message, 0, len(results))
	for _, result := range results {
		out = append(out, result.ToMessage())
	}
	return out
}

func cloneRuntimeHandoffMap(in map[string]RuntimeHandoffTarget) map[string]RuntimeHandoffTarget {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]RuntimeHandoffTarget, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
