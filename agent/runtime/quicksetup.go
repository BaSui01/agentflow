package runtime

import (
	"context"
	"fmt"
	"strings"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/agent/memory"
	"github.com/BaSui01/agentflow/agent/observability"
	mcpproto "github.com/BaSui01/agentflow/agent/protocol/mcp"
	"github.com/BaSui01/agentflow/agent/skills"
	"github.com/BaSui01/agentflow/llm"
	"go.uber.org/zap"
)

// BuildOptions controls out-of-the-box wiring for optional subsystems.
type BuildOptions struct {
	EnableAll bool

	EnableReflection     bool
	EnableToolSelection  bool
	EnablePromptEnhancer bool
	EnableSkills         bool
	EnableMCP            bool
	EnableEnhancedMemory bool
	EnableObservability  bool

	SkillsDirectory string
	SkillsConfig    *skills.SkillManagerConfig

	MCPServerName    string
	MCPServerVersion string

	EnhancedMemoryConfig *memory.EnhancedMemoryConfig

	// ObservabilitySystem, when set, is used instead of the default implementation.
	ObservabilitySystem interface{}

	// InitAgent calls Init(ctx) after wiring.
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
		EnableEnhancedMemory: true,
		EnableObservability:  true,
		SkillsDirectory:      "./skills",
		MCPServerName:        "agentflow-mcp",
		MCPServerVersion:     "0.1.0",
		InitAgent:            false,
	}
}

func enabled(all bool, v bool) bool { return all || v }

// BuildAgent constructs an agent.BaseAgent and wires optional features with sensible defaults.
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

// QuickSetup wires defaults onto an already-constructed BaseAgent.
// This is useful when callers instantiate BaseAgent manually but still want the default subsystems.
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
