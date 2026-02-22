// 工作流端到端测试。
//
// 覆盖工作流定义、执行与结果校验流程。
//go:build e2e

package e2e

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/BaSui01/agentflow/testutil/fixtures"
	"github.com/BaSui01/agentflow/testutil/mocks"
	"github.com/BaSui01/agentflow/types"
)

// --- 工作流测试 ---

// TestWorkflow_SimpleSequential 测试简单的顺序工作流
func TestWorkflow_SimpleSequential(t *testing.T) {
	env := NewTestEnv(t)
	ctx := env.Context()

	steps := []struct {
		name     string
		input    string
		expected string
	}{
		{"step1", "Start workflow", "Step 1 completed"},
		{"step2", "Continue workflow", "Step 2 completed"},
		{"step3", "Finish workflow", "Step 3 completed"},
	}

	results := make([]string, 0, len(steps))
	for _, step := range steps {
		env.Provider.WithResponse(step.expected)
		req := &mocks.GenerateRequest{
			Messages: []types.Message{fixtures.UserMessage(step.input)},
		}
		resp, err := env.Provider.Generate(ctx, req)
		require.NoError(t, err, "Step %s failed", step.name)
		results = append(results, resp.Content)
	}

	assert.Len(t, results, 3)
	for i, step := range steps {
		assert.Equal(t, step.expected, results[i])
	}
}

// TestWorkflow_ParallelExecution 测试并行工作流执行
func TestWorkflow_ParallelExecution(t *testing.T) {
	env := NewTestEnv(t)
	ctx := env.Context()
	tasks := []string{"Task A", "Task B", "Task C", "Task D"}
	env.Provider.WithResponse("Task completed")

	var wg sync.WaitGroup
	results := make(chan string, len(tasks))
	errors := make(chan error, len(tasks))

	for _, task := range tasks {
		wg.Add(1)
		go func(taskName string) {
			defer wg.Done()
			req := &mocks.GenerateRequest{
				Messages: []types.Message{fixtures.UserMessage(taskName)},
			}
			resp, err := env.Provider.Generate(ctx, req)
			if err != nil {
				errors <- err
				return
			}
			results <- resp.Content
		}(task)
	}

	wg.Wait()
	close(results)
	close(errors)

	var resultList []string
	for r := range results {
		resultList = append(resultList, r)
	}
	var errorList []error
	for e := range errors {
		errorList = append(errorList, e)
	}
	assert.Len(t, errorList, 0, "No errors expected")
	assert.Len(t, resultList, len(tasks), "All tasks should complete")
}

// TestWorkflow_ConditionalBranching 测试条件分支工作流
func TestWorkflow_ConditionalBranching(t *testing.T) {
	env := NewTestEnv(t)
	ctx := env.Context()
	testCases := []struct {
		condition string
		branch    string
		expected  string
	}{
		{"value > 10", "high", "Processing high value"},
		{"value <= 10", "low", "Processing low value"},
		{"value == 0", "zero", "Processing zero value"},
	}
	for _, tc := range testCases {
		t.Run(tc.branch, func(t *testing.T) {
			env.Provider.WithResponse(tc.expected)
			req := &mocks.GenerateRequest{
				Messages: []types.Message{
					fixtures.SystemMessage("You are processing a conditional workflow"),
					fixtures.UserMessage("Condition: " + tc.condition + ", Branch: " + tc.branch),
				},
			}
			resp, err := env.Provider.Generate(ctx, req)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, resp.Content)
		})
	}
}

// TestWorkflow_WithToolChain 测试带工具链的工作流
func TestWorkflow_WithToolChain(t *testing.T) {
	env := NewTestEnv(t)
	ctx := env.Context()
	env.Tools.WithToolResult("fetch_data", map[string]any{"data": "raw_data"})
	env.Tools.WithToolResult("process_data", map[string]any{"processed": "cleaned_data"})
	env.Tools.WithToolResult("save_data", map[string]any{"saved": true})

	toolChain := []string{"fetch_data", "process_data", "save_data"}
	var chainResults []any
	for _, toolName := range toolChain {
		result, err := env.Tools.Execute(ctx, toolName, map[string]any{})
		require.NoError(t, err, "Tool %s failed", toolName)
		chainResults = append(chainResults, result)
	}
	assert.Len(t, chainResults, 3)
	assert.Equal(t, 3, env.Tools.GetCallCount())
	calls := env.Tools.GetCalls()
	assert.Equal(t, "fetch_data", calls[0].Name)
	assert.Equal(t, "process_data", calls[1].Name)
	assert.Equal(t, "save_data", calls[2].Name)
}
// TestWorkflow_ErrorHandling 测试工作流错误处理
func TestWorkflow_ErrorHandling(t *testing.T) {
	env := NewTestEnv(t)
	ctx := env.Context()
	env.Tools.WithToolResult("step1", "success")
	env.Tools.WithToolError("step2", assert.AnError)
	env.Tools.WithToolResult("step3", "success")

	steps := []string{"step1", "step2", "step3"}
	var lastSuccessStep string
	var failedStep string
	for _, step := range steps {
		result, err := env.Tools.Execute(ctx, step, map[string]any{})
		if err != nil {
			failedStep = step
			break
		}
		lastSuccessStep = step
		_ = result
	}
	assert.Equal(t, "step1", lastSuccessStep)
	assert.Equal(t, "step2", failedStep)
}

// TestWorkflow_RetryMechanism 测试重试机制
func TestWorkflow_RetryMechanism(t *testing.T) {
	env := NewTestEnv(t)
	ctx := env.Context()
	callCount := 0
	env.Provider.WithGenerateFunc(func(ctx context.Context, req *mocks.GenerateRequest) (*mocks.GenerateResponse, error) {
		callCount++
		if callCount < 3 {
			return nil, assert.AnError
		}
		return &mocks.GenerateResponse{Content: "Success after retry"}, nil
	})

	maxRetries := 5
	var resp *mocks.GenerateResponse
	var err error
	for i := 0; i < maxRetries; i++ {
		req := &mocks.GenerateRequest{
			Messages: []types.Message{fixtures.UserMessage("Test")},
		}
		resp, err = env.Provider.Generate(ctx, req)
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	require.NoError(t, err)
	assert.Equal(t, "Success after retry", resp.Content)
	assert.Equal(t, 3, callCount)
}
// TestWorkflow_Timeout 测试工作流超时
func TestWorkflow_Timeout(t *testing.T) {
	env := NewTestEnv(t)
	ctx, cancel := context.WithTimeout(env.Context(), 100*time.Millisecond)
	defer cancel()

	env.Provider.WithGenerateFunc(func(ctx context.Context, req *mocks.GenerateRequest) (*mocks.GenerateResponse, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(500 * time.Millisecond):
			return &mocks.GenerateResponse{Content: "Slow response"}, nil
		}
	})

	req := &mocks.GenerateRequest{
		Messages: []types.Message{fixtures.UserMessage("Test")},
	}
	_, err := env.Provider.Generate(ctx, req)
	assert.Error(t, err)
	assert.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
}

// TestWorkflow_StateManagement 测试工作流状态管理
func TestWorkflow_StateManagement(t *testing.T) {
	env := NewTestEnv(t)
	ctx := env.Context()

	type WorkflowState struct {
		CurrentStep int
		Data        map[string]any
		Completed   bool
	}
	state := &WorkflowState{
		CurrentStep: 0,
		Data:        make(map[string]any),
		Completed:   false,
	}
	steps := []func(*WorkflowState) error{
		func(s *WorkflowState) error { s.Data["step1"] = "initialized"; s.CurrentStep = 1; return nil },
		func(s *WorkflowState) error { s.Data["step2"] = "processed"; s.CurrentStep = 2; return nil },
		func(s *WorkflowState) error { s.Data["step3"] = "finalized"; s.CurrentStep = 3; s.Completed = true; return nil },
	}
	for _, step := range steps {
		err := step(state)
		require.NoError(t, err)
	}
	assert.True(t, state.Completed)
	assert.Equal(t, 3, state.CurrentStep)
	assert.Equal(t, "initialized", state.Data["step1"])
	assert.Equal(t, "processed", state.Data["step2"])
	assert.Equal(t, "finalized", state.Data["step3"])
	_ = ctx
}
// --- 多 Agent 协作测试 ---

// TestWorkflow_MultiAgentCollaboration 测试多 Agent 协作
func TestWorkflow_MultiAgentCollaboration(t *testing.T) {
	env := NewTestEnv(t)
	ctx := env.Context()
	agents := map[string]*mocks.MockProvider{
		"researcher": mocks.NewMockProvider().WithResponse("Research findings: ..."),
		"analyst":    mocks.NewMockProvider().WithResponse("Analysis results: ..."),
		"writer":     mocks.NewMockProvider().WithResponse("Final report: ..."),
	}
	results := make(map[string]string)

	req := &mocks.GenerateRequest{Messages: []types.Message{fixtures.UserMessage("Research topic X")}}
	resp, err := agents["researcher"].Generate(ctx, req)
	require.NoError(t, err)
	results["research"] = resp.Content

	req = &mocks.GenerateRequest{Messages: []types.Message{
		fixtures.SystemMessage("You are an analyst"),
		fixtures.UserMessage("Analyze: " + results["research"]),
	}}
	resp, err = agents["analyst"].Generate(ctx, req)
	require.NoError(t, err)
	results["analysis"] = resp.Content

	req = &mocks.GenerateRequest{Messages: []types.Message{
		fixtures.SystemMessage("You are a technical writer"),
		fixtures.UserMessage("Write report based on: " + results["analysis"]),
	}}
	resp, err = agents["writer"].Generate(ctx, req)
	require.NoError(t, err)
	results["report"] = resp.Content

	assert.Len(t, results, 3)
	assert.Contains(t, results["research"], "Research")
	assert.Contains(t, results["analysis"], "Analysis")
	assert.Contains(t, results["report"], "report")
	for name, agent := range agents {
		assert.Equal(t, 1, agent.GetCallCount(), "Agent %s should be called once", name)
	}
	_ = env
}

// TestWorkflow_AgentHandoff 测试 Agent 交接
func TestWorkflow_AgentHandoff(t *testing.T) {
	env := NewTestEnv(t)
	ctx := env.Context()
	type HandoffMessage struct {
		FromAgent, ToAgent, Context, Task string
	}
	handoffs := []HandoffMessage{
		{"coordinator", "specialist", "User needs help with X", "Handle specialized task"},
		{"specialist", "coordinator", "Task completed", "Report results"},
	}
	for _, h := range handoffs {
		env.Provider.WithResponse("Handoff acknowledged from " + h.FromAgent + " to " + h.ToAgent)
		req := &mocks.GenerateRequest{Messages: []types.Message{
			fixtures.SystemMessage("Agent handoff in progress"),
			fixtures.UserMessage("Context: " + h.Context + ", Task: " + h.Task),
		}}
		resp, err := env.Provider.Generate(ctx, req)
		require.NoError(t, err)
		assert.Contains(t, resp.Content, "Handoff acknowledged")
	}
	assert.Equal(t, len(handoffs), env.Provider.GetCallCount())
}

// --- 工作流指标测试 ---

// TestWorkflow_MetricsCollection 测试工作流指标收集
func TestWorkflow_MetricsCollection(t *testing.T) {
	SkipIfShort(t)
	env := NewTestEnv(t)
	env.Provider.WithResponse("Metrics test response")
	ctx := env.Context()
	metrics := NewTestMetrics()
	workflowSteps := 10
	metrics.Start()
	for i := 0; i < workflowSteps; i++ {
		stepStart := time.Now()
		req := &mocks.GenerateRequest{
			Messages: []types.Message{fixtures.UserMessage("Step " + string(rune('0'+i)))},
		}
		_, err := env.Provider.Generate(ctx, req)
		stepDuration := time.Since(stepStart)
		metrics.Set("step_"+string(rune('0'+i))+"_duration_ms", stepDuration.Milliseconds())
		metrics.RecordIteration(err == nil)
	}
	metrics.Stop()
	metrics.Set("total_steps", workflowSteps)
	metrics.Set("throughput_steps_per_sec", float64(workflowSteps)/metrics.Duration.Seconds())
	metrics.Report(t)
	assert.Equal(t, 1.0, metrics.SuccessRate)
	assert.Equal(t, workflowSteps, env.Provider.GetCallCount())
}
