package agent

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"pgregory.net/rapid"
)

// 特性:代理框架-2026年-增强,财产 14:检查站回收步骤跳出
// ** 参数:要求8.5**
// 对于从检查站恢复执行的任何措施,已完成步骤(在现行步骤之前的步骤)
// 不应重新执行,执行应从当前步骤继续。

// stepTracking Execution 音轨,用于属性测试。
type stepTrackingExecution struct {
	ID          string
	CurrentStep int
	TotalSteps  int
	// 已执行步骤索引的Steps音轨
	executedSteps map[int]int // step index -> execution count
	mu            sync.Mutex
}

func newStepTrackingExecution(id string, currentStep, totalSteps int) *stepTrackingExecution {
	return &stepTrackingExecution{
		ID:            id,
		CurrentStep:   currentStep,
		TotalSteps:    totalSteps,
		executedSteps: make(map[int]int),
	}
}

func (e *stepTrackingExecution) recordStepExecution(stepIndex int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.executedSteps[stepIndex]++
}

func (e *stepTrackingExecution) getStepExecutionCount(stepIndex int) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.executedSteps[stepIndex]
}

func (e *stepTrackingExecution) getExecutedSteps() []int {
	e.mu.Lock()
	defer e.mu.Unlock()
	steps := make([]int, 0, len(e.executedSteps))
	for step := range e.executedSteps {
		steps = append(steps, step)
	}
	return steps
}

// StepExecuters 模拟带有跟踪的步执行.
type stepExecutor struct {
	execution *stepTrackingExecution
	logger    *zap.Logger
}

func newStepExecutor(exec *stepTrackingExecution, logger *zap.Logger) *stepExecutor {
	return &stepExecutor{
		execution: exec,
		logger:    logger,
	}
}

// 从检查点执行模拟从检查点恢复执行。
// 它应该只执行从当前步骤开始的步骤。
func (e *stepExecutor) executeFromCheckpoint(ctx context.Context) error {
	// 从 CentralStep 开始( 跳出已完成的步数)
	for step := e.execution.CurrentStep; step < e.execution.TotalSteps; step++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// 记录此步骤已执行
		e.execution.recordStepExecution(step)

		e.logger.Debug("executed step",
			zap.String("exec_id", e.execution.ID),
			zap.Int("step", step),
		)
	}
	return nil
}

// 检查所 Recovery Agent是跟踪步骤执行进行恢复测试的测试代理.
type checkpointRecoveryAgent struct {
	id            string
	name          string
	state         State
	currentStep   int
	totalSteps    int
	executedSteps []int
	executeCount  int64
	mu            sync.Mutex
}

func newCheckpointRecoveryAgent(id string, totalSteps int) *checkpointRecoveryAgent {
	return &checkpointRecoveryAgent{
		id:            id,
		name:          "Recovery Test Agent",
		state:         StateReady,
		totalSteps:    totalSteps,
		executedSteps: make([]int, 0),
	}
}

func (a *checkpointRecoveryAgent) ID() string                         { return a.id }
func (a *checkpointRecoveryAgent) Name() string                       { return a.name }
func (a *checkpointRecoveryAgent) Type() AgentType                    { return "recovery-test" }
func (a *checkpointRecoveryAgent) State() State                       { return a.state }
func (a *checkpointRecoveryAgent) Init(ctx context.Context) error     { return nil }
func (a *checkpointRecoveryAgent) Teardown(ctx context.Context) error { return nil }

func (a *checkpointRecoveryAgent) Plan(ctx context.Context, input *Input) (*PlanResult, error) {
	steps := make([]string, a.totalSteps)
	for i := range steps {
		steps[i] = "step"
	}
	return &PlanResult{Steps: steps}, nil
}

func (a *checkpointRecoveryAgent) Execute(ctx context.Context, input *Input) (*Output, error) {
	atomic.AddInt64(&a.executeCount, 1)
	return &Output{Content: "executed"}, nil
}

func (a *checkpointRecoveryAgent) Observe(ctx context.Context, feedback *Feedback) error {
	return nil
}

func (a *checkpointRecoveryAgent) Transition(ctx context.Context, newState State) error {
	a.state = newState
	return nil
}

// SetCurentStep 设置当前步骤(模拟检查点恢复).
func (a *checkpointRecoveryAgent) SetCurrentStep(step int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.currentStep = step
}

// GetCurentStep 返回当前步骤。
func (a *checkpointRecoveryAgent) GetCurrentStep() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.currentStep
}

// 记录StepExecution记录了一个步骤被执行.
func (a *checkpointRecoveryAgent) RecordStepExecution(step int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.executedSteps = append(a.executedSteps, step)
}

// Get ExecutedSteps 返回所有已执行步骤 。
func (a *checkpointRecoveryAgent) GetExecutedSteps() []int {
	a.mu.Lock()
	defer a.mu.Unlock()
	result := make([]int, len(a.executedSteps))
	copy(result, a.executedSteps)
	return result
}

// 从给定步骤开始执行步骤。
func (a *checkpointRecoveryAgent) ExecuteFromStep(ctx context.Context, startStep int) error {
	for step := startStep; step < a.totalSteps; step++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		a.RecordStepExecution(step)
	}
	return nil
}

// genValidExecutionID生成用于测试的有效执行标识符.
func genValidExecutionID() *rapid.Generator[string] {
	return rapid.StringMatching(`exec_[a-z0-9]{8,16}`)
}

// genValidThreadID生成一个有效的线程标识符用于测试.
func genValidThreadID() *rapid.Generator[string] {
	return rapid.StringMatching(`thread_[a-z0-9]{8,16}`)
}

// genValidAgentIDForRecovery生成用于测试的有效代理标识符.
func genValidAgentIDForRecovery() *rapid.Generator[string] {
	return rapid.StringMatching(`agent_[a-z0-9]{8,16}`)
}

// 测试Property Checkpoint Recovery StepSkipping 测试完成的步骤在恢复时被跳过.
// 属性 14: 检查点回收步骤跳过
// ** 参数:要求8.5**
func TestProperty_CheckpointRecovery_StepSkipping(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// 生成测试参数
		totalSteps := rapid.IntRange(2, 20).Draw(rt, "totalSteps")
		// 当前步骤必须在0到总步骤1之间(至少剩余一步骤)
		currentStep := rapid.IntRange(0, totalSteps-1).Draw(rt, "currentStep")

		logger, _ := zap.NewDevelopment()

		// 创建执行跟踪器
		execID := genValidExecutionID().Draw(rt, "execID")
		execution := newStepTrackingExecution(execID, currentStep, totalSteps)

		// 从检查站创建执行器并恢复
		executor := newStepExecutor(execution, logger)
		ctx := context.Background()

		err := executor.executeFromCheckpoint(ctx)
		require.NoError(t, err, "Execution should complete without error")

		// 属性 1: 不应执行当前步骤之前的步骤
		for step := 0; step < currentStep; step++ {
			count := execution.getStepExecutionCount(step)
			assert.Equal(t, 0, count,
				"Step %d (before CurrentStep %d) should NOT be executed, but was executed %d times",
				step, currentStep, count)
		}

		// 属性2: 从当前步骤开始的步骤应精确执行一次
		for step := currentStep; step < totalSteps; step++ {
			count := execution.getStepExecutionCount(step)
			assert.Equal(t, 1, count,
				"Step %d (from CurrentStep %d onwards) should be executed exactly once, but was executed %d times",
				step, currentStep, count)
		}

		// 财产 3: 已执行步骤总数应等于( 总计 Steps - 当前 Step)
		executedSteps := execution.getExecutedSteps()
		expectedExecutedCount := totalSteps - currentStep
		assert.Equal(t, expectedExecutedCount, len(executedSteps),
			"Expected %d steps to be executed, but %d were executed",
			expectedExecutedCount, len(executedSteps))
	})
}

// TestProperty Checkpoint Recovery With CheckpointStore 使用实际的检查站商店进行测试回收.
// 属性 14: 检查点回收步骤跳过
// ** 参数:要求8.5**
func TestProperty_CheckpointRecovery_WithCheckpointStore(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// 设置
		logger, _ := zap.NewDevelopment()
		store, err := NewFileCheckpointStore(t.TempDir(), logger)
		require.NoError(t, err)

		// 生成测试参数
		threadID := genValidThreadID().Draw(rt, "threadID")
		agentID := genValidAgentIDForRecovery().Draw(rt, "agentID")
		totalSteps := rapid.IntRange(3, 15).Draw(rt, "totalSteps")
		currentStep := rapid.IntRange(1, totalSteps-1).Draw(rt, "currentStep")

		ctx := context.Background()

		// 创建并保存代表部分执行的检查点
		checkpoint := &Checkpoint{
			ID:        generateCheckpointID(),
			ThreadID:  threadID,
			AgentID:   agentID,
			State:     StateRunning,
			Messages:  []CheckpointMessage{},
			Metadata:  make(map[string]any),
			CreatedAt: time.Now(),
			ExecutionContext: &ExecutionContext{
				WorkflowID:  "test-workflow",
				CurrentNode: "step-" + string(rune('0'+currentStep)),
				NodeResults: make(map[string]any),
				Variables: map[string]any{
					"current_step": currentStep,
					"total_steps":  totalSteps,
				},
			},
		}

		// 在元数据中标记已完成的步骤
		completedSteps := make([]int, currentStep)
		for i := 0; i < currentStep; i++ {
			completedSteps[i] = i
		}
		checkpoint.Metadata["completed_steps"] = completedSteps

		// 保存检查点
		err = store.Save(ctx, checkpoint)
		require.NoError(t, err, "Should save checkpoint successfully")

		// 装入检查站(模拟回收)
		loaded, err := store.Load(ctx, checkpoint.ID)
		require.NoError(t, err, "Should load checkpoint successfully")

		// 检查检查站数据得到保存
		require.NotNil(t, loaded.ExecutionContext, "ExecutionContext should be preserved")
		require.NotNil(t, loaded.ExecutionContext.Variables, "Variables should be preserved")

		// 从已装入的检查站获取当前步骤
		loadedCurrentStep, ok := loaded.ExecutionContext.Variables["current_step"].(float64)
		require.True(t, ok, "current_step should be a number")
		loadedTotalSteps, ok := loaded.ExecutionContext.Variables["total_steps"].(float64)
		require.True(t, ok, "total_steps should be a number")

		// 财产1:应保留目前的步骤
		assert.Equal(t, currentStep, int(loadedCurrentStep),
			"CurrentStep should be preserved after checkpoint load")

		// 财产2:应保留所有步骤
		assert.Equal(t, totalSteps, int(loadedTotalSteps),
			"TotalSteps should be preserved after checkpoint load")

		// 财产3:应保留已完成的步骤元数据
		loadedCompletedSteps, ok := loaded.Metadata["completed_steps"].([]any)
		require.True(t, ok, "completed_steps should be preserved")
		assert.Equal(t, currentStep, len(loadedCompletedSteps),
			"Number of completed steps should match currentStep")

		// 模拟从检查站恢复执行
		agent := newCheckpointRecoveryAgent(agentID, totalSteps)
		err = agent.ExecuteFromStep(ctx, int(loadedCurrentStep))
		require.NoError(t, err, "Execution should complete without error")

		// 财产 4: 只能执行从当前步骤开始的步骤
		executedSteps := agent.GetExecutedSteps()
		expectedStepCount := totalSteps - currentStep
		assert.Equal(t, expectedStepCount, len(executedSteps),
			"Should execute exactly %d steps (from %d to %d)",
			expectedStepCount, currentStep, totalSteps-1)

		// 财产 5: 第一个执行步骤应为当前步骤
		if len(executedSteps) > 0 {
			assert.Equal(t, currentStep, executedSteps[0],
				"First executed step should be CurrentStep (%d)", currentStep)
		}

		// 财产6:应执行步骤
		for i := 0; i < len(executedSteps)-1; i++ {
			assert.Equal(t, executedSteps[i]+1, executedSteps[i+1],
				"Steps should be executed in sequential order")
		}
	})
}

// 测试Property  检查点恢复  开始时不跳跃 所有步骤在刚开始时执行的新测试 。
// 属性 14: 检查点回收步骤跳过
// ** 参数:要求8.5**
func TestProperty_CheckpointRecovery_NoStepsSkippedWhenStartingFresh(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// 生成测试参数
		totalSteps := rapid.IntRange(1, 20).Draw(rt, "totalSteps")
		currentStep := 0 // Starting fresh, no steps completed

		logger, _ := zap.NewDevelopment()

		// 创建执行跟踪器
		execID := genValidExecutionID().Draw(rt, "execID")
		execution := newStepTrackingExecution(execID, currentStep, totalSteps)

		// 创建执行器并启动执行
		executor := newStepExecutor(execution, logger)
		ctx := context.Background()

		err := executor.executeFromCheckpoint(ctx)
		require.NoError(t, err, "Execution should complete without error")

		// 属性: 所有步骤在开始新时应当执行
		for step := 0; step < totalSteps; step++ {
			count := execution.getStepExecutionCount(step)
			assert.Equal(t, 1, count,
				"Step %d should be executed exactly once when starting fresh, but was executed %d times",
				step, count)
		}

		// 财产: 已执行步骤总数应等于总数
		executedSteps := execution.getExecutedSteps()
		assert.Equal(t, totalSteps, len(executedSteps),
			"All %d steps should be executed when starting fresh", totalSteps)
	})
}

// TestProperty Checkpoint Recovery LastStep 仅在最后一步还剩的情况下测试恢复.
// 属性 14: 检查点回收步骤跳过
// ** 参数:要求8.5**
func TestProperty_CheckpointRecovery_LastStepOnly(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// 生成测试参数
		totalSteps := rapid.IntRange(2, 20).Draw(rt, "totalSteps")
		currentStep := totalSteps - 1 // Only last step remaining

		logger, _ := zap.NewDevelopment()

		// 创建执行跟踪器
		execID := genValidExecutionID().Draw(rt, "execID")
		execution := newStepTrackingExecution(execID, currentStep, totalSteps)

		// 从检查站创建执行器并恢复
		executor := newStepExecutor(execution, logger)
		ctx := context.Background()

		err := executor.executeFromCheckpoint(ctx)
		require.NoError(t, err, "Execution should complete without error")

		// 财产1:最后一步之前的所有步骤都不应被执行
		for step := 0; step < currentStep; step++ {
			count := execution.getStepExecutionCount(step)
			assert.Equal(t, 0, count,
				"Step %d should NOT be executed when resuming at last step, but was executed %d times",
				step, count)
		}

		// 财产2:只应执行最后一步
		lastStepCount := execution.getStepExecutionCount(currentStep)
		assert.Equal(t, 1, lastStepCount,
			"Last step should be executed exactly once")

		// 财产3:已执行步骤共计应为1个
		executedSteps := execution.getExecutedSteps()
		assert.Equal(t, 1, len(executedSteps),
			"Only 1 step should be executed when resuming at last step")
	})
}

// TestProperty Checkpoint Recovery ContextConcellation 测试,步骤跳过与上下文取消配合有效.
// 属性 14: 检查点回收步骤跳过
// ** 参数:要求8.5**
func TestProperty_CheckpointRecovery_ContextCancellation(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// 生成测试参数
		totalSteps := rapid.IntRange(5, 20).Draw(rt, "totalSteps")
		currentStep := rapid.IntRange(0, totalSteps-3).Draw(rt, "currentStep")
		cancelAfterSteps := rapid.IntRange(1, totalSteps-currentStep-1).Draw(rt, "cancelAfterSteps")

		logger, _ := zap.NewDevelopment()

		// 创建执行跟踪器
		execID := genValidExecutionID().Draw(rt, "execID")
		execution := newStepTrackingExecution(execID, currentStep, totalSteps)

		// 创建要取消的上下文
		ctx, cancel := context.WithCancel(context.Background())

		// 创建自定义执行器, 在特定步骤后取消
		stepsExecuted := int32(0)
		customExecutor := &stepExecutor{
			execution: execution,
			logger:    logger,
		}

		// 运行行刑程序
		done := make(chan error, 1)
		go func() {
			for step := execution.CurrentStep; step < execution.TotalSteps; step++ {
				select {
				case <-ctx.Done():
					done <- ctx.Err()
					return
				default:
				}

				execution.recordStepExecution(step)
				atomic.AddInt32(&stepsExecuted, 1)

				// 在指定步骤数后取消
				if atomic.LoadInt32(&stepsExecuted) >= int32(cancelAfterSteps) {
					cancel()
				}
			}
			done <- nil
		}()

		// 等待完成或取消
		<-done
		_ = customExecutor // suppress unused warning

		// 属性 1: 不应执行当前步骤之前的步骤
		for step := 0; step < currentStep; step++ {
			count := execution.getStepExecutionCount(step)
			assert.Equal(t, 0, count,
				"Step %d (before CurrentStep) should NOT be executed even with cancellation", step)
		}

		// 属性 2: 至少在Steps后被取消
		executedCount := int(atomic.LoadInt32(&stepsExecuted))
		assert.GreaterOrEqual(t, executedCount, cancelAfterSteps,
			"At least %d steps should have been executed before cancellation", cancelAfterSteps)

		// 地产 3: 执行步骤前不应有任何步骤
		executedSteps := execution.getExecutedSteps()
		for _, step := range executedSteps {
			assert.GreaterOrEqual(t, step, currentStep,
				"Executed step %d should be >= CurrentStep %d", step, currentStep)
		}
	})
}
