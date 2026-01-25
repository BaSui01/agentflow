package claude

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

func TestClaudeProvider_Name(t *testing.T) {
	provider := NewClaudeProvider(providers.ClaudeConfig{}, zap.NewNop())
	assert.Equal(t, "claude", provider.Name())
}

func TestClaudeProvider_SupportsNativeFunctionCalling(t *testing.T) {
	provider := NewClaudeProvider(providers.ClaudeConfig{}, zap.NewNop())
	assert.True(t, provider.SupportsNativeFunctionCalling())
}

func TestClaudeProvider_DefaultBaseURL(t *testing.T) {
	cfg := providers.ClaudeConfig{
		APIKey: "test-key",
	}
	provider := NewClaudeProvider(cfg, zap.NewNop())
	assert.NotNil(t, provider)
}

func TestClaudeProvider_DefaultModel(t *testing.T) {
	model := chooseClaudeModel(nil, "")
	assert.Equal(t, "claude-opus-4.5-20260105", model, "Default model should be Claude Opus 4.5 (2026)")
}

func TestClaudeProvider_ReasoningModeSupport(t *testing.T) {
	cfg := providers.ClaudeConfig{
		APIKey: "test-key",
	}
	provider := NewClaudeProvider(cfg, zap.NewNop())
	assert.NotNil(t, provider)
}

func TestClaudeProvider_Integration(t *testing.T) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		t.Skip("ANTHROPIC_API_KEY not set, skipping integration test")
	}

	provider := NewClaudeProvider(providers.ClaudeConfig{
		APIKey:  apiKey,
		Model:   "claude-3-5-sonnet-20241022",
		Timeout: 60 * time.Second,
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
			Model: "claude-3-5-sonnet-20241022",
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
			Model: "claude-3-5-sonnet-20241022",
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

	t.Run("ReasoningMode", func(t *testing.T) {
		req := &llm.ChatRequest{
			Model: "claude-3-5-sonnet-20241022",
			Messages: []llm.Message{
				{Role: llm.RoleUser, Content: "What is 2+2?"},
			},
			MaxTokens:     50,
			ReasoningMode: "extended",
		}

		resp, err := provider.Completion(ctx, req)
		require.NoError(t, err)
		assert.NotNil(t, resp)
		assert.NotEmpty(t, resp.Choices)
	})
}
