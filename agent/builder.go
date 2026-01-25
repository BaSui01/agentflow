package agent

import (
	"fmt"

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

// WithSkills 启用 Skills 系统
func (b *AgentBuilder) WithSkills(config interface{}) *AgentBuilder {
	b.skillsConfig = config
	b.config.EnableSkills = true
	return b
}

// WithMCP 启用 MCP 集成
func (b *AgentBuilder) WithMCP(config interface{}) *AgentBuilder {
	b.mcpConfig = config
	b.config.EnableMCP = true
	return b
}

// WithEnhancedMemory 启用增强记忆系统
func (b *AgentBuilder) WithEnhancedMemory(config interface{}) *AgentBuilder {
	b.enhancedMemoryConfig = config
	b.config.EnableEnhancedMemory = true
	return b
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

	// 其他增强功能可以在这里添加
	// 注意：Skills、MCP、EnhancedMemory 等需要额外的依赖，这里只做占位

	return agent, nil
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
