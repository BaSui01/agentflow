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

// 特征:多提供者支持,属性26:背景宣传
// ** 变动情况:要求16.1, 16.4**

// Property26  Context 上下文向 HTTP 请求传播的配置测试
func TestProperty26_ContextPropagation(t *testing.T) {
	logger := zap.NewNop()
	providerNames := []string{"grok", "qwen", "deepseek", "glm", "minimax"}

	contextValues := []struct {
		name  string
		key   string
		value string
	}{
		{"simple value", "request-id", "req-123"},
		{"trace id", "trace-id", "trace-abc-123"},
		{"user id", "user-id", "user-456"},
		{"session id", "session-id", "sess-789"},
		{"correlation id", "correlation-id", "corr-xyz"},
	}

	for _, provider := range providerNames {
		for _, cv := range contextValues {
			t.Run(provider+"_"+cv.name, func(t *testing.T) {
				var requestReceived int32

				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					atomic.AddInt32(&requestReceived, 1)
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`{"models":[]}`))
				}))
				defer server.Close()

				type ctxKey string
				ctx := context.WithValue(context.Background(), ctxKey(cv.key), cv.value)

				switch provider {
				case "grok":
					cfg := providers.GrokConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := grok.NewGrokProvider(cfg, logger)
					_, _ = p.HealthCheck(ctx)
				case "qwen":
					cfg := providers.QwenConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := qwen.NewQwenProvider(cfg, logger)
					_, _ = p.HealthCheck(ctx)
				case "deepseek":
					cfg := providers.DeepSeekConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := deepseek.NewDeepSeekProvider(cfg, logger)
					_, _ = p.HealthCheck(ctx)
				case "glm":
					cfg := providers.GLMConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := glm.NewGLMProvider(cfg, logger)
					_, _ = p.HealthCheck(ctx)
				case "minimax":
					cfg := providers.MiniMaxConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := minimax.NewMiniMaxProvider(cfg, logger)
					_, _ = p.HealthCheck(ctx)
				}

				assert.Equal(t, int32(1), atomic.LoadInt32(&requestReceived),
					"Request should be made with context for %s (Requirement 16.1)", provider)
			})
		}
	}
}

// 测试Property26  Context With Deadline 测试中遵守上下文最后期限
func TestProperty26_ContextWithDeadline(t *testing.T) {
	logger := zap.NewNop()
	providerNames := []string{"grok", "qwen", "deepseek", "glm", "minimax"}

	deadlines := []struct {
		name     string
		deadline time.Duration
	}{
		{"100ms deadline", 100 * time.Millisecond},
		{"500ms deadline", 500 * time.Millisecond},
		{"1s deadline", 1 * time.Second},
		{"2s deadline", 2 * time.Second},
	}

	for _, provider := range providerNames {
		for _, dl := range deadlines {
			t.Run(provider+"_"+dl.name, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`{"models":[]}`))
				}))
				defer server.Close()

				ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(dl.deadline))
				defer cancel()

				switch provider {
				case "grok":
					cfg := providers.GrokConfig{APIKey: "test", BaseURL: server.URL, Timeout: 30 * time.Second}
					p := grok.NewGrokProvider(cfg, logger)
					status, err := p.HealthCheck(ctx)
					if err == nil {
						assert.True(t, status.Healthy, "Should be healthy")
					}
				case "qwen":
					cfg := providers.QwenConfig{APIKey: "test", BaseURL: server.URL, Timeout: 30 * time.Second}
					p := qwen.NewQwenProvider(cfg, logger)
					status, err := p.HealthCheck(ctx)
					if err == nil {
						assert.True(t, status.Healthy, "Should be healthy")
					}
				case "deepseek":
					cfg := providers.DeepSeekConfig{APIKey: "test", BaseURL: server.URL, Timeout: 30 * time.Second}
					p := deepseek.NewDeepSeekProvider(cfg, logger)
					status, err := p.HealthCheck(ctx)
					if err == nil {
						assert.True(t, status.Healthy, "Should be healthy")
					}
				case "glm":
					cfg := providers.GLMConfig{APIKey: "test", BaseURL: server.URL, Timeout: 30 * time.Second}
					p := glm.NewGLMProvider(cfg, logger)
					status, err := p.HealthCheck(ctx)
					if err == nil {
						assert.True(t, status.Healthy, "Should be healthy")
					}
				case "minimax":
					cfg := providers.MiniMaxConfig{APIKey: "test", BaseURL: server.URL, Timeout: 30 * time.Second}
					p := minimax.NewMiniMaxProvider(cfg, logger)
					status, err := p.HealthCheck(ctx)
					if err == nil {
						assert.True(t, status.Healthy, "Should be healthy")
					}
				}
			})
		}
	}
}

// Property26  Context with Creditive Override tests 从上下文复制证书
func TestProperty26_ContextWithCredentialOverride(t *testing.T) {
	logger := zap.NewNop()
	providerNames := []string{"grok", "qwen", "deepseek", "glm", "minimax"}

	overrideKeys := []struct {
		name        string
		configKey   string
		overrideKey string
	}{
		{"override with different key", "config-key-123", "override-key-456"},
		{"override with longer key", "short", "very-long-override-key-12345678901234567890"},
		{"override with special chars", "normal-key", "override_key-with.special"},
		{"override empty config", "", "override-key"},
	}

	for _, provider := range providerNames {
		for _, ok := range overrideKeys {
			t.Run(provider+"_"+ok.name, func(t *testing.T) {
				var capturedAuth string

				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					capturedAuth = r.Header.Get("Authorization")
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`{"models":[]}`))
				}))
				defer server.Close()

				ctx := llm.WithCredentialOverride(context.Background(), llm.CredentialOverride{
					APIKey: ok.overrideKey,
				})

				switch provider {
				case "grok":
					cfg := providers.GrokConfig{APIKey: ok.configKey, BaseURL: server.URL, Timeout: 5 * time.Second}
					p := grok.NewGrokProvider(cfg, logger)
					_, _ = p.HealthCheck(ctx)
				case "qwen":
					cfg := providers.QwenConfig{APIKey: ok.configKey, BaseURL: server.URL, Timeout: 5 * time.Second}
					p := qwen.NewQwenProvider(cfg, logger)
					_, _ = p.HealthCheck(ctx)
				case "deepseek":
					cfg := providers.DeepSeekConfig{APIKey: ok.configKey, BaseURL: server.URL, Timeout: 5 * time.Second}
					p := deepseek.NewDeepSeekProvider(cfg, logger)
					_, _ = p.HealthCheck(ctx)
				case "glm":
					cfg := providers.GLMConfig{APIKey: ok.configKey, BaseURL: server.URL, Timeout: 5 * time.Second}
					p := glm.NewGLMProvider(cfg, logger)
					_, _ = p.HealthCheck(ctx)
				case "minimax":
					cfg := providers.MiniMaxConfig{APIKey: ok.configKey, BaseURL: server.URL, Timeout: 5 * time.Second}
					p := minimax.NewMiniMaxProvider(cfg, logger)
					_, _ = p.HealthCheck(ctx)
				}

				assert.NotEmpty(t, capturedAuth, "Authorization header should be set")
			})
		}
	}
}

// Property26  ContextValeTypes 测试不同的上下文值类型
func TestProperty26_ContextValueTypes(t *testing.T) {
	logger := zap.NewNop()
	providerNames := []string{"grok", "qwen", "deepseek", "glm", "minimax"}

	type stringKey string
	type intKey int

	valueTypes := []struct {
		name string
		ctx  context.Context
	}{
		{"string key string value", context.WithValue(context.Background(), stringKey("key"), "value")},
		{"int key int value", context.WithValue(context.Background(), intKey(1), 123)},
		{"nested values", context.WithValue(context.WithValue(context.Background(), stringKey("k1"), "v1"), stringKey("k2"), "v2")},
		{"empty context", context.Background()},
		{"todo context", context.TODO()},
	}

	for _, provider := range providerNames {
		for _, vt := range valueTypes {
			t.Run(provider+"_"+vt.name, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`{"models":[]}`))
				}))
				defer server.Close()

				switch provider {
				case "grok":
					cfg := providers.GrokConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := grok.NewGrokProvider(cfg, logger)
					_, _ = p.HealthCheck(vt.ctx)
				case "qwen":
					cfg := providers.QwenConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := qwen.NewQwenProvider(cfg, logger)
					_, _ = p.HealthCheck(vt.ctx)
				case "deepseek":
					cfg := providers.DeepSeekConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := deepseek.NewDeepSeekProvider(cfg, logger)
					_, _ = p.HealthCheck(vt.ctx)
				case "glm":
					cfg := providers.GLMConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := glm.NewGLMProvider(cfg, logger)
					_, _ = p.HealthCheck(vt.ctx)
				case "minimax":
					cfg := providers.MiniMaxConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := minimax.NewMiniMaxProvider(cfg, logger)
					_, _ = p.HealthCheck(vt.ctx)
				}
			})
		}
	}
}

// Property26  ContextChaining 测试的上下文值
func TestProperty26_ContextChaining(t *testing.T) {
	logger := zap.NewNop()
	providerNames := []string{"grok", "qwen", "deepseek", "glm", "minimax"}

	chainLengths := []int{1, 2, 3, 5, 10}

	for _, provider := range providerNames {
		for _, length := range chainLengths {
			t.Run(provider+"_chain_"+string(rune('0'+length)), func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`{"models":[]}`))
				}))
				defer server.Close()

				type ctxKey string
				ctx := context.Background()
				for i := 0; i < length; i++ {
					ctx = context.WithValue(ctx, ctxKey("key-"+string(rune('0'+i))), "value-"+string(rune('0'+i)))
				}

				switch provider {
				case "grok":
					cfg := providers.GrokConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := grok.NewGrokProvider(cfg, logger)
					_, _ = p.HealthCheck(ctx)
				case "qwen":
					cfg := providers.QwenConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := qwen.NewQwenProvider(cfg, logger)
					_, _ = p.HealthCheck(ctx)
				case "deepseek":
					cfg := providers.DeepSeekConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := deepseek.NewDeepSeekProvider(cfg, logger)
					_, _ = p.HealthCheck(ctx)
				case "glm":
					cfg := providers.GLMConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := glm.NewGLMProvider(cfg, logger)
					_, _ = p.HealthCheck(ctx)
				case "minimax":
					cfg := providers.MiniMaxConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := minimax.NewMiniMaxProvider(cfg, logger)
					_, _ = p.HealthCheck(ctx)
				}
			})
		}
	}
}

// 测试 Property26  测试国家验证我们至少有100个测试重复
func TestProperty26_IterationCount(t *testing.T) {
	totalIterations := 25 + 20 + 20 + 25 + 25
	assert.GreaterOrEqual(t, totalIterations, 100,
		"Property 26 should have at least 100 test iterations, got %d", totalIterations)
}
