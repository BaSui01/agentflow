package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// ExtensionRegistry encapsulates the 9 optional extension fields extracted from BaseAgent.
type ExtensionRegistry struct {
	reflectionExecutor  ReflectionRunner
	toolSelector        DynamicToolSelectorRunner
	promptEnhancer      PromptEnhancerRunner
	skillManager        SkillDiscoverer
	mcpServer           MCPServerRunner
	lspClient           LSPClientRunner
	lspLifecycle        LSPLifecycleOwner
	enhancedMemory      EnhancedMemoryRunner
	observabilitySystem ObservabilityRunner
	logger              *zap.Logger
}

// NewExtensionRegistry creates a new ExtensionRegistry.
func NewExtensionRegistry(logger *zap.Logger) *ExtensionRegistry {
	return &ExtensionRegistry{logger: logger}
}

// EnableReflection enables the reflection mechanism.
func (r *ExtensionRegistry) EnableReflection(executor ReflectionRunner) {
	r.reflectionExecutor = executor
	r.logger.Info("reflection enabled")
}

// EnableToolSelection enables dynamic tool selection.
func (r *ExtensionRegistry) EnableToolSelection(selector DynamicToolSelectorRunner) {
	r.toolSelector = selector
	r.logger.Info("tool selection enabled")
}

// EnablePromptEnhancer enables prompt enhancement.
func (r *ExtensionRegistry) EnablePromptEnhancer(enhancer PromptEnhancerRunner) {
	r.promptEnhancer = enhancer
	r.logger.Info("prompt enhancer enabled")
}

// EnableSkills enables the skills system.
func (r *ExtensionRegistry) EnableSkills(manager SkillDiscoverer) {
	r.skillManager = manager
	r.logger.Info("skills system enabled")
}

// EnableMCP enables MCP integration.
func (r *ExtensionRegistry) EnableMCP(server MCPServerRunner) {
	r.mcpServer = server
	r.logger.Info("MCP integration enabled")
}

// EnableLSP enables LSP integration.
func (r *ExtensionRegistry) EnableLSP(client LSPClientRunner) {
	r.lspClient = client
	r.logger.Info("LSP integration enabled")
}

// EnableLSPWithLifecycle enables LSP with an optional lifecycle owner.
func (r *ExtensionRegistry) EnableLSPWithLifecycle(client LSPClientRunner, lifecycle LSPLifecycleOwner) {
	r.lspClient = client
	r.lspLifecycle = lifecycle
	r.logger.Info("LSP integration enabled with lifecycle")
}

// EnableEnhancedMemory enables the enhanced memory system.
func (r *ExtensionRegistry) EnableEnhancedMemory(memorySystem EnhancedMemoryRunner) {
	r.enhancedMemory = memorySystem
	r.logger.Info("enhanced memory enabled")
}

// EnableObservability enables the observability system.
func (r *ExtensionRegistry) EnableObservability(obsSystem ObservabilityRunner) {
	r.observabilitySystem = obsSystem
	r.logger.Info("observability enabled")
}

// ReflectionExecutor returns the reflection runner.
func (r *ExtensionRegistry) ReflectionExecutor() ReflectionRunner { return r.reflectionExecutor }

// ToolSelector returns the tool selector runner.
func (r *ExtensionRegistry) ToolSelector() DynamicToolSelectorRunner { return r.toolSelector }

// PromptEnhancerExt returns the prompt enhancer runner.
func (r *ExtensionRegistry) PromptEnhancerExt() PromptEnhancerRunner { return r.promptEnhancer }

// SkillManagerExt returns the skill discoverer.
func (r *ExtensionRegistry) SkillManagerExt() SkillDiscoverer { return r.skillManager }

// MCPServerExt returns the MCP server runner.
func (r *ExtensionRegistry) MCPServerExt() MCPServerRunner { return r.mcpServer }

// LSPClientExt returns the LSP client runner.
func (r *ExtensionRegistry) LSPClientExt() LSPClientRunner { return r.lspClient }

// LSPLifecycleExt returns the LSP lifecycle owner.
func (r *ExtensionRegistry) LSPLifecycleExt() LSPLifecycleOwner { return r.lspLifecycle }

// EnhancedMemoryExt returns the enhanced memory runner.
func (r *ExtensionRegistry) EnhancedMemoryExt() EnhancedMemoryRunner { return r.enhancedMemory }

// ObservabilitySystemExt returns the observability runner.
func (r *ExtensionRegistry) ObservabilitySystemExt() ObservabilityRunner {
	return r.observabilitySystem
}

// GetFeatureStatus returns a map of feature name to enabled status.
func (r *ExtensionRegistry) GetFeatureStatus() map[string]bool {
	return map[string]bool{
		"reflection":      r.reflectionExecutor != nil,
		"tool_selection":  r.toolSelector != nil,
		"prompt_enhancer": r.promptEnhancer != nil,
		"skills":          r.skillManager != nil,
		"mcp":             r.mcpServer != nil,
		"lsp":             r.lspClient != nil,
		"enhanced_memory": r.enhancedMemory != nil,
		"observability":   r.observabilitySystem != nil,
	}
}

// TeardownExtensions cleans up extension resources.
func (r *ExtensionRegistry) TeardownExtensions(ctx context.Context) error {
	if r.lspLifecycle != nil {
		if err := r.lspLifecycle.Close(); err != nil {
			r.logger.Warn("failed to close lsp lifecycle", zap.Error(err))
		}
		return nil
	}
	if r.lspClient != nil {
		if err := r.lspClient.Shutdown(ctx); err != nil {
			r.logger.Warn("failed to shutdown lsp client", zap.Error(err))
		}
	}
	return nil
}

// SaveToEnhancedMemory saves output to enhanced memory and records an episode.
func (r *ExtensionRegistry) SaveToEnhancedMemory(ctx context.Context, agentID string, input *Input, output *Output, useReflection bool) {
	if r.enhancedMemory == nil {
		return
	}
	metadata := map[string]any{
		"trace_id": input.TraceID,
		"tokens":   output.TokensUsed,
		"cost":     output.Cost,
	}
	if err := r.enhancedMemory.SaveShortTerm(ctx, agentID, output.Content, metadata); err != nil {
		r.logger.Warn("failed to save short-term memory", zap.Error(err))
	}
	event := &types.EpisodicEvent{
		ID:        fmt.Sprintf("%s-%d", agentID, time.Now().UnixNano()),
		AgentID:   agentID,
		Type:      "task_execution",
		Content:   output.Content,
		Timestamp: time.Now(),
		Duration:  output.Duration,
		Context: map[string]any{
			"trace_id":   input.TraceID,
			"tokens":     output.TokensUsed,
			"cost":       output.Cost,
			"reflection": useReflection,
		},
	}
	if err := r.enhancedMemory.RecordEpisode(ctx, event); err != nil {
		r.logger.Warn("failed to record episode", zap.Error(err))
	}
}

// ValidateConfiguration validates that enabled features have their executors set.
func (r *ExtensionRegistry) ValidateConfiguration(cfg types.AgentConfig) []string {
	var errors []string
	if cfg.IsReflectionEnabled() && r.reflectionExecutor == nil {
		errors = append(errors, "reflection enabled but executor not set")
	}
	if cfg.IsToolSelectionEnabled() && r.toolSelector == nil {
		errors = append(errors, "tool selection enabled but selector not set")
	}
	if cfg.IsPromptEnhancerEnabled() && r.promptEnhancer == nil {
		errors = append(errors, "prompt enhancer enabled but enhancer not set")
	}
	if cfg.IsSkillsEnabled() && r.skillManager == nil {
		errors = append(errors, "skills enabled but manager not set")
	}
	if cfg.IsMCPEnabled() && r.mcpServer == nil {
		errors = append(errors, "MCP enabled but server not set")
	}
	if cfg.IsLSPEnabled() && r.lspClient == nil {
		errors = append(errors, "LSP enabled but client not set")
	}
	if cfg.IsMemoryEnabled() && r.enhancedMemory == nil {
		errors = append(errors, "enhanced memory enabled but system not set")
	}
	if cfg.IsObservabilityEnabled() && r.observabilitySystem == nil {
		errors = append(errors, "observability enabled but system not set")
	}
	return errors
}

// ExecuteWithReflection delegates to the reflection executor.
func (r *ExtensionRegistry) ExecuteWithReflection(ctx context.Context, input *Input) (*Output, error) {
	if r.reflectionExecutor == nil {
		return nil, NewError(types.ErrAgentNotReady, "reflection executor not set")
	}
	return r.reflectionExecutor.ExecuteWithReflection(ctx, input)
}
