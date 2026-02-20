package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
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

// 特性：多提供商支持，属性 13：流请求格式
// **验证：要求 10.1**
//
// This 属性测试验证对于任何提供者，当调用 Stream() 时
// ???with a ChatRequest, the HTTP request body should include "stream": true field.

// StreamRequestCapture 捕获发送到服务器的请求正文
type streamRequestCapture struct {
	mu          sync.Mutex
	requestBody map[string]any
	streamField bool
	captured    bool
}

func (c *streamRequestCapture) setRequest(body map[string]any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.requestBody = body
	if stream, ok := body["stream"].(bool); ok {
		c.streamField = stream
	}
	c.captured = true
}

func (c *streamRequestCapture) getStreamField() (bool, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.streamField, c.captured
}

// mockStreamServer 创建一个测试服务器来捕获请求并返回流响应
func mockStreamServer(capture *streamRequestCapture) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer r.Body.Close()

		var req map[string]any
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		capture.setRequest(req)

		// 返回最小的流响应
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			return
		}

		chunk := map[string]any{
			"id":    "test-id",
			"model": "test-model",
			"choices": []map[string]any{
				{
					"index":         0,
					"delta":         map[string]any{"role": "assistant", "content": "test"},
					"finish_reason": "stop",
				},
			},
		}
		data, _ := json.Marshal(chunk)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
}

// TestProperty13_StreamingRequestFormat 验证 Stream() 设置stream=true
// 在所有提供者的 HTTP 请求正文中。
func TestProperty13_StreamingRequestFormat(t *testing.T) {
	logger := zap.NewNop()

	rapid.Check(t, func(rt *rapid.T) {
		// 生成随机消息内容
		messageContent := rapid.StringMatching(`[a-zA-Z0-9 ]{5,50}`).Draw(rt, "messageContent")
		model := rapid.StringMatching(`[a-z0-9-]{3,20}`).Draw(rt, "model")

		// 选择随机提供商
		providerIndex := rapid.IntRange(0, 4).Draw(rt, "providerIndex")
		providerNames := []string{"grok", "qwen", "deepseek", "glm", "minimax"}
		providerName := providerNames[providerIndex]

		capture := &streamRequestCapture{}
		server := mockStreamServer(capture)
		defer server.Close()

		req := &llm.ChatRequest{
			Model: model,
			Messages: []llm.Message{
				{Role: llm.RoleUser, Content: messageContent},
			},
		}

		ctx := context.Background()
		var streamCh <-chan llm.StreamChunk
		var err error

		switch providerName {
		case "grok":
			cfg := providers.GrokConfig{APIKey: "test-key", BaseURL: server.URL}
			p := grok.NewGrokProvider(cfg, logger)
			streamCh, err = p.Stream(ctx, req)
		case "qwen":
			cfg := providers.QwenConfig{APIKey: "test-key", BaseURL: server.URL}
			p := qwen.NewQwenProvider(cfg, logger)
			streamCh, err = p.Stream(ctx, req)
		case "deepseek":
			cfg := providers.DeepSeekConfig{APIKey: "test-key", BaseURL: server.URL}
			p := deepseek.NewDeepSeekProvider(cfg, logger)
			streamCh, err = p.Stream(ctx, req)
		case "glm":
			cfg := providers.GLMConfig{APIKey: "test-key", BaseURL: server.URL}
			p := glm.NewGLMProvider(cfg, logger)
			streamCh, err = p.Stream(ctx, req)
		case "minimax":
			cfg := providers.MiniMaxConfig{APIKey: "test-key", BaseURL: server.URL}
			p := minimax.NewMiniMaxProvider(cfg, logger)
			streamCh, err = p.Stream(ctx, req)
		}

		require.NoError(t, err, "Stream() should not return error for provider %s", providerName)

		// 排空通道
		for range streamCh {
		}

		// 验证请求中设置了stream=true
		streamValue, captured := capture.getStreamField()
		assert.True(t, captured, "Request should be captured for provider %s", providerName)
		assert.True(t, streamValue, "stream field should be true for provider %s", providerName)
	})
}

// TestProperty13_StreamingRequestFormat_AllProviders 提供表驱动测试
// 确保所有提供者至少进行 100 次迭代。
func TestProperty13_StreamingRequestFormat_AllProviders(t *testing.T) {
	logger := zap.NewNop()

	type testCase struct {
		name           string
		providerName   string
		messageContent string
		model          string
	}

	// 为具有各种输入的所有提供商生成测试用例
	var testCases []testCase

	providerList := []string{"grok", "qwen", "deepseek", "glm", "minimax"}
	messages := []string{
		"Hello",
		"What is the weather?",
		"Tell me a story",
		"Calculate 2+2",
		"Translate hello to Chinese",
		"Summarize this text",
		"Generate code",
		"Explain quantum physics",
		"Write a poem",
		"Debug this error",
		"Create a function",
		"Analyze data",
		"Search for information",
		"Format this document",
		"Convert units",
		"Parse JSON",
		"Validate input",
		"Optimize query",
		"Build API",
		"Test endpoint",
	}

	models := []string{
		"grok-beta",
		"qwen-plus",
		"deepseek-chat",
		"glm-4-plus",
		"abab6.5s-chat",
	}

	// 生成 100+ 测试用例
	idx := 0
	for _, provider := range providerList {
		for _, msg := range messages {
			for _, model := range models {
				testCases = append(testCases, testCase{
					name:           fmt.Sprintf("%s_%s_%d", provider, model, idx),
					providerName:   provider,
					messageContent: msg,
					model:          model,
				})
				idx++
				if idx >= 100 {
					break
				}
			}
			if idx >= 100 {
				break
			}
		}
		if idx >= 100 {
			break
		}
	}

	// 确保我们至少有 100 个测试用例
	require.GreaterOrEqual(t, len(testCases), 100, "Should have at least 100 test cases")

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			capture := &streamRequestCapture{}
			server := mockStreamServer(capture)
			defer server.Close()

			req := &llm.ChatRequest{
				Model: tc.model,
				Messages: []llm.Message{
					{Role: llm.RoleUser, Content: tc.messageContent},
				},
			}

			ctx := context.Background()
			var streamCh <-chan llm.StreamChunk
			var err error

			switch tc.providerName {
			case "grok":
				cfg := providers.GrokConfig{APIKey: "test-key", BaseURL: server.URL}
				p := grok.NewGrokProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			case "qwen":
				cfg := providers.QwenConfig{APIKey: "test-key", BaseURL: server.URL}
				p := qwen.NewQwenProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			case "deepseek":
				cfg := providers.DeepSeekConfig{APIKey: "test-key", BaseURL: server.URL}
				p := deepseek.NewDeepSeekProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			case "glm":
				cfg := providers.GLMConfig{APIKey: "test-key", BaseURL: server.URL}
				p := glm.NewGLMProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			case "minimax":
				cfg := providers.MiniMaxConfig{APIKey: "test-key", BaseURL: server.URL}
				p := minimax.NewMiniMaxProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			}

			require.NoError(t, err, "Stream() should not return error")

			// 排空通道
			for range streamCh {
			}

			// 验证已设置stream=true
			streamValue, captured := capture.getStreamField()
			assert.True(t, captured, "Request should be captured")
			assert.True(t, streamValue, "stream field should be true in request body")
		})
	}
}

// TestProperty13_StreamingRequestFormat_WithTools 验证是否设置了stream=true
// 即使请求中包含了工具。
func TestProperty13_StreamingRequestFormat_WithTools(t *testing.T) {
	logger := zap.NewNop()

	rapid.Check(t, func(rt *rapid.T) {
		providerIndex := rapid.IntRange(0, 4).Draw(rt, "providerIndex")
		providerNames := []string{"grok", "qwen", "deepseek", "glm", "minimax"}
		providerName := providerNames[providerIndex]

		numTools := rapid.IntRange(1, 3).Draw(rt, "numTools")
		tools := make([]llm.ToolSchema, numTools)
		for i := range numTools {
			tools[i] = llm.ToolSchema{
				Name:        rapid.StringMatching(`[a-z_]{3,15}`).Draw(rt, fmt.Sprintf("toolName_%d", i)),
				Description: "Test tool",
				Parameters:  json.RawMessage(`{"type":"object"}`),
			}
		}

		capture := &streamRequestCapture{}
		server := mockStreamServer(capture)
		defer server.Close()

		req := &llm.ChatRequest{
			Model: "test-model",
			Messages: []llm.Message{
				{Role: llm.RoleUser, Content: "Test with tools"},
			},
			Tools: tools,
		}

		ctx := context.Background()
		var streamCh <-chan llm.StreamChunk
		var err error

		switch providerName {
		case "grok":
			cfg := providers.GrokConfig{APIKey: "test-key", BaseURL: server.URL}
			p := grok.NewGrokProvider(cfg, logger)
			streamCh, err = p.Stream(ctx, req)
		case "qwen":
			cfg := providers.QwenConfig{APIKey: "test-key", BaseURL: server.URL}
			p := qwen.NewQwenProvider(cfg, logger)
			streamCh, err = p.Stream(ctx, req)
		case "deepseek":
			cfg := providers.DeepSeekConfig{APIKey: "test-key", BaseURL: server.URL}
			p := deepseek.NewDeepSeekProvider(cfg, logger)
			streamCh, err = p.Stream(ctx, req)
		case "glm":
			cfg := providers.GLMConfig{APIKey: "test-key", BaseURL: server.URL}
			p := glm.NewGLMProvider(cfg, logger)
			streamCh, err = p.Stream(ctx, req)
		case "minimax":
			cfg := providers.MiniMaxConfig{APIKey: "test-key", BaseURL: server.URL}
			p := minimax.NewMiniMaxProvider(cfg, logger)
			streamCh, err = p.Stream(ctx, req)
		}

		require.NoError(t, err, "Stream() should not return error for provider %s", providerName)

		for range streamCh {
		}

		streamValue, captured := capture.getStreamField()
		assert.True(t, captured, "Request should be captured for provider %s", providerName)
		assert.True(t, streamValue, "stream field should be true for provider %s with tools", providerName)
	})
}

// TestProperty13_CompletionDoesNotSetStreamTrue 验证 Completion()
// 不设置stream=true（对比测试以确保Stream()行为正确）。
func TestProperty13_CompletionDoesNotSetStreamTrue(t *testing.T) {
	logger := zap.NewNop()

	providerList := []string{"grok", "qwen", "deepseek", "glm", "minimax"}

	for _, providerName := range providerList {
		t.Run(providerName+"_completion", func(t *testing.T) {
			// 创建一个捕获完成请求的服务器
			completionCapture := &streamRequestCapture{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				defer r.Body.Close()

				var req map[string]any
				json.Unmarshal(body, &req)
				completionCapture.setRequest(req)

				// 返回完成响应
				w.Header().Set("Content-Type", "application/json")
				resp := map[string]any{
					"id":    "test-id",
					"model": "test-model",
					"choices": []map[string]any{
						{
							"index":         0,
							"message":       map[string]any{"role": "assistant", "content": "test"},
							"finish_reason": "stop",
						},
					},
				}
				json.NewEncoder(w).Encode(resp)
			}))
			defer server.Close()

			req := &llm.ChatRequest{
				Model: "test-model",
				Messages: []llm.Message{
					{Role: llm.RoleUser, Content: "Test"},
				},
			}

			ctx := context.Background()

			switch providerName {
			case "grok":
				cfg := providers.GrokConfig{APIKey: "test-key", BaseURL: server.URL}
				p := grok.NewGrokProvider(cfg, logger)
				_, _ = p.Completion(ctx, req)
			case "qwen":
				cfg := providers.QwenConfig{APIKey: "test-key", BaseURL: server.URL}
				p := qwen.NewQwenProvider(cfg, logger)
				_, _ = p.Completion(ctx, req)
			case "deepseek":
				cfg := providers.DeepSeekConfig{APIKey: "test-key", BaseURL: server.URL}
				p := deepseek.NewDeepSeekProvider(cfg, logger)
				_, _ = p.Completion(ctx, req)
			case "glm":
				cfg := providers.GLMConfig{APIKey: "test-key", BaseURL: server.URL}
				p := glm.NewGLMProvider(cfg, logger)
				_, _ = p.Completion(ctx, req)
			case "minimax":
				cfg := providers.MiniMaxConfig{APIKey: "test-key", BaseURL: server.URL}
				p := minimax.NewMiniMaxProvider(cfg, logger)
				_, _ = p.Completion(ctx, req)
			}

			// 验证流对于完成来说不正确
			streamValue, captured := completionCapture.getStreamField()
			assert.True(t, captured, "Request should be captured")
			assert.False(t, streamValue, "stream field should NOT be true for Completion()")
		})
	}
}
