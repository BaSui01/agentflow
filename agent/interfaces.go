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
	"time"

	"github.com/BaSui01/agentflow/agent/memory"
	"github.com/BaSui01/agentflow/agent/skills"
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
// Implemented by: *mongodb.MongoConversationStore (agent/persistence/mongodb/)
type ConversationStoreProvider interface {
	Create(ctx context.Context, doc *ConversationDoc) error
	GetByID(ctx context.Context, id string) (*ConversationDoc, error)
	AppendMessages(ctx context.Context, conversationID string, msgs []ConversationMessage) error
}

// ConversationDoc is a minimal conversation document for the agent layer.
type ConversationDoc struct {
	ID       string                `json:"id"`
	AgentID  string                `json:"agent_id"`
	TenantID string                `json:"tenant_id"`
	UserID   string                `json:"user_id"`
	Messages []ConversationMessage `json:"messages"`
}

// ConversationMessage is a single message in a conversation document.
type ConversationMessage struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
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
