package agent

import (
	"context"
	"testing"

	"github.com/BaSui01/agentflow/llm"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

// TestNewBaseAgent 测试创建 BaseAgent
func TestNewBaseAgent(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	provider := &testProvider{name: "mock"}
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

	assert.NotNil(t, agent)
	assert.Equal(t, "test-agent", agent.ID())
	assert.Equal(t, "Test Agent", agent.Name())
	assert.Equal(t, TypeGeneric, agent.Type())
	assert.Equal(t, StateInit, agent.State())
}

// TestBaseAgent_Init 测试初始化
func TestBaseAgent_Init(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	provider := &testProvider{name: "mock"}
	memory := &testMemoryManager{
		loadRecentFn: func(ctx context.Context, agentID string, kind MemoryKind, limit int) ([]MemoryRecord, error) {
			return []MemoryRecord{}, nil
		},
	}
	toolManager := &testToolManager{}
	bus := &testEventBus{}

	config := Config{
		ID:    "test-agent",
		Name:  "Test Agent",
		Type:  TypeGeneric,
		Model: "gpt-4",
	}

	agent := NewBaseAgent(config, provider, memory, toolManager, bus, logger)

	ctx := context.Background()
	err := agent.Init(ctx)

	assert.NoError(t, err)
	assert.Equal(t, StateReady, agent.State())
}

// TestBaseAgent_StateTransition 测试状态转换
func TestBaseAgent_StateTransition(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	provider := &testProvider{name: "mock"}
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

	ctx := context.Background()

	// 有效过渡: init - > 准备
	err := agent.Transition(ctx, StateReady)
	assert.NoError(t, err)
	assert.Equal(t, StateReady, agent.State())

	// 有效过渡:准备 - > 运行
	err = agent.Transition(ctx, StateRunning)
	assert.NoError(t, err)
	assert.Equal(t, StateRunning, agent.State())

	// 无效的过渡: 运行 - > 输入
	err = agent.Transition(ctx, StateInit)
	assert.Error(t, err)
	assert.IsType(t, ErrInvalidTransition{}, err)
	assert.Equal(t, StateRunning, agent.State()) // State should not change
}

// TestBaseAgent_Execute 测试执行任务
func TestBaseAgent_Execute(t *testing.T) {
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
					Content: "Hello! How can I help you?",
				},
			},
		},
		Usage: llm.ChatUsage{
			PromptTokens:     10,
			CompletionTokens: 8,
			TotalTokens:      18,
		},
	}

	provider := &testProvider{
		name: "mock",
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

	// 初始化代理
	ctx := context.Background()
	err := agent.Init(ctx)
	assert.NoError(t, err)

	// 执行任务
	input := &Input{
		TraceID: "test-trace",
		Content: "Hello",
	}

	output, err := agent.Execute(ctx, input)

	assert.NoError(t, err)
	assert.NotNil(t, output)
	assert.Equal(t, "test-trace", output.TraceID)
	assert.Equal(t, "Hello! How can I help you?", output.Content)
	assert.Equal(t, 18, output.TokensUsed)
}

// TestBaseAgent_ExecuteNotReady 测试未就绪时执行
func TestBaseAgent_ExecuteNotReady(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	provider := &testProvider{name: "mock"}
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

	ctx := context.Background()
	input := &Input{
		TraceID: "test-trace",
		Content: "Hello",
	}

	// 特工在伊尼特状态,应该失败了
	output, err := agent.Execute(ctx, input)

	assert.Error(t, err)
	assert.Nil(t, output)
	assert.Equal(t, ErrAgentNotReady, err)
}

// TestBaseAgent_SaveMemory 测试保存记忆
func TestBaseAgent_SaveMemory(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	provider := &testProvider{name: "mock"}
	saveCalled := false
	memory := &testMemoryManager{
		saveFn: func(ctx context.Context, rec MemoryRecord) error {
			saveCalled = true
			assert.Equal(t, "test-agent", rec.AgentID)
			assert.Equal(t, MemoryShortTerm, rec.Kind)
			assert.Equal(t, "test content", rec.Content)
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
	}

	agent := NewBaseAgent(config, provider, memory, toolManager, bus, logger)

	ctx := context.Background()

	err := agent.SaveMemory(ctx, "test content", MemoryShortTerm, map[string]any{
		"key": "value",
	})

	assert.NoError(t, err)
	assert.True(t, saveCalled, "Save should have been called")
}

// TestBaseAgent_RecallMemory 测试检索记忆
func TestBaseAgent_RecallMemory(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	provider := &testProvider{name: "mock"}

	expectedRecords := []MemoryRecord{
		{
			ID:      "mem-1",
			AgentID: "test-agent",
			Kind:    MemoryLongTerm,
			Content: "relevant memory",
		},
	}

	memory := &testMemoryManager{
		searchFn: func(ctx context.Context, agentID string, query string, topK int) ([]MemoryRecord, error) {
			return expectedRecords, nil
		},
	}
	toolManager := &testToolManager{}
	bus := &testEventBus{}

	config := Config{
		ID:    "test-agent",
		Name:  "Test Agent",
		Type:  TypeGeneric,
		Model: "gpt-4",
	}

	agent := NewBaseAgent(config, provider, memory, toolManager, bus, logger)

	ctx := context.Background()

	records, err := agent.RecallMemory(ctx, "query", 5)

	assert.NoError(t, err)
	assert.Len(t, records, 1)
	assert.Equal(t, "relevant memory", records[0].Content)
}

// TestBaseAgent_Observe 测试观察反馈
func TestBaseAgent_Observe(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	provider := &testProvider{name: "mock"}
	saveCalled := false
	memory := &testMemoryManager{
		saveFn: func(ctx context.Context, rec MemoryRecord) error {
			saveCalled = true
			assert.Equal(t, "test-agent", rec.AgentID)
			assert.Equal(t, MemoryLongTerm, rec.Kind)
			assert.Equal(t, "feedback content", rec.Content)
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
	}

	agent := NewBaseAgent(config, provider, memory, toolManager, bus, logger)

	ctx := context.Background()

	feedback := &Feedback{
		Type:    "approval",
		Content: "feedback content",
		Data: map[string]any{
			"score": 5,
		},
	}

	err := agent.Observe(ctx, feedback)

	assert.NoError(t, err)
	assert.True(t, saveCalled, "Save should have been called")
}

// TestBaseAgent_Plan 测试生成执行计划
func TestBaseAgent_Plan(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	// Mock LLM 与计划的反应
	mockResponse := &llm.ChatResponse{
		ID:       "test-response",
		Provider: "mock",
		Model:    "gpt-4",
		Choices: []llm.ChatChoice{
			{
				Index:        0,
				FinishReason: "stop",
				Message: llm.Message{
					Role: llm.RoleAssistant,
					Content: `1. First step: Analyze the problem
2. Second step: Design solution
3. Third step: Implement and test`,
				},
			},
		},
		Usage: llm.ChatUsage{
			TotalTokens: 50,
		},
	}

	provider := &testProvider{
		name: "mock",
		completionFn: func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
			return mockResponse, nil
		},
	}
	memory := &testMemoryManager{}
	toolManager := &testToolManager{}
	bus := &testEventBus{}

	config := Config{
		ID:    "test-agent",
		Name:  "Test Agent",
		Type:  TypeGeneric,
		Model: "gpt-4",
		PromptBundle: PromptBundle{
			System: SystemPrompt{
				Identity: "You are a planning expert",
			},
		},
	}

	agent := NewBaseAgent(config, provider, memory, toolManager, bus, logger)

	ctx := context.Background()

	input := &Input{
		TraceID: "test-trace",
		Content: "Build a web application",
	}

	plan, err := agent.Plan(ctx, input)

	assert.NoError(t, err)
	assert.NotNil(t, plan)
	assert.Greater(t, len(plan.Steps), 0)
	assert.Contains(t, plan.Steps[0], "Analyze")
}

// BenchmarkBaseAgent_Execute 性能测试
func BenchmarkBaseAgent_Execute(b *testing.B) {
	logger, _ := zap.NewDevelopment()

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
					Content: "Response",
				},
			},
		},
		Usage: llm.ChatUsage{
			TotalTokens: 10,
		},
	}

	provider := &testProvider{
		name: "mock",
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

	input := &Input{
		TraceID: "test-trace",
		Content: "Hello",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = agent.Execute(ctx, input)
	}
}
