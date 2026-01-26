package a2a

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/agent/structured"
	"github.com/BaSui01/agentflow/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

// Feature: agent-framework-2026-enhancements, Property 9: Agent Card Completeness
// Validates: Requirements 5.1, 5.2, 5.3
// For any registered Agent, the generated AgentCard should contain non-empty Name, Description,
// URL, Version fields, and the Capabilities list should reflect the Agent's actual capabilities.

// propMockAgent implements agent.Agent interface for property testing.
type propMockAgent struct {
	id          string
	name        string
	agentType   agent.AgentType
	description string
	tools       []string
	metadata    map[string]string
}

func (m *propMockAgent) ID() string                         { return m.id }
func (m *propMockAgent) Name() string                       { return m.name }
func (m *propMockAgent) Type() agent.AgentType              { return m.agentType }
func (m *propMockAgent) State() agent.State                 { return agent.StateReady }
func (m *propMockAgent) Init(ctx context.Context) error     { return nil }
func (m *propMockAgent) Teardown(ctx context.Context) error { return nil }
func (m *propMockAgent) Plan(ctx context.Context, input *agent.Input) (*agent.PlanResult, error) {
	return nil, nil
}
func (m *propMockAgent) Execute(ctx context.Context, input *agent.Input) (*agent.Output, error) {
	return &agent.Output{Content: "test"}, nil
}
func (m *propMockAgent) Observe(ctx context.Context, feedback *agent.Feedback) error { return nil }

// Description returns the agent description.
func (m *propMockAgent) Description() string { return m.description }

// Tools returns the agent tools.
func (m *propMockAgent) Tools() []string { return m.tools }

// Metadata returns the agent metadata.
func (m *propMockAgent) Metadata() map[string]string { return m.metadata }

// propToolProvider implements ToolSchemaProvider for property testing.
type propToolProvider struct {
	tools map[string][]llm.ToolSchema
}

func (p *propToolProvider) GetAllowedTools(agentID string) []llm.ToolSchema {
	if tools, ok := p.tools[agentID]; ok {
		return tools
	}
	return nil
}

// TestProperty_AgentCard_Completeness tests that generated AgentCards have all required fields.
func TestProperty_AgentCard_Completeness(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Generate random agent configuration
		agentID := rapid.StringMatching(`[a-z][a-z0-9-]{2,20}`).Draw(rt, "agentID")
		agentName := rapid.StringMatching(`[A-Z][a-zA-Z0-9 ]{2,30}`).Draw(rt, "agentName")
		agentDesc := rapid.StringMatching(`[A-Za-z][a-zA-Z0-9 ,.]{10,100}`).Draw(rt, "agentDesc")
		agentType := rapid.SampledFrom([]agent.AgentType{
			agent.TypeAssistant,
			agent.TypeAnalyzer,
			agent.TypeTranslator,
			agent.TypeSummarizer,
			agent.TypeReviewer,
		}).Draw(rt, "agentType")
		baseURL := rapid.SampledFrom([]string{
			"https://api.example.com",
			"http://localhost:8080",
			"https://agents.mycompany.io",
		}).Draw(rt, "baseURL")

		// Create mock agent
		ag := &propMockAgent{
			id:          agentID,
			name:        agentName,
			agentType:   agentType,
			description: agentDesc,
		}

		// Generate agent card
		gen := NewAgentCardGenerator()
		card := gen.Generate(newAgentAdapter(ag), baseURL)

		// Property: Name must be non-empty
		assert.NotEmpty(t, card.Name, "AgentCard.Name should not be empty")

		// Property: Description must be non-empty
		assert.NotEmpty(t, card.Description, "AgentCard.Description should not be empty")

		// Property: URL must be non-empty
		assert.NotEmpty(t, card.URL, "AgentCard.URL should not be empty")

		// Property: Version must be non-empty
		assert.NotEmpty(t, card.Version, "AgentCard.Version should not be empty")

		// Property: Card should pass validation
		err := card.Validate()
		assert.NoError(t, err, "AgentCard should pass validation")
	})
}

// TestProperty_AgentCard_CapabilitiesReflectAgentType tests that capabilities reflect agent type.
func TestProperty_AgentCard_CapabilitiesReflectAgentType(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Map agent types to expected capabilities
		typeCapabilities := map[agent.AgentType]string{
			agent.TypeAssistant:  "chat",
			agent.TypeAnalyzer:   "analysis",
			agent.TypeTranslator: "translation",
			agent.TypeSummarizer: "summarization",
			agent.TypeReviewer:   "review",
		}

		agentType := rapid.SampledFrom([]agent.AgentType{
			agent.TypeAssistant,
			agent.TypeAnalyzer,
			agent.TypeTranslator,
			agent.TypeSummarizer,
			agent.TypeReviewer,
		}).Draw(rt, "agentType")

		ag := &propMockAgent{
			id:          "test-agent",
			name:        "Test Agent",
			agentType:   agentType,
			description: "A test agent for property testing",
		}

		gen := NewAgentCardGenerator()
		card := gen.Generate(newAgentAdapter(ag), "https://api.example.com")

		// Property: Capabilities list should not be empty
		assert.NotEmpty(t, card.Capabilities, "AgentCard.Capabilities should not be empty")

		// Property: Capabilities should reflect agent type
		expectedCap := typeCapabilities[agentType]
		assert.True(t, card.HasCapability(expectedCap),
			"AgentCard should have capability '%s' for agent type '%s'", expectedCap, agentType)
	})
}

// TestProperty_AgentCard_ToolsReflectAgentTools tests that tools list reflects agent's actual tools.
func TestProperty_AgentCard_ToolsReflectAgentTools(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		agentID := rapid.StringMatching(`[a-z][a-z0-9-]{2,20}`).Draw(rt, "agentID")

		// Generate random number of tools
		numTools := rapid.IntRange(0, 5).Draw(rt, "numTools")
		toolSchemas := make([]llm.ToolSchema, numTools)
		toolNames := make([]string, numTools)

		for i := 0; i < numTools; i++ {
			toolName := rapid.StringMatching(`[a-z][a-z_]{2,15}`).Draw(rt, fmt.Sprintf("toolName_%d", i))
			toolDesc := rapid.StringMatching(`[A-Za-z][a-zA-Z0-9 ]{5,50}`).Draw(rt, fmt.Sprintf("toolDesc_%d", i))

			toolNames[i] = toolName
			toolSchemas[i] = llm.ToolSchema{
				Name:        toolName,
				Description: toolDesc,
				Parameters:  json.RawMessage(`{"type":"object"}`),
			}
		}

		ag := &propMockAgent{
			id:          agentID,
			name:        "Tool Agent",
			agentType:   agent.TypeAssistant,
			description: "Agent with tools",
			tools:       toolNames,
		}

		toolProvider := &propToolProvider{
			tools: map[string][]llm.ToolSchema{
				agentID: toolSchemas,
			},
		}

		gen := NewAgentCardGenerator()
		card := gen.GenerateWithTools(newAgentAdapter(ag), "https://api.example.com", toolProvider)

		// Property: Tools count should match
		assert.Len(t, card.Tools, numTools, "AgentCard.Tools count should match agent tools")

		// Property: All tool names should be present
		for _, toolName := range toolNames {
			assert.True(t, card.HasTool(toolName),
				"AgentCard should have tool '%s'", toolName)
		}
	})
}

// TestProperty_AgentCard_MetadataPreserved tests that metadata is preserved in agent card.
func TestProperty_AgentCard_MetadataPreserved(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Generate random metadata
		numMeta := rapid.IntRange(1, 5).Draw(rt, "numMeta")
		metadata := make(map[string]string)

		for i := 0; i < numMeta; i++ {
			key := rapid.StringMatching(`[a-z][a-z_]{2,10}`).Draw(rt, fmt.Sprintf("metaKey_%d", i))
			value := rapid.StringMatching(`[a-zA-Z0-9]{3,20}`).Draw(rt, fmt.Sprintf("metaValue_%d", i))
			// Skip "version" key as it's handled specially
			if key != "version" {
				metadata[key] = value
			}
		}

		ag := &propMockAgent{
			id:          "meta-agent",
			name:        "Meta Agent",
			agentType:   agent.TypeAssistant,
			description: "Agent with metadata",
			metadata:    metadata,
		}

		gen := NewAgentCardGenerator()
		card := gen.Generate(newAgentAdapter(ag), "https://api.example.com")

		// Property: All metadata should be preserved (except version)
		for key, value := range metadata {
			if key != "version" {
				cardValue, ok := card.GetMetadata(key)
				assert.True(t, ok, "Metadata key '%s' should be present", key)
				assert.Equal(t, value, cardValue, "Metadata value for '%s' should match", key)
			}
		}

		// Property: agent_type and agent_id should be in metadata
		agentType, ok := card.GetMetadata("agent_type")
		assert.True(t, ok, "agent_type should be in metadata")
		assert.Equal(t, string(ag.agentType), agentType)

		agentIDMeta, ok := card.GetMetadata("agent_id")
		assert.True(t, ok, "agent_id should be in metadata")
		assert.Equal(t, ag.id, agentIDMeta)
	})
}

// TestProperty_AgentCard_VersionFromMetadata tests that version is taken from metadata if present.
func TestProperty_AgentCard_VersionFromMetadata(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		version := rapid.StringMatching(`[0-9]+\.[0-9]+\.[0-9]+`).Draw(rt, "version")

		ag := &propMockAgent{
			id:          "versioned-agent",
			name:        "Versioned Agent",
			agentType:   agent.TypeAssistant,
			description: "Agent with version",
			metadata: map[string]string{
				"version": version,
			},
		}

		gen := NewAgentCardGenerator()
		card := gen.Generate(newAgentAdapter(ag), "https://api.example.com")

		// Property: Version should come from metadata
		assert.Equal(t, version, card.Version, "Version should be taken from metadata")
	})
}

// TestProperty_AgentCard_URLFormat tests that URL is correctly formatted.
func TestProperty_AgentCard_URLFormat(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		agentID := rapid.StringMatching(`[a-z][a-z0-9-]{2,20}`).Draw(rt, "agentID")
		baseURL := rapid.SampledFrom([]string{
			"https://api.example.com",
			"https://api.example.com/",
			"http://localhost:8080",
			"http://localhost:8080/",
		}).Draw(rt, "baseURL")

		ag := &propMockAgent{
			id:          agentID,
			name:        "URL Agent",
			agentType:   agent.TypeAssistant,
			description: "Agent for URL testing",
		}

		gen := NewAgentCardGenerator()
		card := gen.Generate(newAgentAdapter(ag), baseURL)

		// Property: URL should contain agent ID
		assert.Contains(t, card.URL, agentID, "URL should contain agent ID")

		// Property: URL should not have double slashes (except protocol)
		urlWithoutProtocol := card.URL[8:] // Skip "https://" or "http://"
		assert.NotContains(t, urlWithoutProtocol, "//", "URL should not have double slashes")
	})
}

// TestProperty_AgentCard_RegisteredAgentHasValidCard tests that registered agents have valid cards.
func TestProperty_AgentCard_RegisteredAgentHasValidCard(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		agentID := rapid.StringMatching(`[a-z][a-z0-9-]{2,20}`).Draw(rt, "agentID")
		agentName := rapid.StringMatching(`[A-Z][a-zA-Z0-9 ]{2,30}`).Draw(rt, "agentName")
		agentDesc := rapid.StringMatching(`[A-Za-z][a-zA-Z0-9 ,.]{10,100}`).Draw(rt, "agentDesc")

		ag := &propMockAgent{
			id:          agentID,
			name:        agentName,
			agentType:   agent.TypeAssistant,
			description: agentDesc,
		}

		// Create server and register agent
		server := NewHTTPServer(&ServerConfig{
			BaseURL: "https://api.example.com",
		})

		err := server.RegisterAgent(ag)
		require.NoError(t, err, "Should register agent successfully")

		// Get agent card
		card, err := server.GetAgentCard(agentID)
		require.NoError(t, err, "Should get agent card")

		// Property: Card should have all required fields
		assert.NotEmpty(t, card.Name, "Name should not be empty")
		assert.NotEmpty(t, card.Description, "Description should not be empty")
		assert.NotEmpty(t, card.URL, "URL should not be empty")
		assert.NotEmpty(t, card.Version, "Version should not be empty")

		// Property: Card should pass validation
		err = card.Validate()
		assert.NoError(t, err, "Card should pass validation")

		// Property: Capabilities should not be empty
		assert.NotEmpty(t, card.Capabilities, "Capabilities should not be empty")
	})
}

// TestProperty_AgentCard_ToolDefinitionCompleteness tests that tool definitions are complete.
func TestProperty_AgentCard_ToolDefinitionCompleteness(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		toolName := rapid.StringMatching(`[a-z][a-z_]{2,15}`).Draw(rt, "toolName")
		toolDesc := rapid.StringMatching(`[A-Za-z][a-zA-Z0-9 ]{5,50}`).Draw(rt, "toolDesc")

		// Create tool schema with parameters
		params := &structured.JSONSchema{
			Type: structured.TypeObject,
			Properties: map[string]*structured.JSONSchema{
				"input": {
					Type:        structured.TypeString,
					Description: "Input parameter",
				},
			},
			Required: []string{"input"},
		}
		paramsJSON, _ := json.Marshal(params)

		toolSchema := llm.ToolSchema{
			Name:        toolName,
			Description: toolDesc,
			Parameters:  paramsJSON,
		}

		toolDef := convertToolSchema(toolSchema)

		// Property: Tool definition should have name
		assert.Equal(t, toolName, toolDef.Name, "Tool name should match")

		// Property: Tool definition should have description
		assert.Equal(t, toolDesc, toolDef.Description, "Tool description should match")

		// Property: Tool definition should have parameters
		assert.NotNil(t, toolDef.Parameters, "Tool parameters should not be nil")
	})
}
