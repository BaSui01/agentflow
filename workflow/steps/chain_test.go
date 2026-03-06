package steps

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/BaSui01/agentflow/llm/capabilities/tools"
	"github.com/BaSui01/agentflow/workflow/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockToolRegistry struct {
	results map[string]json.RawMessage
	errors  map[string]error
	calls   []string
}

func (m *mockToolRegistry) Execute(ctx context.Context, name string, args json.RawMessage) (json.RawMessage, error) {
	m.calls = append(m.calls, name)
	if err, ok := m.errors[name]; ok {
		return nil, err
	}
	if r, ok := m.results[name]; ok {
		return r, nil
	}
	return json.RawMessage(`{}`), nil
}

func TestChainStep_Validate(t *testing.T) {
	t.Run("nil executor", func(t *testing.T) {
		s := NewChainStep("c1", tools.ToolChain{Name: "x", Steps: []tools.ChainStep{{ToolName: "a"}}}, nil)
		err := s.Validate()
		require.Error(t, err)
		assert.ErrorIs(t, err, core.ErrStepNotConfigured)
	})

	t.Run("empty steps", func(t *testing.T) {
		reg := &mockToolRegistry{}
		exec := tools.NewChainExecutor(reg, tools.DefaultParallelConfig())
		s := NewChainStep("c2", tools.ToolChain{Name: "empty", Steps: nil}, exec)
		err := s.Validate()
		require.Error(t, err)
		assert.ErrorIs(t, err, core.ErrStepValidation)
	})

	t.Run("ok", func(t *testing.T) {
		reg := &mockToolRegistry{}
		exec := tools.NewChainExecutor(reg, tools.DefaultParallelConfig())
		s := NewChainStep("c3", tools.ToolChain{Name: "ok", Steps: []tools.ChainStep{{ToolName: "t1"}}}, exec)
		err := s.Validate()
		require.NoError(t, err)
	})
}

func TestChainStep_Execute(t *testing.T) {
	reg := &mockToolRegistry{
		results: map[string]json.RawMessage{
			"step1": json.RawMessage(`{"value": "v1"}`),
			"step2": json.RawMessage(`{"value": "v2", "done": true}`),
		},
		calls: nil,
	}
	exec := tools.NewChainExecutor(reg, tools.DefaultParallelConfig())
	chain := tools.ToolChain{
		Name: "two-step",
		Steps: []tools.ChainStep{
			{ToolName: "step1", Args: map[string]any{"x": 1}},
			{ToolName: "step2", Args: map[string]any{"y": 2}, ArgMapping: map[string]string{"prev": "value"}},
		},
	}
	s := NewChainStep("chain-id", chain, exec)

	ctx := context.Background()
	input := core.StepInput{Data: map[string]any{"initial": "data"}}
	out, err := s.Execute(ctx, input)
	require.NoError(t, err)
	require.NotNil(t, out.Data)
	assert.Equal(t, []string{"step1", "step2"}, reg.calls)
	assert.NotNil(t, out.Data["result"])
	assert.NotNil(t, out.Data["steps"])
	steps, ok := out.Data["steps"].([]tools.ChainStepResult)
	require.True(t, ok)
	require.Len(t, steps, 2)
	assert.Equal(t, "step1", steps[0].ToolName)
	assert.Equal(t, "step2", steps[1].ToolName)
}

func TestChainStep_Execute_NilExecutor(t *testing.T) {
	s := NewChainStep("c1", tools.ToolChain{Steps: []tools.ChainStep{{ToolName: "x"}}}, nil)
	out, err := s.Execute(context.Background(), core.StepInput{})
	require.Error(t, err)
	assert.ErrorIs(t, err, core.ErrStepNotConfigured)
	assert.Empty(t, out.Data)
}

func TestChainStep_Execute_RegistryError(t *testing.T) {
	failErr := errors.New("tool failed")
	reg := &mockToolRegistry{
		results: map[string]json.RawMessage{"step1": json.RawMessage(`{}`)},
		errors:  map[string]error{"step2": failErr},
	}
	exec := tools.NewChainExecutor(reg, tools.DefaultParallelConfig())
	s := NewChainStep("c1", tools.ToolChain{
		Name:  "fail",
		Steps: []tools.ChainStep{{ToolName: "step1"}, {ToolName: "step2"}},
	}, exec)

	out, err := s.Execute(context.Background(), core.StepInput{})
	require.Error(t, err)
	assert.ErrorIs(t, err, core.ErrStepExecution)
	assert.ErrorIs(t, err, failErr)
	assert.Empty(t, out.Data)
}

func TestChainStep_IDAndType(t *testing.T) {
	reg := &mockToolRegistry{}
	exec := tools.NewChainExecutor(reg, tools.DefaultParallelConfig())
	s := NewChainStep("my-chain-id", tools.ToolChain{Steps: []tools.ChainStep{{ToolName: "x"}}}, exec)
	assert.Equal(t, "my-chain-id", s.ID())
	assert.Equal(t, core.StepTypeChain, s.Type())
}
