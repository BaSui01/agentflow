package vendor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewChatProviderFromConfig_BuiltInProviders(t *testing.T) {
	tests := []struct {
		name         string
		providerName string
		wantName     string
	}{
		{name: "openai", providerName: "openai", wantName: "openai"},
		{name: "anthropic", providerName: "anthropic", wantName: "claude"},
		{name: "claude alias", providerName: "claude", wantName: "claude"},
		{name: "gemini", providerName: "gemini", wantName: "gemini"},
		{name: "deepseek", providerName: "deepseek", wantName: "deepseek"},
		{name: "qwen", providerName: "qwen", wantName: "qwen"},
		{name: "glm", providerName: "glm", wantName: "glm"},
		{name: "grok", providerName: "grok", wantName: "grok"},
		{name: "kimi", providerName: "kimi", wantName: "kimi"},
		{name: "mistral", providerName: "mistral", wantName: "mistral"},
		{name: "minimax", providerName: "minimax", wantName: "minimax"},
		{name: "hunyuan", providerName: "hunyuan", wantName: "hunyuan"},
		{name: "doubao", providerName: "doubao", wantName: "doubao"},
		{name: "llama", providerName: "llama", wantName: "llama-together"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := NewChatProviderFromConfig(tt.providerName, ChatProviderConfig{APIKey: "sk-test"}, zap.NewNop())
			require.NoError(t, err)
			require.NotNil(t, p)
			assert.Equal(t, tt.wantName, p.Name())
		})
	}
}

func TestNewChatProviderFromConfig_GenericOpenAICompat(t *testing.T) {
	p, err := NewChatProviderFromConfig("my-provider", ChatProviderConfig{
		APIKey:  "sk-test",
		BaseURL: "https://api.example.com",
		Model:   "x-model",
		Extra: map[string]any{
			"endpoint_path": "/v1/chat/completions",
		},
	}, zap.NewNop())
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.Equal(t, "my-provider", p.Name())
}

func TestNewChatProviderFromConfig_GenericOpenAICompatRequiresBaseURL(t *testing.T) {
	_, err := NewChatProviderFromConfig("unknown-provider", ChatProviderConfig{
		APIKey: "sk-test",
	}, zap.NewNop())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "base_url is required")
}

func TestNewChatProviderFromConfig_LlamaProviderExtra(t *testing.T) {
	p, err := NewChatProviderFromConfig("llama", ChatProviderConfig{
		APIKey: "sk-test",
		Extra: map[string]any{
			"provider": "openrouter",
		},
	}, zap.NewNop())
	require.NoError(t, err)
	assert.Equal(t, "llama-openrouter", p.Name())
}

func TestNewChatProviderFromConfig_GeminiVertexAlias(t *testing.T) {
	p, err := NewChatProviderFromConfig("gemini-vertex", ChatProviderConfig{
		APIKey: "sk-test",
	}, zap.NewNop())
	require.NoError(t, err)
	assert.Equal(t, "gemini", p.Name())
}
