package factory

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewProviderFromConfig_GeminiVertex(t *testing.T) {
	p, err := NewProviderFromConfig("gemini-vertex", ProviderConfig{
		APIKey: "sk-test",
		Extra: map[string]any{
			"project_id": "my-project",
			"region":     "us-central1",
		},
	}, zap.NewNop())
	require.NoError(t, err)
	assert.Equal(t, "gemini", p.Name())
}

func TestNewProviderFromConfig_GeminiExtras(t *testing.T) {
	p, err := NewProviderFromConfig("gemini", ProviderConfig{
		APIKey: "sk-test",
		Extra: map[string]any{
			"project_id": "proj",
			"region":     "eu-west1",
			"auth_type":  "oauth",
		},
	}, zap.NewNop())
	require.NoError(t, err)
	assert.Equal(t, "gemini", p.Name())
}

func TestNewProviderFromConfig_AnthropicExtras(t *testing.T) {
	p, err := NewProviderFromConfig("anthropic", ProviderConfig{
		APIKey: "sk-test",
		Extra: map[string]any{
			"auth_type":          "api_key",
			"anthropic_version":  "2024-01-01",
		},
	}, zap.NewNop())
	require.NoError(t, err)
	assert.Equal(t, "claude", p.Name())
}

func TestNewProviderFromConfig_GenericOpenAICompat(t *testing.T) {
	p, err := NewProviderFromConfig("my-custom-provider", ProviderConfig{
		APIKey:  "sk-test",
		BaseURL: "https://api.custom.com",
		Model:   "custom-model",
		Extra: map[string]any{
			"endpoint_path":   "/v1/completions",
			"models_endpoint": "/v1/models",
			"auth_header":     "X-API-Key",
			"supports_tools":  true,
		},
	}, zap.NewNop())
	require.NoError(t, err)
	assert.Equal(t, "my-custom-provider", p.Name())
}

func TestNewProviderFromConfig_GenericOpenAICompat_WithAPIKeys(t *testing.T) {
	p, err := NewProviderFromConfig("ollama", ProviderConfig{
		BaseURL: "http://localhost:11434",
		Extra: map[string]any{
			"api_keys": []any{"key1", "key2"},
		},
	}, zap.NewNop())
	require.NoError(t, err)
	assert.Equal(t, "ollama", p.Name())
}

func TestNewProviderFromConfig_GenericOpenAICompat_NoBaseURL(t *testing.T) {
	_, err := NewProviderFromConfig("unknown-provider", ProviderConfig{
		APIKey: "sk-test",
	}, zap.NewNop())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "base_url is required")
}

