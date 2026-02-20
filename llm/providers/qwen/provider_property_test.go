package qwen

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

// 特性: 多提供者支持, 属性 3: OpenAI 格式转换兼容提供者
// 审定:要求4.4
func TestProperty3_OpenAIFormatConversion(t *testing.T) {
	testCases := []struct {
		name             string
		messages         []llm.Message
		tools            []llm.ToolSchema
		expectedMessages int
		expectedTools    int
	}{
		{
			name: "simple user message",
			messages: []llm.Message{
				{Role: llm.RoleUser, Content: "Hello"},
			},
			tools:            nil,
			expectedMessages: 1,
			expectedTools:    0,
		},
		{
			name: "system and user messages",
			messages: []llm.Message{
				{Role: llm.RoleSystem, Content: "You are a helpful assistant"},
				{Role: llm.RoleUser, Content: "Hello"},
			},
			tools:            nil,
			expectedMessages: 2,
			expectedTools:    0,
		},
		{
			name: "messages with tools",
			messages: []llm.Message{
				{Role: llm.RoleUser, Content: "What's the weather?"},
			},
			tools: []llm.ToolSchema{
				{
					Name:        "get_weather",
					Description: "Get weather information",
					Parameters:  json.RawMessage(`{"type":"object","properties":{"location":{"type":"string"}}}`),
				},
			},
			expectedMessages: 1,
			expectedTools:    1,
		},
		{
			name: "assistant message with tool calls",
			messages: []llm.Message{
				{Role: llm.RoleUser, Content: "What's the weather in Beijing?"},
				{
					Role: llm.RoleAssistant,
					ToolCalls: []llm.ToolCall{
						{
							ID:        "call_123",
							Name:      "get_weather",
							Arguments: json.RawMessage(`{"location":"Beijing"}`),
						},
					},
				},
			},
			tools:            nil,
			expectedMessages: 2,
			expectedTools:    0,
		},
		{
			name: "tool result message",
			messages: []llm.Message{
				{Role: llm.RoleUser, Content: "What's the weather?"},
				{
					Role: llm.RoleAssistant,
					ToolCalls: []llm.ToolCall{
						{ID: "call_123", Name: "get_weather", Arguments: json.RawMessage(`{"location":"Beijing"}`)},
					},
				},
				{
					Role:       llm.RoleTool,
					Content:    `{"temperature":20,"condition":"sunny"}`,
					ToolCallID: "call_123",
				},
			},
			tools:            nil,
			expectedMessages: 3,
			expectedTools:    0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 创建测试服务器以抓取请求
			var capturedRequest providers.OpenAICompatRequest
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// 解码请求正文
				json.NewDecoder(r.Body).Decode(&capturedRequest)

				// 返回有效的响应
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(providers.OpenAICompatResponse{
					ID:    "test-id",
					Model: "qwen-plus",
					Choices: []providers.OpenAICompatChoice{
						{
							Index:        0,
							FinishReason: "stop",
							Message: providers.OpenAICompatMessage{
								Role:    "assistant",
								Content: "test response",
							},
						},
					},
				})
			}))
			defer server.Close()

			// 以测试服务器 URL 创建提供者
			cfg := providers.QwenConfig{
				BaseProviderConfig: providers.BaseProviderConfig{
					APIKey:  "test-key",
					BaseURL: server.URL,
				},
			}
			provider := NewQwenProvider(cfg, zap.NewNop())

			// 提出完成请求
			ctx := context.Background()
			req := &llm.ChatRequest{
				Messages: tc.messages,
				Tools:    tc.tools,
			}

			_, err := provider.Completion(ctx, req)
			assert.NoError(t, err, "Completion should succeed")

			// 校验请求已转换为 OpenAI 格式
			assert.Equal(t, tc.expectedMessages, len(capturedRequest.Messages),
				"Number of messages should match")
			assert.Equal(t, tc.expectedTools, len(capturedRequest.Tools),
				"Number of tools should match")

			// 校验消息角色保存
			for i, msg := range tc.messages {
				assert.Equal(t, string(msg.Role), capturedRequest.Messages[i].Role,
					"Message role should be preserved")
				if msg.Content != "" {
					assert.Equal(t, msg.Content, capturedRequest.Messages[i].Content,
						"Message content should be preserved")
				}
			}

			// 校验工具调用正确转换
			for i, msg := range tc.messages {
				if len(msg.ToolCalls) > 0 {
					assert.Equal(t, len(msg.ToolCalls), len(capturedRequest.Messages[i].ToolCalls),
						"Number of tool calls should match")
					for j, tc := range msg.ToolCalls {
						assert.Equal(t, tc.ID, capturedRequest.Messages[i].ToolCalls[j].ID,
							"Tool call ID should be preserved")
						assert.Equal(t, tc.Name, capturedRequest.Messages[i].ToolCalls[j].Function.Name,
							"Tool call name should be preserved")
						assert.Equal(t, "function", capturedRequest.Messages[i].ToolCalls[j].Type,
							"Tool call type should be 'function'")
					}
				}
			}

			// 校验工具被正确转换
			for i, tool := range tc.tools {
				assert.Equal(t, tool.Name, capturedRequest.Tools[i].Function.Name,
					"Tool name should be preserved")
				assert.Equal(t, "function", capturedRequest.Tools[i].Type,
					"Tool type should be 'function'")
			}
		})
	}
}
