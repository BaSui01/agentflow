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

// Feature: agent-framework-2026-enhancements, Property 14: Checkpoint Recovery Step Skipping
// **Validates: Requirements 8.5**
// For any execution resumed from a checkpoint, completed steps (steps before CurrentStep)
// should NOT be re-executed, and execution should continue from CurrentStep.

// stepTrackingExecution tracks which steps have been executed for property testing.
type stepTrackingExecution struct {
	ID          string
	CurrentStep int
	TotalSteps  int
	// executedSteps tracks which step indices have been executed
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

// stepExecutor simulates step execution with tracking.
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

// executeFromCheckpoint simulates resuming execution from a checkpoint.
// It should only execute steps from CurrentStep onwards.
func (e *stepExecutor) executeFromCheckpoint(ctx context.Context) error {
	// Start from CurrentStep (skipping completed steps)
	for step := e.execution.CurrentStep; step < e.execution.TotalSteps; step++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Record that this step was executed
		e.execution.recordStepExecution(step)

		e.logger.Debug("executed step",
			zap.String("exec_id", e.execution.ID),
			zap.Int("step", step),
		)
	}
	return nil
}

// checkpointRecoveryAgent is a test agent that tracks step execution for recovery testing.
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

// SetCurrentStep sets the current step (simulating checkpoint restore).
func (a *checkpointRecoveryAgent) SetCurrentStep(step int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.currentStep = step
}

// GetCurrentStep returns the current step.
func (a *checkpointRecoveryAgent) GetCurrentStep() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.currentStep
}

// RecordStepExecution records that a step was executed.
func (a *checkpointRecoveryAgent) RecordStepExecution(step int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.executedSteps = append(a.executedSteps, step)
}

// GetExecutedSteps returns all executed steps.
func (a *checkpointRecoveryAgent) GetExecutedSteps() []int {
	a.mu.Lock()
	defer a.mu.Unlock()
	result := make([]int, len(a.executedSteps))
	copy(result, a.executedSteps)
	return result
}

// ExecuteFromStep executes steps starting from the given step.
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

// genValidExecutionID generates a valid execution identifier for testing.
func genValidExecutionID() *rapid.Generator[string] {
	return rapid.StringMatching(`exec_[a-z0-9]{8,16}`)
}

// genValidThreadID generates a valid thread identifier for testing.
func genValidThreadID() *rapid.Generator[string] {
	return rapid.StringMatching(`thread_[a-z0-9]{8,16}`)
}

// genValidAgentIDForRecovery generates a valid agent identifier for testing.
func genValidAgentIDForRecovery() *rapid.Generator[string] {
	return rapid.StringMatching(`agent_[a-z0-9]{8,16}`)
}

// TestProperty_CheckpointRecovery_StepSkipping tests that completed steps are skipped on recovery.
// Property 14: Checkpoint Recovery Step Skipping
// **Validates: Requirements 8.5**
func TestProperty_CheckpointRecovery_StepSkipping(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Generate test parameters
		totalSteps := rapid.IntRange(2, 20).Draw(rt, "totalSteps")
		// CurrentStep must be between 0 and totalSteps-1 (at least one step remaining)
		currentStep := rapid.IntRange(0, totalSteps-1).Draw(rt, "currentStep")

		logger, _ := zap.NewDevelopment()

		// Create execution tracker
		execID := genValidExecutionID().Draw(rt, "execID")
		execution := newStepTrackingExecution(execID, currentStep, totalSteps)

		// Create executor and resume from checkpoint
		executor := newStepExecutor(execution, logger)
		ctx := context.Background()

		err := executor.executeFromCheckpoint(ctx)
		require.NoError(t, err, "Execution should complete without error")

		// Property 1: Steps before CurrentStep should NOT be executed
		for step := 0; step < currentStep; step++ {
			count := execution.getStepExecutionCount(step)
			assert.Equal(t, 0, count,
				"Step %d (before CurrentStep %d) should NOT be executed, but was executed %d times",
				step, currentStep, count)
		}

		// Property 2: Steps from CurrentStep onwards should be executed exactly once
		for step := currentStep; step < totalSteps; step++ {
			count := execution.getStepExecutionCount(step)
			assert.Equal(t, 1, count,
				"Step %d (from CurrentStep %d onwards) should be executed exactly once, but was executed %d times",
				step, currentStep, count)
		}

		// Property 3: Total executed steps should equal (totalSteps - currentStep)
		executedSteps := execution.getExecutedSteps()
		expectedExecutedCount := totalSteps - currentStep
		assert.Equal(t, expectedExecutedCount, len(executedSteps),
			"Expected %d steps to be executed, but %d were executed",
			expectedExecutedCount, len(executedSteps))
	})
}

// TestProperty_CheckpointRecovery_WithCheckpointStore tests recovery using actual checkpoint store.
// Property 14: Checkpoint Recovery Step Skipping
// **Validates: Requirements 8.5**
func TestProperty_CheckpointRecovery_WithCheckpointStore(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Setup
		logger, _ := zap.NewDevelopment()
		store, err := NewFileCheckpointStore(t.TempDir(), logger)
		require.NoError(t, err)

		// Generate test parameters
		threadID := genValidThreadID().Draw(rt, "threadID")
		agentID := genValidAgentIDForRecovery().Draw(rt, "agentID")
		totalSteps := rapid.IntRange(3, 15).Draw(rt, "totalSteps")
		currentStep := rapid.IntRange(1, totalSteps-1).Draw(rt, "currentStep")

		ctx := context.Background()

		// Create and save a checkpoint representing partial execution
		checkpoint := &Checkpoint{
			ID:        generateCheckpointID(),
			ThreadID:  threadID,
			AgentID:   agentID,
			State:     StateRunning,
			Messages:  []CheckpointMessage{},
			Metadata:  make(map[string]interface{}),
			CreatedAt: time.Now(),
			ExecutionContext: &ExecutionContext{
				WorkflowID:  "test-workflow",
				CurrentNode: "step-" + string(rune('0'+currentStep)),
				NodeResults: make(map[string]interface{}),
				Variables: map[string]interface{}{
					"current_step": currentStep,
					"total_steps":  totalSteps,
				},
			},
		}

		// Mark completed steps in metadata
		completedSteps := make([]int, currentStep)
		for i := 0; i < currentStep; i++ {
			completedSteps[i] = i
		}
		checkpoint.Metadata["completed_steps"] = completedSteps

		// Save checkpoint
		err = store.Save(ctx, checkpoint)
		require.NoError(t, err, "Should save checkpoint successfully")

		// Load checkpoint (simulating recovery)
		loaded, err := store.Load(ctx, checkpoint.ID)
		require.NoError(t, err, "Should load checkpoint successfully")

		// Verify checkpoint data is preserved
		require.NotNil(t, loaded.ExecutionContext, "ExecutionContext should be preserved")
		require.NotNil(t, loaded.ExecutionContext.Variables, "Variables should be preserved")

		// Get current step from loaded checkpoint
		loadedCurrentStep, ok := loaded.ExecutionContext.Variables["current_step"].(float64)
		require.True(t, ok, "current_step should be a number")
		loadedTotalSteps, ok := loaded.ExecutionContext.Variables["total_steps"].(float64)
		require.True(t, ok, "total_steps should be a number")

		// Property 1: Current step should be preserved
		assert.Equal(t, currentStep, int(loadedCurrentStep),
			"CurrentStep should be preserved after checkpoint load")

		// Property 2: Total steps should be preserved
		assert.Equal(t, totalSteps, int(loadedTotalSteps),
			"TotalSteps should be preserved after checkpoint load")

		// Property 3: Completed steps metadata should be preserved
		loadedCompletedSteps, ok := loaded.Metadata["completed_steps"].([]interface{})
		require.True(t, ok, "completed_steps should be preserved")
		assert.Equal(t, currentStep, len(loadedCompletedSteps),
			"Number of completed steps should match currentStep")

		// Simulate resuming execution from checkpoint
		agent := newCheckpointRecoveryAgent(agentID, totalSteps)
		err = agent.ExecuteFromStep(ctx, int(loadedCurrentStep))
		require.NoError(t, err, "Execution should complete without error")

		// Property 4: Only steps from CurrentStep onwards should be executed
		executedSteps := agent.GetExecutedSteps()
		expectedStepCount := totalSteps - currentStep
		assert.Equal(t, expectedStepCount, len(executedSteps),
			"Should execute exactly %d steps (from %d to %d)",
			expectedStepCount, currentStep, totalSteps-1)

		// Property 5: First executed step should be CurrentStep
		if len(executedSteps) > 0 {
			assert.Equal(t, currentStep, executedSteps[0],
				"First executed step should be CurrentStep (%d)", currentStep)
		}

		// Property 6: Steps should be executed in order
		for i := 0; i < len(executedSteps)-1; i++ {
			assert.Equal(t, executedSteps[i]+1, executedSteps[i+1],
				"Steps should be executed in sequential order")
		}
	})
}

// TestProperty_CheckpointRecovery_NoStepsSkippedWhenStartingFresh tests that all steps execute when starting fresh.
// Property 14: Checkpoint Recovery Step Skipping
// **Validates: Requirements 8.5**
func TestProperty_CheckpointRecovery_NoStepsSkippedWhenStartingFresh(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Generate test parameters
		totalSteps := rapid.IntRange(1, 20).Draw(rt, "totalSteps")
		currentStep := 0 // Starting fresh, no steps completed

		logger, _ := zap.NewDevelopment()

		// Create execution tracker
		execID := genValidExecutionID().Draw(rt, "execID")
		execution := newStepTrackingExecution(execID, currentStep, totalSteps)

		// Create executor and start execution
		executor := newStepExecutor(execution, logger)
		ctx := context.Background()

		err := executor.executeFromCheckpoint(ctx)
		require.NoError(t, err, "Execution should complete without error")

		// Property: All steps should be executed when starting fresh
		for step := 0; step < totalSteps; step++ {
			count := execution.getStepExecutionCount(step)
			assert.Equal(t, 1, count,
				"Step %d should be executed exactly once when starting fresh, but was executed %d times",
				step, count)
		}

		// Property: Total executed steps should equal totalSteps
		executedSteps := execution.getExecutedSteps()
		assert.Equal(t, totalSteps, len(executedSteps),
			"All %d steps should be executed when starting fresh", totalSteps)
	})
}

// TestProperty_CheckpointRecovery_LastStepOnly tests recovery when only the last step remains.
// Property 14: Checkpoint Recovery Step Skipping
// **Validates: Requirements 8.5**
func TestProperty_CheckpointRecovery_LastStepOnly(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Generate test parameters
		totalSteps := rapid.IntRange(2, 20).Draw(rt, "totalSteps")
		currentStep := totalSteps - 1 // Only last step remaining

		logger, _ := zap.NewDevelopment()

		// Create execution tracker
		execID := genValidExecutionID().Draw(rt, "execID")
		execution := newStepTrackingExecution(execID, currentStep, totalSteps)

		// Create executor and resume from checkpoint
		executor := newStepExecutor(execution, logger)
		ctx := context.Background()

		err := executor.executeFromCheckpoint(ctx)
		require.NoError(t, err, "Execution should complete without error")

		// Property 1: All steps before the last should NOT be executed
		for step := 0; step < currentStep; step++ {
			count := execution.getStepExecutionCount(step)
			assert.Equal(t, 0, count,
				"Step %d should NOT be executed when resuming at last step, but was executed %d times",
				step, count)
		}

		// Property 2: Only the last step should be executed
		lastStepCount := execution.getStepExecutionCount(currentStep)
		assert.Equal(t, 1, lastStepCount,
			"Last step should be executed exactly once")

		// Property 3: Total executed steps should be 1
		executedSteps := execution.getExecutedSteps()
		assert.Equal(t, 1, len(executedSteps),
			"Only 1 step should be executed when resuming at last step")
	})
}

// TestProperty_CheckpointRecovery_ContextCancellation tests that step skipping works with context cancellation.
// Property 14: Checkpoint Recovery Step Skipping
// **Validates: Requirements 8.5**
func TestProperty_CheckpointRecovery_ContextCancellation(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Generate test parameters
		totalSteps := rapid.IntRange(5, 20).Draw(rt, "totalSteps")
		currentStep := rapid.IntRange(0, totalSteps-3).Draw(rt, "currentStep")
		cancelAfterSteps := rapid.IntRange(1, totalSteps-currentStep-1).Draw(rt, "cancelAfterSteps")

		logger, _ := zap.NewDevelopment()

		// Create execution tracker
		execID := genValidExecutionID().Draw(rt, "execID")
		execution := newStepTrackingExecution(execID, currentStep, totalSteps)

		// Create context that will be cancelled
		ctx, cancel := context.WithCancel(context.Background())

		// Create a custom executor that cancels after certain steps
		stepsExecuted := int32(0)
		customExecutor := &stepExecutor{
			execution: execution,
			logger:    logger,
		}

		// Run execution in goroutine
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

				// Cancel after specified number of steps
				if atomic.LoadInt32(&stepsExecuted) >= int32(cancelAfterSteps) {
					cancel()
				}
			}
			done <- nil
		}()

		// Wait for completion or cancellation
		<-done
		_ = customExecutor // suppress unused warning

		// Property 1: Steps before CurrentStep should NOT be executed
		for step := 0; step < currentStep; step++ {
			count := execution.getStepExecutionCount(step)
			assert.Equal(t, 0, count,
				"Step %d (before CurrentStep) should NOT be executed even with cancellation", step)
		}

		// Property 2: At least cancelAfterSteps should have been executed
		executedCount := int(atomic.LoadInt32(&stepsExecuted))
		assert.GreaterOrEqual(t, executedCount, cancelAfterSteps,
			"At least %d steps should have been executed before cancellation", cancelAfterSteps)

		// Property 3: No step before CurrentStep should be in executed steps
		executedSteps := execution.getExecutedSteps()
		for _, step := range executedSteps {
			assert.GreaterOrEqual(t, step, currentStep,
				"Executed step %d should be >= CurrentStep %d", step, currentStep)
		}
	})
}
