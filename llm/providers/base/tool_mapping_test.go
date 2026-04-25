package providerbase

import (
	"encoding/json"
	"testing"

	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestNormalizeToolChoiceModesAndFlags(t *testing.T) {
	disableParallel := false
	includeServerSide := true

	tests := []struct {
		name        string
		choice      any
		wantMode    string
		wantName    string
		wantAllowed []string
	}{
		{
			name:     "typed auto",
			choice:   types.ToolChoice{Mode: types.ToolChoiceModeAuto},
			wantMode: "auto",
		},
		{
			name:     "typed none",
			choice:   &types.ToolChoice{Mode: types.ToolChoiceModeNone},
			wantMode: "none",
		},
		{
			name:     "typed required",
			choice:   &types.ToolChoice{Mode: types.ToolChoiceModeRequired},
			wantMode: "any",
		},
		{
			name:     "typed specific",
			choice:   &types.ToolChoice{Mode: types.ToolChoiceModeSpecific, ToolName: " lookup_weather "},
			wantMode: "tool",
			wantName: "lookup_weather",
		},
		{
			name: "typed allowed dedupes names",
			choice: &types.ToolChoice{
				Mode:         types.ToolChoiceModeAllowed,
				AllowedTools: []string{" lookup_weather ", "lookup_weather", "search_docs", ""},
			},
			wantMode:    "any",
			wantAllowed: []string{"lookup_weather", "search_docs"},
		},
		{
			name:     "string specific",
			choice:   "lookup_weather",
			wantMode: "tool",
			wantName: "lookup_weather",
		},
		{
			name: "map allowed camel case",
			choice: map[string]any{
				"type":                             "any",
				"allowedFunctionNames":             []any{"lookup_weather", "lookup_weather", "search_docs"},
				"disable_parallel_tool_use":        disableParallel,
				"includeServerSideToolInvocations": includeServerSide,
			},
			wantMode:    "any",
			wantAllowed: []string{"lookup_weather", "search_docs"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := NormalizeToolChoice(tt.choice)
			assert.Equal(t, tt.wantMode, spec.Mode)
			assert.Equal(t, tt.wantName, spec.SpecificName)
			assert.Equal(t, tt.wantAllowed, spec.AllowedFunctionNames)

			if tt.name == "map allowed camel case" {
				require.NotNil(t, spec.DisableParallelToolUse)
				assert.False(t, *spec.DisableParallelToolUse)
				require.NotNil(t, spec.IncludeServerSideToolUse)
				assert.True(t, *spec.IncludeServerSideToolUse)
			}
		})
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

	customWriteback, ok := ToolOutputFromMessage(types.Message{
		Role:       types.RoleTool,
		ToolCallID: "b",
		Name:       "code_exec",
		Content:    "stdout text",
	}, index)
	require.True(t, ok)
	assert.Equal(t, types.ToolTypeCustom, customWriteback.ToolType)
	customOpenAIItem := BuildOpenAIResponsesToolOutputItem(customWriteback, nil)
	assert.Equal(t, "custom_tool_call_output", customOpenAIItem["type"])
	assert.Equal(t, "b", customOpenAIItem["call_id"])
	assert.Equal(t, "stdout text", customOpenAIItem["output"])

	_, ok = ToolOutputFromMessage(types.Message{
		Role:    types.RoleTool,
		Content: "missing id",
	}, index)
	assert.False(t, ok)
}

func TestToolCallDeltaAccumulator(t *testing.T) {
	t.Run("accumulates interleaved function deltas", func(t *testing.T) {
		acc := NewToolCallDeltaAccumulator()
		acc.Register("item_1", types.ToolTypeFunction, " lookup_weather ", "call_1")
		acc.Register("item_2", types.ToolTypeFunction, "search_docs", "")

		acc.Append("item_1", `{"city":`)
		acc.Append("item_2", `{"q":`)
		acc.Append("item_1", `"Hangzhou"}`)
		acc.Append("item_2", `"agentflow"}`)

		call, ok := acc.CompleteFunction("item_1")
		require.True(t, ok)
		assert.Equal(t, "call_1", call.ID)
		assert.Equal(t, types.ToolTypeFunction, call.Type)
		assert.Equal(t, "lookup_weather", call.Name)
		assert.JSONEq(t, `{"city":"Hangzhou"}`, string(call.Arguments))

		call, ok = acc.CompleteFunction("item_2")
		require.True(t, ok)
		assert.Equal(t, "item_2", call.ID)
		assert.Equal(t, "search_docs", call.Name)
		assert.JSONEq(t, `{"q":"agentflow"}`, string(call.Arguments))
	})

	t.Run("accumulates custom tool input", func(t *testing.T) {
		acc := NewToolCallDeltaAccumulator()
		acc.Register("custom_1", types.ToolTypeCustom, "code_exec", "call_custom")
		acc.Append("custom_1", "print")
		acc.Append("custom_1", "('hi')")

		call, ok := acc.CompleteCustom("custom_1")
		require.True(t, ok)
		assert.Equal(t, "call_custom", call.ID)
		assert.Equal(t, types.ToolTypeCustom, call.Type)
		assert.Equal(t, "code_exec", call.Name)
		assert.Equal(t, "print('hi')", call.Input)
	})

	t.Run("drops incomplete calls and clears completed payload", func(t *testing.T) {
		acc := NewToolCallDeltaAccumulator()
		acc.Register("missing_name", types.ToolTypeFunction, "", "call_missing")
		acc.Append("missing_name", `{"city":"Hangzhou"}`)

		_, ok := acc.CompleteFunction("missing_name")
		assert.False(t, ok)

		acc.Register("done", types.ToolTypeFunction, "lookup_weather", "call_done")
		acc.Append("done", `{"city":"Paris"}`)
		_, ok = acc.CompleteFunction("done")
		require.True(t, ok)
		_, ok = acc.CompleteFunction("done")
		assert.False(t, ok)
	})
}
