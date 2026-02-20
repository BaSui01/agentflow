package workflow

import (
	"context"
	"errors"
	"fmt"
	"testing"

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
