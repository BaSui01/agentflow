package providerbase

import (
	"encoding/json"
	"testing"

	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
)

func TestNormalizeToolChoice(t *testing.T) {
	spec := NormalizeToolChoice("required")
	assert.Equal(t, "any", spec.Mode)

	spec = NormalizeToolChoice(map[string]any{
		"type": "function",
		"function": map[string]any{
			"name": "lookup_weather",
		},
	})
	assert.Equal(t, "tool", spec.Mode)
	assert.Equal(t, "lookup_weather", spec.SpecificName)

	spec = NormalizeToolChoice(map[string]any{
		"mode":                                 "validated",
		"allowed_function_names":               []any{"a", "b", "a"},
		"include_server_side_tool_invocations": true,
	})
	assert.Equal(t, "validated", spec.Mode)
	assert.Equal(t, []string{"a", "b"}, spec.AllowedFunctionNames)
	if assert.NotNil(t, spec.IncludeServerSideToolUse) {
		assert.True(t, *spec.IncludeServerSideToolUse)
	}
}

func TestToolParametersSchemaMap(t *testing.T) {
	params := ToolParametersSchemaMap(json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}}}`))
	assert.Equal(t, "object", params["type"])
}

func TestBuildToolCalls(t *testing.T) {
	fn := NewFunctionToolCall("call_1", "search", json.RawMessage(`{"q":"hi"}`))
	assert.Equal(t, types.ToolTypeFunction, fn.Type)
	assert.Equal(t, "search", fn.Name)

	custom := NewCustomToolCall("call_2", "code_exec", "print('hi')")
	assert.Equal(t, types.ToolTypeCustom, custom.Type)
	assert.Equal(t, "code_exec", custom.Name)
	assert.Equal(t, "print('hi')", custom.Input)
}

func TestBuildToolCallTypeIndexAndToolOutputHelpers(t *testing.T) {
	index := BuildToolCallTypeIndex([]types.Message{{
		Role: types.RoleAssistant,
		ToolCalls: []types.ToolCall{
			{ID: "a", Type: "", Name: "search"},
			{ID: "b", Type: types.ToolTypeCustom, Name: "code_exec"},
		},
	}})
	assert.Equal(t, types.ToolTypeFunction, index["a"])
	assert.Equal(t, types.ToolTypeCustom, index["b"])

	resp := ToolOutputResponseMap(`{"ok":true}`)
	assert.Equal(t, true, resp["ok"])
	resp = ToolOutputResponseMap("plain text")
	assert.Equal(t, "plain text", resp["result"])

	raw := AppendToolJSONDelta(nil, `{"city":`)
	raw = AppendToolJSONDelta(raw, `"Paris"}`)
	assert.JSONEq(t, `{"city":"Paris"}`, string(raw))

	chunk := ToolCallChunk(NewFunctionToolCall("call_1", "search", json.RawMessage(`{"q":"x"}`)))
	assert.Len(t, chunk, 1)

	writeback, ok := ToolOutputFromMessage(types.Message{
		Role:        types.RoleTool,
		ToolCallID:  "a",
		Name:        "search",
		Content:     `{"ok":true}`,
		IsToolError: true,
	}, index)
	assert.True(t, ok)
	assert.Equal(t, types.ToolTypeFunction, writeback.ToolType)
	assert.Equal(t, "a", writeback.CallID)
	assert.Equal(t, `{"ok":true}`, writeback.Content)
	assert.True(t, writeback.IsError)

	openAIItem := BuildOpenAIResponsesToolOutputItem(writeback, func(id string) string { return "fc_" + id })
	assert.Equal(t, "function_call_output", openAIItem["type"])
	assert.Equal(t, "fc_a", openAIItem["call_id"])

	anthropicBlock := BuildAnthropicToolResultBlock(writeback)
	assert.Equal(t, "tool_result", anthropicBlock["type"])
	assert.Equal(t, "a", anthropicBlock["tool_use_id"])
	assert.Equal(t, true, anthropicBlock["is_error"])

	geminiResp := BuildGeminiFunctionResponse(writeback)
	assert.Equal(t, true, geminiResp["ok"])
}
