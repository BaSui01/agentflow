// Package a2a provides A2A (Agent-to-Agent) protocol support.
package a2a

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/BaSui01/agentflow/agent/structured"
	"github.com/BaSui01/agentflow/llm"
)

// AgentConfigProvider defines the interface for accessing agent configuration.
// This allows the generator to work with any agent implementation.
type AgentConfigProvider interface {
	ID() string
	Name() string
	Type() AgentType
	Description() string
	Tools() []string
	Metadata() map[string]string
}

// AgentType represents the type of an agent.
type AgentType string

// ToolSchemaProvider provides tool schemas for an agent.
type ToolSchemaProvider interface {
	GetAllowedTools(agentID string) []llm.ToolSchema
}

// AgentCardGenerator generates Agent Cards from agent configurations.
type AgentCardGenerator struct {
	// defaultVersion is used when the agent doesn't specify a version.
	defaultVersion string
}

// NewAgentCardGenerator creates a new AgentCardGenerator.
func NewAgentCardGenerator() *AgentCardGenerator {
	return &AgentCardGenerator{
		defaultVersion: "1.0.0",
	}
}

// NewAgentCardGeneratorWithVersion creates a new AgentCardGenerator with a custom default version.
func NewAgentCardGeneratorWithVersion(version string) *AgentCardGenerator {
	return &AgentCardGenerator{
		defaultVersion: version,
	}
}

// Generate creates an AgentCard from an agent configuration and base URL.
// The baseURL should be the endpoint where the agent can be reached.
func (g *AgentCardGenerator) Generate(config AgentConfigProvider, baseURL string) *AgentCard {
	return g.GenerateWithTools(config, baseURL, nil)
}

// GenerateWithTools creates an AgentCard with tool definitions from a ToolSchemaProvider.
func (g *AgentCardGenerator) GenerateWithTools(config AgentConfigProvider, baseURL string, toolProvider ToolSchemaProvider) *AgentCard {
	// Build the agent URL
	agentURL := buildAgentURL(baseURL, config.ID())

	// Determine version from metadata or use default
	version := g.defaultVersion
	if meta := config.Metadata(); meta != nil {
		if v, ok := meta["version"]; ok && v != "" {
			version = v
		}
	}

	// Create the agent card
	card := NewAgentCard(
		config.Name(),
		config.Description(),
		agentURL,
		version,
	)

	// Add default capability based on agent type
	g.addCapabilitiesFromType(card, config.Type())

	// Add tools if provider is available
	if toolProvider != nil {
		g.addToolsFromProvider(card, config.ID(), toolProvider)
	}

	// Copy metadata
	if meta := config.Metadata(); meta != nil {
		for k, v := range meta {
			if k != "version" { // version is already used
				card.SetMetadata(k, v)
			}
		}
	}

	// Add agent type to metadata
	card.SetMetadata("agent_type", string(config.Type()))
	card.SetMetadata("agent_id", config.ID())

	return card
}

// addCapabilitiesFromType adds default capabilities based on agent type.
func (g *AgentCardGenerator) addCapabilitiesFromType(card *AgentCard, agentType AgentType) {
	typeStr := string(agentType)

	switch typeStr {
	case "assistant":
		card.AddCapability("chat", "Interactive conversation and assistance", CapabilityTypeQuery)
		card.AddCapability("task_execution", "Execute tasks based on user requests", CapabilityTypeTask)
	case "analyzer":
		card.AddCapability("analysis", "Analyze data and provide insights", CapabilityTypeTask)
	case "translator":
		card.AddCapability("translation", "Translate text between languages", CapabilityTypeTask)
	case "summarizer":
		card.AddCapability("summarization", "Summarize text content", CapabilityTypeTask)
	case "reviewer":
		card.AddCapability("review", "Review and provide feedback on content", CapabilityTypeTask)
	default:
		// Generic agent gets a basic task capability
		card.AddCapability("execute", "Execute general tasks", CapabilityTypeTask)
	}
}

// addToolsFromProvider adds tool definitions from a ToolSchemaProvider.
func (g *AgentCardGenerator) addToolsFromProvider(card *AgentCard, agentID string, provider ToolSchemaProvider) {
	schemas := provider.GetAllowedTools(agentID)
	for _, schema := range schemas {
		toolDef := convertToolSchema(schema)
		card.Tools = append(card.Tools, toolDef)
	}
}

// convertToolSchema converts an llm.ToolSchema to a ToolDefinition.
func convertToolSchema(schema llm.ToolSchema) ToolDefinition {
	var params *structured.JSONSchema

	// Parse the parameters JSON into a JSONSchema
	if len(schema.Parameters) > 0 {
		params = &structured.JSONSchema{}
		if err := json.Unmarshal(schema.Parameters, params); err != nil {
			// If parsing fails, create a basic object schema
			params = &structured.JSONSchema{
				Type:        structured.TypeObject,
				Description: "Tool parameters",
			}
		}
	}

	return ToolDefinition{
		Name:        schema.Name,
		Description: schema.Description,
		Parameters:  params,
	}
}

// buildAgentURL constructs the full agent URL from base URL and agent ID.
func buildAgentURL(baseURL, agentID string) string {
	// Ensure baseURL doesn't end with slash
	baseURL = strings.TrimSuffix(baseURL, "/")

	// Build the agent endpoint URL
	return fmt.Sprintf("%s/agents/%s", baseURL, agentID)
}

// SimpleAgentConfig is a simple implementation of AgentConfigProvider for testing and basic usage.
type SimpleAgentConfig struct {
	AgentID          string
	AgentName        string
	AgentType        AgentType
	AgentDescription string
	AgentTools       []string
	AgentMetadata    map[string]string
}

// ID returns the agent ID.
func (c *SimpleAgentConfig) ID() string { return c.AgentID }

// Name returns the agent name.
func (c *SimpleAgentConfig) Name() string { return c.AgentName }

// Type returns the agent type.
func (c *SimpleAgentConfig) Type() AgentType { return c.AgentType }

// Description returns the agent description.
func (c *SimpleAgentConfig) Description() string { return c.AgentDescription }

// Tools returns the list of tool names.
func (c *SimpleAgentConfig) Tools() []string { return c.AgentTools }

// Metadata returns the agent metadata.
func (c *SimpleAgentConfig) Metadata() map[string]string { return c.AgentMetadata }
