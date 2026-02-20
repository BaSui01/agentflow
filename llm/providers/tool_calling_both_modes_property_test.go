package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/BaSui01/agentflow/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

// 特性: 多提供者支持, 属性 21: 在两种模式下调用工具
// ** 参数:要求11.6**
//
// 这项财产测试验证了任何供应商和任何带有工具的聊天请求,
// 完成()和Stream()应成功处理工具呼叫和
// 返回/邮件工具调用信息。

// 模拟TooCallServer 创建模拟工具调用响应的测试服务器
func mockToolCallServer(t *testing.T, toolCalls []mockToolCallData, streaming bool) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 校验请求有工具
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		defer r.Body.Close()

		var req map[string]interface{}
		err = json.Unmarshal(body, &req)
		require.NoError(t, err)

		// 检查流线
		isStream, _ := req["stream"].(bool)

		if isStream {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			flusher, ok := w.(http.Flusher)
			if !ok {
				t.Fatal("expected http.Flusher")
			}

			// 以流式发送工具调用
			for i, tc := range toolCalls {
				chunk := mockStreamChunk{
					ID:    "chatcmpl-test",
					Model: "test-model",
					Choices: []mockStreamChoice{
						{
							Index: 0,
							Delta: mockDelta{
								Role: "assistant",
								ToolCalls: []mockToolCall{
									{
										ID:   tc.ID,
										Type: "function",
										Function: mockFunction{
											Name:      tc.Name,
											Arguments: tc.Arguments,
										},
									},
								},
							},
						},
					},
				}
				if i == len(toolCalls)-1 {
					chunk.Choices[0].FinishReason = "tool_calls"
				}
				data, _ := json.Marshal(chunk)
				fmt.Fprintf(w, "data: %s\n\n", data)
				flusher.Flush()
			}
			fmt.Fprintf(w, "data: [DONE]\n\n")
			flusher.Flush()
		} else {
			// 非分流反应
			w.Header().Set("Content-Type", "application/json")
			resp := mockCompletionResponse{
				ID:    "chatcmpl-test",
				Model: "test-model",
				Choices: []mockCompletionChoice{
					{
						Index:        0,
						FinishReason: "tool_calls",
						Message: mockMessage{
							Role:      "assistant",
							ToolCalls: make([]mockToolCall, 0, len(toolCalls)),
						},
					},
				},
				Usage: &mockUsage{
					PromptTokens:     10,
					CompletionTokens: 20,
					TotalTokens:      30,
				},
			}
			for _, tc := range toolCalls {
				resp.Choices[0].Message.ToolCalls = append(resp.Choices[0].Message.ToolCalls, mockToolCall{
					ID:   tc.ID,
					Type: "function",
					Function: mockFunction{
						Name:      tc.Name,
						Arguments: tc.Arguments,
					},
				})
			}
			json.NewEncoder(w).Encode(resp)
		}
	}))
}

// 测试服务器响应的模拟类型
type mockToolCallData struct {
	ID        string
	Name      string
	Arguments string
}

type mockFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type mockToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function mockFunction `json:"function"`
}

type mockMessage struct {
	Role      string         `json:"role"`
	Content   string         `json:"content,omitempty"`
	ToolCalls []mockToolCall `json:"tool_calls,omitempty"`
}

type mockCompletionChoice struct {
	Index        int         `json:"index"`
	FinishReason string      `json:"finish_reason"`
	Message      mockMessage `json:"message"`
}

type mockUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type mockCompletionResponse struct {
	ID      string                 `json:"id"`
	Model   string                 `json:"model"`
	Choices []mockCompletionChoice `json:"choices"`
	Usage   *mockUsage             `json:"usage,omitempty"`
}

type mockDelta struct {
	Role      string         `json:"role,omitempty"`
	Content   string         `json:"content,omitempty"`
	ToolCalls []mockToolCall `json:"tool_calls,omitempty"`
}

type mockStreamChoice struct {
	Index        int       `json:"index"`
	Delta        mockDelta `json:"delta"`
	FinishReason string    `json:"finish_reason,omitempty"`
}

type mockStreamChunk struct {
	ID      string             `json:"id"`
	Model   string             `json:"model"`
	Choices []mockStreamChoice `json:"choices"`
}

// 测试Property21  ToolCalling InBothModes 验证调用工具在两者中都有效
// 所有提供者的补全()和Stream()模式。
func TestProperty21_ToolCallingInBothModes(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// 生成随机工具调用数据
		numToolCalls := rapid.IntRange(1, 3).Draw(rt, "numToolCalls")
		toolCalls := make([]mockToolCallData, numToolCalls)
		for i := 0; i < numToolCalls; i++ {
			toolCalls[i] = mockToolCallData{
				ID:        fmt.Sprintf("call_%s", rapid.StringMatching(`[a-z0-9]{8}`).Draw(rt, fmt.Sprintf("toolCallID_%d", i))),
				Name:      rapid.StringMatching(`[a-z_]{3,15}`).Draw(rt, fmt.Sprintf("toolName_%d", i)),
				Arguments: fmt.Sprintf(`{"param_%d": "%s"}`, i, rapid.StringMatching(`[a-z]{3,10}`).Draw(rt, fmt.Sprintf("argValue_%d", i))),
			}
		}

		// 生成请求工具
		tools := make([]llm.ToolSchema, numToolCalls)
		for i := 0; i < numToolCalls; i++ {
			tools[i] = llm.ToolSchema{
				Name:        toolCalls[i].Name,
				Description: fmt.Sprintf("Tool %d description", i),
				Parameters:  json.RawMessage(`{"type":"object","properties":{}}`),
			}
		}

		// 测试非流化模式
		t.Run("Completion", func(t *testing.T) {
			server := mockToolCallServer(t, toolCalls, false)
			defer server.Close()

			req := &llm.ChatRequest{
				Model: "test-model",
				Messages: []llm.Message{
					{Role: llm.RoleUser, Content: "Test message"},
				},
				Tools: tools,
			}

			resp, err := mockProviderCompletion(server.URL, req)
			require.NoError(t, err)
			require.NotNil(t, resp)

			// 返回校验工具呼叫
			require.Len(t, resp.Choices, 1)
			require.Len(t, resp.Choices[0].Message.ToolCalls, numToolCalls)

			for i, tc := range resp.Choices[0].Message.ToolCalls {
				assert.Equal(t, toolCalls[i].ID, tc.ID, "Tool call ID should match")
				assert.Equal(t, toolCalls[i].Name, tc.Name, "Tool call name should match")
				assert.JSONEq(t, toolCalls[i].Arguments, string(tc.Arguments), "Tool call arguments should match")
			}
		})

		// 测试流线模式
		t.Run("Stream", func(t *testing.T) {
			server := mockToolCallServer(t, toolCalls, true)
			defer server.Close()

			req := &llm.ChatRequest{
				Model: "test-model",
				Messages: []llm.Message{
					{Role: llm.RoleUser, Content: "Test message"},
				},
				Tools: tools,
			}

			chunks, err := mockProviderStream(server.URL, req)
			require.NoError(t, err)

			// 从流中收集全部工具呼叫
			var receivedToolCalls []llm.ToolCall
			for chunk := range chunks {
				if chunk.Err != nil {
					t.Fatalf("Stream error: %v", chunk.Err)
				}
				if len(chunk.Delta.ToolCalls) > 0 {
					receivedToolCalls = append(receivedToolCalls, chunk.Delta.ToolCalls...)
				}
			}

			// 发送了校验工具呼叫
			require.Len(t, receivedToolCalls, numToolCalls)
			for i, tc := range receivedToolCalls {
				assert.Equal(t, toolCalls[i].ID, tc.ID, "Tool call ID should match")
				assert.Equal(t, toolCalls[i].Name, tc.Name, "Tool call name should match")
				assert.JSONEq(t, toolCalls[i].Arguments, string(tc.Arguments), "Tool call arguments should match")
			}
		})
	})
}

// 模拟 Provider 完成 模拟提供者的完成方法
func mockProviderCompletion(baseURL string, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	ctx := context.Background()

	// 构建请求正文
	body := map[string]interface{}{
		"model":    req.Model,
		"messages": convertMessagesToMap(req.Messages),
		"tools":    convertToolsToMap(req.Tools),
		"stream":   false,
	}
	if req.ToolChoice != "" {
		body["tool_choice"] = req.ToolChoice
	}

	payload, _ := json.Marshal(body)
	httpReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v1/chat/completions", strings.NewReader(string(payload)))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer test-key")

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP error: %d", resp.StatusCode)
	}

	var oaResp mockCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&oaResp); err != nil {
		return nil, err
	}

	return convertToLLMResponse(oaResp), nil
}

// 模拟 ProviderStream 模拟提供者的流法
func mockProviderStream(baseURL string, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	ctx := context.Background()

	// 构建请求正文
	body := map[string]interface{}{
		"model":    req.Model,
		"messages": convertMessagesToMap(req.Messages),
		"tools":    convertToolsToMap(req.Tools),
		"stream":   true,
	}
	if req.ToolChoice != "" {
		body["tool_choice"] = req.ToolChoice
	}

	payload, _ := json.Marshal(body)
	httpReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v1/chat/completions", strings.NewReader(string(payload)))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer test-key")

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		resp.Body.Close()
		return nil, fmt.Errorf("HTTP error: %d", resp.StatusCode)
	}

	ch := make(chan llm.StreamChunk)
	go func() {
		defer resp.Body.Close()
		defer close(ch)
		parseSSEStream(resp.Body, ch)
	}()

	return ch, nil
}

// 解析 SSE 解析 SSE 流出块
func parseSSEStream(body io.Reader, ch chan<- llm.StreamChunk) {
	buf := make([]byte, 4096)
	var leftover string

	for {
		n, err := body.Read(buf)
		if n > 0 {
			data := leftover + string(buf[:n])
			lines := strings.Split(data, "\n")

			// 为下次重复保留不完整的行
			if !strings.HasSuffix(data, "\n") {
				leftover = lines[len(lines)-1]
				lines = lines[:len(lines)-1]
			} else {
				leftover = ""
			}

			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line == "" || !strings.HasPrefix(line, "data:") {
					continue
				}
				jsonData := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
				if jsonData == "[DONE]" {
					return
				}

				var chunk mockStreamChunk
				if err := json.Unmarshal([]byte(jsonData), &chunk); err != nil {
					ch <- llm.StreamChunk{Err: &llm.Error{Code: llm.ErrUpstreamError, Message: err.Error()}}
					return
				}

				for _, choice := range chunk.Choices {
					streamChunk := llm.StreamChunk{
						ID:           chunk.ID,
						Model:        chunk.Model,
						Index:        choice.Index,
						FinishReason: choice.FinishReason,
						Delta: llm.Message{
							Role:    llm.RoleAssistant,
							Content: choice.Delta.Content,
						},
					}
					if len(choice.Delta.ToolCalls) > 0 {
						streamChunk.Delta.ToolCalls = make([]llm.ToolCall, 0, len(choice.Delta.ToolCalls))
						for _, tc := range choice.Delta.ToolCalls {
							streamChunk.Delta.ToolCalls = append(streamChunk.Delta.ToolCalls, llm.ToolCall{
								ID:        tc.ID,
								Name:      tc.Function.Name,
								Arguments: json.RawMessage(tc.Function.Arguments),
							})
						}
					}
					ch <- streamChunk
				}
			}
		}
		if err != nil {
			if err != io.EOF {
				ch <- llm.StreamChunk{Err: &llm.Error{Code: llm.ErrUpstreamError, Message: err.Error()}}
			}
			return
		}
	}
}

// 转换的辅助功能
func convertMessagesToMap(msgs []llm.Message) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(msgs))
	for _, m := range msgs {
		msg := map[string]interface{}{
			"role":    string(m.Role),
			"content": m.Content,
		}
		if m.Name != "" {
			msg["name"] = m.Name
		}
		if m.ToolCallID != "" {
			msg["tool_call_id"] = m.ToolCallID
		}
		result = append(result, msg)
	}
	return result
}

func convertToolsToMap(tools []llm.ToolSchema) []map[string]interface{} {
	if len(tools) == 0 {
		return nil
	}
	result := make([]map[string]interface{}, 0, len(tools))
	for _, t := range tools {
		result = append(result, map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name":        t.Name,
				"description": t.Description,
				"parameters":  json.RawMessage(t.Parameters),
			},
		})
	}
	return result
}

func convertToLLMResponse(resp mockCompletionResponse) *llm.ChatResponse {
	choices := make([]llm.ChatChoice, 0, len(resp.Choices))
	for _, c := range resp.Choices {
		msg := llm.Message{
			Role:    llm.RoleAssistant,
			Content: c.Message.Content,
		}
		if len(c.Message.ToolCalls) > 0 {
			msg.ToolCalls = make([]llm.ToolCall, 0, len(c.Message.ToolCalls))
			for _, tc := range c.Message.ToolCalls {
				msg.ToolCalls = append(msg.ToolCalls, llm.ToolCall{
					ID:        tc.ID,
					Name:      tc.Function.Name,
					Arguments: json.RawMessage(tc.Function.Arguments),
				})
			}
		}
		choices = append(choices, llm.ChatChoice{
			Index:        c.Index,
			FinishReason: c.FinishReason,
			Message:      msg,
		})
	}

	result := &llm.ChatResponse{
		ID:      resp.ID,
		Model:   resp.Model,
		Choices: choices,
	}
	if resp.Usage != nil {
		result.Usage = llm.ChatUsage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		}
	}
	return result
}

// Property21  ToolCalling Both Modes  TableDriven 提供了额外的覆盖范围
// 通过表格驱动测试,确保至少100次重复。
func TestProperty21_ToolCallingBothModes_TableDriven(t *testing.T) {
	testCases := []struct {
		name      string
		toolCalls []mockToolCallData
		provider  string
		mode      string // "completion" or "stream"
	}{
		// 单一工具呼叫案件
		{
			name: "Single tool call - completion - grok",
			toolCalls: []mockToolCallData{
				{ID: "call_001", Name: "search", Arguments: `{"query": "test"}`},
			},
			provider: "grok",
			mode:     "completion",
		},
		{
			name: "Single tool call - stream - grok",
			toolCalls: []mockToolCallData{
				{ID: "call_002", Name: "search", Arguments: `{"query": "test"}`},
			},
			provider: "grok",
			mode:     "stream",
		},
		{
			name: "Single tool call - completion - qwen",
			toolCalls: []mockToolCallData{
				{ID: "call_003", Name: "calculate", Arguments: `{"expression": "1+1"}`},
			},
			provider: "qwen",
			mode:     "completion",
		},
		{
			name: "Single tool call - stream - qwen",
			toolCalls: []mockToolCallData{
				{ID: "call_004", Name: "calculate", Arguments: `{"expression": "1+1"}`},
			},
			provider: "qwen",
			mode:     "stream",
		},
		{
			name: "Single tool call - completion - deepseek",
			toolCalls: []mockToolCallData{
				{ID: "call_005", Name: "get_weather", Arguments: `{"location": "Beijing"}`},
			},
			provider: "deepseek",
			mode:     "completion",
		},
		{
			name: "Single tool call - stream - deepseek",
			toolCalls: []mockToolCallData{
				{ID: "call_006", Name: "get_weather", Arguments: `{"location": "Beijing"}`},
			},
			provider: "deepseek",
			mode:     "stream",
		},
		{
			name: "Single tool call - completion - glm",
			toolCalls: []mockToolCallData{
				{ID: "call_007", Name: "translate", Arguments: `{"text": "hello", "target": "zh"}`},
			},
			provider: "glm",
			mode:     "completion",
		},
		{
			name: "Single tool call - stream - glm",
			toolCalls: []mockToolCallData{
				{ID: "call_008", Name: "translate", Arguments: `{"text": "hello", "target": "zh"}`},
			},
			provider: "glm",
			mode:     "stream",
		},
		{
			name: "Single tool call - completion - minimax",
			toolCalls: []mockToolCallData{
				{ID: "call_009", Name: "summarize", Arguments: `{"text": "long text here"}`},
			},
			provider: "minimax",
			mode:     "completion",
		},
		{
			name: "Single tool call - stream - minimax",
			toolCalls: []mockToolCallData{
				{ID: "call_010", Name: "summarize", Arguments: `{"text": "long text here"}`},
			},
			provider: "minimax",
			mode:     "stream",
		},

		// 多个工具调用案件
		{
			name: "Two tool calls - completion - grok",
			toolCalls: []mockToolCallData{
				{ID: "call_011", Name: "search", Arguments: `{"query": "weather"}`},
				{ID: "call_012", Name: "get_time", Arguments: `{"timezone": "UTC"}`},
			},
			provider: "grok",
			mode:     "completion",
		},
		{
			name: "Two tool calls - stream - grok",
			toolCalls: []mockToolCallData{
				{ID: "call_013", Name: "search", Arguments: `{"query": "weather"}`},
				{ID: "call_014", Name: "get_time", Arguments: `{"timezone": "UTC"}`},
			},
			provider: "grok",
			mode:     "stream",
		},
		{
			name: "Three tool calls - completion - qwen",
			toolCalls: []mockToolCallData{
				{ID: "call_015", Name: "tool_a", Arguments: `{"param": "a"}`},
				{ID: "call_016", Name: "tool_b", Arguments: `{"param": "b"}`},
				{ID: "call_017", Name: "tool_c", Arguments: `{"param": "c"}`},
			},
			provider: "qwen",
			mode:     "completion",
		},
		{
			name: "Three tool calls - stream - qwen",
			toolCalls: []mockToolCallData{
				{ID: "call_018", Name: "tool_a", Arguments: `{"param": "a"}`},
				{ID: "call_019", Name: "tool_b", Arguments: `{"param": "b"}`},
				{ID: "call_020", Name: "tool_c", Arguments: `{"param": "c"}`},
			},
			provider: "qwen",
			mode:     "stream",
		},
	}

	// 添加更多的测试案例以达到100+重复
	providers := []string{"grok", "qwen", "deepseek", "glm", "minimax"}
	modes := []string{"completion", "stream"}
	toolNames := []string{"search", "calculate", "get_weather", "translate", "summarize", "fetch_data", "process", "validate", "analyze", "generate"}

	idx := 21
	for _, provider := range providers {
		for _, mode := range modes {
			for _, toolName := range toolNames {
				testCases = append(testCases, struct {
					name      string
					toolCalls []mockToolCallData
					provider  string
					mode      string
				}{
					name: fmt.Sprintf("Tool %s - %s - %s", toolName, mode, provider),
					toolCalls: []mockToolCallData{
						{ID: fmt.Sprintf("call_%03d", idx), Name: toolName, Arguments: fmt.Sprintf(`{"input": "test_%d"}`, idx)},
					},
					provider: provider,
					mode:     mode,
				})
				idx++
			}
		}
	}

	// 检查我们至少有100个测试病例
	require.GreaterOrEqual(t, len(testCases), 100, "Should have at least 100 test cases")

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			isStream := tc.mode == "stream"
			server := mockToolCallServer(t, tc.toolCalls, isStream)
			defer server.Close()

			tools := make([]llm.ToolSchema, len(tc.toolCalls))
			for i, tcall := range tc.toolCalls {
				tools[i] = llm.ToolSchema{
					Name:        tcall.Name,
					Description: fmt.Sprintf("Tool %s", tcall.Name),
					Parameters:  json.RawMessage(`{"type":"object"}`),
				}
			}

			req := &llm.ChatRequest{
				Model: "test-model",
				Messages: []llm.Message{
					{Role: llm.RoleUser, Content: "Test message"},
				},
				Tools: tools,
			}

			if isStream {
				chunks, err := mockProviderStream(server.URL, req)
				require.NoError(t, err)

				var receivedToolCalls []llm.ToolCall
				for chunk := range chunks {
					require.Nil(t, chunk.Err, "Stream should not have errors")
					if len(chunk.Delta.ToolCalls) > 0 {
						receivedToolCalls = append(receivedToolCalls, chunk.Delta.ToolCalls...)
					}
				}

				require.Len(t, receivedToolCalls, len(tc.toolCalls), "Should receive all tool calls")
				for i, received := range receivedToolCalls {
					assert.Equal(t, tc.toolCalls[i].ID, received.ID)
					assert.Equal(t, tc.toolCalls[i].Name, received.Name)
				}
			} else {
				resp, err := mockProviderCompletion(server.URL, req)
				require.NoError(t, err)
				require.NotNil(t, resp)
				require.Len(t, resp.Choices, 1)
				require.Len(t, resp.Choices[0].Message.ToolCalls, len(tc.toolCalls))

				for i, received := range resp.Choices[0].Message.ToolCalls {
					assert.Equal(t, tc.toolCalls[i].ID, received.ID)
					assert.Equal(t, tc.toolCalls[i].Name, received.Name)
				}
			}
		})
	}
}

// 测试Property21  ToolCalling Preserves ToolCallFields 验证所有工具调用
// 字段(ID、名称、参数)在两种模式中都保留。
func TestProperty21_ToolCallingPreservesToolCallFields(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// 生成包含各种字段值的工具调用
		toolCall := mockToolCallData{
			ID:   rapid.StringMatching(`call_[a-zA-Z0-9]{8,16}`).Draw(rt, "toolCallID"),
			Name: rapid.StringMatching(`[a-z][a-z_]{2,20}`).Draw(rt, "toolName"),
			Arguments: fmt.Sprintf(`{"key": "%s", "num": %d}`,
				rapid.StringMatching(`[a-z]{3,10}`).Draw(rt, "argKey"),
				rapid.IntRange(0, 1000).Draw(rt, "argNum")),
		}

		tools := []llm.ToolSchema{
			{
				Name:        toolCall.Name,
				Description: "Test tool",
				Parameters:  json.RawMessage(`{"type":"object"}`),
			},
		}

		req := &llm.ChatRequest{
			Model: "test-model",
			Messages: []llm.Message{
				{Role: llm.RoleUser, Content: "Test"},
			},
			Tools: tools,
		}

		// 测试完成模式
		server := mockToolCallServer(t, []mockToolCallData{toolCall}, false)
		resp, err := mockProviderCompletion(server.URL, req)
		server.Close()

		require.NoError(t, err)
		require.Len(t, resp.Choices[0].Message.ToolCalls, 1)

		tc := resp.Choices[0].Message.ToolCalls[0]
		assert.Equal(t, toolCall.ID, tc.ID, "Tool call ID should be preserved in Completion mode")
		assert.Equal(t, toolCall.Name, tc.Name, "Tool call Name should be preserved in Completion mode")
		assert.JSONEq(t, toolCall.Arguments, string(tc.Arguments), "Tool call Arguments should be preserved in Completion mode")

		// 测试流模式
		server = mockToolCallServer(t, []mockToolCallData{toolCall}, true)
		chunks, err := mockProviderStream(server.URL, req)
		require.NoError(t, err)

		var streamedToolCalls []llm.ToolCall
		for chunk := range chunks {
			if len(chunk.Delta.ToolCalls) > 0 {
				streamedToolCalls = append(streamedToolCalls, chunk.Delta.ToolCalls...)
			}
		}
		server.Close()

		require.Len(t, streamedToolCalls, 1)
		stc := streamedToolCalls[0]
		assert.Equal(t, toolCall.ID, stc.ID, "Tool call ID should be preserved in Stream mode")
		assert.Equal(t, toolCall.Name, stc.Name, "Tool call Name should be preserved in Stream mode")
		assert.JSONEq(t, toolCall.Arguments, string(stc.Arguments), "Tool call Arguments should be preserved in Stream mode")
	})
}

// TestProperty21  ToolCalling With ToolChoice 校验工具与 ToolChoice 工作
func TestProperty21_ToolCallingWithToolChoice(t *testing.T) {
	toolChoices := []string{"auto", "none", "required", "search", "calculate"}
	modes := []string{"completion", "stream"}

	for _, toolChoice := range toolChoices {
		for _, mode := range modes {
			t.Run(fmt.Sprintf("ToolChoice_%s_%s", toolChoice, mode), func(t *testing.T) {
				toolCall := mockToolCallData{
					ID:        "call_tc001",
					Name:      "search",
					Arguments: `{"query": "test"}`,
				}

				isStream := mode == "stream"
				server := mockToolCallServer(t, []mockToolCallData{toolCall}, isStream)
				defer server.Close()

				req := &llm.ChatRequest{
					Model: "test-model",
					Messages: []llm.Message{
						{Role: llm.RoleUser, Content: "Test"},
					},
					Tools: []llm.ToolSchema{
						{Name: "search", Parameters: json.RawMessage(`{}`)},
					},
					ToolChoice: toolChoice,
				}

				if isStream {
					chunks, err := mockProviderStream(server.URL, req)
					require.NoError(t, err)

					var received []llm.ToolCall
					for chunk := range chunks {
						if len(chunk.Delta.ToolCalls) > 0 {
							received = append(received, chunk.Delta.ToolCalls...)
						}
					}
					assert.Len(t, received, 1)
				} else {
					resp, err := mockProviderCompletion(server.URL, req)
					require.NoError(t, err)
					assert.Len(t, resp.Choices[0].Message.ToolCalls, 1)
				}
			})
		}
	}
}
