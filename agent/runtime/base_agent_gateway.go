package runtime

import (
	"context"
	"time"
	llmtools "github.com/BaSui01/agentflow/llm/capabilities/tools"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	llmgateway "github.com/BaSui01/agentflow/llm/gateway"
	observability "github.com/BaSui01/agentflow/llm/observability"
	types "github.com/BaSui01/agentflow/types"
	zap "go.uber.org/zap"
)

// toolManagerExecutor is a pure delegator with event publishing.
// Whitelist filtering is handled upstream in prepareChatRequest, so this
// executor no longer duplicates that logic.
type toolManagerExecutor struct {
	mgr     ToolManager
	agentID string
	bus     EventBus
}

func newToolManagerExecutor(mgr ToolManager, agentID string, _ []string, bus EventBus) toolManagerExecutor {
	return toolManagerExecutor{mgr: mgr, agentID: agentID, bus: bus}
}

func (e toolManagerExecutor) Execute(ctx context.Context, calls []types.ToolCall) []llmtools.ToolResult {
	traceID, _ := types.TraceID(ctx)
	runID, _ := types.RunID(ctx)
	promptVer, _ := types.PromptBundleVersion(ctx)

	publish := func(stage string, call types.ToolCall, errMsg string) {
		if e.bus == nil {
			return
		}
		e.bus.Publish(&ToolCallEvent{
			AgentID_:            e.agentID,
			RunID:               runID,
			TraceID:             traceID,
			PromptBundleVersion: promptVer,
			ToolCallID:          call.ID,
			ToolName:            call.Name,
			Stage:               stage,
			Error:               errMsg,
			Timestamp_:          time.Now(),
		})
	}

	for _, c := range calls {
		publish("start", c, "")
	}

	if e.mgr == nil {
		out := make([]llmtools.ToolResult, len(calls))
		for i, c := range calls {
			out[i] = llmtools.ToolResult{ToolCallID: c.ID, Name: c.Name, Error: "tool manager not configured"}
			publish("end", c, out[i].Error)
		}
		return out
	}

	results := e.mgr.ExecuteForAgent(ctx, e.agentID, calls)
	for i, c := range calls {
		errMsg := ""
		if i < len(results) {
			errMsg = results[i].Error
		}
		publish("end", c, errMsg)
	}
	return results
}

func (e toolManagerExecutor) ExecuteOne(ctx context.Context, call types.ToolCall) llmtools.ToolResult {
	res := e.Execute(ctx, []types.ToolCall{call})
	if len(res) == 0 {
		return llmtools.ToolResult{ToolCallID: call.ID, Name: call.Name, Error: "no tool result"}
	}
	return res[0]
}
// MainGateway 返回主请求链路使用的 gateway。
func (b *BaseAgent) MainGateway() llmcore.Gateway {
	if b == nil {
		return nil
	}
	return b.mainGateway
}

func (b *BaseAgent) hasMainExecutionSurface() bool {
	return b != nil && b.MainGateway() != nil
}

func (b *BaseAgent) hasDedicatedToolExecutionSurface() bool {
	if b == nil {
		return false
	}
	return b.toolGateway != nil
}

// ToolGateway 返回工具调用链路使用的 gateway（未配置时回退到主 gateway）。
func (b *BaseAgent) ToolGateway() llmcore.Gateway {
	if b == nil {
		return nil
	}
	if b.toolGateway == nil {
		return b.MainGateway()
	}
	return b.toolGateway
}

// SetToolGateway injects a pre-built shared tool gateway.
func (b *BaseAgent) SetToolGateway(gw llmcore.Gateway) {
	b.toolGateway = gw
	b.toolProviderCompat = compatProviderFromGateway(gw)
	b.toolGatewayProvider = nil
}

// SetGateway injects a pre-built shared Gateway instance.
func (b *BaseAgent) SetGateway(gw llmcore.Gateway) {
	b.mainGateway = gw
	b.mainProviderCompat = compatProviderFromGateway(gw)
	b.gatewayProviderCache = nil
}

func (b *BaseAgent) gatewayProvider() llmcore.Provider {
	gateway := b.MainGateway()
	if gateway != nil {
		if b.gatewayProviderCache != nil {
			return b.gatewayProviderCache
		}
		return llmgateway.NewChatProviderAdapter(gateway, b.mainProviderCompat)
	}
	return nil
}

func (b *BaseAgent) gatewayToolProvider() llmcore.Provider {
	if b.hasDedicatedToolExecutionSurface() {
		toolGateway := b.ToolGateway()
		if toolGateway != nil {
			if b.toolGatewayProvider != nil {
				return b.toolGatewayProvider
			}
			return llmgateway.NewChatProviderAdapter(toolGateway, b.toolProviderCompat)
		}
	}
	return b.gatewayProvider()
}

type providerBackedGateway interface {
	ChatProvider() llmcore.Provider
}

func compatProviderFromGateway(gateway llmcore.Gateway) llmcore.Provider {
	if gateway == nil {
		return nil
	}
	backed, ok := gateway.(providerBackedGateway)
	if !ok {
		return nil
	}
	return backed.ChatProvider()
}

func wrapProviderWithGateway(provider llmcore.Provider, logger *zap.Logger, ledger observability.Ledger) llmcore.Gateway {
	if provider == nil {
		return nil
	}
	return llmgateway.New(llmgateway.Config{
		ChatProvider: provider,
		Ledger:       ledger,
		Logger:       logger,
	})
}
