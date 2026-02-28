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
	env.Provider.WithResponse("Hello! I'm here to help you.")
	ctx := env.Context()

	userMsg := fixtures.UserMessage("Hello, agent!")
	err := env.Memory.Add(ctx, userMsg)
	require.NoError(t, err)

	req := &mocks.GenerateRequest{
		Messages: []types.Message{userMsg},
		Model:    env.Config.Agent.Model,
	}
	resp, err := env.Provider.Generate(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, "Hello! I'm here to help you.", resp.Content)

	assistantMsg := fixtures.AssistantMessage(resp.Content)
	err = env.Memory.Add(ctx, assistantMsg)
	require.NoError(t, err)

	messages, err := env.Memory.GetAll(ctx)
	require.NoError(t, err)
	assert.Len(t, messages, 2)
	assert.Equal(t, "user", messages[0].Role)
	assert.Equal(t, "assistant", messages[1].Role)
	assert.Equal(t, 1, env.Provider.GetCallCount())
}
// TestAgentLifecycle_MemoryLimit 测试记忆限制
func TestAgentLifecycle_MemoryLimit(t *testing.T) {
	env := NewTestEnv(t)
	env.Memory.WithMaxMessages(5)
	ctx := env.Context()

	for i := 0; i < 10; i++ {
		msg := fixtures.UserMessage("Message " + string(rune('0'+i)))
		err := env.Memory.Add(ctx, msg)
		require.NoError(t, err)
	}

	messages, err := env.Memory.GetAll(ctx)
	require.NoError(t, err)
	assert.Len(t, messages, 5)
	assert.Contains(t, messages[0].Content, "5")
}

// TestAgentLifecycle_ConcurrentExecution 测试并发执行
func TestAgentLifecycle_ConcurrentExecution(t *testing.T) {
	env := NewTestEnv(t)
	env.Provider.WithResponse("Concurrent response")
	ctx := env.Context()
	concurrency := 10
	done := make(chan bool, concurrency)

	for i := 0; i < concurrency; i++ {
		go func(id int) {
			req := &mocks.GenerateRequest{
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

	successCount := 0
	for i := 0; i < concurrency; i++ {
		if <-done {
			successCount++
		}
	}
	assert.Equal(t, concurrency, successCount)
	assert.Equal(t, concurrency, env.Provider.GetCallCount())
}

// --- 检查点和恢复测试 ---

// TestAgentLifecycle_CheckpointAndRestore 测试检查点保存和恢复
func TestAgentLifecycle_CheckpointAndRestore(t *testing.T) {
	env := NewTestEnv(t)
	ctx := env.Context()

	msgs := fixtures.SimpleConversation()
	for _, msg := range msgs {
		err := env.Memory.Add(ctx, msg)
		require.NoError(t, err)
	}

	checkpoint, err := env.Memory.GetAll(ctx)
	require.NoError(t, err)
	assert.Len(t, checkpoint, 4)

	err = env.Memory.Clear(ctx)
	require.NoError(t, err)

	cleared, err := env.Memory.GetAll(ctx)
	require.NoError(t, err)
	assert.Len(t, cleared, 0)

	for _, msg := range checkpoint {
		err := env.Memory.Add(ctx, msg)
		require.NoError(t, err)
	}

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
		req := &mocks.GenerateRequest{
			Messages: []types.Message{fixtures.UserMessage("Test")},
		}
		_, err := env.Provider.Generate(ctx, req)
		metrics.RecordIteration(err == nil)
	}
	metrics.Stop()

	metrics.Set("iterations", iterations)
	metrics.Set("avg_latency_ms", float64(metrics.Duration.Milliseconds())/float64(iterations))
	metrics.Report(t)

	assert.Equal(t, 1.0, metrics.SuccessRate, "All iterations should succeed")
	assert.Less(t, metrics.Duration, 5*time.Second, "Should complete within 5 seconds")
}

// TestAgentLifecycle_StreamingResponse 测试流式响应
func TestAgentLifecycle_StreamingResponse(t *testing.T) {
	env := NewTestEnv(t)
	chunks := []string{"Hello", ", ", "how ", "can ", "I ", "help ", "you", "?"}
	env.Provider.WithStreamChunks(chunks)
	ctx := env.Context()

	req := &mocks.GenerateRequest{
		Messages: []types.Message{fixtures.UserMessage("Hi!")},
		Stream:   true,
	}
	ch, err := env.Provider.StreamGenerate(ctx, req)
	require.NoError(t, err)

	var content string
	var chunkCount int
	for chunk := range ch {
		content += chunk.Content
		chunkCount++
	}

	assert.Equal(t, "Hello, how can I help you?", content)
	assert.Equal(t, len(chunks), chunkCount)
}

// TestAgentLifecycle_ErrorRecovery 测试错误恢复
func TestAgentLifecycle_ErrorRecovery(t *testing.T) {
	env := NewTestEnv(t)
	env.Provider.WithResponse("Success").WithFailAfter(2)
	ctx := env.Context()

	for i := 0; i < 2; i++ {
		req := &mocks.GenerateRequest{
			Messages: []types.Message{fixtures.UserMessage("Test")},
		}
		resp, err := env.Provider.Generate(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, "Success", resp.Content)
	}

	req := &mocks.GenerateRequest{
		Messages: []types.Message{fixtures.UserMessage("Test")},
	}
	_, err := env.Provider.Generate(ctx, req)
	assert.Error(t, err)
	assert.Equal(t, 3, env.Provider.GetCallCount())
}

// TestAgentLifecycle_ContextCancellation 测试上下文取消
func TestAgentLifecycle_ContextCancellation(t *testing.T) {
	env := NewTestEnv(t)
	ctx, cancel := context.WithCancel(env.Context())
	env.Provider.WithStreamChunks([]string{"chunk1", "chunk2", "chunk3", "chunk4", "chunk5"})

	req := &mocks.GenerateRequest{
		Messages: []types.Message{fixtures.UserMessage("Test")},
		Stream:   true,
	}
	ch, err := env.Provider.StreamGenerate(ctx, req)
	require.NoError(t, err)

	<-ch
	cancel()
	time.Sleep(50 * time.Millisecond)
	assert.Error(t, ctx.Err())
}

// TestAgentLifecycle_WithToolCalls 测试带工具调用的 Agent 执行
func TestAgentLifecycle_WithToolCalls(t *testing.T) {
	env := NewTestEnv(t)
	env.Tools.WithTool("calculator", func(ctx context.Context, args map[string]any) (any, error) {
		a, _ := args["a"].(float64)
		b, _ := args["b"].(float64)
		return a + b, nil
	})

	toolCall := fixtures.CalculatorToolCall("call_001", 2, 3, "add")
	env.Provider.WithToolCalls([]types.ToolCall{toolCall})
	ctx := env.Context()

	userMsg := fixtures.UserMessage("What is 2 + 3?")
	err := env.Memory.Add(ctx, userMsg)
	require.NoError(t, err)

	req := &mocks.GenerateRequest{
		Messages: []types.Message{userMsg},
		Tools:    []types.ToolSchema{fixtures.CalculatorToolSchema()},
	}
	resp, err := env.Provider.Generate(ctx, req)
	require.NoError(t, err)
	require.Len(t, resp.ToolCalls, 1)

	tc := resp.ToolCalls[0]
	result, err := env.Tools.ExecuteToolCall(ctx, tc)
	require.NoError(t, err)
	assert.Equal(t, float64(5), result)

	toolCalls := env.Tools.GetCalls()
	assert.Len(t, toolCalls, 1)
	assert.Equal(t, "calculator", toolCalls[0].Name)
}

// TestAgentLifecycle_MultiTurnConversation 测试多轮对话
func TestAgentLifecycle_MultiTurnConversation(t *testing.T) {
	env := NewTestEnv(t)
	ctx := env.Context()
	turns := 5

	for i := 0; i < turns; i++ {
		env.Provider.WithResponse("Response " + string(rune('1'+i)))
		userMsg := fixtures.UserMessage("Message " + string(rune('1'+i)))
		err := env.Memory.Add(ctx, userMsg)
		require.NoError(t, err)

		history, err := env.Memory.GetAll(ctx)
		require.NoError(t, err)

		req := &mocks.GenerateRequest{Messages: history}
		resp, err := env.Provider.Generate(ctx, req)
		require.NoError(t, err)

		err = env.Memory.Add(ctx, fixtures.AssistantMessage(resp.Content))
		require.NoError(t, err)
	}

	messages, err := env.Memory.GetAll(ctx)
	require.NoError(t, err)
	assert.Len(t, messages, turns*2)
	assert.Equal(t, turns, env.Provider.GetCallCount())
}
