package mcp

import (
	"encoding/json"
	"testing"

	"github.com/BaSui01/agentflow/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFromLLMToolSchema_ValidParameters verifies that valid JSON parameters
// are correctly deserialized into a ToolDefinition.
func TestFromLLMToolSchema_ValidParameters(t *testing.T) {
	schema := llm.ToolSchema{
		Name:        "test_tool",
		Description: "A test tool",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"}}}`),
	}

	td, err := FromLLMToolSchema(schema)
	require.NoError(t, err)
	assert.Equal(t, "test_tool", td.Name)
	assert.Equal(t, "A test tool", td.Description)
	assert.NotNil(t, td.InputSchema)
	assert.Equal(t, "object", td.InputSchema["type"])
}

// TestFromLLMToolSchema_InvalidJSON verifies that invalid JSON in Parameters
// returns an error instead of silently ignoring it.
func TestFromLLMToolSchema_InvalidJSON(t *testing.T) {
	schema := llm.ToolSchema{
		Name:        "bad_tool",
		Description: "Tool with bad params",
		Parameters:  json.RawMessage(`{not valid json}`),
	}

	_, err := FromLLMToolSchema(schema)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal tool parameters")
	assert.Contains(t, err.Error(), "bad_tool")
}

// TestFromLLMToolSchema_EmptyParameters verifies that empty/nil Parameters
// produce a ToolDefinition with nil InputSchema and no error.
func TestFromLLMToolSchema_EmptyParameters(t *testing.T) {
	schema := llm.ToolSchema{
		Name:        "empty_tool",
		Description: "Tool with no params",
		Parameters:  nil,
	}

	td, err := FromLLMToolSchema(schema)
	require.NoError(t, err)
	assert.Equal(t, "empty_tool", td.Name)
	assert.Nil(t, td.InputSchema)
}

// TestFromLLMToolSchema_Roundtrip verifies that ToLLMToolSchema and
// FromLLMToolSchema are inverse operations for well-formed data.
func TestFromLLMToolSchema_Roundtrip(t *testing.T) {
	original := ToolDefinition{
		Name:        "roundtrip_tool",
		Description: "Roundtrip test",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"count": map[string]any{"type": "number"},
			},
		},
	}

	llmSchema := original.ToLLMToolSchema()
	restored, err := FromLLMToolSchema(llmSchema)
	require.NoError(t, err)

	assert.Equal(t, original.Name, restored.Name)
	assert.Equal(t, original.Description, restored.Description)
	assert.Equal(t, "object", restored.InputSchema["type"])
}
