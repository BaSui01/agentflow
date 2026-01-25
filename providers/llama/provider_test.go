package llama

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestLlamaProvider_Name(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		expected string
	}{
		{"Together", "together", "llama-together"},
		{"Replicate", "replicate", "llama-replicate"},
		{"OpenRouter", "openrouter", "llama-openrouter"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewLlamaProvider(providers.LlamaConfig{
				Provider: tt.provider,
			}, zap.NewNop())
			assert.Equal(t, tt.expected, provider.Name())
		})
	}
}

func TestLlamaProvider_SupportsNativeFunctionCalling(t *testing.T) {
	provider := NewLlamaProvider(providers.LlamaConfig{}, zap.NewNop())
	assert.True(t, provider.SupportsNativeFunctionCalling())
}

func TestLlamaProvider_DefaultProvider(t *testing.T) {
	cfg := providers.LlamaConfig{
		APIKey: "test-key",
	}
	provider := NewLlamaProvider(cfg, zap.NewNop())
	assert.Equal(t, "llama-together", provider.Name())
}

func TestLlamaProvider_BaseURLSelection(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		expected string
	}{
		{"Together", "together", "https://api.together.xyz/v1"},
		{"Replicate", "replicate", "https://api.replicate.com/v1"},
		{"OpenRouter", "openrouter", "https://openrouter.ai/api/v1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := providers.LlamaConfig{
				APIKey:   "test-key",
				Provider: tt.provider,
			}
			provider := NewLlamaProvider(cfg, zap.NewNop())
			assert.NotNil(t, provider)
		})
	}
}

func TestLlamaProvider_Integration(t *testing.T) {
	apiKey := os.Getenv("TOGETHER_API_KEY")
	if apiKey == "" {
		t.Skip("TOGETHER_API_KEY not set, skipping integration test")
	}

	provider := NewLlamaProvider(providers.LlamaConfig{
		APIKey:   apiKey,
		Model:    "meta-llama/Llama-3.3-70B-Instruct-Turbo",
		Provider: "together",
		Timeout:  30 * time.Second,
	}, zap.NewNop())

	ctx := context.Background()

	t.Run("HealthCheck", func(t *testing.T) {
		status, err := provider.HealthCheck(ctx)
		require.NoError(t, err)
		assert.True(t, status.Healthy)
		assert.Greater(t, status.Latency, time.Duration(0))
	})

	t.Run("Completion", func(t *testing.T) {
		req := &llm.ChatRequest{
			Model: "meta-llama/Llama-3.3-70B-Instruct-Turbo",
			Messages: []llm.Message{
				{Role: llm.RoleUser, Content: "Say 'test' only"},
			},
			MaxTokens:   10,
			Temperature: 0.1,
		}

		resp, err := provider.Completion(ctx, req)
		require.NoError(t, err)
		assert.NotNil(t, resp)
		assert.NotEmpty(t, resp.Choices)
		assert.NotEmpty(t, resp.Choices[0].Message.Content)
	})

	t.Run("Stream", func(t *testing.T) {
		req := &llm.ChatRequest{
			Model: "meta-llama/Llama-3.3-70B-Instruct-Turbo",
			Messages: []llm.Message{
				{Role: llm.RoleUser, Content: "Count to 3"},
			},
			MaxTokens: 20,
		}

		stream, err := provider.Stream(ctx, req)
		require.NoError(t, err)

		var chunks []llm.StreamChunk
		for chunk := range stream {
			if chunk.Err != nil {
				t.Fatalf("Stream error: %v", chunk.Err)
			}
			chunks = append(chunks, chunk)
		}

		assert.NotEmpty(t, chunks)
	})
}
