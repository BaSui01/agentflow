package factory

import (
	"sync"
	"testing"

	"github.com/BaSui01/agentflow/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// =============================================================================
// Factory Tests
// =============================================================================

func TestNewProviderFromConfig_AllProviders(t *testing.T) {
	logger := zap.NewNop()

	tests := []struct {
		name         string
		providerName string
		cfg          ProviderConfig
		wantName     string
	}{
		{
			name:         "openai",
			providerName: "openai",
			cfg:          ProviderConfig{APIKey: "sk-test"},
			wantName:     "openai",
		},
		{
			name:         "anthropic",
			providerName: "anthropic",
			cfg:          ProviderConfig{APIKey: "sk-test"},
			wantName:     "claude",
		},
		{
			name:         "claude alias",
			providerName: "claude",
			cfg:          ProviderConfig{APIKey: "sk-test"},
			wantName:     "claude",
		},
		{
			name:         "gemini",
			providerName: "gemini",
			cfg:          ProviderConfig{APIKey: "sk-test"},
			wantName:     "gemini",
		},
		{
			name:         "deepseek",
			providerName: "deepseek",
			cfg:          ProviderConfig{APIKey: "sk-test"},
			wantName:     "deepseek",
		},
		{
			name:         "qwen",
			providerName: "qwen",
			cfg:          ProviderConfig{APIKey: "sk-test"},
			wantName:     "qwen",
		},
		{
			name:         "glm",
			providerName: "glm",
			cfg:          ProviderConfig{APIKey: "sk-test"},
			wantName:     "glm",
		},
		{
			name:         "grok",
			providerName: "grok",
			cfg:          ProviderConfig{APIKey: "sk-test"},
			wantName:     "grok",
		},
		{
			name:         "kimi",
			providerName: "kimi",
			cfg:          ProviderConfig{APIKey: "sk-test"},
			wantName:     "kimi",
		},
		{
			name:         "mistral",
			providerName: "mistral",
			cfg:          ProviderConfig{APIKey: "sk-test"},
			wantName:     "mistral",
		},
		{
			name:         "minimax",
			providerName: "minimax",
			cfg:          ProviderConfig{APIKey: "sk-test"},
			wantName:     "minimax",
		},
		{
			name:         "hunyuan",
			providerName: "hunyuan",
			cfg:          ProviderConfig{APIKey: "sk-test"},
			wantName:     "hunyuan",
		},
		{
			name:         "doubao",
			providerName: "doubao",
			cfg:          ProviderConfig{APIKey: "sk-test"},
			wantName:     "doubao",
		},
		{
			name:         "llama",
			providerName: "llama",
			cfg:          ProviderConfig{APIKey: "sk-test"},
			wantName:     "llama-together",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := NewProviderFromConfig(tt.providerName, tt.cfg, logger)
			require.NoError(t, err)
			require.NotNil(t, p)
			assert.Equal(t, tt.wantName, p.Name())
		})
	}
}

func TestNewProviderFromConfig_UnknownProvider(t *testing.T) {
	_, err := NewProviderFromConfig("nonexistent", ProviderConfig{}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown provider")
}

func TestNewProviderFromConfig_OpenAIExtras(t *testing.T) {
	p, err := NewProviderFromConfig("openai", ProviderConfig{
		APIKey: "sk-test",
		Extra: map[string]any{
			"organization":      "org-123",
			"use_responses_api": true,
		},
	}, nil)
	require.NoError(t, err)
	assert.Equal(t, "openai", p.Name())
}

func TestNewProviderFromConfig_LlamaExtras(t *testing.T) {
	p, err := NewProviderFromConfig("llama", ProviderConfig{
		APIKey: "sk-test",
		Extra: map[string]any{
			"provider": "openrouter",
		},
	}, nil)
	require.NoError(t, err)
	assert.Equal(t, "llama-openrouter", p.Name())
}

func TestNewProviderFromConfig_NilLogger(t *testing.T) {
	p, err := NewProviderFromConfig("deepseek", ProviderConfig{APIKey: "sk-test"}, nil)
	require.NoError(t, err)
	require.NotNil(t, p)
}

func TestNewProviderFromConfig_NilExtras(t *testing.T) {
	p, err := NewProviderFromConfig("openai", ProviderConfig{APIKey: "sk-test"}, nil)
	require.NoError(t, err)
	assert.Equal(t, "openai", p.Name())
}

func TestSupportedProviders(t *testing.T) {
	names := SupportedProviders()
	assert.GreaterOrEqual(t, len(names), 13)
	assert.Contains(t, names, "openai")
	assert.Contains(t, names, "claude")
	assert.Contains(t, names, "llama")
}

// =============================================================================
// Registry Tests
// =============================================================================

func TestProviderRegistry_RegisterAndGet(t *testing.T) {
	reg := llm.NewProviderRegistry()
	p, _ := NewProviderFromConfig("deepseek", ProviderConfig{APIKey: "sk-test"}, nil)

	reg.Register("deepseek", p)

	got, ok := reg.Get("deepseek")
	assert.True(t, ok)
	assert.Equal(t, "deepseek", got.Name())

	_, ok = reg.Get("nonexistent")
	assert.False(t, ok)
}

func TestProviderRegistry_DefaultProvider(t *testing.T) {
	reg := llm.NewProviderRegistry()
	p, _ := NewProviderFromConfig("deepseek", ProviderConfig{APIKey: "sk-test"}, nil)
	reg.Register("deepseek", p)

	// No default set yet
	_, err := reg.Default()
	require.Error(t, err)

	// Set default
	err = reg.SetDefault("deepseek")
	require.NoError(t, err)

	got, err := reg.Default()
	require.NoError(t, err)
	assert.Equal(t, "deepseek", got.Name())

	// Set default to unregistered name
	err = reg.SetDefault("nonexistent")
	require.Error(t, err)
}

func TestProviderRegistry_List(t *testing.T) {
	reg := llm.NewProviderRegistry()
	p1, _ := NewProviderFromConfig("deepseek", ProviderConfig{APIKey: "sk-test"}, nil)
	p2, _ := NewProviderFromConfig("qwen", ProviderConfig{APIKey: "sk-test"}, nil)

	reg.Register("deepseek", p1)
	reg.Register("qwen", p2)

	names := reg.List()
	assert.Equal(t, []string{"deepseek", "qwen"}, names)
}

func TestProviderRegistry_Unregister(t *testing.T) {
	reg := llm.NewProviderRegistry()
	p, _ := NewProviderFromConfig("deepseek", ProviderConfig{APIKey: "sk-test"}, nil)
	reg.Register("deepseek", p)
	reg.SetDefault("deepseek")

	reg.Unregister("deepseek")

	_, ok := reg.Get("deepseek")
	assert.False(t, ok)
	assert.Equal(t, 0, reg.Len())

	// Default should be cleared
	_, err := reg.Default()
	require.Error(t, err)
}

func TestProviderRegistry_Len(t *testing.T) {
	reg := llm.NewProviderRegistry()
	assert.Equal(t, 0, reg.Len())

	p, _ := NewProviderFromConfig("deepseek", ProviderConfig{APIKey: "sk-test"}, nil)
	reg.Register("deepseek", p)
	assert.Equal(t, 1, reg.Len())
}

func TestProviderRegistry_ConcurrentAccess(t *testing.T) {
	reg := llm.NewProviderRegistry()
	var wg sync.WaitGroup

	// Concurrent writes
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			p, _ := NewProviderFromConfig("deepseek", ProviderConfig{APIKey: "sk-test"}, nil)
			name := "provider-" + string(rune('a'+idx%26))
			reg.Register(name, p)
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			reg.List()
			reg.Len()
			reg.Get("deepseek")
		}()
	}

	wg.Wait()
	// No panic = pass
}
