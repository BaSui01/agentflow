package agent

import (
	"context"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

// 测试新DynamicTooSelector 测试创建工具选择器
func TestNewDynamicToolSelector(t *testing.T) {
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
	selectorConfig := defaultToolSelectionConfigValue()

	selector := NewDynamicToolSelector(agent, selectorConfig)

	assert.NotNil(t, selector)
	assert.Equal(t, 5, selector.config.MaxTools)
	assert.Equal(t, 0.3, selector.config.MinScore)
}

// TestDynamicTooSelector SelectedTools 失效测试选择器
func TestDynamicToolSelector_SelectTools_Disabled(t *testing.T) {
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
	selectorConfig := defaultToolSelectionConfigValue()
	selectorConfig.Enabled = false

	selector := NewDynamicToolSelector(agent, selectorConfig)

	ctx := context.Background()
	tools := []llm.ToolSchema{
		{Name: "tool1", Description: "Tool 1"},
		{Name: "tool2", Description: "Tool 2"},
	}

	selected, err := selector.SelectTools(ctx, "test task", tools)

	assert.NoError(t, err)
	assert.Equal(t, tools, selected)
}

// TestDynamicTooL 选择器 SelectTools 成功测试工具选择
func TestDynamicToolSelector_SelectTools_Success(t *testing.T) {
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
	selectorConfig := defaultToolSelectionConfigValue()
	selectorConfig.MaxTools = 2
	selectorConfig.UseLLMRanking = false

	selector := NewDynamicToolSelector(agent, selectorConfig)

	ctx := context.Background()
	tools := []llm.ToolSchema{
		{Name: "search_web", Description: "Search the web for information"},
		{Name: "calculate", Description: "Perform mathematical calculations"},
		{Name: "send_email", Description: "Send an email to someone"},
	}

	selected, err := selector.SelectTools(ctx, "search for information online", tools)

	assert.NoError(t, err)
	assert.LessOrEqual(t, len(selected), 2)
	// 由于语义相似, 搜索  web 应当选中
	if len(selected) > 0 {
		assert.Equal(t, "search_web", selected[0].Name)
	}
}

// 测试DynamicTooSelector ScoreTools 测试工具评分
func TestDynamicToolSelector_ScoreTools(t *testing.T) {
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
	selectorConfig := defaultToolSelectionConfigValue()

	selector := NewDynamicToolSelector(agent, selectorConfig)

	ctx := context.Background()
	tools := []llm.ToolSchema{
		{Name: "search_web", Description: "Search the web for information"},
		{Name: "calculate", Description: "Perform mathematical calculations"},
	}

	scores, err := selector.ScoreTools(ctx, "search for information", tools)

	assert.NoError(t, err)
	assert.Len(t, scores, 2)
	assert.Greater(t, scores[0].TotalScore, 0.0)
	assert.LessOrEqual(t, scores[0].TotalScore, 1.0)
}

// Test Dynamic ToolSelector 计算语义相似性测试语义相似性
func TestDynamicToolSelector_calculateSemanticSimilarity(t *testing.T) {
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
	selectorConfig := defaultToolSelectionConfigValue()

	selector := NewDynamicToolSelector(agent, selectorConfig)

	tests := []struct {
		name     string
		task     string
		tool     llm.ToolSchema
		minScore float64
	}{
		{
			name: "high similarity",
			task: "search for information online",
			tool: llm.ToolSchema{
				Name:        "search_web",
				Description: "Search the web for information",
			},
			minScore: 0.5,
		},
		{
			name: "low similarity",
			task: "send an email",
			tool: llm.ToolSchema{
				Name:        "calculate",
				Description: "Perform mathematical calculations",
			},
			minScore: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			similarity := selector.calculateSemanticSimilarity(tt.task, tt.tool)
			assert.GreaterOrEqual(t, similarity, tt.minScore)
			assert.LessOrEqual(t, similarity, 1.0)
		})
	}
}

// TestDynamic ToolSelector  Update ToolStats 测试更新工具统计
func TestDynamicToolSelector_UpdateToolStats(t *testing.T) {
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
	selectorConfig := defaultToolSelectionConfigValue()

	selector := NewDynamicToolSelector(agent, selectorConfig)

	// 更新成功调用的数据
	selector.UpdateToolStats("search_web", true, 100*time.Millisecond, 0.05)

	stats := selector.toolStats["search_web"]
	assert.NotNil(t, stats)
	assert.Equal(t, int64(1), stats.TotalCalls)
	assert.Equal(t, int64(1), stats.SuccessfulCalls)
	assert.Equal(t, int64(0), stats.FailedCalls)
	assert.Equal(t, 100*time.Millisecond, stats.TotalLatency)
	assert.Equal(t, 0.05, stats.AvgCost)

	// 更新无法调用的数据
	selector.UpdateToolStats("search_web", false, 200*time.Millisecond, 0.03)

	assert.Equal(t, int64(2), stats.TotalCalls)
	assert.Equal(t, int64(1), stats.SuccessfulCalls)
	assert.Equal(t, int64(1), stats.FailedCalls)
	assert.Equal(t, 300*time.Millisecond, stats.TotalLatency)
	assert.Equal(t, 0.04, stats.AvgCost) // (0.05 + 0.03) / 2
}

// 测试动态工具Selector 获得可靠性测试可靠性计算
func TestDynamicToolSelector_getReliability(t *testing.T) {
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
	selectorConfig := defaultToolSelectionConfigValue()

	selector := NewDynamicToolSelector(agent, selectorConfig)

	// 添加数据
	selector.toolStats["reliable_tool"] = &ToolStats{
		Name:            "reliable_tool",
		TotalCalls:      10,
		SuccessfulCalls: 9,
		FailedCalls:     1,
	}

	reliability := selector.getReliability("reliable_tool")
	assert.Equal(t, 0.9, reliability)

	// 未知工具应返回默认值
	defaultReliability := selector.getReliability("unknown_tool")
	assert.Equal(t, 0.8, defaultReliability)
}

// TestExtractKeywords 测试关键字提取
func TestExtractKeywords(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{
			name:     "simple text",
			text:     "search for information online",
			expected: 3,
		},
		{
			name:     "with stop words",
			text:     "the quick brown fox",
			expected: 3, // "the" is filtered
		},
		{
			name:     "chinese text",
			text:     "搜索网络信息",
			expected: 1, // Single word without spaces
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keywords := extractKeywords(tt.text)
			assert.Equal(t, tt.expected, len(keywords))
		})
	}
}

// 测试工具索引
func TestParseToolIndices(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected []int
	}{
		{
			name:     "simple list",
			text:     "1,2,3",
			expected: []int{1, 2, 3},
		},
		{
			name:     "with spaces",
			text:     "1, 2, 3",
			expected: []int{1, 2, 3},
		},
		{
			name:     "with newlines",
			text:     "1\n2\n3",
			expected: []int{},
		},
		{
			name:     "mixed format",
			text:     "1,2,3,5",
			expected: []int{1, 2, 3, 5},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			indices := parseToolIndices(tt.text)
			assert.Equal(t, tt.expected, indices)
		})
	}
}

// TestDynamicTooL 选择器 SelectTools WithLLMRanging 测试 LLM 辅助排名
func TestDynamicToolSelector_SelectTools_WithLLMRanking(t *testing.T) {
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
	selectorConfig := defaultToolSelectionConfigValue()
	selectorConfig.MaxTools = 2
	selectorConfig.UseLLMRanking = true

	selector := NewDynamicToolSelector(agent, selectorConfig)

	ctx := context.Background()
	tools := []llm.ToolSchema{
		{Name: "search_web", Description: "Search the web"},
		{Name: "calculate", Description: "Calculate numbers"},
		{Name: "send_email", Description: "Send email"},
		{Name: "read_file", Description: "Read file"},
		{Name: "write_file", Description: "Write file"},
		{Name: "api_call", Description: "Call API"},
	}

	// Mock LLM 排名响应
	rankingResponse := &llm.ChatResponse{
		ID:       "ranking-response",
		Provider: "mock",
		Model:    "gpt-4",
		Choices: []llm.ChatChoice{
			{
				Index:        0,
				FinishReason: "stop",
				Message: llm.Message{
					Role:    llm.RoleAssistant,
					Content: "1,3,2",
				},
			},
		},
	}

	provider.On("Completion", mock.Anything, mock.MatchedBy(func(req *llm.ChatRequest) bool {
		return len(req.Messages) == 2 && req.Messages[0].Role == llm.RoleSystem
	})).Return(rankingResponse, nil)

	selected, err := selector.SelectTools(ctx, "search for information", tools)

	assert.NoError(t, err)
	assert.LessOrEqual(t, len(selected), 2)

	provider.AssertExpectations(t)
}

// 基准动态工具Selector SelectTools 基准工具选择
func BenchmarkDynamicToolSelector_SelectTools(b *testing.B) {
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
	selectorConfig := defaultToolSelectionConfigValue()
	selectorConfig.UseLLMRanking = false

	selector := NewDynamicToolSelector(agent, selectorConfig)

	ctx := context.Background()
	tools := []llm.ToolSchema{
		{Name: "tool1", Description: "Tool 1"},
		{Name: "tool2", Description: "Tool 2"},
		{Name: "tool3", Description: "Tool 3"},
		{Name: "tool4", Description: "Tool 4"},
		{Name: "tool5", Description: "Tool 5"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = selector.SelectTools(ctx, "test task", tools)
	}
}
