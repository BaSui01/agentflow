package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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

// 特性：多提供商支持，属性 14：SSE 响应解析
// **验证：要求 10.2、10.3**
//
// This 属性测试验证对于任何提供商流响应，
// 当接收到以“data:”开头的 SSE 数据行时，提供商应
// 解析 JSON 内容并发出相应的 StreamChunk 消息。

// sseChunkData 表示单个 SSE 块的数据
type sseChunkData struct {
	ID           string
	Model        string
	Content      string
	FinishReason string
}

// mockSSEServer 创建一个返回 SSE 格式响应的测试服务器
func mockSSEServer(chunks []sseChunkData) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			return
		}

		// 将每个块作为 SSE 数据线发送
		for i, chunk := range chunks {
			sseData := map[string]any{
				"id":    chunk.ID,
				"model": chunk.Model,
				"choices": []map[string]any{
					{
						"index": 0,
						"delta": map[string]any{
							"role":    "assistant",
							"content": chunk.Content,
						},
					},
				},
			}
			if i == len(chunks)-1 && chunk.FinishReason != "" {
				sseData["choices"].([]map[string]any)[0]["finish_reason"] = chunk.FinishReason
			}

			data, _ := json.Marshal(sseData)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}

		// 发送 [DONE] 标记
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
}

// TestProperty14_SSEResponseParsing 验证SSE数据线是否正确
// 解析为所有提供者的 StreamChunk 消息。
func TestProperty14_SSEResponseParsing(t *testing.T) {
	logger := zap.NewNop()

	rapid.Check(t, func(rt *rapid.T) {
		// 生成随机 SSE 块
		numChunks := rapid.IntRange(1, 5).Draw(rt, "numChunks")
		chunks := make([]sseChunkData, numChunks)
		expectedContents := make([]string, numChunks)

		for i := range numChunks {
			content := rapid.StringMatching(`[a-zA-Z0-9 ]{1,20}`).Draw(rt, fmt.Sprintf("content_%d", i))
			chunks[i] = sseChunkData{
				ID:      fmt.Sprintf("chatcmpl-%s", rapid.StringMatching(`[a-z0-9]{8}`).Draw(rt, fmt.Sprintf("id_%d", i))),
				Model:   rapid.StringMatching(`[a-z0-9-]{3,15}`).Draw(rt, fmt.Sprintf("model_%d", i)),
				Content: content,
			}
			expectedContents[i] = content
			if i == numChunks-1 {
				chunks[i].FinishReason = "stop"
			}
		}

		// 选择随机提供商
		providerIndex := rapid.IntRange(0, 4).Draw(rt, "providerIndex")
		providerNames := []string{"grok", "qwen", "deepseek", "glm", "minimax"}
		providerName := providerNames[providerIndex]

		server := mockSSEServer(chunks)
		defer server.Close()

		req := &llm.ChatRequest{
			Model: "test-model",
			Messages: []llm.Message{
				{Role: llm.RoleUser, Content: "Test message"},
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
		case "minimax":
			cfg := providers.MiniMaxConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL}}
			p := minimax.NewMiniMaxProvider(cfg, logger)
			streamCh, err = p.Stream(ctx, req)
		}

		require.NoError(t, err, "Stream() should not return error for provider %s", providerName)

		// 从流中收集所有块
		var receivedContents []string
		for chunk := range streamCh {
			require.Nil(t, chunk.Err, "Stream should not have errors for provider %s", providerName)
			if chunk.Delta.Content != "" {
				receivedContents = append(receivedContents, chunk.Delta.Content)
			}
		}

		// 验证所有 SSE 数据行均已解析并作为 StreamChunk 发出
		require.Equal(t, len(expectedContents), len(receivedContents),
			"Should receive same number of chunks as sent for provider %s", providerName)

		for i, expected := range expectedContents {
			assert.Equal(t, expected, receivedContents[i],
				"Content at index %d should match for provider %s", i, providerName)
		}
	})
}

// TestProperty14_SSEResponseParsing_AllProviders 提供表驱动测试
// 确保所有提供者至少进行 100 次迭代。
func TestProperty14_SSEResponseParsing_AllProviders(t *testing.T) {
	logger := zap.NewNop()

	type testCase struct {
		name         string
		providerName string
		chunks       []sseChunkData
	}

	var testCases []testCase

	providerList := []string{"grok", "qwen", "deepseek", "glm", "minimax"}
	contentVariants := []string{
		"Hello", "World", "Test", "Response", "Chunk",
		"Data", "Stream", "Parse", "JSON", "SSE",
		"Message", "Content", "Delta", "Model", "Provider",
		"API", "Request", "Reply", "Token", "Complete",
	}

	// 生成 100+ 测试用例
	idx := 0
	for _, provider := range providerList {
		for i, content := range contentVariants {
			// 单块测试
			testCases = append(testCases, testCase{
				name:         fmt.Sprintf("%s_single_%d", provider, idx),
				providerName: provider,
				chunks: []sseChunkData{
					{ID: fmt.Sprintf("id_%d", idx), Model: "test-model", Content: content, FinishReason: "stop"},
				},
			})
			idx++

			// 多块测试（2块）
			if i+1 < len(contentVariants) {
				testCases = append(testCases, testCase{
					name:         fmt.Sprintf("%s_multi_%d", provider, idx),
					providerName: provider,
					chunks: []sseChunkData{
						{ID: fmt.Sprintf("id_%d", idx), Model: "test-model", Content: content},
						{ID: fmt.Sprintf("id_%d", idx), Model: "test-model", Content: contentVariants[i+1], FinishReason: "stop"},
					},
				})
				idx++
			}
		}
	}

	// 确保我们至少有 100 个测试用例
	require.GreaterOrEqual(t, len(testCases), 100, "Should have at least 100 test cases")

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := mockSSEServer(tc.chunks)
			defer server.Close()

			req := &llm.ChatRequest{
				Model: "test-model",
				Messages: []llm.Message{
					{Role: llm.RoleUser, Content: "Test"},
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
			case "minimax":
				cfg := providers.MiniMaxConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL}}
				p := minimax.NewMiniMaxProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			}

			require.NoError(t, err, "Stream() should not return error")

			var receivedContents []string
			for chunk := range streamCh {
				require.Nil(t, chunk.Err, "Stream should not have errors")
				if chunk.Delta.Content != "" {
					receivedContents = append(receivedContents, chunk.Delta.Content)
				}
			}

			// 验证所有块都已解析
			require.Equal(t, len(tc.chunks), len(receivedContents), "Should receive all chunks")
			for i, expected := range tc.chunks {
				assert.Equal(t, expected.Content, receivedContents[i], "Content should match at index %d", i)
			}
		})
	}
}

// TestProperty14_SSEResponseParsing_DataLineFormat 验证只有行
// 以“data:”开头的被解析为 SSE 事件。
func TestProperty14_SSEResponseParsing_DataLineFormat(t *testing.T) {
	logger := zap.NewNop()

	// 创建一个发送各种线路格式的服务器
	mockServerWithFormats := func(validChunks []sseChunkData) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)

			flusher, ok := w.(http.Flusher)
			if !ok {
				return
			}

			// 发送一些非数据线（应该被忽略）
			fmt.Fprintf(w, ": this is a comment\n\n")
			flusher.Flush()

			fmt.Fprintf(w, "event: message\n\n")
			flusher.Flush()

			// 发送有效数据线
			for i, chunk := range validChunks {
				sseData := map[string]any{
					"id":    chunk.ID,
					"model": chunk.Model,
					"choices": []map[string]any{
						{
							"index": 0,
							"delta": map[string]any{
								"role":    "assistant",
								"content": chunk.Content,
							},
						},
					},
				}
				if i == len(validChunks)-1 {
					sseData["choices"].([]map[string]any)[0]["finish_reason"] = "stop"
				}

				data, _ := json.Marshal(sseData)
				fmt.Fprintf(w, "data: %s\n\n", data)
				flusher.Flush()
			}

			// 发送空行（应被忽略）
			fmt.Fprintf(w, "\n\n")
			flusher.Flush()

			fmt.Fprintf(w, "data: [DONE]\n\n")
			flusher.Flush()
		}))
	}

	rapid.Check(t, func(rt *rapid.T) {
		numChunks := rapid.IntRange(1, 3).Draw(rt, "numChunks")
		chunks := make([]sseChunkData, numChunks)

		for i := range numChunks {
			chunks[i] = sseChunkData{
				ID:      fmt.Sprintf("id_%d", i),
				Model:   "test-model",
				Content: rapid.StringMatching(`[a-zA-Z]{3,10}`).Draw(rt, fmt.Sprintf("content_%d", i)),
			}
		}

		providerIndex := rapid.IntRange(0, 4).Draw(rt, "providerIndex")
		providerNames := []string{"grok", "qwen", "deepseek", "glm", "minimax"}
		providerName := providerNames[providerIndex]

		server := mockServerWithFormats(chunks)
		defer server.Close()

		req := &llm.ChatRequest{
			Model: "test-model",
			Messages: []llm.Message{
				{Role: llm.RoleUser, Content: "Test"},
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
		case "minimax":
			cfg := providers.MiniMaxConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL}}
			p := minimax.NewMiniMaxProvider(cfg, logger)
			streamCh, err = p.Stream(ctx, req)
		}

		require.NoError(t, err, "Stream() should not return error for provider %s", providerName)

		var receivedContents []string
		for chunk := range streamCh {
			require.Nil(t, chunk.Err, "Stream should not have errors")
			if chunk.Delta.Content != "" {
				receivedContents = append(receivedContents, chunk.Delta.Content)
			}
		}

		// 仅应解析有效的数据行
		require.Equal(t, len(chunks), len(receivedContents),
			"Should only receive chunks from valid data: lines for provider %s", providerName)
	})
}

// TestProperty14_SSEResponseParsing_JSONContent 验证 SSE 数据行中的 JSON 内容可被正确解析。
// SSE 中的数据行被正确解析为 StreamChunk 字段。
func TestProperty14_SSEResponseParsing_JSONContent(t *testing.T) {
	logger := zap.NewNop()

	rapid.Check(t, func(rt *rapid.T) {
		// 生成随机块数据
		chunkID := rapid.StringMatching(`chatcmpl-[a-z0-9]{8}`).Draw(rt, "chunkID")
		chunkModel := rapid.StringMatching(`[a-z0-9-]{3,15}`).Draw(rt, "chunkModel")
		chunkContent := rapid.StringMatching(`[a-zA-Z0-9 ]{1,30}`).Draw(rt, "chunkContent")

		chunks := []sseChunkData{
			{ID: chunkID, Model: chunkModel, Content: chunkContent, FinishReason: "stop"},
		}

		providerIndex := rapid.IntRange(0, 4).Draw(rt, "providerIndex")
		providerNames := []string{"grok", "qwen", "deepseek", "glm", "minimax"}
		providerName := providerNames[providerIndex]

		server := mockSSEServer(chunks)
		defer server.Close()

		req := &llm.ChatRequest{
			Model: "test-model",
			Messages: []llm.Message{
				{Role: llm.RoleUser, Content: "Test"},
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
		case "minimax":
			cfg := providers.MiniMaxConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL}}
			p := minimax.NewMiniMaxProvider(cfg, logger)
			streamCh, err = p.Stream(ctx, req)
		}

		require.NoError(t, err, "Stream() should not return error for provider %s", providerName)

		var receivedChunks []llm.StreamChunk
		for chunk := range streamCh {
			require.Nil(t, chunk.Err, "Stream should not have errors")
			receivedChunks = append(receivedChunks, chunk)
		}

		// 验证 JSON 字段是否已正确解析
		require.Len(t, receivedChunks, 1, "Should receive exactly one chunk")
		chunk := receivedChunks[0]

		assert.Equal(t, chunkID, chunk.ID, "Chunk ID should be parsed from JSON for provider %s", providerName)
		assert.Equal(t, chunkModel, chunk.Model, "Chunk Model should be parsed from JSON for provider %s", providerName)
		assert.Equal(t, chunkContent, chunk.Delta.Content, "Chunk Content should be parsed from JSON for provider %s", providerName)
		assert.Equal(t, "stop", chunk.FinishReason, "FinishReason should be parsed from JSON for provider %s", providerName)
	})
}

// TestProperty14_SSEResponseParsing_WithToolCalls 验证 SSE 的回应
// 包含的工具调用已正确解析为 OpenAI 兼容的提供程序。
// 注意：MiniMax 使用基于 XML 的工具调用并单独进行测试。
func TestProperty14_SSEResponseParsing_WithToolCalls(t *testing.T) {
	logger := zap.NewNop()

	// 创建一个通过工具调用返回 SSE 的服务器（OpenAI 格式）
	mockSSEServerWithToolCalls := func(toolCallID, toolName, toolArgs string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)

			flusher, ok := w.(http.Flusher)
			if !ok {
				return
			}

			sseData := map[string]any{
				"id":    "chatcmpl-test",
				"model": "test-model",
				"choices": []map[string]any{
					{
						"index": 0,
						"delta": map[string]any{
							"role": "assistant",
							"tool_calls": []map[string]any{
								{
									"id":   toolCallID,
									"type": "function",
									"function": map[string]any{
										"name":      toolName,
										"arguments": toolArgs,
									},
								},
							},
						},
						"finish_reason": "tool_calls",
					},
				},
			}

			data, _ := json.Marshal(sseData)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()

			fmt.Fprintf(w, "data: [DONE]\n\n")
			flusher.Flush()
		}))
	}

	rapid.Check(t, func(rt *rapid.T) {
		toolCallID := rapid.StringMatching(`call_[a-z0-9]{8}`).Draw(rt, "toolCallID")
		toolName := rapid.StringMatching(`[a-z_]{3,15}`).Draw(rt, "toolName")
		toolArgs := fmt.Sprintf(`{"param": "%s"}`, rapid.StringMatching(`[a-z]{3,10}`).Draw(rt, "paramValue"))

		// 仅测试 OpenAI 兼容的提供程序（不包括使用 XML 格式的 minimax）
		providerIndex := rapid.IntRange(0, 3).Draw(rt, "providerIndex")
		providerNames := []string{"grok", "qwen", "deepseek", "glm"}
		providerName := providerNames[providerIndex]

		server := mockSSEServerWithToolCalls(toolCallID, toolName, toolArgs)
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

		var receivedToolCalls []llm.ToolCall
		for chunk := range streamCh {
			require.Nil(t, chunk.Err, "Stream should not have errors")
			if len(chunk.Delta.ToolCalls) > 0 {
				receivedToolCalls = append(receivedToolCalls, chunk.Delta.ToolCalls...)
			}
		}

		// 验证工具调用是从 SSE 解析的
		require.Len(t, receivedToolCalls, 1, "Should receive one tool call for provider %s", providerName)
		assert.Equal(t, toolCallID, receivedToolCalls[0].ID, "Tool call ID should match for provider %s", providerName)
		assert.Equal(t, toolName, receivedToolCalls[0].Name, "Tool call name should match for provider %s", providerName)
	})
}

// TestProperty14_SSEResponseParsing_MiniMaxXMLToolCalls 验证 MiniMax 的
// SSE 响应中基于 XML 的工具调用已正确解析。
func TestProperty14_SSEResponseParsing_MiniMaxXMLToolCalls(t *testing.T) {
	logger := zap.NewNop()

	// 创建一个使用 MiniMax XML 工具调用返回 SSE 的服务器
	mockMiniMaxSSEServerWithToolCalls := func(toolName, toolArgs string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)

			flusher, ok := w.(http.Flusher)
			if !ok {
				return
			}

			// MiniMax 在内容中使用 XML 格式进行工具调用
			xmlContent := fmt.Sprintf("<tool_calls>\n{\"name\":\"%s\",\"arguments\":%s}\n</tool_calls>", toolName, toolArgs)

			sseData := map[string]any{
				"id":    "chatcmpl-test",
				"model": "test-model",
				"choices": []map[string]any{
					{
						"index": 0,
						"delta": map[string]any{
							"role":    "assistant",
							"content": xmlContent,
						},
						"finish_reason": "tool_calls",
					},
				},
			}

			data, _ := json.Marshal(sseData)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()

			fmt.Fprintf(w, "data: [DONE]\n\n")
			flusher.Flush()
		}))
	}

	rapid.Check(t, func(rt *rapid.T) {
		toolName := rapid.StringMatching(`[a-z_]{3,15}`).Draw(rt, "toolName")
		toolArgs := fmt.Sprintf(`{"param": "%s"}`, rapid.StringMatching(`[a-z]{3,10}`).Draw(rt, "paramValue"))

		server := mockMiniMaxSSEServerWithToolCalls(toolName, toolArgs)
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

		var receivedToolCalls []llm.ToolCall
		for chunk := range streamCh {
			require.Nil(t, chunk.Err, "Stream should not have errors")
			if len(chunk.Delta.ToolCalls) > 0 {
				receivedToolCalls = append(receivedToolCalls, chunk.Delta.ToolCalls...)
			}
		}

		// 验证工具调用是从 SSE 中的 XML 解析的
		require.Len(t, receivedToolCalls, 1, "Should receive one tool call for MiniMax")
		assert.Equal(t, toolName, receivedToolCalls[0].Name, "Tool call name should match for MiniMax")
	})
}
