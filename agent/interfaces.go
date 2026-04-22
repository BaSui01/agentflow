package agent

import (
	"context"
	"encoding/json"
	"fmt"
	memorycore "github.com/BaSui01/agentflow/agent/capabilities/memory"
	"github.com/BaSui01/agentflow/agent/capabilities/reasoning"
	"github.com/BaSui01/agentflow/llm"
	llmtools "github.com/BaSui01/agentflow/llm/capabilities/tools"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
	"math"
	"sort"
	"strings"
	"time"
)

// =============================================================================
// Workflow-Local Interfaces for Optional Agent Features
// =============================================================================
// These interfaces break circular dependencies between agent/ and its sub-packages
// (agent/skills, agent/protocol/mcp, agent/lsp, agent/memory, agent/observability).
//
// Each interface declares ONLY the methods that agent/ actually calls via type
// assertions in integration.go. The concrete implementations in sub-packages
// implicitly satisfy these interfaces (Go duck typing).
//
// See quality-guidelines.md section 15 for the pattern rationale.
// =============================================================================

// ReflectionRunner executes a task with iterative self-reflection.
// Implemented by: *ReflectionExecutor (agent/reflection.go)
type ReflectionRunner interface {
	ExecuteWithReflection(ctx context.Context, input *Input) (*Output, error)
}

// DynamicToolSelectorRunner dynamically selects tools relevant to a given task.
// Implemented by: *DynamicToolSelector (agent/tool_selector.go)
type DynamicToolSelectorRunner interface {
	SelectTools(ctx context.Context, task string, availableTools []types.ToolSchema) ([]types.ToolSchema, error)
}

// PromptEnhancerRunner enhances user prompts with additional context.
// Implemented by: *PromptEnhancer (agent/prompt_enhancer.go)
type PromptEnhancerRunner interface {
	EnhanceUserPrompt(prompt, context string) (string, error)
}

// SkillDiscoverer discovers skills relevant to a task.
// Implemented by: *skills.DefaultSkillManager (agent/skills/)
type SkillDiscoverer interface {
	DiscoverSkills(ctx context.Context, task string) ([]*types.DiscoveredSkill, error)
}

// MCPServerRunner represents an MCP server instance.
// Implemented by: *mcp.MCPServer (agent/protocol/mcp/)
// Currently used only for nil-check (feature status); no methods called directly.
type MCPServerRunner interface{}

// LSPClientRunner represents an LSP client instance.
// Implemented by: *lsp.LSPClient (agent/lsp/)
// Used in Teardown for Shutdown call.
type LSPClientRunner interface {
	Shutdown(ctx context.Context) error
}

// LSPLifecycleOwner represents an optional lifecycle owner for LSP (e.g. *ManagedLSP).
// Used in Teardown for Close call.
type LSPLifecycleOwner interface {
	Close() error
}

// EnhancedMemoryRunner provides advanced memory capabilities.
// Implemented by: *memory.EnhancedMemorySystem (agent/memory/)
type EnhancedMemoryRunner interface {
	LoadWorking(ctx context.Context, agentID string) ([]types.MemoryEntry, error)
	LoadShortTerm(ctx context.Context, agentID string, limit int) ([]types.MemoryEntry, error)
	SaveShortTerm(ctx context.Context, agentID, content string, metadata map[string]any) error
	RecordEpisode(ctx context.Context, event *types.EpisodicEvent) error
}

// ObservabilityRunner provides metrics, tracing, and logging.
// Implemented by: *observability.ObservabilitySystem (agent/observability/)
type ObservabilityRunner interface {
	StartTrace(traceID, agentID string)
	EndTrace(traceID, status string, err error)
	RecordTask(agentID string, success bool, duration time.Duration, tokens int, cost, quality float64)
}

// ExplainabilityRecorder is an optional observability extension for recording
// structured reasoning steps against the execution trace.
type ExplainabilityRecorder interface {
	StartExplainabilityTrace(traceID, sessionID, agentID string)
	AddExplainabilityStep(traceID, stepType, content string, metadata map[string]any)
	EndExplainabilityTrace(traceID string, success bool, output, errorMsg string)
}

// ExplainabilityTimelineRecorder is an optional extension for recording
// high-level decision timeline entries alongside low-level reasoning steps.
type ExplainabilityTimelineRecorder interface {
	AddExplainabilityTimeline(traceID, entryType, summary string, metadata map[string]any)
}

// ExplainabilitySynopsisReader is an optional extension for reading the latest
// completed synopsis for an agent/session so it can be fed back into runtime.
type ExplainabilitySynopsisReader interface {
	GetLatestExplainabilitySynopsis(sessionID, agentID, excludeTraceID string) string
}

type ExplainabilitySynopsisSnapshot struct {
	Synopsis             string
	CompressedHistory    string
	CompressedEventCount int
}

// ExplainabilitySynopsisSnapshotReader is an optional richer reader that
// returns both the short synopsis and compressed long-history summary.
type ExplainabilitySynopsisSnapshotReader interface {
	GetLatestExplainabilitySynopsisSnapshot(sessionID, agentID, excludeTraceID string) ExplainabilitySynopsisSnapshot
}

type MemoryRecallOptions = memorycore.MemoryRecallOptions

type MemoryObservationInput = memorycore.MemoryObservationInput

type MemoryRuntime = memorycore.MemoryRuntime

type UnifiedMemoryFacade = memorycore.UnifiedMemoryFacade

func NewUnifiedMemoryFacade(base MemoryManager, enhanced EnhancedMemoryRunner, logger *zap.Logger) *UnifiedMemoryFacade {
	return memorycore.NewUnifiedMemoryFacade(base, enhanced, logger)
}

func NewDefaultMemoryRuntime(facadeProvider func() *UnifiedMemoryFacade, baseProvider func() MemoryManager, logger *zap.Logger) *memorycore.DefaultMemoryRuntime {
	return memorycore.NewDefaultMemoryRuntime(facadeProvider, baseProvider, logger)
}

type ChatProvider = types.ChatProvider
type ChatRequest = types.ChatRequest
type ChatResponse = types.ChatResponse
type StreamChunk = types.StreamChunk

type ProviderAdapter struct {
	Provider llm.Provider
}

func NewProviderAdapter(p llm.Provider) *ProviderAdapter {
	return &ProviderAdapter{Provider: p}
}

var _ types.ChatProvider = (llm.Provider)(nil)

type ToolResult = types.ToolResult

type ToolExecutorAdapter struct {
	Executor llmtools.ToolExecutor
}

func (a *ToolExecutorAdapter) Execute(ctx context.Context, calls []types.ToolCall) []types.ToolResult {
	if a.Executor == nil {
		return nil
	}
	results := a.Executor.Execute(ctx, calls)
	out := make([]types.ToolResult, len(results))
	for i, r := range results {
		out[i] = types.ToolResult{
			ToolCallID: r.ToolCallID,
			Name:       r.Name,
			Result:     r.Result,
			Error:      r.Error,
			Duration:   r.Duration,
			FromCache:  r.FromCache,
		}
	}
	return out
}

func (a *ToolExecutorAdapter) ExecuteOne(ctx context.Context, call types.ToolCall) types.ToolResult {
	if a.Executor == nil {
		return types.ToolResult{ToolCallID: call.ID, Name: call.Name, Error: "executor not configured"}
	}
	r := a.Executor.ExecuteOne(ctx, call)
	return types.ToolResult{
		ToolCallID: r.ToolCallID,
		Name:       r.Name,
		Result:     r.Result,
		Error:      r.Error,
		Duration:   r.Duration,
		FromCache:  r.FromCache,
	}
}

// =============================================================================
// MongoDB Persistence Store Interfaces (required)
// =============================================================================
// These interfaces decouple agent/ from agent/persistence/mongodb/ to avoid
// hard dependencies. The concrete implementations in mongodb/ implicitly
// satisfy these interfaces via Go duck typing.

// PromptStoreProvider loads active prompt bundles from persistent storage.
// Implemented by: *mongodb.MongoPromptStore (agent/persistence/mongodb/)
type PromptStoreProvider interface {
	GetActive(ctx context.Context, agentType, name, tenantID string) (PromptDocument, error)
}

// PromptDocument is a minimal representation of a stored prompt bundle.
// Mirrors the fields agent/ needs from mongodb.PromptDocument.
type PromptDocument struct {
	Version     string       `json:"version"`
	System      SystemPrompt `json:"system"`
	Constraints []string     `json:"constraints,omitempty"`
}

// ConversationStoreProvider persists conversation history.
// Implemented by: *mongodb.ConversationStoreAdapter (agent/persistence/mongodb/)
type ConversationStoreProvider interface {
	// ---- 原有 ----
	Create(ctx context.Context, doc *ConversationDoc) error
	GetByID(ctx context.Context, id string) (*ConversationDoc, error)
	AppendMessages(ctx context.Context, conversationID string, msgs []ConversationMessage) error

	// ---- 新增 ----
	List(ctx context.Context, tenantID, parentID string, page, pageSize int) ([]*ConversationDoc, int64, error)
	Update(ctx context.Context, id string, updates ConversationUpdate) error
	Delete(ctx context.Context, id string) error
	DeleteByParentID(ctx context.Context, tenantID, parentID string) error
	GetMessages(ctx context.Context, conversationID string, offset, limit int) ([]ConversationMessage, int64, error)
	DeleteMessage(ctx context.Context, conversationID, messageID string) error
	ClearMessages(ctx context.Context, conversationID string) error
	Archive(ctx context.Context, id string) error
}

// ConversationDoc is a minimal conversation document for the agent layer.
type ConversationDoc struct {
	ID       string                `json:"id"`
	ParentID string                `json:"parent_id,omitempty"`
	AgentID  string                `json:"agent_id"`
	TenantID string                `json:"tenant_id"`
	UserID   string                `json:"user_id"`
	Title    string                `json:"title,omitempty"`
	Messages []ConversationMessage `json:"messages"`
}

// ConversationMessage is a single message in a conversation document.
type ConversationMessage struct {
	ID        string    `json:"id,omitempty"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// ConversationUpdate contains the fields that can be updated on a conversation.
type ConversationUpdate struct {
	Title    *string        `json:"title,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// RunStoreProvider records agent execution runs.
// Implemented by: *mongodb.MongoRunStore (agent/persistence/mongodb/)
type RunStoreProvider interface {
	RecordRun(ctx context.Context, doc *RunDoc) error
	UpdateStatus(ctx context.Context, id, status string, output *RunOutputDoc, errMsg string) error
}

// RunDoc is a minimal run document for the agent layer.
type RunDoc struct {
	ID        string    `json:"id"`
	AgentID   string    `json:"agent_id"`
	TenantID  string    `json:"tenant_id"`
	TraceID   string    `json:"trace_id"`
	Status    string    `json:"status"`
	Input     string    `json:"input"`
	StartTime time.Time `json:"start_time"`
}

// RunOutputDoc holds the output portion of a run document.
type RunOutputDoc struct {
	Content      string  `json:"content"`
	TokensUsed   int     `json:"tokens_used"`
	Cost         float64 `json:"cost"`
	FinishReason string  `json:"finish_reason"`
}

// =============================================================================
// Orchestration Interfaces (used by agent/orchestration bridge)
// =============================================================================

// OrchestratorRunner executes a multi-agent orchestration task.
// Implemented by: *orchestration.OrchestratorAdapter (agent/orchestration/)
type OrchestratorRunner interface {
	Execute(ctx context.Context, task *OrchestrationTaskInput) (*OrchestrationTaskOutput, error)
}

// OrchestrationTaskInput is the input for an orchestration task.
type OrchestrationTaskInput struct {
	ID          string         `json:"id"`
	Description string         `json:"description"`
	Input       string         `json:"input"`
	Agents      []string       `json:"agents,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// OrchestrationTaskOutput is the output from an orchestration task.
type OrchestrationTaskOutput struct {
	Pattern   string         `json:"pattern"`
	Output    string         `json:"output"`
	AgentUsed []string       `json:"agent_used,omitempty"`
	Duration  time.Duration  `json:"duration"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// =============================================================================
// Tool Manager Interface (merged from tool_manager.go)
// =============================================================================

// ToolManager为Agent运行时间摘要了"工具列表+工具执行"的能力.
//
// 设计目标:
// - 直接根据pkg/剂/工具避免pkg/剂(取消进口周期)
// - 允许在应用程序层注入不同的执行(默认使用工具)。 工具管理器)
type ToolManager interface {
	GetAllowedTools(agentID string) []types.ToolSchema
	ExecuteForAgent(ctx context.Context, agentID string, calls []types.ToolCall) []llmtools.ToolResult
}

func filterToolSchemasByWhitelist(all []types.ToolSchema, whitelist []string) []types.ToolSchema {
	if len(whitelist) == 0 {
		return all
	}
	allowed := make(map[string]struct{}, len(whitelist))
	for _, name := range whitelist {
		if name == "" {
			continue
		}
		allowed[name] = struct{}{}
	}
	out := make([]types.ToolSchema, 0, len(all))
	for _, s := range all {
		if _, ok := allowed[s.Name]; ok {
			out = append(out, s)
		}
	}
	return out
}

// =============================================================================
// PersistenceStores (merged from persistence_stores.go)
// =============================================================================

// defaultMaxRestoreMessages is the maximum number of messages to restore from conversation history.
const defaultMaxRestoreMessages = 200

// PersistenceStores encapsulates MongoDB persistence store fields extracted from BaseAgent.
type PersistenceStores struct {
	promptStore        PromptStoreProvider
	conversationStore  ConversationStoreProvider
	runStore           RunStoreProvider
	logger             *zap.Logger
	maxRestoreMessages int
}

// NewPersistenceStores creates a new PersistenceStores.
func NewPersistenceStores(logger *zap.Logger) *PersistenceStores {
	return &PersistenceStores{logger: logger}
}

// SetPromptStore sets the prompt store provider.
func (p *PersistenceStores) SetPromptStore(store PromptStoreProvider) {
	p.promptStore = store
}

// SetConversationStore sets the conversation store provider.
func (p *PersistenceStores) SetConversationStore(store ConversationStoreProvider) {
	p.conversationStore = store
}

// SetRunStore sets the run store provider.
func (p *PersistenceStores) SetRunStore(store RunStoreProvider) {
	p.runStore = store
}

// SetMaxRestoreMessages sets the maximum number of messages to restore.
func (p *PersistenceStores) SetMaxRestoreMessages(n int) {
	p.maxRestoreMessages = n
}

// PromptStore returns the prompt store provider.
func (p *PersistenceStores) PromptStore() PromptStoreProvider { return p.promptStore }

// ConversationStore returns the conversation store provider.
func (p *PersistenceStores) ConversationStore() ConversationStoreProvider {
	return p.conversationStore
}

// RunStore returns the run store provider.
func (p *PersistenceStores) RunStore() RunStoreProvider { return p.runStore }

// LoadPrompt attempts to load the active prompt from PromptStore.
// Returns nil if unavailable.
func (p *PersistenceStores) LoadPrompt(ctx context.Context, agentType, name, tenantID string) *PromptDocument {
	if p.promptStore == nil {
		return nil
	}
	doc, err := p.promptStore.GetActive(ctx, agentType, name, tenantID)
	if err != nil {
		p.logger.Debug("no active prompt in store, using config default",
			zap.String("agent_type", agentType),
			zap.String("name", name),
		)
		return nil
	}
	return &doc
}

// RecordRun records an execution run start. Returns the run ID (empty on failure).
func (p *PersistenceStores) RecordRun(ctx context.Context, agentID, tenantID, traceID, input string, startTime time.Time) string {
	if p.runStore == nil {
		return ""
	}
	runID := fmt.Sprintf("run_%s_%d", agentID, startTime.UnixNano())
	doc := &RunDoc{
		ID:        runID,
		AgentID:   agentID,
		TenantID:  tenantID,
		TraceID:   traceID,
		Status:    "running",
		Input:     input,
		StartTime: startTime,
	}
	if err := p.runStore.RecordRun(ctx, doc); err != nil {
		p.logger.Warn("failed to record run start", zap.Error(err))
		return ""
	}
	return runID
}

// UpdateRunStatus updates the status of a run.
func (p *PersistenceStores) UpdateRunStatus(ctx context.Context, runID, status string, output *RunOutputDoc, errMsg string) error {
	if p.runStore == nil || runID == "" {
		return nil
	}
	return p.runStore.UpdateStatus(ctx, runID, status, output, errMsg)
}

// RestoreConversation restores conversation history from the store using sliding window pagination.
func (p *PersistenceStores) RestoreConversation(ctx context.Context, conversationID string) []types.Message {
	if p.conversationStore == nil || conversationID == "" {
		return nil
	}

	limit := p.maxRestoreMessages
	if limit <= 0 {
		limit = defaultMaxRestoreMessages
	}

	// Use GetMessages with pagination to fetch the most recent messages.
	_, total, err := p.conversationStore.GetMessages(ctx, conversationID, 0, 1)
	if err != nil || total == 0 {
		p.logger.Debug("conversation not found or empty, starting fresh",
			zap.String("conversation_id", conversationID),
			zap.Error(err),
		)
		return nil
	}

	offset := int(total) - limit
	if offset < 0 {
		offset = 0
	}

	raw, _, err := p.conversationStore.GetMessages(ctx, conversationID, offset, limit)
	if err != nil {
		p.logger.Debug("failed to restore conversation messages",
			zap.String("conversation_id", conversationID),
			zap.Error(err),
		)
		return nil
	}

	msgs := make([]types.Message, 0, len(raw))
	for _, msg := range raw {
		msgs = append(msgs, types.Message{
			Role:    types.Role(msg.Role),
			Content: msg.Content,
		})
	}
	p.logger.Debug("restored conversation history",
		zap.String("conversation_id", conversationID),
		zap.Int("messages", len(msgs)),
		zap.Int64("total", total),
	)
	return msgs
}

// PersistConversation saves user input and agent output to ConversationStore.
func (p *PersistenceStores) PersistConversation(ctx context.Context, conversationID, agentID, tenantID, userID, inputContent, outputContent string) {
	if p.conversationStore == nil || conversationID == "" {
		return
	}

	now := time.Now()
	newMsgs := []ConversationMessage{
		{Role: string(llm.RoleUser), Content: inputContent, Timestamp: now},
		{Role: string(llm.RoleAssistant), Content: outputContent, Timestamp: now},
	}

	// Try to append to existing conversation first.
	appendErr := p.conversationStore.AppendMessages(ctx, conversationID, newMsgs)
	if appendErr == nil {
		return
	}

	// AppendMessages failed — attempt to create a new conversation.
	doc := &ConversationDoc{
		ID:       conversationID,
		AgentID:  agentID,
		TenantID: tenantID,
		UserID:   userID,
		Messages: newMsgs,
	}
	if createErr := p.conversationStore.Create(ctx, doc); createErr != nil {
		p.logger.Warn("failed to persist conversation",
			zap.String("conversation_id", conversationID),
			zap.NamedError("append_err", appendErr),
			zap.NamedError("create_err", createErr),
		)
	}
}

// ScopedPersistenceStores wraps PersistenceStores and prefixes all IDs with
// an agent-specific scope, ensuring sub-agent store operations are isolated.
type ScopedPersistenceStores struct {
	inner *PersistenceStores
	scope string // typically the sub-agent's agent_id
}

// NewScopedPersistenceStores creates a scoped wrapper.
func NewScopedPersistenceStores(inner *PersistenceStores, scope string) *ScopedPersistenceStores {
	return &ScopedPersistenceStores{inner: inner, scope: scope}
}

// Scope returns the configured scope prefix.
func (s *ScopedPersistenceStores) Scope() string { return s.scope }

func (s *ScopedPersistenceStores) scopedID(id string) string {
	if id == "" {
		return ""
	}
	return s.scope + "/" + id
}

// RecordRun delegates to inner with scoped run ID prefix.
func (s *ScopedPersistenceStores) RecordRun(ctx context.Context, agentID, tenantID, traceID, input string, startTime time.Time) string {
	return s.inner.RecordRun(ctx, s.scopedID(agentID), tenantID, traceID, input, startTime)
}

// UpdateRunStatus delegates to inner.
func (s *ScopedPersistenceStores) UpdateRunStatus(ctx context.Context, runID, status string, output *RunOutputDoc, errMsg string) error {
	return s.inner.UpdateRunStatus(ctx, runID, status, output, errMsg)
}

// RestoreConversation delegates with scoped conversation ID.
func (s *ScopedPersistenceStores) RestoreConversation(ctx context.Context, conversationID string) []types.Message {
	return s.inner.RestoreConversation(ctx, s.scopedID(conversationID))
}

// PersistConversation delegates with scoped conversation ID.
func (s *ScopedPersistenceStores) PersistConversation(ctx context.Context, conversationID, agentID, tenantID, userID, inputContent, outputContent string) {
	s.inner.PersistConversation(ctx, s.scopedID(conversationID), s.scopedID(agentID), tenantID, userID, inputContent, outputContent)
}

// LoadPrompt delegates to inner (prompts are shared, not scoped).
func (s *ScopedPersistenceStores) LoadPrompt(ctx context.Context, agentType, name, tenantID string) *PromptDocument {
	return s.inner.LoadPrompt(ctx, agentType, name, tenantID)
}

// =============================================================================
// Agent-as-Tool Adapter
// =============================================================================

// AgentToolConfig configures how an Agent is exposed as a tool.
type AgentToolConfig struct {
	// Name overrides the default tool name (default: "agent_<agent.Name()>".
	Name string

	// Description overrides the agent's description in the tool schema.
	Description string

	// Timeout limits the agent execution time. Zero means no extra timeout.
	Timeout time.Duration
}

// AgentTool wraps an Agent instance as a callable tool, enabling lightweight
// agent-to-agent delegation via the standard tool-calling interface.
type AgentTool struct {
	agent  Agent
	config AgentToolConfig
	name   string
}

// NewAgentTool creates an AgentTool that wraps the given Agent.
// If config is nil, defaults are used.
func NewAgentTool(agent Agent, config *AgentToolConfig) *AgentTool {
	cfg := AgentToolConfig{}
	if config != nil {
		cfg = *config
	}

	name := cfg.Name
	if name == "" {
		name = "agent_" + agent.Name()
	}

	return &AgentTool{
		agent:  agent,
		config: cfg,
		name:   name,
	}
}

// agentToolArgs is the JSON schema expected in ToolCall.Arguments.
type agentToolArgs struct {
	Input     string            `json:"input"`
	Context   map[string]any    `json:"context,omitempty"`
	Variables map[string]string `json:"variables,omitempty"`
}

// Schema returns the ToolSchema describing this agent-as-tool.
func (at *AgentTool) Schema() types.ToolSchema {
	desc := at.config.Description
	if desc == "" {
		desc = fmt.Sprintf("Delegate a task to the %q agent", at.agent.Name())
	}

	params := json.RawMessage(`{
		"type": "object",
		"properties": {
			"input": {
				"type": "string",
				"description": "The task or query to send to the agent"
			},
			"context": {
				"type": "object",
				"description": "Optional context key-value pairs"
			},
			"variables": {
				"type": "object",
				"description": "Optional variable substitutions",
				"additionalProperties": {"type": "string"}
			}
		},
		"required": ["input"]
	}`)

	return types.ToolSchema{
		Name:        at.name,
		Description: desc,
		Parameters:  params,
	}
}

// Execute handles a ToolCall by delegating to the wrapped Agent.
func (at *AgentTool) Execute(ctx context.Context, call types.ToolCall) llmtools.ToolResult {
	start := time.Now()
	result := llmtools.ToolResult{
		ToolCallID: call.ID,
		Name:       at.name,
	}

	// Parse arguments
	var args agentToolArgs
	if len(call.Arguments) > 0 {
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			result.Error = fmt.Sprintf("invalid arguments: %s", err.Error())
			result.Duration = time.Since(start)
			return result
		}
	}
	if args.Input == "" {
		result.Error = "missing required field: input"
		result.Duration = time.Since(start)
		return result
	}

	// Apply timeout if configured
	execCtx := ctx
	if at.config.Timeout > 0 {
		var cancel context.CancelFunc
		execCtx, cancel = context.WithTimeout(ctx, at.config.Timeout)
		defer cancel()
	}

	// Build agent Input
	input := &Input{
		Content:   args.Input,
		Context:   args.Context,
		Variables: args.Variables,
	}

	// Execute the agent
	output, err := at.agent.Execute(execCtx, input)
	if err != nil {
		result.Error = err.Error()
		result.Duration = time.Since(start)
		return result
	}

	// Marshal the output content as the tool result
	resultJSON, err := json.Marshal(map[string]any{
		"content":       output.Content,
		"tokens_used":   output.TokensUsed,
		"duration":      output.Duration.String(),
		"finish_reason": output.FinishReason,
	})
	if err != nil {
		result.Error = fmt.Sprintf("failed to marshal output: %s", err.Error())
		result.Duration = time.Since(start)
		return result
	}

	result.Result = resultJSON
	result.Duration = time.Since(start)
	return result
}

// Name returns the tool name.
func (at *AgentTool) Name() string {
	return at.name
}

// Agent returns the underlying Agent instance.
func (at *AgentTool) Agent() Agent {
	return at.agent
}

// =============================================================================
// Team (multi-agent collaboration)
// =============================================================================

type TeamMember struct {
	Agent Agent
	Role  string
}

type TeamResult struct {
	Content    string
	TokensUsed int
	Cost       float64
	Duration   time.Duration
	Metadata   map[string]any
}

type TeamOption func(*TeamOptions)
type TeamOptions struct {
	MaxRounds int
	Timeout   time.Duration
	Context   map[string]any
}

func WithMaxRounds(n int) TeamOption {
	return func(o *TeamOptions) { o.MaxRounds = n }
}

func WithTeamTimeout(d time.Duration) TeamOption {
	return func(o *TeamOptions) { o.Timeout = d }
}

func WithTeamContext(ctx map[string]any) TeamOption {
	return func(o *TeamOptions) { o.Context = ctx }
}

type Team interface {
	ID() string
	Members() []TeamMember
	Execute(ctx context.Context, task string, opts ...TeamOption) (*TeamResult, error)
}

// Merged from reflection.go.

// 反射执行器配置
type ReflectionExecutorConfig struct {
	Enabled       bool    `json:"enabled"`
	MaxIterations int     `json:"max_iterations"` // Maximum reflection iterations
	MinQuality    float64 `json:"min_quality"`    // Minimum quality threshold (0-1)
	CriticPrompt  string  `json:"critic_prompt"`  // Critic prompt template
}

// reflectionRunnerAdapter wraps *ReflectionExecutor to satisfy ReflectionRunner.
type reflectionRunnerAdapter struct {
	executor *ReflectionExecutor
}

func (a *reflectionRunnerAdapter) ExecuteWithReflection(ctx context.Context, input *Input) (*Output, error) {
	result, err := a.executor.ExecuteWithReflection(ctx, input)
	if err != nil {
		return nil, err
	}
	return result.FinalOutput, nil
}

func (a *reflectionRunnerAdapter) ReflectStep(ctx context.Context, input *Input, output *Output, state *LoopState) (*LoopReflectionResult, error) {
	return a.executor.ReflectStep(ctx, input, output, state)
}

// AsReflectionRunner wraps a *ReflectionExecutor as a ReflectionRunner.
func AsReflectionRunner(executor *ReflectionExecutor) ReflectionRunner {
	return &reflectionRunnerAdapter{executor: executor}
}

// promptEnhancerRunnerAdapter wraps *PromptEnhancer to satisfy PromptEnhancerRunner.
type promptEnhancerRunnerAdapter struct {
	enhancer *PromptEnhancer
}

func (a *promptEnhancerRunnerAdapter) EnhanceUserPrompt(prompt, context string) (string, error) {
	return a.enhancer.EnhanceUserPrompt(prompt, context), nil
}

// AsPromptEnhancerRunner wraps a *PromptEnhancer as a PromptEnhancerRunner.
func AsPromptEnhancerRunner(enhancer *PromptEnhancer) PromptEnhancerRunner {
	return &promptEnhancerRunnerAdapter{enhancer: enhancer}
}

// 默认反射 Config 返回默认反射配置
func DefaultReflectionConfig() *ReflectionExecutorConfig {
	config := DefaultReflectionExecutorConfig()
	return &config
}

// 默认反射 ExecutorConfig 返回默认反射配置
func DefaultReflectionExecutorConfig() ReflectionExecutorConfig {
	return ReflectionExecutorConfig{
		Enabled:       true,
		MaxIterations: 3,
		MinQuality:    0.7,
		CriticPrompt: `你是一个严格的评审专家。请评估以下任务执行结果的质量。

任务：{{.Task}}

执行结果：
{{.Output}}

请从以下维度评估（0-10分）：
1. 准确性：结果是否准确回答了问题
2. 完整性：是否涵盖了所有必要信息
3. 清晰度：表达是否清晰易懂
4. 相关性：是否紧扣主题

输出格式：
评分：[总分]/10
问题：[具体问题列表]
改进建议：[具体改进建议]`,
	}
}

// Critique 评审结果
type Critique struct {
	Score       float64  `json:"score"`        // 0-1 分数
	IsGood      bool     `json:"is_good"`      // 是否达标
	Issues      []string `json:"issues"`       // 问题列表
	Suggestions []string `json:"suggestions"`  // 改进建议
	RawFeedback string   `json:"raw_feedback"` // 原始反馈
}

// ReflectionResult Reflection 执行结果
type ReflectionResult struct {
	FinalOutput          *Output       `json:"final_output"`
	Iterations           int           `json:"iterations"`
	Critiques            []Critique    `json:"critiques"`
	TotalDuration        time.Duration `json:"total_duration"`
	ImprovedByReflection bool          `json:"improved_by_reflection"`
}

// ReflectionExecutor Reflection 执行器
type ReflectionExecutor struct {
	agent  *BaseAgent
	config ReflectionExecutorConfig
	logger *zap.Logger
}

// NewReflectionExecutor 创建 Reflection 执行器
func NewReflectionExecutor(agent *BaseAgent, config ReflectionExecutorConfig) *ReflectionExecutor {
	policyConfig := reflectionExecutorConfigFromPolicy(agent.loopControlPolicy())
	if config.MaxIterations <= 0 {
		config.MaxIterations = policyConfig.MaxIterations
	}
	if config.MinQuality <= 0 {
		config.MinQuality = policyConfig.MinQuality
	}
	if strings.TrimSpace(config.CriticPrompt) == "" {
		config.CriticPrompt = policyConfig.CriticPrompt
	}

	return &ReflectionExecutor{
		agent:  agent,
		config: config,
		logger: agent.Logger().With(zap.String("component", "reflection")),
	}
}

// ExecuteWithReflection 执行任务并进行 Reflection
func (r *ReflectionExecutor) ExecuteWithReflection(ctx context.Context, input *Input) (*ReflectionResult, error) {
	startTime := time.Now()

	if !r.config.Enabled {
		output, err := r.agent.executeCore(ctx, input)
		if err != nil {
			return nil, err
		}
		return &ReflectionResult{
			FinalOutput:          output,
			Iterations:           1,
			TotalDuration:        time.Since(startTime),
			ImprovedByReflection: false,
		}, nil
	}

	r.logger.Info("starting reflection execution", zap.String("trace_id", input.TraceID), zap.Int("max_iterations", r.config.MaxIterations))
	executor := &LoopExecutor{
		MaxIterations: r.config.MaxIterations,
		StepExecutor: func(ctx context.Context, input *Input, _ *LoopState, _ ReasoningSelection) (*Output, error) {
			return r.agent.executeCore(ctx, input)
		},
		Selector: reasoningModeSelectorFunc(func(_ context.Context, _ *Input, _ *LoopState, _ *reasoning.PatternRegistry, _ bool) ReasoningSelection {
			return ReasoningSelection{Mode: ReasoningModeReflection}
		}),
		Judge:             newReflectionCompletionJudge(r.config.MinQuality, r.critique),
		ReflectionStep:    r.ReflectStep,
		ReflectionEnabled: true,
		CheckpointManager: r.agent.checkpointManager,
		AgentID:           r.agent.ID(),
		Logger:            r.logger,
	}
	output, err := executor.Execute(ctx, input)
	if err != nil {
		return nil, err
	}
	if output.Metadata == nil {
		output.Metadata = make(map[string]any, 4)
	}
	output.Metadata["reflection_iteration_budget"] = r.config.MaxIterations
	output.Metadata["reflection_quality_threshold"] = r.config.MinQuality
	output.Metadata["reflection_budget_scope"] = internalBudgetScope

	duration := time.Since(startTime)
	critiques := outputReflectionCritiques(output)
	improved := len(critiques) > 1
	iterations := output.IterationCount
	if iterations == 0 {
		iterations = 1
	}
	r.logger.Info("reflection execution completed",
		zap.String("trace_id", input.TraceID),
		zap.Int("iterations", iterations),
		zap.Duration("total_duration", duration),
		zap.Bool("improved", improved))

	return &ReflectionResult{
		FinalOutput:          output,
		Iterations:           iterations,
		Critiques:            critiques,
		TotalDuration:        duration,
		ImprovedByReflection: improved,
	}, nil
}

func (r *ReflectionExecutor) ReflectStep(ctx context.Context, input *Input, output *Output, state *LoopState) (*LoopReflectionResult, error) {
	if !r.config.Enabled || input == nil || output == nil {
		return nil, nil
	}

	critique := outputReflectionCritique(output)
	if critique == nil {
		var err error
		critique, err = r.critique(ctx, input.Content, output.Content)
		if err != nil {
			return nil, err
		}
	}

	observation := &LoopObservation{
		Stage:     LoopStageDecideNext,
		Content:   "reflection_completed",
		Iteration: state.Iteration,
		Metadata: map[string]any{
			"reflection_critique": *critique,
			"reflection_score":    critique.Score,
			"reflection_is_good":  critique.IsGood,
		},
	}
	if critique.IsGood || state.Iteration >= state.MaxIterations {
		return &LoopReflectionResult{
			Critique:    critique,
			Observation: observation,
		}, nil
	}

	return &LoopReflectionResult{
		NextInput:   r.refineInput(input, critique),
		Critique:    critique,
		Observation: observation,
	}, nil
}

// critique 评审输出质量
func (r *ReflectionExecutor) critique(ctx context.Context, task, output string) (*Critique, error) {
	// 构建评审提示词
	prompt := r.config.CriticPrompt
	prompt = strings.ReplaceAll(prompt, "{{.Task}}", task)
	prompt = strings.ReplaceAll(prompt, "{{.Output}}", output)

	messages := []types.Message{
		{
			Role:    llm.RoleSystem,
			Content: "你是一个专业的质量评审专家，擅长发现问题并提供建设性建议。",
		},
		{
			Role:    llm.RoleUser,
			Content: prompt,
		},
	}

	// 调用 LLM 进行评审
	resp, err := r.agent.ChatCompletion(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("critique LLM call failed: %w", err)
	}

	choice, err := llm.FirstChoice(resp)
	if err != nil {
		return nil, fmt.Errorf("critique LLM returned no choices: %w", err)
	}

	feedback := choice.Message.Content

	// 解析评审结果
	critique := r.parseCritique(feedback)
	critique.RawFeedback = feedback

	return critique, nil
}

// parseCritique 解析评审反馈
func (r *ReflectionExecutor) parseCritique(feedback string) *Critique {
	critique := &Critique{
		Score:       0.5, // 默认中等分数
		Issues:      []string{},
		Suggestions: []string{},
	}

	lines := strings.Split(feedback, "\n")
	var currentSection string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// 提取分数
		if strings.Contains(line, "评分") || strings.Contains(line, "Score") {
			score := r.extractScore(line)
			if score > 0 {
				critique.Score = score / 10.0 // 转换为 0-1
			}
		}

		// 识别章节
		if strings.Contains(line, "问题") || strings.Contains(line, "Issues") {
			currentSection = "issues"
			continue
		}
		if strings.Contains(line, "改进建议") || strings.Contains(line, "Suggestions") {
			currentSection = "suggestions"
			continue
		}

		// 提取列表项
		if strings.HasPrefix(line, "-") || strings.HasPrefix(line, "•") ||
			(len(line) > 2 && line[0] >= '0' && line[0] <= '9' && line[1] == '.') {
			item := strings.TrimLeft(line, "-•0123456789. ")
			if item != "" {
				switch currentSection {
				case "issues":
					critique.Issues = append(critique.Issues, item)
				case "suggestions":
					critique.Suggestions = append(critique.Suggestions, item)
				}
			}
		}
	}

	// 判断是否达标
	critique.IsGood = critique.Score >= r.config.MinQuality

	return critique
}

// 从文本中提取分数
func (r *ReflectionExecutor) extractScore(text string) float64 {
	// 尝试提取“ X/ 10” 格式
	if idx := strings.Index(text, "/"); idx > 0 {
		// 提取“ /” 之前的部分
		beforeSlash := strings.TrimSpace(text[:idx])
		// 从结尾删除非数字字符
		numStr := ""
		for i := len(beforeSlash) - 1; i >= 0; i-- {
			ch := beforeSlash[i]
			if (ch >= '0' && ch <= '9') || ch == '.' {
				numStr = string(ch) + numStr
			} else if numStr != "" {
				break
			}
		}
		if numStr != "" {
			var score float64
			if _, err := fmt.Sscanf(numStr, "%f", &score); err == nil {
				return score
			}
		}
	}

	// 尝试提取纯数
	var score float64
	if _, err := fmt.Sscanf(text, "%f", &score); err == nil {
		return score
	}

	return 0
}

// refineInput 基于评审反馈改进输入
func (r *ReflectionExecutor) refineInput(original *Input, critique *Critique) *Input {
	// 构建改进提示
	refinementPrompt := fmt.Sprintf(`原始任务：
%s

之前的执行存在以下问题：
%s

改进建议：
%s

请重新执行任务，注意避免上述问题，并采纳改进建议。`,
		original.Content,
		strings.Join(critique.Issues, "\n- "),
		strings.Join(critique.Suggestions, "\n- "),
	)

	// 创建新的输入
	refined := &Input{
		TraceID:   original.TraceID,
		TenantID:  original.TenantID,
		UserID:    original.UserID,
		ChannelID: original.ChannelID,
		Content:   refinementPrompt,
		Context:   original.Context,
		Variables: original.Variables,
	}

	// 在 Context 中记录 Reflection 历史
	if refined.Context == nil {
		refined.Context = make(map[string]any)
	}
	refined.Context["reflection_feedback"] = critique

	return refined
}

func outputReflectionCritiques(output *Output) []Critique {
	if output == nil || output.Metadata == nil {
		return nil
	}
	critiques := make([]Critique, 0, 2)
	if rawCritiques, ok := output.Metadata["reflection_critiques"]; ok {
		storedCritiques, ok := rawCritiques.([]Critique)
		if ok {
			critiques = append(critiques, storedCritiques...)
		}
	}
	if critique := outputReflectionCritique(output); critique != nil {
		critiques = append(critiques, *critique)
	}
	if len(critiques) == 0 {
		return nil
	}
	return critiques
}

func outputReflectionCritique(output *Output) *Critique {
	if output == nil || output.Metadata == nil {
		return nil
	}
	rawCritique, ok := output.Metadata["reflection_critique"]
	if !ok {
		return nil
	}
	critique, ok := rawCritique.(Critique)
	if !ok {
		return nil
	}
	copied := critique
	return &copied
}

type reflectionCompletionJudge struct {
	minQuality float64
	fallback   CompletionJudge
	critiqueFn func(context.Context, string, string) (*Critique, error)
}

func newReflectionCompletionJudge(minQuality float64, critiqueFn func(context.Context, string, string) (*Critique, error)) CompletionJudge {
	if minQuality <= 0 {
		minQuality = 0.7
	}
	return &reflectionCompletionJudge{
		minQuality: minQuality,
		fallback:   NewDefaultCompletionJudge(),
		critiqueFn: critiqueFn,
	}
}

func (j *reflectionCompletionJudge) Judge(ctx context.Context, state *LoopState, output *Output, err error) (*CompletionDecision, error) {
	decision, judgeErr := j.fallback.Judge(ctx, state, output, err)
	if judgeErr != nil || decision == nil || err != nil || output == nil || strings.TrimSpace(output.Content) == "" {
		return decision, judgeErr
	}

	critique, critiqueErr := j.critiqueFn(ctx, state.Goal, output.Content)
	if critiqueErr != nil {
		return nil, critiqueErr
	}
	if output.Metadata == nil {
		output.Metadata = make(map[string]any, 4)
	}
	output.Metadata["reflection_critique"] = *critique
	output.Metadata["reflection_score"] = critique.Score
	output.Metadata["reflection_is_good"] = critique.IsGood
	output.Metadata["reflection_quality_threshold"] = j.minQuality

	if critique.IsGood || critique.Score >= j.minQuality {
		decision.Decision = LoopDecisionDone
		decision.Solved = true
		decision.StopReason = StopReasonSolved
		decision.Confidence = critique.Score
		decision.Reason = "reflection quality acceptable"
		return decision, nil
	}

	if state != nil && state.Iteration >= state.MaxIterations {
		decision.Decision = LoopDecisionDone
		decision.StopReason = StopReasonBlocked
		decision.Confidence = critique.Score
		decision.Reason = "reflection iteration budget exhausted"
		if output.Metadata == nil {
			output.Metadata = make(map[string]any, 4)
		}
		output.Metadata["internal_stop_cause"] = "reflection_iteration_budget_exhausted"
		return decision, nil
	}

	return &CompletionDecision{
		Decision:       LoopDecisionReflect,
		NeedReflection: true,
		StopReason:     StopReasonBlocked,
		Confidence:     critique.Score,
		Reason:         "reflection requested another iteration",
	}, nil
}

func reflectionCritiquesFromObservations(observations []LoopObservation) []Critique {
	critiques := make([]Critique, 0, len(observations))
	for _, observation := range observations {
		if observation.Metadata == nil {
			continue
		}
		raw, ok := observation.Metadata["reflection_critique"]
		if !ok {
			continue
		}
		critique, ok := raw.(Critique)
		if ok {
			critiques = append(critiques, critique)
		}
	}
	return critiques
}

// Merged from tool_selector.go.

// ToolScore 工具评分
type ToolScore struct {
	Tool               types.ToolSchema `json:"tool"`
	SemanticSimilarity float64          `json:"semantic_similarity"` // Semantic similarity (0-1)
	EstimatedCost      float64          `json:"estimated_cost"`      // Estimated cost
	AvgLatency         time.Duration    `json:"avg_latency"`         // Average latency
	ReliabilityScore   float64          `json:"reliability_score"`   // Reliability (0-1)
	TotalScore         float64          `json:"total_score"`         // Total score (0-1)
}

// ToolSelectionConfig 工具选择配置
type ToolSelectionConfig struct {
	Enabled bool `json:"enabled"`

	// 分数
	SemanticWeight    float64 `json:"semantic_weight"`    // Semantic similarity weight
	CostWeight        float64 `json:"cost_weight"`        // Cost weight
	LatencyWeight     float64 `json:"latency_weight"`     // Latency weight
	ReliabilityWeight float64 `json:"reliability_weight"` // Reliability weight

	// 甄选战略
	MaxTools      int     `json:"max_tools"`       // Maximum number of tools to select
	MinScore      float64 `json:"min_score"`       // Minimum score threshold
	UseLLMRanking bool    `json:"use_llm_ranking"` // Whether to use LLM-assisted ranking
}

// 默认工具SecutConfig 返回默认工具选择配置
func DefaultToolSelectionConfig() *ToolSelectionConfig {
	config := defaultToolSelectionConfigValue()
	return &config
}

// 默认工具Secution ConfigValue 返回默认工具选择配置值
func defaultToolSelectionConfigValue() ToolSelectionConfig {
	return ToolSelectionConfig{
		Enabled:           true,
		SemanticWeight:    0.5,
		CostWeight:        0.2,
		LatencyWeight:     0.15,
		ReliabilityWeight: 0.15,
		MaxTools:          5,
		MinScore:          0.3,
		UseLLMRanking:     true,
	}
}

// ToolSelector 工具选择器接口
type ToolSelector interface {
	// SelectTools 基于任务选择最佳工具
	SelectTools(ctx context.Context, task string, availableTools []types.ToolSchema) ([]types.ToolSchema, error)

	// ScoreTools 对工具进行评分
	ScoreTools(ctx context.Context, task string, tools []types.ToolSchema) ([]ToolScore, error)
}

// ReasoningExposureLevel controls which non-default reasoning patterns are
// registered into the runtime. The official execution path remains react,
// with reflection as an opt-in quality enhancement outside the registry.
type ReasoningExposureLevel string

const (
	ReasoningExposureOfficial ReasoningExposureLevel = "official"
	ReasoningExposureAdvanced ReasoningExposureLevel = "advanced"
	ReasoningExposureAll      ReasoningExposureLevel = "all"
)

func normalizeReasoningExposureLevel(level ReasoningExposureLevel) ReasoningExposureLevel {
	switch level {
	case ReasoningExposureAdvanced, ReasoningExposureAll:
		return level
	default:
		return ReasoningExposureOfficial
	}
}

const (
	ReasoningModeReact          = "react"
	ReasoningModeReflection     = "reflection"
	ReasoningModeReWOO          = "rewoo"
	ReasoningModePlanAndExecute = "plan_and_execute"
	ReasoningModeDynamicPlanner = "dynamic_planner"
	ReasoningModeTreeOfThought  = "tree_of_thought"
)

var reasoningModeAliases = map[string]string{
	"react":            ReasoningModeReact,
	"reflection":       ReasoningModeReflection,
	"reflexion":        ReasoningModeReflection,
	"rewoo":            ReasoningModeReWOO,
	"plan_and_execute": ReasoningModePlanAndExecute,
	"plan_execute":     ReasoningModePlanAndExecute,
	"dynamic_planner":  ReasoningModeDynamicPlanner,
	"tree_of_thought":  ReasoningModeTreeOfThought,
	"tree-of-thought":  ReasoningModeTreeOfThought,
	"tree of thought":  ReasoningModeTreeOfThought,
	"tot":              ReasoningModeTreeOfThought,
}

var reasoningPatternCandidates = map[string][]string{
	ReasoningModeReflection:     {"reflexion", ReasoningModeReflection},
	ReasoningModeReWOO:          {ReasoningModeReWOO},
	ReasoningModePlanAndExecute: {ReasoningModePlanAndExecute, "plan_execute"},
	ReasoningModeDynamicPlanner: {ReasoningModeDynamicPlanner},
	ReasoningModeTreeOfThought:  {ReasoningModeTreeOfThought},
}

type ReasoningSelection struct {
	Mode    string
	Pattern reasoning.ReasoningPattern
}

type ReasoningModeSelector interface {
	Select(ctx context.Context, input *Input, state *LoopState, registry *reasoning.PatternRegistry, reflectionEnabled bool) ReasoningSelection
}

type reasoningModeSelectorFunc func(ctx context.Context, input *Input, state *LoopState, registry *reasoning.PatternRegistry, reflectionEnabled bool) ReasoningSelection

func (f reasoningModeSelectorFunc) Select(ctx context.Context, input *Input, state *LoopState, registry *reasoning.PatternRegistry, reflectionEnabled bool) ReasoningSelection {
	return f(ctx, input, state, registry, reflectionEnabled)
}

type DefaultReasoningModeSelector struct{}

func NewDefaultReasoningModeSelector() ReasoningModeSelector { return DefaultReasoningModeSelector{} }

func (DefaultReasoningModeSelector) Select(ctx context.Context, input *Input, state *LoopState, registry *reasoning.PatternRegistry, reflectionEnabled bool) ReasoningSelection {
	if selection, ok := runtimeSelectResumedReasoningMode(state, registry, reflectionEnabled); ok {
		return selection
	}
	if runtimeShouldUseReflection(input, state, registry, reflectionEnabled) {
		return runtimeBuildReasoningSelection(ReasoningModeReflection, registry)
	}
	if runtimeShouldUseTreeOfThought(input, state, registry) {
		return runtimeBuildReasoningSelection(ReasoningModeTreeOfThought, registry)
	}
	if runtimeShouldUseDynamicPlanner(input, state, registry) {
		return runtimeBuildReasoningSelection(ReasoningModeDynamicPlanner, registry)
	}
	if runtimeShouldUsePlanAndExecute(input, state, registry) {
		return runtimeBuildReasoningSelection(ReasoningModePlanAndExecute, registry)
	}
	if runtimeShouldUseReWOO(input, state, registry) {
		return runtimeBuildReasoningSelection(ReasoningModeReWOO, registry)
	}
	return runtimeBuildReasoningSelection(ReasoningModeReact, registry)
}

func runtimeSelectResumedReasoningMode(state *LoopState, registry *reasoning.PatternRegistry, reflectionEnabled bool) (ReasoningSelection, bool) {
	if state == nil {
		return ReasoningSelection{}, false
	}
	if state.CurrentStage == "" || state.CurrentStage == LoopStagePerceive {
		return ReasoningSelection{}, false
	}
	mode := runtimeNormalizeReasoningMode(state.SelectedReasoningMode)
	if mode == "" {
		return ReasoningSelection{}, false
	}
	return runtimeBuildReasoningSelectionWithFallback(mode, registry, reflectionEnabled), true
}

func runtimeBuildReasoningSelectionWithFallback(mode string, registry *reasoning.PatternRegistry, reflectionEnabled bool) ReasoningSelection {
	selection := runtimeBuildReasoningSelection(mode, registry)
	if selection.Mode == ReasoningModeReflection && !reflectionEnabled && selection.Pattern == nil {
		return runtimeBuildReasoningSelection(ReasoningModeReact, registry)
	}
	if selection.Mode != ReasoningModeReact && selection.Mode != ReasoningModeReflection && selection.Pattern == nil {
		return runtimeBuildReasoningSelection(ReasoningModeReact, registry)
	}
	return selection
}

func runtimeBuildReasoningSelection(mode string, registry *reasoning.PatternRegistry) ReasoningSelection {
	normalized := runtimeNormalizeReasoningMode(mode)
	if normalized == "" {
		normalized = ReasoningModeReact
	}
	selection := ReasoningSelection{Mode: normalized}
	if registry == nil {
		return selection
	}
	for _, candidate := range reasoningPatternCandidates[normalized] {
		pattern, ok := registry.Get(candidate)
		if ok {
			selection.Pattern = pattern
			return selection
		}
	}
	return selection
}

func runtimeNormalizeReasoningMode(value string) string {
	key := strings.ToLower(strings.TrimSpace(value))
	if key == "" {
		return ""
	}
	if normalized, ok := reasoningModeAliases[key]; ok {
		return normalized
	}
	return ""
}

func runtimeShouldUseReflection(input *Input, state *LoopState, registry *reasoning.PatternRegistry, reflectionEnabled bool) bool {
	if !reflectionEnabled && !hasReasoningPattern(registry, ReasoningModeReflection) {
		return false
	}
	return contextBool(input, "requires_reflection") ||
		contextBool(input, "need_reflection") ||
		contextBool(input, "quality_critical") ||
		contextBool(input, "needs_critique") ||
		contextString(input, "current_stage") == "reflection" ||
		contextString(input, "loop_stage") == "reflection" ||
		(state != nil && state.Decision == LoopDecisionReflect) ||
		(state != nil && state.Iteration > 1 && state.LastOutput != nil && strings.TrimSpace(state.LastOutput.Content) == "")
}

func runtimeShouldUseReWOO(input *Input, state *LoopState, registry *reasoning.PatternRegistry) bool {
	if !hasReasoningPattern(registry, ReasoningModeReWOO) {
		return false
	}
	if state != nil && len(state.Plan) > 0 && len(state.Plan) >= 3 {
		return true
	}
	if state != nil && state.CurrentStage == LoopStageValidate {
		return true
	}
	return contextBool(input, "tool_intensive") ||
		contextBool(input, "tool_verification_required") ||
		contextBool(input, "requires_tools") ||
		contextBool(input, "requires_observationless_tool_plan") ||
		intContextAtLeast(input, "tool_count", 2) ||
		contentContainsAny(input, "tool", "tools", "search", "collect", "gather", "retrieve", "crawl", "inspect")
}

func runtimeShouldUsePlanAndExecute(input *Input, state *LoopState, registry *reasoning.PatternRegistry) bool {
	if !hasReasoningPattern(registry, ReasoningModePlanAndExecute) {
		return false
	}
	if state != nil && len(state.Plan) > 0 {
		return true
	}
	return contextBool(input, "requires_plan") ||
		contextBool(input, "multi_step") ||
		contextBool(input, "needs_replanning") ||
		contextBool(input, "complex_task") ||
		intContextAtLeast(input, "plan_steps", 2) ||
		contentContainsAny(input, "plan", "steps", "implement", "execute", "roadmap", "break down")
}

func runtimeShouldUseDynamicPlanner(input *Input, state *LoopState, registry *reasoning.PatternRegistry) bool {
	if !hasReasoningPattern(registry, ReasoningModeDynamicPlanner) {
		return false
	}
	return contextBool(input, "requires_backtracking") ||
		contextBool(input, "blocked") ||
		contextBool(input, "requires_alternative_paths") ||
		contextBool(input, "dynamic_replanning") ||
		contextBool(input, "search_space_large") ||
		(state != nil && state.Decision == LoopDecisionReplan) ||
		(state != nil && state.StopReason == StopReasonBlocked) ||
		contentContainsAny(input, "backtrack", "alternative", "constraint", "optimize")
}

func runtimeShouldUseTreeOfThought(input *Input, state *LoopState, registry *reasoning.PatternRegistry) bool {
	if !hasReasoningPattern(registry, ReasoningModeTreeOfThought) {
		return false
	}
	if state != nil && state.Iteration > 1 && state.Confidence < 0.5 {
		return true
	}
	return contextBool(input, "high_uncertainty") ||
		contextBool(input, "explore_multiple_paths") ||
		contextBool(input, "compare_branches") ||
		intContextAtLeast(input, "candidate_count", 3) ||
		contentContainsAny(input, "compare options", "multiple approaches", "explore", "brainstorm", "tradeoff", "uncertain")
}

func hasReasoningPattern(registry *reasoning.PatternRegistry, mode string) bool {
	if registry == nil {
		return false
	}
	for _, candidate := range reasoningPatternCandidates[mode] {
		if _, ok := registry.Get(candidate); ok {
			return true
		}
	}
	return false
}

// DynamicToolSelector 动态工具选择器
type DynamicToolSelector struct {
	agent  *BaseAgent
	config ToolSelectionConfig

	// 工具统计(可以从数据库中加载)
	toolStats map[string]*ToolStats

	logger *zap.Logger
}

// ToolStats 工具统计信息
type ToolStats struct {
	Name            string
	TotalCalls      int64
	SuccessfulCalls int64
	FailedCalls     int64
	TotalLatency    time.Duration
	AvgCost         float64
}

// NewDynamicToolSelector 创建动态工具选择器
func NewDynamicToolSelector(agent *BaseAgent, config ToolSelectionConfig) *DynamicToolSelector {
	if config.MaxTools <= 0 {
		config.MaxTools = 5
	}
	if config.MinScore <= 0 {
		config.MinScore = 0.3
	}

	return &DynamicToolSelector{
		agent:     agent,
		config:    config,
		toolStats: make(map[string]*ToolStats),
		logger:    agent.Logger().With(zap.String("component", "tool_selector")),
	}
}

// SelectTools 选择最佳工具
func (s *DynamicToolSelector) SelectTools(ctx context.Context, task string, availableTools []types.ToolSchema) ([]types.ToolSchema, error) {
	if !s.config.Enabled || len(availableTools) == 0 {
		return availableTools, nil
	}

	s.logger.Debug("selecting tools",
		zap.String("task", task),
		zap.Int("available_tools", len(availableTools)),
	)

	// 1. 对所有工具评分
	scores, err := s.ScoreTools(ctx, task, availableTools)
	if err != nil {
		s.logger.Warn("tool scoring failed, using all tools", zap.Error(err))
		return availableTools, nil
	}

	// 2. 按分数排序
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].TotalScore > scores[j].TotalScore
	})

	// 3. 可选：使用 LLM 进行二次排序
	if s.config.UseLLMRanking && len(scores) > s.config.MaxTools {
		scores, err = s.llmRanking(ctx, task, scores)
		if err != nil {
			s.logger.Warn("LLM ranking failed, using score-based ranking", zap.Error(err))
		}
	}

	// 4. 选择 Top-K 工具
	selected := []types.ToolSchema{}
	for i, score := range scores {
		if i >= s.config.MaxTools {
			break
		}
		if score.TotalScore < s.config.MinScore {
			break
		}
		selected = append(selected, score.Tool)
	}

	s.logger.Info("tools selected",
		zap.Int("selected", len(selected)),
		zap.Int("total", len(availableTools)),
	)

	return selected, nil
}

func (b *BaseAgent) toolSelectionMiddleware() ExecutionMiddleware {
	return func(ctx context.Context, input *Input, next ExecutionFunc) (*Output, error) {
		b.logger.Debug("selecting tools dynamically", zap.String("trace_id", input.TraceID))
		availableTools := b.toolManager.GetAllowedTools(b.ID())
		selected, err := b.extensions.ToolSelector().SelectTools(ctx, input.Content, availableTools)
		if err != nil {
			b.logger.Warn("tool selection failed", zap.String("trace_id", input.TraceID), zap.Error(err))
		} else {
			toolNames := make([]string, 0, len(selected))
			for _, tool := range selected {
				name := strings.TrimSpace(tool.Name)
				if name == "" {
					continue
				}
				toolNames = append(toolNames, name)
			}

			override := &RunConfig{}
			if len(toolNames) == 0 {
				override.DisableTools = true
			} else {
				override.ToolWhitelist = toolNames
			}
			ctx = WithRunConfig(ctx, MergeRunConfig(GetRunConfig(ctx), override))

			b.logger.Info("tools selected dynamically",
				zap.String("trace_id", input.TraceID),
				zap.Strings("selected_tools", toolNames),
				zap.Bool("tools_disabled", len(toolNames) == 0),
			)
		}
		return next(ctx, input)
	}
}

// ScoreTools 对工具进行评分
func (s *DynamicToolSelector) ScoreTools(ctx context.Context, task string, tools []types.ToolSchema) ([]ToolScore, error) {
	scores := make([]ToolScore, len(tools))

	for i, tool := range tools {
		score := ToolScore{
			Tool: tool,
		}

		// 1. 语义相似度（基于描述匹配）
		score.SemanticSimilarity = s.calculateSemanticSimilarity(task, tool)

		// 2. 成本评估
		score.EstimatedCost = s.estimateCost(tool)

		// 3. 延迟评估
		score.AvgLatency = s.getAvgLatency(tool.Name)

		// 4. 可靠性评估
		score.ReliabilityScore = s.getReliability(tool.Name)

		// 5. 计算综合得分
		score.TotalScore = s.calculateTotalScore(score)

		scores[i] = score
	}

	return scores, nil
}

// 计算任务和工具之间的语义相似性
func (s *DynamicToolSelector) calculateSemanticSimilarity(task string, tool types.ToolSchema) float64 {
	// 简化版本:基于关键字的匹配
	// 生产应使用矢量嵌入法+余弦相近性

	taskLower := strings.ToLower(task)
	toolDesc := strings.ToLower(tool.Description)
	toolName := strings.ToLower(tool.Name)

	// 提取任务关键字
	keywords := extractKeywords(taskLower)

	matchCount := 0
	for _, keyword := range keywords {
		if strings.Contains(toolDesc, keyword) || strings.Contains(toolName, keyword) {
			matchCount++
		}
	}

	if len(keywords) == 0 {
		return 0.5 // Default medium similarity
	}

	similarity := float64(matchCount) / float64(len(keywords))

	// 准确姓名匹配的奖金
	for _, keyword := range keywords {
		if strings.Contains(toolName, keyword) {
			similarity = math.Min(1.0, similarity+0.2)
		}
	}

	return similarity
}

// 成本估计工具执行费用
func (s *DynamicToolSelector) estimateCost(tool types.ToolSchema) float64 {
	// 简化版本:根据工具类型估算
	// 制作应使用历史数据统计

	name := strings.ToLower(tool.Name)

	// 高成本工具
	if strings.Contains(name, "api") || strings.Contains(name, "external") {
		return 0.1
	}

	// 中成本工具
	if strings.Contains(name, "search") || strings.Contains(name, "query") {
		return 0.05
	}

	// 低成本工具
	return 0.01
}

// getAvgLatency 获取平均延迟
func (s *DynamicToolSelector) getAvgLatency(toolName string) time.Duration {
	if stats, ok := s.toolStats[toolName]; ok && stats.TotalCalls > 0 {
		return stats.TotalLatency / time.Duration(stats.TotalCalls)
	}

	// 默认延迟估计
	return 500 * time.Millisecond
}

// getReliability 获取可靠性分数
func (s *DynamicToolSelector) getReliability(toolName string) float64 {
	if stats, ok := s.toolStats[toolName]; ok && stats.TotalCalls > 0 {
		return float64(stats.SuccessfulCalls) / float64(stats.TotalCalls)
	}

	// 新工具的默认可靠性
	return 0.8
}

// 计算总加权分数
func (s *DynamicToolSelector) calculateTotalScore(score ToolScore) float64 {
	// 使每个度量标准规范化
	semanticScore := score.SemanticSimilarity

	// 降低成本更好(倒数)
	costScore := 1.0 - math.Min(1.0, score.EstimatedCost*10)

	// 更低的延迟度(反之,假设5分为最差)
	latencyScore := 1.0 - math.Min(1.0, float64(score.AvgLatency)/float64(5*time.Second))

	reliabilityScore := score.ReliabilityScore

	// 加权总和
	total := semanticScore*s.config.SemanticWeight +
		costScore*s.config.CostWeight +
		latencyScore*s.config.LatencyWeight +
		reliabilityScore*s.config.ReliabilityWeight

	return total
}

// llmRanking 使用 LLM 进行二级排名
func (s *DynamicToolSelector) llmRanking(ctx context.Context, task string, scores []ToolScore) ([]ToolScore, error) {
	// 构建工具列表描述
	toolList := []string{}
	for i, score := range scores {
		if i >= s.config.MaxTools*2 { // Only let LLM rank top 2*MaxTools
			break
		}
		toolList = append(toolList, fmt.Sprintf("%d. %s: %s (Score: %.2f)",
			i+1, score.Tool.Name, score.Tool.Description, score.TotalScore))
	}

	prompt := fmt.Sprintf(`任务：%s

可用工具列表：
%s

请从上述工具中选择最适合完成任务的 %d 个工具，按优先级排序。
只输出工具编号，用逗号分隔，例如：1,3,5`,
		task,
		strings.Join(toolList, "\n"),
		s.config.MaxTools,
	)

	messages := []types.Message{
		{
			Role:    llm.RoleSystem,
			Content: "你是一个工具选择专家，擅长为任务选择最合适的工具。",
		},
		{
			Role:    llm.RoleUser,
			Content: prompt,
		},
	}

	resp, err := s.agent.ChatCompletion(ctx, messages)
	if err != nil {
		return scores, err
	}

	choice, err := llm.FirstChoice(resp)
	if err != nil {
		return scores, err
	}

	// LLM 返回的解析工具索引
	selected := parseToolIndices(choice.Message.Content)

	// 重排工具
	reordered := []ToolScore{}
	for _, idx := range selected {
		if idx > 0 && idx <= len(scores) {
			reordered = append(reordered, scores[idx-1])
		}
	}

	// 添加剩余工具
	usedIndices := make(map[int]bool)
	for _, idx := range selected {
		usedIndices[idx] = true
	}
	for i := range scores {
		if !usedIndices[i+1] {
			reordered = append(reordered, scores[i])
		}
	}

	return reordered, nil
}

// AsToolSelectorRunner wraps a *DynamicToolSelector as a DynamicToolSelectorRunner.
// Since the interface now uses concrete types, this is a direct cast.
func AsToolSelectorRunner(selector *DynamicToolSelector) DynamicToolSelectorRunner {
	return selector
}

// UpdateToolStats 更新工具统计信息
func (s *DynamicToolSelector) UpdateToolStats(toolName string, success bool, latency time.Duration, cost float64) {
	if s.toolStats[toolName] == nil {
		s.toolStats[toolName] = &ToolStats{
			Name: toolName,
		}
	}

	stats := s.toolStats[toolName]
	stats.TotalCalls++
	if success {
		stats.SuccessfulCalls++
	} else {
		stats.FailedCalls++
	}
	stats.TotalLatency += latency

	// 更新平均费用(变动平均数)
	if stats.TotalCalls == 1 {
		stats.AvgCost = cost
	} else {
		stats.AvgCost = (stats.AvgCost*float64(stats.TotalCalls-1) + cost) / float64(stats.TotalCalls)
	}
}

// 取出关键字从文本中取出关键字(简化版)
func extractKeywords(text string) []string {
	// 删除常见的句号
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "and": true, "or": true,
		"but": true, "in": true, "on": true, "at": true, "to": true,
		"for": true, "of": true, "with": true, "by": true, "from": true,
		"是": true, "的": true, "了": true, "在": true, "和": true,
		"与": true, "或": true, "但": true, "对": true, "从": true,
	}

	words := strings.Fields(text)
	keywords := []string{}

	// 定义要删除的标点( 使用原始字符串来避免逃跑)
	punctuation := `,.!?;:"'()[]{}，。！？；：（）【】`

	for _, word := range words {
		word = strings.Trim(word, punctuation)
		if len(word) > 2 && !stopWords[word] {
			keywords = append(keywords, word)
		}
	}

	return keywords
}

// 解析工具索引
// 只解析逗号分隔格式, 返回新行分隔为空
func parseToolIndices(text string) []int {
	indices := []int{}

	// 检查文本是否包含不含逗号的新行 - 返回为空
	if strings.Contains(text, "\n") && !strings.Contains(text, ",") {
		return indices
	}

	// 删除所有空格和新行
	text = strings.ReplaceAll(text, " ", "")
	text = strings.ReplaceAll(text, "\n", "")

	// 以逗号分隔
	parts := strings.Split(text, ",")

	for _, part := range parts {
		if part == "" {
			continue
		}
		var idx int
		if _, err := fmt.Sscanf(part, "%d", &idx); err == nil {
			indices = append(indices, idx)
		}
	}

	return indices
}
