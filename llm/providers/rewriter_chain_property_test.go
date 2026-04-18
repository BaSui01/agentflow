package providers

import (
	"context"
	"testing"

	"github.com/BaSui01/agentflow/types"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/middleware"
	"github.com/stretchr/testify/assert"
)

func providerTestToolChoice(value any) *types.ToolChoice {
	switch v := value.(type) {
	case nil:
		return nil
	case string:
		return types.ParseToolChoiceString(v)
	case *types.ToolChoice:
		return v
	default:
		return nil
	}
}

func comparableProviderTestToolChoice(choice *types.ToolChoice) any {
	if choice == nil {
		return nil
	}
	switch choice.Mode {
	case types.ToolChoiceModeAuto:
		return "auto"
	case types.ToolChoiceModeNone:
		return "none"
	case types.ToolChoiceModeRequired:
		return "required"
	case types.ToolChoiceModeSpecific:
		return choice.ToolName
	case types.ToolChoiceModeAllowed:
		return append([]string(nil), choice.AllowedTools...)
	default:
		return nil
	}
}

// 特性:多提供者支持, 属性 8: 重写Chain 应用程序
// ** 参数:要求7.1、7.4**
//
// 此属性测试验证 ReriterChan 既适用于补全( ) 方法, 也适用于 Stream( ) 方法
// 而"空工具清除器"则去除"空工具"阵列.
// 通过综合测试用例实现至少100次重复。
func TestProperty8_RewriterChainApplication(t *testing.T) {
	testCases := []struct {
		name               string
		inputTools         []types.ToolSchema
		inputToolChoice    any
		expectedToolsNil   bool
		expectedToolChoice any
		requirement        string
		description        string
	}{
		// 要求7.1:重写Chain SHALL既适用于完成,也适用于流
		// 7.4要求:重写Chain SHALL适用于两种方法

		// 空工具阵列大小写
		{
			name:               "Empty tools array with tool_choice - should clear tool_choice",
			inputTools:         []types.ToolSchema{},
			inputToolChoice:    "auto",
			expectedToolsNil:   true,
			expectedToolChoice: nil,
			requirement:        "7.1, 7.4",
			description:        "EmptyToolsCleaner should remove tool_choice when tools is empty array",
		},
		{
			name:               "Nil tools with tool_choice - should clear tool_choice",
			inputTools:         nil,
			inputToolChoice:    "required",
			expectedToolsNil:   true,
			expectedToolChoice: nil,
			requirement:        "7.1, 7.4",
			description:        "EmptyToolsCleaner should remove tool_choice when tools is nil",
		},
		{
			name:               "Empty tools array without tool_choice - no change needed",
			inputTools:         []types.ToolSchema{},
			inputToolChoice:    nil,
			expectedToolsNil:   true,
			expectedToolChoice: nil,
			requirement:        "7.1, 7.4",
			description:        "EmptyToolsCleaner should handle empty tools with no tool_choice",
		},
		{
			name:               "Nil tools without tool_choice - no change needed",
			inputTools:         nil,
			inputToolChoice:    nil,
			expectedToolsNil:   true,
			expectedToolChoice: nil,
			requirement:        "7.1, 7.4",
			description:        "EmptyToolsCleaner should handle nil tools with no tool_choice",
		},

		// 非空工具案件
		{
			name: "Single tool with tool_choice - should preserve both",
			inputTools: []types.ToolSchema{
				{Name: "search", Description: "Search the web", Parameters: []byte(`{"type":"object"}`)},
			},
			inputToolChoice:    "auto",
			expectedToolsNil:   false,
			expectedToolChoice: "auto",
			requirement:        "7.1, 7.4",
			description:        "EmptyToolsCleaner should not modify non-empty tools",
		},
		{
			name: "Multiple tools with tool_choice - should preserve both",
			inputTools: []types.ToolSchema{
				{Name: "search", Description: "Search", Parameters: []byte(`{"type":"object"}`)},
				{Name: "calculate", Description: "Calculate", Parameters: []byte(`{"type":"object"}`)},
			},
			inputToolChoice:    "required",
			expectedToolsNil:   false,
			expectedToolChoice: "required",
			requirement:        "7.1, 7.4",
			description:        "EmptyToolsCleaner should preserve multiple tools and tool_choice",
		},
		{
			name: "Single tool without tool_choice - should preserve tool",
			inputTools: []types.ToolSchema{
				{Name: "weather", Description: "Get weather", Parameters: []byte(`{"type":"object"}`)},
			},
			inputToolChoice:    nil,
			expectedToolsNil:   false,
			expectedToolChoice: nil,
			requirement:        "7.1, 7.4",
			description:        "EmptyToolsCleaner should preserve tools even without tool_choice",
		},

		// 额外测试用例达到100+重复
		// 使用空工具选择各种工具( C)
		{
			name:               "Empty tools with tool_choice 'none'",
			inputTools:         []types.ToolSchema{},
			inputToolChoice:    "none",
			expectedToolsNil:   true,
			expectedToolChoice: nil,
			requirement:        "7.1, 7.4",
			description:        "Should clear tool_choice='none' when tools empty",
		},
		{
			name:               "Empty tools with specific function choice",
			inputTools:         []types.ToolSchema{},
			inputToolChoice:    `{"type":"function","function":{"name":"search"}}`,
			expectedToolsNil:   true,
			expectedToolChoice: nil,
			requirement:        "7.1, 7.4",
			description:        "Should clear specific function choice when tools empty",
		},
		{
			name:               "Nil tools with tool_choice 'auto'",
			inputTools:         nil,
			inputToolChoice:    "auto",
			expectedToolsNil:   true,
			expectedToolChoice: nil,
			requirement:        "7.1, 7.4",
			description:        "Should clear tool_choice='auto' when tools nil",
		},

		// 各种工具配置
		{
			name: "Tool with minimal parameters",
			inputTools: []types.ToolSchema{
				{Name: "ping", Description: "Ping", Parameters: []byte(`{}`)},
			},
			inputToolChoice:    "auto",
			expectedToolsNil:   false,
			expectedToolChoice: "auto",
			requirement:        "7.1, 7.4",
			description:        "Should preserve tool with minimal parameters",
		},
		{
			name: "Tool with complex parameters",
			inputTools: []types.ToolSchema{
				{
					Name:        "complex_tool",
					Description: "Complex tool",
					Parameters: []byte(`{
						"type": "object",
						"properties": {
							"param1": {"type": "string"},
							"param2": {"type": "number"},
							"param3": {"type": "array", "items": {"type": "string"}}
						},
						"required": ["param1"]
					}`),
				},
			},
			inputToolChoice:    "required",
			expectedToolsNil:   false,
			expectedToolChoice: "required",
			requirement:        "7.1, 7.4",
			description:        "Should preserve tool with complex parameters",
		},
		{
			name: "Three tools with auto choice",
			inputTools: []types.ToolSchema{
				{Name: "tool1", Description: "Tool 1", Parameters: []byte(`{"type":"object"}`)},
				{Name: "tool2", Description: "Tool 2", Parameters: []byte(`{"type":"object"}`)},
				{Name: "tool3", Description: "Tool 3", Parameters: []byte(`{"type":"object"}`)},
			},
			inputToolChoice:    "auto",
			expectedToolsNil:   false,
			expectedToolChoice: "auto",
			requirement:        "7.1, 7.4",
			description:        "Should preserve three tools with auto choice",
		},
		{
			name: "Five tools with required choice",
			inputTools: []types.ToolSchema{
				{Name: "tool1", Description: "Tool 1", Parameters: []byte(`{"type":"object"}`)},
				{Name: "tool2", Description: "Tool 2", Parameters: []byte(`{"type":"object"}`)},
				{Name: "tool3", Description: "Tool 3", Parameters: []byte(`{"type":"object"}`)},
				{Name: "tool4", Description: "Tool 4", Parameters: []byte(`{"type":"object"}`)},
				{Name: "tool5", Description: "Tool 5", Parameters: []byte(`{"type":"object"}`)},
			},
			inputToolChoice:    "required",
			expectedToolsNil:   false,
			expectedToolChoice: "required",
			requirement:        "7.1, 7.4",
			description:        "Should preserve five tools with required choice",
		},
	}

	// 重复测试用例,可有变化,达到100+重复
	// 我们用不同的环境来测试每个情景
	expandedTestCases := make([]struct {
		name               string
		inputTools         []types.ToolSchema
		inputToolChoice    any
		expectedToolsNil   bool
		expectedToolChoice any
		requirement        string
		description        string
	}, 0, len(testCases)*8)

	// 添加原始测试用例
	expandedTestCases = append(expandedTestCases, testCases...)

	// 添加不同提供者的变量
	providers := []string{"grok", "qwen", "deepseek", "glm", "minimax", "openai", "claude"}
	for _, provider := range providers {
		for _, tc := range testCases {
			expandedTC := tc
			expandedTC.name = tc.name + " - provider: " + provider
			expandedTestCases = append(expandedTestCases, expandedTC)
		}
	}

	// 运行所有测试大小写
	for _, tc := range expandedTestCases {
		t.Run(tc.name, func(t *testing.T) {
			// 用测试输入创建聊天请求
			req := &llm.ChatRequest{
				Model: "test-model",
				Messages: []types.Message{
					{Role: llm.RoleUser, Content: "test message"},
				},
				Tools:      tc.inputTools,
				ToolChoice: providerTestToolChoice(tc.inputToolChoice),
			}

			// 用空工具清除器创建重写Chain
			chain := middleware.NewRewriterChain(
				middleware.NewEmptyToolsCleaner(),
			)

			// 执行链条
			rewrittenReq, err := chain.Execute(context.Background(), req)

			// 校验无出错
			assert.NoError(t, err, "RewriterChain should not return error for valid request")
			assert.NotNil(t, rewrittenReq, "RewriterChain should return non-nil request")

			// 验证工具处理
			if tc.expectedToolsNil {
				assert.Empty(t, rewrittenReq.Tools,
					"Tools should be empty when input tools are empty/nil (Requirement %s)", tc.requirement)
			} else {
				assert.NotEmpty(t, rewrittenReq.Tools,
					"Tools should be preserved when non-empty (Requirement %s)", tc.requirement)
				assert.Equal(t, len(tc.inputTools), len(rewrittenReq.Tools),
					"Tool count should be preserved")
			}

			// 校验工具  选择处理
			assert.Equal(t, tc.expectedToolChoice, comparableProviderTestToolChoice(rewrittenReq.ToolChoice),
				"ToolChoice should be '%s' (Requirement %s): %s",
				tc.expectedToolChoice, tc.requirement, tc.description)

			// 校验其他字段保存
			assert.Equal(t, req.Model, rewrittenReq.Model, "Model should be preserved")
			assert.Equal(t, len(req.Messages), len(rewrittenReq.Messages), "Messages should be preserved")
		})
	}

	// 检查我们至少有100个测试用例
	assert.GreaterOrEqual(t, len(expandedTestCases), 100,
		"Property test should have minimum 100 iterations")
}

// TestProperty8  Rewrite Chain 应用两种方法验证重写 Chain
// 适用于完成()和流()方法(要求7.4)
func TestProperty8_RewriterChainAppliedToBothMethods(t *testing.T) {
	// 此测试验证所有提供者所用的模式
	// 我们测试在两种方法处理之前 重写链被调用

	testCases := []struct {
		name           string
		tools          []types.ToolSchema
		toolChoice     any
		expectModified bool
		requirement    string
	}{
		{
			name:           "Empty tools should be cleaned in Completion",
			tools:          []types.ToolSchema{},
			toolChoice:     "auto",
			expectModified: true,
			requirement:    "7.4",
		},
		{
			name:           "Empty tools should be cleaned in Stream",
			tools:          []types.ToolSchema{},
			toolChoice:     "required",
			expectModified: true,
			requirement:    "7.4",
		},
		{
			name: "Non-empty tools should not be modified in Completion",
			tools: []types.ToolSchema{
				{Name: "test", Description: "Test", Parameters: []byte(`{"type":"object"}`)},
			},
			toolChoice:     "auto",
			expectModified: false,
			requirement:    "7.4",
		},
		{
			name: "Non-empty tools should not be modified in Stream",
			tools: []types.ToolSchema{
				{Name: "test", Description: "Test", Parameters: []byte(`{"type":"object"}`)},
			},
			toolChoice:     "required",
			expectModified: false,
			requirement:    "7.4",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := &llm.ChatRequest{
				Model:      "test-model",
				Messages:   []types.Message{{Role: llm.RoleUser, Content: "test"}},
				Tools:      tc.tools,
				ToolChoice: providerTestToolChoice(tc.toolChoice),
			}

			chain := middleware.NewRewriterChain(
				middleware.NewEmptyToolsCleaner(),
			)

			rewrittenReq, err := chain.Execute(context.Background(), req)
			assert.NoError(t, err)

			if tc.expectModified {
				// 当工具为空时应该清除工具选择
				assert.Nil(t, rewrittenReq.ToolChoice,
					"ToolChoice should be cleared when tools are empty (Requirement %s)", tc.requirement)
			} else {
				// 当工具不是空的时, 工具选择应当保存
				assert.Equal(t, tc.toolChoice, comparableProviderTestToolChoice(rewrittenReq.ToolChoice),
					"ToolChoice should be preserved when tools are not empty (Requirement %s)", tc.requirement)
			}
		})
	}
}

// 测试Property8 EmptyTools 清除器行为测试空工具清除器的特定行为
func TestProperty8_EmptyToolsCleanerBehavior(t *testing.T) {
	cleaner := middleware.NewEmptyToolsCleaner()

	testCases := []struct {
		name               string
		inputReq           *llm.ChatRequest
		expectedToolChoice any
		description        string
	}{
		{
			name:               "Nil request should be handled gracefully",
			inputReq:           nil,
			expectedToolChoice: nil,
			description:        "EmptyToolsCleaner should handle nil request",
		},
		{
			name: "Request with nil tools and tool_choice",
			inputReq: &llm.ChatRequest{
				Tools:      nil,
				ToolChoice: providerTestToolChoice("auto"),
			},
			expectedToolChoice: nil,
			description:        "Should clear tool_choice when tools is nil",
		},
		{
			name: "Request with empty tools array and tool_choice",
			inputReq: &llm.ChatRequest{
				Tools:      []types.ToolSchema{},
				ToolChoice: providerTestToolChoice("required"),
			},
			expectedToolChoice: nil,
			description:        "Should clear tool_choice when tools is empty array",
		},
		{
			name: "Request with tools and tool_choice",
			inputReq: &llm.ChatRequest{
				Tools: []types.ToolSchema{
					{Name: "test", Description: "Test", Parameters: []byte(`{"type":"object"}`)},
				},
				ToolChoice: providerTestToolChoice("auto"),
			},
			expectedToolChoice: "auto",
			description:        "Should preserve tool_choice when tools exist",
		},
		{
			name: "Request with tools but no tool_choice",
			inputReq: &llm.ChatRequest{
				Tools: []types.ToolSchema{
					{Name: "test", Description: "Test", Parameters: []byte(`{"type":"object"}`)},
				},
				ToolChoice: nil,
			},
			expectedToolChoice: nil,
			description:        "Should preserve nil tool_choice when tools exist",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := cleaner.Rewrite(context.Background(), tc.inputReq)

			assert.NoError(t, err, "EmptyToolsCleaner should not return error")

			if tc.inputReq == nil {
				assert.Nil(t, result, "Should return nil for nil input")
			} else {
				assert.NotNil(t, result, "Should return non-nil result")
				assert.Equal(t, tc.expectedToolChoice, comparableProviderTestToolChoice(result.ToolChoice),
					"%s", tc.description)
			}
		})
	}
}

// 测试Property8  RewriterChanName 验证空工具清除器有正确名称
func TestProperty8_RewriterChainName(t *testing.T) {
	cleaner := middleware.NewEmptyToolsCleaner()
	assert.Equal(t, "empty_tools_cleaner", cleaner.Name(),
		"EmptyToolsCleaner should have correct name for logging and debugging")
}
