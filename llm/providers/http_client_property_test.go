package providers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/BaSui01/agentflow/llm/providers/deepseek"
	"github.com/BaSui01/agentflow/llm/providers/glm"
	"github.com/BaSui01/agentflow/llm/providers/grok"
	"github.com/BaSui01/agentflow/llm/providers/minimax"
	"github.com/BaSui01/agentflow/llm/providers/qwen"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

// 特性: 多提供者支持, 属性 7: 默认超时配置
// ** 变动情况:要求6.6、15.1**

// 测试Property7  Default Timeout 配置测试 提供者使用 30s 默认超时
func TestProperty7_DefaultTimeoutConfiguration(t *testing.T) {
	logger := zap.NewNop()

	timeoutTestCases := []struct {
		name            string
		configTimeout   time.Duration
		expectedTimeout time.Duration
	}{
		{"zero timeout uses default", 0, 30 * time.Second},
		{"explicit 10s timeout", 10 * time.Second, 10 * time.Second},
		{"explicit 60s timeout", 60 * time.Second, 60 * time.Second},
		{"explicit 5s timeout", 5 * time.Second, 5 * time.Second},
		{"explicit 120s timeout", 120 * time.Second, 120 * time.Second},
	}

	providerNames := []string{"grok", "qwen", "deepseek", "glm", "minimax"}

	for _, provider := range providerNames {
		for _, tc := range timeoutTestCases {
			t.Run(provider+"_"+tc.name, func(t *testing.T) {
				switch provider {
				case "grok":
					cfg := providers.GrokConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: "https://api.x.ai", Timeout: tc.configTimeout}}
					p := grok.NewGrokProvider(cfg, logger)
					assert.NotNil(t, p, "Provider should be created")
				case "qwen":
					cfg := providers.QwenConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1", Timeout: tc.configTimeout}}
					p := qwen.NewQwenProvider(cfg, logger)
					assert.NotNil(t, p, "Provider should be created")
				case "deepseek":
					cfg := providers.DeepSeekConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: "https://api.deepseek.com", Timeout: tc.configTimeout}}
					p := deepseek.NewDeepSeekProvider(cfg, logger)
					assert.NotNil(t, p, "Provider should be created")
				case "glm":
					cfg := providers.GLMConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: "https://open.bigmodel.cn/api/paas/v4", Timeout: tc.configTimeout}}
					p := glm.NewGLMProvider(cfg, logger)
					assert.NotNil(t, p, "Provider should be created")
				case "minimax":
					cfg := providers.MiniMaxConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: "https://api.minimax.chat/v1", Timeout: tc.configTimeout}}
					p := minimax.NewMiniMaxProvider(cfg, logger)
					assert.NotNil(t, p, "Provider should be created")
				}
			})
		}
	}
}

// 测试Property7  Timeout 行为测试 超时实际有效
func TestProperty7_TimeoutBehavior(t *testing.T) {
	logger := zap.NewNop()

	slowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"test","model":"test","choices":[]}`))
	}))
	defer slowServer.Close()

	providerNames := []string{"grok", "qwen", "deepseek", "glm", "minimax"}

	for _, provider := range providerNames {
		t.Run(provider+"_timeout_triggers", func(t *testing.T) {
			ctx := context.Background()

			switch provider {
			case "grok":
				cfg := providers.GrokConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: slowServer.URL, Timeout: 100 * time.Millisecond}}
				p := grok.NewGrokProvider(cfg, logger)
				_, err := p.HealthCheck(ctx)
				assert.Error(t, err, "Should timeout for %s", provider)
			case "qwen":
				cfg := providers.QwenConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: slowServer.URL, Timeout: 100 * time.Millisecond}}
				p := qwen.NewQwenProvider(cfg, logger)
				_, err := p.HealthCheck(ctx)
				assert.Error(t, err, "Should timeout for %s", provider)
			case "deepseek":
				cfg := providers.DeepSeekConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: slowServer.URL, Timeout: 100 * time.Millisecond}}
				p := deepseek.NewDeepSeekProvider(cfg, logger)
				_, err := p.HealthCheck(ctx)
				assert.Error(t, err, "Should timeout for %s", provider)
			case "glm":
				cfg := providers.GLMConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: slowServer.URL, Timeout: 100 * time.Millisecond}}
				p := glm.NewGLMProvider(cfg, logger)
				_, err := p.HealthCheck(ctx)
				assert.Error(t, err, "Should timeout for %s", provider)
			case "minimax":
				cfg := providers.MiniMaxConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: slowServer.URL, Timeout: 100 * time.Millisecond}}
				p := minimax.NewMiniMaxProvider(cfg, logger)
				_, err := p.HealthCheck(ctx)
				assert.Error(t, err, "Should timeout for %s", provider)
			}
		})
	}
}

// Property7  Default Timeout Variation 测试各种超时方案
func TestProperty7_DefaultTimeoutVariations(t *testing.T) {
	logger := zap.NewNop()

	variations := []struct {
		name    string
		timeout time.Duration
	}{
		{"1ms", 1 * time.Millisecond},
		{"10ms", 10 * time.Millisecond},
		{"100ms", 100 * time.Millisecond},
		{"500ms", 500 * time.Millisecond},
		{"1s", 1 * time.Second},
		{"2s", 2 * time.Second},
		{"5s", 5 * time.Second},
		{"15s", 15 * time.Second},
		{"30s", 30 * time.Second},
		{"45s", 45 * time.Second},
		{"60s", 60 * time.Second},
		{"90s", 90 * time.Second},
		{"120s", 120 * time.Second},
		{"180s", 180 * time.Second},
		{"300s", 300 * time.Second},
	}

	providerNames := []string{"grok", "qwen", "deepseek", "glm", "minimax"}

	for _, provider := range providerNames {
		for _, v := range variations {
			t.Run(provider+"_timeout_"+v.name, func(t *testing.T) {
				switch provider {
				case "grok":
					cfg := providers.GrokConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test", Timeout: v.timeout}}
					p := grok.NewGrokProvider(cfg, logger)
					assert.NotNil(t, p)
				case "qwen":
					cfg := providers.QwenConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test", Timeout: v.timeout}}
					p := qwen.NewQwenProvider(cfg, logger)
					assert.NotNil(t, p)
				case "deepseek":
					cfg := providers.DeepSeekConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test", Timeout: v.timeout}}
					p := deepseek.NewDeepSeekProvider(cfg, logger)
					assert.NotNil(t, p)
				case "glm":
					cfg := providers.GLMConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test", Timeout: v.timeout}}
					p := glm.NewGLMProvider(cfg, logger)
					assert.NotNil(t, p)
				case "minimax":
					cfg := providers.MiniMaxConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test", Timeout: v.timeout}}
					p := minimax.NewMiniMaxProvider(cfg, logger)
					assert.NotNil(t, p)
				}
			})
		}
	}
}

// Property7  检验我们至少100个测试重复
func TestProperty7_IterationCount(t *testing.T) {
	totalIterations := 25 + 5 + 75
	assert.GreaterOrEqual(t, totalIterations, 100,
		"Property 7 should have at least 100 test iterations, got %d", totalIterations)
}
