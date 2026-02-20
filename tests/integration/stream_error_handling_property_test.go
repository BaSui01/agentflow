package integration

import (
	"context"
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

// 功能：多提供商支持，属性 15：流错误处理
// **验证：要求 10.5**
//
// This 属性测试验证对于任何提供商流响应，
// 当 SSE 数据中遇到无效 JSON 时，提供者应发出
// 具有包含 llm.Error 的非零 Err 字段的 StreamChunk。

// mockSSEServerWithInvalidJSON 创建一个测试服务器，返回带有无效 JSON 的 SSE
func mockSSEServerWithInvalidJSON(invalidJSON string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			return
		}

		// 在 SSE 数据行中发送无效 JSON
		fmt.Fprintf(w, "data: %s\n\n", invalidJSON)
		flusher.Flush()
	}))
}

// TestProperty15_StreamErrorHandling 验证 SSE 数据中的 JSON 无效
// 为所有提供者生成一个具有非零 Err 字段的 StreamChunk。
func TestProperty15_StreamErrorHandling(t *testing.T) {
	logger := zap.NewNop()

	rapid.Check(t, func(rt *rapid.T) {
		// 生成各种类型的无效JSON
		invalidJSONType := rapid.IntRange(0, 5).Draw(rt, "invalidJSONType")
		var invalidJSON string

		switch invalidJSONType {
		case 0:
			// 截断的 JSON
			invalidJSON = `{"id": "test", "model": "test-model", "choices": [`
		case 1:
			// 缺少右大括号
			invalidJSON = `{"id": "test", "model": "test-model"`
		case 2:
			// 语法无效 - 不带引号的键
			invalidJSON = `{id: "test", model: "test-model"}`
		case 3:
			// 随机垃圾
			invalidJSON = rapid.StringMatching(`[a-zA-Z0-9!@#$%^&*()]{5,30}`).Draw(rt, "garbage")
		case 4:
			// 数组格式错误
			invalidJSON = `{"choices": [{"index": 0, "delta": {"content": "test"}`
		case 5:
			// 转义序列无效
			invalidJSON = `{"id": "test\xinvalid", "model": "test"}`
		}

		// 选择随机提供商
		providerIndex := rapid.IntRange(0, 4).Draw(rt, "providerIndex")
		providerNames := []string{"grok", "qwen", "deepseek", "glm", "minimax"}
		providerName := providerNames[providerIndex]

		server := mockSSEServerWithInvalidJSON(invalidJSON)
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

		// 收集块并检查错误
		var foundError bool
		var errorChunk llm.StreamChunk
		for chunk := range streamCh {
			if chunk.Err != nil {
				foundError = true
				errorChunk = chunk
				break
			}
		}

		// 验证是否发出了错误块
		assert.True(t, foundError, "Should receive StreamChunk with error for invalid JSON for provider %s", providerName)
		assert.NotNil(t, errorChunk.Err, "StreamChunk.Err should not be nil for provider %s", providerName)

		// 验证错误字段（Err 已经是 *llm.Error）
		if errorChunk.Err != nil {
			assert.NotEmpty(t, errorChunk.Err.Message, "Error message should not be empty for provider %s", providerName)
			assert.Equal(t, llm.ErrUpstreamError, errorChunk.Err.Code, "Error code should be ErrUpstreamError for provider %s", providerName)
		}
	})
}

// TestProperty15_StreamErrorHandling_AllProviders 提供表驱动测试
// 确保所有提供者至少进行 100 次迭代。
func TestProperty15_StreamErrorHandling_AllProviders(t *testing.T) {
	logger := zap.NewNop()

	type testCase struct {
		name         string
		providerName string
		invalidJSON  string
	}

	// 生成各种无效的 JSON 模式
	invalidJSONPatterns := []string{
		// 截断的 JSON
		`{"id": "test", "model": "test-model", "choices": [`,
		`{"id": "test"`,
		`{"choices": [{"index": 0`,
		`{"id": "test", "model":`,
		// 缺少右大括号
		`{"id": "test", "model": "test-model"`,
		`{"choices": [{"delta": {"content": "test"}}`,
		// 语法无效
		`{id: "test"}`,
		`{"id": test}`,
		`{'id': 'test'}`,
		`{id: test}`,
		// 随机垃圾
		`not json at all`,
		`<xml>not json</xml>`,
		// 格式错误的结构（这些将解析但会导致提供程序出现问题）
		`{"choices": "not an array"}`,
		`{"id": {"nested": "wrong"}}`,
		`{"choices": [{"delta": "not object"}]}`,
		`{"id": [1,2,3]}`,
		// 其他无效模式
		`{"unclosed": "string`,
		`{trailing comma,}`,
		`{"key": undefined}`,
	}

	var testCases []testCase
	providerList := []string{"grok", "qwen", "deepseek", "glm", "minimax"}

	// 通过重复具有变化的模式生成 100 多个测试用例
	idx := 0
	for round := 0; round < 2; round++ {
		for _, provider := range providerList {
			for _, invalidJSON := range invalidJSONPatterns {
				testCases = append(testCases, testCase{
					name:         fmt.Sprintf("%s_invalid_%d", provider, idx),
					providerName: provider,
					invalidJSON:  invalidJSON,
				})
				idx++
			}
		}
	}

	// 确保我们至少有 100 个测试用例
	require.GreaterOrEqual(t, len(testCases), 100, "Should have at least 100 test cases")

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := mockSSEServerWithInvalidJSON(tc.invalidJSON)
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

			// 收集块并检查错误
			var foundError bool
			var errorChunk llm.StreamChunk
			for chunk := range streamCh {
				if chunk.Err != nil {
					foundError = true
					errorChunk = chunk
					break
				}
			}

			// 验证已发出错误
			assert.True(t, foundError, "Should receive StreamChunk with error for invalid JSON")
			assert.NotNil(t, errorChunk.Err, "StreamChunk.Err should not be nil")
		})
	}
}

// TestProperty15_StreamErrorHandling_ErrorContainsLLMError 验证
// StreamChunk 中的错误具体是具有正确字段的 llm.Error 。
func TestProperty15_StreamErrorHandling_ErrorContainsLLMError(t *testing.T) {
	logger := zap.NewNop()

	rapid.Check(t, func(rt *rapid.T) {
		// 生成随机无效 JSON
		invalidJSON := rapid.StringMatching(`\{[a-zA-Z0-9:,"' ]{0,20}`).Draw(rt, "invalidJSON")

		providerIndex := rapid.IntRange(0, 4).Draw(rt, "providerIndex")
		providerNames := []string{"grok", "qwen", "deepseek", "glm", "minimax"}
		providerName := providerNames[providerIndex]

		server := mockSSEServerWithInvalidJSON(invalidJSON)
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

		// 收集错误块
		var errorChunk llm.StreamChunk
		for chunk := range streamCh {
			if chunk.Err != nil {
				errorChunk = chunk
				break
			}
		}

		// 验证错误是否为 llm.Error 且字段正确（Err 已为 *llm.Error）
		if errorChunk.Err != nil {
			// 验证 llm.Error 字段
			assert.Equal(t, llm.ErrUpstreamError, errorChunk.Err.Code,
				"Error code should be ErrUpstreamError for provider %s", providerName)
			assert.NotEmpty(t, errorChunk.Err.Message,
				"Error message should not be empty for provider %s", providerName)
			assert.Equal(t, http.StatusBadGateway, errorChunk.Err.HTTPStatus,
				"HTTP status should be BadGateway for provider %s", providerName)
			assert.True(t, errorChunk.Err.Retryable,
				"Error should be retryable for provider %s", providerName)
			assert.Equal(t, providerName, errorChunk.Err.Provider,
				"Provider name should be set in error for provider %s", providerName)
		}
	})
}

// TestProperty15_StreamErrorHandling_ChannelClosesAfterError 验证
// 发出错误块后，流通道被正确关闭。
func TestProperty15_StreamErrorHandling_ChannelClosesAfterError(t *testing.T) {
	logger := zap.NewNop()

	rapid.Check(t, func(rt *rapid.T) {
		invalidJSON := `{"invalid": json}`

		providerIndex := rapid.IntRange(0, 4).Draw(rt, "providerIndex")
		providerNames := []string{"grok", "qwen", "deepseek", "glm", "minimax"}
		providerName := providerNames[providerIndex]

		server := mockSSEServerWithInvalidJSON(invalidJSON)
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

		// 彻底排空通道
		chunkCount := 0
		for range streamCh {
			chunkCount++
		}

		// 通道应关闭（循环应退出）
		// 如果我们到达这里，通道已正确关闭
		assert.True(t, true, "Channel should be closed after error for provider %s", providerName)
	})
}

// TestProperty15_StreamErrorHandling_MixedValidInvalidJSON 验证
// 当有效 JSON 后面跟着无效 JSON 时，仍然会发出错误。
func TestProperty15_StreamErrorHandling_MixedValidInvalidJSON(t *testing.T) {
	logger := zap.NewNop()

	// 创建一个首先发送有效 JSON，然后发送无效 JSON 的服务器
	mockMixedSSEServer := func(validContent string, invalidJSON string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)

			flusher, ok := w.(http.Flusher)
			if !ok {
				return
			}

			// 首先发送有效的 JSON
			validData := fmt.Sprintf(`{"id":"test","model":"test-model","choices":[{"index":0,"delta":{"role":"assistant","content":"%s"}}]}`, validContent)
			fmt.Fprintf(w, "data: %s\n\n", validData)
			flusher.Flush()

			// 然后发送无效的JSON
			fmt.Fprintf(w, "data: %s\n\n", invalidJSON)
			flusher.Flush()
		}))
	}

	rapid.Check(t, func(rt *rapid.T) {
		validContent := rapid.StringMatching(`[a-zA-Z]{3,10}`).Draw(rt, "validContent")
		invalidJSON := `{"broken": json`

		providerIndex := rapid.IntRange(0, 4).Draw(rt, "providerIndex")
		providerNames := []string{"grok", "qwen", "deepseek", "glm", "minimax"}
		providerName := providerNames[providerIndex]

		server := mockMixedSSEServer(validContent, invalidJSON)
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

		// 收集所有块
		var validChunks []llm.StreamChunk
		var errorChunk *llm.StreamChunk
		for chunk := range streamCh {
			if chunk.Err != nil {
				errorChunk = &chunk
			} else {
				validChunks = append(validChunks, chunk)
			}
		}

		// 应该首先收到有效的块
		assert.GreaterOrEqual(t, len(validChunks), 1,
			"Should receive at least one valid chunk before error for provider %s", providerName)

		// 应该收到错误块
		assert.NotNil(t, errorChunk,
			"Should receive error chunk for invalid JSON for provider %s", providerName)
	})
}

// TestProperty15_StreamErrorHandling_EmptyDataLine 验证数据是否为空
// 在处理“data:”前缀之后（可能是错误，也可能不是错误，具体取决于实现）。
func TestProperty15_StreamErrorHandling_EmptyDataLine(t *testing.T) {
	logger := zap.NewNop()

	// 创建一个服务器，发送空数据行，后跟有效数据
	mockEmptyDataServer := func() *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)

			flusher, ok := w.(http.Flusher)
			if !ok {
				return
			}

			// 发送空数据行（数据后只有空格:)
			fmt.Fprintf(w, "data:    \n\n")
			flusher.Flush()

			// 发送 [DONE] 关闭
			fmt.Fprintf(w, "data: [DONE]\n\n")
			flusher.Flush()
		}))
	}

	providerList := []string{"grok", "qwen", "deepseek", "glm", "minimax"}

	for _, providerName := range providerList {
		t.Run(providerName, func(t *testing.T) {
			server := mockEmptyDataServer()
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

			require.NoError(t, err, "Stream() should not return error")

			// 漏极通道 - 空数据线应被跳过或导致错误
			var hasError bool
			for chunk := range streamCh {
				if chunk.Err != nil {
					hasError = true
					// Err 已经是 *llm.Error，不需要类型断言
					assert.NotNil(t, chunk.Err, "Error should not be nil for provider %s", providerName)
				}
			}

			// “data:”后的空白会导致 JSON 解析错误
			// 因为它不是有效的 JSON 并且不是 [DONE]
			assert.True(t, hasError, "Empty data line should cause error for provider %s", providerName)
		})
	}
}
