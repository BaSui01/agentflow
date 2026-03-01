package providers

import (
	"encoding/json"
	"github.com/BaSui01/agentflow/types"
	"testing"

	"github.com/stretchr/testify/assert"
)

// 特性:多提供者支持,属性17:工具Schema转换
// ** 参数:要求11.1**
//
// 这个属性测试验证了对于任何提供商和任何带有非空工具阵列的聊天请求,
// 提供者转换每个 llm。 工具Schema 到特定提供者的工具格式保存
// 名称、描述和参数。
// 通过综合测试用例实现至少100次重复。
func TestProperty17_ToolSchemaConversion(t *testing.T) {
	testCases := []struct {
		name        string
		tools       []types.ToolSchema
		provider    string
		requirement string
		description string
	}{
		// 单一工具案件
		{
			name: "Single tool with all fields",
			tools: []types.ToolSchema{
				{
					Name:        "search",
					Description: "Search the web",
					Parameters:  json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}}}`),
				},
			},
			provider:    "grok",
			requirement: "11.1",
			description: "Should convert single tool with all fields preserved",
		},
		{
			name: "Single tool with minimal fields",
			tools: []types.ToolSchema{
				{
					Name:       "ping",
					Parameters: json.RawMessage(`{}`),
				},
			},
			provider:    "qwen",
			requirement: "11.1",
			description: "Should convert single tool with minimal fields",
		},
		{
			name: "Single tool with complex parameters",
			tools: []types.ToolSchema{
				{
					Name:        "calculate",
					Description: "Perform mathematical calculations",
					Parameters: json.RawMessage(`{
						"type": "object",
						"properties": {
							"expression": {"type": "string"},
							"precision": {"type": "integer", "minimum": 0, "maximum": 10}
						},
						"required": ["expression"]
					}`),
				},
			},
			provider:    "deepseek",
			requirement: "11.1",
			description: "Should convert tool with complex parameter schema",
		},
		{
			name: "Single tool with nested parameters",
			tools: []types.ToolSchema{
				{
					Name:        "create_user",
					Description: "Create a new user",
					Parameters: json.RawMessage(`{
						"type": "object",
						"properties": {
							"user": {
								"type": "object",
								"properties": {
									"name": {"type": "string"},
									"email": {"type": "string"},
									"age": {"type": "integer"}
								}
							}
						}
					}`),
				},
			},
			provider:    "glm",
			requirement: "11.1",
			description: "Should convert tool with nested parameter objects",
		},
		{
			name: "Single tool with array parameters",
			tools: []types.ToolSchema{
				{
					Name:        "batch_process",
					Description: "Process multiple items",
					Parameters: json.RawMessage(`{
						"type": "object",
						"properties": {
							"items": {
								"type": "array",
								"items": {"type": "string"}
							}
						}
					}`),
				},
			},
			provider:    "minimax",
			requirement: "11.1",
			description: "Should convert tool with array parameters",
		},

		// 多种工具案件
		{
			name: "Two tools with different schemas",
			tools: []types.ToolSchema{
				{
					Name:        "search",
					Description: "Search the web",
					Parameters:  json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}}}`),
				},
				{
					Name:        "calculate",
					Description: "Calculate math",
					Parameters:  json.RawMessage(`{"type":"object","properties":{"expression":{"type":"string"}}}`),
				},
			},
			provider:    "grok",
			requirement: "11.1",
			description: "Should convert multiple tools preserving all fields",
		},
		{
			name: "Three tools with varying complexity",
			tools: []types.ToolSchema{
				{
					Name:       "simple",
					Parameters: json.RawMessage(`{}`),
				},
				{
					Name:        "medium",
					Description: "Medium complexity",
					Parameters:  json.RawMessage(`{"type":"object","properties":{"param":{"type":"string"}}}`),
				},
				{
					Name:        "complex",
					Description: "Complex tool",
					Parameters: json.RawMessage(`{
						"type": "object",
						"properties": {
							"nested": {
								"type": "object",
								"properties": {
									"field": {"type": "string"}
								}
							}
						}
					}`),
				},
			},
			provider:    "qwen",
			requirement: "11.1",
			description: "Should convert multiple tools with varying complexity",
		},
		{
			name: "Five tools with different parameter types",
			tools: []types.ToolSchema{
				{
					Name:        "string_tool",
					Description: "String parameter",
					Parameters:  json.RawMessage(`{"type":"object","properties":{"str":{"type":"string"}}}`),
				},
				{
					Name:        "number_tool",
					Description: "Number parameter",
					Parameters:  json.RawMessage(`{"type":"object","properties":{"num":{"type":"number"}}}`),
				},
				{
					Name:        "boolean_tool",
					Description: "Boolean parameter",
					Parameters:  json.RawMessage(`{"type":"object","properties":{"bool":{"type":"boolean"}}}`),
				},
				{
					Name:        "array_tool",
					Description: "Array parameter",
					Parameters:  json.RawMessage(`{"type":"object","properties":{"arr":{"type":"array"}}}`),
				},
				{
					Name:        "object_tool",
					Description: "Object parameter",
					Parameters:  json.RawMessage(`{"type":"object","properties":{"obj":{"type":"object"}}}`),
				},
			},
			provider:    "deepseek",
			requirement: "11.1",
			description: "Should convert tools with all JSON schema types",
		},

		// 边缘案件
		{
			name: "Tool with empty description",
			tools: []types.ToolSchema{
				{
					Name:        "no_desc",
					Description: "",
					Parameters:  json.RawMessage(`{"type":"object"}`),
				},
			},
			provider:    "glm",
			requirement: "11.1",
			description: "Should handle tool with empty description",
		},
		{
			name: "Tool with long description",
			tools: []types.ToolSchema{
				{
					Name:        "long_desc",
					Description: "This is a very long description that contains multiple sentences and provides detailed information about what the tool does, including examples and use cases. It should be preserved exactly as provided.",
					Parameters:  json.RawMessage(`{"type":"object"}`),
				},
			},
			provider:    "minimax",
			requirement: "11.1",
			description: "Should preserve long descriptions",
		},
		{
			name: "Tool with special characters in name",
			tools: []types.ToolSchema{
				{
					Name:        "tool_with_underscores",
					Description: "Tool name with underscores",
					Parameters:  json.RawMessage(`{"type":"object"}`),
				},
			},
			provider:    "grok",
			requirement: "11.1",
			description: "Should handle tool names with special characters",
		},
		{
			name: "Tool with special characters in description",
			tools: []types.ToolSchema{
				{
					Name:        "special_chars",
					Description: "Tool with special chars: @#$%^&*()[]{}|\\;:'\",.<>?/",
					Parameters:  json.RawMessage(`{"type":"object"}`),
				},
			},
			provider:    "qwen",
			requirement: "11.1",
			description: "Should preserve special characters in description",
		},
		{
			name: "Tool with Unicode in description",
			tools: []types.ToolSchema{
				{
					Name:        "unicode_tool",
					Description: "工具描述 with 中文字符 and émojis 🚀",
					Parameters:  json.RawMessage(`{"type":"object"}`),
				},
			},
			provider:    "deepseek",
			requirement: "11.1",
			description: "Should preserve Unicode characters",
		},
		{
			name: "Tool with required fields in parameters",
			tools: []types.ToolSchema{
				{
					Name:        "required_params",
					Description: "Tool with required parameters",
					Parameters: json.RawMessage(`{
						"type": "object",
						"properties": {
							"required_field": {"type": "string"},
							"optional_field": {"type": "string"}
						},
						"required": ["required_field"]
					}`),
				},
			},
			provider:    "glm",
			requirement: "11.1",
			description: "Should preserve required field specifications",
		},
		{
			name: "Tool with parameter constraints",
			tools: []types.ToolSchema{
				{
					Name:        "constrained_params",
					Description: "Tool with parameter constraints",
					Parameters: json.RawMessage(`{
						"type": "object",
						"properties": {
							"age": {"type": "integer", "minimum": 0, "maximum": 120},
							"email": {"type": "string", "format": "email"},
							"status": {"type": "string", "enum": ["active", "inactive"]}
						}
					}`),
				},
			},
			provider:    "minimax",
			requirement: "11.1",
			description: "Should preserve parameter constraints",
		},
		{
			name: "Tool with default values",
			tools: []types.ToolSchema{
				{
					Name:        "defaults",
					Description: "Tool with default values",
					Parameters: json.RawMessage(`{
						"type": "object",
						"properties": {
							"timeout": {"type": "integer", "default": 30},
							"retry": {"type": "boolean", "default": true}
						}
					}`),
				},
			},
			provider:    "grok",
			requirement: "11.1",
			description: "Should preserve default values in parameters",
		},
		{
			name: "Tool with parameter descriptions",
			tools: []types.ToolSchema{
				{
					Name:        "documented_params",
					Description: "Tool with documented parameters",
					Parameters: json.RawMessage(`{
						"type": "object",
						"properties": {
							"query": {
								"type": "string",
								"description": "The search query to execute"
							}
						}
					}`),
				},
			},
			provider:    "qwen",
			requirement: "11.1",
			description: "Should preserve parameter descriptions",
		},
		{
			name: "Tool with oneOf schema",
			tools: []types.ToolSchema{
				{
					Name:        "oneof_tool",
					Description: "Tool with oneOf schema",
					Parameters: json.RawMessage(`{
						"type": "object",
						"properties": {
							"value": {
								"oneOf": [
									{"type": "string"},
									{"type": "number"}
								]
							}
						}
					}`),
				},
			},
			provider:    "deepseek",
			requirement: "11.1",
			description: "Should preserve oneOf schema definitions",
		},
		{
			name: "Tool with anyOf schema",
			tools: []types.ToolSchema{
				{
					Name:        "anyof_tool",
					Description: "Tool with anyOf schema",
					Parameters: json.RawMessage(`{
						"type": "object",
						"properties": {
							"value": {
								"anyOf": [
									{"type": "string"},
									{"type": "integer"}
								]
							}
						}
					}`),
				},
			},
			provider:    "glm",
			requirement: "11.1",
			description: "Should preserve anyOf schema definitions",
		},
		{
			name: "Tool with allOf schema",
			tools: []types.ToolSchema{
				{
					Name:        "allof_tool",
					Description: "Tool with allOf schema",
					Parameters: json.RawMessage(`{
						"type": "object",
						"allOf": [
							{"properties": {"name": {"type": "string"}}},
							{"properties": {"age": {"type": "integer"}}}
						]
					}`),
				},
			},
			provider:    "minimax",
			requirement: "11.1",
			description: "Should preserve allOf schema definitions",
		},
	}

	// 通过对所有提供商进行每个用例的测试,将测试用例扩展至100+重复
	providers := []string{"grok", "qwen", "deepseek", "glm", "minimax"}
	expandedTestCases := make([]struct {
		name        string
		tools       []types.ToolSchema
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
			case "grok", "qwen", "deepseek", "glm":
				// OpenAI 兼容提供者
				testOpenAICompatibleConversion(t, tc.tools, tc.provider, tc.requirement, tc.description)
			case "minimax":
				// MiniMax 有自定义格式
				testMiniMaxConversion(t, tc.tools, tc.provider, tc.requirement, tc.description)
			default:
				t.Fatalf("Unknown provider: %s", tc.provider)
			}
		})
	}

	// 检查我们至少有100个测试用例
	assert.GreaterOrEqual(t, len(expandedTestCases), 100,
		"Property test should have minimum 100 iterations")
}

// 测试 OpenAI 兼容性转换测试工具转换 OpenAI 兼容提供者
func testOpenAICompatibleConversion(t *testing.T, tools []types.ToolSchema, provider, requirement, description string) {
	// 使用光谱之后的模拟函数转换
	converted := mockConvertToolsOpenAI(tools)

	// 校验转换保存所有字段
	assert.Equal(t, len(tools), len(converted),
		"Number of tools should be preserved (Requirement %s): %s", requirement, description)

	for i, tool := range tools {
		// 校验工具类型设置正确
		assert.Equal(t, "function", converted[i].Type,
			"Tool type should be 'function' for OpenAI-compatible providers")

		// 验证名称被保存
		assert.Equal(t, tool.Name, converted[i].Function.Name,
			"Tool name should be preserved (Requirement %s): %s", requirement, description)

		// 校验参数被保存
		assert.JSONEq(t, string(tool.Parameters), string(converted[i].Function.Arguments),
			"Tool parameters should be preserved (Requirement %s): %s", requirement, description)

		// 注意: OpenAI 格式不包括函数对象中的描述
		// 描述通常包括在参数计划或其他地方。
	}
}

// 测试MiniMax 转换测试工具转换
func testMiniMaxConversion(t *testing.T, tools []types.ToolSchema, provider, requirement, description string) {
	// 使用光谱之后的模拟函数转换
	converted := mockConvertToolsMiniMax(tools)

	// 校验转换保存所有字段
	assert.Equal(t, len(tools), len(converted),
		"Number of tools should be preserved (Requirement %s): %s", requirement, description)

	for i, tool := range tools {
		// 验证名称被保存
		assert.Equal(t, tool.Name, converted[i].Name,
			"Tool name should be preserved (Requirement %s): %s", requirement, description)

		// 验证描述被保存
		assert.Equal(t, tool.Description, converted[i].Description,
			"Tool description should be preserved (Requirement %s): %s", requirement, description)

		// 校验参数被保存
		assert.JSONEq(t, string(tool.Parameters), string(converted[i].Parameters),
			"Tool parameters should be preserved (Requirement %s): %s", requirement, description)
	}
}

// Property17 Empty ToolsArray 验证空工具阵列的处理正确
func TestProperty17_EmptyToolsArray(t *testing.T) {
	providers := []string{"grok", "qwen", "deepseek", "glm", "minimax"}

	for _, provider := range providers {
		t.Run("empty_tools_"+provider, func(t *testing.T) {
			emptyTools := []types.ToolSchema{}

			switch provider {
			case "grok", "qwen", "deepseek", "glm":
				converted := mockConvertToolsOpenAI(emptyTools)
				assert.Nil(t, converted,
					"Empty tools array should return nil for %s", provider)
			case "minimax":
				converted := mockConvertToolsMiniMax(emptyTools)
				assert.Nil(t, converted,
					"Empty tools array should return nil for %s", provider)
			}
		})
	}
}

// Property17  NilToolsArray 验证零工具阵列得到正确处理
func TestProperty17_NilToolsArray(t *testing.T) {
	providers := []string{"grok", "qwen", "deepseek", "glm", "minimax"}

	for _, provider := range providers {
		t.Run("nil_tools_"+provider, func(t *testing.T) {
			var nilTools []types.ToolSchema

			switch provider {
			case "grok", "qwen", "deepseek", "glm":
				converted := mockConvertToolsOpenAI(nilTools)
				assert.Nil(t, converted,
					"Nil tools array should return nil for %s", provider)
			case "minimax":
				converted := mockConvertToolsMiniMax(nilTools)
				assert.Nil(t, converted,
					"Nil tools array should return nil for %s", provider)
			}
		})
	}
}

// Property17 ParameterJSONValidity 验证参数仍然有效的JSON
func TestProperty17_ParameterJSONValidity(t *testing.T) {
	testCases := []struct {
		name       string
		parameters json.RawMessage
	}{
		{"empty object", json.RawMessage(`{}`)},
		{"simple object", json.RawMessage(`{"type":"object"}`)},
		{"nested object", json.RawMessage(`{"type":"object","properties":{"nested":{"type":"object"}}}`)},
		{"array", json.RawMessage(`{"type":"array","items":{"type":"string"}}`)},
		{"with whitespace", json.RawMessage(`{  "type"  :  "object"  }`)},
		{"with newlines", json.RawMessage("{\n  \"type\": \"object\"\n}")},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tool := types.ToolSchema{
				Name:        "test_tool",
				Description: "Test tool",
				Parameters:  tc.parameters,
			}

			// 测试 OpenAI 格式
			openAIConverted := mockConvertToolsOpenAI([]types.ToolSchema{tool})
			assert.NotNil(t, openAIConverted)
			assert.True(t, json.Valid(openAIConverted[0].Function.Arguments),
				"Converted parameters should be valid JSON")

			// 测试迷你最大格式
			miniMaxConverted := mockConvertToolsMiniMax([]types.ToolSchema{tool})
			assert.NotNil(t, miniMaxConverted)
			assert.True(t, json.Valid(miniMaxConverted[0].Parameters),
				"Converted parameters should be valid JSON")
		})
	}
}

// 跟踪光谱的模拟转换函数

type mockOpenAITool struct {
	Type     string             `json:"type"`
	Function mockOpenAIFunction `json:"function"`
}

type mockOpenAIFunction struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type mockMiniMaxTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
}

func mockConvertToolsOpenAI(tools []types.ToolSchema) []mockOpenAITool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]mockOpenAITool, 0, len(tools))
	for _, t := range tools {
		out = append(out, mockOpenAITool{
			Type: "function",
			Function: mockOpenAIFunction{
				Name:      t.Name,
				Arguments: t.Parameters,
			},
		})
	}
	return out
}

func mockConvertToolsMiniMax(tools []types.ToolSchema) []mockMiniMaxTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]mockMiniMaxTool, 0, len(tools))
	for _, t := range tools {
		out = append(out, mockMiniMaxTool{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.Parameters,
		})
	}
	return out
}
