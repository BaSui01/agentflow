package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type mockChainRegistry struct {
	results map[string]json.RawMessage
	errors  map[string]error
	calls   []string
}

func (m *mockChainRegistry) Execute(ctx context.Context, name string, args json.RawMessage) (json.RawMessage, error) {
	m.calls = append(m.calls, name)
	if err, ok := m.errors[name]; ok {
		return nil, err
	}
	if r, ok := m.results[name]; ok {
		return r, nil
	}
	return json.RawMessage(`{}`), nil
}

func TestChainExecutor_ExecuteChain_Sequential(t *testing.T) {
	reg := &mockChainRegistry{
		results: map[string]json.RawMessage{
			"step1": json.RawMessage(`{"value": "from-step1"}`),
			"step2": json.RawMessage(`{"value": "from-step2", "combined": true}`),
		},
		calls: nil,
	}
	exec := NewChainExecutor(reg, DefaultParallelConfig())
	chain := ToolChain{
		Name: "test-chain",
		Steps: []ChainStep{
			{ToolName: "step1", Args: map[string]any{"input": "a"}},
			{ToolName: "step2", Args: map[string]any{"x": 1}, ArgMapping: map[string]string{"prev": "value"}},
		},
	}

	ctx := context.Background()
	result, err := exec.ExecuteChain(ctx, chain, map[string]any{"initial": "data"})
	require.NoError(t, err)
	require.Len(t, result.Steps, 2)
	assert.Equal(t, "step1", result.Steps[0].ToolName)
	assert.Equal(t, "step2", result.Steps[1].ToolName)
	assert.Nil(t, result.Steps[0].Error)
	assert.Nil(t, result.Steps[1].Error)
	assert.Equal(t, []string{"step1", "step2"}, reg.calls)
	assert.NotNil(t, result.FinalOutput)
	assert.GreaterOrEqual(t, result.Duration, time.Duration(0))
}

func TestChainExecutor_ExecuteChain_ErrorFail(t *testing.T) {
	failErr := errors.New("tool failed")
	reg := &mockChainRegistry{
		results: map[string]json.RawMessage{"step1": json.RawMessage(`{}`)},
		errors:  map[string]error{"step2": failErr},
		calls:   nil,
	}
	exec := NewChainExecutor(reg, DefaultParallelConfig())
	chain := ToolChain{
		Name: "fail-chain",
		Steps: []ChainStep{
			{ToolName: "step1"},
			{ToolName: "step2", OnError: "fail"},
		},
	}

	ctx := context.Background()
	result, err := exec.ExecuteChain(ctx, chain, nil)
	assert.ErrorIs(t, err, failErr)
	require.Len(t, result.Steps, 2)
	assert.Nil(t, result.Steps[0].Error)
	assert.ErrorIs(t, result.Steps[1].Error, failErr)
}

func TestChainExecutor_ExecuteChain_ErrorSkip(t *testing.T) {
	reg := &mockChainRegistry{
		results: map[string]json.RawMessage{"step1": json.RawMessage(`{}`)},
		errors:  map[string]error{"step2": errors.New("skip me")},
		calls:   nil,
	}
	exec := NewChainExecutor(reg, DefaultParallelConfig())
	chain := ToolChain{
		Name: "skip-chain",
		Steps: []ChainStep{
			{ToolName: "step1"},
			{ToolName: "step2", OnError: "skip"},
		},
	}

	ctx := context.Background()
	result, err := exec.ExecuteChain(ctx, chain, nil)
	require.NoError(t, err)
	require.Len(t, result.Steps, 2)
	assert.True(t, result.Steps[1].Skipped)
}

func TestChainExecutor_ExecuteChain_EmptySteps(t *testing.T) {
	reg := &mockChainRegistry{calls: nil}
	exec := NewChainExecutor(reg, DefaultParallelConfig())
	chain := ToolChain{Name: "empty", Steps: nil}

	ctx := context.Background()
	result, err := exec.ExecuteChain(ctx, chain, map[string]any{"k": "v"})
	require.NoError(t, err)
	assert.Len(t, result.Steps, 0)
	assert.JSONEq(t, `{"k":"v"}`, string(result.FinalOutput))
}

func TestRegistryAsChainExecutor(t *testing.T) {
	r := NewDefaultRegistry(zap.NewNop())
	err := r.Register("echo", func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
		return args, nil
	}, ToolMetadata{})
	require.NoError(t, err)

	like := RegistryAsChainExecutor(r)
	out, err := like.Execute(context.Background(), "echo", json.RawMessage(`{"x":1}`))
	require.NoError(t, err)
	assert.JSONEq(t, `{"x":1}`, string(out))
}
