package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/BaSui01/agentflow/llm/providers/hunyuan"
	"github.com/BaSui01/agentflow/llm/providers/kimi"
	"github.com/BaSui01/agentflow/llm/providers/llama"
	"github.com/BaSui01/agentflow/llm/providers/mistral"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestNewProviders_Compatibility 测试所有新提供程序是否与提供程序接口兼容
func TestNewProviders_Compatibility(t *testing.T) {
	logger := zap.NewNop()

	providers := []struct {
		name     string
		provider llm.Provider
	}{
		{
			name: "Mistral",
			provider: mistral.NewMistralProvider(providers.MistralConfig{
				APIKey: "test-key",
			}, logger),
		},
		{
			name: "Hunyuan",
			provider: hunyuan.NewHunyuanProvider(providers.HunyuanConfig{
				APIKey: "test-key",
			}, logger),
		},
		{
			name: "Kimi",
			provider: kimi.NewKimiProvider(providers.KimiConfig{
				APIKey: "test-key",
			}, logger),
		},
		{
			name: "Llama",
			provider: llama.NewLlamaProvider(providers.LlamaConfig{
				APIKey: "test-key",
			}, logger),
		},
	}

	for _, p := range providers {
		t.Run(p.name, func(t *testing.T) {
			// 测试接口合规性
			assert.NotEmpty(t, p.provider.Name())
			assert.True(t, p.provider.SupportsNativeFunctionCalling())
		})
	}
}

// TestNewProviders_ResilientWrapper 测试新提供程序是否与 ResilientProvider 配合使用
func TestNewProviders_ResilientWrapper(t *testing.T) {
	logger := zap.NewNop()
	ctx := context.Background()

	providers := []struct {
		name     string
		provider llm.Provider
		skip     bool
	}{
		{
			name: "Mistral",
			provider: mistral.NewMistralProvider(providers.MistralConfig{
				APIKey: os.Getenv("MISTRAL_API_KEY"),
			}, logger),
			skip: os.Getenv("MISTRAL_API_KEY") == "",
		},
		{
			name: "Hunyuan",
			provider: hunyuan.NewHunyuanProvider(providers.HunyuanConfig{
				APIKey: os.Getenv("HUNYUAN_API_KEY"),
			}, logger),
			skip: os.Getenv("HUNYUAN_API_KEY") == "",
		},
		{
			name: "Kimi",
			provider: kimi.NewKimiProvider(providers.KimiConfig{
				APIKey: os.Getenv("KIMI_API_KEY"),
			}, logger),
			skip: os.Getenv("KIMI_API_KEY") == "",
		},
		{
			name: "Llama",
			provider: llama.NewLlamaProvider(providers.LlamaConfig{
				APIKey: os.Getenv("TOGETHER_API_KEY"),
			}, logger),
			skip: os.Getenv("TOGETHER_API_KEY") == "",
		},
	}

	for _, p := range providers {
		t.Run(p.name, func(t *testing.T) {
			if p.skip {
				t.Skipf("%s API key not set", p.name)
			}

			// 与弹性提供商一起包裹
			resilient := llm.NewResilientProviderSimple(p.provider, nil, logger)

			// 测试基本完成
			req := &llm.ChatRequest{
				Messages: []llm.Message{
					{Role: llm.RoleUser, Content: "Hello"},
				},
				MaxTokens:   10,
				Temperature: 0.1,
			}

			resp, err := resilient.Completion(ctx, req)
			require.NoError(t, err)
			assert.NotNil(t, resp)
			assert.NotEmpty(t, resp.Choices)
		})
	}
}

// TestNewProviders_FunctionCalling 测试函数调用支持
func TestNewProviders_FunctionCalling(t *testing.T) {
	logger := zap.NewNop()
	ctx := context.Background()

	weatherTool := llm.ToolSchema{
		Name:        "get_weather",
		Description: "Get weather information",
		Parameters:  []byte(`{"type":"object","properties":{"location":{"type":"string"}},"required":["location"]}`),
	}

	providers := []struct {
		name     string
		provider llm.Provider
		skip     bool
	}{
		{
			name: "Mistral",
			provider: mistral.NewMistralProvider(providers.MistralConfig{
				APIKey: os.Getenv("MISTRAL_API_KEY"),
			}, logger),
			skip: os.Getenv("MISTRAL_API_KEY") == "",
		},
		{
			name: "Kimi",
			provider: kimi.NewKimiProvider(providers.KimiConfig{
				APIKey: os.Getenv("KIMI_API_KEY"),
			}, logger),
			skip: os.Getenv("KIMI_API_KEY") == "",
		},
	}

	for _, p := range providers {
		t.Run(p.name, func(t *testing.T) {
			if p.skip {
				t.Skipf("%s API key not set", p.name)
			}

			req := &llm.ChatRequest{
				Messages: []llm.Message{
					{Role: llm.RoleUser, Content: "What's the weather in Paris?"},
				},
				Tools:       []llm.ToolSchema{weatherTool},
				ToolChoice:  "auto",
				MaxTokens:   100,
				Temperature: 0.1,
			}

			resp, err := p.provider.Completion(ctx, req)
			require.NoError(t, err)
			assert.NotNil(t, resp)
		})
	}
}

// BenchmarkNewProviders 对所有新提供商进行基准测试
func BenchmarkNewProviders(b *testing.B) {
	logger := zap.NewNop()
	ctx := context.Background()

	providers := []struct {
		name     string
		provider llm.Provider
		skip     bool
	}{
		{
			name: "Mistral",
			provider: mistral.NewMistralProvider(providers.MistralConfig{
				APIKey:  os.Getenv("MISTRAL_API_KEY"),
				Timeout: 10 * time.Second,
			}, logger),
			skip: os.Getenv("MISTRAL_API_KEY") == "",
		},
		{
			name: "Hunyuan",
			provider: hunyuan.NewHunyuanProvider(providers.HunyuanConfig{
				APIKey:  os.Getenv("HUNYUAN_API_KEY"),
				Timeout: 10 * time.Second,
			}, logger),
			skip: os.Getenv("HUNYUAN_API_KEY") == "",
		},
		{
			name: "Kimi",
			provider: kimi.NewKimiProvider(providers.KimiConfig{
				APIKey:  os.Getenv("KIMI_API_KEY"),
				Timeout: 10 * time.Second,
			}, logger),
			skip: os.Getenv("KIMI_API_KEY") == "",
		},
		{
			name: "Llama",
			provider: llama.NewLlamaProvider(providers.LlamaConfig{
				APIKey:  os.Getenv("TOGETHER_API_KEY"),
				Timeout: 10 * time.Second,
			}, logger),
			skip: os.Getenv("TOGETHER_API_KEY") == "",
		},
	}

	for _, p := range providers {
		b.Run(p.name, func(b *testing.B) {
			if p.skip {
				b.Skipf("%s API key not set", p.name)
			}

			req := &llm.ChatRequest{
				Messages: []llm.Message{
					{Role: llm.RoleUser, Content: "Hi"},
				},
				MaxTokens: 5,
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := p.provider.Completion(ctx, req)
				if err != nil {
					b.Fatalf("Completion failed: %v", err)
				}
			}
		})
	}
}
