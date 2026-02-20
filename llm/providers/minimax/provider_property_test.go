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
// 注意: MiniMax 已迁移至 openaicompat 基座，使用标准 OpenAI 格式的 tool_calls 字段。
func TestProperty19_ToolCallResponseParsing(t *testing.T) {
	testCases := []struct {
		name              string
		responseToolCalls []providers.OpenAICompatToolCall
		responseContent   string
		expectedToolCalls int
		expectedToolNames []string
	}{
		{
			name: "single tool call",
			responseToolCalls: []providers.OpenAICompatToolCall{
				{ID: "call_1", Type: "function", Function: providers.OpenAICompatFunction{
					Name: "get_weather", Arguments: json.RawMessage(`{"location":"Beijing"}`)}},
			},
			expectedToolCalls: 1,
			expectedToolNames: []string{"get_weather"},
		},
		{
			name: "multiple tool calls",
			responseToolCalls: []providers.OpenAICompatToolCall{
				{ID: "call_1", Type: "function", Function: providers.OpenAICompatFunction{
					Name: "get_weather", Arguments: json.RawMessage(`{"location":"Beijing"}`)}},
				{ID: "call_2", Type: "function", Function: providers.OpenAICompatFunction{
					Name: "get_time", Arguments: json.RawMessage(`{"timezone":"Asia/Shanghai"}`)}},
			},
			expectedToolCalls: 2,
			expectedToolNames: []string{"get_weather", "get_time"},
		},
		{
			name:              "no tool calls",
			responseContent:   "This is a regular response without tool calls",
			expectedToolCalls: 0,
			expectedToolNames: []string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(providers.OpenAICompatResponse{
					ID:    "test-id",
					Model: "abab6.5s-chat",
					Choices: []providers.OpenAICompatChoice{
						{
							Index:        0,
							FinishReason: "stop",
							Message: providers.OpenAICompatMessage{
								Role:      "assistant",
								Content:   tc.responseContent,
								ToolCalls: tc.responseToolCalls,
							},
						},
					},
				})
			}))
			defer server.Close()

			cfg := providers.MiniMaxConfig{
				BaseProviderConfig: providers.BaseProviderConfig{
					APIKey:  "test-key",
					BaseURL: server.URL,
				},
			}
			provider := NewMiniMaxProvider(cfg, zap.NewNop())

			ctx := context.Background()
			req := &llm.ChatRequest{
				Messages: []llm.Message{
					{Role: llm.RoleUser, Content: "test"},
				},
			}

			resp, err := provider.Completion(ctx, req)
			assert.NoError(t, err, "Completion should succeed")
			assert.Equal(t, 1, len(resp.Choices), "Should have one choice")

			toolCalls := resp.Choices[0].Message.ToolCalls
			assert.Equal(t, tc.expectedToolCalls, len(toolCalls),
				"Number of tool calls should match")

			for i, expectedName := range tc.expectedToolNames {
				assert.Equal(t, expectedName, toolCalls[i].Name,
					"Tool name should match")
				assert.NotEmpty(t, toolCalls[i].ID,
					"Tool call ID should not be empty")
				assert.NotNil(t, toolCalls[i].Arguments,
					"Tool call arguments should not be nil")
			}
		})
	}
}
