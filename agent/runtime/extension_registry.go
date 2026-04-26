package runtime

import (
	"context"
	agentcore "github.com/BaSui01/agentflow/agent/core"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
	"time"
)

type ExtensionRegistry struct {
	inner        *agentcore.ExtensionRegistry[Input, Output]
	lspClient    LSPClientRunner
	lspLifecycle LSPLifecycleOwner
}

func NewExtensionRegistry(logger *zap.Logger) *ExtensionRegistry {
	return &ExtensionRegistry{inner: agentcore.NewExtensionRegistry[Input, Output](logger)}
}

func (r *ExtensionRegistry) EnableReflection(executor ReflectionRunner) {
	r.inner.EnableReflection(executor)
}

func (r *ExtensionRegistry) EnableToolSelection(selector DynamicToolSelectorRunner) {
	r.inner.EnableToolSelection(selector)
}

func (r *ExtensionRegistry) EnablePromptEnhancer(enhancer PromptEnhancerRunner) {
	r.inner.EnablePromptEnhancer(enhancer)
}

func (r *ExtensionRegistry) EnableSkills(manager SkillDiscoverer) {
	r.inner.EnableSkills(manager)
}

func (r *ExtensionRegistry) EnableMCP(server MCPServerRunner) {
	r.inner.EnableMCP(server)
}

func (r *ExtensionRegistry) EnableLSP(client LSPClientRunner) {
	r.lspClient = client
	r.inner.EnableLSP(client)
}

func (r *ExtensionRegistry) EnableLSPWithLifecycle(client LSPClientRunner, lifecycle LSPLifecycleOwner) {
	r.lspClient = client
	r.lspLifecycle = lifecycle
	r.inner.EnableLSPWithLifecycle(client, lifecycle)
}

func (r *ExtensionRegistry) EnableEnhancedMemory(memorySystem EnhancedMemoryRunner) {
	r.inner.EnableEnhancedMemory(memorySystem)
}

func (r *ExtensionRegistry) EnableObservability(obsSystem ObservabilityRunner) {
	r.inner.EnableObservability(obsSystem)
}

func (r *ExtensionRegistry) ReflectionExecutor() ReflectionRunner {
	return r.inner.ReflectionExecutor()
}

func (r *ExtensionRegistry) ToolSelector() DynamicToolSelectorRunner { return r.inner.ToolSelector() }

func (r *ExtensionRegistry) PromptEnhancerExt() PromptEnhancerRunner {
	return r.inner.PromptEnhancerExt()
}

func (r *ExtensionRegistry) SkillManagerExt() SkillDiscoverer { return r.inner.SkillManagerExt() }

func (r *ExtensionRegistry) MCPServerExt() MCPServerRunner { return r.inner.MCPServerExt() }

func (r *ExtensionRegistry) LSPClientExt() LSPClientRunner {
	r.syncLegacyLSP()
	return r.inner.LSPClientExt()
}

func (r *ExtensionRegistry) LSPLifecycleExt() LSPLifecycleOwner {
	r.syncLegacyLSP()
	return r.inner.LSPLifecycleExt()
}

func (r *ExtensionRegistry) EnhancedMemoryExt() EnhancedMemoryRunner {
	return r.inner.EnhancedMemoryExt()
}

func (r *ExtensionRegistry) ObservabilitySystemExt() ObservabilityRunner {
	return r.inner.ObservabilitySystemExt()
}

func (r *ExtensionRegistry) GetFeatureStatus() map[string]bool {
	r.syncLegacyLSP()
	return r.inner.GetFeatureStatus()
}

func (r *ExtensionRegistry) TeardownExtensions(ctx context.Context) error {
	r.syncLegacyLSP()
	return r.inner.TeardownExtensions(ctx)
}

func (r *ExtensionRegistry) SaveToEnhancedMemory(ctx context.Context, agentID string, input *Input, output *Output, useReflection bool) {
	r.inner.SaveToEnhancedMemory(ctx, agentID, agentcore.EnhancedMemoryRecord{
		TraceID:       input.TraceID,
		Content:       output.Content,
		TokensUsed:    output.TokensUsed,
		Cost:          output.Cost,
		Duration:      output.Duration,
		UseReflection: useReflection,
		RecordedAt:    time.Now(),
	})
}

func (r *ExtensionRegistry) ValidateConfiguration(cfg types.AgentConfig) []string {
	r.syncLegacyLSP()
	return r.inner.ValidateConfiguration(cfg)
}

func (r *ExtensionRegistry) ExecuteWithReflection(ctx context.Context, input *Input) (*Output, error) {
	if r.inner.ReflectionExecutor() == nil {
		return nil, NewError(types.ErrAgentNotReady, "reflection executor not set")
	}
	return r.inner.ExecuteWithReflection(ctx, input)
}

func (r *ExtensionRegistry) syncLegacyLSP() {
	if r == nil || r.inner == nil {
		return
	}
	if r.lspLifecycle != nil && r.inner.LSPLifecycleExt() == nil {
		r.inner.EnableLSPWithLifecycle(r.lspClient, r.lspLifecycle)
		return
	}
	if r.lspClient != nil && r.inner.LSPClientExt() == nil {
		r.inner.EnableLSP(r.lspClient)
	}
}
