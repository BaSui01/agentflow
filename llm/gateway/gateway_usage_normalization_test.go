package gateway

import (
	"context"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/llm"
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
		Payload: &llm.ChatRequest{
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
		Payload: &llm.ChatRequest{
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

func (p *normalizationProvider) Completion(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	return &llm.ChatResponse{
		ID:       "resp_1",
		Provider: "normalization-provider",
		Model:    req.Model,
		Choices: []llm.ChatChoice{
			{
				Index: 0,
				Message: types.Message{
					Role:    types.RoleAssistant,
					Content: "ok",
				},
			},
		},
		Usage: llm.ChatUsage{
			PromptTokens:     7,
			CompletionTokens: 3,
			TotalTokens:      0,
		},
		CreatedAt: time.Now(),
	}, nil
}

func (p *normalizationProvider) Stream(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	out := make(chan llm.StreamChunk, 1)
	out <- llm.StreamChunk{
		ID:       "chunk_1",
		Provider: "normalization-provider",
		Model:    req.Model,
		Delta: types.Message{
			Role:    types.RoleAssistant,
			Content: "partial",
		},
		Usage: &llm.ChatUsage{
			PromptTokens:     2,
			CompletionTokens: 1,
			TotalTokens:      0,
		},
	}
	close(out)
	return out, nil
}

func (p *normalizationProvider) HealthCheck(ctx context.Context) (*llm.HealthStatus, error) {
	return &llm.HealthStatus{Healthy: true}, nil
}

func (p *normalizationProvider) Name() string { return "normalization-provider" }

func (p *normalizationProvider) SupportsNativeFunctionCalling() bool { return false }

func (p *normalizationProvider) ListModels(ctx context.Context) ([]llm.Model, error) { return nil, nil }

func (p *normalizationProvider) Endpoints() llm.ProviderEndpoints { return llm.ProviderEndpoints{} }
