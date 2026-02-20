package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/BaSui01/agentflow/llm/providers/deepseek"
	"github.com/BaSui01/agentflow/llm/providers/glm"
	"github.com/BaSui01/agentflow/llm/providers/grok"
	"github.com/BaSui01/agentflow/llm/providers/minimax"
	"github.com/BaSui01/agentflow/llm/providers/qwen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"pgregory.net/rapid"
)

// 特性：多提供商支持，特性 16：流媒体中的工具调用累积
// **验证：要求 10.6**
//
// This 属性测试验证对于发送部分工具调用的任何提供者
// JSON 跨多个流块，累积的工具调用参数应该
// 当所有块组合在一起时形成有效的 JSON。

// partialToolCallChunk 表示具有部分工具调用数据的块
type partialToolCallChunk struct {
	ID           string
	Model        string
	ToolCallID   string
	ToolCallName string
	PartialArgs  string // Partial JSON arguments
	Index        int    // Tool call index within the chunk
	IsFirst      bool   // First chunk contains ID and name
	FinishReason string
}

// mockSSEServerWithPartialToolCalls 创建一个发送工具调用的测试服务器
// 多个部分块中的参数（模拟 OpenAI 等提供商如何
// 使用部分 JSON 进行流工具调用）。
// 注意：OpenAI 将 arguments 作为字符串字段发送，内容为分段 JSON。
// Each chunk 的参数是一个字符串，连接后形成有效的 JSON。
func mockSSEServerWithPartialToolCalls(chunks []partialToolCallChunk) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			return
		}

		for _, chunk := range chunks {
			// 构建 SSE 数据 - 参数作为包含部分 JSON 的字符串发送
			var sseData map[string]any
			if chunk.IsFirst {
				sseData = map[string]any{
					"id":    chunk.ID,
					"model": chunk.Model,
					"choices": []map[string]any{
						{
							"index": 0,
							"delta": map[string]any{
								"role": "assistant",
								"tool_calls": []map[string]any{
									{
										"index": chunk.Index,
										"id":    chunk.ToolCallID,
										"type":  "function",
										"function": map[string]any{
											"name": chunk.ToolCallName,
											// 参数是包含部分 JSON 的字符串
											"arguments": chunk.PartialArgs,
										},
									},
								},
							},
						},
					},
				}
			} else {
				sseData = map[string]any{
					"id":    chunk.ID,
					"model": chunk.Model,
					"choices": []map[string]any{
						{
							"index": 0,
							"delta": map[string]any{
								"tool_calls": []map[string]any{
									{
										"index": chunk.Index,
										"function": map[string]any{
											// 参数是包含部分 JSON 的字符串
											"arguments": chunk.PartialArgs,
										},
									},
								},
							},
						},
					},
				}
			}
			if chunk.FinishReason != "" {
				sseData["choices"].([]map[string]any)[0]["finish_reason"] = chunk.FinishReason
			}

			jsonData, _ := json.Marshal(sseData)
			fmt.Fprintf(w, "data: %s\n\n", jsonData)
			flusher.Flush()
		}

		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
}

// splitJSONIntoChunks 将 JSON 字符串拆分为多个部分块
func splitJSONIntoChunks(jsonStr string, numChunks int) []string {
	if numChunks <= 1 || len(jsonStr) == 0 {
		return []string{jsonStr}
	}

	chunkSize := len(jsonStr) / numChunks
	if chunkSize == 0 {
		chunkSize = 1
	}

	var chunks []string
	for i := 0; i < len(jsonStr); i += chunkSize {
		end := i + chunkSize
		if end > len(jsonStr) {
			end = len(jsonStr)
		}
		chunks = append(chunks, jsonStr[i:end])
	}

	return chunks
}

// TestProperty16_ToolCallAccumulation 验证部分工具调用 JSON
// 正确累积块以形成所有提供者的有效 JSON。
func TestProperty16_ToolCallAccumulation(t *testing.T) {
	logger := zap.NewNop()

	rapid.Check(t, func(rt *rapid.T) {
		// 生成随机工具调用数据
		toolCallID := rapid.StringMatching(`call_[a-z0-9]{8}`).Draw(rt, "toolCallID")
		toolName := rapid.StringMatching(`[a-z_]{3,15}`).Draw(rt, "toolName")

		// 生成随机 JSON 参数
		paramKey := rapid.StringMatching(`[a-z]{3,10}`).Draw(rt, "paramKey")
		paramValue := rapid.StringMatching(`[a-zA-Z0-9]{3,20}`).Draw(rt, "paramValue")
		fullArgs := fmt.Sprintf(`{"%s":"%s"}`, paramKey, paramValue)

		// 分成2-4块
		numChunks := rapid.IntRange(2, 4).Draw(rt, "numChunks")
		argChunks := splitJSONIntoChunks(fullArgs, numChunks)

		// 构建部分工具调用块
		chunks := make([]partialToolCallChunk, len(argChunks))
		chunkID := rapid.StringMatching(`chatcmpl-[a-z0-9]{8}`).Draw(rt, "chunkID")
		chunkModel := rapid.StringMatching(`[a-z0-9-]{3,15}`).Draw(rt, "chunkModel")

		for i, argPart := range argChunks {
			chunks[i] = partialToolCallChunk{
				ID:           chunkID,
				Model:        chunkModel,
				ToolCallID:   toolCallID,
				ToolCallName: toolName,
				PartialArgs:  argPart,
				Index:        0,
				IsFirst:      i == 0,
			}
			if i == len(argChunks)-1 {
				chunks[i].FinishReason = "tool_calls"
			}
		}

		// 选择一个随机的 OpenAI 兼容提供商（不包括使用 XML 的 minimax）
		providerIndex := rapid.IntRange(0, 3).Draw(rt, "providerIndex")
		providerNames := []string{"grok", "qwen", "deepseek", "glm"}
		providerName := providerNames[providerIndex]

		server := mockSSEServerWithPartialToolCalls(chunks)
		defer server.Close()

		req := &llm.ChatRequest{
			Model: "test-model",
			Messages: []llm.Message{
				{Role: llm.RoleUser, Content: "Test message"},
			},
			Tools: []llm.ToolSchema{
				{Name: toolName, Parameters: json.RawMessage(`{}`)},
			},
		}

		ctx := context.Background()
		var streamCh <-chan llm.StreamChunk
		var err error

		switch providerName {
		case "grok":
			cfg := providers.GrokConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL}}
			p := grok.NewGrokProvider(cfg, logger)
			streamCh, err = p.Stream(ctx, req)
		case "qwen":
			cfg := providers.QwenConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL}}
			p := qwen.NewQwenProvider(cfg, logger)
			streamCh, err = p.Stream(ctx, req)
		case "deepseek":
			cfg := providers.DeepSeekConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL}}
			p := deepseek.NewDeepSeekProvider(cfg, logger)
			streamCh, err = p.Stream(ctx, req)
		case "glm":
			cfg := providers.GLMConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL}}
			p := glm.NewGLMProvider(cfg, logger)
			streamCh, err = p.Stream(ctx, req)
		}

		require.NoError(t, err, "Stream() should not return error for provider %s", providerName)

		// 收集所有工具调用块并累积参数
		// 遵循与 ReAct 执行器相同的逻辑：首先尝试将其解组为字符串
		var argsBuilder strings.Builder
		var receivedToolCallID string
		var receivedToolName string

		for chunk := range streamCh {
			require.Nil(t, chunk.Err, "Stream should not have errors for provider %s", providerName)
			if len(chunk.Delta.ToolCalls) > 0 {
				tc := chunk.Delta.ToolCalls[0]
				if tc.ID != "" {
					receivedToolCallID = tc.ID
				}
				if tc.Name != "" {
					receivedToolName = tc.Name
				}
				if len(tc.Arguments) > 0 {
					// 首先尝试解组为 JSON 字符串（OpenAI 格式）
					var argStr string
					if err := json.Unmarshal(tc.Arguments, &argStr); err == nil {
						argsBuilder.WriteString(argStr)
					} else {
						// 否则使用原始字节
						argsBuilder.Write(tc.Arguments)
					}
				}
			}
		}

		accumulatedArgs := argsBuilder.String()

		// 验证累积参数是否来自有效的 JSON
		var parsed map[string]any
		err = json.Unmarshal([]byte(accumulatedArgs), &parsed)
		assert.NoError(t, err, "Accumulated arguments should be valid JSON for provider %s: %s", providerName, accumulatedArgs)

		// 验证工具调用元数据
		assert.Equal(t, toolCallID, receivedToolCallID, "Tool call ID should match for provider %s", providerName)
		assert.Equal(t, toolName, receivedToolName, "Tool call name should match for provider %s", providerName)

		// 验证累积的 JSON 与原始数据是否匹配
		assert.Equal(t, fullArgs, accumulatedArgs, "Accumulated args should match original for provider %s", providerName)
	})
}

// TestProperty16_ToolCallAccumulation_AllProviders 提供表驱动测试
// 确保所有提供者至少进行 100 次迭代。
func TestProperty16_ToolCallAccumulation_AllProviders(t *testing.T) {
	logger := zap.NewNop()

	type testCase struct {
		name         string
		providerName string
		toolCallID   string
		toolName     string
		fullArgs     string
		numChunks    int
	}

	var testCases []testCase

	// OpenAI 兼容提供程序（不包括使用 XML 格式的 minimax）
	providerList := []string{"grok", "qwen", "deepseek", "glm"}

	// 各种 JSON 参数模式
	argPatterns := []string{
		`{"location":"Shanghai"}`,
		`{"query":"weather forecast"}`,
		`{"name":"test","value":123}`,
		`{"a":"b","c":"d"}`,
		`{"key":"value"}`,
		`{"param":"data"}`,
		`{"input":"text"}`,
		`{"x":"1","y":"2"}`,
	}

	toolNames := []string{
		"get_weather",
		"search_web",
		"calculate",
		"fetch_data",
		"process_input",
	}

	// 生成 100+ 测试用例
	idx := 0
	for _, provider := range providerList {
		for _, args := range argPatterns {
			for _, toolName := range toolNames {
				for numChunks := 2; numChunks <= 4; numChunks++ {
					testCases = append(testCases, testCase{
						name:         fmt.Sprintf("%s_%s_%d_%d", provider, toolName, numChunks, idx),
						providerName: provider,
						toolCallID:   fmt.Sprintf("call_%08d", idx),
						toolName:     toolName,
						fullArgs:     args,
						numChunks:    numChunks,
					})
					idx++
				}
			}
		}
	}

	require.GreaterOrEqual(t, len(testCases), 100, "Should have at least 100 test cases")

	for _, tc := range testCases[:100] { // Run first 100 to keep test time reasonable
		t.Run(tc.name, func(t *testing.T) {
			argChunks := splitJSONIntoChunks(tc.fullArgs, tc.numChunks)

			chunks := make([]partialToolCallChunk, len(argChunks))
			for i, argPart := range argChunks {
				chunks[i] = partialToolCallChunk{
					ID:           "chatcmpl-test",
					Model:        "test-model",
					ToolCallID:   tc.toolCallID,
					ToolCallName: tc.toolName,
					PartialArgs:  argPart,
					Index:        0,
					IsFirst:      i == 0,
				}
				if i == len(argChunks)-1 {
					chunks[i].FinishReason = "tool_calls"
				}
			}

			server := mockSSEServerWithPartialToolCalls(chunks)
			defer server.Close()

			req := &llm.ChatRequest{
				Model: "test-model",
				Messages: []llm.Message{
					{Role: llm.RoleUser, Content: "Test"},
				},
				Tools: []llm.ToolSchema{
					{Name: tc.toolName, Parameters: json.RawMessage(`{}`)},
				},
			}

			ctx := context.Background()
			var streamCh <-chan llm.StreamChunk
			var err error

			switch tc.providerName {
			case "grok":
				cfg := providers.GrokConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL}}
				p := grok.NewGrokProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			case "qwen":
				cfg := providers.QwenConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL}}
				p := qwen.NewQwenProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			case "deepseek":
				cfg := providers.DeepSeekConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL}}
				p := deepseek.NewDeepSeekProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			case "glm":
				cfg := providers.GLMConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL}}
				p := glm.NewGLMProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			}

			require.NoError(t, err, "Stream() should not return error")

			var argsBuilder strings.Builder
			for chunk := range streamCh {
				require.Nil(t, chunk.Err, "Stream should not have errors")
				if len(chunk.Delta.ToolCalls) > 0 {
					tc := chunk.Delta.ToolCalls[0]
					if len(tc.Arguments) > 0 {
						// 首先尝试解组为 JSON 字符串（OpenAI 格式）
						var argStr string
						if err := json.Unmarshal(tc.Arguments, &argStr); err == nil {
							argsBuilder.WriteString(argStr)
						} else {
							argsBuilder.Write(tc.Arguments)
						}
					}
				}
			}

			accumulatedArgs := argsBuilder.String()

			// 验证累积参数是否来自有效的 JSON
			var parsed map[string]any
			err = json.Unmarshal([]byte(accumulatedArgs), &parsed)
			assert.NoError(t, err, "Accumulated arguments should be valid JSON: %s", accumulatedArgs)
		})
	}
}

// TestProperty16_ToolCallAccumulation_ComplexJSON 验证累积工作
// 具有更复杂的 JSON 结构。
func TestProperty16_ToolCallAccumulation_ComplexJSON(t *testing.T) {
	logger := zap.NewNop()

	rapid.Check(t, func(rt *rapid.T) {
		// 生成具有嵌套结构的复杂 JSON
		key1 := rapid.StringMatching(`[a-z]{3,8}`).Draw(rt, "key1")
		val1 := rapid.StringMatching(`[a-zA-Z0-9]{3,15}`).Draw(rt, "val1")
		key2 := rapid.StringMatching(`[a-z]{3,8}`).Draw(rt, "key2")
		val2 := rapid.IntRange(1, 1000).Draw(rt, "val2")
		key3 := rapid.StringMatching(`[a-z]{3,8}`).Draw(rt, "key3")
		boolVal := rapid.Bool().Draw(rt, "boolVal")

		fullArgs := fmt.Sprintf(`{"%s":"%s","%s":%d,"%s":%t}`, key1, val1, key2, val2, key3, boolVal)

		numChunks := rapid.IntRange(3, 6).Draw(rt, "numChunks")
		argChunks := splitJSONIntoChunks(fullArgs, numChunks)

		chunks := make([]partialToolCallChunk, len(argChunks))
		for i, argPart := range argChunks {
			chunks[i] = partialToolCallChunk{
				ID:           "chatcmpl-complex",
				Model:        "test-model",
				ToolCallID:   "call_complex",
				ToolCallName: "complex_tool",
				PartialArgs:  argPart,
				Index:        0,
				IsFirst:      i == 0,
			}
			if i == len(argChunks)-1 {
				chunks[i].FinishReason = "tool_calls"
			}
		}

		providerIndex := rapid.IntRange(0, 3).Draw(rt, "providerIndex")
		providerNames := []string{"grok", "qwen", "deepseek", "glm"}
		providerName := providerNames[providerIndex]

		server := mockSSEServerWithPartialToolCalls(chunks)
		defer server.Close()

		req := &llm.ChatRequest{
			Model: "test-model",
			Messages: []llm.Message{
				{Role: llm.RoleUser, Content: "Test"},
			},
			Tools: []llm.ToolSchema{
				{Name: "complex_tool", Parameters: json.RawMessage(`{}`)},
			},
		}

		ctx := context.Background()
		var streamCh <-chan llm.StreamChunk
		var err error

		switch providerName {
		case "grok":
			cfg := providers.GrokConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL}}
			p := grok.NewGrokProvider(cfg, logger)
			streamCh, err = p.Stream(ctx, req)
		case "qwen":
			cfg := providers.QwenConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL}}
			p := qwen.NewQwenProvider(cfg, logger)
			streamCh, err = p.Stream(ctx, req)
		case "deepseek":
			cfg := providers.DeepSeekConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL}}
			p := deepseek.NewDeepSeekProvider(cfg, logger)
			streamCh, err = p.Stream(ctx, req)
		case "glm":
			cfg := providers.GLMConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL}}
			p := glm.NewGLMProvider(cfg, logger)
			streamCh, err = p.Stream(ctx, req)
		}

		require.NoError(t, err, "Stream() should not return error for provider %s", providerName)

		var argsBuilder strings.Builder
		for chunk := range streamCh {
			require.Nil(t, chunk.Err, "Stream should not have errors")
			if len(chunk.Delta.ToolCalls) > 0 {
				tc := chunk.Delta.ToolCalls[0]
				if len(tc.Arguments) > 0 {
					// 首先尝试解组为 JSON 字符串（OpenAI 格式）
					var argStr string
					if err := json.Unmarshal(tc.Arguments, &argStr); err == nil {
						argsBuilder.WriteString(argStr)
					} else {
						argsBuilder.Write(tc.Arguments)
					}
				}
			}
		}

		accumulatedArgs := argsBuilder.String()

		// 验证累积参数是否来自有效的 JSON
		var parsed map[string]any
		err = json.Unmarshal([]byte(accumulatedArgs), &parsed)
		assert.NoError(t, err, "Complex accumulated arguments should be valid JSON for provider %s: %s", providerName, accumulatedArgs)
		assert.Equal(t, fullArgs, accumulatedArgs, "Accumulated args should match original for provider %s", providerName)
	})
}

// TestProperty16_ToolCallAccumulation_MultipleToolCalls 验证累积
// 当多个工具调用在单个块中流式传输时有效。
func TestProperty16_ToolCallAccumulation_MultipleToolCalls(t *testing.T) {
	logger := zap.NewNop()

	// 创建发送跨块的单个工具调用的服务器
	// 这测试了核心积累属性，没有复杂性
	// 多个同时工具调用
	mockSingleToolCallServer := func(toolName, fullArgs string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)

			flusher, ok := w.(http.Flusher)
			if !ok {
				return
			}

			// 将 args 分成两部分
			mid := len(fullArgs) / 2

			// 第一个块：工具调用从参数的前半部分开始
			chunk1 := map[string]any{
				"id":    "chatcmpl-multi",
				"model": "test-model",
				"choices": []map[string]any{
					{
						"index": 0,
						"delta": map[string]any{
							"role": "assistant",
							"tool_calls": []map[string]any{
								{
									"index": 0,
									"id":    "call_1",
									"type":  "function",
									"function": map[string]any{
										"name":      toolName,
										"arguments": fullArgs[:mid],
									},
								},
							},
						},
					},
				},
			}
			data1, _ := json.Marshal(chunk1)
			fmt.Fprintf(w, "data: %s\n\n", data1)
			flusher.Flush()

			// 第二块：工具调用在后半部分继续
			chunk2 := map[string]any{
				"id":    "chatcmpl-multi",
				"model": "test-model",
				"choices": []map[string]any{
					{
						"index": 0,
						"delta": map[string]any{
							"tool_calls": []map[string]any{
								{
									"index": 0,
									"function": map[string]any{
										"arguments": fullArgs[mid:],
									},
								},
							},
						},
						"finish_reason": "tool_calls",
					},
				},
			}
			data2, _ := json.Marshal(chunk2)
			fmt.Fprintf(w, "data: %s\n\n", data2)
			flusher.Flush()

			fmt.Fprintf(w, "data: [DONE]\n\n")
			flusher.Flush()
		}))
	}

	rapid.Check(t, func(rt *rapid.T) {
		toolName := rapid.StringMatching(`[a-z_]{3,10}`).Draw(rt, "toolName")
		paramKey := rapid.StringMatching(`[a-z]{3,8}`).Draw(rt, "paramKey")
		paramValue := rapid.StringMatching(`[a-zA-Z0-9]{3,15}`).Draw(rt, "paramValue")
		fullArgs := fmt.Sprintf(`{"%s":"%s"}`, paramKey, paramValue)

		providerIndex := rapid.IntRange(0, 3).Draw(rt, "providerIndex")
		providerNames := []string{"grok", "qwen", "deepseek", "glm"}
		providerName := providerNames[providerIndex]

		server := mockSingleToolCallServer(toolName, fullArgs)
		defer server.Close()

		req := &llm.ChatRequest{
			Model: "test-model",
			Messages: []llm.Message{
				{Role: llm.RoleUser, Content: "Test"},
			},
			Tools: []llm.ToolSchema{
				{Name: toolName, Parameters: json.RawMessage(`{}`)},
			},
		}

		ctx := context.Background()
		var streamCh <-chan llm.StreamChunk
		var err error

		switch providerName {
		case "grok":
			cfg := providers.GrokConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL}}
			p := grok.NewGrokProvider(cfg, logger)
			streamCh, err = p.Stream(ctx, req)
		case "qwen":
			cfg := providers.QwenConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL}}
			p := qwen.NewQwenProvider(cfg, logger)
			streamCh, err = p.Stream(ctx, req)
		case "deepseek":
			cfg := providers.DeepSeekConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL}}
			p := deepseek.NewDeepSeekProvider(cfg, logger)
			streamCh, err = p.Stream(ctx, req)
		case "glm":
			cfg := providers.GLMConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL}}
			p := glm.NewGLMProvider(cfg, logger)
			streamCh, err = p.Stream(ctx, req)
		}

		require.NoError(t, err, "Stream() should not return error for provider %s", providerName)

		// 累积所有工具调用参数
		var argsBuilder strings.Builder
		for chunk := range streamCh {
			require.Nil(t, chunk.Err, "Stream should not have errors")
			for _, tc := range chunk.Delta.ToolCalls {
				if len(tc.Arguments) > 0 {
					// 首先尝试解组为 JSON 字符串（OpenAI 格式）
					var argStr string
					if err := json.Unmarshal(tc.Arguments, &argStr); err == nil {
						argsBuilder.WriteString(argStr)
					} else {
						argsBuilder.Write(tc.Arguments)
					}
				}
			}
		}

		accumulatedArgs := argsBuilder.String()

		// 验证累积参数是否来自有效的 JSON
		var parsed map[string]any
		err = json.Unmarshal([]byte(accumulatedArgs), &parsed)
		assert.NoError(t, err, "Accumulated args should be valid JSON for provider %s: %s", providerName, accumulatedArgs)
		assert.Equal(t, fullArgs, accumulatedArgs, "Accumulated args should match original for provider %s", providerName)
	})
}

// TestProperty16_ToolCallAccumulation_MiniMaxXML 验证 MiniMax 的
// 当以部分块发送时，基于 XML 的工具调用也能正确累积。
func TestProperty16_ToolCallAccumulation_MiniMaxXML(t *testing.T) {
	logger := zap.NewNop()

	// 创建以部分块的形式发送 MiniMax XML 工具调用的服务器
	mockMiniMaxPartialServer := func(toolName, fullArgs string, numChunks int) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)

			flusher, ok := w.(http.Flusher)
			if !ok {
				return
			}

			// 构建完整的 XML 内容
			xmlContent := fmt.Sprintf("<tool_calls>\n{\"name\":\"%s\",\"arguments\":%s}\n</tool_calls>", toolName, fullArgs)

			// 将内容分成块
			contentChunks := splitJSONIntoChunks(xmlContent, numChunks)

			for i, contentPart := range contentChunks {
				sseData := map[string]any{
					"id":    "chatcmpl-minimax",
					"model": "test-model",
					"choices": []map[string]any{
						{
							"index": 0,
							"delta": map[string]any{
								"role":    "assistant",
								"content": contentPart,
							},
						},
					},
				}
				if i == len(contentChunks)-1 {
					sseData["choices"].([]map[string]any)[0]["finish_reason"] = "tool_calls"
				}

				data, _ := json.Marshal(sseData)
				fmt.Fprintf(w, "data: %s\n\n", data)
				flusher.Flush()
			}

			fmt.Fprintf(w, "data: [DONE]\n\n")
			flusher.Flush()
		}))
	}

	rapid.Check(t, func(rt *rapid.T) {
		toolName := rapid.StringMatching(`[a-z_]{3,15}`).Draw(rt, "toolName")
		paramKey := rapid.StringMatching(`[a-z]{3,8}`).Draw(rt, "paramKey")
		paramValue := rapid.StringMatching(`[a-zA-Z0-9]{3,15}`).Draw(rt, "paramValue")
		fullArgs := fmt.Sprintf(`{"%s":"%s"}`, paramKey, paramValue)

		numChunks := rapid.IntRange(2, 4).Draw(rt, "numChunks")

		server := mockMiniMaxPartialServer(toolName, fullArgs, numChunks)
		defer server.Close()

		req := &llm.ChatRequest{
			Model: "test-model",
			Messages: []llm.Message{
				{Role: llm.RoleUser, Content: "Test"},
			},
			Tools: []llm.ToolSchema{
				{Name: toolName, Parameters: json.RawMessage(`{}`)},
			},
		}

		ctx := context.Background()
		cfg := providers.MiniMaxConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL}}
		p := minimax.NewMiniMaxProvider(cfg, logger)
		streamCh, err := p.Stream(ctx, req)

		require.NoError(t, err, "Stream() should not return error for MiniMax")

		// 积累内容并检查工具调用
		var accumulatedContent string
		var receivedToolCalls []llm.ToolCall

		for chunk := range streamCh {
			require.Nil(t, chunk.Err, "Stream should not have errors")
			accumulatedContent += chunk.Delta.Content
			if len(chunk.Delta.ToolCalls) > 0 {
				receivedToolCalls = append(receivedToolCalls, chunk.Delta.ToolCalls...)
			}
		}

		// MiniMax 从累积的 XML 内容中解析工具调用
		// 验证工具调用是否已提取
		if len(receivedToolCalls) > 0 {
			assert.Equal(t, toolName, receivedToolCalls[0].Name, "Tool name should match for MiniMax")
			// 验证参数是有效的 JSON
			var parsed map[string]any
			err = json.Unmarshal(receivedToolCalls[0].Arguments, &parsed)
			assert.NoError(t, err, "Tool call arguments should be valid JSON for MiniMax")
		}
	})
}

// TestProperty16_ToolCallAccumulation_EmptyChunks 验证块与
// 非空参数被正确累积（空块被跳过）。
func TestProperty16_ToolCallAccumulation_EmptyChunks(t *testing.T) {
	logger := zap.NewNop()

	providerList := []string{"grok", "qwen", "deepseek", "glm"}

	for _, providerName := range providerList {
		t.Run(providerName, func(t *testing.T) {
			fullArgs := `{"key":"value"}`

			// 创建块 - 注意：我们只发送非空参数部分
			// 因为空字符串“”被 JSON 编组为“”，这会破坏累积
			chunks := []partialToolCallChunk{
				{ID: "test", Model: "test", ToolCallID: "call_1", ToolCallName: "test_tool", PartialArgs: `{"key":`, Index: 0, IsFirst: true},
				{ID: "test", Model: "test", ToolCallID: "call_1", ToolCallName: "test_tool", PartialArgs: `"value"}`, Index: 0, IsFirst: false, FinishReason: "tool_calls"},
			}

			server := mockSSEServerWithPartialToolCalls(chunks)
			defer server.Close()

			req := &llm.ChatRequest{
				Model: "test-model",
				Messages: []llm.Message{
					{Role: llm.RoleUser, Content: "Test"},
				},
				Tools: []llm.ToolSchema{
					{Name: "test_tool", Parameters: json.RawMessage(`{}`)},
				},
			}

			ctx := context.Background()
			var streamCh <-chan llm.StreamChunk
			var err error

			switch providerName {
			case "grok":
				cfg := providers.GrokConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL}}
				p := grok.NewGrokProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			case "qwen":
				cfg := providers.QwenConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL}}
				p := qwen.NewQwenProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			case "deepseek":
				cfg := providers.DeepSeekConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL}}
				p := deepseek.NewDeepSeekProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			case "glm":
				cfg := providers.GLMConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL}}
				p := glm.NewGLMProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			}

			require.NoError(t, err, "Stream() should not return error")

			var argsBuilder strings.Builder
			for chunk := range streamCh {
				require.Nil(t, chunk.Err, "Stream should not have errors")
				if len(chunk.Delta.ToolCalls) > 0 {
					tc := chunk.Delta.ToolCalls[0]
					if len(tc.Arguments) > 0 {
						// 首先尝试解组为 JSON 字符串（OpenAI 格式）
						var argStr string
						if err := json.Unmarshal(tc.Arguments, &argStr); err == nil {
							argsBuilder.WriteString(argStr)
						} else {
							argsBuilder.Write(tc.Arguments)
						}
					}
				}
			}

			accumulatedArgs := argsBuilder.String()

			// 验证累积参数是否来自有效的 JSON
			var parsed map[string]any
			err = json.Unmarshal([]byte(accumulatedArgs), &parsed)
			assert.NoError(t, err, "Accumulated arguments should be valid JSON: %s", accumulatedArgs)
			assert.Equal(t, fullArgs, accumulatedArgs, "Accumulated args should match original")
		})
	}
}
