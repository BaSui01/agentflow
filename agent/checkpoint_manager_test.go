package agent

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestCheckpointManager_CreateCheckpoint(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	store, err := NewFileCheckpointStore(t.TempDir(), logger)
	require.NoError(t, err)

	manager := NewCheckpointManager(store, logger)

	// Create a mock agent
	agent := &mockAgent{
		id:    "test-agent",
		state: StateReady,
	}

	threadID := "test-thread"

	// Create checkpoint
	err = manager.CreateCheckpoint(context.Background(), agent, threadID)
	require.NoError(t, err)

	// Verify checkpoint was created
	checkpoints, err := store.List(context.Background(), threadID, 10)
	require.NoError(t, err)
	assert.Len(t, checkpoints, 1)
	assert.Equal(t, agent.id, checkpoints[0].AgentID)
	assert.Equal(t, StateReady, checkpoints[0].State)
}

func TestCheckpointManager_RollbackToVersion(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	store, err := NewFileCheckpointStore(t.TempDir(), logger)
	require.NoError(t, err)

	manager := NewCheckpointManager(store, logger)

	// Create a mock agent
	agent := &mockAgent{
		id:    "test-agent",
		state: StateInit,
	}

	threadID := "test-thread"
	ctx := context.Background()

	// Create first checkpoint (version 1)
	err = manager.CreateCheckpoint(ctx, agent, threadID)
	require.NoError(t, err)

	// Change state and create second checkpoint (version 2)
	agent.state = StateRunning
	err = manager.CreateCheckpoint(ctx, agent, threadID)
	require.NoError(t, err)

	// Change state and create third checkpoint (version 3)
	agent.state = StateReady
	err = manager.CreateCheckpoint(ctx, agent, threadID)
	require.NoError(t, err)

	// Verify we have 3 versions
	versions, err := manager.ListVersions(ctx, threadID)
	require.NoError(t, err)
	assert.Len(t, versions, 3)

	// Rollback to version 1
	err = manager.RollbackToVersion(ctx, agent, threadID, 1)
	require.NoError(t, err)

	// Verify agent state was restored
	assert.Equal(t, StateInit, agent.state)

	// Verify a new checkpoint was created (version 4)
	versions, err = manager.ListVersions(ctx, threadID)
	require.NoError(t, err)
	assert.Len(t, versions, 4)
	assert.Equal(t, 4, versions[3].Version)
}

func TestCheckpointManager_CompareVersions(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	store, err := NewFileCheckpointStore(t.TempDir(), logger)
	require.NoError(t, err)

	manager := NewCheckpointManager(store, logger)

	agent := &mockAgent{
		id:    "test-agent",
		state: StateInit,
	}

	threadID := "test-thread"
	ctx := context.Background()

	// Create first checkpoint
	err = manager.CreateCheckpoint(ctx, agent, threadID)
	require.NoError(t, err)

	time.Sleep(10 * time.Millisecond)

	// Change state and create second checkpoint
	agent.state = StateReady
	err = manager.CreateCheckpoint(ctx, agent, threadID)
	require.NoError(t, err)

	// Compare versions
	diff, err := manager.CompareVersions(ctx, threadID, 1, 2)
	require.NoError(t, err)

	assert.Equal(t, threadID, diff.ThreadID)
	assert.Equal(t, 1, diff.Version1)
	assert.Equal(t, 2, diff.Version2)
	assert.True(t, diff.StateChanged)
	assert.Equal(t, StateInit, diff.OldState)
	assert.Equal(t, StateReady, diff.NewState)
	assert.Greater(t, diff.TimeDiff, time.Duration(0))
}

func TestCheckpointManager_AutoSave(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	store, err := NewFileCheckpointStore(t.TempDir(), logger)
	require.NoError(t, err)

	manager := NewCheckpointManager(store, logger)

	agent := &mockAgent{
		id:    "test-agent",
		state: StateReady,
	}

	threadID := "test-thread"
	ctx := context.Background()

	// Enable auto-save with short interval
	err = manager.EnableAutoSave(ctx, agent, threadID, 50*time.Millisecond)
	require.NoError(t, err)

	// Wait for a few auto-saves
	time.Sleep(200 * time.Millisecond)

	// Disable auto-save
	manager.DisableAutoSave()

	// Verify multiple checkpoints were created
	checkpoints, err := store.List(ctx, threadID, 10)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(checkpoints), 2, "Expected at least 2 auto-saved checkpoints")
}

// mockAgent is a simple mock implementation of Agent interface for testing
type mockAgent struct {
	id    string
	state State
}

func (m *mockAgent) ID() string {
	return m.id
}

func (m *mockAgent) Name() string {
	return "Mock Agent"
}

func (m *mockAgent) Type() AgentType {
	return "mock"
}

func (m *mockAgent) State() State {
	return m.state
}

func (m *mockAgent) Init(ctx context.Context) error {
	m.state = StateReady
	return nil
}

func (m *mockAgent) Teardown(ctx context.Context) error {
	return nil
}

func (m *mockAgent) Plan(ctx context.Context, input *Input) (*PlanResult, error) {
	return &PlanResult{}, nil
}

func (m *mockAgent) Execute(ctx context.Context, input *Input) (*Output, error) {
	return &Output{}, nil
}

func (m *mockAgent) Observe(ctx context.Context, feedback *Feedback) error {
	return nil
}

func (m *mockAgent) Transition(ctx context.Context, newState State) error {
	m.state = newState
	return nil
}
