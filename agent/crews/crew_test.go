package crews

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// mockCrewAgent implements CrewAgent with function callbacks.
type mockCrewAgent struct {
	id          string
	executeFn   func(ctx context.Context, task CrewTask) (*TaskResult, error)
	negotiateFn func(ctx context.Context, proposal Proposal) (*NegotiationResult, error)
}

func (m *mockCrewAgent) ID() string { return m.id }

func (m *mockCrewAgent) Execute(ctx context.Context, task CrewTask) (*TaskResult, error) {
	if m.executeFn != nil {
		return m.executeFn(ctx, task)
	}
	return &TaskResult{TaskID: task.ID, Output: "default output"}, nil
}

func (m *mockCrewAgent) Negotiate(ctx context.Context, proposal Proposal) (*NegotiationResult, error) {
	if m.negotiateFn != nil {
		return m.negotiateFn(ctx, proposal)
	}
	return &NegotiationResult{Accepted: true, Response: m.id}, nil
}

func newTestCrew(t *testing.T, process ProcessType, agents ...CrewAgent) *Crew {
	t.Helper()
	crew := NewCrew(CrewConfig{
		Name:    "test-crew",
		Process: process,
	}, zap.NewNop())
	for _, a := range agents {
		crew.AddMember(a, Role{
			Name:            a.ID() + "-role",
			Description:     "test role",
			Skills:          []string{"testing"},
			AllowDelegation: a.ID() == "manager",
		})
	}
	return crew
}

// --- D1 Tests ---

func TestCrew_Execute_Sequential(t *testing.T) {
	executedTasks := make([]string, 0)

	agent1 := &mockCrewAgent{
		id: "agent-1",
		executeFn: func(_ context.Context, task CrewTask) (*TaskResult, error) {
			executedTasks = append(executedTasks, task.ID)
			return &TaskResult{TaskID: task.ID, Output: "result-" + task.ID}, nil
		},
	}

	crew := newTestCrew(t, ProcessSequential, agent1)
	crew.AddTask(CrewTask{ID: "task-1", Description: "first task"})
	crew.AddTask(CrewTask{ID: "task-2", Description: "second task"})

	result, err := crew.Execute(context.Background())
	require.NoError(t, err)
	assert.Len(t, result.TaskResults, 2)
	assert.Equal(t, "result-task-1", result.TaskResults["task-1"].Output)
	assert.Equal(t, "result-task-2", result.TaskResults["task-2"].Output)
	// Tasks should execute in order
	assert.Equal(t, []string{"task-1", "task-2"}, executedTasks)
	assert.False(t, result.EndTime.IsZero())
}

func TestCrew_Execute_Sequential_Error(t *testing.T) {
	agent1 := &mockCrewAgent{
		id: "agent-1",
		executeFn: func(_ context.Context, task CrewTask) (*TaskResult, error) {
			return nil, errors.New("execution failed")
		},
	}

	crew := newTestCrew(t, ProcessSequential, agent1)
	crew.AddTask(CrewTask{ID: "task-1", Description: "failing task"})

	result, err := crew.Execute(context.Background())
	require.NoError(t, err) // crew itself doesn't return error, stores it in TaskResult
	assert.NotNil(t, result.TaskResults["task-1"])
	assert.Equal(t, "execution failed", result.TaskResults["task-1"].Error)
}

func TestCrew_Execute_Sequential_AssignedTo(t *testing.T) {
	executedBy := ""
	agent1 := &mockCrewAgent{
		id: "agent-1",
		executeFn: func(_ context.Context, task CrewTask) (*TaskResult, error) {
			executedBy = "agent-1"
			return &TaskResult{TaskID: task.ID, Output: "done"}, nil
		},
	}
	agent2 := &mockCrewAgent{
		id: "agent-2",
		executeFn: func(_ context.Context, task CrewTask) (*TaskResult, error) {
			executedBy = "agent-2"
			return &TaskResult{TaskID: task.ID, Output: "done"}, nil
		},
	}

	crew := newTestCrew(t, ProcessSequential, agent1, agent2)
	crew.AddTask(CrewTask{ID: "task-1", Description: "assigned task", AssignedTo: "agent-2"})

	_, err := crew.Execute(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "agent-2", executedBy)
}

func TestCrew_Execute_Hierarchical(t *testing.T) {
	managerExecuted := false
	workerExecuted := false

	manager := &mockCrewAgent{
		id: "manager",
		executeFn: func(_ context.Context, task CrewTask) (*TaskResult, error) {
			managerExecuted = true
			return &TaskResult{TaskID: task.ID, Output: "manager-result"}, nil
		},
		negotiateFn: func(_ context.Context, _ Proposal) (*NegotiationResult, error) {
			return &NegotiationResult{Accepted: true}, nil
		},
	}
	worker := &mockCrewAgent{
		id: "worker-1",
		executeFn: func(_ context.Context, task CrewTask) (*TaskResult, error) {
			workerExecuted = true
			return &TaskResult{TaskID: task.ID, Output: "worker-result"}, nil
		},
		negotiateFn: func(_ context.Context, _ Proposal) (*NegotiationResult, error) {
			return &NegotiationResult{Accepted: true}, nil
		},
	}

	crew := newTestCrew(t, ProcessHierarchical, manager, worker)
	crew.AddTask(CrewTask{ID: "task-1", Description: "delegated task"})

	result, err := crew.Execute(context.Background())
	require.NoError(t, err)
	assert.Len(t, result.TaskResults, 1)
	// Worker should execute since it's idle and manager delegates
	assert.True(t, workerExecuted || managerExecuted, "at least one agent should execute")
}

func TestCrew_Execute_Hierarchical_Rejected(t *testing.T) {
	// When worker rejects, manager should execute
	managerExecuted := false

	manager := &mockCrewAgent{
		id: "manager",
		executeFn: func(_ context.Context, task CrewTask) (*TaskResult, error) {
			managerExecuted = true
			return &TaskResult{TaskID: task.ID, Output: "manager-did-it"}, nil
		},
	}
	worker := &mockCrewAgent{
		id: "worker-1",
		negotiateFn: func(_ context.Context, _ Proposal) (*NegotiationResult, error) {
			return &NegotiationResult{Accepted: false, Response: "too busy"}, nil
		},
		executeFn: func(_ context.Context, task CrewTask) (*TaskResult, error) {
			return &TaskResult{TaskID: task.ID, Output: "worker-did-it"}, nil
		},
	}

	crew := newTestCrew(t, ProcessHierarchical, manager, worker)
	crew.AddTask(CrewTask{ID: "task-1", Description: "rejected task"})

	result, err := crew.Execute(context.Background())
	require.NoError(t, err)
	assert.True(t, managerExecuted, "manager should execute when worker rejects")
	assert.Equal(t, "manager-did-it", result.TaskResults["task-1"].Output)
}

func TestCrew_Execute_Hierarchical_NegotiationError(t *testing.T) {
	// When negotiation errors, manager should fallback to executing itself
	managerExecuted := false

	manager := &mockCrewAgent{
		id: "manager",
		executeFn: func(_ context.Context, task CrewTask) (*TaskResult, error) {
			managerExecuted = true
			return &TaskResult{TaskID: task.ID, Output: "manager-fallback"}, nil
		},
	}
	worker := &mockCrewAgent{
		id: "worker-1",
		negotiateFn: func(_ context.Context, _ Proposal) (*NegotiationResult, error) {
			return nil, errors.New("negotiation error")
		},
	}

	crew := newTestCrew(t, ProcessHierarchical, manager, worker)
	crew.AddTask(CrewTask{ID: "task-1", Description: "error task"})

	result, err := crew.Execute(context.Background())
	require.NoError(t, err)
	assert.True(t, managerExecuted)
	assert.Equal(t, "manager-fallback", result.TaskResults["task-1"].Output)
}

func TestCrew_Execute_Consensus(t *testing.T) {
	agent1 := &mockCrewAgent{
		id: "agent-1",
		negotiateFn: func(_ context.Context, _ Proposal) (*NegotiationResult, error) {
			return &NegotiationResult{Accepted: true, Response: "agent-2"}, nil
		},
		executeFn: func(_ context.Context, task CrewTask) (*TaskResult, error) {
			return &TaskResult{TaskID: task.ID, Output: "agent-1-result"}, nil
		},
	}
	agent2 := &mockCrewAgent{
		id: "agent-2",
		negotiateFn: func(_ context.Context, _ Proposal) (*NegotiationResult, error) {
			return &NegotiationResult{Accepted: true, Response: "agent-2"}, nil
		},
		executeFn: func(_ context.Context, task CrewTask) (*TaskResult, error) {
			return &TaskResult{TaskID: task.ID, Output: "agent-2-result"}, nil
		},
	}

	crew := newTestCrew(t, ProcessConsensus, agent1, agent2)
	crew.AddTask(CrewTask{ID: "task-1", Description: "consensus task"})

	result, err := crew.Execute(context.Background())
	require.NoError(t, err)
	assert.Len(t, result.TaskResults, 1)
	// Both agents vote for agent-2, so agent-2 should execute
	assert.Equal(t, "agent-2-result", result.TaskResults["task-1"].Output)
}

func TestCrew_Execute_Consensus_NoVotes(t *testing.T) {
	// When no votes, fallback to findBestMember
	agent1 := &mockCrewAgent{
		id: "agent-1",
		negotiateFn: func(_ context.Context, _ Proposal) (*NegotiationResult, error) {
			return &NegotiationResult{Accepted: true, Response: ""}, nil // empty response = no vote
		},
		executeFn: func(_ context.Context, task CrewTask) (*TaskResult, error) {
			return &TaskResult{TaskID: task.ID, Output: "fallback-result"}, nil
		},
	}

	crew := newTestCrew(t, ProcessConsensus, agent1)
	crew.AddTask(CrewTask{ID: "task-1", Description: "no-vote task"})

	result, err := crew.Execute(context.Background())
	require.NoError(t, err)
	assert.Len(t, result.TaskResults, 1)
	assert.Equal(t, "fallback-result", result.TaskResults["task-1"].Output)
}

func TestCrew_Execute_Consensus_NegotiationError(t *testing.T) {
	// When negotiation errors, that member's vote is skipped
	agent1 := &mockCrewAgent{
		id: "agent-1",
		negotiateFn: func(_ context.Context, _ Proposal) (*NegotiationResult, error) {
			return nil, errors.New("negotiation failed")
		},
		executeFn: func(_ context.Context, task CrewTask) (*TaskResult, error) {
			return &TaskResult{TaskID: task.ID, Output: "agent-1-result"}, nil
		},
	}
	agent2 := &mockCrewAgent{
		id: "agent-2",
		negotiateFn: func(_ context.Context, _ Proposal) (*NegotiationResult, error) {
			return &NegotiationResult{Accepted: true, Response: "agent-2"}, nil
		},
		executeFn: func(_ context.Context, task CrewTask) (*TaskResult, error) {
			return &TaskResult{TaskID: task.ID, Output: "agent-2-result"}, nil
		},
	}

	crew := newTestCrew(t, ProcessConsensus, agent1, agent2)
	crew.AddTask(CrewTask{ID: "task-1", Description: "partial-vote task"})

	result, err := crew.Execute(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "agent-2-result", result.TaskResults["task-1"].Output)
}

func TestCrew_FindBestMember(t *testing.T) {
	agent1 := &mockCrewAgent{id: "agent-1"}
	agent2 := &mockCrewAgent{id: "agent-2"}

	crew := newTestCrew(t, ProcessSequential, agent1, agent2)

	tests := []struct {
		name     string
		task     *CrewTask
		setup    func()
		wantID   string
		wantNil  bool
	}{
		{
			name:   "assigned to specific agent",
			task:   &CrewTask{ID: "t1", AssignedTo: "agent-2"},
			wantID: "agent-2",
		},
		{
			name:   "assigned to nonexistent agent falls back to idle",
			task:   &CrewTask{ID: "t2", AssignedTo: "nonexistent"},
			wantID: "", // returns any idle member
		},
		{
			name: "returns idle member",
			task: &CrewTask{ID: "t3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.setup()
			}
			member := crew.findBestMember(tt.task)
			if tt.wantNil {
				assert.Nil(t, member)
			} else if tt.wantID != "" {
				require.NotNil(t, member)
				assert.Equal(t, tt.wantID, member.ID)
			} else {
				assert.NotNil(t, member)
			}
		})
	}
}

func TestCrew_Execute_EmptyTasks(t *testing.T) {
	agent1 := &mockCrewAgent{id: "agent-1"}
	crew := newTestCrew(t, ProcessSequential, agent1)
	// No tasks added

	result, err := crew.Execute(context.Background())
	require.NoError(t, err)
	assert.Empty(t, result.TaskResults)
	assert.False(t, result.EndTime.IsZero())
}

func TestCrew_Execute_NoMembers(t *testing.T) {
	crew := NewCrew(CrewConfig{
		Name:    "empty-crew",
		Process: ProcessSequential,
	}, zap.NewNop())
	crew.AddTask(CrewTask{ID: "task-1", Description: "orphan task"})

	_, err := crew.Execute(context.Background())
	assert.Error(t, err, "should error when no member can handle task")
}

func TestCrew_AddTask_AutoID(t *testing.T) {
	crew := NewCrew(CrewConfig{Name: "test"}, zap.NewNop())
	crew.AddTask(CrewTask{Description: "no id"})
	crew.AddTask(CrewTask{Description: "also no id"})

	assert.Equal(t, "task_1", crew.Tasks[0].ID)
	assert.Equal(t, "task_2", crew.Tasks[1].ID)
}
