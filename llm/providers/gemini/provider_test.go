package gemini

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestGeminiProvider_Name(t *testing.T) {
	provider := NewGeminiProvider(providers.GeminiConfig{}, zap.NewNop())
	assert.Equal(t, "gemini", provider.Name())
}

func TestGeminiProvider_SupportsNativeFunctionCalling(t *testing.T) {
	provider := NewGeminiProvider(providers.GeminiConfig{}, zap.NewNop())
	assert.True(t, provider.SupportsNativeFunctionCalling())
}

func TestGeminiProvider_DefaultBaseURL(t *testing.T) {
	cfg := providers.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key"},
	}
	provider := NewGeminiProvider(cfg, zap.NewNop())
	assert.NotNil(t, provider)
}

func TestGeminiProvider_DefaultModel(t *testing.T) {
	model := providers.ChooseModel(nil, "", "gemini-3-pro")
	assert.Equal(t, "gemini-3-pro", model, "Default model should be Gemini 3 Pro (2026)")
}

func TestGeminiProvider_ThoughtSignaturesSupport(t *testing.T) {
	cfg := providers.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key"},
	}
	provider := NewGeminiProvider(cfg, zap.NewNop())
	assert.NotNil(t, provider)
}

func TestGeminiProvider_Integration(t *testing.T) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		t.Skip("GEMINI_API_KEY not set, skipping integration test")
	}

	provider := NewGeminiProvider(providers.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			APIKey:  apiKey,
			Model:   "gemini-2.0-flash-exp",
			Timeout: 30 * time.Second,
		},
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
			Model: "gemini-2.0-flash-exp",
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
			Model: "gemini-2.0-flash-exp",
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
