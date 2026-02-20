package openai

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

func TestOpenAIProvider_Name(t *testing.T) {
	provider := NewOpenAIProvider(providers.OpenAIConfig{}, zap.NewNop())
	assert.Equal(t, "openai", provider.Name())
}

func TestOpenAIProvider_SupportsNativeFunctionCalling(t *testing.T) {
	provider := NewOpenAIProvider(providers.OpenAIConfig{}, zap.NewNop())
	assert.True(t, provider.SupportsNativeFunctionCalling())
}

func TestOpenAIProvider_DefaultBaseURL(t *testing.T) {
	cfg := providers.OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key"},
	}
	provider := NewOpenAIProvider(cfg, zap.NewNop())
	assert.NotNil(t, provider)
}

func TestOpenAIProvider_DefaultModel(t *testing.T) {
	req := &llm.ChatRequest{}
	cfg := providers.OpenAIConfig{}
	provider := NewOpenAIProvider(cfg, zap.NewNop())
	
	model := providers.ChooseModel(req, provider.openaiCfg.Model, "gpt-5.2")
	assert.Equal(t, "gpt-5.2", model, "Default model should be GPT-5.2 (2026)")
}

func TestOpenAIProvider_ResponsesAPISupport(t *testing.T) {
	cfg := providers.OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key"},
		UseResponsesAPI:    true,
	}
	provider := NewOpenAIProvider(cfg, zap.NewNop())
	assert.NotNil(t, provider)
	assert.True(t, provider.openaiCfg.UseResponsesAPI)
}

func TestOpenAIProvider_Integration(t *testing.T) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set, skipping integration test")
	}

	provider := NewOpenAIProvider(providers.OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			APIKey:  apiKey,
			Model:   "gpt-4o-mini",
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
			Model: "gpt-4o-mini",
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
			Model: "gpt-4o-mini",
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

	t.Run("ThoughtSignatures", func(t *testing.T) {
		req := &llm.ChatRequest{
			Model: "gpt-4o-mini",
			Messages: []llm.Message{
				{Role: llm.RoleUser, Content: "What is 2+2?"},
			},
			MaxTokens:         50,
			ThoughtSignatures: []string{"test-signature"},
		}

		resp, err := provider.Completion(ctx, req)
		require.NoError(t, err)
		assert.NotNil(t, resp)
		assert.NotEmpty(t, resp.Choices)
	})
}
