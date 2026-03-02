package agent

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

import (
	"context"
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/llm"
	llmtools "github.com/BaSui01/agentflow/llm/capabilities/tools"

	"github.com/BaSui01/agentflow/agent/memory"
	"github.com/BaSui01/agentflow/agent/skills"
	"github.com/BaSui01/agentflow/types"

	"go.uber.org/zap"
)

// ReflectionRunner executes a task with iterative self-reflection.
// Implemented by: *ReflectionExecutor (agent/reflection.go)
type ReflectionRunner interface {
	ExecuteWithReflection(ctx context.Context, input *Input) (any, error)
}

// DynamicToolSelectorRunner dynamically selects tools relevant to a given task.
// This uses any for availableTools to match the integration.go call site signature.
// Implemented by: *DynamicToolSelector (agent/tool_selector.go) via adapter
type DynamicToolSelectorRunner interface {
	SelectTools(ctx context.Context, task string, availableTools any) (any, error)
}

// PromptEnhancerRunner enhances user prompts with additional context.
// Implemented by: *PromptEnhancer (agent/prompt_enhancer.go)
type PromptEnhancerRunner interface {
	EnhanceUserPrompt(prompt, context string) (string, error)
}

// SkillDiscoverer discovers skills relevant to a task.
// Implemented by: *skills.DefaultSkillManager (agent/skills/)
type SkillDiscoverer interface {
	DiscoverSkills(ctx context.Context, task string) ([]*skills.Skill, error)
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
	LoadWorking(ctx context.Context, agentID string) ([]any, error)
	LoadShortTerm(ctx context.Context, agentID string, limit int) ([]any, error)
	SaveShortTerm(ctx context.Context, agentID, content string, metadata map[string]any) error
	RecordEpisode(ctx context.Context, event *memory.EpisodicEvent) error
}

// ObservabilityRunner provides metrics, tracing, and logging.
// Implemented by: *observability.ObservabilitySystem (agent/observability/)
type ObservabilityRunner interface {
	StartTrace(traceID, agentID string)
	EndTrace(traceID, status string, err error)
	RecordTask(agentID string, success bool, duration time.Duration, tokens int, cost, quality float64)
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

// PersistenceStores encapsulates MongoDB persistence store fields extracted from BaseAgent.
type PersistenceStores struct {
	promptStore       PromptStoreProvider
	conversationStore ConversationStoreProvider
	runStore          RunStoreProvider
	logger            *zap.Logger
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

// RestoreConversation restores conversation history from the store.
func (p *PersistenceStores) RestoreConversation(ctx context.Context, conversationID string) []types.Message {
	if p.conversationStore == nil || conversationID == "" {
		return nil
	}
	conv, err := p.conversationStore.GetByID(ctx, conversationID)
	if err != nil {
		p.logger.Debug("conversation not found or error, starting fresh",
			zap.String("conversation_id", conversationID),
			zap.Error(err),
		)
		return nil
	}
	if conv == nil {
		return nil
	}
	var msgs []types.Message
	for _, msg := range conv.Messages {
		msgs = append(msgs, types.Message{
			Role:    types.Role(msg.Role),
			Content: msg.Content,
		})
	}
	p.logger.Debug("restored conversation history",
		zap.String("conversation_id", conversationID),
		zap.Int("messages", len(msgs)),
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
