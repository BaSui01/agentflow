package integration

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/tools"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

// testToolExecutor 是用于测试的函数回调工具执行器替身
type testToolExecutor struct {
	executeFn    func(ctx context.Context, calls []llm.ToolCall) []tools.ToolResult
	executeOneFn func(ctx context.Context, call llm.ToolCall) tools.ToolResult
}

func (e *testToolExecutor) Execute(ctx context.Context, calls []llm.ToolCall) []tools.ToolResult {
	if e.executeFn != nil {
		return e.executeFn(ctx, calls)
	}
	return nil
}

func (e *testToolExecutor) ExecuteOne(ctx context.Context, call llm.ToolCall) tools.ToolResult {
	if e.executeOneFn != nil {
		return e.executeOneFn(ctx, call)
	}
	return tools.ToolResult{}
}

// TestReActLoop_SingleToolCall 使用单个工具调用测试 ReAct 循环
func TestReActLoop_SingleToolCall(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	// 第一次法学硕士通话 - 决定使用工具
	firstResp := &llm.ChatResponse{
		ID:       "resp-1",
		Provider: "test-provider",
		Model:    "gpt-4",
		Choices: []llm.ChatChoice{
			{
				Index:        0,
				FinishReason: "tool_calls",
				Message: llm.Message{
					Role:    llm.RoleAssistant,
					Content: "I'll check the weather for you.",
					ToolCalls: []llm.ToolCall{
						{
							ID:        "call_1",
							Name:      "get_weather",
							Arguments: json.RawMessage(`{"city":"Tokyo"}`),
						},
					},
				},
			},
		},
		Usage: llm.ChatUsage{TotalTokens: 20},
	}

	// 第二次法学硕士通话 - 最终答案
	secondResp := &llm.ChatResponse{
		ID:       "resp-2",
		Provider: "test-provider",
		Model:    "gpt-4",
		Choices: []llm.ChatChoice{
			{
				Index:        0,
				FinishReason: "stop",
				Message: llm.Message{
					Role:    llm.RoleAssistant,
					Content: "The weather in Tokyo is sunny with a temperature of 25°C.",
				},
			},
		},
		Usage: llm.ChatUsage{TotalTokens: 30},
	}

	callCount := 0
	provider := &testProvider{
		name: "test-provider",
		completionFn: func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
			callCount++
			if callCount == 1 {
				return firstResp, nil
			}
			return secondResp, nil
		},
	}

	// 模拟工具执行
	toolResults := []tools.ToolResult{
		{
			ToolCallID: "call_1",
			Name:       "get_weather",
			Result:     json.RawMessage(`{"temperature":25,"condition":"sunny"}`),
		},
	}

	executor := &testToolExecutor{
		executeFn: func(ctx context.Context, calls []llm.ToolCall) []tools.ToolResult {
			return toolResults
		},
	}

	config := tools.ReActConfig{
		MaxIterations: 5,
		StopOnError:   true,
	}

	reactExecutor := tools.NewReActExecutor(provider, executor, config, logger)

	ctx := context.Background()
	req := &llm.ChatRequest{
		Model: "gpt-4",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "What's the weather in Tokyo?"},
		},
		Tools: []llm.ToolSchema{
			{
				Name:        "get_weather",
				Description: "Get weather information for a city",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"city": {"type": "string"}
					},
					"required": ["city"]
				}`),
			},
		},
	}

	// 执行ReAct循环
	resp, steps, err := reactExecutor.Execute(ctx, req)

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Len(t, steps, 2)
	assert.Equal(t, "The weather in Tokyo is sunny with a temperature of 25°C.", resp.Choices[0].Message.Content)
}

// TestReActLoop_MultipleToolCalls 使用多个工具调用测试 ReAct 循环
func TestReActLoop_MultipleToolCalls(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	// 第一次调用 - 添加
	resp1 := &llm.ChatResponse{
		ID:       "resp-1",
		Provider: "test-provider",
		Model:    "gpt-4",
		Choices: []llm.ChatChoice{
			{
				Index:        0,
				FinishReason: "tool_calls",
				Message: llm.Message{
					Role: llm.RoleAssistant,
					ToolCalls: []llm.ToolCall{
						{
							ID:        "call_1",
							Name:      "add",
							Arguments: json.RawMessage(`{"a":10,"b":20}`),
						},
					},
				},
			},
		},
		Usage: llm.ChatUsage{TotalTokens: 15},
	}

	// 第二次调用 - 乘法
	resp2 := &llm.ChatResponse{
		ID:       "resp-2",
		Provider: "test-provider",
		Model:    "gpt-4",
		Choices: []llm.ChatChoice{
			{
				Index:        0,
				FinishReason: "tool_calls",
				Message: llm.Message{
					Role: llm.RoleAssistant,
					ToolCalls: []llm.ToolCall{
						{
							ID:        "call_2",
							Name:      "multiply",
							Arguments: json.RawMessage(`{"a":30,"b":2}`),
						},
					},
				},
			},
		},
		Usage: llm.ChatUsage{TotalTokens: 20},
	}

	// 最后通话 - 应答
	resp3 := &llm.ChatResponse{
		ID:       "resp-3",
		Provider: "test-provider",
		Model:    "gpt-4",
		Choices: []llm.ChatChoice{
			{
				Index:        0,
				FinishReason: "stop",
				Message: llm.Message{
					Role:    llm.RoleAssistant,
					Content: "The result is 60.",
				},
			},
		},
		Usage: llm.ChatUsage{TotalTokens: 10},
	}

	completionCallCount := 0
	provider := &testProvider{
		name: "test-provider",
		completionFn: func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
			completionCallCount++
			switch completionCallCount {
			case 1:
				return resp1, nil
			case 2:
				return resp2, nil
			default:
				return resp3, nil
			}
		},
	}

	executeCallCount := 0
	executor := &testToolExecutor{
		executeFn: func(ctx context.Context, calls []llm.ToolCall) []tools.ToolResult {
			executeCallCount++
			if executeCallCount == 1 {
				return []tools.ToolResult{
					{
						ToolCallID: "call_1",
						Name:       "add",
						Result:     json.RawMessage(`30`),
					},
				}
			}
			return []tools.ToolResult{
				{
					ToolCallID: "call_2",
					Name:       "multiply",
					Result:     json.RawMessage(`60`),
				},
			}
		},
	}

	config := tools.ReActConfig{
		MaxIterations: 5,
		StopOnError:   false,
	}

	reactExecutor := tools.NewReActExecutor(provider, executor, config, logger)

	ctx := context.Background()
	req := &llm.ChatRequest{
		Model: "gpt-4",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "Calculate 10 + 20 and then multiply by 2"},
		},
		Tools: []llm.ToolSchema{
			{
				Name:        "add",
				Description: "Add two numbers",
			},
			{
				Name:        "multiply",
				Description: "Multiply two numbers",
			},
		},
	}

	// 执行ReAct循环
	resp, steps, err := reactExecutor.Execute(ctx, req)

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Len(t, steps, 3)
	assert.Equal(t, "The result is 60.", resp.Choices[0].Message.Content)
}

// TestReActLoop_ToolError 使用工具错误测试 ReAct 循环
func TestReActLoop_ToolError(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	// LLM调用工具
	resp1 := &llm.ChatResponse{
		ID:       "resp-1",
		Provider: "test-provider",
		Model:    "gpt-4",
		Choices: []llm.ChatChoice{
			{
				Index:        0,
				FinishReason: "tool_calls",
				Message: llm.Message{
					Role: llm.RoleAssistant,
					ToolCalls: []llm.ToolCall{
						{
							ID:        "call_1",
							Name:      "get_weather",
							Arguments: json.RawMessage(`{"city":"Unknown"}`),
						},
					},
				},
			},
		},
		Usage: llm.ChatUsage{TotalTokens: 10},
	}

	provider := &testProvider{
		name: "test-provider",
		completionFn: func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
			return resp1, nil
		},
	}

	// 模拟工具执行时出错
	toolResults := []tools.ToolResult{
		{
			ToolCallID: "call_1",
			Name:       "get_weather",
			Error:      "city not found",
		},
	}

	executor := &testToolExecutor{
		executeFn: func(ctx context.Context, calls []llm.ToolCall) []tools.ToolResult {
			return toolResults
		},
	}

	config := tools.ReActConfig{
		MaxIterations: 5,
		StopOnError:   true,
	}

	reactExecutor := tools.NewReActExecutor(provider, executor, config, logger)

	ctx := context.Background()
	req := &llm.ChatRequest{
		Model: "gpt-4",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "Get weather"},
		},
		Tools: []llm.ToolSchema{
			{Name: "get_weather", Description: "Get weather"},
		},
	}

	// 执行 ReAct 循环 - 应因错误而停止
	resp, steps, err := reactExecutor.Execute(ctx, req)

	assert.Error(t, err)
	assert.NotNil(t, resp)
	assert.Len(t, steps, 1)
	assert.Contains(t, err.Error(), "tool execution failed")
}

// TestReActLoop_MaxIterations 测试 ReAct 循环达到最大迭代次数
func TestReActLoop_MaxIterations(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	// 始终返回工具调用
	resp := &llm.ChatResponse{
		ID:       "resp",
		Provider: "test-provider",
		Model:    "gpt-4",
		Choices: []llm.ChatChoice{
			{
				Index:        0,
				FinishReason: "tool_calls",
				Message: llm.Message{
					Role: llm.RoleAssistant,
					ToolCalls: []llm.ToolCall{
						{
							ID:        "call",
							Name:      "test_tool",
							Arguments: json.RawMessage(`{}`),
						},
					},
				},
			},
		},
		Usage: llm.ChatUsage{TotalTokens: 10},
	}

	provider := &testProvider{
		name: "test-provider",
		completionFn: func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
			return resp, nil
		},
	}

	toolResults := []tools.ToolResult{
		{
			ToolCallID: "call",
			Name:       "test_tool",
			Result:     json.RawMessage(`"ok"`),
		},
	}

	executor := &testToolExecutor{
		executeFn: func(ctx context.Context, calls []llm.ToolCall) []tools.ToolResult {
			return toolResults
		},
	}

	config := tools.ReActConfig{
		MaxIterations: 2,
		StopOnError:   false,
	}

	reactExecutor := tools.NewReActExecutor(provider, executor, config, logger)

	ctx := context.Background()
	req := &llm.ChatRequest{
		Model: "gpt-4",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "Test"},
		},
		Tools: []llm.ToolSchema{
			{Name: "test_tool", Description: "Test tool"},
		},
	}

	// 执行 ReAct 循环 - 应达到最大迭代次数
	_, steps, err := reactExecutor.Execute(ctx, req)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "max iterations reached")
	assert.Len(t, steps, 2)
}

// BenchmarkReActLoop 基准测试 ReAct 循环性能
func BenchmarkReActLoop(b *testing.B) {
	logger, _ := zap.NewDevelopment()

	resp := &llm.ChatResponse{
		ID:       "resp",
		Provider: "test-provider",
		Model:    "gpt-4",
		Choices: []llm.ChatChoice{
			{
				Index:        0,
				FinishReason: "stop",
				Message: llm.Message{
					Role:    llm.RoleAssistant,
					Content: "Done",
				},
			},
		},
	}

	provider := &testProvider{
		name:           "test-provider",
		supportsNative: true,
		completionFn: func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
			return resp, nil
		},
	}

	executor := &testToolExecutor{}

	config := tools.ReActConfig{
		MaxIterations: 5,
		StopOnError:   true,
	}

	reactExecutor := tools.NewReActExecutor(provider, executor, config, logger)

	ctx := context.Background()
	req := &llm.ChatRequest{
		Model: "gpt-4",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "Test"},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = reactExecutor.Execute(ctx, req)
	}
}
