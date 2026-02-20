package workflow

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/BaSui01/agentflow/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================
// Mock implementations for AgentAdapter tests
// ============================================================

// mockAgentInterface implements AgentInterface for testing.
type mockAgentInterface struct {
	id        string
	name      string
	response  string
	err       error
	calls     int
	lastInput string
}

func (a *mockAgentInterface) Execute(ctx context.Context, input string) (string, error) {
	a.calls++
	a.lastInput = input
	if a.err != nil {
		return "", a.err
	}
	return a.response, nil
}

func (a *mockAgentInterface) ID() string   { return a.id }
func (a *mockAgentInterface) Name() string { return a.name }

// ============================================================
// AgentAdapter tests
// ============================================================

func TestAgentAdapter_Execute(t *testing.T) {
	agent := &mockAgentInterface{
		id:       "agent-1",
		name:     "test-agent",
		response: "agent output",
	}

	adapter := NewAgentAdapter(agent)

	assert.Equal(t, "agent-1", adapter.ID())
	assert.Equal(t, "test-agent", adapter.Name())

	result, err := adapter.Execute(context.Background(), "hello")
	require.NoError(t, err)
	assert.Equal(t, "agent output", result)
	assert.Equal(t, "hello", agent.lastInput)
}

func TestAgentAdapter_Execute_DefaultInputConversion(t *testing.T) {
	agent := &mockAgentInterface{
		id:       "agent-1",
		name:     "test-agent",
		response: "ok",
	}

	adapter := NewAgentAdapter(agent)

	// Non-string input gets converted via fmt.Sprintf
	result, err := adapter.Execute(context.Background(), 42)
	require.NoError(t, err)
	assert.Equal(t, "ok", result)
	assert.Equal(t, "42", agent.lastInput)
}

func TestAgentAdapter_Execute_CustomInputMapper(t *testing.T) {
	agent := &mockAgentInterface{
		id:       "agent-1",
		name:     "test-agent",
		response: "ok",
	}

	adapter := NewAgentAdapter(agent, WithAgentInputMapper(func(input any) (string, error) {
		m, ok := input.(map[string]string)
		if !ok {
			return "", fmt.Errorf("expected map input")
		}
		return m["query"], nil
	}))

	result, err := adapter.Execute(context.Background(), map[string]string{"query": "search term"})
	require.NoError(t, err)
	assert.Equal(t, "ok", result)
	assert.Equal(t, "search term", agent.lastInput)
}

func TestAgentAdapter_Execute_CustomOutputMapper(t *testing.T) {
	agent := &mockAgentInterface{
		id:       "agent-1",
		name:     "test-agent",
		response: "raw output",
	}

	adapter := NewAgentAdapter(agent, WithAgentOutputMapper(func(output string) (any, error) {
		return map[string]string{"result": output}, nil
	}))

	result, err := adapter.Execute(context.Background(), "input")
	require.NoError(t, err)
	m, ok := result.(map[string]string)
	require.True(t, ok)
	assert.Equal(t, "raw output", m["result"])
}

func TestAgentAdapter_Execute_AgentError(t *testing.T) {
	agent := &mockAgentInterface{
		id:   "agent-1",
		name: "test-agent",
		err:  errors.New("agent failed"),
	}

	adapter := NewAgentAdapter(agent)

	_, err := adapter.Execute(context.Background(), "input")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agent execution failed")
	assert.Contains(t, err.Error(), "agent failed")
}

func TestAgentAdapter_Execute_InputMapperError(t *testing.T) {
	agent := &mockAgentInterface{
		id:       "agent-1",
		name:     "test-agent",
		response: "ok",
	}

	adapter := NewAgentAdapter(agent, WithAgentInputMapper(func(input any) (string, error) {
		return "", errors.New("bad input")
	}))

	_, err := adapter.Execute(context.Background(), "input")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "input mapping failed")
}

func TestAgentAdapter_AsAgentExecutor(t *testing.T) {
	agent := &mockAgentInterface{
		id:       "agent-1",
		name:     "test-agent",
		response: "step output",
	}

	adapter := NewAgentAdapter(agent)

	// Verify it satisfies AgentExecutor interface
	var executor AgentExecutor = adapter
	assert.Equal(t, "agent-1", executor.ID())
	assert.Equal(t, "test-agent", executor.Name())

	result, err := executor.Execute(context.Background(), "step input")
	require.NoError(t, err)
	assert.Equal(t, "step output", result)
}

func TestAgentAdapter_InAgentStep(t *testing.T) {
	agent := &mockAgentInterface{
		id:       "agent-1",
		name:     "test-agent",
		response: "agent result",
	}

	adapter := NewAgentAdapter(agent)
	step := NewAgentStep(adapter)

	assert.Equal(t, "agent:test-agent", step.Name())
	assert.Equal(t, "agent-1", step.AgentID())

	result, err := step.Execute(context.Background(), "workflow input")
	require.NoError(t, err)
	assert.Equal(t, "agent result", result)
}

// ============================================================
// Mock agent.Agent for NativeAgentAdapter tests
// ============================================================

// mockNativeAgent implements agent.Agent for testing.
type mockNativeAgent struct {
	id        string
	name      string
	output    *agent.Output
	err       error
	calls     int
	lastInput *agent.Input
}

func (a *mockNativeAgent) ID() string              { return a.id }
func (a *mockNativeAgent) Name() string             { return a.name }
func (a *mockNativeAgent) Type() agent.AgentType    { return agent.TypeGeneric }
func (a *mockNativeAgent) State() agent.State       { return agent.StateReady }
func (a *mockNativeAgent) Init(context.Context) error    { return nil }
func (a *mockNativeAgent) Teardown(context.Context) error { return nil }
func (a *mockNativeAgent) Plan(context.Context, *agent.Input) (*agent.PlanResult, error) {
	return nil, nil
}
func (a *mockNativeAgent) Observe(context.Context, *agent.Feedback) error { return nil }

func (a *mockNativeAgent) Execute(ctx context.Context, input *agent.Input) (*agent.Output, error) {
	a.calls++
	a.lastInput = input
	if a.err != nil {
		return nil, a.err
	}
	return a.output, nil
}

// ============================================================
// NativeAgentAdapter tests
// ============================================================

func TestNativeAgentAdapter_Execute_StringInput(t *testing.T) {
	mock := &mockNativeAgent{
		id:   "native-1",
		name: "native-agent",
		output: &agent.Output{
			Content: "hello from native agent",
		},
	}

	adapter := NewNativeAgentAdapter(mock)

	assert.Equal(t, "native-1", adapter.ID())
	assert.Equal(t, "native-agent", adapter.Name())

	result, err := adapter.Execute(context.Background(), "test prompt")
	require.NoError(t, err)

	out, ok := result.(*agent.Output)
	require.True(t, ok)
	assert.Equal(t, "hello from native agent", out.Content)
	assert.Equal(t, "test prompt", mock.lastInput.Content)
	assert.Equal(t, 1, mock.calls)
}

func TestNativeAgentAdapter_Execute_NativeInput(t *testing.T) {
	mock := &mockNativeAgent{
		id:     "native-1",
		name:   "native-agent",
		output: &agent.Output{Content: "ok"},
	}

	adapter := NewNativeAgentAdapter(mock)

	inp := &agent.Input{
		TraceID: "trace-123",
		Content: "direct input",
		Context: map[string]any{"key": "value"},
	}

	result, err := adapter.Execute(context.Background(), inp)
	require.NoError(t, err)

	out, ok := result.(*agent.Output)
	require.True(t, ok)
	assert.Equal(t, "ok", out.Content)
	// Should pass through the *agent.Input directly
	assert.Equal(t, "trace-123", mock.lastInput.TraceID)
	assert.Equal(t, "direct input", mock.lastInput.Content)
	assert.Equal(t, "value", mock.lastInput.Context["key"])
}

func TestNativeAgentAdapter_Execute_MapInput(t *testing.T) {
	mock := &mockNativeAgent{
		id:     "native-1",
		name:   "native-agent",
		output: &agent.Output{Content: "ok"},
	}

	adapter := NewNativeAgentAdapter(mock)

	mapInput := map[string]any{
		"content":    "map content",
		"trace_id":   "trace-456",
		"tenant_id":  "tenant-1",
		"user_id":    "user-1",
		"channel_id": "channel-1",
		"context":    map[string]any{"extra": "data"},
		"variables":  map[string]string{"var1": "val1"},
	}

	result, err := adapter.Execute(context.Background(), mapInput)
	require.NoError(t, err)

	out, ok := result.(*agent.Output)
	require.True(t, ok)
	assert.Equal(t, "ok", out.Content)
	assert.Equal(t, "map content", mock.lastInput.Content)
	assert.Equal(t, "trace-456", mock.lastInput.TraceID)
	assert.Equal(t, "tenant-1", mock.lastInput.TenantID)
	assert.Equal(t, "user-1", mock.lastInput.UserID)
	assert.Equal(t, "channel-1", mock.lastInput.ChannelID)
	assert.Equal(t, "data", mock.lastInput.Context["extra"])
	assert.Equal(t, "val1", mock.lastInput.Variables["var1"])
}

func TestNativeAgentAdapter_Execute_NilInput(t *testing.T) {
	mock := &mockNativeAgent{
		id:     "native-1",
		name:   "native-agent",
		output: &agent.Output{Content: "ok"},
	}

	adapter := NewNativeAgentAdapter(mock)

	result, err := adapter.Execute(context.Background(), nil)
	require.NoError(t, err)

	out, ok := result.(*agent.Output)
	require.True(t, ok)
	assert.Equal(t, "ok", out.Content)
	assert.NotNil(t, mock.lastInput)
	assert.Equal(t, "", mock.lastInput.Content)
}

func TestNativeAgentAdapter_Execute_AgentError(t *testing.T) {
	mock := &mockNativeAgent{
		id:   "native-1",
		name: "native-agent",
		err:  errors.New("native agent failed"),
	}

	adapter := NewNativeAgentAdapter(mock)

	_, err := adapter.Execute(context.Background(), "input")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "NativeAgentAdapter: agent execution failed")
	assert.Contains(t, err.Error(), "native agent failed")
}

func TestNativeAgentAdapter_Execute_NonStringInput(t *testing.T) {
	mock := &mockNativeAgent{
		id:     "native-1",
		name:   "native-agent",
		output: &agent.Output{Content: "ok"},
	}

	adapter := NewNativeAgentAdapter(mock)

	// Integer input gets converted to string via fmt.Sprintf
	result, err := adapter.Execute(context.Background(), 42)
	require.NoError(t, err)

	out, ok := result.(*agent.Output)
	require.True(t, ok)
	assert.Equal(t, "ok", out.Content)
	assert.Equal(t, "42", mock.lastInput.Content)
}

func TestNativeAgentAdapter_ImplementsAgentExecutor(t *testing.T) {
	mock := &mockNativeAgent{
		id:     "native-1",
		name:   "native-agent",
		output: &agent.Output{Content: "ok"},
	}

	var executor AgentExecutor = NewNativeAgentAdapter(mock)
	assert.Equal(t, "native-1", executor.ID())
	assert.Equal(t, "native-agent", executor.Name())
}

func TestNativeAgentAdapter_InAgentStep(t *testing.T) {
	mock := &mockNativeAgent{
		id:     "native-1",
		name:   "native-agent",
		output: &agent.Output{Content: "step result"},
	}

	adapter := NewNativeAgentAdapter(mock)
	step := NewAgentStep(adapter)

	assert.Equal(t, "agent:native-agent", step.Name())
	assert.Equal(t, "native-1", step.AgentID())

	result, err := step.Execute(context.Background(), "workflow input")
	require.NoError(t, err)

	out, ok := result.(*agent.Output)
	require.True(t, ok)
	assert.Equal(t, "step result", out.Content)
}
