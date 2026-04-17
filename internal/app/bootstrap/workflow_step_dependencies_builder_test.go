package bootstrap

import (
	"context"
	"testing"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/llm"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
	"github.com/BaSui01/agentflow/workflow/core"
	"github.com/BaSui01/agentflow/workflow/engine"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type workflowDepsTestAgent struct {
	id string
}

func (a *workflowDepsTestAgent) ID() string                     { return a.id }
func (a *workflowDepsTestAgent) Name() string                   { return a.id }
func (a *workflowDepsTestAgent) Type() agent.AgentType          { return agent.TypeGeneric }
func (a *workflowDepsTestAgent) State() agent.State             { return agent.StateReady }
func (a *workflowDepsTestAgent) Init(context.Context) error     { return nil }
func (a *workflowDepsTestAgent) Teardown(context.Context) error { return nil }
func (a *workflowDepsTestAgent) Plan(context.Context, *agent.Input) (*agent.PlanResult, error) {
	return nil, nil
}
func (a *workflowDepsTestAgent) Observe(context.Context, *agent.Feedback) error { return nil }
func (a *workflowDepsTestAgent) Execute(context.Context, *agent.Input) (*agent.Output, error) {
	return &agent.Output{Content: "ok"}, nil
}

type workflowDepsTestGateway struct {
	streamChunks []llmcore.UnifiedChunk
}

func (g workflowDepsTestGateway) Invoke(context.Context, *llmcore.UnifiedRequest) (*llmcore.UnifiedResponse, error) {
	return &llmcore.UnifiedResponse{
		Output: &llm.ChatResponse{
			Model: "test-model",
			Choices: []llm.ChatChoice{{
				Message: types.Message{Content: "ok"},
			}},
			Usage: types.ChatUsage{PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2},
		},
	}, nil
}

func (g workflowDepsTestGateway) Stream(context.Context, *llmcore.UnifiedRequest) (<-chan llmcore.UnifiedChunk, error) {
	ch := make(chan llmcore.UnifiedChunk, len(g.streamChunks))
	for _, chunk := range g.streamChunks {
		ch <- chunk
	}
	close(ch)
	return ch, nil
}

func TestBuildStepDependencies_InjectsAgentResolverForOrchestration(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	expected := &workflowDepsTestAgent{id: "agent-orch"}
	deps := buildStepDependencies(WorkflowRuntimeOptions{
		AgentResolver: func(ctx context.Context, agentID string) (agent.Agent, error) {
			require.Equal(t, "agent-orch", agentID)
			return expected, nil
		},
	}, zap.NewNop())

	require.NotNil(t, deps.AgentExecutor)
	require.NotNil(t, deps.AgentResolver)

	resolved, err := deps.AgentResolver.ResolveAgent(ctx, "agent-orch")
	require.NoError(t, err)
	require.Same(t, expected, resolved)

	node, err := engine.BuildExecutionNode(engine.StepSpec{
		ID:                     "orch-step",
		Type:                   core.StepTypeOrchestration,
		OrchestrationMode:      "round_robin",
		OrchestrationAgents:    []string{"agent-orch"},
		OrchestrationMaxRounds: 1,
	}, deps)
	require.NoError(t, err)
	require.NoError(t, node.Step.Validate())
}

func TestWorkflowGatewayAdapter_StreamMapsGatewayChunks(t *testing.T) {
	t.Parallel()

	reasoning := "plan"
	adapter := newWorkflowGatewayAdapter(workflowDepsTestGateway{
		streamChunks: []llmcore.UnifiedChunk{
			{
				Output: &llm.StreamChunk{
					Model: "test-model",
					Delta: types.Message{
						Content:          "hi",
						ReasoningContent: &reasoning,
					},
				},
			},
			{
				Usage: &llmcore.Usage{
					PromptTokens:     1,
					CompletionTokens: 2,
					TotalTokens:      3,
				},
			},
		},
	}, "fallback-model")

	stream, err := adapter.Stream(context.Background(), &core.LLMRequest{Model: "test-model", Prompt: "hello"})
	require.NoError(t, err)

	var chunks []core.LLMStreamChunk
	for chunk := range stream {
		chunks = append(chunks, chunk)
	}

	require.Len(t, chunks, 3)
	require.Equal(t, "hi", chunks[0].Delta)
	require.NotNil(t, chunks[0].ReasoningContent)
	require.Equal(t, "plan", *chunks[0].ReasoningContent)
	require.Equal(t, 3, chunks[2].Usage.TotalTokens)
	require.True(t, chunks[2].Done)
}
