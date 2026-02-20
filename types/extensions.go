package types

import "context"

// ============================================================
// Extension Interfaces
// These interfaces define contracts for optional Agent capabilities.
// They replace the use of any in BaseAgent for type safety.
// ============================================================

// ReflectionExtension provides self-evaluation and iterative improvement.
type ReflectionExtension interface {
	// ExecuteWithReflection executes a task with reflection loop.
	ExecuteWithReflection(ctx context.Context, input any) (any, error)
	// IsEnabled returns whether reflection is enabled.
	IsEnabled() bool
}

// ToolSelectionExtension provides dynamic tool selection based on context.
type ToolSelectionExtension interface {
	// SelectTools selects relevant tools for a given query.
	SelectTools(ctx context.Context, query string, candidates []ToolSchema) ([]ToolSchema, error)
	// GetSelectionStrategy returns the current selection strategy.
	GetSelectionStrategy() string
}

// PromptEnhancerExtension provides prompt optimization and enhancement.
type PromptEnhancerExtension interface {
	// Enhance improves a prompt for better LLM performance.
	Enhance(ctx context.Context, prompt string, context map[string]any) (string, error)
	// GetEnhancementMode returns the current enhancement mode.
	GetEnhancementMode() string
}

// SkillsExtension provides dynamic skill loading and execution.
type SkillsExtension interface {
	// LoadSkill loads a skill by name.
	LoadSkill(ctx context.Context, name string) error
	// ExecuteSkill executes a loaded skill.
	ExecuteSkill(ctx context.Context, name string, input any) (any, error)
	// ListSkills returns available skills.
	ListSkills() []string
}

// MCPExtension provides Model Context Protocol support.
type MCPExtension interface {
	// Connect establishes MCP connection.
	Connect(ctx context.Context, endpoint string) error
	// Disconnect closes MCP connection.
	Disconnect(ctx context.Context) error
	// SendMessage sends a message via MCP.
	SendMessage(ctx context.Context, message any) (any, error)
	// IsConnected returns connection status.
	IsConnected() bool
}

// EnhancedMemoryExtension provides advanced memory capabilities.
type EnhancedMemoryExtension interface {
	// StoreWithImportance stores memory with importance scoring.
	StoreWithImportance(ctx context.Context, content string, importance float64) error
	// RetrieveByRelevance retrieves memories by relevance to query.
	RetrieveByRelevance(ctx context.Context, query string, topK int) ([]MemoryRecord, error)
	// Consolidate performs memory consolidation.
	Consolidate(ctx context.Context) error
	// Decay applies memory decay based on time and access patterns.
	Decay(ctx context.Context) error
}

// ObservabilityExtension provides metrics, tracing, and logging.
type ObservabilityExtension interface {
	// RecordMetric records a metric value.
	RecordMetric(name string, value float64, tags map[string]string)
	// StartSpan starts a new trace span.
	StartSpan(ctx context.Context, name string) (context.Context, SpanHandle)
	// LogEvent logs an event with structured data.
	LogEvent(level string, message string, fields map[string]any)
}

// SpanHandle represents a trace span that can be ended.
type SpanHandle interface {
	// End ends the span.
	End()
	// SetAttribute sets an attribute on the span.
	SetAttribute(key string, value any)
	// RecordError records an error on the span.
	RecordError(err error)
}

// GuardrailsExtension provides input/output validation.
type GuardrailsExtension interface {
	// ValidateInput validates input before processing.
	ValidateInput(ctx context.Context, input string) (*ValidationResult, error)
	// ValidateOutput validates output before returning.
	ValidateOutput(ctx context.Context, output string) (*ValidationResult, error)
	// FilterOutput applies filters to output.
	FilterOutput(ctx context.Context, output string) (string, error)
}

// ValidationResult represents the result of a validation check.
type ValidationResult struct {
	Valid    bool              `json:"valid"`
	Errors   []ValidationError `json:"errors,omitempty"`
	Warnings []string          `json:"warnings,omitempty"`
	Filtered string            `json:"filtered,omitempty"`
}

// ValidationError represents a single validation error.
type ValidationError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
}

// ============================================================
// Extension Registry
// Provides a type-safe way to manage extensions.
// ============================================================

// ExtensionRegistry holds all optional extensions for an agent.
type ExtensionRegistry struct {
	Reflection    ReflectionExtension
	ToolSelection ToolSelectionExtension
	PromptEnhancer PromptEnhancerExtension
	Skills        SkillsExtension
	MCP           MCPExtension
	EnhancedMemory EnhancedMemoryExtension
	Observability ObservabilityExtension
	Guardrails    GuardrailsExtension
}

// HasReflection returns true if reflection extension is available.
func (r *ExtensionRegistry) HasReflection() bool {
	return r.Reflection != nil
}

// HasToolSelection returns true if tool selection extension is available.
func (r *ExtensionRegistry) HasToolSelection() bool {
	return r.ToolSelection != nil
}

// HasPromptEnhancer returns true if prompt enhancer extension is available.
func (r *ExtensionRegistry) HasPromptEnhancer() bool {
	return r.PromptEnhancer != nil
}

// HasSkills returns true if skills extension is available.
func (r *ExtensionRegistry) HasSkills() bool {
	return r.Skills != nil
}

// HasMCP returns true if MCP extension is available.
func (r *ExtensionRegistry) HasMCP() bool {
	return r.MCP != nil
}

// HasEnhancedMemory returns true if enhanced memory extension is available.
func (r *ExtensionRegistry) HasEnhancedMemory() bool {
	return r.EnhancedMemory != nil
}

// HasObservability returns true if observability extension is available.
func (r *ExtensionRegistry) HasObservability() bool {
	return r.Observability != nil
}

// HasGuardrails returns true if guardrails extension is available.
func (r *ExtensionRegistry) HasGuardrails() bool {
	return r.Guardrails != nil
}
