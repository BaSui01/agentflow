package vendor

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	claude "github.com/BaSui01/agentflow/llm/providers/anthropic"
	"github.com/BaSui01/agentflow/llm/providers/gemini"
	"github.com/BaSui01/agentflow/llm/providers/openai"
	"github.com/BaSui01/agentflow/llm/providers/openaicompat"
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
		{name: "openai responses alias", providerName: "openai-responses", wantName: "openai"},
		{name: "anthropic", providerName: "anthropic", wantName: "claude"},
		{name: "claude alias", providerName: "claude", wantName: "claude"},
		{name: "anthropic sdk alias", providerName: "anthropic-sdk-go", wantName: "claude"},
		{name: "gemini", providerName: "gemini", wantName: "gemini"},
		{name: "google genai alias", providerName: "google-genai", wantName: "gemini"},
		{name: "vertex ai alias", providerName: "vertex-ai", wantName: "gemini"},
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

func TestNewChatProviderFromConfig_OpenAIResponsesAlias(t *testing.T) {
	p, err := NewChatProviderFromConfig("openai-responses", ChatProviderConfig{
		APIKey:  "sk-test",
		BaseURL: "https://api.example.com",
	}, zap.NewNop())
	require.NoError(t, err)

	openAIProvider, ok := p.(*openai.OpenAIProvider)
	require.True(t, ok)
	assert.Equal(t, "https://api.example.com/v1/responses", openAIProvider.Endpoints().Completion)
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
		Extra: map[string]any{
			"project_id": "demo-project",
			"region":     "asia-east1",
		},
	}, zap.NewNop())
	require.NoError(t, err)
	assert.Equal(t, "gemini", p.Name())
	assert.Contains(t, p.Endpoints().Completion, "/v1/projects/demo-project/locations/asia-east1/publishers/google/models/")
}

func TestNewChatProviderFromConfig_VertexAIAlias(t *testing.T) {
	p, err := NewChatProviderFromConfig("vertex-ai", ChatProviderConfig{
		APIKey: "oauth-token",
		Extra: map[string]any{
			"project_id": "demo-project",
			"region":     "us-central1",
		},
	}, zap.NewNop())
	require.NoError(t, err)
	assert.Equal(t, "gemini", p.Name())
	assert.Contains(t, p.Endpoints().Completion, "/v1/projects/demo-project/locations/us-central1/publishers/google/models/")
}

func TestNewChatProviderFromConfig_OpenAIExtras(t *testing.T) {
	p, err := NewChatProviderFromConfig("openai", ChatProviderConfig{
		APIKey:  "sk-test",
		BaseURL: "https://api.example.com",
		Extra: map[string]any{
			"organization":      "org-test",
			"use_responses_api": true,
		},
	}, zap.NewNop())
	require.NoError(t, err)

	openAIProvider, ok := p.(*openai.OpenAIProvider)
	require.True(t, ok)
	assert.Equal(t, "https://api.example.com/v1/responses", openAIProvider.Endpoints().Completion)

	req, err := openAIProvider.NewRequest(context.Background(), http.MethodPost, "/v1/chat/completions", nil, "sk-test")
	require.NoError(t, err)
	assert.Equal(t, "Bearer sk-test", req.Header.Get("Authorization"))
	assert.Equal(t, "org-test", req.Header.Get("OpenAI-Organization"))
}

func TestNewChatProviderFromConfig_AnthropicVersionExtra(t *testing.T) {
	var capturedVersion string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedVersion = r.Header.Get("anthropic-version")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[]}`)
	}))
	t.Cleanup(server.Close)

	p, err := NewChatProviderFromConfig("anthropic", ChatProviderConfig{
		APIKey:  "sk-test",
		BaseURL: server.URL,
		Extra: map[string]any{
			"anthropic_version": "2024-01-01",
		},
	}, zap.NewNop())
	require.NoError(t, err)

	claudeProvider, ok := p.(*claude.ClaudeProvider)
	require.True(t, ok)
	_, err = claudeProvider.ListModels(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "2024-01-01", capturedVersion)
}

func TestNewChatProviderFromConfig_GeminiOAuthExtra(t *testing.T) {
	var capturedAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"models":[]}`)
	}))
	t.Cleanup(server.Close)

	p, err := NewChatProviderFromConfig("gemini", ChatProviderConfig{
		APIKey:  "oauth-token",
		BaseURL: server.URL,
		Extra: map[string]any{
			"auth_type": "oauth",
		},
	}, zap.NewNop())
	require.NoError(t, err)

	geminiProvider, ok := p.(*gemini.GeminiProvider)
	require.True(t, ok)
	_, err = geminiProvider.ListModels(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "Bearer oauth-token", capturedAuth)
}

func TestNewChatProviderFromConfig_GenericOpenAICompatExtraConfig(t *testing.T) {
	p, err := NewChatProviderFromConfig("custom-provider", ChatProviderConfig{
		APIKey:  "sk-test",
		BaseURL: "https://compat.example.com",
		Model:   "compat-model",
		Extra: map[string]any{
			"endpoint_path":   "/v2/chat",
			"models_endpoint": "/v2/models",
			"auth_header":     "X-API-Key",
			"supports_tools":  false,
		},
	}, zap.NewNop())
	require.NoError(t, err)

	compatProvider, ok := p.(*openaicompat.Provider)
	require.True(t, ok)
	assert.Equal(t, "https://compat.example.com/v2/chat", compatProvider.Endpoints().Completion)
	assert.Equal(t, "https://compat.example.com/v2/models", compatProvider.Endpoints().Models)
	assert.False(t, compatProvider.SupportsNativeFunctionCalling())

	req, err := compatProvider.NewRequest(context.Background(), http.MethodPost, "/v2/chat", nil, "sk-test")
	require.NoError(t, err)
	assert.Equal(t, "sk-test", req.Header.Get("X-API-Key"))
	assert.Empty(t, req.Header.Get("Authorization"))
}
