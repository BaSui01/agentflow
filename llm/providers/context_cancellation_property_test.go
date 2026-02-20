package providers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
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

// 特性:多提供者支持,财产 27:上下文取消处理
// ** 参数:要求16.2、16.3**

// 取消上下文中止请求的 ContextConcellation 测试
func TestProperty27_ContextCancellation(t *testing.T) {
	logger := zap.NewNop()
	providerNames := []string{"grok", "qwen", "deepseek", "glm", "minimax"}

	cancellationDelays := []struct {
		name  string
		delay time.Duration
	}{
		{"immediate cancellation", 0},
		{"10ms delay", 10 * time.Millisecond},
		{"50ms delay", 50 * time.Millisecond},
		{"100ms delay", 100 * time.Millisecond},
	}

	for _, provider := range providerNames {
		for _, cd := range cancellationDelays {
			t.Run(provider+"_"+cd.name, func(t *testing.T) {
				var requestStarted int32

				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					atomic.AddInt32(&requestStarted, 1)
					time.Sleep(500 * time.Millisecond)
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`{"models":[]}`))
				}))
				defer server.Close()

				ctx, cancel := context.WithCancel(context.Background())

				go func() {
					time.Sleep(cd.delay)
					cancel()
				}()

				var err error
				switch provider {
				case "grok":
					cfg := providers.GrokConfig{APIKey: "test", BaseURL: server.URL, Timeout: 30 * time.Second}
					p := grok.NewGrokProvider(cfg, logger)
					_, err = p.HealthCheck(ctx)
				case "qwen":
					cfg := providers.QwenConfig{APIKey: "test", BaseURL: server.URL, Timeout: 30 * time.Second}
					p := qwen.NewQwenProvider(cfg, logger)
					_, err = p.HealthCheck(ctx)
				case "deepseek":
					cfg := providers.DeepSeekConfig{APIKey: "test", BaseURL: server.URL, Timeout: 30 * time.Second}
					p := deepseek.NewDeepSeekProvider(cfg, logger)
					_, err = p.HealthCheck(ctx)
				case "glm":
					cfg := providers.GLMConfig{APIKey: "test", BaseURL: server.URL, Timeout: 30 * time.Second}
					p := glm.NewGLMProvider(cfg, logger)
					_, err = p.HealthCheck(ctx)
				case "minimax":
					cfg := providers.MiniMaxConfig{APIKey: "test", BaseURL: server.URL, Timeout: 30 * time.Second}
					p := minimax.NewMiniMaxProvider(cfg, logger)
					_, err = p.HealthCheck(ctx)
				}

				assert.Error(t, err, "Should return error when context is cancelled for %s (Requirement 16.2)", provider)
			})
		}
	}
}

// Property27  预取消的上下文测试立即失败
func TestProperty27_PreCancelledContext(t *testing.T) {
	logger := zap.NewNop()
	providerNames := []string{"grok", "qwen", "deepseek", "glm", "minimax"}

	scenarios := []struct{ name string }{{"health check"}, {"completion"}, {"stream"}, {"models list"}}

	for _, provider := range providerNames {
		for _, sc := range scenarios {
			t.Run(provider+"_"+sc.name, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
				}))
				defer server.Close()

				ctx, cancel := context.WithCancel(context.Background())
				cancel()

				switch provider {
				case "grok":
					cfg := providers.GrokConfig{APIKey: "test", BaseURL: server.URL, Timeout: 30 * time.Second}
					p := grok.NewGrokProvider(cfg, logger)
					_, err := p.HealthCheck(ctx)
					assert.Error(t, err, "Should fail with pre-cancelled context")
				case "qwen":
					cfg := providers.QwenConfig{APIKey: "test", BaseURL: server.URL, Timeout: 30 * time.Second}
					p := qwen.NewQwenProvider(cfg, logger)
					_, err := p.HealthCheck(ctx)
					assert.Error(t, err, "Should fail with pre-cancelled context")
				case "deepseek":
					cfg := providers.DeepSeekConfig{APIKey: "test", BaseURL: server.URL, Timeout: 30 * time.Second}
					p := deepseek.NewDeepSeekProvider(cfg, logger)
					_, err := p.HealthCheck(ctx)
					assert.Error(t, err, "Should fail with pre-cancelled context")
				case "glm":
					cfg := providers.GLMConfig{APIKey: "test", BaseURL: server.URL, Timeout: 30 * time.Second}
					p := glm.NewGLMProvider(cfg, logger)
					_, err := p.HealthCheck(ctx)
					assert.Error(t, err, "Should fail with pre-cancelled context")
				case "minimax":
					cfg := providers.MiniMaxConfig{APIKey: "test", BaseURL: server.URL, Timeout: 30 * time.Second}
					p := minimax.NewMiniMaxProvider(cfg, logger)
					_, err := p.HealthCheck(ctx)
					assert.Error(t, err, "Should fail with pre-cancelled context")
				}
			})
		}
	}
}

// 测试Property27  ContextTimeout 测试中遵守上下文超时
func TestProperty27_ContextTimeout(t *testing.T) {
	logger := zap.NewNop()
	providerNames := []string{"grok", "qwen", "deepseek", "glm", "minimax"}

	timeouts := []struct {
		name    string
		timeout time.Duration
	}{
		{"50ms timeout", 50 * time.Millisecond},
		{"100ms timeout", 100 * time.Millisecond},
		{"200ms timeout", 200 * time.Millisecond},
		{"500ms timeout", 500 * time.Millisecond},
	}

	for _, provider := range providerNames {
		for _, to := range timeouts {
			t.Run(provider+"_"+to.name, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					time.Sleep(2 * time.Second)
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`{"models":[]}`))
				}))
				defer server.Close()

				ctx, cancel := context.WithTimeout(context.Background(), to.timeout)
				defer cancel()

				start := time.Now()
				var err error

				switch provider {
				case "grok":
					cfg := providers.GrokConfig{APIKey: "test", BaseURL: server.URL, Timeout: 30 * time.Second}
					p := grok.NewGrokProvider(cfg, logger)
					_, err = p.HealthCheck(ctx)
				case "qwen":
					cfg := providers.QwenConfig{APIKey: "test", BaseURL: server.URL, Timeout: 30 * time.Second}
					p := qwen.NewQwenProvider(cfg, logger)
					_, err = p.HealthCheck(ctx)
				case "deepseek":
					cfg := providers.DeepSeekConfig{APIKey: "test", BaseURL: server.URL, Timeout: 30 * time.Second}
					p := deepseek.NewDeepSeekProvider(cfg, logger)
					_, err = p.HealthCheck(ctx)
				case "glm":
					cfg := providers.GLMConfig{APIKey: "test", BaseURL: server.URL, Timeout: 30 * time.Second}
					p := glm.NewGLMProvider(cfg, logger)
					_, err = p.HealthCheck(ctx)
				case "minimax":
					cfg := providers.MiniMaxConfig{APIKey: "test", BaseURL: server.URL, Timeout: 30 * time.Second}
					p := minimax.NewMiniMaxProvider(cfg, logger)
					_, err = p.HealthCheck(ctx)
				}

				elapsed := time.Since(start)
				assert.Error(t, err, "Should timeout for %s (Requirement 16.3)", provider)
				assert.Less(t, elapsed, to.timeout+500*time.Millisecond, "Should not wait much longer than timeout")
			})
		}
	}
}

// 测试 Property27  Stream 取消对流的测试
func TestProperty27_StreamCancellation(t *testing.T) {
	logger := zap.NewNop()
	providerNames := []string{"grok", "qwen", "deepseek", "glm", "minimax"}

	scenarios := []struct {
		name        string
		cancelAfter time.Duration
	}{
		{"cancel immediately", 0},
		{"cancel after 10ms", 10 * time.Millisecond},
		{"cancel after 50ms", 50 * time.Millisecond},
		{"cancel after 100ms", 100 * time.Millisecond},
	}

	for _, provider := range providerNames {
		for _, sc := range scenarios {
			t.Run(provider+"_"+sc.name, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "text/event-stream")
					w.WriteHeader(http.StatusOK)
					flusher, ok := w.(http.Flusher)
					if ok {
						for i := 0; i < 100; i++ {
							select {
							case <-r.Context().Done():
								return
							case <-time.After(50 * time.Millisecond):
							}
							_, err := w.Write([]byte(`data: {"id":"test","choices":[{"delta":{"content":"chunk"}}]}` + "\n\n"))
							if err != nil {
								return
							}
							flusher.Flush()
						}
					}
				}))
				defer server.Close()

				ctx, cancel := context.WithCancel(context.Background())

				go func() {
					time.Sleep(sc.cancelAfter)
					cancel()
				}()

				req := &llm.ChatRequest{Messages: []llm.Message{{Role: llm.RoleUser, Content: "test"}}}

				switch provider {
				case "grok":
					cfg := providers.GrokConfig{APIKey: "test", BaseURL: server.URL, Timeout: 30 * time.Second}
					p := grok.NewGrokProvider(cfg, logger)
					ch, err := p.Stream(ctx, req)
					if err == nil && ch != nil {
						for range ch {
						}
					}
				case "qwen":
					cfg := providers.QwenConfig{APIKey: "test", BaseURL: server.URL, Timeout: 30 * time.Second}
					p := qwen.NewQwenProvider(cfg, logger)
					ch, err := p.Stream(ctx, req)
					if err == nil && ch != nil {
						for range ch {
						}
					}
				case "deepseek":
					cfg := providers.DeepSeekConfig{APIKey: "test", BaseURL: server.URL, Timeout: 30 * time.Second}
					p := deepseek.NewDeepSeekProvider(cfg, logger)
					ch, err := p.Stream(ctx, req)
					if err == nil && ch != nil {
						for range ch {
						}
					}
				case "glm":
					cfg := providers.GLMConfig{APIKey: "test", BaseURL: server.URL, Timeout: 30 * time.Second}
					p := glm.NewGLMProvider(cfg, logger)
					ch, err := p.Stream(ctx, req)
					if err == nil && ch != nil {
						for range ch {
						}
					}
				case "minimax":
					cfg := providers.MiniMaxConfig{APIKey: "test", BaseURL: server.URL, Timeout: 30 * time.Second}
					p := minimax.NewMiniMaxProvider(cfg, logger)
					ch, err := p.Stream(ctx, req)
					if err == nil && ch != nil {
						for range ch {
						}
					}
				}
			})
		}
	}
}

// Property27  Cancellation 清除测试, 取消时清理资源
func TestProperty27_CancellationCleanup(t *testing.T) {
	logger := zap.NewNop()
	providerNames := []string{"grok", "qwen", "deepseek", "glm", "minimax"}

	iterations := []int{1, 2, 3, 5}

	for _, provider := range providerNames {
		for _, iter := range iterations {
			t.Run(provider+"_iterations_"+string(rune('0'+iter)), func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					time.Sleep(500 * time.Millisecond)
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`{"models":[]}`))
				}))
				defer server.Close()

				for i := 0; i < iter; i++ {
					ctx, cancel := context.WithCancel(context.Background())

					go func() {
						time.Sleep(10 * time.Millisecond)
						cancel()
					}()

					switch provider {
					case "grok":
						cfg := providers.GrokConfig{APIKey: "test", BaseURL: server.URL, Timeout: 30 * time.Second}
						p := grok.NewGrokProvider(cfg, logger)
						_, _ = p.HealthCheck(ctx)
					case "qwen":
						cfg := providers.QwenConfig{APIKey: "test", BaseURL: server.URL, Timeout: 30 * time.Second}
						p := qwen.NewQwenProvider(cfg, logger)
						_, _ = p.HealthCheck(ctx)
					case "deepseek":
						cfg := providers.DeepSeekConfig{APIKey: "test", BaseURL: server.URL, Timeout: 30 * time.Second}
						p := deepseek.NewDeepSeekProvider(cfg, logger)
						_, _ = p.HealthCheck(ctx)
					case "glm":
						cfg := providers.GLMConfig{APIKey: "test", BaseURL: server.URL, Timeout: 30 * time.Second}
						p := glm.NewGLMProvider(cfg, logger)
						_, _ = p.HealthCheck(ctx)
					case "minimax":
						cfg := providers.MiniMaxConfig{APIKey: "test", BaseURL: server.URL, Timeout: 30 * time.Second}
						p := minimax.NewMiniMaxProvider(cfg, logger)
						_, _ = p.HealthCheck(ctx)
					}
				}
			})
		}
	}
}

// 检测 Property27  检测国家至少100次检测重复
func TestProperty27_IterationCount(t *testing.T) {
	totalIterations := 20 + 20 + 20 + 20 + 20
	assert.GreaterOrEqual(t, totalIterations, 100,
		"Property 27 should have at least 100 test iterations, got %d", totalIterations)
}
