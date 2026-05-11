package runtime

import (
	"context"
	"strings"

	toolcap "github.com/BaSui01/agentflow/agent/capabilities/tools"
	llmtools "github.com/BaSui01/agentflow/llm/capabilities/tools"
	"github.com/BaSui01/agentflow/types"
)

const (
	toolRiskSafeRead         = toolcap.ToolRiskSafeRead
	toolRiskRequiresApproval = toolcap.ToolRiskRequiresApproval
	toolRiskUnknown          = toolcap.ToolRiskUnknown
)

func classifyToolRiskByName(name string) string {
	return toolcap.ClassifyToolRiskByName(name)
}

func groupToolRisks(names []string) map[string][]string {
	return toolcap.GroupToolRisks(normalizeStringSlice(names))
}

// PreparedToolProtocol is the runtime-resolved tool execution bundle consumed
// by completion flows.
type PreparedToolProtocol struct {
	Executor     llmtools.ToolExecutor
	HandoffTools map[string]RuntimeHandoffTarget
	ToolRisks    map[string]string
	AllowedTools []string
	Authorize    AuthorizeFunc
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
	if prepared.Authorize != nil {
		return executeAuthorizedToolCalls(ctx, prepared, calls)
	}
	return prepared.Executor.Execute(ctx, calls)
}

func executeAuthorizedToolCalls(ctx context.Context, prepared *PreparedToolProtocol, calls []types.ToolCall) []types.ToolResult {
	if len(calls) == 0 {
		return nil
	}
	out := make([]types.ToolResult, 0, len(calls))
	authz := NewAuthzMiddleware(prepared.Authorize)
	for _, call := range calls {
		result, err := authz.Execute(ctx, &toolAuthorizationInput{
			ToolCall:  &call,
			ToolRisks: prepared.ToolRisks,
		})
		if err != nil {
			out = append(out, types.ToolResult{ToolCallID: call.ID, Name: call.Name, Error: err.Error()})
			continue
		}
		if result.Action == HookActionAbort {
			out = append(out, types.ToolResult{ToolCallID: call.ID, Name: call.Name, Error: result.Reason})
			continue
		}
		out = append(out, prepared.Executor.ExecuteOne(ctx, call))
	}
	return out
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

func toolRiskForPreparedRequest(pr *preparedRequest, toolName string, metadata map[string]string) string {
	if metadata != nil {
		if risk := strings.TrimSpace(metadata["hosted_tool_risk"]); risk != "" {
			return risk
		}
	}
	if pr != nil && len(pr.toolRisks) > 0 {
		if risk, ok := pr.toolRisks[strings.TrimSpace(toolName)]; ok {
			return risk
		}
	}
	return classifyToolRiskByName(toolName)
}
