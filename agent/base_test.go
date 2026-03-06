package agent

import (
	"context"
	"github.com/BaSui01/agentflow/types"
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

	config := testAgentConfig("test-agent", "Test Agent", "gpt-4")

	agent := NewBaseAgent(config, provider, memory, toolManager, bus, logger, nil)

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

	config := testAgentConfig("test-agent", "Test Agent", "gpt-4")

	agent := NewBaseAgent(config, provider, memory, toolManager, bus, logger, nil)

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

	config := testAgentConfig("test-agent", "Test Agent", "gpt-4")

	agent := NewBaseAgent(config, provider, memory, toolManager, bus, logger, nil)

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
				Message: types.Message{
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

	config := testAgentConfig("test-agent", "Test Agent", "gpt-4")
	config.Runtime.SystemPrompt = "You are a helpful assistant"

	agent := NewBaseAgent(config, provider, memory, toolManager, bus, logger, nil)

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

	config := testAgentConfig("test-agent", "Test Agent", "gpt-4")

	agent := NewBaseAgent(config, provider, memory, toolManager, bus, logger, nil)

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

	config := testAgentConfig("test-agent", "Test Agent", "gpt-4")

	agent := NewBaseAgent(config, provider, memory, toolManager, bus, logger, nil)

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

	config := testAgentConfig("test-agent", "Test Agent", "gpt-4")

	agent := NewBaseAgent(config, provider, memory, toolManager, bus, logger, nil)

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

	config := testAgentConfig("test-agent", "Test Agent", "gpt-4")

	agent := NewBaseAgent(config, provider, memory, toolManager, bus, logger, nil)

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
				Message: types.Message{
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

	config := testAgentConfig("test-agent", "Test Agent", "gpt-4")
	config.Runtime.SystemPrompt = "You are a planning expert"

	agent := NewBaseAgent(config, provider, memory, toolManager, bus, logger, nil)

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
				Message: types.Message{
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

	config := testAgentConfig("test-agent", "Test Agent", "gpt-4")
	config.Runtime.SystemPrompt = "You are a helpful assistant"

	agent := NewBaseAgent(config, provider, memory, toolManager, bus, logger, nil)

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

// TestSaveMemory_WriteThroughCache verifies that SaveMemory appends the new
// record to the in-process recentMemory cache so that subsequent Execute()
// calls see it without a full reload from the MemoryManager.
func TestSaveMemory_WriteThroughCache(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	var saved []MemoryRecord
	mem := &testMemoryManager{
		loadRecentFn: func(_ context.Context, _ string, _ MemoryKind, _ int) ([]MemoryRecord, error) {
			return nil, nil // start empty
		},
		saveFn: func(_ context.Context, rec MemoryRecord) error {
			saved = append(saved, rec)
			return nil
		},
	}

	agent := NewBaseAgent(testAgentConfig("mem-test", "mem-test", ""), &testProvider{name: "mock"}, mem, nil, nil, logger, nil)
	ctx := context.Background()
	_ = agent.Init(ctx)

	// Cache should be empty after Init (no records in store).
	agent.recentMemoryMu.RLock()
	assert.Empty(t, agent.recentMemory)
	agent.recentMemoryMu.RUnlock()

	// Save a record.
	err := agent.SaveMemory(ctx, "hello", MemoryShortTerm, nil)
	assert.NoError(t, err)

	// Cache should now contain the saved record.
	agent.recentMemoryMu.RLock()
	assert.Len(t, agent.recentMemory, 1)
	assert.Equal(t, "hello", agent.recentMemory[0].Content)
	agent.recentMemoryMu.RUnlock()

	// Underlying store should also have it.
	assert.Len(t, saved, 1)
}

// TestSaveMemory_CacheEviction verifies that the cache is bounded by
// defaultMaxRecentMemory and evicts the oldest entries when full.
func TestSaveMemory_CacheEviction(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	mem := &testMemoryManager{
		loadRecentFn: func(_ context.Context, _ string, _ MemoryKind, _ int) ([]MemoryRecord, error) {
			return nil, nil
		},
		saveFn: func(_ context.Context, _ MemoryRecord) error { return nil },
	}

	agent := NewBaseAgent(testAgentConfig("evict-test", "evict-test", ""), &testProvider{name: "mock"}, mem, nil, nil, logger, nil)
	ctx := context.Background()
	_ = agent.Init(ctx)

	// Fill cache beyond the limit.
	for i := 0; i < defaultMaxRecentMemory+10; i++ {
		_ = agent.SaveMemory(ctx, "msg", MemoryShortTerm, nil)
	}

	agent.recentMemoryMu.RLock()
	assert.Equal(t, defaultMaxRecentMemory, len(agent.recentMemory))
	agent.recentMemoryMu.RUnlock()
}
