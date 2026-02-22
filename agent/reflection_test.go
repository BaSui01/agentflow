package agent

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/BaSui01/agentflow/llm"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

// 测试新引用执行器测试创建反射执行器
func TestNewReflectionExecutor(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	provider := &testProvider{name: "test"}
	memory := &testMemoryManager{}
	toolManager := &testToolManager{}
	bus := &testEventBus{}

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

// 测试Reflection 执行器 Execute with Reflection Disabled 测试 已禁用反射
func TestReflectionExecutor_ExecuteWithReflection_Disabled(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	// Mock LLM 响应
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

	provider := &testProvider{
		name: "test",
		completionFn: func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
			return mockResponse, nil
		},
	}
	memory := &testMemoryManager{
		loadRecentFn: func(ctx context.Context, agentID string, kind MemoryKind, limit int) ([]MemoryRecord, error) {
			return []MemoryRecord{}, nil
		},
		saveFn: func(ctx context.Context, rec MemoryRecord) error {
			return nil
		},
	}
	toolManager := &testToolManager{}
	bus := &testEventBus{}

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

	ctx := context.Background()
	_ = agent.Init(ctx)

	reflectionConfig := DefaultReflectionExecutorConfig()
	reflectionConfig.Enabled = false

	executor := NewReflectionExecutor(agent, reflectionConfig)

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
}

// 测试引用执行器  执行引用  成功测试反射
func TestReflectionExecutor_ExecuteWithReflection_Success(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	// 第一次执行 -- -- 质量低
	firstResponse := &llm.ChatResponse{
		ID: "response-1", Provider: "mock", Model: "gpt-4",
		Choices: []llm.ChatChoice{{
			Index: 0, FinishReason: "stop",
			Message: llm.Message{Role: llm.RoleAssistant, Content: "Short answer"},
		}},
		Usage: llm.ChatUsage{TotalTokens: 10},
	}

	// 批评反应 - 低分数
	critiqueResponse := &llm.ChatResponse{
		ID: "critique-1", Provider: "mock", Model: "gpt-4",
		Choices: []llm.ChatChoice{{
			Index: 0, FinishReason: "stop",
			Message: llm.Message{
				Role:    llm.RoleAssistant,
				Content: "评分：5/10\n问题：\n- 回答太简短\n改进建议：\n- 提供更详细的信息",
			},
		}},
		Usage: llm.ChatUsage{TotalTokens: 20},
	}

	// 第二次执行 -- -- 高质量
	secondResponse := &llm.ChatResponse{
		ID: "response-2", Provider: "mock", Model: "gpt-4",
		Choices: []llm.ChatChoice{{
			Index: 0, FinishReason: "stop",
			Message: llm.Message{
				Role:    llm.RoleAssistant,
				Content: "Detailed and comprehensive answer with all necessary information",
			},
		}},
		Usage: llm.ChatUsage{TotalTokens: 30},
	}

	// 第二个批评 - 高分
	secondCritiqueResponse := &llm.ChatResponse{
		ID: "critique-2", Provider: "mock", Model: "gpt-4",
		Choices: []llm.ChatChoice{{
			Index: 0, FinishReason: "stop",
			Message: llm.Message{
				Role:    llm.RoleAssistant,
				Content: "评分：9/10\n问题：\n改进建议：",
			},
		}},
		Usage: llm.ChatUsage{TotalTokens: 15},
	}

	// Track call sequence: 1=first exec, 2=critique, 3=second exec, 4=second critique
	var callCount atomic.Int32
	provider := &testProvider{
		name: "test",
		completionFn: func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
			n := callCount.Add(1)
			// Critique calls have the quality reviewer system prompt
			isCritique := req.Messages[0].Content == "你是一个专业的质量评审专家，擅长发现问题并提供建设性建议。"
			switch {
			case n == 1: // first execution
				return firstResponse, nil
			case n == 2 && isCritique: // first critique
				return critiqueResponse, nil
			case n == 3: // second execution (improved)
				return secondResponse, nil
			case n == 4 && isCritique: // second critique
				return secondCritiqueResponse, nil
			default:
				return firstResponse, nil
			}
		},
	}
	memory := &testMemoryManager{
		loadRecentFn: func(ctx context.Context, agentID string, kind MemoryKind, limit int) ([]MemoryRecord, error) {
			return []MemoryRecord{}, nil
		},
		saveFn: func(ctx context.Context, rec MemoryRecord) error {
			return nil
		},
	}
	toolManager := &testToolManager{}
	bus := &testEventBus{}

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

	ctx := context.Background()
	_ = agent.Init(ctx)

	reflectionConfig := DefaultReflectionExecutorConfig()
	reflectionConfig.MaxIterations = 2

	executor := NewReflectionExecutor(agent, reflectionConfig)

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
	assert.GreaterOrEqual(t, int(callCount.Load()), 4, "should have at least 4 Completion calls")
}

// 测试引用执行器  praseCritic 测试批判解
func TestReflectionExecutor_parseCritique(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	provider := &testProvider{name: "test"}
	memory := &testMemoryManager{}
	toolManager := &testToolManager{}
	bus := &testEventBus{}

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

// 测试引用执行器  提取分数测试
func TestReflectionExecutor_extractScore(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	provider := &testProvider{name: "test"}
	memory := &testMemoryManager{}
	toolManager := &testToolManager{}
	bus := &testEventBus{}

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
