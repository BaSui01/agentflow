package integration

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

// TestReActLoop_SingleToolCall tests ReAct loop with single tool call
func TestReActLoop_SingleToolCall(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	provider := &MockProvider{name: "test-provider"}
	executor := &MockToolExecutor{}

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

	// First LLM call - decides to use tool
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

	// Second LLM call - final answer
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

	provider.On("Completion", ctx, mock.MatchedBy(func(r *llm.ChatRequest) bool {
		return len(r.Messages) == 1
	})).Return(firstResp, nil).Once()

	provider.On("Completion", ctx, mock.MatchedBy(func(r *llm.ChatRequest) bool {
		return len(r.Messages) == 3 // user + assistant + tool
	})).Return(secondResp, nil).Once()

	// Mock tool execution
	toolResults := []tools.ToolResult{
		{
			ToolCallID: "call_1",
			Name:       "get_weather",
			Result:     json.RawMessage(`{"temperature":25,"condition":"sunny"}`),
		},
	}

	executor.On("Execute", ctx, mock.Anything).Return(toolResults)

	// Execute ReAct loop
	resp, steps, err := reactExecutor.Execute(ctx, req)

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Len(t, steps, 2)
	assert.Equal(t, "The weather in Tokyo is sunny with a temperature of 25°C.", resp.Choices[0].Message.Content)

	provider.AssertExpectations(t)
	executor.AssertExpectations(t)
}

// TestReActLoop_MultipleToolCalls tests ReAct loop with multiple tool calls
func TestReActLoop_MultipleToolCalls(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	provider := &MockProvider{name: "test-provider"}
	executor := &MockToolExecutor{}

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

	// First call - add
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

	// Second call - multiply
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

	// Final call - answer
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

	provider.On("Completion", ctx, mock.MatchedBy(func(r *llm.ChatRequest) bool {
		return len(r.Messages) == 1
	})).Return(resp1, nil).Once()

	provider.On("Completion", ctx, mock.MatchedBy(func(r *llm.ChatRequest) bool {
		return len(r.Messages) == 3
	})).Return(resp2, nil).Once()

	provider.On("Completion", ctx, mock.MatchedBy(func(r *llm.ChatRequest) bool {
		return len(r.Messages) == 5
	})).Return(resp3, nil).Once()

	// Mock tool executions
	executor.On("Execute", ctx, mock.MatchedBy(func(calls []llm.ToolCall) bool {
		return len(calls) == 1 && calls[0].Name == "add"
	})).Return([]tools.ToolResult{
		{
			ToolCallID: "call_1",
			Name:       "add",
			Result:     json.RawMessage(`30`),
		},
	}).Once()

	executor.On("Execute", ctx, mock.MatchedBy(func(calls []llm.ToolCall) bool {
		return len(calls) == 1 && calls[0].Name == "multiply"
	})).Return([]tools.ToolResult{
		{
			ToolCallID: "call_2",
			Name:       "multiply",
			Result:     json.RawMessage(`60`),
		},
	}).Once()

	// Execute ReAct loop
	resp, steps, err := reactExecutor.Execute(ctx, req)

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Len(t, steps, 3)
	assert.Equal(t, "The result is 60.", resp.Choices[0].Message.Content)

	provider.AssertExpectations(t)
	executor.AssertExpectations(t)
}

// TestReActLoop_ToolError tests ReAct loop with tool error
func TestReActLoop_ToolError(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	provider := &MockProvider{name: "test-provider"}
	executor := &MockToolExecutor{}

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

	// LLM calls tool
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

	provider.On("Completion", ctx, mock.Anything).Return(resp1, nil).Once()

	// Mock tool execution with error
	toolResults := []tools.ToolResult{
		{
			ToolCallID: "call_1",
			Name:       "get_weather",
			Error:      "city not found",
		},
	}

	executor.On("Execute", ctx, mock.Anything).Return(toolResults)

	// Execute ReAct loop - should stop on error
	resp, steps, err := reactExecutor.Execute(ctx, req)

	assert.Error(t, err)
	assert.NotNil(t, resp)
	assert.Len(t, steps, 1)
	assert.Contains(t, err.Error(), "tool execution failed")

	provider.AssertExpectations(t)
	executor.AssertExpectations(t)
}

// TestReActLoop_MaxIterations tests ReAct loop reaching max iterations
func TestReActLoop_MaxIterations(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	provider := &MockProvider{name: "test-provider"}
	executor := &MockToolExecutor{}

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

	// Always return tool calls
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

	provider.On("Completion", ctx, mock.Anything).Return(resp, nil)

	toolResults := []tools.ToolResult{
		{
			ToolCallID: "call",
			Name:       "test_tool",
			Result:     json.RawMessage(`"ok"`),
		},
	}

	executor.On("Execute", ctx, mock.Anything).Return(toolResults)

	// Execute ReAct loop - should reach max iterations
	_, steps, err := reactExecutor.Execute(ctx, req)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "max iterations reached")
	assert.Len(t, steps, 2)
}

// MockToolExecutor for testing
type MockToolExecutor struct {
	mock.Mock
}

func (m *MockToolExecutor) Execute(ctx context.Context, calls []llm.ToolCall) []tools.ToolResult {
	args := m.Called(ctx, calls)
	return args.Get(0).([]tools.ToolResult)
}

// BenchmarkReActLoop benchmarks ReAct loop performance
func BenchmarkReActLoop(b *testing.B) {
	logger, _ := zap.NewDevelopment()
	provider := &MockProvider{name: "test-provider"}
	executor := &MockToolExecutor{}

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

	provider.On("Completion", ctx, mock.Anything).Return(resp, nil)
	provider.On("SupportsNativeFunctionCalling").Return(true)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = reactExecutor.Execute(ctx, req)
	}
}
