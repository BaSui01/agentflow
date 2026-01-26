// Package a2a provides A2A (Agent-to-Agent) protocol support for cross-system agent interoperability.
// It implements the Google A2A specification for agent discovery, capability description, and message exchange.
package a2a

import (
	"github.com/BaSui01/agentflow/agent/structured"
)

// CapabilityType represents the type of capability an agent provides.
type CapabilityType string

const (
	// CapabilityTypeTask indicates the agent can execute tasks.
	CapabilityTypeTask CapabilityType = "task"
	// CapabilityTypeQuery indicates the agent can answer queries.
	CapabilityTypeQuery CapabilityType = "query"
	// CapabilityTypeStream indicates the agent supports streaming responses.
	CapabilityTypeStream CapabilityType = "stream"
)

// Capability defines an agent's capability in the A2A protocol.
type Capability struct {
	// Name is the unique identifier for this capability.
	Name string `json:"name"`
	// Description provides a human-readable description of what this capability does.
	Description string `json:"description"`
	// Type indicates the capability type (task, query, stream).
	Type CapabilityType `json:"type"`
}

// ToolDefinition defines a tool that an agent can use or expose.
type ToolDefinition struct {
	// Name is the unique identifier for this tool.
	Name string `json:"name"`
	// Description provides a human-readable description of what this tool does.
	Description string `json:"description"`
	// Parameters defines the JSON Schema for the tool's input parameters.
	Parameters *structured.JSONSchema `json:"parameters"`
}

// AgentCard represents an A2A Agent Card that describes an agent's capabilities and metadata.
// It follows the Google A2A specification for agent discovery and interoperability.
type AgentCard struct {
	// Name is the unique identifier for this agent.
	Name string `json:"name"`
	// Description provides a human-readable description of the agent's purpose.
	Description string `json:"description"`
	// URL is the endpoint where this agent can be reached.
	URL string `json:"url"`
	// Version indicates the agent's version.
	Version string `json:"version"`
	// Capabilities lists the capabilities this agent provides.
	Capabilities []Capability `json:"capabilities"`
	// InputSchema defines the JSON Schema for the agent's expected input format.
	InputSchema *structured.JSONSchema `json:"input_schema,omitempty"`
	// OutputSchema defines the JSON Schema for the agent's output format.
	OutputSchema *structured.JSONSchema `json:"output_schema,omitempty"`
	// Tools lists the tools this agent can use or expose.
	Tools []ToolDefinition `json:"tools,omitempty"`
	// Metadata contains additional key-value pairs for extensibility.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// NewAgentCard creates a new AgentCard with the required fields.
func NewAgentCard(name, description, url, version string) *AgentCard {
	return &AgentCard{
		Name:         name,
		Description:  description,
		URL:          url,
		Version:      version,
		Capabilities: make([]Capability, 0),
		Tools:        make([]ToolDefinition, 0),
		Metadata:     make(map[string]string),
	}
}

// AddCapability adds a capability to the agent card.
func (c *AgentCard) AddCapability(name, description string, capType CapabilityType) *AgentCard {
	c.Capabilities = append(c.Capabilities, Capability{
		Name:        name,
		Description: description,
		Type:        capType,
	})
	return c
}

// AddTool adds a tool definition to the agent card.
func (c *AgentCard) AddTool(name, description string, parameters *structured.JSONSchema) *AgentCard {
	c.Tools = append(c.Tools, ToolDefinition{
		Name:        name,
		Description: description,
		Parameters:  parameters,
	})
	return c
}

// SetInputSchema sets the input schema for the agent card.
func (c *AgentCard) SetInputSchema(schema *structured.JSONSchema) *AgentCard {
	c.InputSchema = schema
	return c
}

// SetOutputSchema sets the output schema for the agent card.
func (c *AgentCard) SetOutputSchema(schema *structured.JSONSchema) *AgentCard {
	c.OutputSchema = schema
	return c
}

// SetMetadata sets a metadata key-value pair.
func (c *AgentCard) SetMetadata(key, value string) *AgentCard {
	if c.Metadata == nil {
		c.Metadata = make(map[string]string)
	}
	c.Metadata[key] = value
	return c
}

// GetMetadata retrieves a metadata value by key.
func (c *AgentCard) GetMetadata(key string) (string, bool) {
	if c.Metadata == nil {
		return "", false
	}
	value, ok := c.Metadata[key]
	return value, ok
}

// HasCapability checks if the agent has a specific capability by name.
func (c *AgentCard) HasCapability(name string) bool {
	for _, cap := range c.Capabilities {
		if cap.Name == name {
			return true
		}
	}
	return false
}

// GetCapability retrieves a capability by name.
func (c *AgentCard) GetCapability(name string) *Capability {
	for i := range c.Capabilities {
		if c.Capabilities[i].Name == name {
			return &c.Capabilities[i]
		}
	}
	return nil
}

// HasTool checks if the agent has a specific tool by name.
func (c *AgentCard) HasTool(name string) bool {
	for _, tool := range c.Tools {
		if tool.Name == name {
			return true
		}
	}
	return false
}

// GetTool retrieves a tool definition by name.
func (c *AgentCard) GetTool(name string) *ToolDefinition {
	for i := range c.Tools {
		if c.Tools[i].Name == name {
			return &c.Tools[i]
		}
	}
	return nil
}

// Validate checks if the AgentCard has all required fields.
func (c *AgentCard) Validate() error {
	if c.Name == "" {
		return ErrMissingName
	}
	if c.Description == "" {
		return ErrMissingDescription
	}
	if c.URL == "" {
		return ErrMissingURL
	}
	if c.Version == "" {
		return ErrMissingVersion
	}
	return nil
}
