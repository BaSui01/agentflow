package agent

import (
	"fmt"
	"strings"
	"sync"

	"github.com/BaSui01/agentflow/agent/skills"
	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// Agent Factory 是创建 Agent 实例的函数
type AgentFactory func(
	config types.AgentConfig,
	provider llm.Provider,
	memory MemoryManager,
	toolManager ToolManager,
	bus EventBus,
	logger *zap.Logger,
) (Agent, error)

// Agent Registry 管理代理类型注册和创建
// 它提供了一种集中的方式 注册和即时处理不同的代理类型
type AgentRegistry struct {
	mu        sync.RWMutex
	factories map[AgentType]AgentFactory
	logger    *zap.Logger
}

// 新建代理注册
func NewAgentRegistry(logger *zap.Logger) *AgentRegistry {
	registry := &AgentRegistry{
		factories: make(map[AgentType]AgentFactory),
		logger:    logger,
	}

	// 注册内置代理类型
	registry.registerBuiltinTypes()

	return registry
}

// 注册 BuiltinTyps 注册默认代理类型
func (r *AgentRegistry) registerBuiltinTypes() {
	// 通用代理工厂 — 无预装配置
	r.Register(TypeGeneric, newTypedAgentFactory(TypeGeneric))

	// 助理代理工厂 — 预装 communication + reasoning 提示
	r.Register(TypeAssistant, newTypedAgentFactory(TypeAssistant))

	// 分析剂厂 — 预装 data + reasoning 提示
	r.Register(TypeAnalyzer, newTypedAgentFactory(TypeAnalyzer))

	// 翻译代理工厂 — 预装 communication 提示
	r.Register(TypeTranslator, newTypedAgentFactory(TypeTranslator))

	// 总结剂厂 — 预装 reasoning 提示
	r.Register(TypeSummarizer, newTypedAgentFactory(TypeSummarizer))

	// 审查员代理工厂 — 预装 coding + reasoning 提示
	r.Register(TypeReviewer, newTypedAgentFactory(TypeReviewer))
}

// newTypedAgentFactory creates an AgentFactory that applies type-specific PromptBundle defaults.
func newTypedAgentFactory(agentType AgentType) AgentFactory {
	return func(
		config types.AgentConfig,
		provider llm.Provider,
		memory MemoryManager,
		toolManager ToolManager,
		bus EventBus,
		logger *zap.Logger,
	) (Agent, error) {
		ensureAgentType(&config)
		// Apply type-specific defaults only if the user hasn't set a PromptBundle
		if strings.TrimSpace(config.Runtime.SystemPrompt) == "" {
			config.Runtime.SystemPrompt = defaultPromptBundleForType(agentType).RenderSystemPrompt()
		}
		// Apply type-specific skill categories into Metadata for SkillManager discovery
		if cats := defaultSkillCategoriesForType(agentType); len(cats) > 0 {
			if config.Metadata == nil {
				config.Metadata = make(map[string]string)
			}
			if _, exists := config.Metadata["skill_categories"]; !exists {
				config.Metadata["skill_categories"] = joinSkillCategories(cats)
			}
		}
		return NewBaseAgent(config, provider, memory, toolManager, bus, logger), nil
	}
}

// defaultPromptBundleForType returns a pre-configured PromptBundle for each agent type.
// Generic type returns a zero PromptBundle (no preset behavior).
func defaultPromptBundleForType(agentType AgentType) PromptBundle {
	switch agentType {
	case TypeAssistant:
		return PromptBundle{
			Version: "1.0.0",
			System: SystemPrompt{
				Role:     "You are a helpful assistant with strong communication and reasoning skills.",
				Identity: "assistant",
				Policies: []string{
					"Provide clear, well-structured responses",
					"Ask clarifying questions when the request is ambiguous",
					"Use appropriate tone and language for the context",
				},
			},
		}
	case TypeAnalyzer:
		return PromptBundle{
			Version: "1.0.0",
			System: SystemPrompt{
				Role:     "You are a data analysis specialist with strong reasoning capabilities.",
				Identity: "analyzer",
				Policies: []string{
					"Present data-driven insights with supporting evidence",
					"Use structured formats (tables, lists) for clarity",
					"Identify patterns, anomalies, and trends in data",
				},
			},
		}
	case TypeTranslator:
		return PromptBundle{
			Version: "1.0.0",
			System: SystemPrompt{
				Role:     "You are a professional translator with expertise in cross-cultural communication.",
				Identity: "translator",
				Policies: []string{
					"Preserve the original meaning and tone during translation",
					"Adapt cultural references appropriately for the target audience",
					"Flag ambiguous terms that may have multiple translations",
				},
			},
		}
	case TypeSummarizer:
		return PromptBundle{
			Version: "1.0.0",
			System: SystemPrompt{
				Role:     "You are a summarization specialist with strong reasoning skills.",
				Identity: "summarizer",
				Policies: []string{
					"Extract key points and main arguments from the source material",
					"Maintain factual accuracy while condensing information",
					"Organize summaries with clear structure and hierarchy",
				},
			},
		}
	case TypeReviewer:
		return PromptBundle{
			Version: "1.0.0",
			System: SystemPrompt{
				Role:     "You are a code review specialist with expertise in software engineering best practices.",
				Identity: "reviewer",
				Policies: []string{
					"Identify bugs, security vulnerabilities, and performance issues",
					"Suggest improvements following established coding standards",
					"Provide constructive feedback with concrete examples",
					"Check for proper error handling and edge cases",
				},
			},
		}
	default:
		// TypeGeneric and unknown types — no preset
		return PromptBundle{}
	}
}

// defaultSkillCategoriesForType returns the recommended skill categories for each agent type.
// These categories are used by SkillManager to discover and load relevant skills.
func defaultSkillCategoriesForType(agentType AgentType) []skills.SkillCategory {
	switch agentType {
	case TypeAssistant:
		return []skills.SkillCategory{skills.CategoryCommunication, skills.CategoryReasoning}
	case TypeAnalyzer:
		return []skills.SkillCategory{skills.CategoryData, skills.CategoryReasoning}
	case TypeTranslator:
		return []skills.SkillCategory{skills.CategoryCommunication}
	case TypeSummarizer:
		return []skills.SkillCategory{skills.CategoryReasoning}
	case TypeReviewer:
		return []skills.SkillCategory{skills.CategoryCoding, skills.CategoryReasoning}
	default:
		return nil
	}
}

// joinSkillCategories joins skill categories into a comma-separated string.
func joinSkillCategories(cats []skills.SkillCategory) string {
	parts := make([]string, len(cats))
	for i, c := range cats {
		parts[i] = string(c)
	}
	return strings.Join(parts, ",")
}

// 登记册登记具有工厂功能的新代理类型
func (r *AgentRegistry) Register(agentType AgentType, factory AgentFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.factories[agentType] = factory
	r.logger.Info("agent type registered",
		zap.String("type", string(agentType)),
	)
}

// 未注册从注册簿中删除代理类型
func (r *AgentRegistry) Unregister(agentType AgentType) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.factories, agentType)
	r.logger.Info("agent type unregistered",
		zap.String("type", string(agentType)),
	)
}

// 创建指定类型的新代理实例
func (r *AgentRegistry) Create(
	config types.AgentConfig,
	provider llm.Provider,
	memory MemoryManager,
	toolManager ToolManager,
	bus EventBus,
	logger *zap.Logger,
) (Agent, error) {
	r.mu.RLock()
	factory, exists := r.factories[AgentType(config.Core.Type)]
	r.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("agent type %q not registered", config.Core.Type)
	}

	agent, err := factory(config, provider, memory, toolManager, bus, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create agent of type %q: %w", config.Core.Type, err)
	}

	r.logger.Info("agent created",
		zap.String("type", config.Core.Type),
		zap.String("id", config.Core.ID),
		zap.String("name", config.Core.Name),
	)

	return agent, nil
}

// 如果已注册代理类型, 正在注册检查
func (r *AgentRegistry) IsRegistered(agentType AgentType) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.factories[agentType]
	return exists
}

// 列表类型返回所有已注册代理类型
func (r *AgentRegistry) ListTypes() []AgentType {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]AgentType, 0, len(r.factories))
	for t := range r.factories {
		types = append(types, t)
	}

	return types
}

// Global Registry 是默认代理注册实例
var (
	GlobalRegistry     *AgentRegistry
	globalRegistryOnce sync.Once
	globalRegistryMu   sync.RWMutex
)

// Init Global Registry将全球代理登记初始化。
// 此函数可以安全多次调用 - 只有第一个调用会初始化 。
func InitGlobalRegistry(logger *zap.Logger) {
	globalRegistryOnce.Do(func() {
		GlobalRegistry = NewAgentRegistry(logger)
	})
}

// Create Agent 使用全球登记册创建代理
func CreateAgent(
	config types.AgentConfig,
	provider llm.Provider,
	memory MemoryManager,
	toolManager ToolManager,
	bus EventBus,
	logger *zap.Logger,
) (Agent, error) {
	globalRegistryMu.RLock()
	registry := GlobalRegistry
	globalRegistryMu.RUnlock()

	if registry == nil {
		return nil, fmt.Errorf("global registry not initialized, call InitGlobalRegistry first")
	}
	return registry.Create(config, provider, memory, toolManager, bus, logger)
}
