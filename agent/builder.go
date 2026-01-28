package agent

import (
	"fmt"
	"os"
	"strings"

	"github.com/BaSui01/agentflow/agent/memory"
	mcpproto "github.com/BaSui01/agentflow/agent/protocol/mcp"
	"github.com/BaSui01/agentflow/agent/skills"
	"github.com/BaSui01/agentflow/llm"
	"go.uber.org/zap"
)

// AgentBuilder 提供流式构建 Agent 的能力
// 支持链式调用，简化 Agent 创建过程
type AgentBuilder struct {
	config      Config
	provider    llm.Provider
	memory      MemoryManager
	toolManager ToolManager
	bus         EventBus
	logger      *zap.Logger

	// 增强功能配置
	reflectionConfig     *ReflectionExecutorConfig
	toolSelectionConfig  *ToolSelectionConfig
	promptEnhancerConfig *PromptEnhancerConfig
	skillsConfig         interface{} // 避免循环依赖
	mcpConfig            interface{}
	enhancedMemoryConfig interface{}
	observabilityConfig  interface{}

	errors []error
}

// NewAgentBuilder 创建 Agent 构建器
func NewAgentBuilder(config Config) *AgentBuilder {
	return &AgentBuilder{
		config: config,
		errors: make([]error, 0),
	}
}

// WithProvider 设置 LLM Provider
func (b *AgentBuilder) WithProvider(provider llm.Provider) *AgentBuilder {
	if provider == nil {
		b.errors = append(b.errors, fmt.Errorf("provider cannot be nil"))
		return b
	}
	b.provider = provider
	return b
}

// WithMemory 设置记忆管理器
func (b *AgentBuilder) WithMemory(memory MemoryManager) *AgentBuilder {
	b.memory = memory
	return b
}

// WithToolManager 设置工具管理器
func (b *AgentBuilder) WithToolManager(toolManager ToolManager) *AgentBuilder {
	b.toolManager = toolManager
	return b
}

// WithEventBus 设置事件总线
func (b *AgentBuilder) WithEventBus(bus EventBus) *AgentBuilder {
	b.bus = bus
	return b
}

// WithLogger 设置日志器
func (b *AgentBuilder) WithLogger(logger *zap.Logger) *AgentBuilder {
	if logger == nil {
		b.errors = append(b.errors, fmt.Errorf("logger cannot be nil"))
		return b
	}
	b.logger = logger
	return b
}

// WithReflection 启用 Reflection 机制
func (b *AgentBuilder) WithReflection(config *ReflectionExecutorConfig) *AgentBuilder {
	if config == nil {
		config = DefaultReflectionConfig()
	}
	b.reflectionConfig = config
	b.config.EnableReflection = true
	return b
}

// WithToolSelection 启用动态工具选择
func (b *AgentBuilder) WithToolSelection(config *ToolSelectionConfig) *AgentBuilder {
	if config == nil {
		config = DefaultToolSelectionConfig()
	}
	b.toolSelectionConfig = config
	b.config.EnableToolSelection = true
	return b
}

// WithPromptEnhancer 启用提示词增强
func (b *AgentBuilder) WithPromptEnhancer(config *PromptEnhancerConfig) *AgentBuilder {
	if config == nil {
		config = DefaultPromptEnhancerConfig()
	}
	b.promptEnhancerConfig = config
	b.config.EnablePromptEnhancer = true
	return b
}

// SkillsOptions configures how the builder creates a default skills manager.
type SkillsOptions struct {
	Directory string
	Config    skills.SkillManagerConfig
}

// MCPServerOptions configures how the builder creates a default MCP server.
type MCPServerOptions struct {
	Name    string
	Version string
}

// WithSkills 启用 Skills 系统
func (b *AgentBuilder) WithSkills(config interface{}) *AgentBuilder {
	b.skillsConfig = config
	b.config.EnableSkills = true
	return b
}

// WithDefaultSkills enables the built-in skills manager and optionally scans a directory.
func (b *AgentBuilder) WithDefaultSkills(directory string, config *skills.SkillManagerConfig) *AgentBuilder {
	opts := SkillsOptions{
		Directory: strings.TrimSpace(directory),
		Config:    skills.DefaultSkillManagerConfig(),
	}
	if config != nil {
		opts.Config = *config
	}
	return b.WithSkills(opts)
}

// WithMCP 启用 MCP 集成
func (b *AgentBuilder) WithMCP(config interface{}) *AgentBuilder {
	b.mcpConfig = config
	b.config.EnableMCP = true
	return b
}

// WithDefaultMCPServer enables the built-in MCP server with a default name/version.
func (b *AgentBuilder) WithDefaultMCPServer(name, version string) *AgentBuilder {
	return b.WithMCP(MCPServerOptions{
		Name:    strings.TrimSpace(name),
		Version: strings.TrimSpace(version),
	})
}

// WithEnhancedMemory 启用增强记忆系统
func (b *AgentBuilder) WithEnhancedMemory(config interface{}) *AgentBuilder {
	b.enhancedMemoryConfig = config
	b.config.EnableEnhancedMemory = true
	return b
}

// WithDefaultEnhancedMemory enables the built-in enhanced memory system with in-memory stores.
func (b *AgentBuilder) WithDefaultEnhancedMemory(config *memory.EnhancedMemoryConfig) *AgentBuilder {
	if config == nil {
		cfg := memory.DefaultEnhancedMemoryConfig()
		return b.WithEnhancedMemory(cfg)
	}
	return b.WithEnhancedMemory(*config)
}

// WithObservability 启用可观测性系统
func (b *AgentBuilder) WithObservability(config interface{}) *AgentBuilder {
	b.observabilityConfig = config
	b.config.EnableObservability = true
	return b
}

// Build 构建 Agent 实例
func (b *AgentBuilder) Build() (*BaseAgent, error) {
	// 检查构建过程中的错误
	if len(b.errors) > 0 {
		return nil, fmt.Errorf("builder has %d errors: %v", len(b.errors), b.errors[0])
	}

	// 验证必需字段
	if b.provider == nil {
		return nil, fmt.Errorf("provider is required")
	}

	// 设置默认 logger
	if b.logger == nil {
		b.logger = zap.NewNop()
	}

	// 创建基础 Agent
	agent := NewBaseAgent(
		b.config,
		b.provider,
		b.memory,
		b.toolManager,
		b.bus,
		b.logger,
	)

	// If feature flags were enabled directly on Config, fall back to default configs.
	if b.config.EnableReflection && b.reflectionConfig == nil {
		b.reflectionConfig = DefaultReflectionConfig()
	}
	if b.config.EnableToolSelection && b.toolSelectionConfig == nil {
		b.toolSelectionConfig = DefaultToolSelectionConfig()
	}
	if b.config.EnablePromptEnhancer && b.promptEnhancerConfig == nil {
		b.promptEnhancerConfig = DefaultPromptEnhancerConfig()
	}

	// Enable advanced features
	if b.config.EnableReflection && b.reflectionConfig != nil {
		reflectionExecutor := NewReflectionExecutor(agent, *b.reflectionConfig)
		agent.EnableReflection(reflectionExecutor)
	}

	if b.config.EnableToolSelection && b.toolSelectionConfig != nil {
		toolSelector := NewDynamicToolSelector(agent, *b.toolSelectionConfig)
		agent.EnableToolSelection(toolSelector)
	}

	if b.config.EnablePromptEnhancer && b.promptEnhancerConfig != nil {
		promptEnhancer := NewPromptEnhancer(*b.promptEnhancerConfig)
		agent.EnablePromptEnhancer(promptEnhancer)
	}

	if err := b.enableOptionalFeatures(agent); err != nil {
		return nil, err
	}

	return agent, nil
}

func (b *AgentBuilder) enableOptionalFeatures(agent *BaseAgent) error {
	if b.config.EnableSkills {
		if err := b.enableSkills(agent); err != nil {
			return err
		}
	}
	if b.config.EnableMCP {
		if err := b.enableMCP(agent); err != nil {
			return err
		}
	}
	if b.config.EnableEnhancedMemory {
		if err := b.enableEnhancedMemory(agent); err != nil {
			return err
		}
	}
	// Observability has an import-cycle with agent/observability (it imports agent).
	// Builders can still accept a provided instance; for out-of-the-box wiring use agent/runtime.
	if b.config.EnableObservability && b.observabilityConfig != nil {
		agent.EnableObservability(b.observabilityConfig)
	}
	return nil
}

func (b *AgentBuilder) enableSkills(agent *BaseAgent) error {
	switch v := b.skillsConfig.(type) {
	case nil:
		mgr := skills.NewSkillManager(skills.DefaultSkillManagerConfig(), b.logger)
		if _, err := os.Stat("./skills"); err == nil {
			_ = mgr.ScanDirectory("./skills")
		}
		agent.EnableSkills(mgr)
		return nil
	case string:
		dir := strings.TrimSpace(v)
		mgr := skills.NewSkillManager(skills.DefaultSkillManagerConfig(), b.logger)
		if dir != "" {
			if err := mgr.ScanDirectory(dir); err != nil {
				return fmt.Errorf("scan skills directory %q: %w", dir, err)
			}
		}
		agent.EnableSkills(mgr)
		return nil
	case SkillsOptions:
		mgr := skills.NewSkillManager(v.Config, b.logger)
		if v.Directory != "" {
			if err := mgr.ScanDirectory(v.Directory); err != nil {
				return fmt.Errorf("scan skills directory %q: %w", v.Directory, err)
			}
		}
		agent.EnableSkills(mgr)
		return nil
	case skills.SkillManagerConfig:
		mgr := skills.NewSkillManager(v, b.logger)
		agent.EnableSkills(mgr)
		return nil
	case *skills.DefaultSkillManager:
		agent.EnableSkills(v)
		return nil
	case skills.SkillManager:
		agent.EnableSkills(v)
		return nil
	default:
		agent.EnableSkills(v)
		return nil
	}
}

func (b *AgentBuilder) enableMCP(agent *BaseAgent) error {
	switch v := b.mcpConfig.(type) {
	case nil:
		agent.EnableMCP(mcpproto.NewMCPServer("agentflow-mcp", "0.1.0", b.logger))
		return nil
	case MCPServerOptions:
		name := v.Name
		version := v.Version
		if name == "" {
			name = "agentflow-mcp"
		}
		if version == "" {
			version = "0.1.0"
		}
		agent.EnableMCP(mcpproto.NewMCPServer(name, version, b.logger))
		return nil
	case string:
		name := strings.TrimSpace(v)
		if name == "" {
			name = "agentflow-mcp"
		}
		agent.EnableMCP(mcpproto.NewMCPServer(name, "0.1.0", b.logger))
		return nil
	default:
		agent.EnableMCP(v)
		return nil
	}
}

func (b *AgentBuilder) enableEnhancedMemory(agent *BaseAgent) error {
	switch v := b.enhancedMemoryConfig.(type) {
	case nil:
		cfg := memory.DefaultEnhancedMemoryConfig()
		agent.EnableEnhancedMemory(memory.NewDefaultEnhancedMemorySystem(cfg, b.logger))
		return nil
	case memory.EnhancedMemoryConfig:
		agent.EnableEnhancedMemory(memory.NewDefaultEnhancedMemorySystem(v, b.logger))
		return nil
	case *memory.EnhancedMemorySystem:
		agent.EnableEnhancedMemory(v)
		return nil
	default:
		agent.EnableEnhancedMemory(v)
		return nil
	}
}

// Validate 验证配置是否有效
func (b *AgentBuilder) Validate() error {
	if len(b.errors) > 0 {
		return fmt.Errorf("builder has %d errors: %v", len(b.errors), b.errors[0])
	}

	if b.config.ID == "" {
		return fmt.Errorf("agent ID is required")
	}

	if b.config.Name == "" {
		return fmt.Errorf("agent name is required")
	}

	if b.config.Model == "" {
		return fmt.Errorf("model is required")
	}

	if b.provider == nil {
		return fmt.Errorf("provider is required")
	}

	return nil
}
