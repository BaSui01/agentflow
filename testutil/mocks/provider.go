package mocks

import (
	"context"
	"time"

	llm "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
)

type successProvider struct {
	content string
}

func NewSuccessProvider(content string) llm.Provider {
	return &successProvider{content: content}
}

func (p *successProvider) Completion(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	return &llm.ChatResponse{
		Model: req.Model,
		Choices: []llm.ChatChoice{
			{
				Index: 0,
				Message: types.Message{
					Role:    types.RoleAssistant,
					Content: p.content,
				},
			},
		},
		Usage: llm.ChatUsage{},
	}, nil
}

func (p *successProvider) Stream(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	ch := make(chan llm.StreamChunk, 1)
	ch <- llm.StreamChunk{
		Model: req.Model,
		Delta: types.Message{
			Role:    types.RoleAssistant,
			Content: p.content,
		},
	}
	close(ch)
	return ch, nil
}

func (p *successProvider) HealthCheck(ctx context.Context) (*llm.HealthStatus, error) {
	return &llm.HealthStatus{Healthy: true, Latency: 1 * time.Millisecond}, nil
}

func (p *successProvider) Name() string                        { return "test-success-provider" }
func (p *successProvider) SupportsNativeFunctionCalling() bool { return true }
func (p *successProvider) ListModels(ctx context.Context) ([]llm.Model, error) {
	return []llm.Model{{ID: "test-model"}}, nil
}
func (p *successProvider) Endpoints() llm.ProviderEndpoints {
	return llm.ProviderEndpoints{}
}
