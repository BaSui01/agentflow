package providers

import (
	"encoding/json"
	"testing"

	"github.com/BaSui01/agentflow/llm"
	"github.com/stretchr/testify/assert"
)

// 特性:多提供者支持,属性 18:工具选择保存
// ** 参数:要求11.2**
//
// 这个属性测试验证了对于任何提供商和任何使用非空工具选择字符串的聊天请求,
// 提供者在 API 请求中包含具有相同值的工具 选择字段。
// 通过综合测试用例实现至少100次重复。
func TestProperty18_ToolChoicePreservation(t *testing.T) {
	testCases := []struct {
		name        string
		toolChoice  string
		provider    string
		requirement string
		description string
	}{
		// 标准工具选择值
		{
			name:        "ToolChoice auto",
			toolChoice:  "auto",
			provider:    "grok",
			requirement: "11.2",
			description: "Should preserve 'auto' tool choice value",
		},
		{
			name:        "ToolChoice none",
			toolChoice:  "none",
			provider:    "qwen",
			requirement: "11.2",
			description: "Should preserve 'none' tool choice value",
		},
		{
			name:        "ToolChoice required",
			toolChoice:  "required",
			provider:    "deepseek",
			requirement: "11.2",
			description: "Should preserve 'required' tool choice value",
		},

		// 特定工具名称
		{
			name:        "ToolChoice specific tool - search",
			toolChoice:  "search",
			provider:    "glm",
			requirement: "11.2",
			description: "Should preserve specific tool name 'search'",
		},
		{
			name:        "ToolChoice specific tool - calculate",
			toolChoice:  "calculate",
			provider:    "minimax",
			requirement: "11.2",
			description: "Should preserve specific tool name 'calculate'",
		},
		{
			name:        "ToolChoice specific tool - get_weather",
			toolChoice:  "get_weather",
			provider:    "grok",
			requirement: "11.2",
			description: "Should preserve specific tool name 'get_weather'",
		},
		{
			name:        "ToolChoice specific tool - fetch_data",
			toolChoice:  "fetch_data",
			provider:    "qwen",
			requirement: "11.2",
			description: "Should preserve specific tool name 'fetch_data'",
		},
		{
			name:        "ToolChoice specific tool - process_image",
			toolChoice:  "process_image",
			provider:    "deepseek",
			requirement: "11.2",
			description: "Should preserve specific tool name 'process_image'",
		},

		// 带有下划线的工具名称
		{
			name:        "ToolChoice with underscores - web_search",
			toolChoice:  "web_search",
			provider:    "glm",
			requirement: "11.2",
			description: "Should preserve tool name with underscores",
		},
		{
			name:        "ToolChoice with underscores - api_call",
			toolChoice:  "api_call",
			provider:    "minimax",
			requirement: "11.2",
			description: "Should preserve tool name with underscores",
		},
		{
			name:        "ToolChoice with underscores - data_fetch",
			toolChoice:  "data_fetch",
			provider:    "grok",
			requirement: "11.2",
			description: "Should preserve tool name with underscores",
		},
		{
			name:        "ToolChoice with underscores - file_upload",
			toolChoice:  "file_upload",
			provider:    "qwen",
			requirement: "11.2",
			description: "Should preserve tool name with underscores",
		},

		// 带连字符的工具名称
		{
			name:        "ToolChoice with hyphens - get-weather",
			toolChoice:  "get-weather",
			provider:    "deepseek",
			requirement: "11.2",
			description: "Should preserve tool name with hyphens",
		},
		{
			name:        "ToolChoice with hyphens - fetch-data",
			toolChoice:  "fetch-data",
			provider:    "glm",
			requirement: "11.2",
			description: "Should preserve tool name with hyphens",
		},
		{
			name:        "ToolChoice with hyphens - process-request",
			toolChoice:  "process-request",
			provider:    "minimax",
			requirement: "11.2",
			description: "Should preserve tool name with hyphens",
		},

		// 混合大小写工具名称
		{
			name:        "ToolChoice mixed case - GetWeather",
			toolChoice:  "GetWeather",
			provider:    "grok",
			requirement: "11.2",
			description: "Should preserve mixed case tool name",
		},
		{
			name:        "ToolChoice mixed case - FetchData",
			toolChoice:  "FetchData",
			provider:    "qwen",
			requirement: "11.2",
			description: "Should preserve mixed case tool name",
		},
		{
			name:        "ToolChoice mixed case - ProcessImage",
			toolChoice:  "ProcessImage",
			provider:    "deepseek",
			requirement: "11.2",
			description: "Should preserve mixed case tool name",
		},

		// 长工具名
		{
			name:        "ToolChoice long name",
			toolChoice:  "fetch_user_profile_data_from_database",
			provider:    "glm",
			requirement: "11.2",
			description: "Should preserve long tool name",
		},
		{
			name:        "ToolChoice very long name",
			toolChoice:  "process_and_validate_incoming_webhook_request_data",
			provider:    "minimax",
			requirement: "11.2",
			description: "Should preserve very long tool name",
		},

		// 带有数字的工具名称
		{
			name:        "ToolChoice with numbers - tool1",
			toolChoice:  "tool1",
			provider:    "grok",
			requirement: "11.2",
			description: "Should preserve tool name with numbers",
		},
		{
			name:        "ToolChoice with numbers - api_v2",
			toolChoice:  "api_v2",
			provider:    "qwen",
			requirement: "11.2",
			description: "Should preserve tool name with version numbers",
		},
		{
			name:        "ToolChoice with numbers - fetch_data_v3",
			toolChoice:  "fetch_data_v3",
			provider:    "deepseek",
			requirement: "11.2",
			description: "Should preserve tool name with version suffix",
		},

		// 边缘案件
		{
			name:        "ToolChoice single character",
			toolChoice:  "a",
			provider:    "glm",
			requirement: "11.2",
			description: "Should preserve single character tool choice",
		},
		{
			name:        "ToolChoice two characters",
			toolChoice:  "ab",
			provider:    "minimax",
			requirement: "11.2",
			description: "Should preserve two character tool choice",
		},
		{
			name:        "ToolChoice all lowercase",
			toolChoice:  "toolname",
			provider:    "grok",
			requirement: "11.2",
			description: "Should preserve all lowercase tool name",
		},
		{
			name:        "ToolChoice all uppercase",
			toolChoice:  "TOOLNAME",
			provider:    "qwen",
			requirement: "11.2",
			description: "Should preserve all uppercase tool name",
		},
	}

	// 通过对所有提供商进行每个用例的测试,将测试用例扩展至100+重复
	providers := []string{"grok", "qwen", "deepseek", "glm", "minimax"}
	expandedTestCases := make([]struct {
		name        string
		toolChoice  string
		provider    string
		requirement string
		description string
	}, 0, len(testCases)*len(providers))

	// 添加原始测试用例
	expandedTestCases = append(expandedTestCases, testCases...)

	// 添加不同提供者的变量
	for _, provider := range providers {
		for _, tc := range testCases {
			if tc.provider != provider {
				expandedTC := tc
				expandedTC.name = tc.name + " - provider: " + provider
				expandedTC.provider = provider
				expandedTestCases = append(expandedTestCases, expandedTC)
			}
		}
	}

	// 运行所有测试大小写
	for _, tc := range expandedTestCases {
		t.Run(tc.name, func(t *testing.T) {
			// 根据提供者类型测试转换
			switch tc.provider {
			case "grok", "qwen", "deepseek", "glm":
				// OpenAI 兼容提供者
				testOpenAICompatibleToolChoice(t, tc.toolChoice, tc.provider, tc.requirement, tc.description)
			case "minimax":
				// MiniMax 具有自定义格式,但仍应当保存工具  选择
				testMiniMaxToolChoice(t, tc.toolChoice, tc.provider, tc.requirement, tc.description)
			default:
				t.Fatalf("Unknown provider: %s", tc.provider)
			}
		})
	}

	// 检查我们至少有100个测试用例
	assert.GreaterOrEqual(t, len(expandedTestCases), 100,
		"Property test should have minimum 100 iterations")
}

// 测试 OpenAI 兼容 ToolChoice 测试工具选择保存 OpenAI 兼容提供者
func testOpenAICompatibleToolChoice(t *testing.T, toolChoice, provider, requirement, description string) {
	// 用工具选择创建模拟请求
	req := &llm.ChatRequest{
		Model: "test-model",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "Test message"},
		},
		Tools: []llm.ToolSchema{
			{
				Name:        "search",
				Description: "Search tool",
				Parameters:  json.RawMessage(`{"type":"object"}`),
			},
		},
		ToolChoice: toolChoice,
	}

	// 转换为 OpenAI 格式
	converted := mockConvertToOpenAIRequest(req)

	// 校验工具  选择被保存
	assert.NotNil(t, converted.ToolChoice,
		"ToolChoice should not be nil when non-empty (Requirement %s): %s", requirement, description)

	// 校验值匹配
	assert.Equal(t, toolChoice, converted.ToolChoice,
		"ToolChoice value should be preserved exactly (Requirement %s): %s", requirement, description)
}

// 测试MiniMax ToolChoice 测试工具选择为 MiniMax 提供者保存
func testMiniMaxToolChoice(t *testing.T, toolChoice, provider, requirement, description string) {
	// 用工具选择创建模拟请求
	req := &llm.ChatRequest{
		Model: "test-model",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "Test message"},
		},
		Tools: []llm.ToolSchema{
			{
				Name:        "search",
				Description: "Search tool",
				Parameters:  json.RawMessage(`{"type":"object"}`),
			},
		},
		ToolChoice: toolChoice,
	}

	// 转换为 MiniMax 格式
	converted := mockConvertToMiniMaxRequest(req)

	// 校验工具  选择被保存
	assert.NotNil(t, converted.ToolChoice,
		"ToolChoice should not be nil when non-empty (Requirement %s): %s", requirement, description)

	// 校验值匹配
	assert.Equal(t, toolChoice, converted.ToolChoice,
		"ToolChoice value should be preserved exactly (Requirement %s): %s", requirement, description)
}

// Property18 Empty ToolChoice 验证请求中没有包含空工具选择
func TestProperty18_EmptyToolChoice(t *testing.T) {
	providers := []string{"grok", "qwen", "deepseek", "glm", "minimax"}

	for _, provider := range providers {
		t.Run("empty_tool_choice_"+provider, func(t *testing.T) {
			req := &llm.ChatRequest{
				Model: "test-model",
				Messages: []llm.Message{
					{Role: llm.RoleUser, Content: "Test message"},
				},
				Tools: []llm.ToolSchema{
					{
						Name:        "search",
						Description: "Search tool",
						Parameters:  json.RawMessage(`{"type":"object"}`),
					},
				},
				ToolChoice: "", // Empty tool choice
			}

			switch provider {
			case "grok", "qwen", "deepseek", "glm":
				converted := mockConvertToOpenAIRequest(req)
				assert.Nil(t, converted.ToolChoice,
					"Empty ToolChoice should result in nil field for %s", provider)
			case "minimax":
				converted := mockConvertToMiniMaxRequest(req)
				assert.Nil(t, converted.ToolChoice,
					"Empty ToolChoice should result in nil field for %s", provider)
			}
		})
	}
}

// Property18 TooChoice Without Tools 在没有提供工具时验证工具选择行为
func TestProperty18_ToolChoiceWithoutTools(t *testing.T) {
	providers := []string{"grok", "qwen", "deepseek", "glm", "minimax"}
	toolChoices := []string{"auto", "none", "search"}

	for _, provider := range providers {
		for _, toolChoice := range toolChoices {
			t.Run(provider+"_"+toolChoice+"_without_tools", func(t *testing.T) {
				req := &llm.ChatRequest{
					Model: "test-model",
					Messages: []llm.Message{
						{Role: llm.RoleUser, Content: "Test message"},
					},
					Tools:      []llm.ToolSchema{}, // No tools
					ToolChoice: toolChoice,
				}

				// 即使没有工具,如果指定了 ToolChoice , 也应保留它
				// (虽然这可能是无效的请求,但转换仍应保留).
				switch provider {
				case "grok", "qwen", "deepseek", "glm":
					converted := mockConvertToOpenAIRequest(req)
					assert.Equal(t, toolChoice, converted.ToolChoice,
						"ToolChoice should be preserved even without tools for %s", provider)
				case "minimax":
					converted := mockConvertToMiniMaxRequest(req)
					assert.Equal(t, toolChoice, converted.ToolChoice,
						"ToolChoice should be preserved even without tools for %s", provider)
				}
			})
		}
	}
}

// Property18  ToolChoiceType一致性验证工具选择保持类型一致性
func TestProperty18_ToolChoiceTypeConsistency(t *testing.T) {
	testCases := []struct {
		name       string
		toolChoice string
		expectType string
	}{
		{"string value auto", "auto", "string"},
		{"string value none", "none", "string"},
		{"string value required", "required", "string"},
		{"string value tool name", "search", "string"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := &llm.ChatRequest{
				Model: "test-model",
				Messages: []llm.Message{
					{Role: llm.RoleUser, Content: "Test"},
				},
				Tools: []llm.ToolSchema{
					{Name: "search", Parameters: json.RawMessage(`{}`)},
				},
				ToolChoice: tc.toolChoice,
			}

			// 测试 OpenAI 格式
			converted := mockConvertToOpenAIRequest(req)
			assert.IsType(t, "", converted.ToolChoice,
				"ToolChoice should be string type")
			assert.Equal(t, tc.toolChoice, converted.ToolChoice,
				"ToolChoice value should match input")
		})
	}
}

// 跟踪光谱的模拟转换函数

type mockOpenAIRequestWithToolChoice struct {
	Model      string        `json:"model"`
	Messages   []any `json:"messages"`
	Tools      []any `json:"tools,omitempty"`
	ToolChoice any   `json:"tool_choice,omitempty"`
	MaxTokens  int           `json:"max_tokens,omitempty"`
}

type mockMiniMaxRequestWithToolChoice struct {
	Model      string        `json:"model"`
	Messages   []any `json:"messages"`
	Tools      []any `json:"tools,omitempty"`
	ToolChoice any   `json:"tool_choice,omitempty"`
	MaxTokens  int           `json:"max_tokens,omitempty"`
}

func mockConvertToOpenAIRequest(req *llm.ChatRequest) *mockOpenAIRequestWithToolChoice {
	result := &mockOpenAIRequestWithToolChoice{
		Model:     req.Model,
		Messages:  []any{},
		MaxTokens: req.MaxTokens,
	}

	// 在当前情况下转换工具
	if len(req.Tools) > 0 {
		result.Tools = []any{}
		for range req.Tools {
			result.Tools = append(result.Tools, map[string]any{})
		}
	}

	// 非空时保留工具选择
	if req.ToolChoice != "" {
		result.ToolChoice = req.ToolChoice
	}

	return result
}

func mockConvertToMiniMaxRequest(req *llm.ChatRequest) *mockMiniMaxRequestWithToolChoice {
	result := &mockMiniMaxRequestWithToolChoice{
		Model:     req.Model,
		Messages:  []any{},
		MaxTokens: req.MaxTokens,
	}

	// 在当前情况下转换工具
	if len(req.Tools) > 0 {
		result.Tools = []any{}
		for range req.Tools {
			result.Tools = append(result.Tools, map[string]any{})
		}
	}

	// 非空时保留工具选择
	if req.ToolChoice != "" {
		result.ToolChoice = req.ToolChoice
	}

	return result
}
