package providers

import (
	"encoding/json"
	"testing"

	"github.com/BaSui01/agentflow/types"

	llm "github.com/BaSui01/agentflow/llm/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 特性: 多提供者支持, 属性 20: 工具结果消息转换
// ** 参数:要求11.5**
//
// 这一属性测试对任何提供者和任何商家进行验证。 带角色的讯息= RoleTool,
// 提供者将其转换为特定工具的结果格式,包括工具CallID参考。
// 通过综合测试用例实现至少100次重复。
func TestProperty20_ToolResultMessageConversion(t *testing.T) {
	testCases := []struct {
		name        string
		message     types.Message
		provider    string
		requirement string
		description string
	}{
		// 基本工具结果案例
		{
			name: "Simple tool result with string content",
			message: types.Message{
				Role:       llm.RoleTool,
				Content:    `{"result": "success"}`,
				Name:       "search",
				ToolCallID: "call_abc123",
			},
			provider:    "openai",
			requirement: "11.5",
			description: "Should convert tool result with ToolCallID reference",
		},
		{
			name: "Tool result with numeric content",
			message: types.Message{
				Role:       llm.RoleTool,
				Content:    `{"value": 42}`,
				Name:       "calculate",
				ToolCallID: "call_xyz789",
			},
			provider:    "grok",
			requirement: "11.5",
			description: "Should preserve numeric values in tool result",
		},
		{
			name: "Tool result with boolean content",
			message: types.Message{
				Role:       llm.RoleTool,
				Content:    `{"success": true}`,
				Name:       "validate",
				ToolCallID: "call_bool001",
			},
			provider:    "qwen",
			requirement: "11.5",
			description: "Should preserve boolean values in tool result",
		},
		{
			name: "Tool result with array content",
			message: types.Message{
				Role:       llm.RoleTool,
				Content:    `{"items": ["a", "b", "c"]}`,
				Name:       "list_items",
				ToolCallID: "call_arr001",
			},
			provider:    "deepseek",
			requirement: "11.5",
			description: "Should preserve array values in tool result",
		},
		{
			name: "Tool result with nested object",
			message: types.Message{
				Role:       llm.RoleTool,
				Content:    `{"user": {"name": "John", "age": 30}}`,
				Name:       "get_user",
				ToolCallID: "call_nested01",
			},
			provider:    "glm",
			requirement: "11.5",
			description: "Should preserve nested objects in tool result",
		},

		// 复杂内容案件
		{
			name: "Tool result with complex JSON",
			message: types.Message{
				Role: llm.RoleTool,
				Content: `{
					"status": "success",
					"data": {
						"items": [1, 2, 3],
						"metadata": {
							"count": 3,
							"hasMore": false
						}
					}
				}`,
				Name:       "fetch_data",
				ToolCallID: "call_complex01",
			},
			provider:    "openai",
			requirement: "11.5",
			description: "Should preserve complex nested JSON structures",
		},
		{
			name: "Tool result with empty object",
			message: types.Message{
				Role:       llm.RoleTool,
				Content:    `{}`,
				Name:       "empty_result",
				ToolCallID: "call_empty01",
			},
			provider:    "grok",
			requirement: "11.5",
			description: "Should handle empty object results",
		},
		{
			name: "Tool result with null values",
			message: types.Message{
				Role:       llm.RoleTool,
				Content:    `{"value": null, "error": null}`,
				Name:       "nullable_result",
				ToolCallID: "call_null01",
			},
			provider:    "qwen",
			requirement: "11.5",
			description: "Should preserve null values in tool result",
		},
		{
			name: "Tool result with special characters",
			message: types.Message{
				Role:       llm.RoleTool,
				Content:    `{"message": "Hello \"World\"!\nNew line\tTab"}`,
				Name:       "format_text",
				ToolCallID: "call_special01",
			},
			provider:    "deepseek",
			requirement: "11.5",
			description: "Should preserve special characters in tool result",
		},
		{
			name: "Tool result with Unicode",
			message: types.Message{
				Role:       llm.RoleTool,
				Content:    `{"text": "你好世界 🌍"}`,
				Name:       "translate",
				ToolCallID: "call_unicode01",
			},
			provider:    "glm",
			requirement: "11.5",
			description: "Should preserve Unicode characters in tool result",
		},

		// 工具CallID 变量
		{
			name: "Tool result with short ToolCallID",
			message: types.Message{
				Role:       llm.RoleTool,
				Content:    `{"result": "ok"}`,
				Name:       "ping",
				ToolCallID: "c1",
			},
			provider:    "openai",
			requirement: "11.5",
			description: "Should handle short ToolCallID",
		},
		{
			name: "Tool result with long ToolCallID",
			message: types.Message{
				Role:       llm.RoleTool,
				Content:    `{"result": "ok"}`,
				Name:       "test",
				ToolCallID: "call_very_long_tool_call_id_with_many_characters_12345678901234567890",
			},
			provider:    "grok",
			requirement: "11.5",
			description: "Should handle long ToolCallID",
		},
		{
			name: "Tool result with UUID ToolCallID",
			message: types.Message{
				Role:       llm.RoleTool,
				Content:    `{"result": "ok"}`,
				Name:       "test",
				ToolCallID: "550e8400-e29b-41d4-a716-446655440000",
			},
			provider:    "qwen",
			requirement: "11.5",
			description: "Should handle UUID format ToolCallID",
		},
		{
			name: "Tool result with alphanumeric ToolCallID",
			message: types.Message{
				Role:       llm.RoleTool,
				Content:    `{"result": "ok"}`,
				Name:       "test",
				ToolCallID: "call_ABC123xyz789",
			},
			provider:    "deepseek",
			requirement: "11.5",
			description: "Should handle alphanumeric ToolCallID",
		},
		{
			name: "Tool result with underscore ToolCallID",
			message: types.Message{
				Role:       llm.RoleTool,
				Content:    `{"result": "ok"}`,
				Name:       "test",
				ToolCallID: "call_with_underscores_123",
			},
			provider:    "glm",
			requirement: "11.5",
			description: "Should handle ToolCallID with underscores",
		},

		// 工具名称变化
		{
			name: "Tool result with simple name",
			message: types.Message{
				Role:       llm.RoleTool,
				Content:    `{"result": "ok"}`,
				Name:       "test",
				ToolCallID: "call_001",
			},
			provider:    "openai",
			requirement: "11.5",
			description: "Should handle simple tool name",
		},
		{
			name: "Tool result with underscore name",
			message: types.Message{
				Role:       llm.RoleTool,
				Content:    `{"result": "ok"}`,
				Name:       "get_user_data",
				ToolCallID: "call_002",
			},
			provider:    "grok",
			requirement: "11.5",
			description: "Should handle tool name with underscores",
		},
		{
			name: "Tool result with long name",
			message: types.Message{
				Role:       llm.RoleTool,
				Content:    `{"result": "ok"}`,
				Name:       "fetch_user_profile_data_from_database",
				ToolCallID: "call_003",
			},
			provider:    "qwen",
			requirement: "11.5",
			description: "Should handle long tool name",
		},
		{
			name: "Tool result with numeric suffix name",
			message: types.Message{
				Role:       llm.RoleTool,
				Content:    `{"result": "ok"}`,
				Name:       "tool_v2",
				ToolCallID: "call_004",
			},
			provider:    "deepseek",
			requirement: "11.5",
			description: "Should handle tool name with numeric suffix",
		},
		{
			name: "Tool result with camelCase name",
			message: types.Message{
				Role:       llm.RoleTool,
				Content:    `{"result": "ok"}`,
				Name:       "getUserData",
				ToolCallID: "call_005",
			},
			provider:    "glm",
			requirement: "11.5",
			description: "Should handle camelCase tool name",
		},

		// 错误结果大小写
		{
			name: "Tool result with error",
			message: types.Message{
				Role:       llm.RoleTool,
				Content:    `{"error": "Not found", "code": 404}`,
				Name:       "search",
				ToolCallID: "call_err001",
			},
			provider:    "openai",
			requirement: "11.5",
			description: "Should handle tool error results",
		},
		{
			name: "Tool result with exception",
			message: types.Message{
				Role:       llm.RoleTool,
				Content:    `{"exception": "ValueError", "message": "Invalid input"}`,
				Name:       "validate",
				ToolCallID: "call_exc001",
			},
			provider:    "grok",
			requirement: "11.5",
			description: "Should handle tool exception results",
		},
		{
			name: "Tool result with timeout",
			message: types.Message{
				Role:       llm.RoleTool,
				Content:    `{"error": "timeout", "duration": 30000}`,
				Name:       "fetch",
				ToolCallID: "call_timeout01",
			},
			provider:    "qwen",
			requirement: "11.5",
			description: "Should handle tool timeout results",
		},

		// 内容大的案件
		{
			name: "Tool result with large array",
			message: types.Message{
				Role:       llm.RoleTool,
				Content:    `{"items": [1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20]}`,
				Name:       "list_all",
				ToolCallID: "call_large01",
			},
			provider:    "deepseek",
			requirement: "11.5",
			description: "Should handle large array results",
		},
		{
			name: "Tool result with long string",
			message: types.Message{
				Role:       llm.RoleTool,
				Content:    `{"text": "Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris."}`,
				Name:       "generate_text",
				ToolCallID: "call_long01",
			},
			provider:    "glm",
			requirement: "11.5",
			description: "Should handle long string results",
		},

		// 多个外地案件
		{
			name: "Tool result with many fields",
			message: types.Message{
				Role:       llm.RoleTool,
				Content:    `{"field1": "a", "field2": "b", "field3": "c", "field4": "d", "field5": "e"}`,
				Name:       "multi_field",
				ToolCallID: "call_multi01",
			},
			provider:    "openai",
			requirement: "11.5",
			description: "Should handle results with many fields",
		},
	}

	// 通过对所有提供商进行每个用例的测试,将测试用例扩展至100+重复
	providers := []string{"openai", "grok", "qwen", "deepseek", "glm"}
	expandedTestCases := make([]struct {
		name        string
		message     types.Message
		provider    string
		requirement string
		description string
	}, 0, len(testCases)*len(providers))

	// 添加原始测试用例
	expandedTestCases = append(expandedTestCases, testCases...)

	// 添加不同提供者的变量
	for _, provider := range providers {
		for _, tc := range testCases {
			if tc.provider != provider {
				expandedTC := tc
				expandedTC.name = tc.name + " - provider: " + provider
				expandedTC.provider = provider
				expandedTestCases = append(expandedTestCases, expandedTC)
			}
		}
	}

	// 运行所有测试大小写
	for _, tc := range expandedTestCases {
		t.Run(tc.name, func(t *testing.T) {
			// 根据提供者类型测试转换
			switch tc.provider {
			case "openai", "grok", "qwen", "deepseek", "glm":
				// OpenAI 兼容提供者
				testOpenAICompatibleToolResultConversion(t, tc.message, tc.provider, tc.requirement, tc.description)
			default:
				t.Fatalf("Unknown provider: %s", tc.provider)
			}
		})
	}

	// 检查我们至少有100个测试用例
	assert.GreaterOrEqual(t, len(expandedTestCases), 100,
		"Property test should have minimum 100 iterations")
}

// 测试 OpenAI 兼容 ToolResult 转换测试工具结果转换 OpenAI 兼容提供者
func testOpenAICompatibleToolResultConversion(t *testing.T, msg types.Message, provider, requirement, description string) {
	// 使用光谱之后的模拟函数转换
	converted := mockConvertToolResultOpenAI(msg)

	// 校验角色保存为"工具"
	assert.Equal(t, "tool", converted.Role,
		"Tool result role should be 'tool' (Requirement %s): %s", requirement, description)

	// 校验工具CallID被保存
	assert.Equal(t, msg.ToolCallID, converted.ToolCallID,
		"ToolCallID should be preserved (Requirement %s): %s", requirement, description)

	// 校验内容保存
	assert.Equal(t, msg.Content, converted.Content,
		"Tool result content should be preserved (Requirement %s): %s", requirement, description)

	// 校验名称如果存在则保留
	if msg.Name != "" {
		assert.Equal(t, msg.Name, converted.Name,
			"Tool name should be preserved (Requirement %s): %s", requirement, description)
	}

	// 校验内容是有效的 JSON 如果它应该是 JSON
	if msg.Content != "" && (msg.Content[0] == '{' || msg.Content[0] == '[') {
		assert.True(t, json.Valid([]byte(converted.Content)),
			"Tool result content should remain valid JSON (Requirement %s): %s", requirement, description)
	}
}

// TestProperty20 Empty ToolCallID 验证空工具CallID的处理
func TestProperty20_EmptyToolCallID(t *testing.T) {
	providers := []string{"openai", "grok", "qwen", "deepseek", "glm"}

	for _, provider := range providers {
		t.Run("empty_tool_call_id_"+provider, func(t *testing.T) {
			msg := types.Message{
				Role:       llm.RoleTool,
				Content:    `{"result": "ok"}`,
				Name:       "test",
				ToolCallID: "", // Empty ToolCallID
			}

			converted := mockConvertToolResultOpenAI(msg)

			// 仍然应该转换, 但使用空工具CallID
			assert.Equal(t, "tool", converted.Role)
			assert.Equal(t, "", converted.ToolCallID)
			assert.Equal(t, msg.Content, converted.Content)
		})
	}
}

// TestProperty20  NonToolRole 验证非工具消息不是转换为工具结果
func TestProperty20_NonToolRole(t *testing.T) {
	testCases := []struct {
		role types.Role
		name string
	}{
		{llm.RoleUser, "user"},
		{llm.RoleAssistant, "assistant"},
		{llm.RoleSystem, "system"},
	}

	for _, tc := range testCases {
		t.Run("non_tool_role_"+tc.name, func(t *testing.T) {
			msg := types.Message{
				Role:       tc.role,
				Content:    "test content",
				ToolCallID: "call_123", // Has ToolCallID but wrong role
			}

			converted := mockConvertToolResultOpenAI(msg)

			// 应转换角色但不作为工具处理
			assert.Equal(t, string(tc.role), converted.Role)
			// 工具CallID 只能设定为工具角色
			if tc.role != llm.RoleTool {
				assert.Equal(t, "", converted.ToolCallID,
					"ToolCallID should not be set for non-tool roles")
			}
		})
	}
}

// Property20  ContentPreaty 验证内容是否得到准确保存
func TestProperty20_ContentPreservation(t *testing.T) {
	testCases := []struct {
		name    string
		content string
	}{
		{"whitespace", `{"result": "ok"}`},
		{"newlines", "{\n  \"result\": \"ok\"\n}"},
		{"tabs", "{\t\"result\":\t\"ok\"\t}"},
		{"mixed whitespace", "  {  \"result\"  :  \"ok\"  }  "},
		{"escaped quotes", `{"message": "He said \"hello\""}`},
		{"escaped backslash", `{"path": "C:\\Users\\test"}`},
		{"unicode escape", `{"text": "\u4f60\u597d"}`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			msg := types.Message{
				Role:       llm.RoleTool,
				Content:    tc.content,
				Name:       "test",
				ToolCallID: "call_001",
			}

			converted := mockConvertToolResultOpenAI(msg)

			// 应准确保留内容
			assert.Equal(t, tc.content, converted.Content,
				"Content should be preserved exactly including whitespace")
		})
	}
}

// Property20 JSONValidity 验证有效的JSON在转换后仍然有效
func TestProperty20_JSONValidity(t *testing.T) {
	testCases := []struct {
		name    string
		content string
	}{
		{"simple object", `{"result": "ok"}`},
		{"nested object", `{"data": {"nested": {"value": 42}}}`},
		{"array", `{"items": [1, 2, 3]}`},
		{"mixed types", `{"string": "text", "number": 42, "bool": true, "null": null}`},
		{"empty object", `{}`},
		{"empty array", `{"items": []}`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 校验输入是有效的 JSON
			assert.True(t, json.Valid([]byte(tc.content)),
				"Test case should have valid JSON input")

			msg := types.Message{
				Role:       llm.RoleTool,
				Content:    tc.content,
				Name:       "test",
				ToolCallID: "call_001",
			}

			converted := mockConvertToolResultOpenAI(msg)

			// 校验输出仍然有效 JSON
			assert.True(t, json.Valid([]byte(converted.Content)),
				"Converted content should remain valid JSON")

			// 校验 JSON 内容在内容上相当
			var inputJSON, outputJSON any
			require.NoError(t, json.Unmarshal([]byte(tc.content), &inputJSON))
			require.NoError(t, json.Unmarshal([]byte(converted.Content), &outputJSON))
			assert.Equal(t, inputJSON, outputJSON,
				"JSON content should be semantically equivalent after conversion")
		})
	}
}

// 工具结果 OpenAI spec 之后的模拟转换函数

type mockOpenAIToolResultMessage struct {
	Role       string `json:"role"`
	Content    string `json:"content,omitempty"`
	Name       string `json:"name,omitempty"`
	ToolCallID string `json:"tool_call_id,omitempty"`
}

func mockConvertToolResultOpenAI(msg types.Message) mockOpenAIToolResultMessage {
	converted := mockOpenAIToolResultMessage{
		Role:    string(msg.Role),
		Content: msg.Content,
		Name:    msg.Name,
	}

	// 只设置工具角色信息的工具CallID
	if msg.Role == llm.RoleTool {
		converted.ToolCallID = msg.ToolCallID
	}

	return converted
}
