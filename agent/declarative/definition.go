package declarative

// AgentDefinition is a declarative Agent specification.
// It extends workflow/dsl.AgentDef with identity, versioning, memory,
// guardrails, and feature toggles.
// This struct is designed to be deserialized from YAML or JSON files.
type AgentDefinition struct {
	// Identity
	ID          string `yaml:"id" json:"id"`
	Name        string `yaml:"name" json:"name"`
	Type        string `yaml:"type,omitempty" json:"type,omitempty"` // "react", "plan_execute", "rewoo", etc.
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	Version     string `yaml:"version,omitempty" json:"version,omitempty"`

	// LLM configuration
	Model       string  `yaml:"model" json:"model"`
	Provider    string  `yaml:"provider,omitempty" json:"provider,omitempty"`
	Temperature float64 `yaml:"temperature,omitempty" json:"temperature,omitempty"`
	MaxTokens   int     `yaml:"max_tokens,omitempty" json:"max_tokens,omitempty"`

	// Prompt
	SystemPrompt string `yaml:"system_prompt,omitempty" json:"system_prompt,omitempty"`

	// Tools (string names for simple references, or ToolDefinition for richer config)
	Tools           []string         `yaml:"tools,omitempty" json:"tools,omitempty"`
	ToolDefinitions []ToolDefinition `yaml:"tool_definitions,omitempty" json:"tool_definitions,omitempty"`

	// Memory configuration
	Memory *MemoryConfig `yaml:"memory,omitempty" json:"memory,omitempty"`

	// Guardrails configuration
	Guardrails *GuardrailsConfig `yaml:"guardrails,omitempty" json:"guardrails,omitempty"`

	// Optional feature toggles
	Features AgentFeatures `yaml:"features,omitempty" json:"features,omitempty"`

	// Metadata
	Metadata map[string]string `yaml:"metadata,omitempty" json:"metadata,omitempty"`
}

// AgentFeatures controls optional Agent capabilities.
type AgentFeatures struct {
	EnableReflection     bool `yaml:"enable_reflection,omitempty" json:"enable_reflection,omitempty"`
	EnableToolSelection  bool `yaml:"enable_tool_selection,omitempty" json:"enable_tool_selection,omitempty"`
	EnablePromptEnhancer bool `yaml:"enable_prompt_enhancer,omitempty" json:"enable_prompt_enhancer,omitempty"`
	EnableSkills         bool `yaml:"enable_skills,omitempty" json:"enable_skills,omitempty"`
	EnableMCP            bool `yaml:"enable_mcp,omitempty" json:"enable_mcp,omitempty"`
	EnableObservability  bool `yaml:"enable_observability,omitempty" json:"enable_observability,omitempty"`
	MaxReActIterations   int  `yaml:"max_react_iterations,omitempty" json:"max_react_iterations,omitempty"`
}

// ToolDefinition describes a tool with name and description.
// Use this when you need richer tool metadata than a plain string name.
type ToolDefinition struct {
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
}

// MemoryConfig configures the Agent's memory subsystem.
type MemoryConfig struct {
	Type     string `yaml:"type" json:"type"`         // "short_term", "long_term", "both"
	Capacity int    `yaml:"capacity" json:"capacity"` // max number of memory entries
}

// GuardrailsConfig configures input/output validation guardrails.
type GuardrailsConfig struct {
	MaxRetries      int    `yaml:"max_retries,omitempty" json:"max_retries,omitempty"`
	OnInputFailure  string `yaml:"on_input_failure,omitempty" json:"on_input_failure,omitempty"`   // "reject", "warn", "ignore"
	OnOutputFailure string `yaml:"on_output_failure,omitempty" json:"on_output_failure,omitempty"` // "reject", "warn", "ignore"
}
