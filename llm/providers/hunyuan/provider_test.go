package hunyuan

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

func TestHunyuanProvider_Name(t *testing.T) {
	provider := NewHunyuanProvider(providers.HunyuanConfig{}, zap.NewNop())
	assert.Equal(t, "hunyuan", provider.Name())
}

func TestHunyuanProvider_SupportsNativeFunctionCalling(t *testing.T) {
	provider := NewHunyuanProvider(providers.HunyuanConfig{}, zap.NewNop())
	assert.True(t, provider.SupportsNativeFunctionCalling())
}

func TestHunyuanProvider_DefaultBaseURL(t *testing.T) {
	cfg := providers.HunyuanConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key"}}
	provider := NewHunyuanProvider(cfg, zap.NewNop())
	assert.NotNil(t, provider)
}

func TestHunyuanProvider_Integration(t *testing.T) {
	apiKey := os.Getenv("HUNYUAN_API_KEY")
	if apiKey == "" {
		t.Skip("HUNYUAN_API_KEY not set, skipping integration test")
	}

	provider := NewHunyuanProvider(providers.HunyuanConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			APIKey:  apiKey,
			Model:   "hunyuan-lite",
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
			Model: "hunyuan-lite",
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
			Model: "hunyuan-lite",
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
