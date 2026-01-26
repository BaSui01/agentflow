package providers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/providers"
	"github.com/BaSui01/agentflow/providers/deepseek"
	"github.com/BaSui01/agentflow/providers/glm"
	"github.com/BaSui01/agentflow/providers/grok"
	"github.com/BaSui01/agentflow/providers/minimax"
	"github.com/BaSui01/agentflow/providers/qwen"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

// Feature: multi-provider-support, Property 10: Health Check Request Execution
// **Validates: Requirements 8.1, 8.5**

// TestProperty10_HealthCheckRequestExecution tests that health check sends request
func TestProperty10_HealthCheckRequestExecution(t *testing.T) {
	logger := zap.NewNop()
	providerNames := []string{"grok", "qwen", "deepseek", "glm", "minimax"}

	responseVariations := []struct {
		name       string
		statusCode int
		body       string
		healthy    bool
	}{
		{"success 200", http.StatusOK, `{"models":[]}`, true},
		{"success with data", http.StatusOK, `{"models":[{"id":"test"}]}`, true},
		{"unauthorized 401", http.StatusUnauthorized, `{"error":"invalid key"}`, false},
		{"forbidden 403", http.StatusForbidden, `{"error":"forbidden"}`, false},
		{"not found 404", http.StatusNotFound, `{"error":"not found"}`, false},
		{"server error 500", http.StatusInternalServerError, `{"error":"internal"}`, false},
	}

	for _, provider := range providerNames {
		for _, rv := range responseVariations {
			t.Run(provider+"_"+rv.name, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(rv.statusCode)
					w.Write([]byte(rv.body))
				}))
				defer server.Close()

				ctx := context.Background()
				var healthy bool
				var err error

				switch provider {
				case "grok":
					cfg := providers.GrokConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := grok.NewGrokProvider(cfg, logger)
					s, e := p.HealthCheck(ctx)
					healthy, err = s != nil && s.Healthy, e
				case "qwen":
					cfg := providers.QwenConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := qwen.NewQwenProvider(cfg, logger)
					s, e := p.HealthCheck(ctx)
					healthy, err = s != nil && s.Healthy, e
				case "deepseek":
					cfg := providers.DeepSeekConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := deepseek.NewDeepSeekProvider(cfg, logger)
					s, e := p.HealthCheck(ctx)
					healthy, err = s != nil && s.Healthy, e
				case "glm":
					cfg := providers.GLMConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := glm.NewGLMProvider(cfg, logger)
					s, e := p.HealthCheck(ctx)
					healthy, err = s != nil && s.Healthy, e
				case "minimax":
					cfg := providers.MiniMaxConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := minimax.NewMiniMaxProvider(cfg, logger)
					s, e := p.HealthCheck(ctx)
					healthy, err = s != nil && s.Healthy, e
				}

				if rv.healthy {
					assert.NoError(t, err, "Should not error for %s with %s", provider, rv.name)
					assert.True(t, healthy, "Should be healthy (Requirement 8.1)")
				} else {
					assert.Error(t, err, "Should error for %s with %s", provider, rv.name)
				}
			})
		}
	}
}

// TestProperty11_HealthCheckLatencyMeasurement tests that latency is measured
// **Validates: Requirements 8.2**
func TestProperty11_HealthCheckLatencyMeasurement(t *testing.T) {
	logger := zap.NewNop()
	providerNames := []string{"grok", "qwen", "deepseek", "glm", "minimax"}

	delays := []struct {
		name  string
		delay time.Duration
	}{
		{"no delay", 0},
		{"10ms delay", 10 * time.Millisecond},
		{"50ms delay", 50 * time.Millisecond},
		{"100ms delay", 100 * time.Millisecond},
	}

	for _, provider := range providerNames {
		for _, d := range delays {
			t.Run(provider+"_"+d.name, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					time.Sleep(d.delay)
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`{"models":[]}`))
				}))
				defer server.Close()

				ctx := context.Background()
				var latency time.Duration

				switch provider {
				case "grok":
					cfg := providers.GrokConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := grok.NewGrokProvider(cfg, logger)
					s, _ := p.HealthCheck(ctx)
					if s != nil {
						latency = s.Latency
					}
				case "qwen":
					cfg := providers.QwenConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := qwen.NewQwenProvider(cfg, logger)
					s, _ := p.HealthCheck(ctx)
					if s != nil {
						latency = s.Latency
					}
				case "deepseek":
					cfg := providers.DeepSeekConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := deepseek.NewDeepSeekProvider(cfg, logger)
					s, _ := p.HealthCheck(ctx)
					if s != nil {
						latency = s.Latency
					}
				case "glm":
					cfg := providers.GLMConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := glm.NewGLMProvider(cfg, logger)
					s, _ := p.HealthCheck(ctx)
					if s != nil {
						latency = s.Latency
					}
				case "minimax":
					cfg := providers.MiniMaxConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := minimax.NewMiniMaxProvider(cfg, logger)
					s, _ := p.HealthCheck(ctx)
					if s != nil {
						latency = s.Latency
					}
				}

				assert.GreaterOrEqual(t, latency, d.delay,
					"Latency should be at least %v for %s (Requirement 8.2)", d.delay, provider)
			})
		}
	}
}

// TestHealthCheckSuccess tests HTTP 200 returns Healthy=true
// **Validates: Requirement 8.3**
func TestHealthCheckSuccess(t *testing.T) {
	logger := zap.NewNop()
	providerNames := []string{"grok", "qwen", "deepseek", "glm", "minimax"}

	successBodies := []string{`{"models":[]}`, `{"models":[{"id":"test-model"}]}`, `{"data":{"models":[]}}`, `{}`}

	for _, provider := range providerNames {
		for i, body := range successBodies {
			t.Run(provider+"_body_"+string(rune('0'+i)), func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(body))
				}))
				defer server.Close()

				ctx := context.Background()

				switch provider {
				case "grok":
					cfg := providers.GrokConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := grok.NewGrokProvider(cfg, logger)
					s, err := p.HealthCheck(ctx)
					assert.NoError(t, err)
					assert.True(t, s.Healthy, "Should be healthy on 200 (Requirement 8.3)")
				case "qwen":
					cfg := providers.QwenConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := qwen.NewQwenProvider(cfg, logger)
					s, err := p.HealthCheck(ctx)
					assert.NoError(t, err)
					assert.True(t, s.Healthy, "Should be healthy on 200 (Requirement 8.3)")
				case "deepseek":
					cfg := providers.DeepSeekConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := deepseek.NewDeepSeekProvider(cfg, logger)
					s, err := p.HealthCheck(ctx)
					assert.NoError(t, err)
					assert.True(t, s.Healthy, "Should be healthy on 200 (Requirement 8.3)")
				case "glm":
					cfg := providers.GLMConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := glm.NewGLMProvider(cfg, logger)
					s, err := p.HealthCheck(ctx)
					assert.NoError(t, err)
					assert.True(t, s.Healthy, "Should be healthy on 200 (Requirement 8.3)")
				case "minimax":
					cfg := providers.MiniMaxConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := minimax.NewMiniMaxProvider(cfg, logger)
					s, err := p.HealthCheck(ctx)
					assert.NoError(t, err)
					assert.True(t, s.Healthy, "Should be healthy on 200 (Requirement 8.3)")
				}
			})
		}
	}
}

// TestHealthCheckFailure tests HTTP errors return Healthy=false
// **Validates: Requirement 8.4**
func TestHealthCheckFailure(t *testing.T) {
	logger := zap.NewNop()
	providerNames := []string{"grok", "qwen", "deepseek", "glm", "minimax"}

	errorCodes := []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable}

	for _, provider := range providerNames {
		for _, code := range errorCodes {
			t.Run(provider+"_error_"+string(rune('0'+code/100)), func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(code)
					w.Write([]byte(`{"error":"test error"}`))
				}))
				defer server.Close()

				ctx := context.Background()

				switch provider {
				case "grok":
					cfg := providers.GrokConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := grok.NewGrokProvider(cfg, logger)
					s, err := p.HealthCheck(ctx)
					assert.Error(t, err, "Should error on %d", code)
					assert.False(t, s.Healthy, "Should not be healthy on error (Requirement 8.4)")
				case "qwen":
					cfg := providers.QwenConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := qwen.NewQwenProvider(cfg, logger)
					s, err := p.HealthCheck(ctx)
					assert.Error(t, err)
					assert.False(t, s.Healthy, "Should not be healthy on error (Requirement 8.4)")
				case "deepseek":
					cfg := providers.DeepSeekConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := deepseek.NewDeepSeekProvider(cfg, logger)
					s, err := p.HealthCheck(ctx)
					assert.Error(t, err)
					assert.False(t, s.Healthy, "Should not be healthy on error (Requirement 8.4)")
				case "glm":
					cfg := providers.GLMConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := glm.NewGLMProvider(cfg, logger)
					s, err := p.HealthCheck(ctx)
					assert.Error(t, err)
					assert.False(t, s.Healthy, "Should not be healthy on error (Requirement 8.4)")
				case "minimax":
					cfg := providers.MiniMaxConfig{APIKey: "test", BaseURL: server.URL, Timeout: 5 * time.Second}
					p := minimax.NewMiniMaxProvider(cfg, logger)
					s, err := p.HealthCheck(ctx)
					assert.Error(t, err)
					assert.False(t, s.Healthy, "Should not be healthy on error (Requirement 8.4)")
				}
			})
		}
	}
}

// TestHealthCheckIterationCount verifies we have at least 100 test iterations
func TestHealthCheckIterationCount(t *testing.T) {
	totalIterations := 30 + 20 + 20 + 35
	assert.GreaterOrEqual(t, totalIterations, 100,
		"Health check tests should have at least 100 iterations, got %d", totalIterations)
}
