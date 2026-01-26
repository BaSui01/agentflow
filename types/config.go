// Package types provides unified type definitions for the AgentFlow framework.
package types

import "time"

// ============================================================
// Agent Configuration Types
// Provides a structured, modular configuration system.
// ============================================================

// AgentConfig represents the complete configuration for an agent.
// This replaces the flat Config structure with a more organized approach.
type AgentConfig struct {
	// Core configuration (required)
	Core CoreConfig `json:"core"`

	// LLM configuration
	LLM LLMConfig `json:"llm"`

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

// LLMConfig contains LLM-related settings.
type LLMConfig struct {
	Model       string   `json:"model"`
	Provider    string   `json:"provider,omitempty"`
	MaxTokens   int      `json:"max_tokens,omitempty"`
	Temperature float32  `json:"temperature,omitempty"`
	TopP        float32  `json:"top_p,omitempty"`
	Stop        []string `json:"stop,omitempty"`
}

// FeaturesConfig contains optional feature configurations.
// Each feature is enabled by providing its configuration (nil = disabled).
type FeaturesConfig struct {
	Reflection    *ReflectionConfig    `json:"reflection,omitempty"`
	ToolSelection *ToolSelectionConfig `json:"tool_selection,omitempty"`
	PromptEnhancer *PromptEnhancerConfig `json:"prompt_enhancer,omitempty"`
	Guardrails    *GuardrailsConfig    `json:"guardrails,omitempty"`
	Memory        *MemoryConfig        `json:"memory,omitempty"`
}

// ExtensionsConfig contains extension-specific configurations.
type ExtensionsConfig struct {
	Skills        *SkillsConfig        `json:"skills,omitempty"`
	MCP           *MCPConfig           `json:"mcp,omitempty"`
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

// ToolSelectionConfig configures dynamic tool selection.
type ToolSelectionConfig struct {
	Enabled         bool    `json:"enabled"`
	MaxTools        int     `json:"max_tools,omitempty"`
	SimilarityThreshold float64 `json:"similarity_threshold,omitempty"`
	Strategy        string  `json:"strategy,omitempty"` // "semantic", "keyword", "hybrid"
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

// GuardrailsConfig configures input/output validation.
type GuardrailsConfig struct {
	Enabled            bool     `json:"enabled"`
	MaxInputLength     int      `json:"max_input_length,omitempty"`
	BlockedKeywords    []string `json:"blocked_keywords,omitempty"`
	PIIDetection       bool     `json:"pii_detection,omitempty"`
	InjectionDetection bool     `json:"injection_detection,omitempty"`
	MaxRetries         int      `json:"max_retries,omitempty"`
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

// ============================================================
// Extension Configurations
// ============================================================

// SkillsConfig configures the skills system.
type SkillsConfig struct {
	Enabled     bool     `json:"enabled"`
	SkillPaths  []string `json:"skill_paths,omitempty"`
	AutoLoad    bool     `json:"auto_load,omitempty"`
	MaxLoaded   int      `json:"max_loaded,omitempty"`
}

// MCPConfig configures Model Context Protocol integration.
type MCPConfig struct {
	Enabled   bool   `json:"enabled"`
	Endpoint  string `json:"endpoint,omitempty"`
	AuthToken string `json:"auth_token,omitempty"`
	Timeout   time.Duration `json:"timeout,omitempty"`
}

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

// ============================================================
// Configuration Helpers
// ============================================================

// IsReflectionEnabled checks if reflection is enabled.
func (c *AgentConfig) IsReflectionEnabled() bool {
	return c.Features.Reflection != nil && c.Features.Reflection.Enabled
}

// IsToolSelectionEnabled checks if tool selection is enabled.
func (c *AgentConfig) IsToolSelectionEnabled() bool {
	return c.Features.ToolSelection != nil && c.Features.ToolSelection.Enabled
}

// IsGuardrailsEnabled checks if guardrails are enabled.
func (c *AgentConfig) IsGuardrailsEnabled() bool {
	return c.Features.Guardrails != nil && c.Features.Guardrails.Enabled
}

// IsMemoryEnabled checks if memory is enabled.
func (c *AgentConfig) IsMemoryEnabled() bool {
	return c.Features.Memory != nil && c.Features.Memory.Enabled
}

// IsSkillsEnabled checks if skills are enabled.
func (c *AgentConfig) IsSkillsEnabled() bool {
	return c.Extensions.Skills != nil && c.Extensions.Skills.Enabled
}

// IsMCPEnabled checks if MCP is enabled.
func (c *AgentConfig) IsMCPEnabled() bool {
	return c.Extensions.MCP != nil && c.Extensions.MCP.Enabled
}

// IsObservabilityEnabled checks if observability is enabled.
func (c *AgentConfig) IsObservabilityEnabled() bool {
	return c.Extensions.Observability != nil && c.Extensions.Observability.Enabled
}

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
