package runtime

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type runtimeHandoffFakeAgent struct {
	id        string
	name      string
	lastInput *Input
	executeFn func(ctx context.Context, input *Input) (*Output, error)
}

func (a *runtimeHandoffFakeAgent) ID() string {
	return a.id
}

func (a *runtimeHandoffFakeAgent) Name() string {
	if a.name != "" {
		return a.name
	}
	return a.id
}

func (a *runtimeHandoffFakeAgent) Type() AgentType { return TypeAssistant }
func (a *runtimeHandoffFakeAgent) State() State    { return StateReady }
func (a *runtimeHandoffFakeAgent) Init(context.Context) error {
	return nil
}
func (a *runtimeHandoffFakeAgent) Teardown(context.Context) error {
	return nil
}
func (a *runtimeHandoffFakeAgent) Plan(context.Context, *Input) (*PlanResult, error) {
	return nil, nil
}
func (a *runtimeHandoffFakeAgent) Execute(ctx context.Context, input *Input) (*Output, error) {
	a.lastInput = input
	if a.executeFn != nil {
		return a.executeFn(ctx, input)
	}
	return &Output{Content: "handled by " + a.ID()}, nil
}
func (a *runtimeHandoffFakeAgent) Observe(context.Context, *Feedback) error {
	return nil
}

func TestRuntimeHandoffTargetsFromContext_NormalizesDedupesAndSkipsCurrent(t *testing.T) {
	target := &runtimeHandoffFakeAgent{id: "target-agent", name: "Target Agent"}
	current := &runtimeHandoffFakeAgent{id: "owner-agent", name: "Owner Agent"}

	ctx := WithRuntimeHandoffTargets(context.Background(), []RuntimeHandoffTarget{
		{Agent: target},
		{Agent: nil},
		{Agent: target, Description: "duplicate ignored"},
		{Agent: current},
	})

	got := runtimeHandoffTargetsFromContext(ctx, "owner-agent")

	require.Len(t, got, 1)
	assert.Same(t, target, got[0].Agent)
	assert.Equal(t, "transfer_to_target_agent", got[0].ToolName)
	assert.Equal(t, "Handoff to the Target Agent agent to continue handling the request.", got[0].Description)
}

func TestRuntimeHandoffExecutor_ExecuteRoutesHandoffAndReturnsControlPayload(t *testing.T) {
	target := &runtimeHandoffFakeAgent{id: "target-agent", name: "Target Agent"}
	owner := &BaseAgent{
		config: types.AgentConfig{Core: types.CoreConfig{ID: "owner-agent", Name: "Owner Agent"}},
		logger: zap.NewNop(),
	}
	executor := newRuntimeHandoffExecutor(owner, toolManagerExecutor{}, []RuntimeHandoffTarget{{Agent: target}})
	ctx := WithRuntimeConversationMessages(context.Background(), []types.Message{{
		Role:    types.RoleUser,
		Content: "original question",
	}})

	result := executor.ExecuteOne(ctx, types.ToolCall{
		ID:        "call-1",
		Name:      "transfer_to_target_agent",
		Arguments: json.RawMessage(`{"input":"please continue"}`),
	})

	require.Empty(t, result.Error)
	require.NotNil(t, target.lastInput)
	assert.Equal(t, "please continue", target.lastInput.Content)
	assert.Equal(t, "owner-agent", target.lastInput.ChannelID)

	control := result.Control()
	require.NotNil(t, control)
	require.NotNil(t, control.Handoff)
	assert.Equal(t, types.ToolResultControlTypeHandoff, control.Type)
	assert.Equal(t, "owner-agent", control.Handoff.FromAgentID)
	assert.Equal(t, "target-agent", control.Handoff.ToAgentID)
	assert.Equal(t, "Target Agent", control.Handoff.ToAgentName)
	assert.Equal(t, "handled by target-agent", control.Handoff.Output)
}
