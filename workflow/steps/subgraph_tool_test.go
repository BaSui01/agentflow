package steps

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockSubgraphExecutor struct {
	result any
	err    error
}

func (m *mockSubgraphExecutor) ExecuteSubgraph(_ context.Context, _ any) (any, error) {
	return m.result, m.err
}

func TestSubgraphTool_Execute_Success(t *testing.T) {
	tool := NewSubgraphTool("test-tool", "runs a subgraph", &mockSubgraphExecutor{
		result: map[string]any{"answer": "42"},
	})

	args, _ := json.Marshal(map[string]string{"query": "what"})
	result, err := tool.Execute(context.Background(), args)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(result, &parsed))
	assert.Equal(t, "42", parsed["answer"])
}

func TestSubgraphTool_Execute_NoExecutor(t *testing.T) {
	tool := NewSubgraphTool("nil-exec", "broken", nil)
	_, err := tool.Execute(context.Background(), nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no executor configured")
}

func TestSubgraphTool_ToolSchema(t *testing.T) {
	tool := NewSubgraphTool("my-tool", "my description", nil)
	schema := tool.ToolSchema()
	assert.Equal(t, "my-tool", schema["name"])
	assert.Equal(t, "my description", schema["description"])
}
