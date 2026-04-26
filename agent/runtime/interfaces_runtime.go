package runtime

import (
	"context"
	agentcontext "github.com/BaSui01/agentflow/agent/execution/context"
	agentevents "github.com/BaSui01/agentflow/agent/observability/events"
	agentpersistence "github.com/BaSui01/agentflow/agent/persistence"
	llmtools "github.com/BaSui01/agentflow/llm/capabilities/tools"
	llm "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
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

type ExplainabilitySynopsisSnapshot = agentcontext.ExplainabilitySynopsisSnapshot

// ExplainabilitySynopsisSnapshotReader is an optional richer reader that
// returns both the short synopsis and compressed long-history summary.
type ExplainabilitySynopsisSnapshotReader interface {
	GetLatestExplainabilitySynopsisSnapshot(sessionID, agentID, excludeTraceID string) ExplainabilitySynopsisSnapshot
}

type RuntimeStreamEventType = agentevents.RuntimeStreamEventType

type SDKStreamEventType = agentevents.SDKStreamEventType

type SDKRunItemEventName = agentevents.SDKRunItemEventName

type RuntimeToolCall = agentevents.RuntimeToolCall

type RuntimeToolResult = agentevents.RuntimeToolResult

type RuntimeStreamEvent = agentevents.RuntimeStreamEvent

type RuntimeStreamEmitter = agentevents.RuntimeStreamEmitter

const (
	RuntimeStreamToken        = agentevents.RuntimeStreamToken
	RuntimeStreamReasoning    = agentevents.RuntimeStreamReasoning
	RuntimeStreamToolCall     = agentevents.RuntimeStreamToolCall
	RuntimeStreamToolResult   = agentevents.RuntimeStreamToolResult
	RuntimeStreamToolProgress = agentevents.RuntimeStreamToolProgress
	RuntimeStreamApproval     = agentevents.RuntimeStreamApproval
	RuntimeStreamSession      = agentevents.RuntimeStreamSession
	RuntimeStreamStatus       = agentevents.RuntimeStreamStatus
	RuntimeStreamSteering     = agentevents.RuntimeStreamSteering
	RuntimeStreamStopAndSend  = agentevents.RuntimeStreamStopAndSend
)

const (
	SDKRawResponseEvent  = agentevents.SDKRawResponseEvent
	SDKRunItemEvent      = agentevents.SDKRunItemEvent
	SDKAgentUpdatedEvent = agentevents.SDKAgentUpdatedEvent
)

const (
	SDKMessageOutputCreated = agentevents.SDKMessageOutputCreated
	SDKHandoffRequested     = agentevents.SDKHandoffRequested
	SDKToolCalled           = agentevents.SDKToolCalled
	SDKToolSearchCalled     = agentevents.SDKToolSearchCalled
	SDKToolSearchOutput     = agentevents.SDKToolSearchOutput
	SDKToolOutput           = agentevents.SDKToolOutput
	SDKReasoningCreated     = agentevents.SDKReasoningCreated
	SDKApprovalRequested    = agentevents.SDKApprovalRequested
	SDKApprovalResponse     = agentevents.SDKApprovalResponse
	SDKMCPApprovalRequested = agentevents.SDKMCPApprovalRequested
	SDKMCPApprovalResponse  = agentevents.SDKMCPApprovalResponse
	SDKMCPListTools         = agentevents.SDKMCPListTools
)

var SDKHandoffOccured = agentevents.SDKHandoffOccured

func WithRuntimeStreamEmitter(ctx context.Context, emit RuntimeStreamEmitter) context.Context {
	return agentevents.WithRuntimeStreamEmitter(ctx, emit)
}

func runtimeStreamEmitterFromContext(ctx context.Context) (RuntimeStreamEmitter, bool) {
	return agentevents.RuntimeStreamEmitterFromContext(ctx)
}

func emitRuntimeStatus(emit RuntimeStreamEmitter, status string, event RuntimeStreamEvent) {
	agentevents.EmitRuntimeStatus(emit, status, event)
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
// These interfaces and DTOs now live in agent/persistence/stores.go; root keeps
// aliases so existing imports do not break.

type PromptStoreProvider = agentpersistence.PromptStoreProvider
type PromptDocument = agentpersistence.PromptDocument
type ConversationStoreProvider = agentpersistence.ConversationStoreProvider
type ConversationDoc = agentpersistence.ConversationDoc
type ConversationMessage = agentpersistence.ConversationMessage
type ConversationUpdate = agentpersistence.ConversationUpdate
type RunStoreProvider = agentpersistence.RunStoreProvider
type RunDoc = agentpersistence.RunDoc
type RunOutputDoc = agentpersistence.RunOutputDoc

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

type PersistenceStores = agentpersistence.PersistenceStores
type ScopedPersistenceStores = agentpersistence.ScopedPersistenceStores

func NewPersistenceStores(logger *zap.Logger) *PersistenceStores {
	return agentpersistence.NewPersistenceStores(logger)
}

func NewScopedPersistenceStores(inner *PersistenceStores, scope string) *ScopedPersistenceStores {
	return agentpersistence.NewScopedPersistenceStores(inner, scope)
}
