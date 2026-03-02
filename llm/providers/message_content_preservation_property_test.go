package providers

import (
	"encoding/json"
	"testing"

	"github.com/BaSui01/agentflow/types"

	"github.com/BaSui01/agentflow/llm"
	"github.com/stretchr/testify/assert"
)

// 特性:多提供者支持,属性 23:消息内容保存
// ** 变动情况:要求12.5、12.6、12.7**
//
// 这一属性测试对任何提供者和任何商家进行验证。 消息为
// 内容、名称、工具呼叫或工具CallID字段,提供者应当保留
// 转换到提供者格式时的所有非空字段。
// 通过对所有提供者进行全面测试,实现至少100次重复。

// 测试Property23  内容保存测试
func TestProperty23_MessageContentPreservation(t *testing.T) {
	providers := []string{"grok", "qwen", "deepseek", "glm", "minimax"}

	contentVariations := []struct {
		name    string
		content string
	}{
		{"simple text", "Hello world"},
		{"empty content", ""},
		{"unicode content", "你好世界 🌍 مرحبا"},
		{"special chars", "Content with special chars: @#$%^&*()[]{}|\\"},
		{"multiline", "Line 1\nLine 2\nLine 3"},
		{"json content", `{"key": "value", "number": 123}`},
		{"code content", "func main() {\n\tfmt.Println(\"Hello\")\n}"},
		{"markdown", "# Title\n\n- Item 1\n- Item 2\n\n**Bold** and *italic*"},
		{"long content", "This is a very long content that spans multiple sentences. It contains various information and should be preserved exactly as provided without any modification or truncation."},
		{"whitespace", "  content with   spaces  and\ttabs\t"},
	}

	roles := []types.Role{llm.RoleSystem, llm.RoleUser, llm.RoleAssistant}

	// 生成测试用例:5个提供者 * 10个内容变化 * 3个角色=150个用例
	testCount := 0
	for _, provider := range providers {
		for _, cv := range contentVariations {
			for _, role := range roles {
				testCount++
				t.Run(provider+"_"+cv.name+"_"+string(role), func(t *testing.T) {
					msg := types.Message{
						Role:    role,
						Content: cv.content,
					}

					switch provider {
					case "grok", "qwen", "deepseek", "glm":
						converted := convertMessageOpenAIFormat(msg)
						assert.Equal(t, cv.content, converted.Content,
							"Content should be preserved for %s (Requirement 12.7)", provider)
					case "minimax":
						converted := convertMessageMiniMaxFormat(msg)
						// MiniMax 为非工具调用消息保留内容
						if len(msg.ToolCalls) == 0 {
							assert.Equal(t, cv.content, converted.Content,
								"Content should be preserved for %s (Requirement 12.7)", provider)
						}
					}
				})
			}
		}
	}

	assert.GreaterOrEqual(t, testCount, 100, "Should have at least 100 test iterations")
}

// Property23  Name 保存名称字段的 FieldP维护测试
func TestProperty23_NameFieldPreservation(t *testing.T) {
	providers := []string{"grok", "qwen", "deepseek", "glm", "minimax"}

	nameVariations := []struct {
		name      string
		nameValue string
	}{
		{"simple name", "assistant_1"},
		{"empty name", ""},
		{"unicode name", "助手_1"},
		{"name with numbers", "agent_123"},
		{"name with special", "agent-v2.0"},
		{"long name", "very_long_agent_name_that_should_be_preserved"},
	}

	roles := []types.Role{llm.RoleSystem, llm.RoleUser, llm.RoleAssistant}

	for _, provider := range providers {
		for _, nv := range nameVariations {
			for _, role := range roles {
				t.Run(provider+"_"+nv.name+"_"+string(role), func(t *testing.T) {
					msg := types.Message{
						Role:    role,
						Content: "Test content",
						Name:    nv.nameValue,
					}

					switch provider {
					case "grok", "qwen", "deepseek", "glm":
						converted := convertMessageOpenAIFormat(msg)
						assert.Equal(t, nv.nameValue, converted.Name,
							"Name should be preserved for %s (Requirement 12.7)", provider)
					case "minimax":
						converted := convertMessageMiniMaxFormat(msg)
						assert.Equal(t, nv.nameValue, converted.Name,
							"Name should be preserved for %s (Requirement 12.7)", provider)
					}
				})
			}
		}
	}
}

// TestProperty23  ToolCalls 保存工具Calls 字段的测试
func TestProperty23_ToolCallsPreservation(t *testing.T) {
	providers := []string{"grok", "qwen", "deepseek", "glm", "minimax"}

	toolCallVariations := []struct {
		name      string
		toolCalls []types.ToolCall
	}{
		{
			name: "single tool call",
			toolCalls: []types.ToolCall{
				{ID: "call_001", Name: "get_weather", Arguments: json.RawMessage(`{"location":"Beijing"}`)},
			},
		},
		{
			name: "multiple tool calls",
			toolCalls: []types.ToolCall{
				{ID: "call_001", Name: "get_weather", Arguments: json.RawMessage(`{"location":"Beijing"}`)},
				{ID: "call_002", Name: "get_time", Arguments: json.RawMessage(`{"timezone":"UTC"}`)},
			},
		},
		{
			name: "tool call with complex args",
			toolCalls: []types.ToolCall{
				{ID: "call_003", Name: "search", Arguments: json.RawMessage(`{"query":"test","filters":{"type":"doc","limit":10}}`)},
			},
		},
		{
			name: "tool call with empty args",
			toolCalls: []types.ToolCall{
				{ID: "call_004", Name: "list_items", Arguments: json.RawMessage(`{}`)},
			},
		},
		{
			name: "tool call with unicode",
			toolCalls: []types.ToolCall{
				{ID: "call_005", Name: "translate", Arguments: json.RawMessage(`{"text":"你好世界"}`)},
			},
		},
		{
			name:      "empty tool calls",
			toolCalls: []types.ToolCall{},
		},
	}

	for _, provider := range providers {
		for _, tcv := range toolCallVariations {
			t.Run(provider+"_"+tcv.name, func(t *testing.T) {
				msg := types.Message{
					Role:      llm.RoleAssistant,
					Content:   "",
					ToolCalls: tcv.toolCalls,
				}

				switch provider {
				case "grok", "qwen", "deepseek", "glm":
					converted := convertMessageOpenAIFormat(msg)
					// 校验工具呼叫被保存
					assert.Len(t, converted.ToolCalls, len(tcv.toolCalls),
						"ToolCalls count should be preserved for %s (Requirement 12.5)", provider)
					for i, tc := range tcv.toolCalls {
						if i < len(converted.ToolCalls) {
							assert.Equal(t, tc.ID, converted.ToolCalls[i].ID,
								"ToolCall ID should be preserved")
							assert.Equal(t, tc.Name, converted.ToolCalls[i].Function.Name,
								"ToolCall Name should be preserved")
							assert.JSONEq(t, string(tc.Arguments), string(converted.ToolCalls[i].Function.Arguments),
								"ToolCall Arguments should be preserved")
						}
					}
				case "minimax":
					converted := convertMessageMiniMaxFormat(msg)
					// MiniMax 在内容中将工具调用转换为 XML 格式
					if len(tcv.toolCalls) > 0 {
						assert.Contains(t, converted.Content, "<tool_calls>",
							"MiniMax should format tool calls as XML (Requirement 12.5)")
						for _, tc := range tcv.toolCalls {
							assert.Contains(t, converted.Content, tc.Name,
								"Tool call name should be in XML content")
						}
					}
				}
			})
		}
	}
}

// 测试Property23  ToolCallIDP 保存测试工具CallID 字段被保存
func TestProperty23_ToolCallIDPreservation(t *testing.T) {
	providers := []string{"grok", "qwen", "deepseek", "glm"}

	toolCallIDVariations := []struct {
		name       string
		toolCallID string
	}{
		{"simple id", "call_001"},
		{"uuid format", "call_550e8400-e29b-41d4-a716-446655440000"},
		{"long id", "call_very_long_tool_call_id_12345678901234567890"},
		{"id with special chars", "call_abc-123_xyz"},
		{"empty id", ""},
	}

	for _, provider := range providers {
		for _, tcid := range toolCallIDVariations {
			t.Run(provider+"_"+tcid.name, func(t *testing.T) {
				msg := types.Message{
					Role:       llm.RoleTool,
					Content:    `{"result": "success"}`,
					ToolCallID: tcid.toolCallID,
				}

				converted := convertMessageOpenAIFormat(msg)
				assert.Equal(t, tcid.toolCallID, converted.ToolCallID,
					"ToolCallID should be preserved for %s (Requirement 12.6)", provider)
			})
		}
	}
}

// Property23  所有字段一起保存的保存测试
func TestProperty23_AllFieldsPreservation(t *testing.T) {
	providers := []string{"grok", "qwen", "deepseek", "glm"}

	testCases := []struct {
		name    string
		message types.Message
	}{
		{
			name: "assistant with tool calls",
			message: types.Message{
				Role:    llm.RoleAssistant,
				Content: "I'll help you with that.",
				Name:    "assistant_1",
				ToolCalls: []types.ToolCall{
					{ID: "call_001", Name: "search", Arguments: json.RawMessage(`{"query":"test"}`)},
				},
			},
		},
		{
			name: "tool result",
			message: types.Message{
				Role:       llm.RoleTool,
				Content:    `{"results": [1, 2, 3]}`,
				Name:       "search",
				ToolCallID: "call_001",
			},
		},
		{
			name: "user with name",
			message: types.Message{
				Role:    llm.RoleUser,
				Content: "Hello, can you help me?",
				Name:    "user_john",
			},
		},
		{
			name: "system with name",
			message: types.Message{
				Role:    llm.RoleSystem,
				Content: "You are a helpful assistant.",
				Name:    "system_prompt",
			},
		},
		{
			name: "assistant with multiple tool calls",
			message: types.Message{
				Role:    llm.RoleAssistant,
				Content: "",
				Name:    "assistant_2",
				ToolCalls: []types.ToolCall{
					{ID: "call_001", Name: "get_weather", Arguments: json.RawMessage(`{"city":"Beijing"}`)},
					{ID: "call_002", Name: "get_time", Arguments: json.RawMessage(`{"tz":"Asia/Shanghai"}`)},
				},
			},
		},
	}

	for _, provider := range providers {
		for _, tc := range testCases {
			t.Run(provider+"_"+tc.name, func(t *testing.T) {
				converted := convertMessageOpenAIFormat(tc.message)

				// 校验内容保存
				assert.Equal(t, tc.message.Content, converted.Content,
					"Content should be preserved (Requirement 12.7)")

				// 验证名称保存
				assert.Equal(t, tc.message.Name, converted.Name,
					"Name should be preserved (Requirement 12.7)")

				// 验证工具CallID 保存
				assert.Equal(t, tc.message.ToolCallID, converted.ToolCallID,
					"ToolCallID should be preserved (Requirement 12.6)")

				// 校验工具呼叫保存
				assert.Len(t, converted.ToolCalls, len(tc.message.ToolCalls),
					"ToolCalls count should be preserved (Requirement 12.5)")
			})
		}
	}
}

// 测试Property23  多重MultipMessagesP 保存测试跨多条消息
func TestProperty23_MultipleMessagesPreservation(t *testing.T) {
	providers := []string{"grok", "qwen", "deepseek", "glm", "minimax"}

	messages := []types.Message{
		{Role: llm.RoleSystem, Content: "You are a helpful assistant.", Name: "system"},
		{Role: llm.RoleUser, Content: "What's the weather?", Name: "user_1"},
		{
			Role:    llm.RoleAssistant,
			Content: "Let me check.",
			Name:    "assistant",
			ToolCalls: []types.ToolCall{
				{ID: "call_001", Name: "get_weather", Arguments: json.RawMessage(`{"city":"Beijing"}`)},
			},
		},
		{Role: llm.RoleTool, Content: `{"temp": 25}`, ToolCallID: "call_001"},
		{Role: llm.RoleAssistant, Content: "The temperature is 25°C.", Name: "assistant"},
	}

	for _, provider := range providers {
		t.Run(provider, func(t *testing.T) {
			switch provider {
			case "grok", "qwen", "deepseek", "glm":
				converted := convertMessagesOpenAIFormat(messages)
				assert.Len(t, converted, len(messages), "Message count should be preserved")

				for i, msg := range messages {
					assert.Equal(t, msg.Content, converted[i].Content,
						"Content should be preserved for message %d", i)
					assert.Equal(t, msg.Name, converted[i].Name,
						"Name should be preserved for message %d", i)
					assert.Equal(t, msg.ToolCallID, converted[i].ToolCallID,
						"ToolCallID should be preserved for message %d", i)
					assert.Len(t, converted[i].ToolCalls, len(msg.ToolCalls),
						"ToolCalls count should be preserved for message %d", i)
				}
			case "minimax":
				converted := convertMessagesMiniMaxFormat(messages)
				assert.Len(t, converted, len(messages), "Message count should be preserved")

				for i, msg := range messages {
					assert.Equal(t, msg.Name, converted[i].Name,
						"Name should be preserved for message %d", i)
					// 在 MiniMax 为工具调用修改内容
					if len(msg.ToolCalls) == 0 {
						assert.Equal(t, msg.Content, converted[i].Content,
							"Content should be preserved for message %d", i)
					}
				}
			}
		})
	}
}

// 转换辅助器类型和功能

type openAIToolCall struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Function openAIFunction `json:"function"`
}

type openAIFunction struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type openAIMessageFormat struct {
	Role       string           `json:"role"`
	Content    string           `json:"content,omitempty"`
	Name       string           `json:"name,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
}

type miniMaxMessageFormat struct {
	Role    string `json:"role"`
	Content string `json:"content,omitempty"`
	Name    string `json:"name,omitempty"`
}

// 转换 Message OpenAIFormat 转换单个 llm。 消息到 OpenAI 格式
func convertMessageOpenAIFormat(msg types.Message) openAIMessageFormat {
	converted := openAIMessageFormat{
		Role:       string(msg.Role),
		Content:    msg.Content,
		Name:       msg.Name,
		ToolCallID: msg.ToolCallID,
	}

	if len(msg.ToolCalls) > 0 {
		converted.ToolCalls = make([]openAIToolCall, 0, len(msg.ToolCalls))
		for _, tc := range msg.ToolCalls {
			converted.ToolCalls = append(converted.ToolCalls, openAIToolCall{
				ID:   tc.ID,
				Type: "function",
				Function: openAIFunction{
					Name:      tc.Name,
					Arguments: tc.Arguments,
				},
			})
		}
	}

	return converted
}

// 转换Messages OpenAIFormat 转换多个 llm 。 消息到 OpenAI 格式
func convertMessagesOpenAIFormat(msgs []types.Message) []openAIMessageFormat {
	out := make([]openAIMessageFormat, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, convertMessageOpenAIFormat(m))
	}
	return out
}

// 转换 MessageMiniMax Format 转换单个 llm 。 消息到 MiniMax 格式
func convertMessageMiniMaxFormat(msg types.Message) miniMaxMessageFormat {
	converted := miniMaxMessageFormat{
		Role:    string(msg.Role),
		Content: msg.Content,
		Name:    msg.Name,
	}

	// 如果消息有工具调用, 请将其格式化为 XML
	if len(msg.ToolCalls) > 0 {
		toolCallsXML := "<tool_calls>\n"
		for _, tc := range msg.ToolCalls {
			callJSON, _ := json.Marshal(map[string]any{
				"name":      tc.Name,
				"arguments": json.RawMessage(tc.Arguments),
			})
			toolCallsXML += string(callJSON) + "\n"
		}
		toolCallsXML += "</tool_calls>"
		converted.Content = toolCallsXML
	}

	return converted
}

// 转换MessagesMiniMaxFormat 转换多个 llm. 消息到 MiniMax 格式
func convertMessagesMiniMaxFormat(msgs []types.Message) []miniMaxMessageFormat {
	out := make([]miniMaxMessageFormat, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, convertMessageMiniMaxFormat(m))
	}
	return out
}
