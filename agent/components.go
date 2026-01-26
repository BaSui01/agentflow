// Package agent provides the core agent framework for AgentFlow.
package agent

import (
	"context"
	"sync"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// ============================================================
// Agent Components
// These components break down BaseAgent's responsibilities into
// smaller, focused units following the Single Responsibility Principle.
// ============================================================

// AgentIdentity manages agent identity information.
type AgentIdentity struct {
	id          string
	name        string
	agentType   AgentType
	description string
}

// NewAgentIdentity creates a new AgentIdentity.
func NewAgentIdentity(id, name string, agentType AgentType) *AgentIdentity {
	return &AgentIdentity{
		id:        id,
		name:      name,
		agentType: agentType,
	}
}

// ID returns the agent's unique identifier.
func (i *AgentIdentity) ID() string { return i.id }

// Name returns the agent's name.
func (i *AgentIdentity) Name() string { return i.name }

// Type returns the agent's type.
func (i *AgentIdentity) Type() AgentType { return i.agentType }

// Description returns the agent's description.
func (i *AgentIdentity) Description() string { return i.description }

// SetDescription sets the agent's description.
func (i *AgentIdentity) SetDescription(desc string) { i.description = desc }

// ============================================================
// State Manager (Lightweight state management for ModularAgent)
// ============================================================

// StateManager manages agent state transitions (lightweight version).
type StateManager struct {
	state   State
	stateMu sync.RWMutex
	execMu  sync.Mutex
	bus     EventBus
	logger  *zap.Logger
	agentID string
}

// NewStateManager creates a new StateManager.
func NewStateManager(agentID string, bus EventBus, logger *zap.Logger) *StateManager {
	return &StateManager{
		state:   StateInit,
		bus:     bus,
		logger:  logger,
		agentID: agentID,
	}
}

// State returns the current state.
func (sm *StateManager) State() State {
	sm.stateMu.RLock()
	defer sm.stateMu.RUnlock()
	return sm.state
}

// Transition performs a state transition with validation.
func (sm *StateManager) Transition(ctx context.Context, to State) error {
	sm.stateMu.Lock()
	defer sm.stateMu.Unlock()

	from := sm.state
	if !CanTransition(from, to) {
		return ErrInvalidTransition{From: from, To: to}
	}

	sm.state = to
	sm.logger.Info("state transition",
		zap.String("from", string(from)),
		zap.String("to", string(to)))

	// Publish state change event
	if sm.bus != nil {
		sm.bus.Publish(&StateChangeEvent{
			AgentID_:   sm.agentID,
			FromState:  from,
			ToState:    to,
			Timestamp_: time.Now(),
		})
	}

	return nil
}

// TryLockExec attempts to acquire the execution lock.
func (sm *StateManager) TryLockExec() bool {
	return sm.execMu.TryLock()
}

// UnlockExec releases the execution lock.
func (sm *StateManager) UnlockExec() {
	sm.execMu.Unlock()
}

// EnsureReady checks if the agent is in ready state.
func (sm *StateManager) EnsureReady() error {
	if sm.State() != StateReady {
		return ErrAgentNotReady
	}
	return nil
}

// ============================================================
// LLM Executor
// ============================================================

// LLMExecutor handles LLM interactions.
type LLMExecutor struct {
	provider       llm.Provider
	model          string
	maxTokens      int
	temperature    float32
	contextManager ContextManager
	logger         *zap.Logger
}

// LLMExecutorConfig configures the LLM executor.
type LLMExecutorConfig struct {
	Model       string
	MaxTokens   int
	Temperature float32
}

// NewLLMExecutor creates a new LLMExecutor.
func NewLLMExecutor(provider llm.Provider, config LLMExecutorConfig, logger *zap.Logger) *LLMExecutor {
	return &LLMExecutor{
		provider:    provider,
		model:       config.Model,
		maxTokens:   config.MaxTokens,
		temperature: config.Temperature,
		logger:      logger,
	}
}

// SetContextManager sets the context manager for message optimization.
func (e *LLMExecutor) SetContextManager(cm ContextManager) {
	e.contextManager = cm
}

// Provider returns the underlying LLM provider.
func (e *LLMExecutor) Provider() llm.Provider {
	return e.provider
}

// Complete sends a completion request to the LLM.
func (e *LLMExecutor) Complete(ctx context.Context, messages []llm.Message) (*llm.ChatResponse, error) {
	if e.provider == nil {
		return nil, ErrProviderNotSet
	}

	// Apply context optimization if available
	if e.contextManager != nil && len(messages) > 1 {
		query := extractLastUserQuery(messages)
		optimized, err := e.contextManager.PrepareMessages(ctx, messages, query)
		if err != nil {
			e.logger.Warn("context optimization failed", zap.Error(err))
		} else {
			messages = optimized
		}
	}

	req := &llm.ChatRequest{
		Model:       e.model,
		Messages:    messages,
		MaxTokens:   e.maxTokens,
		Temperature: e.temperature,
	}

	return e.provider.Completion(ctx, req)
}

// Stream sends a streaming request to the LLM.
func (e *LLMExecutor) Stream(ctx context.Context, messages []llm.Message) (<-chan llm.StreamChunk, error) {
	if e.provider == nil {
		return nil, ErrProviderNotSet
	}

	req := &llm.ChatRequest{
		Model:       e.model,
		Messages:    messages,
		MaxTokens:   e.maxTokens,
		Temperature: e.temperature,
	}

	return e.provider.Stream(ctx, req)
}

// extractLastUserQuery extracts the last user message content.
func extractLastUserQuery(messages []llm.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == llm.RoleUser {
			return messages[i].Content
		}
	}
	return ""
}

// ============================================================
// Extension Manager
// ============================================================

// ExtensionManager manages optional agent extensions.
type ExtensionManager struct {
	reflection     types.ReflectionExtension
	toolSelection  types.ToolSelectionExtension
	promptEnhancer types.PromptEnhancerExtension
	skills         types.SkillsExtension
	mcp            types.MCPExtension
	enhancedMemory types.EnhancedMemoryExtension
	observability  types.ObservabilityExtension
	guardrails     types.GuardrailsExtension
	logger         *zap.Logger
}

// NewExtensionManager creates a new ExtensionManager.
func NewExtensionManager(logger *zap.Logger) *ExtensionManager {
	return &ExtensionManager{
		logger: logger,
	}
}

// SetReflection sets the reflection extension.
func (em *ExtensionManager) SetReflection(ext types.ReflectionExtension) {
	em.reflection = ext
	em.logger.Info("reflection extension registered")
}

// SetToolSelection sets the tool selection extension.
func (em *ExtensionManager) SetToolSelection(ext types.ToolSelectionExtension) {
	em.toolSelection = ext
	em.logger.Info("tool selection extension registered")
}

// SetPromptEnhancer sets the prompt enhancer extension.
func (em *ExtensionManager) SetPromptEnhancer(ext types.PromptEnhancerExtension) {
	em.promptEnhancer = ext
	em.logger.Info("prompt enhancer extension registered")
}

// SetSkills sets the skills extension.
func (em *ExtensionManager) SetSkills(ext types.SkillsExtension) {
	em.skills = ext
	em.logger.Info("skills extension registered")
}

// SetMCP sets the MCP extension.
func (em *ExtensionManager) SetMCP(ext types.MCPExtension) {
	em.mcp = ext
	em.logger.Info("MCP extension registered")
}

// SetEnhancedMemory sets the enhanced memory extension.
func (em *ExtensionManager) SetEnhancedMemory(ext types.EnhancedMemoryExtension) {
	em.enhancedMemory = ext
	em.logger.Info("enhanced memory extension registered")
}

// SetObservability sets the observability extension.
func (em *ExtensionManager) SetObservability(ext types.ObservabilityExtension) {
	em.observability = ext
	em.logger.Info("observability extension registered")
}

// SetGuardrails sets the guardrails extension.
func (em *ExtensionManager) SetGuardrails(ext types.GuardrailsExtension) {
	em.guardrails = ext
	em.logger.Info("guardrails extension registered")
}

// Reflection returns the reflection extension.
func (em *ExtensionManager) Reflection() types.ReflectionExtension { return em.reflection }

// ToolSelection returns the tool selection extension.
func (em *ExtensionManager) ToolSelection() types.ToolSelectionExtension { return em.toolSelection }

// PromptEnhancer returns the prompt enhancer extension.
func (em *ExtensionManager) PromptEnhancer() types.PromptEnhancerExtension { return em.promptEnhancer }

// Skills returns the skills extension.
func (em *ExtensionManager) Skills() types.SkillsExtension { return em.skills }

// MCP returns the MCP extension.
func (em *ExtensionManager) MCP() types.MCPExtension { return em.mcp }

// EnhancedMemory returns the enhanced memory extension.
func (em *ExtensionManager) EnhancedMemory() types.EnhancedMemoryExtension { return em.enhancedMemory }

// Observability returns the observability extension.
func (em *ExtensionManager) Observability() types.ObservabilityExtension { return em.observability }

// Guardrails returns the guardrails extension.
func (em *ExtensionManager) Guardrails() types.GuardrailsExtension { return em.guardrails }

// HasReflection checks if reflection is available.
func (em *ExtensionManager) HasReflection() bool { return em.reflection != nil }

// HasToolSelection checks if tool selection is available.
func (em *ExtensionManager) HasToolSelection() bool { return em.toolSelection != nil }

// HasGuardrails checks if guardrails are available.
func (em *ExtensionManager) HasGuardrails() bool { return em.guardrails != nil }

// HasObservability checks if observability is available.
func (em *ExtensionManager) HasObservability() bool { return em.observability != nil }

// ============================================================
// Modular Agent (New Architecture)
// ============================================================

// ModularAgent is a refactored agent using composition over inheritance.
// It delegates responsibilities to specialized components.
type ModularAgent struct {
	identity   *AgentIdentity
	stateManager *StateManager
	llm        *LLMExecutor
	extensions *ExtensionManager
	memory     MemoryManager
	tools      ToolManager
	bus        EventBus
	logger     *zap.Logger
}

// ModularAgentConfig configures a ModularAgent.
type ModularAgentConfig struct {
	ID          string
	Name        string
	Type        AgentType
	Description string
	LLM         LLMExecutorConfig
}

// NewModularAgent creates a new ModularAgent.
func NewModularAgent(
	config ModularAgentConfig,
	provider llm.Provider,
	memory MemoryManager,
	tools ToolManager,
	bus EventBus,
	logger *zap.Logger,
) *ModularAgent {
	if logger == nil {
		logger = zap.NewNop()
	}

	agentLogger := logger.With(
		zap.String("agent_id", config.ID),
		zap.String("agent_type", string(config.Type)),
	)

	identity := NewAgentIdentity(config.ID, config.Name, config.Type)
	identity.SetDescription(config.Description)

	return &ModularAgent{
		identity:     identity,
		stateManager: NewStateManager(config.ID, bus, agentLogger),
		llm:          NewLLMExecutor(provider, config.LLM, agentLogger),
		extensions:   NewExtensionManager(agentLogger),
		memory:       memory,
		tools:        tools,
		bus:          bus,
		logger:       agentLogger,
	}
}

// ID returns the agent's ID.
func (a *ModularAgent) ID() string { return a.identity.ID() }

// Name returns the agent's name.
func (a *ModularAgent) Name() string { return a.identity.Name() }

// Type returns the agent's type.
func (a *ModularAgent) Type() AgentType { return a.identity.Type() }

// State returns the current state.
func (a *ModularAgent) State() State { return a.stateManager.State() }

// Init initializes the agent.
func (a *ModularAgent) Init(ctx context.Context) error {
	a.logger.Info("initializing modular agent")

	// Load recent memory if available
	if a.memory != nil {
		records, err := a.memory.LoadRecent(ctx, a.identity.ID(), MemoryShortTerm, 10)
		if err != nil {
			a.logger.Warn("failed to load memory", zap.Error(err))
		} else {
			a.logger.Debug("loaded recent memory", zap.Int("count", len(records)))
		}
	}

	return a.stateManager.Transition(ctx, StateReady)
}

// Teardown cleans up the agent.
func (a *ModularAgent) Teardown(ctx context.Context) error {
	a.logger.Info("tearing down modular agent")
	return nil
}

// Execute executes a task.
func (a *ModularAgent) Execute(ctx context.Context, input *Input) (*Output, error) {
	startTime := time.Now()

	// Ensure agent is ready
	if err := a.stateManager.EnsureReady(); err != nil {
		return nil, err
	}

	// Try to acquire execution lock
	if !a.stateManager.TryLockExec() {
		return nil, ErrAgentBusy
	}
	defer a.stateManager.UnlockExec()

	// Validate input with guardrails if available
	if a.extensions.HasGuardrails() {
		result, err := a.extensions.Guardrails().ValidateInput(ctx, input.Content)
		if err != nil {
			return nil, err
		}
		if !result.Valid {
			return nil, NewError(ErrCodeGuardrailsViolated, "input validation failed")
		}
	}

	// Build messages
	messages := []llm.Message{
		{Role: llm.RoleUser, Content: input.Content},
	}

	// Execute LLM
	resp, err := a.llm.Complete(ctx, messages)
	if err != nil {
		return nil, err
	}

	content := ""
	if len(resp.Choices) > 0 {
		content = resp.Choices[0].Message.Content
	}

	// Validate output with guardrails if available
	if a.extensions.HasGuardrails() {
		result, err := a.extensions.Guardrails().ValidateOutput(ctx, content)
		if err != nil {
			return nil, err
		}
		if !result.Valid {
			return nil, NewError(ErrCodeGuardrailsViolated, "output validation failed")
		}
		if result.Filtered != "" {
			content = result.Filtered
		}
	}

	return &Output{
		TraceID:      input.TraceID,
		Content:      content,
		TokensUsed:   resp.Usage.TotalTokens,
		Duration:     time.Since(startTime),
		FinishReason: resp.Choices[0].FinishReason,
	}, nil
}

// Plan generates an execution plan.
func (a *ModularAgent) Plan(ctx context.Context, input *Input) (*PlanResult, error) {
	// Delegate to LLM with planning prompt
	planPrompt := "Please create a step-by-step plan for: " + input.Content

	messages := []llm.Message{
		{Role: llm.RoleUser, Content: planPrompt},
	}

	resp, err := a.llm.Complete(ctx, messages)
	if err != nil {
		return nil, err
	}

	content := ""
	if len(resp.Choices) > 0 {
		content = resp.Choices[0].Message.Content
	}

	return &PlanResult{
		Steps: parsePlanSteps(content),
	}, nil
}

// Observe processes feedback.
func (a *ModularAgent) Observe(ctx context.Context, feedback *Feedback) error {
	a.logger.Info("observing feedback",
		zap.String("type", feedback.Type),
		zap.String("content", feedback.Content))
	return nil
}

// Extensions returns the extension manager.
func (a *ModularAgent) Extensions() *ExtensionManager {
	return a.extensions
}

// LLM returns the LLM executor.
func (a *ModularAgent) LLM() *LLMExecutor {
	return a.llm
}

// Memory returns the memory manager.
func (a *ModularAgent) Memory() MemoryManager {
	return a.memory
}

// Tools returns the tool manager.
func (a *ModularAgent) Tools() ToolManager {
	return a.tools
}
