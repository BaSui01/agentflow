package a2a

import (
	"encoding/json"
	"testing"

	"github.com/BaSui01/agentflow/agent/structured"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAgentCard(t *testing.T) {
	card := NewAgentCard("test-agent", "A test agent", "http://localhost:8080", "1.0.0")

	assert.Equal(t, "test-agent", card.Name)
	assert.Equal(t, "A test agent", card.Description)
	assert.Equal(t, "http://localhost:8080", card.URL)
	assert.Equal(t, "1.0.0", card.Version)
	assert.NotNil(t, card.Capabilities)
	assert.NotNil(t, card.Tools)
	assert.NotNil(t, card.Metadata)
}

func TestAgentCard_AddCapability(t *testing.T) {
	card := NewAgentCard("test-agent", "A test agent", "http://localhost:8080", "1.0.0")

	card.AddCapability("search", "Search capability", CapabilityTypeQuery)
	card.AddCapability("execute", "Execute tasks", CapabilityTypeTask)

	assert.Len(t, card.Capabilities, 2)
	assert.Equal(t, "search", card.Capabilities[0].Name)
	assert.Equal(t, CapabilityTypeQuery, card.Capabilities[0].Type)
	assert.Equal(t, "execute", card.Capabilities[1].Name)
	assert.Equal(t, CapabilityTypeTask, card.Capabilities[1].Type)
}

func TestAgentCard_AddTool(t *testing.T) {
	card := NewAgentCard("test-agent", "A test agent", "http://localhost:8080", "1.0.0")

	params := structured.NewObjectSchema()
	params.AddProperty("query", structured.NewStringSchema())
	params.AddRequired("query")

	card.AddTool("search_tool", "A search tool", params)

	assert.Len(t, card.Tools, 1)
	assert.Equal(t, "search_tool", card.Tools[0].Name)
	assert.Equal(t, "A search tool", card.Tools[0].Description)
	assert.NotNil(t, card.Tools[0].Parameters)
}

func TestAgentCard_SetSchemas(t *testing.T) {
	card := NewAgentCard("test-agent", "A test agent", "http://localhost:8080", "1.0.0")

	inputSchema := structured.NewObjectSchema()
	inputSchema.AddProperty("input", structured.NewStringSchema())

	outputSchema := structured.NewObjectSchema()
	outputSchema.AddProperty("output", structured.NewStringSchema())

	card.SetInputSchema(inputSchema).SetOutputSchema(outputSchema)

	assert.NotNil(t, card.InputSchema)
	assert.NotNil(t, card.OutputSchema)
}

func TestAgentCard_Metadata(t *testing.T) {
	card := NewAgentCard("test-agent", "A test agent", "http://localhost:8080", "1.0.0")

	card.SetMetadata("author", "test")
	card.SetMetadata("license", "MIT")

	value, ok := card.GetMetadata("author")
	assert.True(t, ok)
	assert.Equal(t, "test", value)

	value, ok = card.GetMetadata("license")
	assert.True(t, ok)
	assert.Equal(t, "MIT", value)

	_, ok = card.GetMetadata("nonexistent")
	assert.False(t, ok)
}

func TestAgentCard_HasCapability(t *testing.T) {
	card := NewAgentCard("test-agent", "A test agent", "http://localhost:8080", "1.0.0")
	card.AddCapability("search", "Search capability", CapabilityTypeQuery)

	assert.True(t, card.HasCapability("search"))
	assert.False(t, card.HasCapability("nonexistent"))
}

func TestAgentCard_GetCapability(t *testing.T) {
	card := NewAgentCard("test-agent", "A test agent", "http://localhost:8080", "1.0.0")
	card.AddCapability("search", "Search capability", CapabilityTypeQuery)

	cap := card.GetCapability("search")
	require.NotNil(t, cap)
	assert.Equal(t, "search", cap.Name)
	assert.Equal(t, CapabilityTypeQuery, cap.Type)

	assert.Nil(t, card.GetCapability("nonexistent"))
}

func TestAgentCard_HasTool(t *testing.T) {
	card := NewAgentCard("test-agent", "A test agent", "http://localhost:8080", "1.0.0")
	card.AddTool("search_tool", "A search tool", nil)

	assert.True(t, card.HasTool("search_tool"))
	assert.False(t, card.HasTool("nonexistent"))
}

func TestAgentCard_GetTool(t *testing.T) {
	card := NewAgentCard("test-agent", "A test agent", "http://localhost:8080", "1.0.0")
	card.AddTool("search_tool", "A search tool", nil)

	tool := card.GetTool("search_tool")
	require.NotNil(t, tool)
	assert.Equal(t, "search_tool", tool.Name)

	assert.Nil(t, card.GetTool("nonexistent"))
}

func TestAgentCard_Validate(t *testing.T) {
	tests := []struct {
		name    string
		card    *AgentCard
		wantErr error
	}{
		{
			name:    "valid card",
			card:    NewAgentCard("test", "desc", "http://localhost", "1.0.0"),
			wantErr: nil,
		},
		{
			name:    "missing name",
			card:    &AgentCard{Description: "desc", URL: "http://localhost", Version: "1.0.0"},
			wantErr: ErrMissingName,
		},
		{
			name:    "missing description",
			card:    &AgentCard{Name: "test", URL: "http://localhost", Version: "1.0.0"},
			wantErr: ErrMissingDescription,
		},
		{
			name:    "missing url",
			card:    &AgentCard{Name: "test", Description: "desc", Version: "1.0.0"},
			wantErr: ErrMissingURL,
		},
		{
			name:    "missing version",
			card:    &AgentCard{Name: "test", Description: "desc", URL: "http://localhost"},
			wantErr: ErrMissingVersion,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.card.Validate()
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestAgentCard_JSONSerialization(t *testing.T) {
	card := NewAgentCard("test-agent", "A test agent", "http://localhost:8080", "1.0.0")
	card.AddCapability("search", "Search capability", CapabilityTypeQuery)
	card.AddTool("search_tool", "A search tool", structured.NewObjectSchema())
	card.SetMetadata("author", "test")

	// Serialize to JSON
	data, err := json.Marshal(card)
	require.NoError(t, err)

	// Deserialize from JSON
	var decoded AgentCard
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	// Verify fields
	assert.Equal(t, card.Name, decoded.Name)
	assert.Equal(t, card.Description, decoded.Description)
	assert.Equal(t, card.URL, decoded.URL)
	assert.Equal(t, card.Version, decoded.Version)
	assert.Len(t, decoded.Capabilities, 1)
	assert.Len(t, decoded.Tools, 1)
	assert.Equal(t, "test", decoded.Metadata["author"])
}

func TestCapabilityType_Constants(t *testing.T) {
	assert.Equal(t, CapabilityType("task"), CapabilityTypeTask)
	assert.Equal(t, CapabilityType("query"), CapabilityTypeQuery)
	assert.Equal(t, CapabilityType("stream"), CapabilityTypeStream)
}
