package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/BaSui01/agentflow/agent/skills"
	"github.com/BaSui01/agentflow/llm"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
)

// Agent Factory 是创建 Agent 实例的函数
type AgentFactory func(
	config types.AgentConfig,
	gateway llmcore.Gateway,
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
		gateway llmcore.Gateway,
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
		return buildRegistryAgent(config, gateway, memory, toolManager, bus, logger)
	}
}

func buildRegistryAgent(
	config types.AgentConfig,
	gateway llmcore.Gateway,
	memory MemoryManager,
	toolManager ToolManager,
	bus EventBus,
	logger *zap.Logger,
) (Agent, error) {
	ag, err := newAgentBuilder(config).
		WithGateway(gateway).
		WithMemory(memory).
		WithToolManager(toolManager).
		WithEventBus(bus).
		WithLogger(logger).
		Build()
	if err != nil {
		return nil, err
	}

	// Keep the registry default factory on the same post-build path as runtime.Builder:
	// inject the default reasoning registry and validate the finalized runtime wiring.
	ag.SetReasoningRegistry(NewDefaultReasoningRegistry(
		ag.MainGateway(),
		ag.Config().LLM.Model,
		toolManager,
		ag.ID(),
		bus,
		logger,
	))
	if err := ag.ValidateConfiguration(); err != nil {
		return nil, err
	}

	return ag, nil
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
	gateway llmcore.Gateway,
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

	agent, err := factory(config, gateway, memory, toolManager, bus, logger)
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
	gateway llmcore.Gateway,
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
	return registry.Create(config, gateway, memory, toolManager, bus, logger)
}

// =============================================================================
// Resolver (merged from resolver.go)
// =============================================================================
// CachingResolver resolves agent IDs to live Agent instances, creating them
// on demand via AgentRegistry and caching them for reuse. It uses singleflight
// to ensure concurrent requests for the same agentID only trigger one
// Create+Init cycle.
type CachingResolver struct {
	registry       *AgentRegistry
	provider       llm.Provider
	memory         MemoryManager // optional; nil means stateless agents
	enhancedMemory EnhancedMemoryRunner
	tools          ToolManager
	logger         *zap.Logger
	agents         sync.Map
	group          singleflight.Group
	toolNames      []string
	modelHint      string

	// MongoDB persistence stores (required)
	promptStore       PromptStoreProvider
	conversationStore ConversationStoreProvider
	runStore          RunStoreProvider
}

// NewCachingResolver creates a CachingResolver backed by the given registry
// and main LLM provider.
func NewCachingResolver(registry *AgentRegistry, provider llm.Provider, logger *zap.Logger) *CachingResolver {
	return &CachingResolver{
		registry: registry,
		provider: provider,
		logger:   logger,
	}
}

// WithMemory sets the MemoryManager used when creating new agent instances.
// When non-nil, agents created by this resolver will have memory capabilities.
func (r *CachingResolver) WithMemory(m MemoryManager) *CachingResolver {
	r.memory = m
	return r
}

// WithEnhancedMemory sets the enhanced memory system used when creating new
// agent instances. When non-nil, resolved BaseAgent instances will use this
// shared enhanced memory runtime instead of a per-agent default instance.
func (r *CachingResolver) WithEnhancedMemory(mem EnhancedMemoryRunner) *CachingResolver {
	r.enhancedMemory = mem
	return r
}

// WithToolManager sets the ToolManager used when creating new agent instances.
// When non-nil, resolved agents can call tools during execution.
func (r *CachingResolver) WithToolManager(m ToolManager) *CachingResolver {
	r.tools = m
	return r
}

// WithRuntimeTools sets a default tool whitelist for resolved agents.
// If empty, the resolver derives tool names from ToolManager.GetAllowedTools(agentID).
func (r *CachingResolver) WithRuntimeTools(toolNames []string) *CachingResolver {
	if len(toolNames) == 0 {
		r.toolNames = nil
		return r
	}
	out := make([]string, 0, len(toolNames))
	seen := make(map[string]struct{}, len(toolNames))
	for _, name := range toolNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	if len(out) == 0 {
		r.toolNames = nil
		return r
	}
	r.toolNames = out
	return r
}

// WithDefaultModel sets the default model used for resolved agents.
// Agent request can still override it at runtime via model routing params.
func (r *CachingResolver) WithDefaultModel(model string) *CachingResolver {
	r.modelHint = strings.TrimSpace(model)
	return r
}

// WithPromptStore sets the PromptStoreProvider for resolved agents.
func (r *CachingResolver) WithPromptStore(s PromptStoreProvider) *CachingResolver {
	r.promptStore = s
	return r
}

// WithConversationStore sets the ConversationStoreProvider for resolved agents.
func (r *CachingResolver) WithConversationStore(s ConversationStoreProvider) *CachingResolver {
	r.conversationStore = s
	return r
}

// WithRunStore sets the RunStoreProvider for resolved agents.
func (r *CachingResolver) WithRunStore(s RunStoreProvider) *CachingResolver {
	r.runStore = s
	return r
}

// Resolve returns a cached Agent for agentID, or creates and initialises one.
func (r *CachingResolver) Resolve(ctx context.Context, agentID string) (Agent, error) {
	// Fast path: already cached.
	if cached, ok := r.agents.Load(agentID); ok {
		return cached.(Agent), nil
	}

	// Deduplicate concurrent creation for the same ID.
	result, err, _ := r.group.Do(agentID, func() (any, error) {
		// Double-check after acquiring the flight.
		if cached, ok := r.agents.Load(agentID); ok {
			return cached, nil
		}

		cfg := types.AgentConfig{
			Core: types.CoreConfig{
				ID:   agentID,
				Name: agentID,
				Type: string(TypeGeneric),
			},
			LLM: types.LLMConfig{
				Model: r.defaultResolverModel(),
			},
		}
		toolNames := r.toolNames
		if len(toolNames) == 0 && r.tools != nil {
			schemas := r.tools.GetAllowedTools(agentID)
			if len(schemas) > 0 {
				toolNames = make([]string, 0, len(schemas))
				for _, schema := range schemas {
					name := strings.TrimSpace(schema.Name)
					if name == "" {
						continue
					}
					toolNames = append(toolNames, name)
				}
			}
		}
		if len(toolNames) > 0 {
			cfg.Runtime.Tools = append([]string(nil), toolNames...)
		}
		ag, err := r.registry.Create(cfg, wrapProviderWithGateway(r.provider, r.logger, nil), r.memory, r.tools, nil, r.logger)
		if err != nil {
			return nil, fmt.Errorf("failed to create agent %q: %w", agentID, err)
		}

		// Inject MongoDB persistence stores.
		if ba, ok := ag.(*BaseAgent); ok {
			ba.SetPromptStore(r.promptStore)
			ba.SetConversationStore(r.conversationStore)
			ba.SetRunStore(r.runStore)
			if r.enhancedMemory != nil {
				ba.EnableEnhancedMemory(r.enhancedMemory)
			}
		}

		if err := ag.Init(ctx); err != nil {
			return nil, fmt.Errorf("failed to init agent %q: %w", agentID, err)
		}

		r.agents.Store(agentID, ag)
		return ag, nil
	})
	if err != nil {
		return nil, err
	}
	return result.(Agent), nil
}

func (r *CachingResolver) defaultResolverModel() string {
	if model := strings.TrimSpace(r.modelHint); model != "" {
		return model
	}
	if r.provider != nil {
		if name := strings.TrimSpace(r.provider.Name()); name != "" {
			return name
		}
	}
	if provider := compatProviderFromGateway(wrapProviderWithGateway(r.provider, r.logger, nil)); provider != nil {
		if name := strings.TrimSpace(provider.Name()); name != "" {
			return name
		}
	}
	return "resolver-default"
}

// TeardownAll tears down all cached agent instances. Intended to be called
// during graceful shutdown.
func (r *CachingResolver) TeardownAll(ctx context.Context) {
	r.agents.Range(func(key, value any) bool {
		if ag, ok := value.(Agent); ok {
			if err := ag.Teardown(ctx); err != nil {
				r.logger.Warn("Failed to teardown cached agent",
					zap.String("agent_id", key.(string)),
					zap.Error(err))
			}
		}
		return true
	})
}

// ResetCache tears down and removes all cached agent instances.
// Future Resolve calls will recreate agents with latest runtime settings.
func (r *CachingResolver) ResetCache(ctx context.Context) {
	r.agents.Range(func(key, value any) bool {
		if ag, ok := value.(Agent); ok {
			if err := ag.Teardown(ctx); err != nil {
				r.logger.Warn("Failed to teardown cached agent during reset",
					zap.String("agent_id", key.(string)),
					zap.Error(err))
			}
		}
		r.agents.Delete(key)
		return true
	})
}
