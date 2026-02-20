package agent

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// toolTestAgent is a hand-written mock implementing the Agent interface for AgentTool tests.
// Named distinctly to avoid collision with mockAgent in other test files.
type toolTestAgent struct {
	id        string
	name      string
	agentType AgentType
	state     State

	executeFn func(ctx context.Context, input *Input) (*Output, error)
	mu        sync.Mutex
	calls     []*Input
}

func newToolTestAgent(id, name string) *toolTestAgent {
	return &toolTestAgent{
		id:        id,
		name:      name,
		agentType: TypeGeneric,
		state:     StateReady,
	}
}

func (m *toolTestAgent) ID() string      { return m.id }
func (m *toolTestAgent) Name() string    { return m.name }
func (m *toolTestAgent) Type() AgentType { return m.agentType }
func (m *toolTestAgent) State() State    { return m.state }

func (m *toolTestAgent) Init(ctx context.Context) error     { return nil }
func (m *toolTestAgent) Teardown(ctx context.Context) error { return nil }
func (m *toolTestAgent) Plan(ctx context.Context, input *Input) (*PlanResult, error) {
	return &PlanResult{}, nil
}
func (m *toolTestAgent) Observe(ctx context.Context, feedback *Feedback) error { return nil }

func (m *toolTestAgent) Execute(ctx context.Context, input *Input) (*Output, error) {
	m.mu.Lock()
	m.calls = append(m.calls, input)
	m.mu.Unlock()

	if m.executeFn != nil {
		return m.executeFn(ctx, input)
	}
	return &Output{
		Content:      "mock response to: " + input.Content,
		TokensUsed:   42,
		Duration:     100 * time.Millisecond,
		FinishReason: "stop",
	}, nil
}

func (m *toolTestAgent) getCalls() []*Input {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]*Input, len(m.calls))
	copy(cp, m.calls)
	return cp
}

// --- Tests ---

func TestAgentTool_Schema(t *testing.T) {
	agent := newToolTestAgent("agent-1", "summarizer")
	at := NewAgentTool(agent, nil)

	schema := at.Schema()

	assert.Equal(t, "agent_summarizer", schema.Name)
	assert.Contains(t, schema.Description, "summarizer")
	assert.NotEmpty(t, schema.Parameters)

	// Verify parameters is valid JSON with expected structure
	var params map[string]any
	err := json.Unmarshal(schema.Parameters, &params)
	require.NoError(t, err)
	assert.Equal(t, "object", params["type"])

	props, ok := params["properties"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, props, "input")
	assert.Contains(t, props, "context")
	assert.Contains(t, props, "variables")
}

func TestAgentTool_Schema_CustomNameAndDescription(t *testing.T) {
	agent := newToolTestAgent("agent-1", "summarizer")
	at := NewAgentTool(agent, &AgentToolConfig{
		Name:        "my_custom_tool",
		Description: "A custom description",
	})

	schema := at.Schema()

	assert.Equal(t, "my_custom_tool", schema.Name)
	assert.Equal(t, "A custom description", schema.Description)
}

func TestAgentTool_Execute_Success(t *testing.T) {
	agent := newToolTestAgent("agent-1", "summarizer")
	at := NewAgentTool(agent, nil)

	args, _ := json.Marshal(agentToolArgs{
		Input:   "Summarize this document",
		Context: map[string]any{"source": "test"},
	})

	call := types.ToolCall{
		ID:        "call-1",
		Name:      "agent_summarizer",
		Arguments: args,
	}

	result := at.Execute(context.Background(), call)

	assert.Equal(t, "call-1", result.ToolCallID)
	assert.Equal(t, "agent_summarizer", result.Name)
	assert.Empty(t, result.Error)
	assert.NotEmpty(t, result.Result)
	assert.True(t, result.Duration >= 0)

	// Verify the result content
	var resultData map[string]any
	err := json.Unmarshal(result.Result, &resultData)
	require.NoError(t, err)
	assert.Contains(t, resultData["content"], "mock response to: Summarize this document")

	// Verify the agent received the correct input
	calls := agent.getCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, "Summarize this document", calls[0].Content)
	assert.Equal(t, "test", calls[0].Context["source"])
}

func TestAgentTool_Execute_InvalidArguments(t *testing.T) {
	agent := newToolTestAgent("agent-1", "summarizer")
	at := NewAgentTool(agent, nil)

	call := types.ToolCall{
		ID:        "call-1",
		Name:      "agent_summarizer",
		Arguments: json.RawMessage(`{invalid json`),
	}

	result := at.Execute(context.Background(), call)

	assert.Equal(t, "call-1", result.ToolCallID)
	assert.Contains(t, result.Error, "invalid arguments")
	assert.Empty(t, result.Result)
}

func TestAgentTool_Execute_MissingInput(t *testing.T) {
	agent := newToolTestAgent("agent-1", "summarizer")
	at := NewAgentTool(agent, nil)

	args, _ := json.Marshal(agentToolArgs{})

	call := types.ToolCall{
		ID:        "call-1",
		Name:      "agent_summarizer",
		Arguments: args,
	}

	result := at.Execute(context.Background(), call)

	assert.Contains(t, result.Error, "missing required field: input")
}

func TestAgentTool_Execute_AgentError(t *testing.T) {
	agent := newToolTestAgent("agent-1", "summarizer")
	agent.executeFn = func(ctx context.Context, input *Input) (*Output, error) {
		return nil, errors.New("agent internal error")
	}
	at := NewAgentTool(agent, nil)

	args, _ := json.Marshal(agentToolArgs{Input: "test"})
	call := types.ToolCall{
		ID:        "call-1",
		Name:      "agent_summarizer",
		Arguments: args,
	}

	result := at.Execute(context.Background(), call)

	assert.Equal(t, "agent internal error", result.Error)
	assert.Empty(t, result.Result)
}

func TestAgentTool_Execute_Timeout(t *testing.T) {
	agent := newToolTestAgent("agent-1", "slow-agent")
	agent.executeFn = func(ctx context.Context, input *Input) (*Output, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(5 * time.Second):
			return &Output{Content: "should not reach"}, nil
		}
	}

	at := NewAgentTool(agent, &AgentToolConfig{
		Timeout: 50 * time.Millisecond,
	})

	args, _ := json.Marshal(agentToolArgs{Input: "test"})
	call := types.ToolCall{
		ID:        "call-1",
		Name:      "agent_slow-agent",
		Arguments: args,
	}

	result := at.Execute(context.Background(), call)

	assert.NotEmpty(t, result.Error)
	assert.Contains(t, result.Error, "context deadline exceeded")
}

func TestAgentTool_Execute_ConcurrentSafety(t *testing.T) {
	agent := newToolTestAgent("agent-1", "concurrent")
	at := NewAgentTool(agent, nil)

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			args, _ := json.Marshal(agentToolArgs{
				Input: "concurrent request",
			})
			call := types.ToolCall{
				ID:        "call-concurrent",
				Name:      "agent_concurrent",
				Arguments: args,
			}
			result := at.Execute(context.Background(), call)
			assert.Empty(t, result.Error)
			assert.NotEmpty(t, result.Result)
		}(i)
	}

	wg.Wait()

	calls := agent.getCalls()
	assert.Len(t, calls, goroutines)
}

func TestAgentTool_Name(t *testing.T) {
	agent := newToolTestAgent("agent-1", "helper")
	at := NewAgentTool(agent, nil)
	assert.Equal(t, "agent_helper", at.Name())

	at2 := NewAgentTool(agent, &AgentToolConfig{Name: "custom"})
	assert.Equal(t, "custom", at2.Name())
}

func TestAgentTool_Agent(t *testing.T) {
	agent := newToolTestAgent("agent-1", "helper")
	at := NewAgentTool(agent, nil)
	assert.Equal(t, agent, at.Agent())
}
