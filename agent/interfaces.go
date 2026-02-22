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
	RecordEpisode(ctx context.Context, event any) error
}

// ObservabilityRunner provides metrics, tracing, and logging.
// Implemented by: *observability.ObservabilitySystem (agent/observability/)
type ObservabilityRunner interface {
	StartTrace(traceID, agentID string)
	EndTrace(traceID, status string, err error)
	RecordTask(agentID string, success bool, duration time.Duration, tokens int, cost, quality float64)
}
