package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/internal/usecase"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type routingAwareAgent struct {
	lastExecuteCtx context.Context
	lastPlanCtx    context.Context
	output         *agent.Output
	executeCalls   int
}

func (a *routingAwareAgent) ID() string                     { return "routing-agent" }
func (a *routingAwareAgent) Name() string                   { return "routing-agent" }
func (a *routingAwareAgent) Type() agent.AgentType          { return agent.TypeAssistant }
func (a *routingAwareAgent) State() agent.State             { return agent.StateReady }
func (a *routingAwareAgent) Init(context.Context) error     { return nil }
func (a *routingAwareAgent) Teardown(context.Context) error { return nil }
func (a *routingAwareAgent) Observe(context.Context, *agent.Feedback) error {
	return nil
}

func (a *routingAwareAgent) Plan(ctx context.Context, _ *agent.Input) (*agent.PlanResult, error) {
	a.lastPlanCtx = ctx
	return &agent.PlanResult{Steps: []string{"s1"}}, nil
}

func (a *routingAwareAgent) Execute(ctx context.Context, _ *agent.Input) (*agent.Output, error) {
	a.lastExecuteCtx = ctx
	a.executeCalls++
	if a.output != nil {
		return a.output, nil
	}
	return &agent.Output{
		TraceID:      "trace-1",
		Content:      "ok",
		TokensUsed:   12,
		Cost:         0,
		Duration:     time.Millisecond,
		FinishReason: "stop",
		Metadata: map[string]any{
			"current_stage":           "planning",
			"iteration_count":         2,
			"selected_reasoning_mode": "deep",
			"stop_reason":             "solved",
			"checkpoint_id":           "ckpt-1",
			"resumable":               true,
		},
	}, nil
}

func TestAgentService_ExecuteAgent_AppliesRoutingContext(t *testing.T) {
	ag := &routingAwareAgent{}
	svc := usecase.NewDefaultAgentService(nil, func(ctx context.Context, _ string) (agent.Agent, error) {
		return ag, nil
	})

	resp, _, err := svc.ExecuteAgent(context.Background(), usecase.AgentExecuteRequest{
		AgentID:     "routing-agent",
		Content:     "hello",
		Model:       "gpt-4o",
		Provider:    "openai",
		RoutePolicy: "health_first",
		Metadata:    map[string]string{"tenant": "t1"},
		Tags:        []string{"prod", "prod"},
	}, "trace-1")
	require.Nil(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, ag.lastExecuteCtx)

	model, ok := types.LLMModel(ag.lastExecuteCtx)
	require.True(t, ok)
	assert.Equal(t, "gpt-4o", model)

	provider, ok := types.LLMProvider(ag.lastExecuteCtx)
	require.True(t, ok)
	assert.Equal(t, "openai", provider)

	routePolicy, ok := types.LLMRoutePolicy(ag.lastExecuteCtx)
	require.True(t, ok)
	assert.Equal(t, "health_first", routePolicy)

	rc := agent.GetRunConfig(ag.lastExecuteCtx)
	require.NotNil(t, rc)
	require.NotNil(t, rc.Model)
	require.NotNil(t, rc.Provider)
	require.NotNil(t, rc.RoutePolicy)
	assert.Equal(t, "gpt-4o", *rc.Model)
	assert.Equal(t, "openai", *rc.Provider)
	assert.Equal(t, "health_first", *rc.RoutePolicy)
	assert.Equal(t, "openai", rc.Metadata["chat_provider"])
	assert.Equal(t, "health_first", rc.Metadata["route_policy"])
	assert.Equal(t, "t1", rc.Metadata["tenant"])
	assert.Equal(t, []string{"prod"}, rc.Tags)
	assert.Equal(t, "planning", resp.CurrentStage)
	assert.Equal(t, 2, resp.IterationCount)
	assert.Equal(t, "deep", resp.SelectedReasoningMode)
	assert.Equal(t, "solved", resp.StopReason)
	assert.Equal(t, "ckpt-1", resp.CheckpointID)
	assert.True(t, resp.Resumable)
}

func TestAgentService_ExecuteAgent_PrefersOutputFieldsOverMetadata(t *testing.T) {
	ag := &routingAwareAgent{
		output: &agent.Output{
			TraceID:               "trace-1",
			Content:               "ok",
			TokensUsed:            12,
			Cost:                  0,
			Duration:              time.Millisecond,
			FinishReason:          "finish-stop",
			CurrentStage:          "respond",
			IterationCount:        4,
			SelectedReasoningMode: "tree_of_thought",
			StopReason:            "max_iterations",
			CheckpointID:          "ckpt-top-level",
			Resumable:             true,
			Metadata: map[string]any{
				"current_stage":           "planning",
				"iteration_count":         2,
				"selected_reasoning_mode": "deep",
				"stop_reason":             "solved",
				"checkpoint_id":           "ckpt-meta",
				"resumable":               false,
			},
		},
	}
	svc := usecase.NewDefaultAgentService(nil, func(ctx context.Context, _ string) (agent.Agent, error) {
		return ag, nil
	})

	resp, _, err := svc.ExecuteAgent(context.Background(), usecase.AgentExecuteRequest{
		AgentID: "routing-agent",
		Content: "hello",
	}, "trace-1")
	require.Nil(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "respond", resp.CurrentStage)
	assert.Equal(t, 4, resp.IterationCount)
	assert.Equal(t, "tree_of_thought", resp.SelectedReasoningMode)
	assert.Equal(t, "max_iterations", resp.StopReason)
	assert.Equal(t, "ckpt-top-level", resp.CheckpointID)
	assert.True(t, resp.Resumable)
}

func TestAgentService_PlanAgent_AppliesRoutingContext(t *testing.T) {
	ag := &routingAwareAgent{}
	svc := usecase.NewDefaultAgentService(nil, func(ctx context.Context, _ string) (agent.Agent, error) {
		return ag, nil
	})

	plan, err := svc.PlanAgent(context.Background(), usecase.AgentExecuteRequest{
		AgentID:     "routing-agent",
		Content:     "hello",
		RoutePolicy: "latency_first",
	}, "trace-1")
	require.Nil(t, err)
	require.NotNil(t, plan)
	require.NotNil(t, ag.lastPlanCtx)

	rc := agent.GetRunConfig(ag.lastPlanCtx)
	require.NotNil(t, rc)
	require.NotNil(t, rc.RoutePolicy)
	assert.Equal(t, "latency_first", *rc.RoutePolicy)
}

func TestAgentService_ExecuteAgent_MultiAgentDefaultsToParallel(t *testing.T) {
	agents := map[string]*routingAwareAgent{
		"agent-1": {},
		"agent-2": {},
	}
	svc := usecase.NewDefaultAgentService(nil, func(ctx context.Context, id string) (agent.Agent, error) {
		ag, ok := agents[id]
		if !ok {
			return nil, assert.AnError
		}
		return ag, nil
	})

	resp, _, err := svc.ExecuteAgent(context.Background(), usecase.AgentExecuteRequest{
		AgentIDs: []string{"agent-1", "agent-2"},
		Content:  "hello",
	}, "trace-2")
	require.Nil(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "ok\n\nok", resp.Content)
	assert.Equal(t, "parallel", resp.Metadata["mode"])
	assert.Equal(t, 2, resp.Metadata["agent_count"])
	require.NotNil(t, agents["agent-1"].lastExecuteCtx)
	require.NotNil(t, agents["agent-2"].lastExecuteCtx)
}

func TestAgentService_ExecuteAgent_SingleAgentDoesNotRouteThroughModeLoop(t *testing.T) {
	ag := &routingAwareAgent{
		output: &agent.Output{
			TraceID: "trace-single",
			Content: "single-agent-result",
			Metadata: map[string]any{
				"mode": "single-agent-direct",
			},
		},
	}
	svc := usecase.NewDefaultAgentService(nil, func(ctx context.Context, _ string) (agent.Agent, error) {
		return ag, nil
	})

	resp, _, err := svc.ExecuteAgent(context.Background(), usecase.AgentExecuteRequest{
		AgentID: "routing-agent",
		Mode:    "loop",
		Content: "hello",
	}, "trace-single")
	require.Nil(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 1, ag.executeCalls)
	assert.Equal(t, "single-agent-result", resp.Content)
	assert.Equal(t, "single-agent-direct", resp.Metadata["mode"])
	assert.NotEqual(t, "loop", resp.Metadata["mode"])
	assert.NotEqual(t, "reasoning", resp.Metadata["mode"])
}

func TestAgentService_SupportedExecutionModes_ContainsParallel(t *testing.T) {
	assert.Contains(t, usecase.SupportedExecutionModes(), "parallel")
	assert.True(t, usecase.IsSupportedExecutionMode("parallel"))
	assert.False(t, usecase.IsSupportedExecutionMode("unsupported"))
}
