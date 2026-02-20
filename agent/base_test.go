package agent

import (
	"context"
	"testing"

	"github.com/BaSui01/agentflow/llm"
	llmtools "github.com/BaSui01/agentflow/llm/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

// MockProvider 模拟 LLM Provider
type MockProvider struct {
	mock.Mock
}

func (m *MockProvider) Completion(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*llm.ChatResponse), args.Error(1)
}

func (m *MockProvider) Stream(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(<-chan llm.StreamChunk), args.Error(1)
}

func (m *MockProvider) HealthCheck(ctx context.Context) (*llm.HealthStatus, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*llm.HealthStatus), args.Error(1)
}

func (m *MockProvider) Name() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockProvider) SupportsNativeFunctionCalling() bool {
	args := m.Called()
	return args.Bool(0)
}

func (m *MockProvider) ListModels(ctx context.Context) ([]llm.Model, error) {
	_ = ctx
	return nil, nil
}

// MockMemoryManager 模拟记忆管理器
type MockMemoryManager struct {
	mock.Mock
}

func (m *MockMemoryManager) Save(ctx context.Context, rec MemoryRecord) error {
	args := m.Called(ctx, rec)
	return args.Error(0)
}

func (m *MockMemoryManager) Delete(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockMemoryManager) Clear(ctx context.Context, agentID string, kind MemoryKind) error {
	args := m.Called(ctx, agentID, kind)
	return args.Error(0)
}

func (m *MockMemoryManager) LoadRecent(ctx context.Context, agentID string, kind MemoryKind, limit int) ([]MemoryRecord, error) {
	args := m.Called(ctx, agentID, kind, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]MemoryRecord), args.Error(1)
}

func (m *MockMemoryManager) Search(ctx context.Context, agentID string, query string, topK int) ([]MemoryRecord, error) {
	args := m.Called(ctx, agentID, query, topK)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]MemoryRecord), args.Error(1)
}

func (m *MockMemoryManager) Get(ctx context.Context, id string) (*MemoryRecord, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*MemoryRecord), args.Error(1)
}

// MockToolManager 模拟工具管理器
type MockToolManager struct {
	mock.Mock
}

func (m *MockToolManager) ExecuteForAgent(ctx context.Context, agentID string, calls []llm.ToolCall) []llmtools.ToolResult {
	args := m.Called(ctx, agentID, calls)
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).([]llmtools.ToolResult)
}

func (m *MockToolManager) GetAllowedTools(agentID string) []llm.ToolSchema {
	args := m.Called(agentID)
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).([]llm.ToolSchema)
}

// MockEventBus 模拟事件总线
type MockEventBus struct {
	mock.Mock
}

func (m *MockEventBus) Publish(event Event) {
	m.Called(event)
}

func (m *MockEventBus) Subscribe(eventType EventType, handler EventHandler) string {
	args := m.Called(eventType, handler)
	return args.String(0)
}

func (m *MockEventBus) Unsubscribe(subscriptionID string) {
	m.Called(subscriptionID)
}

// TestNewBaseAgent 测试创建 BaseAgent
func TestNewBaseAgent(t *testing.T) {
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

	assert.NotNil(t, agent)
	assert.Equal(t, "test-agent", agent.ID())
	assert.Equal(t, "Test Agent", agent.Name())
	assert.Equal(t, TypeGeneric, agent.Type())
	assert.Equal(t, StateInit, agent.State())
}

// TestBaseAgent_Init 测试初始化
func TestBaseAgent_Init(t *testing.T) {
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

	// 模拟内存装入
	memory.On("LoadRecent", mock.Anything, "test-agent", MemoryShortTerm, 10).
		Return([]MemoryRecord{}, nil)

	// 模拟事件发布 - 发布需要单个事件参数
	bus.On("Publish", mock.AnythingOfType("*agent.StateChangeEvent")).
		Return()

	ctx := context.Background()
	err := agent.Init(ctx)

	assert.NoError(t, err)
	assert.Equal(t, StateReady, agent.State())

	memory.AssertExpectations(t)
	bus.AssertExpectations(t)
}

// TestBaseAgent_StateTransition 测试状态转换
func TestBaseAgent_StateTransition(t *testing.T) {
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

	// 模拟事件发布 - 发布需要单个事件参数
	bus.On("Publish", mock.AnythingOfType("*agent.StateChangeEvent")).
		Return()

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

	bus.AssertExpectations(t)
}

// TestBaseAgent_Execute 测试执行任务
func TestBaseAgent_Execute(t *testing.T) {
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

	// 初始化代理
	memory.On("LoadRecent", mock.Anything, "test-agent", MemoryShortTerm, 10).
		Return([]MemoryRecord{}, nil)
	bus.On("Publish", mock.AnythingOfType("*agent.StateChangeEvent")).
		Return()

	ctx := context.Background()
	err := agent.Init(ctx)
	assert.NoError(t, err)

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

	provider.On("Completion", mock.Anything, mock.Anything).
		Return(mockResponse, nil)

	// 保存记忆
	memory.On("Save", mock.Anything, mock.Anything).
		Return(nil)

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

	provider.AssertExpectations(t)
	memory.AssertExpectations(t)
	bus.AssertExpectations(t)
}

// TestBaseAgent_ExecuteNotReady 测试未就绪时执行
func TestBaseAgent_ExecuteNotReady(t *testing.T) {
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

	ctx := context.Background()

	// 保存记忆
	memory.On("Save", mock.Anything, mock.MatchedBy(func(rec MemoryRecord) bool {
		return rec.AgentID == "test-agent" &&
			rec.Kind == MemoryShortTerm &&
			rec.Content == "test content"
	})).Return(nil)

	err := agent.SaveMemory(ctx, "test content", MemoryShortTerm, map[string]any{
		"key": "value",
	})

	assert.NoError(t, err)
	memory.AssertExpectations(t)
}

// TestBaseAgent_RecallMemory 测试检索记忆
func TestBaseAgent_RecallMemory(t *testing.T) {
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

	ctx := context.Background()

	// 模拟内存搜索
	expectedRecords := []MemoryRecord{
		{
			ID:      "mem-1",
			AgentID: "test-agent",
			Kind:    MemoryLongTerm,
			Content: "relevant memory",
		},
	}

	memory.On("Search", mock.Anything, "test-agent", "query", 5).
		Return(expectedRecords, nil)

	records, err := agent.RecallMemory(ctx, "query", 5)

	assert.NoError(t, err)
	assert.Len(t, records, 1)
	assert.Equal(t, "relevant memory", records[0].Content)

	memory.AssertExpectations(t)
}

// TestBaseAgent_Observe 测试观察反馈
func TestBaseAgent_Observe(t *testing.T) {
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

	ctx := context.Background()

	// 保存记忆
	memory.On("Save", mock.Anything, mock.MatchedBy(func(rec MemoryRecord) bool {
		return rec.AgentID == "test-agent" &&
			rec.Kind == MemoryLongTerm &&
			rec.Content == "feedback content"
	})).Return(nil)

	// 模拟事件发布 - 发布需要单个事件参数
	bus.On("Publish", mock.AnythingOfType("*agent.FeedbackEvent")).
		Return()

	feedback := &Feedback{
		Type:    "approval",
		Content: "feedback content",
		Data: map[string]any{
			"score": 5,
		},
	}

	err := agent.Observe(ctx, feedback)

	assert.NoError(t, err)
	memory.AssertExpectations(t)
	bus.AssertExpectations(t)
}

// TestBaseAgent_Plan 测试生成执行计划
func TestBaseAgent_Plan(t *testing.T) {
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
				Identity: "You are a planning expert",
			},
		},
	}

	agent := NewBaseAgent(config, provider, memory, toolManager, bus, logger)

	ctx := context.Background()

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

	provider.On("Completion", mock.Anything, mock.Anything).
		Return(mockResponse, nil)

	input := &Input{
		TraceID: "test-trace",
		Content: "Build a web application",
	}

	plan, err := agent.Plan(ctx, input)

	assert.NoError(t, err)
	assert.NotNil(t, plan)
	assert.Greater(t, len(plan.Steps), 0)
	assert.Contains(t, plan.Steps[0], "Analyze")

	provider.AssertExpectations(t)
}

// BenchmarkBaseAgent_Execute 性能测试
func BenchmarkBaseAgent_Execute(b *testing.B) {
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

	// 设置模拟
	memory.On("LoadRecent", mock.Anything, "test-agent", MemoryShortTerm, 10).
		Return([]MemoryRecord{}, nil)
	bus.On("Publish", mock.AnythingOfType("*agent.StateChangeEvent")).
		Return()

	ctx := context.Background()
	_ = agent.Init(ctx)

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

	provider.On("Completion", mock.Anything, mock.Anything).
		Return(mockResponse, nil)
	memory.On("Save", mock.Anything, mock.Anything).
		Return(nil)

	input := &Input{
		TraceID: "test-trace",
		Content: "Hello",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = agent.Execute(ctx, input)
	}
}
