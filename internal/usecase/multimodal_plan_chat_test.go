package usecase

import (
	"context"
	"strings"
	"testing"
	"time"

	llm "github.com/BaSui01/agentflow/llm/core"
	llmgateway "github.com/BaSui01/agentflow/llm/gateway"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type scriptedMultimodalUsecaseProvider struct{}

func (m *scriptedMultimodalUsecaseProvider) Completion(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	_ = ctx
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = "gpt-4o-mini"
	}
	content := "single response"
	if len(req.Messages) > 0 {
		first := req.Messages[0].Content
		switch {
		case strings.Contains(first, "Create a visual production plan"):
			content = `{"goal":"launch teaser","shots":[{"purpose":"hook","visual":"close-up","action":"reveal product","camera":"dolly in","duration_sec":0}]}`
		case strings.Contains(first, "orchestration planner"):
			content = "1. gather facts\n2. produce answer"
		case strings.Contains(first, "executor agent"):
			content = "final answer"
		}
	}
	return &llm.ChatResponse{
		ID:    "scripted-usecase",
		Model: model,
		Choices: []llm.ChatChoice{{
			Index: 0,
			Message: types.Message{
				Role:    types.RoleAssistant,
				Content: content,
			},
		}},
		CreatedAt: time.Now(),
	}, nil
}

func (m *scriptedMultimodalUsecaseProvider) Stream(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	ch := make(chan llm.StreamChunk)
	close(ch)
	return ch, nil
}

func (m *scriptedMultimodalUsecaseProvider) HealthCheck(ctx context.Context) (*llm.HealthStatus, error) {
	return &llm.HealthStatus{Healthy: true}, nil
}

func (m *scriptedMultimodalUsecaseProvider) Name() string { return "scripted-multimodal-usecase" }

func (m *scriptedMultimodalUsecaseProvider) SupportsNativeFunctionCalling() bool { return true }

func (m *scriptedMultimodalUsecaseProvider) ListModels(ctx context.Context) ([]llm.Model, error) {
	return nil, nil
}

func (m *scriptedMultimodalUsecaseProvider) Endpoints() llm.ProviderEndpoints {
	return llm.ProviderEndpoints{}
}

func TestDefaultMultimodalService_GeneratePlan(t *testing.T) {
	gateway := llmgateway.New(llmgateway.Config{ChatProvider: &scriptedMultimodalUsecaseProvider{}})
	service := NewDefaultMultimodalService(MultimodalRuntime{
		Gateway:          gateway,
		ChatEnabled:      true,
		DefaultChatModel: "gpt-4o-mini",
	})

	result, err := service.GeneratePlan(context.Background(), MultimodalPlanRequest{Prompt: "launch teaser", ShotCount: 2, Advanced: true})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Plan)
	assert.Equal(t, "launch teaser", result.Plan.Goal)
	require.Len(t, result.Plan.Shots, 1)
	assert.Equal(t, 1, result.Plan.Shots[0].ID)
	assert.Equal(t, 3, result.Plan.Shots[0].DurationSec)
}

func TestDefaultMultimodalService_ChatAgentMode(t *testing.T) {
	gateway := llmgateway.New(llmgateway.Config{ChatProvider: &scriptedMultimodalUsecaseProvider{}})
	service := NewDefaultMultimodalService(MultimodalRuntime{
		Gateway:          gateway,
		ChatEnabled:      true,
		DefaultChatModel: "gpt-4o-mini",
	})

	result, err := service.Chat(context.Background(), MultimodalChatRequest{
		Messages:  []types.Message{{Role: types.RoleUser, Content: "draft a multimodal launch response"}},
		AgentMode: true,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "agent", result.Mode)
	assert.Equal(t, "1. gather facts\n2. produce answer", result.PlannerOutput)
	assert.Equal(t, "final answer", result.FinalText)
	require.NotNil(t, result.FinalResponse)
}
