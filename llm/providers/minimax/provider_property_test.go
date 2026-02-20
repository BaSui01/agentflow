package minimax

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

// 特性: 多提供者支持, 属性 19: 工具呼叫响应解析
// 核实:所需经费3.8、11.3
func TestProperty19_ToolCallResponseParsing(t *testing.T) {
	testCases := []struct {
		name              string
		responseContent   string
		expectedToolCalls int
		expectedToolNames []string
	}{
		{
			name: "single tool call",
			responseContent: `<tool_calls>
{"name":"get_weather","arguments":{"location":"Beijing"}}
</tool_calls>`,
			expectedToolCalls: 1,
			expectedToolNames: []string{"get_weather"},
		},
		{
			name: "multiple tool calls",
			responseContent: `<tool_calls>
{"name":"get_weather","arguments":{"location":"Beijing"}}
{"name":"get_time","arguments":{"timezone":"Asia/Shanghai"}}
</tool_calls>`,
			expectedToolCalls: 2,
			expectedToolNames: []string{"get_weather", "get_time"},
		},
		{
			name: "tool call with complex arguments",
			responseContent: `<tool_calls>
{"name":"search","arguments":{"query":"AI news","filters":{"date":"2024","category":"tech"}}}
</tool_calls>`,
			expectedToolCalls: 1,
			expectedToolNames: []string{"search"},
		},
		{
			name:              "no tool calls",
			responseContent:   "This is a regular response without tool calls",
			expectedToolCalls: 0,
			expectedToolNames: []string{},
		},
		{
			name: "tool call with text before and after",
			responseContent: `Let me check the weather for you.
<tool_calls>
{"name":"get_weather","arguments":{"location":"Shanghai"}}
</tool_calls>
I'll get that information.`,
			expectedToolCalls: 1,
			expectedToolNames: []string{"get_weather"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 创建返回指定响应的测试服务器
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(miniMaxResponse{
					ID:    "test-id",
					Model: "abab6.5s-chat",
					Choices: []struct {
						Index        int             `json:"index"`
						FinishReason string          `json:"finish_reason"`
						Message      miniMaxMessage  `json:"message"`
						Delta        *miniMaxMessage `json:"delta,omitempty"`
					}{
						{
							Index:        0,
							FinishReason: "stop",
							Message: miniMaxMessage{
								Role:    "assistant",
								Content: tc.responseContent,
							},
						},
					},
				})
			}))
			defer server.Close()

			// 以测试服务器 URL 创建提供者
			cfg := providers.MiniMaxConfig{
				APIKey:  "test-key",
				BaseURL: server.URL,
			}
			provider := NewMiniMaxProvider(cfg, zap.NewNop())

			// 提出完成请求
			ctx := context.Background()
			req := &llm.ChatRequest{
				Messages: []llm.Message{
					{Role: llm.RoleUser, Content: "test"},
				},
			}

			resp, err := provider.Completion(ctx, req)
			assert.NoError(t, err, "Completion should succeed")

			// 校验工具呼叫被正确解析
			assert.Equal(t, 1, len(resp.Choices), "Should have one choice")

			toolCalls := resp.Choices[0].Message.ToolCalls
			assert.Equal(t, tc.expectedToolCalls, len(toolCalls),
				"Number of tool calls should match")

			// 校验工具名称
			for i, expectedName := range tc.expectedToolNames {
				assert.Equal(t, expectedName, toolCalls[i].Name,
					"Tool name should match")
				assert.NotEmpty(t, toolCalls[i].ID,
					"Tool call ID should not be empty")
				assert.NotNil(t, toolCalls[i].Arguments,
					"Tool call arguments should not be nil")
			}

			// 校验内容被清理( XML 已删除)
			if tc.expectedToolCalls > 0 {
				assert.NotContains(t, resp.Choices[0].Message.Content, "<tool_calls>",
					"Content should not contain XML tags")
			}
		})
	}

	// 测试 parseMiniMax ToolCalls 直接函数
	t.Run("parseMiniMaxToolCalls function", func(t *testing.T) {
		testCases := []struct {
			name     string
			input    string
			expected int
		}{
			{
				name: "valid single tool call",
				input: `<tool_calls>
{"name":"func1","arguments":{"key":"value"}}
</tool_calls>`,
				expected: 1,
			},
			{
				name: "valid multiple tool calls",
				input: `<tool_calls>
{"name":"func1","arguments":{"key1":"value1"}}
{"name":"func2","arguments":{"key2":"value2"}}
</tool_calls>`,
				expected: 2,
			},
			{
				name:     "no tool calls",
				input:    "regular text without tool calls",
				expected: 0,
			},
			{
				name:     "empty tool calls",
				input:    "<tool_calls></tool_calls>",
				expected: 0,
			},
			{
				name: "tool calls with empty lines",
				input: `<tool_calls>

{"name":"func1","arguments":{"key":"value"}}

</tool_calls>`,
				expected: 1,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				toolCalls := parseMiniMaxToolCalls(tc.input)
				assert.Equal(t, tc.expected, len(toolCalls),
					"Number of parsed tool calls should match")
			})
		}
	})
}
