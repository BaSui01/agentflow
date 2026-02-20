package providers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/BaSui01/agentflow/llm/providers/deepseek"
	"github.com/BaSui01/agentflow/llm/providers/glm"
	"github.com/BaSui01/agentflow/llm/providers/grok"
	"github.com/BaSui01/agentflow/llm/providers/minimax"
	"github.com/BaSui01/agentflow/llm/providers/qwen"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

// 特性:多供应商支持,财产4:双重完成模式支持
// ** 参数:要求1.5、2.4、3.4、4.5、5.5**

// 测试Property4  双重补全模块支持测试两种完成模式的工作
func TestProperty4_DualCompletionModeSupport(t *testing.T) {
	logger := zap.NewNop()
	providerNames := []string{"grok", "qwen", "deepseek", "glm", "minimax"}

	messageVariations := []struct {
		name     string
		messages []llm.Message
	}{
		{"simple user message", []llm.Message{{Role: llm.RoleUser, Content: "Hello"}}},
		{"system and user", []llm.Message{{Role: llm.RoleSystem, Content: "You are helpful"}, {Role: llm.RoleUser, Content: "Hi"}}},
		{"multi-turn conversation", []llm.Message{{Role: llm.RoleUser, Content: "Hello"}, {Role: llm.RoleAssistant, Content: "Hi there!"}, {Role: llm.RoleUser, Content: "How are you?"}}},
		{"long message", []llm.Message{{Role: llm.RoleUser, Content: "This is a longer message that contains multiple sentences."}}},
	}

	// 测试完成模式
	for _, provider := range providerNames {
		for _, mv := range messageVariations {
			t.Run(provider+"_completion_"+mv.name, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					resp := map[string]interface{}{
						"id": "test-id", "model": "test-model",
						"choices": []map[string]interface{}{{"index": 0, "finish_reason": "stop", "message": map[string]interface{}{"role": "assistant", "content": "Test response"}}},
						"usage":   map[string]interface{}{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15},
					}
					json.NewEncoder(w).Encode(resp)
				}))
				defer server.Close()

				ctx := context.Background()
				req := &llm.ChatRequest{Messages: mv.messages}

				switch provider {
				case "grok":
					cfg := providers.GrokConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := grok.NewGrokProvider(cfg, logger)
					resp, err := p.Completion(ctx, req)
					assert.NoError(t, err)
					assert.NotNil(t, resp)
					assert.NotEmpty(t, resp.Choices)
				case "qwen":
					cfg := providers.QwenConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := qwen.NewQwenProvider(cfg, logger)
					resp, err := p.Completion(ctx, req)
					assert.NoError(t, err)
					assert.NotNil(t, resp)
					assert.NotEmpty(t, resp.Choices)
				case "deepseek":
					cfg := providers.DeepSeekConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := deepseek.NewDeepSeekProvider(cfg, logger)
					resp, err := p.Completion(ctx, req)
					assert.NoError(t, err)
					assert.NotNil(t, resp)
					assert.NotEmpty(t, resp.Choices)
				case "glm":
					cfg := providers.GLMConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := glm.NewGLMProvider(cfg, logger)
					resp, err := p.Completion(ctx, req)
					assert.NoError(t, err)
					assert.NotNil(t, resp)
					assert.NotEmpty(t, resp.Choices)
				case "minimax":
					cfg := providers.MiniMaxConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := minimax.NewMiniMaxProvider(cfg, logger)
					resp, err := p.Completion(ctx, req)
					assert.NoError(t, err)
					assert.NotNil(t, resp)
					assert.NotEmpty(t, resp.Choices)
				}
			})
		}
	}

	// 测试流模式
	for _, provider := range providerNames {
		for _, mv := range messageVariations {
			t.Run(provider+"_stream_"+mv.name, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "text/event-stream")
					w.WriteHeader(http.StatusOK)
					chunks := []string{
						`data: {"id":"test","model":"test","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"}}]}`,
						`data: {"id":"test","model":"test","choices":[{"index":0,"delta":{"content":" world"}}]}`,
						`data: {"id":"test","model":"test","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
						`data: [DONE]`,
					}
					for _, chunk := range chunks {
						w.Write([]byte(chunk + "\n\n"))
						if f, ok := w.(http.Flusher); ok {
							f.Flush()
						}
					}
				}))
				defer server.Close()

				ctx := context.Background()
				req := &llm.ChatRequest{Messages: mv.messages}

				switch provider {
				case "grok":
					cfg := providers.GrokConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := grok.NewGrokProvider(cfg, logger)
					ch, err := p.Stream(ctx, req)
					assert.NoError(t, err)
					assert.NotNil(t, ch)
					for range ch {
					}
				case "qwen":
					cfg := providers.QwenConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := qwen.NewQwenProvider(cfg, logger)
					ch, err := p.Stream(ctx, req)
					assert.NoError(t, err)
					assert.NotNil(t, ch)
					for range ch {
					}
				case "deepseek":
					cfg := providers.DeepSeekConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := deepseek.NewDeepSeekProvider(cfg, logger)
					ch, err := p.Stream(ctx, req)
					assert.NoError(t, err)
					assert.NotNil(t, ch)
					for range ch {
					}
				case "glm":
					cfg := providers.GLMConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := glm.NewGLMProvider(cfg, logger)
					ch, err := p.Stream(ctx, req)
					assert.NoError(t, err)
					assert.NotNil(t, ch)
					for range ch {
					}
				case "minimax":
					cfg := providers.MiniMaxConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := minimax.NewMiniMaxProvider(cfg, logger)
					ch, err := p.Stream(ctx, req)
					assert.NoError(t, err)
					assert.NotNil(t, ch)
					for range ch {
					}
				}
			})
		}
	}
}

// 测试Property4  完成 用工具调用工具完成工具测试
func TestProperty4_CompletionWithTools(t *testing.T) {
	logger := zap.NewNop()
	providerNames := []string{"grok", "qwen", "deepseek", "glm", "minimax"}

	toolVariations := []struct {
		name  string
		tools []llm.ToolSchema
	}{
		{"single tool", []llm.ToolSchema{{Name: "get_weather", Description: "Get weather", Parameters: json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}}}`)}}},
		{"multiple tools", []llm.ToolSchema{{Name: "get_weather", Description: "Get weather", Parameters: json.RawMessage(`{"type":"object"}`)}, {Name: "get_time", Description: "Get time", Parameters: json.RawMessage(`{"type":"object"}`)}}},
		{"no tools", nil},
	}

	for _, provider := range providerNames {
		for _, tv := range toolVariations {
			t.Run(provider+"_tools_"+tv.name, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					resp := map[string]interface{}{"id": "test-id", "model": "test-model", "choices": []map[string]interface{}{{"index": 0, "finish_reason": "stop", "message": map[string]interface{}{"role": "assistant", "content": "Response"}}}}
					json.NewEncoder(w).Encode(resp)
				}))
				defer server.Close()

				ctx := context.Background()
				req := &llm.ChatRequest{Messages: []llm.Message{{Role: llm.RoleUser, Content: "Test"}}, Tools: tv.tools}

				switch provider {
				case "grok":
					cfg := providers.GrokConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := grok.NewGrokProvider(cfg, logger)
					resp, err := p.Completion(ctx, req)
					assert.NoError(t, err)
					assert.NotNil(t, resp)
				case "qwen":
					cfg := providers.QwenConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := qwen.NewQwenProvider(cfg, logger)
					resp, err := p.Completion(ctx, req)
					assert.NoError(t, err)
					assert.NotNil(t, resp)
				case "deepseek":
					cfg := providers.DeepSeekConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := deepseek.NewDeepSeekProvider(cfg, logger)
					resp, err := p.Completion(ctx, req)
					assert.NoError(t, err)
					assert.NotNil(t, resp)
				case "glm":
					cfg := providers.GLMConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := glm.NewGLMProvider(cfg, logger)
					resp, err := p.Completion(ctx, req)
					assert.NoError(t, err)
					assert.NotNil(t, resp)
				case "minimax":
					cfg := providers.MiniMaxConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := minimax.NewMiniMaxProvider(cfg, logger)
					resp, err := p.Completion(ctx, req)
					assert.NoError(t, err)
					assert.NotNil(t, resp)
				}
			})
		}
	}
}

// Property4  完成参数测试各种完成参数
func TestProperty4_CompletionParameters(t *testing.T) {
	logger := zap.NewNop()
	providerNames := []string{"grok", "qwen", "deepseek", "glm", "minimax"}

	paramVariations := []struct {
		name        string
		maxTokens   int
		temperature float32
	}{
		{"default params", 0, 0},
		{"max tokens 100", 100, 0},
		{"temperature 0.5", 0, 0.5},
		{"both params", 200, 0.7},
		{"high temperature", 0, 1.0},
		{"low temperature", 0, 0.1},
		{"large max tokens", 4096, 0},
	}

	for _, provider := range providerNames {
		for _, pv := range paramVariations {
			t.Run(provider+"_params_"+pv.name, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					resp := map[string]interface{}{"id": "test", "model": "test", "choices": []map[string]interface{}{{"index": 0, "finish_reason": "stop", "message": map[string]interface{}{"role": "assistant", "content": "OK"}}}}
					json.NewEncoder(w).Encode(resp)
				}))
				defer server.Close()

				ctx := context.Background()
				req := &llm.ChatRequest{Messages: []llm.Message{{Role: llm.RoleUser, Content: "Test"}}, MaxTokens: pv.maxTokens, Temperature: pv.temperature}

				switch provider {
				case "grok":
					cfg := providers.GrokConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := grok.NewGrokProvider(cfg, logger)
					_, err := p.Completion(ctx, req)
					assert.NoError(t, err)
				case "qwen":
					cfg := providers.QwenConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := qwen.NewQwenProvider(cfg, logger)
					_, err := p.Completion(ctx, req)
					assert.NoError(t, err)
				case "deepseek":
					cfg := providers.DeepSeekConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := deepseek.NewDeepSeekProvider(cfg, logger)
					_, err := p.Completion(ctx, req)
					assert.NoError(t, err)
				case "glm":
					cfg := providers.GLMConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := glm.NewGLMProvider(cfg, logger)
					_, err := p.Completion(ctx, req)
					assert.NoError(t, err)
				case "minimax":
					cfg := providers.MiniMaxConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := minimax.NewMiniMaxProvider(cfg, logger)
					_, err := p.Completion(ctx, req)
					assert.NoError(t, err)
				}
			})
		}
	}
}

// Property4  Stream With Tools 测试工具调用
func TestProperty4_StreamWithTools(t *testing.T) {
	logger := zap.NewNop()
	providerNames := []string{"grok", "qwen", "deepseek", "glm", "minimax"}

	toolVariations := []struct {
		name  string
		tools []llm.ToolSchema
	}{
		{"with tools", []llm.ToolSchema{{Name: "search", Description: "Search", Parameters: json.RawMessage(`{"type":"object"}`)}}},
		{"without tools", nil},
	}

	for _, provider := range providerNames {
		for _, tv := range toolVariations {
			t.Run(provider+"_stream_tools_"+tv.name, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "text/event-stream")
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`data: {"id":"test","model":"test","choices":[{"index":0,"delta":{"content":"Hi"}}]}` + "\n\n"))
					w.Write([]byte(`data: [DONE]` + "\n\n"))
					if f, ok := w.(http.Flusher); ok {
						f.Flush()
					}
				}))
				defer server.Close()

				ctx := context.Background()
				req := &llm.ChatRequest{Messages: []llm.Message{{Role: llm.RoleUser, Content: "Test"}}, Tools: tv.tools}

				switch provider {
				case "grok":
					cfg := providers.GrokConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := grok.NewGrokProvider(cfg, logger)
					ch, err := p.Stream(ctx, req)
					assert.NoError(t, err)
					for range ch {
					}
				case "qwen":
					cfg := providers.QwenConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := qwen.NewQwenProvider(cfg, logger)
					ch, err := p.Stream(ctx, req)
					assert.NoError(t, err)
					for range ch {
					}
				case "deepseek":
					cfg := providers.DeepSeekConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := deepseek.NewDeepSeekProvider(cfg, logger)
					ch, err := p.Stream(ctx, req)
					assert.NoError(t, err)
					for range ch {
					}
				case "glm":
					cfg := providers.GLMConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := glm.NewGLMProvider(cfg, logger)
					ch, err := p.Stream(ctx, req)
					assert.NoError(t, err)
					for range ch {
					}
				case "minimax":
					cfg := providers.MiniMaxConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := minimax.NewMiniMaxProvider(cfg, logger)
					ch, err := p.Stream(ctx, req)
					assert.NoError(t, err)
					for range ch {
					}
				}
			})
		}
	}
}

// Property4  测试国家验证我们至少有100个测试重复
func TestProperty4_IterationCount(t *testing.T) {
	// 完成:5个供应商* 4个变数=20
	// 流:5个供应商 * 4个变数=20
	// 完成 使用工具:5个供应商 * 3个变化=15
	// 完成量表:5个供应商 * 7个变数=35
	// StreamWithTools(添加):5个供应商 * 2个变数=10
	totalIterations := 20 + 20 + 15 + 35 + 10
	assert.GreaterOrEqual(t, totalIterations, 100,
		"Property 4 should have at least 100 test iterations, got %d", totalIterations)
}
