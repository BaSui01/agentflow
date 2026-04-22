package agent_test

import (
	"context"
	"testing"
	"time"

	. "github.com/BaSui01/agentflow/agent"
	agentcheckpoint "github.com/BaSui01/agentflow/agent/persistence/checkpoint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestCheckpointManager_CreateCheckpoint(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	store, err := agentcheckpoint.NewFileCheckpointStore(t.TempDir(), logger)
	require.NoError(t, err)

	manager := NewCheckpointManager(store, logger)

	// 创建模拟代理
	agent := &mockAgent{
		id:    "test-agent",
		state: StateReady,
	}

	threadID := "test-thread"

	// 创建检查站
	err = manager.CreateCheckpoint(context.Background(), agent, threadID)
	require.NoError(t, err)

	// 检查检查站已经建立
	checkpoints, err := store.List(context.Background(), threadID, 10)
	require.NoError(t, err)
	assert.Len(t, checkpoints, 1)
	assert.Equal(t, agent.id, checkpoints[0].AgentID)
	assert.Equal(t, StateReady, checkpoints[0].State)
}

func TestCheckpointManager_RollbackToVersion(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	store, err := agentcheckpoint.NewFileCheckpointStore(t.TempDir(), logger)
	require.NoError(t, err)

	manager := NewCheckpointManager(store, logger)

	// 创建模拟代理
	agent := &mockAgent{
		id:    "test-agent",
		state: StateInit,
	}

	threadID := "test-thread"
	ctx := context.Background()

	// 创建第一个检查点版本。
	err = manager.CreateCheckpoint(ctx, agent, threadID)
	require.NoError(t, err)

	// 切换状态后创建第二个检查点版本。
	agent.state = StateRunning
	err = manager.CreateCheckpoint(ctx, agent, threadID)
	require.NoError(t, err)

	// 再次切换状态后创建第三个检查点版本。
	agent.state = StateReady
	err = manager.CreateCheckpoint(ctx, agent, threadID)
	require.NoError(t, err)

	// 核实我们有3个版本
	versions, err := manager.ListVersions(ctx, threadID)
	require.NoError(t, err)
	assert.Len(t, versions, 3)

	// 回滚到第一版
	err = manager.RollbackToVersion(ctx, agent, threadID, 1)
	require.NoError(t, err)

	// 验证代理状态已恢复
	assert.Equal(t, StateInit, agent.state)

	// 核查新检查站(第4版)
	versions, err = manager.ListVersions(ctx, threadID)
	require.NoError(t, err)
	assert.Len(t, versions, 4)
	assert.Equal(t, 4, versions[3].Version)
}

func TestCheckpointManager_CompareVersions(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	store, err := agentcheckpoint.NewFileCheckpointStore(t.TempDir(), logger)
	require.NoError(t, err)

	manager := NewCheckpointManager(store, logger)

	agent := &mockAgent{
		id:    "test-agent",
		state: StateInit,
	}

	threadID := "test-thread"
	ctx := context.Background()

	// 创建第一个检查站
	err = manager.CreateCheckpoint(ctx, agent, threadID)
	require.NoError(t, err)

	time.Sleep(10 * time.Millisecond)

	// 更改状态并创建第二个检查站
	agent.state = StateReady
	err = manager.CreateCheckpoint(ctx, agent, threadID)
	require.NoError(t, err)

	// 比较版本
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
	store, err := agentcheckpoint.NewFileCheckpointStore(t.TempDir(), logger)
	require.NoError(t, err)

	manager := NewCheckpointManager(store, logger)

	agent := &mockAgent{
		id:    "test-agent",
		state: StateReady,
	}

	threadID := "test-thread"
	ctx := context.Background()

	// 启用短间隔自动保存
	err = manager.EnableAutoSave(ctx, agent, threadID, 50*time.Millisecond)
	require.NoError(t, err)

	// 等几个自动取出
	time.Sleep(200 * time.Millisecond)

	// 禁用自动保存
	manager.DisableAutoSave()

	// 核查设立了多个检查站
	checkpoints, err := store.List(ctx, threadID, 10)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(checkpoints), 2, "Expected at least 2 auto-saved checkpoints")
}

func TestCheckpointManager_LoadCheckpointForAgent(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	store, err := agentcheckpoint.NewFileCheckpointStore(t.TempDir(), logger)
	require.NoError(t, err)

	manager := NewCheckpointManager(store, logger)
	agent := &mockAgent{id: "test-agent", state: StateReady}
	checkpoint := &Checkpoint{
		ID:        GenerateCheckpointID(),
		ThreadID:  "thread-1",
		AgentID:   "test-agent",
		State:     StateRunning,
		CreatedAt: time.Now(),
	}
	require.NoError(t, store.Save(context.Background(), checkpoint))

	loaded, err := manager.LoadCheckpointForAgent(context.Background(), agent, checkpoint.ID)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, checkpoint.ID, loaded.ID)
	assert.Equal(t, StateRunning, agent.state)
}

func TestCheckpointManager_LoadLatestCheckpointForAgent(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	store, err := agentcheckpoint.NewFileCheckpointStore(t.TempDir(), logger)
	require.NoError(t, err)

	manager := NewCheckpointManager(store, logger)
	agent := &mockAgent{id: "test-agent", state: StateReady}
	threadID := "thread-1"
	require.NoError(t, store.Save(context.Background(), &Checkpoint{
		ID:        GenerateCheckpointID(),
		ThreadID:  threadID,
		AgentID:   "test-agent",
		State:     StateInit,
		CreatedAt: time.Now().Add(-time.Minute),
	}))
	latest := &Checkpoint{
		ID:        GenerateCheckpointID(),
		ThreadID:  threadID,
		AgentID:   "test-agent",
		State:     StateRunning,
		CreatedAt: time.Now(),
	}
	require.NoError(t, store.Save(context.Background(), latest))

	loaded, err := manager.LoadLatestCheckpointForAgent(context.Background(), agent, threadID)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, latest.ID, loaded.ID)
	assert.Equal(t, StateRunning, agent.state)
}

func TestCheckpointManager_SaveCheckpointPreservesLoopPersistenceFields(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	store, err := agentcheckpoint.NewFileCheckpointStore(t.TempDir(), logger)
	require.NoError(t, err)

	manager := NewCheckpointManager(store, logger)
	checkpoint := &Checkpoint{
		ID:                  "cp-loop-fields",
		ThreadID:            "thread-1",
		AgentID:             "test-agent",
		LoopStateID:         "loop-1",
		RunID:               "run-1",
		Goal:                "close task",
		CurrentPlanID:       "loop-1-plan-4",
		PlanVersion:         4,
		CurrentStepID:       "step-4",
		ObservationsSummary: "plan:ready | act:partial",
		LastOutputSummary:   "partial answer",
		LastError:           "temporary failure",
		State:               StateRunning,
		Metadata: map[string]any{
			"loop_state_id":        "loop-1",
			"current_plan_id":      "loop-1-plan-4",
			"plan_version":         4,
			"observations_summary": "plan:ready | act:partial",
			"last_output_summary":  "partial answer",
			"last_error":           "temporary failure",
		},
		ExecutionContext: &ExecutionContext{
			CurrentNode:         string(LoopStageObserve),
			LoopStateID:         "loop-1",
			RunID:               "run-1",
			AgentID:             "test-agent",
			Goal:                "close task",
			CurrentPlanID:       "loop-1-plan-4",
			PlanVersion:         4,
			CurrentStepID:       "step-4",
			ObservationsSummary: "plan:ready | act:partial",
			LastOutputSummary:   "partial answer",
			LastError:           "temporary failure",
			Variables: map[string]any{
				"loop_state_id":        "loop-1",
				"current_plan_id":      "loop-1-plan-4",
				"plan_version":         4,
				"observations_summary": "plan:ready | act:partial",
				"last_output_summary":  "partial answer",
				"last_error":           "temporary failure",
			},
		},
	}

	require.NoError(t, manager.SaveCheckpoint(context.Background(), checkpoint))

	loaded, err := manager.LoadCheckpoint(context.Background(), checkpoint.ID)
	require.NoError(t, err)
	assert.Equal(t, "loop-1", loaded.LoopStateID)
	assert.Equal(t, "run-1", loaded.RunID)
	assert.Equal(t, "loop-1-plan-4", loaded.CurrentPlanID)
	assert.Equal(t, 4, loaded.PlanVersion)
	assert.Equal(t, "step-4", loaded.CurrentStepID)
	assert.Equal(t, "plan:ready | act:partial", loaded.ObservationsSummary)
	assert.Equal(t, "partial answer", loaded.LastOutputSummary)
	assert.Equal(t, "temporary failure", loaded.LastError)
	require.NotNil(t, loaded.ExecutionContext)
	assert.Equal(t, "loop-1", loaded.ExecutionContext.LoopStateID)
	assert.Equal(t, "temporary failure", loaded.ExecutionContext.LastError)
}

func TestCheckpointManager_SaveCheckpointNormalizesLoopPersistenceFieldsFromExecutionContext(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	store, err := agentcheckpoint.NewFileCheckpointStore(t.TempDir(), logger)
	require.NoError(t, err)

	manager := NewCheckpointManager(store, logger)
	checkpoint := &Checkpoint{
		ID:       "cp-normalize",
		ThreadID: "thread-2",
		AgentID:  "test-agent",
		State:    StateRunning,
		ExecutionContext: &ExecutionContext{
			CurrentNode: string(LoopStageObserve),
			Variables: map[string]any{
				"loop_state_id":        "loop-ctx",
				"run_id":               "run-ctx",
				"goal":                 "resume work",
				"current_plan_id":      "loop-ctx-plan-3",
				"plan_version":         3,
				"current_step":         "step-3",
				"observations_summary": "plan:ready | act:partial",
				"last_output_summary":  "partial output",
				"last_error":           "temporary issue",
			},
		},
	}

	require.NoError(t, manager.SaveCheckpoint(context.Background(), checkpoint))

	loaded, err := manager.LoadCheckpoint(context.Background(), checkpoint.ID)
	require.NoError(t, err)
	assert.Equal(t, "loop-ctx", loaded.LoopStateID)
	assert.Equal(t, "run-ctx", loaded.RunID)
	assert.Equal(t, "resume work", loaded.Goal)
	assert.Equal(t, "loop-ctx-plan-3", loaded.CurrentPlanID)
	assert.Equal(t, 3, loaded.PlanVersion)
	assert.Equal(t, "step-3", loaded.CurrentStepID)
	assert.Equal(t, "plan:ready | act:partial", loaded.ObservationsSummary)
	assert.Equal(t, "partial output", loaded.LastOutputSummary)
	assert.Equal(t, "temporary issue", loaded.LastError)
	require.NotNil(t, loaded.Metadata)
	assert.Equal(t, "step-3", loaded.Metadata["current_step"])
	require.NotNil(t, loaded.ExecutionContext)
	assert.Equal(t, "step-3", loaded.ExecutionContext.Variables["current_step_id"])
}

// 模拟 Agent 是 Agent 接口的简单模拟执行,用于测试
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
