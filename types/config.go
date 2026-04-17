package types

import "time"

// ============================================================
// Agent Configuration Types
// Provides a structured, modular configuration system.
// ============================================================

// AgentConfig represents the complete configuration for an agent (runtime behavior).
// Note: This is distinct from config.AgentConfig which is for deployment configuration
// (YAML/env loading, flat structure). This type uses a modular nested structure
// (Core/LLM/Features/Extensions) for runtime agent behavior configuration.
// Deployment-to-runtime conversion is implemented in config.AgentConfig.ToRuntimeConfig().
type AgentConfig struct {
	// Core configuration (required)
	Core CoreConfig `json:"core"`

	// LLM configuration
	LLM LLMConfig `json:"llm"`

	// Runtime execution behavior configuration
	Runtime RuntimeConfig `json:"runtime,omitempty"`

	// Context orchestration configuration
	Context *ContextConfig `json:"context,omitempty"`

	// Feature configurations (optional)
	Features FeaturesConfig `json:"features,omitempty"`

	// Extension configurations (optional)
	Extensions ExtensionsConfig `json:"extensions,omitempty"`

	// Metadata for custom data
	Metadata map[string]string `json:"metadata,omitempty"`
}

// CoreConfig contains essential agent identity and behavior settings.
type CoreConfig struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}

// LLMConfig contains LLM-related settings for runtime agent behavior (model parameters).
// Note: This is distinct from config.LLMConfig which contains infrastructure connection
// parameters (APIKey, BaseURL, Timeout, MaxRetries) for deployment configuration.
type LLMConfig struct {
	Model       string   `json:"model"`
	Provider    string   `json:"provider,omitempty"`
	MaxTokens   int      `json:"max_tokens,omitempty"`
	Temperature float32  `json:"temperature,omitempty"`
	TopP        float32  `json:"top_p,omitempty"`
	Stop        []string `json:"stop,omitempty"`
}

// RuntimeConfig contains runtime execution behavior options.
type RuntimeConfig struct {
	SystemPrompt       string   `json:"system_prompt,omitempty"`
	Tools              []string `json:"tools,omitempty"`
	Handoffs           []string `json:"handoffs,omitempty"`
	MaxReActIterations int      `json:"max_react_iterations,omitempty"`
	MaxLoopIterations  int      `json:"max_loop_iterations,omitempty"`
	ToolModel          string   `json:"tool_model,omitempty"`
}

// ContextConfig configures context assembly, budgeting, and compression.
type ContextConfig struct {
	Enabled                          bool    `json:"enabled"`
	MaxContextTokens                 int     `json:"max_context_tokens,omitempty"`
	ReserveForOutput                 int     `json:"reserve_for_output,omitempty"`
	SoftLimit                        float64 `json:"soft_limit,omitempty"`
	WarnLimit                        float64 `json:"warn_limit,omitempty"`
	HardLimit                        float64 `json:"hard_limit,omitempty"`
	TargetUsage                      float64 `json:"target_usage,omitempty"`
	KeepSystem                       bool    `json:"keep_system,omitempty"`
	KeepLastN                        int     `json:"keep_last_n,omitempty"`
	EnableSummarize                  bool    `json:"enable_summarize,omitempty"`
	EnableMetrics                    bool    `json:"enable_metrics,omitempty"`
	TraceFeedbackEnabled             bool    `json:"trace_feedback_enabled,omitempty"`
	TraceFeedbackComplexityThreshold int     `json:"trace_feedback_complexity_threshold,omitempty"`
	TraceSynopsisMinScore            int     `json:"trace_synopsis_min_score,omitempty"`
	TraceHistoryMinScore             int     `json:"trace_history_min_score,omitempty"`
	TraceMemoryRecallMinScore        int     `json:"trace_memory_recall_min_score,omitempty"`
	TraceHistoryMaxUsageRatio        float64 `json:"trace_history_max_usage_ratio,omitempty"`
	MemoryBudgetRatio                float64 `json:"memory_budget_ratio,omitempty"`
	RetrievalBudgetRatio             float64 `json:"retrieval_budget_ratio,omitempty"`
	ToolStateBudgetRatio             float64 `json:"tool_state_budget_ratio,omitempty"`
}

// DefaultContextConfig returns sensible defaults for context orchestration.
func DefaultContextConfig() *ContextConfig {
	return &ContextConfig{
		Enabled:                          true,
		MaxContextTokens:                 32000,
		ReserveForOutput:                 4096,
		SoftLimit:                        0.7,
		WarnLimit:                        0.85,
		HardLimit:                        0.95,
		TargetUsage:                      0.5,
		KeepSystem:                       true,
		KeepLastN:                        2,
		EnableSummarize:                  true,
		EnableMetrics:                    true,
		TraceFeedbackEnabled:             true,
		TraceFeedbackComplexityThreshold: 2,
		TraceSynopsisMinScore:            2,
		TraceHistoryMinScore:             3,
		TraceMemoryRecallMinScore:        2,
		TraceHistoryMaxUsageRatio:        0.85,
		MemoryBudgetRatio:                0.2,
		RetrievalBudgetRatio:             0.2,
		ToolStateBudgetRatio:             0.2,
	}
}

func (c *ContextConfig) IsEnabled() bool { return c != nil && c.Enabled }

// FeaturesConfig contains optional feature configurations.
// Each feature is enabled by providing its configuration (nil = disabled).
type FeaturesConfig struct {
	Reflection     *ReflectionConfig     `json:"reflection,omitempty"`
	ToolSelection  *ToolSelectionConfig  `json:"tool_selection,omitempty"`
	PromptEnhancer *PromptEnhancerConfig `json:"prompt_enhancer,omitempty"`
	Guardrails     *GuardrailsConfig     `json:"guardrails,omitempty"`
	Memory         *MemoryConfig         `json:"memory,omitempty"`
}

// ExtensionsConfig contains extension-specific configurations.
type ExtensionsConfig struct {
	Skills        *SkillsConfig        `json:"skills,omitempty"`
	MCP           *MCPConfig           `json:"mcp,omitempty"`
	LSP           *LSPConfig           `json:"lsp,omitempty"`
	Observability *ObservabilityConfig `json:"observability,omitempty"`
}

// ============================================================
// Feature Configurations
// ============================================================

// ReflectionConfig configures the reflection/self-improvement feature.
type ReflectionConfig struct {
	Enabled       bool    `json:"enabled"`
	MaxIterations int     `json:"max_iterations,omitempty"`
	MinQuality    float64 `json:"min_quality,omitempty"`
	CriticPrompt  string  `json:"critic_prompt,omitempty"`
}

// DefaultReflectionConfig returns sensible defaults for reflection.
func DefaultReflectionConfig() *ReflectionConfig {
	return &ReflectionConfig{
		Enabled:       true,
		MaxIterations: 3,
		MinQuality:    0.7,
	}
}

func (c *ReflectionConfig) IsEnabled() bool { return c != nil && c.Enabled }

// ToolSelectionConfig configures dynamic tool selection.
type ToolSelectionConfig struct {
	Enabled             bool    `json:"enabled"`
	MaxTools            int     `json:"max_tools,omitempty"`
	SimilarityThreshold float64 `json:"similarity_threshold,omitempty"`
	Strategy            string  `json:"strategy,omitempty"` // "semantic", "keyword", "hybrid"
}

// DefaultToolSelectionConfig returns sensible defaults for tool selection.
func DefaultToolSelectionConfig() *ToolSelectionConfig {
	return &ToolSelectionConfig{
		Enabled:             true,
		MaxTools:            5,
		SimilarityThreshold: 0.7,
		Strategy:            "hybrid",
	}
}

func (c *ToolSelectionConfig) IsEnabled() bool { return c != nil && c.Enabled }

// PromptEnhancerConfig configures prompt enhancement.
type PromptEnhancerConfig struct {
	Enabled bool   `json:"enabled"`
	Mode    string `json:"mode,omitempty"` // "basic", "advanced", "custom"
}

// DefaultPromptEnhancerConfig returns sensible defaults.
func DefaultPromptEnhancerConfig() *PromptEnhancerConfig {
	return &PromptEnhancerConfig{
		Enabled: true,
		Mode:    "basic",
	}
}

func (c *PromptEnhancerConfig) IsEnabled() bool { return c != nil && c.Enabled }

// GuardrailsConfig configures input/output validation.
type GuardrailsConfig struct {
	Enabled            bool     `json:"enabled"`
	MaxInputLength     int      `json:"max_input_length,omitempty"`
	BlockedKeywords    []string `json:"blocked_keywords,omitempty"`
	PIIDetection       bool     `json:"pii_detection,omitempty"`
	InjectionDetection bool     `json:"injection_detection,omitempty"`
	MaxRetries         int      `json:"max_retries,omitempty"`
	OnInputFailure     string   `json:"on_input_failure,omitempty"`
	OnOutputFailure    string   `json:"on_output_failure,omitempty"`
}

// DefaultGuardrailsConfig returns sensible defaults.
func DefaultGuardrailsConfig() *GuardrailsConfig {
	return &GuardrailsConfig{
		Enabled:            true,
		MaxInputLength:     10000,
		PIIDetection:       true,
		InjectionDetection: true,
		MaxRetries:         2,
	}
}

func (c *GuardrailsConfig) IsEnabled() bool { return c != nil && c.Enabled }

// MemoryConfig configures the memory system.
type MemoryConfig struct {
	Enabled          bool          `json:"enabled"`
	ShortTermTTL     time.Duration `json:"short_term_ttl,omitempty"`
	MaxShortTermSize int           `json:"max_short_term_size,omitempty"`
	EnableLongTerm   bool          `json:"enable_long_term,omitempty"`
	EnableEpisodic   bool          `json:"enable_episodic,omitempty"`
	DecayEnabled     bool          `json:"decay_enabled,omitempty"`
}

// DefaultMemoryConfig returns sensible defaults.
func DefaultMemoryConfig() *MemoryConfig {
	return &MemoryConfig{
		Enabled:          true,
		ShortTermTTL:     time.Hour,
		MaxShortTermSize: 100,
		EnableLongTerm:   true,
		EnableEpisodic:   true,
		DecayEnabled:     true,
	}
}

func (c *MemoryConfig) IsEnabled() bool { return c != nil && c.Enabled }

// ============================================================
// Extension Configurations
// ============================================================

// SkillsConfig configures the skills system.
type SkillsConfig struct {
	Enabled    bool     `json:"enabled"`
	SkillPaths []string `json:"skill_paths,omitempty"`
	AutoLoad   bool     `json:"auto_load,omitempty"`
	MaxLoaded  int      `json:"max_loaded,omitempty"`
}

func (c *SkillsConfig) IsEnabled() bool { return c != nil && c.Enabled }

// MCPConfig configures Model Context Protocol integration.
type MCPConfig struct {
	Enabled   bool          `json:"enabled"`
	Endpoint  string        `json:"endpoint,omitempty"`
	AuthToken string        `json:"auth_token,omitempty"`
	Timeout   time.Duration `json:"timeout,omitempty"`
}

func (c *MCPConfig) IsEnabled() bool { return c != nil && c.Enabled }

// LSPConfig configures Language Server Protocol integration.
type LSPConfig struct {
	Enabled bool `json:"enabled"`
}

func (c *LSPConfig) IsEnabled() bool { return c != nil && c.Enabled }

// ObservabilityConfig configures metrics, tracing, and logging.
type ObservabilityConfig struct {
	Enabled        bool   `json:"enabled"`
	MetricsEnabled bool   `json:"metrics_enabled,omitempty"`
	TracingEnabled bool   `json:"tracing_enabled,omitempty"`
	LogLevel       string `json:"log_level,omitempty"`
	ServiceName    string `json:"service_name,omitempty"`
}

// DefaultObservabilityConfig returns sensible defaults.
func DefaultObservabilityConfig() *ObservabilityConfig {
	return &ObservabilityConfig{
		Enabled:        true,
		MetricsEnabled: true,
		TracingEnabled: true,
		LogLevel:       "info",
	}
}

func (c *ObservabilityConfig) IsEnabled() bool { return c != nil && c.Enabled }

// ============================================================
// Configuration Helpers
// ============================================================

func (c *AgentConfig) IsReflectionEnabled() bool     { return c.Features.Reflection.IsEnabled() }
func (c *AgentConfig) IsToolSelectionEnabled() bool  { return c.Features.ToolSelection.IsEnabled() }
func (c *AgentConfig) IsContextEnabled() bool        { return c.Context.IsEnabled() }
func (c *AgentConfig) IsGuardrailsEnabled() bool     { return c.Features.Guardrails.IsEnabled() }
func (c *AgentConfig) IsMemoryEnabled() bool         { return c.Features.Memory.IsEnabled() }
func (c *AgentConfig) IsPromptEnhancerEnabled() bool { return c.Features.PromptEnhancer.IsEnabled() }
func (c *AgentConfig) IsSkillsEnabled() bool         { return c.Extensions.Skills.IsEnabled() }
func (c *AgentConfig) IsMCPEnabled() bool            { return c.Extensions.MCP.IsEnabled() }
func (c *AgentConfig) IsLSPEnabled() bool            { return c.Extensions.LSP.IsEnabled() }
func (c *AgentConfig) IsObservabilityEnabled() bool  { return c.Extensions.Observability.IsEnabled() }

// Validate validates the configuration.
func (c *AgentConfig) Validate() error {
	if c.Core.ID == "" {
		return &Error{Code: ErrInvalidRequest, Message: "agent ID is required"}
	}
	if c.Core.Name == "" {
		return &Error{Code: ErrInvalidRequest, Message: "agent name is required"}
	}
	if c.LLM.Model == "" {
		return &Error{Code: ErrInvalidRequest, Message: "LLM model is required"}
	}
	return nil
}
