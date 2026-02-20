package declarative

// AgentDefinition is a declarative Agent specification.
// It extends workflow/dsl.AgentDef with identity, versioning, and feature toggles.
// This struct is designed to be deserialized from YAML or JSON files.
type AgentDefinition struct {
	// Identity
	ID          string `yaml:"id" json:"id"`
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	Version     string `yaml:"version,omitempty" json:"version,omitempty"`

	// LLM configuration
	Model       string  `yaml:"model" json:"model"`
	Provider    string  `yaml:"provider,omitempty" json:"provider,omitempty"`
	Temperature float64 `yaml:"temperature,omitempty" json:"temperature,omitempty"`
	MaxTokens   int     `yaml:"max_tokens,omitempty" json:"max_tokens,omitempty"`

	// Prompt
	SystemPrompt string `yaml:"system_prompt,omitempty" json:"system_prompt,omitempty"`

	// Tools
	Tools []string `yaml:"tools,omitempty" json:"tools,omitempty"`

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
