package kimi

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

func TestKimiProvider_Name(t *testing.T) {
	provider := NewKimiProvider(providers.KimiConfig{}, zap.NewNop())
	assert.Equal(t, "kimi", provider.Name())
}

func TestKimiProvider_SupportsNativeFunctionCalling(t *testing.T) {
	provider := NewKimiProvider(providers.KimiConfig{}, zap.NewNop())
	assert.True(t, provider.SupportsNativeFunctionCalling())
}

func TestKimiProvider_DefaultBaseURL(t *testing.T) {
	cfg := providers.KimiConfig{
		APIKey: "test-key",
	}
	provider := NewKimiProvider(cfg, zap.NewNop())
	assert.NotNil(t, provider)
}

func TestKimiProvider_Integration(t *testing.T) {
	apiKey := os.Getenv("KIMI_API_KEY")
	if apiKey == "" {
		t.Skip("KIMI_API_KEY not set, skipping integration test")
	}

	provider := NewKimiProvider(providers.KimiConfig{
		APIKey:  apiKey,
		Model:   "moonshot-v1-8k",
		Timeout: 30 * time.Second,
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
			Model: "moonshot-v1-8k",
			Messages: []llm.Message{
				{Role: llm.RoleUser, Content: "你好"},
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
			Model: "moonshot-v1-8k",
			Messages: []llm.Message{
				{Role: llm.RoleUser, Content: "数到3"},
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
