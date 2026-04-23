package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	skills "github.com/BaSui01/agentflow/agent/capabilities/tools"
	registrycore "github.com/BaSui01/agentflow/agent/collaboration/multiagent/registrycore"
	agentlsp "github.com/BaSui01/agentflow/agent/integration/lsp"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
	"github.com/google/uuid"
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
		if strings.TrimSpace(config.ExecutionOptions().Control.SystemPrompt) == "" {
			prompt := defaultPromptBundleForType(agentType).RenderSystemPrompt()
			config.Control.SystemPrompt = prompt
			config.Runtime.SystemPrompt = prompt
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

	// Keep the registry default factory on the same post-build path as
	// agent/execution/runtime.Builder:
	// inject the default reasoning registry and validate the finalized runtime wiring.
	ag.SetReasoningRegistry(NewDefaultReasoningRegistry(
		ag.MainGateway(),
		ag.Config().ExecutionOptions().Model.Model,
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
// 该入口只服务 registry 扩展流；常规 Agent 构造应优先使用
// agent/execution/runtime.Builder。
// 此函数可以安全多次调用 - 只有第一个调用会初始化 。
func InitGlobalRegistry(logger *zap.Logger) {
	globalRegistryOnce.Do(func() {
		GlobalRegistry = NewAgentRegistry(logger)
	})
}

// Deprecated: prefer agent/execution/runtime.Builder for regular construction.
// Use AgentRegistry.Create only when you intentionally rely on the typed
// global-registry extension flow.
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
	gateway        llmcore.Gateway
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
// and main LLM gateway.
func NewCachingResolver(registry *AgentRegistry, gateway llmcore.Gateway, logger *zap.Logger) *CachingResolver {
	return &CachingResolver{
		registry: registry,
		gateway:  gateway,
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
			cfg.Tools.AllowedTools = append([]string(nil), toolNames...)
			cfg.Runtime.Tools = append([]string(nil), toolNames...)
		}
		ag, err := r.registry.Create(cfg, r.gateway, r.memory, r.tools, nil, r.logger)
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
	if provider := compatProviderFromGateway(r.gateway); provider != nil {
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

// =============================================================================
// LifecycleManager (merged from lifecycle.go)
// =============================================================================

// LifecycleManager 管理 Agent 的生命周期
// 提供启动、停止、健康检查等功能
type LifecycleManager struct {
	agent  Agent
	logger *zap.Logger

	mu       sync.RWMutex
	running  bool
	stopChan chan struct{}
	doneChan chan struct{}

	// 健康检查
	healthCheckInterval time.Duration
	lastHealthCheck     time.Time
	healthStatus        HealthStatus
}

// HealthStatus 健康状态
//
// L-002: 项目中存在两个 HealthStatus 结构体，服务于不同层次：
//   - agent.HealthStatus（本定义）— Agent 层健康状态，包含 State 字段
//   - llmcore.HealthStatus — LLM Provider 层健康状态，包含 Latency/ErrorRate 字段
//
// 两者字段不同，无法统一。如需跨层传递，请使用各自的转换函数。
type HealthStatus struct {
	Healthy   bool      `json:"healthy"`
	State     State     `json:"state"`
	LastCheck time.Time `json:"last_check"`
	Message   string    `json:"message,omitempty"`
}

// NewLifecycleManager 创建生命周期管理器
func NewLifecycleManager(agent Agent, logger *zap.Logger) *LifecycleManager {
	return &LifecycleManager{
		agent:               agent,
		logger:              logger,
		stopChan:            make(chan struct{}),
		doneChan:            make(chan struct{}),
		healthCheckInterval: 30 * time.Second,
		healthStatus: HealthStatus{
			Healthy: false,
			State:   agent.State(),
		},
	}
}

// Start 启动 Agent
func (lm *LifecycleManager) Start(ctx context.Context) error {
	lm.mu.Lock()
	if lm.running {
		lm.mu.Unlock()
		return fmt.Errorf("agent already running")
	}
	lm.running = true
	lm.mu.Unlock()

	lm.logger.Info("starting agent lifecycle manager",
		zap.String("agent_id", lm.agent.ID()),
		zap.String("agent_name", lm.agent.Name()),
	)

	// 初始化 Agent
	if err := lm.agent.Init(ctx); err != nil {
		lm.mu.Lock()
		lm.running = false
		lm.mu.Unlock()
		return fmt.Errorf("failed to initialize agent: %w", err)
	}

	// 启动健康检查，将 stopChan/doneChan 快照传入，避免 Restart 竞态
	go lm.healthCheckLoop(ctx, lm.stopChan, lm.doneChan)

	lm.logger.Info("agent lifecycle manager started")
	return nil
}

// Stop 停止 Agent
func (lm *LifecycleManager) Stop(ctx context.Context) error {
	lm.mu.Lock()
	if !lm.running {
		lm.mu.Unlock()
		return fmt.Errorf("agent not running")
	}
	// 在同一个临界区内设置 running = false 并 close channel，
	// 防止两个并发 Stop() 都通过检查后 double-close panic。
	lm.running = false
	close(lm.stopChan)
	lm.mu.Unlock()

	lm.logger.Info("stopping agent lifecycle manager",
		zap.String("agent_id", lm.agent.ID()),
	)

	// 等待健康检查循环结束
	select {
	case <-lm.doneChan:
	case <-time.After(5 * time.Second):
		lm.logger.Warn("health check loop did not stop in time")
	}

	// 清理 Agent 资源
	if err := lm.agent.Teardown(ctx); err != nil {
		lm.logger.Error("failed to teardown agent", zap.Error(err))
		return err
	}

	lm.logger.Info("agent lifecycle manager stopped")
	return nil
}

// IsRunning 检查是否正在运行
func (lm *LifecycleManager) IsRunning() bool {
	lm.mu.RLock()
	defer lm.mu.RUnlock()
	return lm.running
}

// GetHealthStatus 获取健康状态
func (lm *LifecycleManager) GetHealthStatus() HealthStatus {
	lm.mu.RLock()
	defer lm.mu.RUnlock()
	return lm.healthStatus
}

// healthCheckLoop 健康检查循环
// stop 和 done 作为参数传入，避免 Restart 替换 lm.stopChan/lm.doneChan 后
// 旧 goroutine 通过 lm 字段访问到新 channel 导致竞态。
func (lm *LifecycleManager) healthCheckLoop(ctx context.Context, stop <-chan struct{}, done chan struct{}) {
	defer close(done)

	ticker := time.NewTicker(lm.healthCheckInterval)
	defer ticker.Stop()

	// 立即执行一次健康检查
	lm.performHealthCheck()

	for {
		select {
		case <-stop:
			lm.logger.Info("health check loop stopped")
			return
		case <-ticker.C:
			lm.performHealthCheck()
		case <-ctx.Done():
			lm.logger.Info("health check loop cancelled")
			return
		}
	}
}

// performHealthCheck 执行健康检查
func (lm *LifecycleManager) performHealthCheck() {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	state := lm.agent.State()
	now := time.Now()

	// 判断健康状态
	healthy := state == StateReady || state == StateRunning
	message := ""

	if !healthy {
		message = fmt.Sprintf("agent in unhealthy state: %s", state)
	}

	lm.healthStatus = HealthStatus{
		Healthy:   healthy,
		State:     state,
		LastCheck: now,
		Message:   message,
	}

	lm.lastHealthCheck = now

	if !healthy {
		lm.logger.Warn("agent health check failed",
			zap.String("agent_id", lm.agent.ID()),
			zap.String("state", string(state)),
			zap.String("message", message),
		)
	} else {
		lm.logger.Debug("agent health check passed",
			zap.String("agent_id", lm.agent.ID()),
			zap.String("state", string(state)),
		)
	}
}

// Restart 重启 Agent
func (lm *LifecycleManager) Restart(ctx context.Context) error {
	lm.logger.Info("restarting agent",
		zap.String("agent_id", lm.agent.ID()),
	)

	// 停止
	if err := lm.Stop(ctx); err != nil {
		return fmt.Errorf("failed to stop agent: %w", err)
	}

	// 在锁保护下重新创建通道，防止与并发读取竞争
	lm.mu.Lock()
	lm.stopChan = make(chan struct{})
	lm.doneChan = make(chan struct{})
	lm.mu.Unlock()

	// 启动
	if err := lm.Start(ctx); err != nil {
		return fmt.Errorf("failed to start agent: %w", err)
	}

	lm.logger.Info("agent restarted successfully")
	return nil
}

// =============================================================================
// ManagedLSP (merged from lifecycle.go)
// =============================================================================

const (
	defaultLSPServerName    = "agentflow-lsp"
	defaultLSPServerVersion = "0.1.0"
)

// ManagedLSP 封装了进程内的 LSP client/server 及其生命周期。
type ManagedLSP struct {
	Client *agentlsp.LSPClient
	Server *agentlsp.LSPServer

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	clientToServerReader *io.PipeReader
	clientToServerWriter *io.PipeWriter
	serverToClientReader *io.PipeReader
	serverToClientWriter *io.PipeWriter

	logger *zap.Logger
}

// NewManagedLSP 创建并启动一个进程内的 LSP runtime。
func NewManagedLSP(info agentlsp.ServerInfo, logger *zap.Logger) *ManagedLSP {
	if logger == nil {
		logger = zap.NewNop()
	}

	if strings.TrimSpace(info.Name) == "" {
		info.Name = defaultLSPServerName
	}
	if strings.TrimSpace(info.Version) == "" {
		info.Version = defaultLSPServerVersion
	}

	clientToServerReader, clientToServerWriter := io.Pipe()
	serverToClientReader, serverToClientWriter := io.Pipe()

	server := agentlsp.NewLSPServer(info, clientToServerReader, serverToClientWriter, logger)
	client := agentlsp.NewLSPClient(serverToClientReader, clientToServerWriter, logger)

	runtimeCtx, cancel := context.WithCancel(context.Background())
	runtime := &ManagedLSP{
		Client:               client,
		Server:               server,
		ctx:                  runtimeCtx,
		cancel:               cancel,
		clientToServerReader: clientToServerReader,
		clientToServerWriter: clientToServerWriter,
		serverToClientReader: serverToClientReader,
		serverToClientWriter: serverToClientWriter,
		logger:               logger.With(zap.String("component", "managed_lsp")),
	}

	runtime.start()

	return runtime
}

func (m *ManagedLSP) start() {
	m.wg.Add(2)

	go func() {
		defer m.wg.Done()
		if err := m.Server.Start(m.ctx); err != nil && err != context.Canceled {
			m.logger.Debug("managed lsp server stopped", zap.Error(err))
		}
	}()

	go func() {
		defer m.wg.Done()
		if err := m.Client.Start(m.ctx); err != nil && err != context.Canceled {
			m.logger.Debug("managed lsp client loop stopped", zap.Error(err))
		}
	}()
}

// Close 关闭 runtime 并回收后台 goroutine。
func (m *ManagedLSP) Close() error {
	if m == nil {
		return nil
	}

	m.cancel()
	_ = m.clientToServerReader.Close()
	_ = m.clientToServerWriter.Close()
	_ = m.serverToClientReader.Close()
	_ = m.serverToClientWriter.Close()
	m.wg.Wait()
	return nil
}

// =============================================================================
// AsyncExecutor / SubagentManager / RealtimeCoordinator (merged from async_execution.go)
// =============================================================================

// executionResult bundles the outcome of an async execution into a single value,
// eliminating the dual-channel select race between resultCh and errorCh.
type executionResult = registrycore.ExecutionResult[Output]

// AsyncExecutor 异步 Agent 执行器（基于 Anthropic 2026 标准）
// 支持异步 Subagent 创建和实时协调
type AsyncExecutor struct {
	agent   Agent
	manager *SubagentManager
	logger  *zap.Logger
}

// NewAsyncExecutor 创建异步执行器
func NewAsyncExecutor(agent Agent, logger *zap.Logger) *AsyncExecutor {
	return &AsyncExecutor{
		agent:   agent,
		manager: NewSubagentManager(logger),
		logger:  logger.With(zap.String("component", "async_executor")),
	}
}

// ExecuteAsync 异步执行任务
func (e *AsyncExecutor) ExecuteAsync(ctx context.Context, input *Input) (*AsyncExecution, error) {
	execution := newAsyncExecution(generateExecutionID(), e.agent.ID(), input)

	e.logger.Info("starting async execution",
		zap.String("execution_id", execution.ID),
		zap.String("agent_id", e.agent.ID()),
	)

	registrycore.RunExecution(registrycore.ExecutionRunner[Input, Output, Agent, *AsyncExecution]{
		Context:      ctx,
		Agent:        e.agent,
		Input:        input,
		Exec:         execution,
		ExecutionID:  execution.ID,
		AgentID:      e.agent.ID(),
		Logger:       e.logger,
		TracerName:   "agent",
		SpanName:     "async_execution",
		PanicMessage: "async execution panicked",
		Execute: func(execCtx context.Context, agent Agent, execInput *Input) (*Output, error) {
			return agent.Execute(execCtx, execInput)
		},
		Callbacks: asyncExecutionCallbacks(),
	})

	return execution, nil
}

// ExecuteWithSubagents 使用 Subagents 并行执行
func (e *AsyncExecutor) ExecuteWithSubagents(ctx context.Context, input *Input, subagents []Agent) (*Output, error) {
	e.logger.Info("executing with subagents",
		zap.String("agent_id", e.agent.ID()),
		zap.Int("subagents", len(subagents)),
	)

	// 1. 创建并行执行上下文
	execCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	results, err := registrycore.CollectParallelResults(registrycore.ParallelExecutionConfig[Input, Output, Agent, *AsyncExecution]{
		Context:   execCtx,
		Input:     input,
		Subagents: subagents,
		Spawn:     e.manager.SpawnSubagent,
		Wait: func(exec *AsyncExecution, waitCtx context.Context) (*Output, error) {
			return exec.Wait(waitCtx)
		},
		OnSpawnError: func(subagent Agent, err error) {
			e.logger.Warn("failed to spawn subagent",
				zap.String("subagent_id", subagent.ID()),
				zap.String("task_type", "subagent_parallel"),
				zap.Error(err),
			)
		},
		OnWaitError: func(exec *AsyncExecution, err error) {
			e.logger.Warn("subagent execution failed",
				zap.String("execution_id", exec.ID),
				zap.String("subagent_id", exec.AgentID),
				zap.String("task_type", "subagent_parallel"),
				zap.Error(err),
			)
		},
	})
	if err != nil {
		return nil, err
	}

	combined := e.combineResults(results)

	e.logger.Info("subagent execution completed",
		zap.Int("successful", len(results)),
		zap.Int("total", len(subagents)),
	)

	return combined, nil
}

// combineResults 合并多个 Subagent 结果
func (e *AsyncExecutor) combineResults(results []*Output) *Output {
	if len(results) == 1 {
		return results[0]
	}

	combined := &Output{
		TraceID: results[0].TraceID,
		Content: "",
		Metadata: map[string]any{
			"subagent_count": len(results),
		},
	}

	var sb strings.Builder
	var maxDuration time.Duration
	for i, result := range results {
		sb.WriteString(fmt.Sprintf("## Subagent %d\n%s\n\n", i+1, result.Content))
		combined.TokensUsed += result.TokensUsed
		combined.Cost += result.Cost
		if result.Duration > maxDuration {
			maxDuration = result.Duration
		}
	}
	combined.Content = sb.String()
	combined.Duration = maxDuration

	return combined
}

// AsyncExecution 异步执行状态
//
// 重要：调用者必须调用 Wait() 或从 doneCh 读取结果，否则发送 goroutine
// 可能因 ctx 取消而丢弃结果，但 doneCh 本身（带 1 缓冲）不会泄漏。
// 如果不再需要结果，请确保取消传入的 context 以释放相关资源。(T-013)
type AsyncExecution struct {
	ID        string
	AgentID   string
	Input     *Input
	StartTime time.Time

	// mu protects mutable fields: status, errMsg, output, endTime.
	mu      sync.RWMutex
	status  ExecutionStatus
	errMsg  string
	output  *Output
	endTime time.Time

	doneCh     chan struct{}
	doneOnce   sync.Once
	waitResult executionResult
}

// setCompleted atomically marks the execution as completed.
func (e *AsyncExecution) setCompleted(output *Output) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.status = ExecutionStatusCompleted
	e.output = output
	e.endTime = time.Now()
}

// setFailed atomically marks the execution as failed.
func (e *AsyncExecution) setFailed(err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.status = ExecutionStatusFailed
	e.errMsg = err.Error()
	e.endTime = time.Now()
}

// GetStatus returns the current execution status.
func (e *AsyncExecution) GetStatus() ExecutionStatus {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.status
}

// GetError returns the error message, if any.
func (e *AsyncExecution) GetError() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.errMsg
}

// GetOutput returns the execution output, if completed.
func (e *AsyncExecution) GetOutput() *Output {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.output
}

// GetEndTime returns when the execution finished.
func (e *AsyncExecution) GetEndTime() time.Time {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.endTime
}

// ExecutionStatus 执行状态
type ExecutionStatus string

const (
	ExecutionStatusPending   ExecutionStatus = "pending"
	ExecutionStatusRunning   ExecutionStatus = "running"
	ExecutionStatusCompleted ExecutionStatus = "completed"
	ExecutionStatusFailed    ExecutionStatus = "failed"
	ExecutionStatusCanceled  ExecutionStatus = "canceled"
)

// Wait 等待执行完成。可安全地被多次调用，
// 首次调用消费 doneCh 并缓存结果，后续调用直接返回缓存值。
func (e *AsyncExecution) Wait(ctx context.Context) (*Output, error) {
	select {
	case <-e.doneCh:
		return e.waitResult.Output, e.waitResult.Err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (e *AsyncExecution) notifyDone(res executionResult) {
	e.doneOnce.Do(func() {
		e.waitResult = res
		close(e.doneCh)
	})
}

// SubagentManager Subagent 管理器
type SubagentManager struct {
	core *registrycore.SubagentManager[Input, Output, Agent, *AsyncExecution, ExecutionStatus]
}

// NewSubagentManager 创建 Subagent 管理器
func NewSubagentManager(logger *zap.Logger) *SubagentManager {
	return &SubagentManager{
		core: registrycore.NewSubagentManager(registrycore.ManagerConfig[Input, Output, Agent, *AsyncExecution, ExecutionStatus]{
			Logger:         logger,
			Component:      "subagent_manager",
			NewExecutionID: generateExecutionID,
			CloneInput:     copyInput,
			PrepareContext: prepareSubagentContext,
			NewExecution:   newAsyncExecution,
			Callbacks:      asyncExecutionCallbacks(),
			GetStatus: func(exec *AsyncExecution) ExecutionStatus {
				return exec.GetStatus()
			},
			GetEndTime: func(exec *AsyncExecution) time.Time {
				return exec.GetEndTime()
			},
			GetID: func(exec *AsyncExecution) string {
				return exec.ID
			},
			CompletedStatuses: []ExecutionStatus{
				ExecutionStatusCompleted,
				ExecutionStatusFailed,
			},
		}),
	}
}

// Close 停止自动清理 goroutine。
func (m *SubagentManager) Close() {
	if m == nil || m.core == nil {
		return
	}
	m.core.Close()
}

// SpawnSubagent 创建 Subagent 执行
func (m *SubagentManager) SpawnSubagent(ctx context.Context, subagent Agent, input *Input) (*AsyncExecution, error) {
	return m.core.SpawnSubagent(ctx, subagent, input)
}

// GetExecution 获取执行状态
func (m *SubagentManager) GetExecution(executionID string) (*AsyncExecution, error) {
	if m == nil || m.core == nil {
		return nil, fmt.Errorf("execution not found: %s", executionID)
	}
	return m.core.GetExecution(executionID)
}

// ListExecutions 列出所有执行
func (m *SubagentManager) ListExecutions() []*AsyncExecution {
	if m == nil || m.core == nil {
		return nil
	}
	return m.core.ListExecutions()
}

// CleanupCompleted 清理已完成的执行
func (m *SubagentManager) CleanupCompleted(olderThan time.Duration) int {
	if m == nil || m.core == nil {
		return 0
	}
	return m.core.CleanupCompleted(olderThan)
}

// generateExecutionID 生成执行 ID
// Uses UUID for distributed uniqueness.
func generateExecutionID() string {
	return registrycore.GenerateExecutionID()
}

// copyInput 深拷贝 Input，防止并发 subagent 共享 map 导致 data race。
func copyInput(src *Input) *Input {
	dst := &Input{
		TraceID:   src.TraceID,
		TenantID:  src.TenantID,
		UserID:    src.UserID,
		ChannelID: src.ChannelID,
		Content:   src.Content,
	}
	if src.Context != nil {
		dst.Context = make(map[string]any, len(src.Context))
		for k, v := range src.Context {
			dst.Context[k] = v
		}
	}
	if src.Variables != nil {
		dst.Variables = make(map[string]string, len(src.Variables))
		for k, v := range src.Variables {
			dst.Variables[k] = v
		}
	}
	if src.Overrides != nil {
		cp := *src.Overrides
		if src.Overrides.Stop != nil {
			cp.Stop = make([]string, len(src.Overrides.Stop))
			copy(cp.Stop, src.Overrides.Stop)
		}
		if src.Overrides.Metadata != nil {
			cp.Metadata = make(map[string]string, len(src.Overrides.Metadata))
			for k, v := range src.Overrides.Metadata {
				cp.Metadata[k] = v
			}
		}
		if src.Overrides.Tags != nil {
			cp.Tags = make([]string, len(src.Overrides.Tags))
			copy(cp.Tags, src.Overrides.Tags)
		}
		dst.Overrides = &cp
	}
	return dst
}

// ====== 实时协调器 ======

// RealtimeCoordinator 实时协调器
// 支持 Subagents 之间的实时通信和协调
type RealtimeCoordinator struct {
	manager  *SubagentManager
	eventBus EventBus
	logger   *zap.Logger
}

// NewRealtimeCoordinator 创建实时协调器
func NewRealtimeCoordinator(manager *SubagentManager, eventBus EventBus, logger *zap.Logger) *RealtimeCoordinator {
	return &RealtimeCoordinator{
		manager:  manager,
		eventBus: eventBus,
		logger:   logger.With(zap.String("component", "realtime_coordinator")),
	}
}

// CoordinateSubagents 协调多个 Subagents
func (c *RealtimeCoordinator) CoordinateSubagents(ctx context.Context, subagents []Agent, input *Input) (*Output, error) {
	c.logger.Info("coordinating subagents",
		zap.Int("count", len(subagents)),
	)

	results, err := registrycore.CollectParallelResults(registrycore.ParallelExecutionConfig[Input, Output, Agent, *AsyncExecution]{
		Context:   ctx,
		Input:     input,
		Subagents: subagents,
		Spawn:     c.manager.SpawnSubagent,
		Wait: func(exec *AsyncExecution, waitCtx context.Context) (*Output, error) {
			return exec.Wait(waitCtx)
		},
		OnSpawnError: func(subagent Agent, err error) {
			c.logger.Warn("failed to spawn subagent",
				zap.String("subagent_id", subagent.ID()),
				zap.Error(err),
			)
		},
		OnWaitError: func(exec *AsyncExecution, err error) {
			c.logger.Warn("subagent failed",
				zap.String("execution_id", exec.ID),
				zap.Error(err),
			)
		},
		OnSuccess: func(exec *AsyncExecution, output *Output) {
			if c.eventBus == nil {
				return
			}
			c.eventBus.Publish(&SubagentCompletedEvent{
				ExecutionID: exec.ID,
				AgentID:     exec.AgentID,
				Output:      output,
				Timestamp_:  time.Now(),
			})
		},
		IgnoreContextCancellation: true,
	})
	if err != nil {
		return nil, err
	}

	// 4. 合并结果
	combined := &Output{
		TraceID: input.TraceID,
		Content: "",
		Metadata: map[string]any{
			"subagent_count": len(results),
		},
	}

	var sb strings.Builder
	var maxDuration time.Duration
	for i, result := range results {
		sb.WriteString(fmt.Sprintf("## Result %d\n%s\n\n", i+1, result.Content))
		combined.TokensUsed += result.TokensUsed
		combined.Cost += result.Cost
		if result.Duration > maxDuration {
			maxDuration = result.Duration
		}
	}
	combined.Content = sb.String()
	combined.Duration = maxDuration

	c.logger.Info("coordination completed",
		zap.Int("successful", len(results)),
		zap.Int("total", len(subagents)),
	)

	return combined, nil
}

func asyncExecutionCallbacks() registrycore.ExecutionCallbacks[*AsyncExecution, Output] {
	return registrycore.ExecutionCallbacks[*AsyncExecution, Output]{
		SetCompleted: func(exec *AsyncExecution, output *Output) {
			exec.setCompleted(output)
		},
		SetFailed: func(exec *AsyncExecution, err error) {
			exec.setFailed(err)
		},
		NotifyDone: func(exec *AsyncExecution, result executionResult) {
			exec.notifyDone(result)
		},
	}
}

func newAsyncExecution(executionID, agentID string, input *Input) *AsyncExecution {
	return &AsyncExecution{
		ID:        executionID,
		AgentID:   agentID,
		Input:     input,
		status:    ExecutionStatusRunning,
		StartTime: time.Now(),
		doneCh:    make(chan struct{}),
	}
}

func prepareSubagentContext(ctx context.Context, executionID string) context.Context {
	childCtx := ctx
	if parentRunID, ok := types.RunID(ctx); ok {
		childCtx = types.WithParentRunID(childCtx, parentRunID)
	}
	childCtx = types.WithSpanID(childCtx, "span_"+uuid.New().String())
	childCtx = types.WithRunID(childCtx, executionID)
	return childCtx
}

// SubagentCompletedEvent Subagent 完成事件
type SubagentCompletedEvent struct {
	ExecutionID string
	AgentID     string
	Output      *Output
	Timestamp_  time.Time
}

func (e *SubagentCompletedEvent) Timestamp() time.Time { return e.Timestamp_ }
func (e *SubagentCompletedEvent) Type() EventType      { return EventSubagentCompleted }

// =============================================================================
// SteeringChannel / SessionManager (merged from steering.go)
// =============================================================================

// 类型别名：方便包内使用，避免到处写 types.SteeringXxx
type SteeringMessage = types.SteeringMessage
type SteeringMessageType = types.SteeringMessageType

// 常量别名
const (
	SteeringTypeGuide       = types.SteeringTypeGuide
	SteeringTypeStopAndSend = types.SteeringTypeStopAndSend
)

// SteeringChannel 双向通信通道，用于在流式生成过程中注入用户指令
type SteeringChannel struct {
	ch     chan SteeringMessage
	closed atomic.Bool
}

// NewSteeringChannel 创建一个带缓冲的 steering 通道
func NewSteeringChannel(bufSize int) *SteeringChannel {
	if bufSize <= 0 {
		bufSize = 1
	}
	return &SteeringChannel{
		ch: make(chan SteeringMessage, bufSize),
	}
}

var (
	// ErrSteeringChannelClosed 通道已关闭
	ErrSteeringChannelClosed = errors.New("steering channel is closed")
	// ErrSteeringChannelFull 通道已满（非阻塞发送失败）
	ErrSteeringChannelFull = errors.New("steering channel is full")
)

// Send 向通道发送一条 steering 消息（非阻塞，panic-safe）
func (sc *SteeringChannel) Send(msg SteeringMessage) (err error) {
	if sc.closed.Load() {
		return ErrSteeringChannelClosed
	}
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}
	// recover 防止 Send 和 Close 之间的 TOCTOU 竞态导致 send-on-closed-channel panic
	defer func() {
		if r := recover(); r != nil {
			err = ErrSteeringChannelClosed
		}
	}()
	select {
	case sc.ch <- msg:
		return nil
	default:
		return ErrSteeringChannelFull
	}
}

// Receive 返回底层 channel，用于 select 监听
func (sc *SteeringChannel) Receive() <-chan SteeringMessage {
	return sc.ch
}

// Close 关闭通道
func (sc *SteeringChannel) Close() {
	if sc.closed.CompareAndSwap(false, true) {
		close(sc.ch)
	}
}

// IsClosed 检查通道是否已关闭
func (sc *SteeringChannel) IsClosed() bool {
	return sc.closed.Load()
}

// --- context 注入/提取 ---

type steeringChannelKey struct{}

// WithSteeringChannel 将 SteeringChannel 注入 context
func WithSteeringChannel(ctx context.Context, ch *SteeringChannel) context.Context {
	if ch == nil {
		return ctx
	}
	return context.WithValue(ctx, steeringChannelKey{}, ch)
}

// SteeringChannelFromContext 从 context 中提取 SteeringChannel
func SteeringChannelFromContext(ctx context.Context) (*SteeringChannel, bool) {
	if ctx == nil {
		return nil, false
	}
	ch, ok := ctx.Value(steeringChannelKey{}).(*SteeringChannel)
	return ch, ok && ch != nil
}

// steerChOrNil 返回 steering channel 的接收端，如果不存在则返回 nil（select 中永远不会触发）
func steerChOrNil(ch *SteeringChannel) <-chan SteeringMessage {
	if ch == nil {
		return nil
	}
	return ch.Receive()
}

// ExecutionSession 跟踪一个活跃的流式执行。
// SteeringChannel 是唯一的运行状态源，避免额外状态字段分叉。
type ExecutionSession struct {
	ID         string           `json:"id"`
	AgentID    string           `json:"agent_id"`
	SteeringCh *SteeringChannel `json:"-"`
	CreatedAt  time.Time        `json:"created_at"`
}

// Status 返回当前会话状态（单一状态源：SteeringChannel.IsClosed）。
func (s *ExecutionSession) Status() string {
	if s.SteeringCh.IsClosed() {
		return "completed"
	}
	return "running"
}

// Complete 标记会话为已完成（关闭 steering channel）。
func (s *ExecutionSession) Complete() {
	s.SteeringCh.Close()
}

// IsRunning 检查会话是否仍在运行。
func (s *ExecutionSession) IsRunning() bool {
	return !s.SteeringCh.IsClosed()
}

// SessionManager 管理活跃的流式执行会话（内存 map + 自动过期清理）。
type SessionManager struct {
	sessions sync.Map
	stopOnce sync.Once
	stopCh   chan struct{}
}

// NewSessionManager 创建会话管理器并启动后台清理 goroutine。
func NewSessionManager() *SessionManager {
	m := &SessionManager{
		stopCh: make(chan struct{}),
	}
	go m.cleanupLoop()
	return m
}

// Create 创建一个新的执行会话。
func (m *SessionManager) Create(agentID string) *ExecutionSession {
	sess := &ExecutionSession{
		ID:         fmt.Sprintf("exec_%s", uuid.New().String()[:12]),
		AgentID:    agentID,
		SteeringCh: NewSteeringChannel(4),
		CreatedAt:  time.Now(),
	}
	m.sessions.Store(sess.ID, sess)
	return sess
}

// Get 根据 ID 获取会话。
func (m *SessionManager) Get(id string) (*ExecutionSession, bool) {
	v, ok := m.sessions.Load(id)
	if !ok {
		return nil, false
	}
	return v.(*ExecutionSession), true
}

// Remove 移除会话并关闭其 steering channel。
func (m *SessionManager) Remove(id string) {
	if v, loaded := m.sessions.LoadAndDelete(id); loaded {
		v.(*ExecutionSession).Complete()
	}
}

// Cleanup 清理过期会话：已完成的超过 maxAge 清理，活跃的不强制终止。
func (m *SessionManager) Cleanup(maxAge time.Duration) {
	cutoff := time.Now().Add(-maxAge)
	m.sessions.Range(func(key, value any) bool {
		sess := value.(*ExecutionSession)
		if sess.CreatedAt.Before(cutoff) && !sess.IsRunning() {
			m.sessions.Delete(key)
		}
		return true
	})
}

// Stop 停止后台清理 goroutine。
func (m *SessionManager) Stop() {
	m.stopOnce.Do(func() {
		close(m.stopCh)
	})
}

// cleanupLoop 每 60s 清理超过 30min 的过期会话。
func (m *SessionManager) cleanupLoop() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.Cleanup(30 * time.Minute)
		case <-m.stopCh:
			return
		}
	}
}
