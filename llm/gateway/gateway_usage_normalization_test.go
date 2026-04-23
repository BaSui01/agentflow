package gateway

import (
	"context"
	"testing"
	"time"

	llmcore "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/require"
)

func TestNormalizeUsage_FillsTotals(t *testing.T) {
	usage := normalizeUsage(llmcore.Usage{
		PromptTokens:     7,
		CompletionTokens: 3,
		TotalTokens:      0,
		InputUnits:       1,
		OutputUnits:      2,
		TotalUnits:       0,
	})
	require.Equal(t, 10, usage.TotalTokens)
	require.Equal(t, 2, usage.TotalUnits)
}

func TestService_Invoke_NormalizesChatUsage(t *testing.T) {
	service := New(Config{
		ChatProvider: &normalizationProvider{},
	})

	resp, err := service.Invoke(context.Background(), &llmcore.UnifiedRequest{
		Capability: llmcore.CapabilityChat,
		ModelHint:  "gpt-4o-mini",
		Payload: &llmcore.ChatRequest{
			Model: "gpt-4o-mini",
			Messages: []types.Message{
				{Role: types.RoleUser, Content: "hello"},
			},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, 10, resp.Usage.TotalTokens)
}

func TestService_Stream_NormalizesChunkUsage(t *testing.T) {
	service := New(Config{
		ChatProvider: &normalizationProvider{},
	})

	ch, err := service.Stream(context.Background(), &llmcore.UnifiedRequest{
		Capability: llmcore.CapabilityChat,
		ModelHint:  "gpt-4o-mini",
		Payload: &llmcore.ChatRequest{
			Model: "gpt-4o-mini",
			Messages: []types.Message{
				{Role: types.RoleUser, Content: "hello"},
			},
		},
	})
	require.NoError(t, err)

	first, ok := <-ch
	require.True(t, ok)
	require.NotNil(t, first.Usage)
	require.Equal(t, 3, first.Usage.TotalTokens)
}

type normalizationProvider struct{}

func (p *normalizationProvider) Completion(ctx context.Context, req *llmcore.ChatRequest) (*llmcore.ChatResponse, error) {
	return &llmcore.ChatResponse{
		ID:       "resp_1",
		Provider: "normalization-provider",
		Model:    req.Model,
		Choices: []llmcore.ChatChoice{
			{
				Index: 0,
				Message: types.Message{
					Role:    types.RoleAssistant,
					Content: "ok",
				},
			},
		},
		Usage: llmcore.ChatUsage{
			PromptTokens:     7,
			CompletionTokens: 3,
			TotalTokens:      0,
		},
		CreatedAt: time.Now(),
	}, nil
}

func (p *normalizationProvider) Stream(ctx context.Context, req *llmcore.ChatRequest) (<-chan llmcore.StreamChunk, error) {
	out := make(chan llmcore.StreamChunk, 1)
	out <- llmcore.StreamChunk{
		ID:       "chunk_1",
		Provider: "normalization-provider",
		Model:    req.Model,
		Delta: types.Message{
			Role:    types.RoleAssistant,
			Content: "partial",
		},
		Usage: &llmcore.ChatUsage{
			PromptTokens:     2,
			CompletionTokens: 1,
			TotalTokens:      0,
		},
	}
	close(out)
	return out, nil
}

func (p *normalizationProvider) HealthCheck(ctx context.Context) (*llmcore.HealthStatus, error) {
	return &llmcore.HealthStatus{Healthy: true}, nil
}

func (p *normalizationProvider) Name() string { return "normalization-provider" }

func (p *normalizationProvider) SupportsNativeFunctionCalling() bool { return false }

func (p *normalizationProvider) ListModels(ctx context.Context) ([]llmcore.Model, error) {
	return nil, nil
}

func (p *normalizationProvider) Endpoints() llmcore.ProviderEndpoints {
	return llmcore.ProviderEndpoints{}
}
