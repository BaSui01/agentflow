package orchestration

import (
	"context"
	"testing"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/agent/crews"
	"github.com/BaSui01/agentflow/agent/handoff"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func handoffTask() handoff.Task {
	return handoff.Task{Type: "test", Description: "test"}
}

func crewsProposal() crews.Proposal {
	return crews.Proposal{Message: "test"}
}

// ---------------------------------------------------------------------------
// CollaborationAdapter
// ---------------------------------------------------------------------------

func TestNewCollaborationAdapter(t *testing.T) {
	tests := []struct {
		name               string
		coordinationType   string
		logger             *zap.Logger
		expectedCoordType  string
	}{
		{"nil logger defaults", "consensus", nil, "consensus"},
		{"empty type defaults to debate", "", zap.NewNop(), "debate"},
		{"explicit type", "pipeline", zap.NewNop(), "pipeline"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := NewCollaborationAdapter(tt.coordinationType, tt.logger)
			require.NotNil(t, a)
			assert.Equal(t, tt.expectedCoordType, a.coordinationType)
		})
	}
}

func TestCollaborationAdapter_Name(t *testing.T) {
	a := NewCollaborationAdapter("debate", zap.NewNop())
	assert.Equal(t, PatternCollaboration, a.Name())
}

func TestCollaborationAdapter_CanHandle(t *testing.T) {
	a := NewCollaborationAdapter("debate", zap.NewNop())

	assert.False(t, a.CanHandle(&OrchestrationTask{
		Agents: []agent.Agent{newMockAgent("a1", "w1", agent.TypeGeneric)},
	}))
	assert.True(t, a.CanHandle(&OrchestrationTask{
		Agents: []agent.Agent{
			newMockAgent("a1", "w1", agent.TypeGeneric),
			newMockAgent("a2", "w2", agent.TypeGeneric),
		},
	}))
}

func TestCollaborationAdapter_Priority(t *testing.T) {
	a := NewCollaborationAdapter("debate", zap.NewNop())

	assert.Equal(t, 80, a.Priority(&OrchestrationTask{
		Agents: []agent.Agent{
			newMockAgent("a1", "w1", agent.TypeGeneric),
			newMockAgent("a2", "w2", agent.TypeGeneric),
		},
	}))
	assert.Equal(t, 50, a.Priority(&OrchestrationTask{
		Agents: []agent.Agent{
			newMockAgent("a1", "w1", agent.TypeGeneric),
			newMockAgent("a2", "w2", agent.TypeGeneric),
			newMockAgent("a3", "w3", agent.TypeGeneric),
		},
	}))
}

func TestCollaborationAdapter_Execute(t *testing.T) {
	types := []string{"debate", "consensus", "pipeline", "broadcast"}
	for _, ct := range types {
		t.Run(ct, func(t *testing.T) {
			a := NewCollaborationAdapter(ct, zap.NewNop())
			task := &OrchestrationTask{
				ID:          "test",
				Description: "test task",
				Input:       &agent.Input{Content: "hello"},
				Agents: []agent.Agent{
					newMockAgent("a1", "w1", agent.TypeGeneric),
					newMockAgent("a2", "w2", agent.TypeGeneric),
				},
			}
			result, err := a.Execute(context.Background(), task)
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Len(t, result.AgentUsed, 2)
			assert.Equal(t, ct, result.Metadata["coordination_type"])
		})
	}
}

// ---------------------------------------------------------------------------
// HierarchicalAdapter
// ---------------------------------------------------------------------------

func TestNewHierarchicalAdapter(t *testing.T) {
	a := NewHierarchicalAdapter(nil)
	require.NotNil(t, a)
	a2 := NewHierarchicalAdapter(zap.NewNop())
	require.NotNil(t, a2)
}

func TestHierarchicalAdapter_Name(t *testing.T) {
	a := NewHierarchicalAdapter(zap.NewNop())
	assert.Equal(t, PatternHierarchical, a.Name())
}

func TestHierarchicalAdapter_CanHandle(t *testing.T) {
	a := NewHierarchicalAdapter(zap.NewNop())

	// 1 agent, no supervisor -> false
	assert.False(t, a.CanHandle(&OrchestrationTask{
		Agents: []agent.Agent{newMockAgent("a1", "w1", agent.TypeGeneric)},
	}))

	// 2 agents, no supervisor -> false
	assert.False(t, a.CanHandle(&OrchestrationTask{
		Agents: []agent.Agent{
			newMockAgent("a1", "w1", agent.TypeGeneric),
			newMockAgent("a2", "w2", agent.TypeGeneric),
		},
	}))

	// 2 agents with supervisor -> true
	assert.True(t, a.CanHandle(&OrchestrationTask{
		Agents: []agent.Agent{
			newMockAgent("s1", "supervisor-main", agent.TypeGeneric),
			newMockAgent("w1", "worker", agent.TypeGeneric),
		},
	}))
}

func TestHierarchicalAdapter_Priority(t *testing.T) {
	a := NewHierarchicalAdapter(zap.NewNop())

	assert.Equal(t, 90, a.Priority(&OrchestrationTask{
		Agents: []agent.Agent{
			newMockAgent("s1", "supervisor-main", agent.TypeGeneric),
			newMockAgent("w1", "worker", agent.TypeGeneric),
		},
	}))
	assert.Equal(t, 30, a.Priority(&OrchestrationTask{
		Agents: []agent.Agent{
			newMockAgent("a1", "w1", agent.TypeGeneric),
			newMockAgent("a2", "w2", agent.TypeGeneric),
		},
	}))
}

func TestHierarchicalAdapter_Execute_TooFewAgents(t *testing.T) {
	a := NewHierarchicalAdapter(zap.NewNop())
	_, err := a.Execute(context.Background(), &OrchestrationTask{
		Agents: []agent.Agent{newMockAgent("a1", "w1", agent.TypeGeneric)},
		Input:  &agent.Input{Content: "test"},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least 2 agents")
}

func TestHierarchicalAdapter_Execute_WithSupervisor(t *testing.T) {
	a := NewHierarchicalAdapter(zap.NewNop())
	task := &OrchestrationTask{
		ID:          "test",
		Description: "test task",
		Input:       &agent.Input{Content: "hello"},
		Agents: []agent.Agent{
			newMockAgent("s1", "supervisor-main", agent.TypeGeneric),
			newMockAgent("w1", "worker1", agent.TypeGeneric),
			newMockAgent("w2", "worker2", agent.TypeGeneric),
		},
	}
	result, err := a.Execute(context.Background(), task)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(t, result.AgentUsed, 3)
	assert.Equal(t, "s1", result.Metadata["supervisor"])
}

func TestHierarchicalAdapter_Execute_NoSupervisor_UsesFirst(t *testing.T) {
	a := NewHierarchicalAdapter(zap.NewNop())
	task := &OrchestrationTask{
		ID:          "test",
		Description: "test task",
		Input:       &agent.Input{Content: "hello"},
		Agents: []agent.Agent{
			newMockAgent("a1", "worker1", agent.TypeGeneric),
			newMockAgent("a2", "worker2", agent.TypeGeneric),
		},
	}
	result, err := a.Execute(context.Background(), task)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "a1", result.Metadata["supervisor"])
}

// ---------------------------------------------------------------------------
// HandoffAdapter
// ---------------------------------------------------------------------------

func TestNewHandoffAdapter(t *testing.T) {
	a := NewHandoffAdapter(nil)
	require.NotNil(t, a)
}

func TestHandoffAdapter_Name(t *testing.T) {
	a := NewHandoffAdapter(zap.NewNop())
	assert.Equal(t, PatternHandoff, a.Name())
}

func TestHandoffAdapter_CanHandle(t *testing.T) {
	a := NewHandoffAdapter(zap.NewNop())
	assert.True(t, a.CanHandle(&OrchestrationTask{
		Agents: []agent.Agent{newMockAgent("a1", "w1", agent.TypeGeneric)},
	}))
	assert.False(t, a.CanHandle(&OrchestrationTask{
		Agents: []agent.Agent{},
	}))
}

func TestHandoffAdapter_Priority(t *testing.T) {
	a := NewHandoffAdapter(zap.NewNop())
	assert.Equal(t, 100, a.Priority(&OrchestrationTask{
		Agents: []agent.Agent{newMockAgent("a1", "w1", agent.TypeGeneric)},
	}))
	assert.Equal(t, 20, a.Priority(&OrchestrationTask{
		Agents: []agent.Agent{
			newMockAgent("a1", "w1", agent.TypeGeneric),
			newMockAgent("a2", "w2", agent.TypeGeneric),
		},
	}))
}

func TestHandoffAdapter_Execute_NoAgents(t *testing.T) {
	a := NewHandoffAdapter(zap.NewNop())
	_, err := a.Execute(context.Background(), &OrchestrationTask{
		Agents: []agent.Agent{},
		Input:  &agent.Input{Content: "test"},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least 1 agent")
}

func TestHandoffAdapter_Execute_Success(t *testing.T) {
	a := NewHandoffAdapter(zap.NewNop())
	task := &OrchestrationTask{
		ID:          "test",
		Description: "test task",
		Input:       &agent.Input{Content: "hello"},
		Agents: []agent.Agent{
			newMockAgent("a1", "worker1", agent.TypeGeneric),
		},
	}
	result, err := a.Execute(context.Background(), task)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, []string{"a1"}, result.AgentUsed)
}

func TestHandoffAdapter_Execute_NonStringResult(t *testing.T) {
	a := NewHandoffAdapter(zap.NewNop())
	ma := newMockAgent("a1", "worker1", agent.TypeGeneric)
	ma.executeFn = func(_ context.Context, _ *agent.Input) (*agent.Output, error) {
		return &agent.Output{Content: "result-content"}, nil
	}
	task := &OrchestrationTask{
		ID:          "test",
		Description: "test task",
		Input:       &agent.Input{Content: "hello"},
		Agents:      []agent.Agent{ma},
	}
	result, err := a.Execute(context.Background(), task)
	require.NoError(t, err)
	require.NotNil(t, result)
}

// ---------------------------------------------------------------------------
// handoffAgentAdapter
// ---------------------------------------------------------------------------

func TestHandoffAgentAdapter_ID(t *testing.T) {
	ma := newMockAgent("test-id", "test-name", agent.TypeGeneric)
	adapter := &handoffAgentAdapter{agent: ma}
	assert.Equal(t, "test-id", adapter.ID())
}

func TestHandoffAgentAdapter_Capabilities(t *testing.T) {
	ma := newMockAgent("test-id", "test-name", agent.TypeGeneric)
	adapter := &handoffAgentAdapter{agent: ma}
	caps := adapter.Capabilities()
	require.Len(t, caps, 1)
	assert.Equal(t, "test-name", caps[0].Name)
	assert.Contains(t, caps[0].TaskTypes, "orchestration")
}

func TestHandoffAgentAdapter_CanHandle(t *testing.T) {
	ma := newMockAgent("test-id", "test-name", agent.TypeGeneric)
	adapter := &handoffAgentAdapter{agent: ma}
	assert.True(t, adapter.CanHandle(handoffTask()))
}

func TestHandoffAgentAdapter_AcceptHandoff(t *testing.T) {
	ma := newMockAgent("test-id", "test-name", agent.TypeGeneric)
	adapter := &handoffAgentAdapter{agent: ma}
	assert.NoError(t, adapter.AcceptHandoff(context.Background(), nil))
}

// ---------------------------------------------------------------------------
// CrewAdapter
// ---------------------------------------------------------------------------

func TestNewCrewAdapter(t *testing.T) {
	a := NewCrewAdapter(nil)
	require.NotNil(t, a)
}

func TestCrewAdapter_Name(t *testing.T) {
	a := NewCrewAdapter(zap.NewNop())
	assert.Equal(t, PatternCrew, a.Name())
}

func TestCrewAdapter_CanHandle(t *testing.T) {
	a := NewCrewAdapter(zap.NewNop())
	assert.False(t, a.CanHandle(&OrchestrationTask{
		Agents: []agent.Agent{newMockAgent("a1", "w1", agent.TypeGeneric)},
	}))
	assert.True(t, a.CanHandle(&OrchestrationTask{
		Agents: []agent.Agent{
			newMockAgent("a1", "w1", agent.TypeGeneric),
			newMockAgent("a2", "w2", agent.TypeGeneric),
		},
	}))
}

func TestCrewAdapter_Priority(t *testing.T) {
	a := NewCrewAdapter(zap.NewNop())
	assert.Equal(t, 85, a.Priority(&OrchestrationTask{
		Metadata: map[string]any{"roles": true},
		Agents: []agent.Agent{
			newMockAgent("a1", "w1", agent.TypeGeneric),
			newMockAgent("a2", "w2", agent.TypeGeneric),
		},
	}))
	assert.Equal(t, 40, a.Priority(&OrchestrationTask{
		Agents: []agent.Agent{
			newMockAgent("a1", "w1", agent.TypeGeneric),
			newMockAgent("a2", "w2", agent.TypeGeneric),
		},
	}))
}

func TestCrewAdapter_Execute_NoAgents(t *testing.T) {
	a := NewCrewAdapter(zap.NewNop())
	_, err := a.Execute(context.Background(), &OrchestrationTask{
		Agents: []agent.Agent{},
		Input:  &agent.Input{Content: "test"},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least 1 agent")
}

func TestCrewAdapter_Execute_Success(t *testing.T) {
	a := NewCrewAdapter(zap.NewNop())
	task := &OrchestrationTask{
		ID:          "test",
		Description: "test task",
		Input:       &agent.Input{Content: "hello", TraceID: "trace-1"},
		Agents: []agent.Agent{
			newMockAgent("a1", "analyst", agent.TypeGeneric),
			newMockAgent("a2", "writer", agent.TypeGeneric),
		},
	}
	result, err := a.Execute(context.Background(), task)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(t, result.AgentUsed, 2)
	assert.Equal(t, "trace-1", result.Output.TraceID)
}

// ---------------------------------------------------------------------------
// crewAgentAdapter
// ---------------------------------------------------------------------------

func TestCrewAgentAdapter_ID(t *testing.T) {
	ma := newMockAgent("test-id", "test-name", agent.TypeGeneric)
	adapter := &crewAgentAdapter{agent: ma}
	assert.Equal(t, "test-id", adapter.ID())
}

func TestCrewAgentAdapter_Negotiate(t *testing.T) {
	ma := newMockAgent("test-id", "test-name", agent.TypeGeneric)
	adapter := &crewAgentAdapter{agent: ma}
	result, err := adapter.Negotiate(context.Background(), crewsProposal())
	require.NoError(t, err)
	assert.True(t, result.Accepted)
	assert.Equal(t, "test-id", result.Response)
}

