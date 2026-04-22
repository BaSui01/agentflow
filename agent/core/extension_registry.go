package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

type ReflectionRunner[InputT any, OutputT any] interface {
	ExecuteWithReflection(ctx context.Context, input *InputT) (*OutputT, error)
}

type DynamicToolSelectorRunner interface {
	SelectTools(ctx context.Context, task string, availableTools []types.ToolSchema) ([]types.ToolSchema, error)
}

type PromptEnhancerRunner interface {
	EnhanceUserPrompt(prompt, context string) (string, error)
}

type SkillDiscoverer interface {
	DiscoverSkills(ctx context.Context, task string) ([]*types.DiscoveredSkill, error)
}

type MCPServerRunner interface{}

type LSPClientRunner interface {
	Shutdown(ctx context.Context) error
}

type LSPLifecycleOwner interface {
	Close() error
}

type EnhancedMemoryRunner interface {
	LoadWorking(ctx context.Context, agentID string) ([]types.MemoryEntry, error)
	LoadShortTerm(ctx context.Context, agentID string, limit int) ([]types.MemoryEntry, error)
	SaveShortTerm(ctx context.Context, agentID, content string, metadata map[string]any) error
	RecordEpisode(ctx context.Context, event *types.EpisodicEvent) error
}

type EnhancedMemoryRecord struct {
	TraceID       string
	Content       string
	TokensUsed    int
	Cost          float64
	Duration      time.Duration
	UseReflection bool
	RecordedAt    time.Time
}

var errReflectionExecutorNotSet = errors.New("reflection executor not set")

// ExtensionRegistry encapsulates the optional extension fields extracted from BaseAgent.
type ExtensionRegistry[InputT any, OutputT any] struct {
	reflectionExecutor  ReflectionRunner[InputT, OutputT]
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

func NewExtensionRegistry[InputT any, OutputT any](logger *zap.Logger) *ExtensionRegistry[InputT, OutputT] {
	return &ExtensionRegistry[InputT, OutputT]{logger: logger}
}

func (r *ExtensionRegistry[InputT, OutputT]) EnableReflection(executor ReflectionRunner[InputT, OutputT]) {
	r.reflectionExecutor = executor
	r.logger.Info("reflection enabled")
}

func (r *ExtensionRegistry[InputT, OutputT]) EnableToolSelection(selector DynamicToolSelectorRunner) {
	r.toolSelector = selector
	r.logger.Info("tool selection enabled")
}

func (r *ExtensionRegistry[InputT, OutputT]) EnablePromptEnhancer(enhancer PromptEnhancerRunner) {
	r.promptEnhancer = enhancer
	r.logger.Info("prompt enhancer enabled")
}

func (r *ExtensionRegistry[InputT, OutputT]) EnableSkills(manager SkillDiscoverer) {
	r.skillManager = manager
	r.logger.Info("skills system enabled")
}

func (r *ExtensionRegistry[InputT, OutputT]) EnableMCP(server MCPServerRunner) {
	r.mcpServer = server
	r.logger.Info("MCP integration enabled")
}

func (r *ExtensionRegistry[InputT, OutputT]) EnableLSP(client LSPClientRunner) {
	r.lspClient = client
	r.logger.Info("LSP integration enabled")
}

func (r *ExtensionRegistry[InputT, OutputT]) EnableLSPWithLifecycle(client LSPClientRunner, lifecycle LSPLifecycleOwner) {
	r.lspClient = client
	r.lspLifecycle = lifecycle
	r.logger.Info("LSP integration enabled with lifecycle")
}

func (r *ExtensionRegistry[InputT, OutputT]) EnableEnhancedMemory(memorySystem EnhancedMemoryRunner) {
	r.enhancedMemory = memorySystem
	r.logger.Info("enhanced memory enabled")
}

func (r *ExtensionRegistry[InputT, OutputT]) EnableObservability(obsSystem ObservabilityRunner) {
	r.observabilitySystem = obsSystem
	r.logger.Info("observability enabled")
}

func (r *ExtensionRegistry[InputT, OutputT]) ReflectionExecutor() ReflectionRunner[InputT, OutputT] {
	return r.reflectionExecutor
}

func (r *ExtensionRegistry[InputT, OutputT]) ToolSelector() DynamicToolSelectorRunner {
	return r.toolSelector
}

func (r *ExtensionRegistry[InputT, OutputT]) PromptEnhancerExt() PromptEnhancerRunner {
	return r.promptEnhancer
}

func (r *ExtensionRegistry[InputT, OutputT]) SkillManagerExt() SkillDiscoverer { return r.skillManager }

func (r *ExtensionRegistry[InputT, OutputT]) MCPServerExt() MCPServerRunner { return r.mcpServer }

func (r *ExtensionRegistry[InputT, OutputT]) LSPClientExt() LSPClientRunner { return r.lspClient }

func (r *ExtensionRegistry[InputT, OutputT]) LSPLifecycleExt() LSPLifecycleOwner {
	return r.lspLifecycle
}

func (r *ExtensionRegistry[InputT, OutputT]) EnhancedMemoryExt() EnhancedMemoryRunner {
	return r.enhancedMemory
}

func (r *ExtensionRegistry[InputT, OutputT]) ObservabilitySystemExt() ObservabilityRunner {
	return r.observabilitySystem
}

func (r *ExtensionRegistry[InputT, OutputT]) GetFeatureStatus() map[string]bool {
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

func (r *ExtensionRegistry[InputT, OutputT]) TeardownExtensions(ctx context.Context) error {
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

func (r *ExtensionRegistry[InputT, OutputT]) SaveToEnhancedMemory(ctx context.Context, agentID string, record EnhancedMemoryRecord) {
	if r.enhancedMemory == nil {
		return
	}
	recordedAt := record.RecordedAt
	if recordedAt.IsZero() {
		recordedAt = time.Now()
	}
	metadata := map[string]any{
		"trace_id": record.TraceID,
		"tokens":   record.TokensUsed,
		"cost":     record.Cost,
	}
	if err := r.enhancedMemory.SaveShortTerm(ctx, agentID, record.Content, metadata); err != nil {
		r.logger.Warn("failed to save short-term memory", zap.Error(err))
	}
	event := &types.EpisodicEvent{
		ID:        fmt.Sprintf("%s-%d", agentID, recordedAt.UnixNano()),
		AgentID:   agentID,
		Type:      "task_execution",
		Content:   record.Content,
		Timestamp: recordedAt,
		Duration:  record.Duration,
		Context: map[string]any{
			"trace_id":   record.TraceID,
			"tokens":     record.TokensUsed,
			"cost":       record.Cost,
			"reflection": record.UseReflection,
		},
	}
	if err := r.enhancedMemory.RecordEpisode(ctx, event); err != nil {
		r.logger.Warn("failed to record episode", zap.Error(err))
	}
}

func (r *ExtensionRegistry[InputT, OutputT]) ValidateConfiguration(cfg types.AgentConfig) []string {
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

func (r *ExtensionRegistry[InputT, OutputT]) ExecuteWithReflection(ctx context.Context, input *InputT) (*OutputT, error) {
	if r.reflectionExecutor == nil {
		return nil, errReflectionExecutorNotSet
	}
	return r.reflectionExecutor.ExecuteWithReflection(ctx, input)
}
