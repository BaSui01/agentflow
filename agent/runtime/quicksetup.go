package runtime

import (
	"context"
	"fmt"
	"strings"

	"github.com/BaSui01/agentflow/agent"
	agentlsp "github.com/BaSui01/agentflow/agent/lsp"
	"github.com/BaSui01/agentflow/agent/memory"
	"github.com/BaSui01/agentflow/agent/observability"
	mcpproto "github.com/BaSui01/agentflow/agent/protocol/mcp"
	"github.com/BaSui01/agentflow/agent/skills"
	"github.com/BaSui01/agentflow/llm"
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
	ObservabilitySystem interface{}

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

// BuildAgent 构造一个代理. BaseAgent和有线的可选特性,带有合理的默认.
func BuildAgent(ctx context.Context, cfg agent.Config, provider llm.Provider, logger *zap.Logger, opts BuildOptions) (*agent.BaseAgent, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	cfg2 := cfg
	cfg2.EnableObservability = enabled(opts.EnableAll, opts.EnableObservability)

	b := agent.NewAgentBuilder(cfg2).
		WithProvider(provider).
		WithLogger(logger)

	if enabled(opts.EnableAll, opts.EnableReflection) {
		b.WithReflection(nil)
	}
	if enabled(opts.EnableAll, opts.EnableToolSelection) {
		b.WithToolSelection(nil)
	}
	if enabled(opts.EnableAll, opts.EnablePromptEnhancer) {
		b.WithPromptEnhancer(nil)
	}

	if enabled(opts.EnableAll, opts.EnableSkills) {
		dir := strings.TrimSpace(opts.SkillsDirectory)
		b.WithDefaultSkills(dir, opts.SkillsConfig)
	}
	if enabled(opts.EnableAll, opts.EnableMCP) {
		b.WithMCP(agent.MCPServerOptions{
			Name:    strings.TrimSpace(opts.MCPServerName),
			Version: strings.TrimSpace(opts.MCPServerVersion),
		})
	}
	if enabled(opts.EnableAll, opts.EnableLSP) {
		b.WithDefaultLSPServer(strings.TrimSpace(opts.LSPServerName), strings.TrimSpace(opts.LSPServerVersion))
	}
	if enabled(opts.EnableAll, opts.EnableEnhancedMemory) {
		b.WithDefaultEnhancedMemory(opts.EnhancedMemoryConfig)
	}

	ag, err := b.Build()
	if err != nil {
		return nil, err
	}

	if cfg2.EnableObservability {
		if opts.ObservabilitySystem != nil {
			ag.EnableObservability(opts.ObservabilitySystem)
		} else {
			ag.EnableObservability(observability.NewObservabilitySystem(logger))
		}
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

// QuickSetup 电线默认为已经构建的 Base Agent 。
// 当呼叫者手动即时BaseAgent但仍然想要默认子系统时,此功能是有用的.
func QuickSetup(ctx context.Context, ag *agent.BaseAgent, opts BuildOptions) error {
	if ag == nil {
		return fmt.Errorf("agent is nil")
	}
	logger := ag.Logger()
	if logger == nil {
		logger = zap.NewNop()
	}

	if enabled(opts.EnableAll, opts.EnableSkills) && !ag.GetFeatureStatus()["skills"] {
		mgr := skills.NewSkillManager(skills.DefaultSkillManagerConfig(), logger)
		dir := strings.TrimSpace(opts.SkillsDirectory)
		if dir != "" {
			_ = mgr.ScanDirectory(dir)
		}
		ag.EnableSkills(mgr)
	}
	if enabled(opts.EnableAll, opts.EnableMCP) && !ag.GetFeatureStatus()["mcp"] {
		name := strings.TrimSpace(opts.MCPServerName)
		if name == "" {
			name = "agentflow-mcp"
		}
		version := strings.TrimSpace(opts.MCPServerVersion)
		if version == "" {
			version = "0.1.0"
		}
		ag.EnableMCP(mcpproto.NewMCPServer(name, version, logger))
	}
	if enabled(opts.EnableAll, opts.EnableLSP) && !ag.GetFeatureStatus()["lsp"] {
		name := strings.TrimSpace(opts.LSPServerName)
		if name == "" {
			name = "agentflow-lsp"
		}
		version := strings.TrimSpace(opts.LSPServerVersion)
		if version == "" {
			version = "0.1.0"
		}
		runtime := agent.NewManagedLSP(agentlsp.ServerInfo{Name: name, Version: version}, logger)
		ag.EnableLSPWithLifecycle(runtime.Client, runtime)
	}
	if enabled(opts.EnableAll, opts.EnableEnhancedMemory) && !ag.GetFeatureStatus()["enhanced_memory"] {
		memCfg := memory.DefaultEnhancedMemoryConfig()
		if opts.EnhancedMemoryConfig != nil {
			memCfg = *opts.EnhancedMemoryConfig
		}
		ag.EnableEnhancedMemory(memory.NewDefaultEnhancedMemorySystem(memCfg, logger))
	}
	if enabled(opts.EnableAll, opts.EnableObservability) && !ag.GetFeatureStatus()["observability"] {
		if opts.ObservabilitySystem != nil {
			ag.EnableObservability(opts.ObservabilitySystem)
		} else {
			ag.EnableObservability(observability.NewObservabilitySystem(logger))
		}
	}

	if err := ag.ValidateConfiguration(); err != nil {
		return err
	}
	if opts.InitAgent {
		return ag.Init(ctx)
	}
	return nil
}
