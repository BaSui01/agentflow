package agent

import (
	"context"
	"testing"

	"github.com/BaSui01/agentflow/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

// TestNewReflectionExecutor tests creating reflection executor
func TestNewReflectionExecutor(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	provider := new(MockProvider)
	memory := new(MockMemoryManager)
	toolManager := new(MockToolManager)
	bus := new(MockEventBus)

	config := Config{
		ID:    "test-agent",
		Name:  "Test Agent",
		Type:  TypeGeneric,
		Model: "gpt-4",
	}

	agent := NewBaseAgent(config, provider, memory, toolManager, bus, logger)
	reflectionConfig := DefaultReflectionExecutorConfig()

	executor := NewReflectionExecutor(agent, reflectionConfig)

	assert.NotNil(t, executor)
	assert.Equal(t, 3, executor.config.MaxIterations)
	assert.Equal(t, 0.7, executor.config.MinQuality)
}

// TestReflectionExecutor_ExecuteWithReflection_Disabled tests disabled reflection
func TestReflectionExecutor_ExecuteWithReflection_Disabled(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	provider := new(MockProvider)
	memory := new(MockMemoryManager)
	toolManager := new(MockToolManager)
	bus := new(MockEventBus)

	config := Config{
		ID:    "test-agent",
		Name:  "Test Agent",
		Type:  TypeGeneric,
		Model: "gpt-4",
		PromptBundle: PromptBundle{
			System: SystemPrompt{
				Identity: "You are a helpful assistant",
			},
		},
	}

	agent := NewBaseAgent(config, provider, memory, toolManager, bus, logger)

	// Initialize agent
	memory.On("LoadRecent", mock.Anything, "test-agent", MemoryShortTerm, 10).
		Return([]MemoryRecord{}, nil)
	bus.On("Publish", mock.Anything, EventStateChange, mock.Anything).
		Return(nil)

	ctx := context.Background()
	_ = agent.Init(ctx)

	reflectionConfig := DefaultReflectionExecutorConfig()
	reflectionConfig.Enabled = false

	executor := NewReflectionExecutor(agent, reflectionConfig)

	// Mock LLM response
	mockResponse := &llm.ChatResponse{
		ID:       "test-response",
		Provider: "mock",
		Model:    "gpt-4",
		Choices: []llm.ChatChoice{
			{
				Index:        0,
				FinishReason: "stop",
				Message: llm.Message{
					Role:    llm.RoleAssistant,
					Content: "Hello! How can I help?",
				},
			},
		},
		Usage: llm.ChatUsage{TotalTokens: 10},
	}

	provider.On("Completion", mock.Anything, mock.Anything).
		Return(mockResponse, nil)
	memory.On("Save", mock.Anything, mock.Anything).
		Return(nil)

	input := &Input{
		TraceID: "test-trace",
		Content: "Hello",
	}

	result, err := executor.ExecuteWithReflection(ctx, input)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 1, result.Iterations)
	assert.False(t, result.ImprovedByReflection)
	assert.Equal(t, "Hello! How can I help?", result.FinalOutput.Content)

	provider.AssertExpectations(t)
	memory.AssertExpectations(t)
}

// TestReflectionExecutor_ExecuteWithReflection_Success tests successful reflection
func TestReflectionExecutor_ExecuteWithReflection_Success(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	provider := new(MockProvider)
	memory := new(MockMemoryManager)
	toolManager := new(MockToolManager)
	bus := new(MockEventBus)

	config := Config{
		ID:    "test-agent",
		Name:  "Test Agent",
		Type:  TypeGeneric,
		Model: "gpt-4",
		PromptBundle: PromptBundle{
			System: SystemPrompt{
				Identity: "You are a helpful assistant",
			},
		},
	}

	agent := NewBaseAgent(config, provider, memory, toolManager, bus, logger)

	// Initialize agent
	memory.On("LoadRecent", mock.Anything, "test-agent", MemoryShortTerm, 10).
		Return([]MemoryRecord{}, nil)
	bus.On("Publish", mock.Anything, EventStateChange, mock.Anything).
		Return(nil)

	ctx := context.Background()
	_ = agent.Init(ctx)

	reflectionConfig := DefaultReflectionExecutorConfig()
	reflectionConfig.MaxIterations = 2

	executor := NewReflectionExecutor(agent, reflectionConfig)

	// First execution - low quality
	firstResponse := &llm.ChatResponse{
		ID:       "response-1",
		Provider: "mock",
		Model:    "gpt-4",
		Choices: []llm.ChatChoice{
			{
				Index:        0,
				FinishReason: "stop",
				Message: llm.Message{
					Role:    llm.RoleAssistant,
					Content: "Short answer",
				},
			},
		},
		Usage: llm.ChatUsage{TotalTokens: 10},
	}

	// Critique response - low score
	critiqueResponse := &llm.ChatResponse{
		ID:       "critique-1",
		Provider: "mock",
		Model:    "gpt-4",
		Choices: []llm.ChatChoice{
			{
				Index:        0,
				FinishReason: "stop",
				Message: llm.Message{
					Role:    llm.RoleAssistant,
					Content: "评分：5/10\n问题：\n- 回答太简短\n改进建议：\n- 提供更详细的信息",
				},
			},
		},
		Usage: llm.ChatUsage{TotalTokens: 20},
	}

	// Second execution - high quality
	secondResponse := &llm.ChatResponse{
		ID:       "response-2",
		Provider: "mock",
		Model:    "gpt-4",
		Choices: []llm.ChatChoice{
			{
				Index:        0,
				FinishReason: "stop",
				Message: llm.Message{
					Role:    llm.RoleAssistant,
					Content: "Detailed and comprehensive answer with all necessary information",
				},
			},
		},
		Usage: llm.ChatUsage{TotalTokens: 30},
	}

	// Second critique - high score
	secondCritiqueResponse := &llm.ChatResponse{
		ID:       "critique-2",
		Provider: "mock",
		Model:    "gpt-4",
		Choices: []llm.ChatChoice{
			{
				Index:        0,
				FinishReason: "stop",
				Message: llm.Message{
					Role:    llm.RoleAssistant,
					Content: "评分：9/10\n问题：\n改进建议：",
				},
			},
		},
		Usage: llm.ChatUsage{TotalTokens: 15},
	}

	// Setup mock calls in order
	provider.On("Completion", mock.Anything, mock.MatchedBy(func(req *llm.ChatRequest) bool {
		return len(req.Messages) == 2 && req.Messages[1].Content == "Hello"
	})).Return(firstResponse, nil).Once()

	provider.On("Completion", mock.Anything, mock.MatchedBy(func(req *llm.ChatRequest) bool {
		return req.Messages[0].Content == "你是一个专业的质量评审专家，擅长发现问题并提供建设性建议。"
	})).Return(critiqueResponse, nil).Once()

	provider.On("Completion", mock.Anything, mock.MatchedBy(func(req *llm.ChatRequest) bool {
		return len(req.Messages) == 2 && req.Messages[1].Content != "Hello"
	})).Return(secondResponse, nil).Once()

	provider.On("Completion", mock.Anything, mock.MatchedBy(func(req *llm.ChatRequest) bool {
		return req.Messages[0].Content == "你是一个专业的质量评审专家，擅长发现问题并提供建设性建议。"
	})).Return(secondCritiqueResponse, nil).Once()

	memory.On("Save", mock.Anything, mock.Anything).Return(nil)

	input := &Input{
		TraceID: "test-trace",
		Content: "Hello",
	}

	result, err := executor.ExecuteWithReflection(ctx, input)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 2, result.Iterations)
	assert.True(t, result.ImprovedByReflection)
	assert.Len(t, result.Critiques, 2)
	assert.False(t, result.Critiques[0].IsGood)
	assert.True(t, result.Critiques[1].IsGood)

	provider.AssertExpectations(t)
}

// TestReflectionExecutor_parseCritique tests critique parsing
func TestReflectionExecutor_parseCritique(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	provider := new(MockProvider)
	memory := new(MockMemoryManager)
	toolManager := new(MockToolManager)
	bus := new(MockEventBus)

	config := Config{
		ID:    "test-agent",
		Name:  "Test Agent",
		Type:  TypeGeneric,
		Model: "gpt-4",
	}

	agent := NewBaseAgent(config, provider, memory, toolManager, bus, logger)
	reflectionConfig := DefaultReflectionExecutorConfig()
	executor := NewReflectionExecutor(agent, reflectionConfig)

	feedback := `评分：8/10
问题：
- 缺少具体示例
- 表达不够清晰
改进建议：
- 添加代码示例
- 使用更简洁的语言`

	critique := executor.parseCritique(feedback)

	assert.NotNil(t, critique)
	assert.Equal(t, 0.8, critique.Score)
	assert.True(t, critique.IsGood) // 0.8 >= 0.7
	assert.Len(t, critique.Issues, 2)
	assert.Len(t, critique.Suggestions, 2)
	assert.Contains(t, critique.Issues[0], "示例")
	assert.Contains(t, critique.Suggestions[0], "代码")
}

// TestReflectionExecutor_extractScore tests score extraction
func TestReflectionExecutor_extractScore(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	provider := new(MockProvider)
	memory := new(MockMemoryManager)
	toolManager := new(MockToolManager)
	bus := new(MockEventBus)

	config := Config{
		ID:    "test-agent",
		Name:  "Test Agent",
		Type:  TypeGeneric,
		Model: "gpt-4",
	}

	agent := NewBaseAgent(config, provider, memory, toolManager, bus, logger)
	reflectionConfig := DefaultReflectionExecutorConfig()
	executor := NewReflectionExecutor(agent, reflectionConfig)

	tests := []struct {
		name     string
		text     string
		expected float64
	}{
		{
			name:     "slash format",
			text:     "评分：8/10",
			expected: 8.0,
		},
		{
			name:     "pure number",
			text:     "7.5",
			expected: 7.5,
		},
		{
			name:     "no score",
			text:     "no score here",
			expected: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := executor.extractScore(tt.text)
			assert.Equal(t, tt.expected, score)
		})
	}
}
