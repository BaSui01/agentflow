package runtime

import (
	"context"
	"fmt"
	"strings"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/agent/memory"
	agentobs "github.com/BaSui01/agentflow/agent/observability"
	"github.com/BaSui01/agentflow/agent/reasoning"
	"github.com/BaSui01/agentflow/agent/skills"
	"github.com/BaSui01/agentflow/llm"
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
	MemoryManager      agent.MemoryManager
	ToolManager        agent.ToolManager
	EventBus           agent.EventBus
	LSPClient          agent.LSPClientRunner

	// Optional pass-throughs for AgentBuilder advanced wiring.
	PromptStore       agent.PromptStoreProvider
	ConversationStore agent.ConversationStoreProvider
	RunStore          agent.RunStoreProvider
	Orchestrator      agent.OrchestratorRunner
	ReasoningRegistry *reasoning.PatternRegistry

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
		InitAgent:            false,
	}
}

func enabled(all bool, v bool) bool { return all || v }

// Builder 是 agent/runtime 的唯一构建入口。
type Builder struct {
	provider     llm.Provider
	toolProvider llm.Provider
	ledger       llmobs.Ledger
	logger       *zap.Logger
	options      BuildOptions
	toolScope    []string // tool whitelist for sub-agent isolation (empty = all tools)
}

// NewBuilder 创建 runtime builder。logger 为必选参数，nil 时 panic。
func NewBuilder(provider llm.Provider, logger *zap.Logger) *Builder {
	if logger == nil {
		panic("runtime.Builder: logger is required and cannot be nil")
	}
	return &Builder{
		provider: provider,
		logger:   logger,
		options:  BuildOptions{},
	}
}

// WithOptions 设置构建选项。
func (b *Builder) WithOptions(opts BuildOptions) *Builder {
	b.options = opts
	return b
}

// WithToolProvider 设置工具调用专用 Provider（双模型模式）。
// 未设置时，工具调用退化使用主 provider。
func (b *Builder) WithToolProvider(provider llm.Provider) *Builder {
	b.toolProvider = provider
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

	agentBuilder := agent.NewAgentBuilder(cfg2).
		WithProvider(b.provider).
		WithLogger(b.logger)
	if b.ledger != nil {
		agentBuilder.WithLedger(b.ledger)
	}

	if opts.MaxReActIterations > 0 {
		agentBuilder.WithMaxReActIterations(opts.MaxReActIterations)
	}
	if opts.MemoryManager != nil {
		agentBuilder.WithMemory(opts.MemoryManager)
	}
	if opts.ToolManager != nil {
		agentBuilder.WithToolManager(opts.ToolManager)
	}
	if opts.EventBus != nil {
		agentBuilder.WithEventBus(opts.EventBus)
	}
	if b.toolProvider != nil {
		agentBuilder.WithToolProvider(b.toolProvider)
	}
	if opts.PromptStore != nil {
		agentBuilder.WithPromptStore(opts.PromptStore)
	}
	if opts.ConversationStore != nil {
		agentBuilder.WithConversationStore(opts.ConversationStore)
	}
	if opts.RunStore != nil {
		agentBuilder.WithRunStore(opts.RunStore)
	}
	if opts.Orchestrator != nil {
		agentBuilder.WithOrchestrator(opts.Orchestrator)
		if agentBuilder.Orchestrator() == nil {
			return nil, fmt.Errorf("configure orchestrator: unexpected nil")
		}
	}
	if opts.ReasoningRegistry != nil {
		agentBuilder.WithReasoning(opts.ReasoningRegistry)
		if agentBuilder.ReasoningRegistry() == nil {
			return nil, fmt.Errorf("configure reasoning registry: unexpected nil")
		}
	}

	if enabled(opts.EnableAll, opts.EnableReflection) {
		agentBuilder.WithReflection(nil)
	}
	if enabled(opts.EnableAll, opts.EnableToolSelection) {
		agentBuilder.WithToolSelection(nil)
	}
	if enabled(opts.EnableAll, opts.EnablePromptEnhancer) {
		agentBuilder.WithPromptEnhancer(nil)
	}

	if enabled(opts.EnableAll, opts.EnableSkills) {
		dir := strings.TrimSpace(opts.SkillsDirectory)
		agentBuilder.WithDefaultSkills(dir, opts.SkillsConfig)
	}
	if enabled(opts.EnableAll, opts.EnableMCP) {
		agentBuilder.WithDefaultMCPServer(strings.TrimSpace(opts.MCPServerName), strings.TrimSpace(opts.MCPServerVersion))
	}
	if opts.LSPClient != nil {
		agentBuilder.WithLSP(opts.LSPClient)
	} else if enabled(opts.EnableAll, opts.EnableLSP) {
		agentBuilder.WithDefaultLSPServer(strings.TrimSpace(opts.LSPServerName), strings.TrimSpace(opts.LSPServerVersion))
	}
	if enabled(opts.EnableAll, opts.EnableEnhancedMemory) {
		agentBuilder.WithDefaultEnhancedMemory(opts.EnhancedMemoryConfig)
	}
	if obsEnabled {
		if opts.ObservabilitySystem != nil {
			agentBuilder.WithObservability(opts.ObservabilitySystem)
		} else {
			agentBuilder.WithObservability(agentobs.NewObservabilitySystem(b.logger))
		}
	}

	if err := agentBuilder.Validate(); err != nil {
		return nil, err
	}

	ag, err := agentBuilder.Build()
	if err != nil {
		return nil, err
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
