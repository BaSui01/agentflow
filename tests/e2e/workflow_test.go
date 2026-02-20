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

	// 定义工作流步骤
	steps := []struct {
		name     string
		input    string
		expected string
	}{
		{"step1", "Start workflow", "Step 1 completed"},
		{"step2", "Continue workflow", "Step 2 completed"},
		{"step3", "Finish workflow", "Step 3 completed"},
	}

	// 执行工作流
	results := make([]string, 0, len(steps))
	for _, step := range steps {
		// 配置 provider 响应
		env.Provider.WithResponse(step.expected)

		// 执行步骤
		req := &types.GenerateRequest{
			Messages: []types.Message{fixtures.UserMessage(step.input)},
		}
		resp, err := env.Provider.Generate(ctx, req)
		require.NoError(t, err, "Step %s failed", step.name)

		results = append(results, resp.Content)
	}

	// 验证所有步骤完成
	assert.Len(t, results, 3)
	for i, step := range steps {
		assert.Equal(t, step.expected, results[i])
	}
}

// TestWorkflow_ParallelExecution 测试并行工作流执行
func TestWorkflow_ParallelExecution(t *testing.T) {
	env := NewTestEnv(t)

	ctx := env.Context()

	// 定义并行任务
	tasks := []string{"Task A", "Task B", "Task C", "Task D"}
	env.Provider.WithResponse("Task completed")

	// 并行执行
	var wg sync.WaitGroup
	results := make(chan string, len(tasks))
	errors := make(chan error, len(tasks))

	for _, task := range tasks {
		wg.Add(1)
		go func(taskName string) {
			defer wg.Done()

			req := &types.GenerateRequest{
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

	// 等待完成
	wg.Wait()
	close(results)
	close(errors)

	// 验证结果
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

	// 模拟条件分支
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

			req := &types.GenerateRequest{
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

	// 注册工具链
	env.Tools.WithToolResult("fetch_data", map[string]any{"data": "raw_data"})
	env.Tools.WithToolResult("process_data", map[string]any{"processed": "cleaned_data"})
	env.Tools.WithToolResult("save_data", map[string]any{"saved": true})

	// 执行工具链
	toolChain := []string{"fetch_data", "process_data", "save_data"}
	var chainResults []any

	for _, toolName := range toolChain {
		result, err := env.Tools.Execute(ctx, toolName, map[string]any{})
		require.NoError(t, err, "Tool %s failed", toolName)
		chainResults = append(chainResults, result)
	}

	// 验证工具链执行
	assert.Len(t, chainResults, 3)
	assert.Equal(t, 3, env.Tools.GetCallCount())

	// 验证调用顺序
	calls := env.Tools.GetCalls()
	assert.Equal(t, "fetch_data", calls[0].Name)
	assert.Equal(t, "process_data", calls[1].Name)
	assert.Equal(t, "save_data", calls[2].Name)
}

// TestWorkflow_ErrorHandling 测试工作流错误处理
func TestWorkflow_ErrorHandling(t *testing.T) {
	env := NewTestEnv(t)

	ctx := env.Context()

	// 配置第二个工具失败
	env.Tools.WithToolResult("step1", "success")
	env.Tools.WithToolError("step2", assert.AnError)
	env.Tools.WithToolResult("step3", "success")

	// 执行工作流，期望在 step2 失败
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

	// 验证错误处理
	assert.Equal(t, "step1", lastSuccessStep)
	assert.Equal(t, "step2", failedStep)
}

// TestWorkflow_RetryMechanism 测试重试机制
func TestWorkflow_RetryMechanism(t *testing.T) {
	env := NewTestEnv(t)

	ctx := env.Context()

	// 配置 provider 在前两次失败，第三次成功
	callCount := 0
	env.Provider.WithGenerateFunc(func(ctx context.Context, req *types.GenerateRequest) (*types.GenerateResponse, error) {
		callCount++
		if callCount < 3 {
			return nil, assert.AnError
		}
		return fixtures.SimpleResponse("Success after retry"), nil
	})

	// 实现简单的重试逻辑
	maxRetries := 5
	var resp *types.GenerateResponse
	var err error

	for i := 0; i < maxRetries; i++ {
		req := &types.GenerateRequest{
			Messages: []types.Message{fixtures.UserMessage("Test")},
		}
		resp, err = env.Provider.Generate(ctx, req)
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond) // 短暂延迟
	}

	// 验证重试成功
	require.NoError(t, err)
	assert.Equal(t, "Success after retry", resp.Content)
	assert.Equal(t, 3, callCount)
}

// TestWorkflow_Timeout 测试工作流超时
func TestWorkflow_Timeout(t *testing.T) {
	env := NewTestEnv(t)

	// 创建短超时上下文
	ctx, cancel := context.WithTimeout(env.Context(), 100*time.Millisecond)
	defer cancel()

	// 配置慢响应
	env.Provider.WithGenerateFunc(func(ctx context.Context, req *types.GenerateRequest) (*types.GenerateResponse, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(500 * time.Millisecond):
			return fixtures.SimpleResponse("Slow response"), nil
		}
	})

	// 执行请求
	req := &types.GenerateRequest{
		Messages: []types.Message{fixtures.UserMessage("Test")},
	}
	_, err := env.Provider.Generate(ctx, req)

	// 验证超时
	assert.Error(t, err)
	assert.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
}

// TestWorkflow_StateManagement 测试工作流状态管理
func TestWorkflow_StateManagement(t *testing.T) {
	env := NewTestEnv(t)

	ctx := env.Context()

	// 模拟工作流状态
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

	// 定义工作流步骤
	steps := []func(*WorkflowState) error{
		func(s *WorkflowState) error {
			s.Data["step1"] = "initialized"
			s.CurrentStep = 1
			return nil
		},
		func(s *WorkflowState) error {
			s.Data["step2"] = "processed"
			s.CurrentStep = 2
			return nil
		},
		func(s *WorkflowState) error {
			s.Data["step3"] = "finalized"
			s.CurrentStep = 3
			s.Completed = true
			return nil
		},
	}

	// 执行工作流
	for _, step := range steps {
		err := step(state)
		require.NoError(t, err)
	}

	// 验证最终状态
	assert.True(t, state.Completed)
	assert.Equal(t, 3, state.CurrentStep)
	assert.Equal(t, "initialized", state.Data["step1"])
	assert.Equal(t, "processed", state.Data["step2"])
	assert.Equal(t, "finalized", state.Data["step3"])

	_ = ctx // 使用 ctx 避免未使用警告
}

// --- 多 Agent 协作测试 ---

// TestWorkflow_MultiAgentCollaboration 测试多 Agent 协作
func TestWorkflow_MultiAgentCollaboration(t *testing.T) {
	env := NewTestEnv(t)

	ctx := env.Context()

	// 创建多个 mock provider 模拟不同的 Agent
	agents := map[string]*mocks.MockProvider{
		"researcher":  mocks.NewMockProvider().WithResponse("Research findings: ..."),
		"analyst":     mocks.NewMockProvider().WithResponse("Analysis results: ..."),
		"writer":      mocks.NewMockProvider().WithResponse("Final report: ..."),
	}

	// 模拟协作流程
	results := make(map[string]string)

	// 1. Researcher 收集信息
	req := &types.GenerateRequest{
		Messages: []types.Message{fixtures.UserMessage("Research topic X")},
	}
	resp, err := agents["researcher"].Generate(ctx, req)
	require.NoError(t, err)
	results["research"] = resp.Content

	// 2. Analyst 分析数据
	req = &types.GenerateRequest{
		Messages: []types.Message{
			fixtures.SystemMessage("You are an analyst"),
			fixtures.UserMessage("Analyze: " + results["research"]),
		},
	}
	resp, err = agents["analyst"].Generate(ctx, req)
	require.NoError(t, err)
	results["analysis"] = resp.Content

	// 3. Writer 生成报告
	req = &types.GenerateRequest{
		Messages: []types.Message{
			fixtures.SystemMessage("You are a technical writer"),
			fixtures.UserMessage("Write report based on: " + results["analysis"]),
		},
	}
	resp, err = agents["writer"].Generate(ctx, req)
	require.NoError(t, err)
	results["report"] = resp.Content

	// 验证协作结果
	assert.Len(t, results, 3)
	assert.Contains(t, results["research"], "Research")
	assert.Contains(t, results["analysis"], "Analysis")
	assert.Contains(t, results["report"], "report")

	// 验证每个 Agent 都被调用了一次
	for name, agent := range agents {
		assert.Equal(t, 1, agent.GetCallCount(), "Agent %s should be called once", name)
	}
}

// TestWorkflow_AgentHandoff 测试 Agent 交接
func TestWorkflow_AgentHandoff(t *testing.T) {
	env := NewTestEnv(t)

	ctx := env.Context()

	// 模拟 Agent 交接场景
	type HandoffMessage struct {
		FromAgent string
		ToAgent   string
		Context   string
		Task      string
	}

	handoffs := []HandoffMessage{
		{FromAgent: "coordinator", ToAgent: "specialist", Context: "User needs help with X", Task: "Handle specialized task"},
		{FromAgent: "specialist", ToAgent: "coordinator", Context: "Task completed", Task: "Report results"},
	}

	// 执行交接流程
	for _, handoff := range handoffs {
		env.Provider.WithResponse("Handoff acknowledged from " + handoff.FromAgent + " to " + handoff.ToAgent)

		req := &types.GenerateRequest{
			Messages: []types.Message{
				fixtures.SystemMessage("Agent handoff in progress"),
				fixtures.UserMessage("Context: " + handoff.Context + ", Task: " + handoff.Task),
			},
		}

		resp, err := env.Provider.Generate(ctx, req)
		require.NoError(t, err)
		assert.Contains(t, resp.Content, "Handoff acknowledged")
	}

	// 验证交接次数
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

	// 执行工作流并收集指标
	workflowSteps := 10
	metrics.Start()

	for i := 0; i < workflowSteps; i++ {
		stepStart := time.Now()

		req := &types.GenerateRequest{
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

	// 验证指标
	assert.Equal(t, 1.0, metrics.SuccessRate)
	assert.Equal(t, workflowSteps, env.Provider.GetCallCount())
}
