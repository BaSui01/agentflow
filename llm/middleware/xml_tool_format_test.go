package middleware

import (
	"encoding/json"
	"testing"

	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatToolsAsXML_Empty(t *testing.T) {
	result := FormatToolsAsXML(nil)
	assert.Empty(t, result)

	result = FormatToolsAsXML([]types.ToolSchema{})
	assert.Empty(t, result)
}

func TestFormatToolsAsXML_SingleTool(t *testing.T) {
	tools := []types.ToolSchema{
		{
			Name:        "get_weather",
			Description: "Get the current weather for a city",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}`),
		},
	}

	result := FormatToolsAsXML(tools)

	assert.Contains(t, result, "get_weather")
	assert.Contains(t, result, "Get the current weather for a city")
	assert.Contains(t, result, `"city"`)
	assert.Contains(t, result, "<tool_calls>")
	assert.Contains(t, result, "</tool_calls>")
	assert.Contains(t, result, "# Available Tools")
}

func TestFormatToolsAsXML_MultipleTools(t *testing.T) {
	tools := []types.ToolSchema{
		{
			Name:        "search",
			Description: "Search the web",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}}}`),
		},
		{
			Name:        "calculator",
			Description: "Perform calculations",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"expression":{"type":"string"}}}`),
		},
	}

	result := FormatToolsAsXML(tools)

	assert.Contains(t, result, "Tool 1: search")
	assert.Contains(t, result, "Tool 2: calculator")
	assert.Contains(t, result, "Search the web")
	assert.Contains(t, result, "Perform calculations")
}

func TestFormatToolsAsXML_NoDescription(t *testing.T) {
	tools := []types.ToolSchema{
		{
			Name:       "noop",
			Parameters: json.RawMessage(`{}`),
		},
	}

	result := FormatToolsAsXML(tools)
	assert.Contains(t, result, "noop")
	assert.NotContains(t, result, "Description:")
}

func TestFormatToolsAsXML_InvalidJSON(t *testing.T) {
	// 即使 JSON 无效也不应 panic，应原样输出
	tools := []types.ToolSchema{
		{
			Name:       "broken",
			Parameters: json.RawMessage(`not-valid-json`),
		},
	}

	require.NotPanics(t, func() {
		result := FormatToolsAsXML(tools)
		assert.Contains(t, result, "broken")
	})
}
