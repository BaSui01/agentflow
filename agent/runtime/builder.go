package runtime

import (
	"context"
	"fmt"
	"strings"

	"github.com/BaSui01/agentflow/agent"
	agentcontext "github.com/BaSui01/agentflow/agent/context"
	agentlsp "github.com/BaSui01/agentflow/agent/lsp"
	"github.com/BaSui01/agentflow/agent/memory"
	agentobs "github.com/BaSui01/agentflow/agent/observability"
	mcpproto "github.com/BaSui01/agentflow/agent/protocol/mcp"
	"github.com/BaSui01/agentflow/agent/reasoning"
	"github.com/BaSui01/agentflow/agent/skills"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	llmobs "github.com/BaSui01/agentflow/llm/observability"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// 为可选子系统构建可选控件外接线.
type BuildOptions struct {
	EnableAll bool

	EnableReflection     bool
	EnableToolSelection  bool
	EnablePromptEnhancer bool
	EnableSkills         bool
	EnableMCP            bool
	EnableLSP            bool
	EnableEnhancedMemory bool
	EnableObservability  bool

	SkillsDirectory string
	SkillsConfig    *skills.SkillManagerConfig

	MCPServerName    string
	MCPServerVersion string
	LSPServerName    string
	LSPServerVersion string

	EnhancedMemoryConfig *memory.EnhancedMemoryConfig

	// 设定时使用可观察性系统代替默认执行.
	ObservabilitySystem agent.ObservabilityRunner

	// Optional pass-throughs for AgentBuilder runtime controls.
	MaxReActIterations int
	MaxLoopIterations  int
	MaxConcurrency     int
	MemoryManager      agent.MemoryManager
	ToolManager        agent.ToolManager
	RetrievalProvider  agent.RetrievalProvider
	ToolStateProvider  agent.ToolStateProvider
	EventBus           agent.EventBus
	LSPClient          agent.LSPClientRunner

	// Optional pass-throughs for AgentBuilder advanced wiring.
	PromptStore       agent.PromptStoreProvider
	ConversationStore agent.ConversationStoreProvider
	RunStore          agent.RunStoreProvider
	CheckpointManager *agent.CheckpointManager
	Orchestrator      agent.OrchestratorRunner
	ReasoningRegistry *reasoning.PatternRegistry
	// ReasoningExposure controls which non-default reasoning strategies are
	// registered into the runtime when ReasoningRegistry is not provided.
	ReasoningExposure agent.ReasoningExposureLevel

	// InitAgent在接线后呼叫Init(ctx).
	InitAgent bool
}

func DefaultBuildOptions() BuildOptions {
	return BuildOptions{
		EnableAll:            true,
		EnableReflection:     true,
		EnableToolSelection:  true,
		EnablePromptEnhancer: true,
		EnableSkills:         true,
		EnableMCP:            true,
		EnableLSP:            true,
		EnableEnhancedMemory: true,
		EnableObservability:  true,
		SkillsDirectory:      "./skills",
		MCPServerName:        "agentflow-mcp",
		MCPServerVersion:     "0.1.0",
		LSPServerName:        "agentflow-lsp",
		LSPServerVersion:     "0.1.0",
		ReasoningExposure:    agent.ReasoningExposureOfficial,
		InitAgent:            false,
	}
}

func enabled(all bool, v bool) bool { return all || v }

// Builder 是 agent/runtime 的唯一构建入口。
type Builder struct {
	gateway     llmcore.Gateway
	toolGateway llmcore.Gateway
	ledger      llmobs.Ledger
	logger      *zap.Logger
	options     BuildOptions
	toolScope   []string // tool whitelist for sub-agent isolation (empty = all tools)
}

var defaultRuntimeReasoningModes = []string{}

// NewBuilder 创建 runtime builder。logger 为必选参数，nil 时 panic。
func NewBuilder(gateway llmcore.Gateway, logger *zap.Logger) *Builder {
	if logger == nil {
		panic("runtime.Builder: logger is required and cannot be nil")
	}
	return &Builder{
		gateway: gateway,
		logger:  logger,
		options: BuildOptions{},
	}
}

// WithOptions 设置构建选项。
func (b *Builder) WithOptions(opts BuildOptions) *Builder {
	b.options = opts
	return b
}

// WithToolGateway 设置工具调用专用 Gateway（双模型模式）。
// 未设置时，工具调用退化使用主 gateway。
func (b *Builder) WithToolGateway(gateway llmcore.Gateway) *Builder {
	b.toolGateway = gateway
	return b
}

// WithLedger 设置 cost/usage 落账器。
func (b *Builder) WithLedger(ledger llmobs.Ledger) *Builder {
	b.ledger = ledger
	return b
}

// WithToolScope limits the tools available to the built agent.
// Only tools whose names appear in the whitelist will be accessible.
// An empty list means all tools are available (no restriction).
func (b *Builder) WithToolScope(toolNames []string) *Builder {
	b.toolScope = toolNames
	return b
}

// Build 构造一个 BaseAgent 并按选项接线可选子系统。
func (b *Builder) Build(ctx context.Context, cfg types.AgentConfig) (*agent.BaseAgent, error) {
	opts := b.options
	if b.logger == nil {
		panic("runtime.Builder.Build: logger is required and cannot be nil")
	}
	if b.gateway == nil {
		return nil, agent.ErrProviderNotSet
	}
	if strings.TrimSpace(cfg.LLM.Model) == "" {
		return nil, agent.NewError(types.ErrInputValidation, "config.Model is required")
	}

	cfg2 := cfg
	// Apply tool scope restriction for sub-agent isolation
	if len(b.toolScope) > 0 {
		cfg2.Runtime.Tools = b.toolScope
	}
	obsEnabled := enabled(opts.EnableAll, opts.EnableObservability)
	if cfg2.Extensions.Observability == nil {
		cfg2.Extensions.Observability = &types.ObservabilityConfig{}
	}
	cfg2.Extensions.Observability.Enabled = obsEnabled
	if opts.MaxReActIterations > 0 {
		cfg2.Runtime.MaxReActIterations = opts.MaxReActIterations
	}
	if opts.MaxLoopIterations > 0 {
		cfg2.Runtime.MaxLoopIterations = opts.MaxLoopIterations
	}

	ag := agent.NewBaseAgent(
		cfg2,
		b.gateway,
		opts.MemoryManager,
		opts.ToolManager,
		opts.EventBus,
		b.logger,
		b.ledger,
	)
	if b.toolGateway != nil {
		ag.SetToolGateway(b.toolGateway)
	}
	if opts.MaxConcurrency > 0 {
		ag.SetMaxConcurrency(opts.MaxConcurrency)
	}
	ag.SetPromptStore(opts.PromptStore)
	ag.SetConversationStore(opts.ConversationStore)
	ag.SetRunStore(opts.RunStore)
	manager := agent.ContextManager(nil)
	cfgContext := agentcontext.ConfigFromAgentConfig(ag.Config())
	if cfgContext.Enabled {
		manager = agentcontext.NewAgentContextManager(cfgContext, b.logger)
	}
	ag.SetContextManager(manager)
	ag.SetRetrievalProvider(opts.RetrievalProvider)
	ag.SetToolStateProvider(opts.ToolStateProvider)

	if enabled(opts.EnableAll, opts.EnableReflection) {
		reflectionConfig := reflectionConfigFromTypes(cfg2.Features.Reflection)
		reflectionExecutor := agent.NewReflectionExecutor(ag, reflectionConfig)
		ag.EnableReflection(agent.AsReflectionRunner(reflectionExecutor))
	}
	if enabled(opts.EnableAll, opts.EnableToolSelection) {
		toolSelectionConfig := toolSelectionConfigFromTypes(cfg2.Features.ToolSelection)
		toolSelector := agent.NewDynamicToolSelector(ag, toolSelectionConfig)
		ag.EnableToolSelection(agent.AsToolSelectorRunner(toolSelector))
	}
	if enabled(opts.EnableAll, opts.EnablePromptEnhancer) {
		promptEnhancerConfig := promptEnhancerConfigFromTypes(cfg2.Features.PromptEnhancer)
		promptEnhancer := agent.NewPromptEnhancer(promptEnhancerConfig)
		ag.EnablePromptEnhancer(agent.AsPromptEnhancerRunner(promptEnhancer))
	}
	if enabled(opts.EnableAll, opts.EnableSkills) {
		dir := strings.TrimSpace(opts.SkillsDirectory)
		cfgSkills := skills.DefaultSkillManagerConfig()
		if opts.SkillsConfig != nil {
			cfgSkills = *opts.SkillsConfig
		}
		mgr := skills.NewSkillManager(cfgSkills, b.logger)
		if dir != "" {
			if err := mgr.ScanDirectory(dir); err != nil {
				return nil, fmt.Errorf("scan skills directory %q: %w", dir, err)
			}
		}
		ag.EnableSkills(mgr)
	}
	if enabled(opts.EnableAll, opts.EnableMCP) {
		mcpName := strings.TrimSpace(opts.MCPServerName)
		if mcpName == "" {
			mcpName = "agentflow-mcp"
		}
		mcpVersion := strings.TrimSpace(opts.MCPServerVersion)
		if mcpVersion == "" {
			mcpVersion = "0.1.0"
		}
		ag.EnableMCP(mcpproto.NewMCPServer(mcpName, mcpVersion, b.logger))
	}
	if opts.LSPClient != nil {
		ag.EnableLSP(opts.LSPClient)
	} else if enabled(opts.EnableAll, opts.EnableLSP) {
		lspName := strings.TrimSpace(opts.LSPServerName)
		if lspName == "" {
			lspName = "agentflow-lsp"
		}
		lspVersion := strings.TrimSpace(opts.LSPServerVersion)
		if lspVersion == "" {
			lspVersion = "0.1.0"
		}
		lspRuntime := agent.NewManagedLSP(agentlsp.ServerInfo{Name: lspName, Version: lspVersion}, b.logger)
		ag.EnableLSPWithLifecycle(lspRuntime.Client, lspRuntime)
	}
	if enabled(opts.EnableAll, opts.EnableEnhancedMemory) {
		memCfg := memory.DefaultEnhancedMemoryConfig()
		if opts.EnhancedMemoryConfig != nil {
			memCfg = *opts.EnhancedMemoryConfig
		}
		ag.EnableEnhancedMemory(memory.NewDefaultEnhancedMemorySystem(memCfg, b.logger))
	}
	if obsEnabled {
		if opts.ObservabilitySystem != nil {
			ag.EnableObservability(opts.ObservabilitySystem)
		} else {
			ag.EnableObservability(agentobs.NewObservabilitySystem(b.logger))
		}
	}

	reasoningRegistry := resolveRuntimeReasoningRegistry(ag.MainGateway(), cfg2.LLM.Model, cfg2.Core.ID, opts, b.logger)
	ag.SetReasoningRegistry(reasoningRegistry)
	ag.SetReasoningModeSelector(agent.NewDefaultReasoningModeSelector())
	ag.SetCompletionJudge(agent.NewDefaultCompletionJudge())
	if opts.CheckpointManager != nil {
		ag.SetCheckpointManager(opts.CheckpointManager)
	}

	if err := ag.ValidateConfiguration(); err != nil {
		return nil, err
	}

	if opts.InitAgent {
		if err := ag.Init(ctx); err != nil {
			return nil, fmt.Errorf("init agent: %w", err)
		}
	}

	return ag, nil
}

func reflectionConfigFromTypes(cfg *types.ReflectionConfig) agent.ReflectionExecutorConfig {
	out := agent.DefaultReflectionConfig()
	if cfg == nil {
		return *out
	}
	if cfg.MaxIterations > 0 {
		out.MaxIterations = cfg.MaxIterations
	}
	if cfg.MinQuality > 0 {
		out.MinQuality = cfg.MinQuality
	}
	if strings.TrimSpace(cfg.CriticPrompt) != "" {
		out.CriticPrompt = cfg.CriticPrompt
	}
	return *out
}

func toolSelectionConfigFromTypes(cfg *types.ToolSelectionConfig) agent.ToolSelectionConfig {
	out := agent.DefaultToolSelectionConfig()
	if cfg == nil {
		return *out
	}
	if cfg.MaxTools > 0 {
		out.MaxTools = cfg.MaxTools
	}
	if cfg.SimilarityThreshold > 0 {
		out.MinScore = cfg.SimilarityThreshold
	}
	return *out
}

func promptEnhancerConfigFromTypes(cfg *types.PromptEnhancerConfig) agent.PromptEnhancerConfig {
	out := agent.DefaultPromptEnhancerConfig()
	if cfg == nil {
		return *out
	}
	return *out
}

func resolveRuntimeReasoningRegistry(
	gateway llmcore.Gateway,
	model string,
	agentID string,
	opts BuildOptions,
	logger *zap.Logger,
) *reasoning.PatternRegistry {
	if opts.ReasoningRegistry != nil {
		return opts.ReasoningRegistry
	}
	return agent.NewReasoningRegistryForExposure(
		gateway,
		model,
		opts.ToolManager,
		agentID,
		opts.EventBus,
		opts.ReasoningExposure,
		logger,
	)
}
