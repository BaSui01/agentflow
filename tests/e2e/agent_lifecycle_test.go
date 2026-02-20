// Agent 生命周期端到端测试。
//
// 覆盖创建、执行、检查点与恢复流程。
//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/BaSui01/agentflow/testutil/fixtures"
	"github.com/BaSui01/agentflow/testutil/mocks"
	"github.com/BaSui01/agentflow/types"
)

// --- Agent 生命周期测试 ---

// TestAgentLifecycle_BasicExecution 测试基本的 Agent 执行流程
func TestAgentLifecycle_BasicExecution(t *testing.T) {
	env := NewTestEnv(t)

	// 配置 mock provider 返回简单响应
	env.Provider.WithResponse("Hello! I'm here to help you.")

	// 模拟 Agent 执行
	ctx := env.Context()

	// 1. 添加用户消息到记忆
	userMsg := fixtures.UserMessage("Hello, agent!")
	err := env.Memory.Add(ctx, userMsg)
	require.NoError(t, err)

	// 2. 调用 provider 生成响应
	req := &types.GenerateRequest{
		Messages: []types.Message{userMsg},
		Model:    env.Config.Agent.Model,
	}
	resp, err := env.Provider.Generate(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, "Hello! I'm here to help you.", resp.Content)

	// 3. 添加助手响应到记忆
	assistantMsg := fixtures.AssistantMessage(resp.Content)
	err = env.Memory.Add(ctx, assistantMsg)
	require.NoError(t, err)

	// 4. 验证记忆状态
	messages, err := env.Memory.GetAll(ctx)
	require.NoError(t, err)
	assert.Len(t, messages, 2)
	assert.Equal(t, "user", messages[0].Role)
	assert.Equal(t, "assistant", messages[1].Role)

	// 5. 验证 provider 调用次数
	assert.Equal(t, 1, env.Provider.GetCallCount())
}

// TestAgentLifecycle_WithToolCalls 测试带工具调用的 Agent 执行
func TestAgentLifecycle_WithToolCalls(t *testing.T) {
	env := NewTestEnv(t)

	// 注册计算器工具
	env.Tools.WithTool("calculator", func(ctx context.Context, args map[string]any) (any, error) {
		a, _ := args["a"].(float64)
		b, _ := args["b"].(float64)
		return a + b, nil
	})

	// 配置 provider 返回工具调用
	toolCall := fixtures.CalculatorToolCall("call_001", 2, 3, "add")
	env.Provider.WithToolCalls([]types.ToolCall{toolCall})

	ctx := env.Context()

	// 1. 用户请求计算
	userMsg := fixtures.UserMessage("What is 2 + 3?")
	err := env.Memory.Add(ctx, userMsg)
	require.NoError(t, err)

	// 2. 获取 LLM 响应（包含工具调用）
	req := &types.GenerateRequest{
		Messages: []types.Message{userMsg},
		Tools:    []types.Tool{fixtures.CalculatorTool()},
	}
	resp, err := env.Provider.Generate(ctx, req)
	require.NoError(t, err)
	require.Len(t, resp.ToolCalls, 1)

	// 3. 执行工具调用
	tc := resp.ToolCalls[0]
	result, err := env.Tools.Execute(ctx, tc.Name, tc.Arguments)
	require.NoError(t, err)
	assert.Equal(t, float64(5), result)

	// 4. 验证工具调用记录
	toolCalls := env.Tools.GetCalls()
	assert.Len(t, toolCalls, 1)
	assert.Equal(t, "calculator", toolCalls[0].Name)
}

// TestAgentLifecycle_MultiTurnConversation 测试多轮对话
func TestAgentLifecycle_MultiTurnConversation(t *testing.T) {
	env := NewTestEnv(t)

	ctx := env.Context()
	turns := 5

	// 模拟多轮对话
	for i := 0; i < turns; i++ {
		// 配置不同的响应
		env.Provider.WithResponse("Response " + string(rune('1'+i)))

		// 用户消息
		userMsg := fixtures.UserMessage("Message " + string(rune('1'+i)))
		err := env.Memory.Add(ctx, userMsg)
		require.NoError(t, err)

		// 获取所有历史消息
		history, err := env.Memory.GetAll(ctx)
		require.NoError(t, err)

		// 生成响应
		req := &types.GenerateRequest{
			Messages: history,
		}
		resp, err := env.Provider.Generate(ctx, req)
		require.NoError(t, err)

		// 添加助手响应
		assistantMsg := fixtures.AssistantMessage(resp.Content)
		err = env.Memory.Add(ctx, assistantMsg)
		require.NoError(t, err)
	}

	// 验证最终状态
	messages, err := env.Memory.GetAll(ctx)
	require.NoError(t, err)
	assert.Len(t, messages, turns*2) // 每轮 2 条消息

	// 验证 provider 调用次数
	assert.Equal(t, turns, env.Provider.GetCallCount())
}

// TestAgentLifecycle_StreamingResponse 测试流式响应
func TestAgentLifecycle_StreamingResponse(t *testing.T) {
	env := NewTestEnv(t)

	// 配置流式响应
	chunks := []string{"Hello", ", ", "how ", "can ", "I ", "help ", "you", "?"}
	env.Provider.WithStreamChunks(chunks)

	ctx := env.Context()

	// 发起流式请求
	req := &types.GenerateRequest{
		Messages: []types.Message{fixtures.UserMessage("Hi!")},
		Stream:   true,
	}

	ch, err := env.Provider.Stream(ctx, req)
	require.NoError(t, err)

	// 收集流式响应
	var content string
	var chunkCount int
	for chunk := range ch {
		content += chunk.Content
		chunkCount++
	}

	// 验证完整内容
	assert.Equal(t, "Hello, how can I help you?", content)
	assert.Equal(t, len(chunks), chunkCount)
}

// TestAgentLifecycle_ErrorRecovery 测试错误恢复
func TestAgentLifecycle_ErrorRecovery(t *testing.T) {
	env := NewTestEnv(t)

	// 配置 provider 在第 2 次调用后失败
	env.Provider.WithResponse("Success").WithFailAfter(2)

	ctx := env.Context()

	// 前两次调用应该成功
	for i := 0; i < 2; i++ {
		req := &types.GenerateRequest{
			Messages: []types.Message{fixtures.UserMessage("Test")},
		}
		resp, err := env.Provider.Generate(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, "Success", resp.Content)
	}

	// 第三次调用应该失败
	req := &types.GenerateRequest{
		Messages: []types.Message{fixtures.UserMessage("Test")},
	}
	_, err := env.Provider.Generate(ctx, req)
	assert.Error(t, err)

	// 验证调用次数
	assert.Equal(t, 3, env.Provider.GetCallCount())
}

// TestAgentLifecycle_ContextCancellation 测试上下文取消
func TestAgentLifecycle_ContextCancellation(t *testing.T) {
	env := NewTestEnv(t)

	// 创建可取消的上下文
	ctx, cancel := context.WithCancel(env.Context())

	// 配置流式响应
	env.Provider.WithStreamChunks([]string{"chunk1", "chunk2", "chunk3", "chunk4", "chunk5"})

	// 发起流式请求
	req := &types.GenerateRequest{
		Messages: []types.Message{fixtures.UserMessage("Test")},
		Stream:   true,
	}

	ch, err := env.Provider.Stream(ctx, req)
	require.NoError(t, err)

	// 读取一个块后取消
	<-ch
	cancel()

	// 等待一小段时间让取消生效
	time.Sleep(50 * time.Millisecond)

	// 验证上下文已取消
	assert.Error(t, ctx.Err())
}

// TestAgentLifecycle_MemoryLimit 测试记忆限制
func TestAgentLifecycle_MemoryLimit(t *testing.T) {
	env := NewTestEnv(t)

	// 设置记忆限制为 5 条消息
	env.Memory.WithMaxMessages(5)

	ctx := env.Context()

	// 添加 10 条消息
	for i := 0; i < 10; i++ {
		msg := fixtures.UserMessage("Message " + string(rune('0'+i)))
		err := env.Memory.Add(ctx, msg)
		require.NoError(t, err)
	}

	// 验证只保留最近 5 条
	messages, err := env.Memory.GetAll(ctx)
	require.NoError(t, err)
	assert.Len(t, messages, 5)

	// 验证是最近的 5 条
	assert.Contains(t, messages[0].Content, "5")
}

// TestAgentLifecycle_ConcurrentExecution 测试并发执行
func TestAgentLifecycle_ConcurrentExecution(t *testing.T) {
	env := NewTestEnv(t)

	env.Provider.WithResponse("Concurrent response")

	ctx := env.Context()
	concurrency := 10
	done := make(chan bool, concurrency)

	// 并发执行
	for i := 0; i < concurrency; i++ {
		go func(id int) {
			req := &types.GenerateRequest{
				Messages: []types.Message{fixtures.UserMessage("Concurrent test " + string(rune('0'+id)))},
			}
			resp, err := env.Provider.Generate(ctx, req)
			if err == nil && resp.Content == "Concurrent response" {
				done <- true
			} else {
				done <- false
			}
		}(i)
	}

	// 等待所有完成
	successCount := 0
	for i := 0; i < concurrency; i++ {
		if <-done {
			successCount++
		}
	}

	// 验证所有请求都成功
	assert.Equal(t, concurrency, successCount)
	assert.Equal(t, concurrency, env.Provider.GetCallCount())
}

// --- 检查点和恢复测试 ---

// TestAgentLifecycle_CheckpointAndRestore 测试检查点保存和恢复
func TestAgentLifecycle_CheckpointAndRestore(t *testing.T) {
	env := NewTestEnv(t)

	ctx := env.Context()

	// 1. 建立初始对话状态
	messages := fixtures.SimpleConversation()
	for _, msg := range messages {
		err := env.Memory.Add(ctx, msg)
		require.NoError(t, err)
	}

	// 2. 保存检查点（模拟）
	checkpoint, err := env.Memory.GetAll(ctx)
	require.NoError(t, err)
	assert.Len(t, checkpoint, 4)

	// 3. 清空记忆
	err = env.Memory.Clear(ctx)
	require.NoError(t, err)

	// 4. 验证记忆已清空
	cleared, err := env.Memory.GetAll(ctx)
	require.NoError(t, err)
	assert.Len(t, cleared, 0)

	// 5. 从检查点恢复
	for _, msg := range checkpoint {
		err := env.Memory.Add(ctx, msg)
		require.NoError(t, err)
	}

	// 6. 验证恢复成功
	restored, err := env.Memory.GetAll(ctx)
	require.NoError(t, err)
	assert.Len(t, restored, 4)
	assert.Equal(t, checkpoint[0].Content, restored[0].Content)
}

// --- 性能测试 ---

// TestAgentLifecycle_PerformanceBaseline 测试性能基线
func TestAgentLifecycle_PerformanceBaseline(t *testing.T) {
	SkipIfShort(t)

	env := NewTestEnv(t)
	env.Provider.WithResponse("Performance test response")

	ctx := env.Context()
	iterations := 100
	metrics := NewTestMetrics()

	metrics.Start()
	for i := 0; i < iterations; i++ {
		req := &types.GenerateRequest{
			Messages: []types.Message{fixtures.UserMessage("Test")},
		}
		_, err := env.Provider.Generate(ctx, req)
		metrics.RecordIteration(err == nil)
	}
	metrics.Stop()

	metrics.Set("iterations", iterations)
	metrics.Set("avg_latency_ms", float64(metrics.Duration.Milliseconds())/float64(iterations))
	metrics.Report(t)

	// 验证性能指标
	assert.Equal(t, 1.0, metrics.SuccessRate, "All iterations should succeed")
	assert.Less(t, metrics.Duration, 5*time.Second, "Should complete within 5 seconds")
}
