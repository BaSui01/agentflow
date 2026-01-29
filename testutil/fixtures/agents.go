// =============================================================================
// 📦 测试数据工厂 - Agent 测试数据
// =============================================================================
// 提供预定义的 Agent 配置和状态，用于测试
// =============================================================================
package fixtures

import (
	"encoding/json"
	"time"

	"github.com/BaSui01/agentflow/types"
)

// =============================================================================
// 🤖 Agent 配置工厂
// =============================================================================

// DefaultAgentConfig 返回默认的 Agent 配置
func DefaultAgentConfig() types.AgentConfig {
	return types.AgentConfig{
		Core: types.CoreConfig{
			ID:          "test-agent-001",
			Name:        "test-agent",
			Type:        "assistant",
			Description: "Test agent for unit tests",
		},
		LLM: types.LLMConfig{
			Model:       "gpt-4",
			Provider:    "openai",
			MaxTokens:   4096,
			Temperature: 0.7,
		},
		Features: types.FeaturesConfig{
			Memory: &types.MemoryConfig{
				Enabled:          true,
				ShortTermTTL:     time.Hour,
				MaxShortTermSize: 100,
				EnableLongTerm:   true,
			},
			Guardrails: &types.GuardrailsConfig{
				Enabled:            true,
				MaxInputLength:     10000,
				PIIDetection:       true,
				InjectionDetection: true,
			},
		},
		Metadata: map[string]string{
			"environment": "test",
		},
	}
}

// MinimalAgentConfig 返回最小化的 Agent 配置
func MinimalAgentConfig() types.AgentConfig {
	return types.AgentConfig{
		Core: types.CoreConfig{
			ID:   "minimal-agent-001",
			Name: "minimal-agent",
			Type: "assistant",
		},
		LLM: types.LLMConfig{
			Model:       "gpt-3.5-turbo",
			MaxTokens:   1024,
			Temperature: 0.5,
		},
	}
}

// StreamingAgentConfig 返回启用流式输出的 Agent 配置
func StreamingAgentConfig() types.AgentConfig {
	cfg := DefaultAgentConfig()
	cfg.Core.ID = "streaming-agent-001"
	cfg.Core.Name = "streaming-agent"
	return cfg
}

// HighIterationAgentConfig 返回高迭代次数的 Agent 配置
func HighIterationAgentConfig() types.AgentConfig {
	cfg := DefaultAgentConfig()
	cfg.Core.ID = "high-iteration-agent-001"
	cfg.Core.Name = "high-iteration-agent"
	cfg.Features.Reflection = &types.ReflectionConfig{
		Enabled:       true,
		MaxIterations: 50,
		MinQuality:    0.5,
	}
	return cfg
}

// LowTemperatureAgentConfig 返回低温度（更确定性）的 Agent 配置
func LowTemperatureAgentConfig() types.AgentConfig {
	cfg := DefaultAgentConfig()
	cfg.Core.ID = "deterministic-agent-001"
	cfg.Core.Name = "deterministic-agent"
	cfg.LLM.Temperature = 0.0
	return cfg
}

// HighTemperatureAgentConfig 返回高温度（更创造性）的 Agent 配置
func HighTemperatureAgentConfig() types.AgentConfig {
	cfg := DefaultAgentConfig()
	cfg.Core.ID = "creative-agent-001"
	cfg.Core.Name = "creative-agent"
	cfg.LLM.Temperature = 1.5
	return cfg
}

// ReflectionEnabledConfig 返回启用反思功能的配置
func ReflectionEnabledConfig() types.AgentConfig {
	cfg := DefaultAgentConfig()
	cfg.Features.Reflection = types.DefaultReflectionConfig()
	return cfg
}

// ToolSelectionEnabledConfig 返回启用工具选择的配置
func ToolSelectionEnabledConfig() types.AgentConfig {
	cfg := DefaultAgentConfig()
	cfg.Features.ToolSelection = types.DefaultToolSelectionConfig()
	return cfg
}

// FullFeaturedConfig 返回启用所有功能的配置
func FullFeaturedConfig() types.AgentConfig {
	return types.AgentConfig{
		Core: types.CoreConfig{
			ID:          "full-featured-agent-001",
			Name:        "full-featured-agent",
			Type:        "assistant",
			Description: "Agent with all features enabled",
		},
		LLM: types.LLMConfig{
			Model:       "gpt-4",
			Provider:    "openai",
			MaxTokens:   4096,
			Temperature: 0.7,
		},
		Features: types.FeaturesConfig{
			Reflection:     types.DefaultReflectionConfig(),
			ToolSelection:  types.DefaultToolSelectionConfig(),
			PromptEnhancer: types.DefaultPromptEnhancerConfig(),
			Guardrails:     types.DefaultGuardrailsConfig(),
			Memory:         types.DefaultMemoryConfig(),
		},
		Extensions: types.ExtensionsConfig{
			Observability: types.DefaultObservabilityConfig(),
		},
	}
}

// =============================================================================
// 💬 消息工厂
// =============================================================================

// UserMessage 创建用户消息
func UserMessage(content string) types.Message {
	return types.Message{
		Role:    types.RoleUser,
		Content: content,
	}
}

// AssistantMessage 创建助手消息
func AssistantMessage(content string) types.Message {
	return types.Message{
		Role:    types.RoleAssistant,
		Content: content,
	}
}

// SystemMessage 创建系统消息
func SystemMessage(content string) types.Message {
	return types.Message{
		Role:    types.RoleSystem,
		Content: content,
	}
}

// ToolMessage 创建工具消息
func ToolMessage(toolCallID, content string) types.Message {
	return types.Message{
		Role:       types.RoleTool,
		Content:    content,
		ToolCallID: toolCallID,
	}
}

// AssistantMessageWithToolCalls 创建带工具调用的助手消息
func AssistantMessageWithToolCalls(content string, toolCalls []types.ToolCall) types.Message {
	return types.Message{
		Role:      types.RoleAssistant,
		Content:   content,
		ToolCalls: toolCalls,
	}
}

// =============================================================================
// 📝 对话历史工厂
// =============================================================================

// SimpleConversation 返回简单的对话历史
func SimpleConversation() []types.Message {
	return []types.Message{
		UserMessage("Hello!"),
		AssistantMessage("Hi there! How can I help you today?"),
		UserMessage("What's the weather like?"),
		AssistantMessage("I don't have access to real-time weather data, but I can help you find weather information if you tell me your location."),
	}
}

// ConversationWithToolCalls 返回包含工具调用的对话历史
func ConversationWithToolCalls() []types.Message {
	return []types.Message{
		UserMessage("What's 2 + 2?"),
		AssistantMessageWithToolCalls("Let me calculate that for you.", []types.ToolCall{
			{
				ID:        "call_123",
				Name:      "calculator",
				Arguments: json.RawMessage(`{"a": 2, "b": 2, "op": "add"}`),
			},
		}),
		ToolMessage("call_123", "4"),
		AssistantMessage("2 + 2 equals 4."),
	}
}

// LongConversation 返回较长的对话历史
func LongConversation(turns int) []types.Message {
	messages := make([]types.Message, 0, turns*2)
	for i := 0; i < turns; i++ {
		messages = append(messages,
			UserMessage("This is user message number "+string(rune('0'+i%10))),
			AssistantMessage("This is assistant response number "+string(rune('0'+i%10))),
		)
	}
	return messages
}

// =============================================================================
// 🔧 工具定义工厂
// =============================================================================

// CalculatorToolSchema 返回计算器工具定义
func CalculatorToolSchema() types.ToolSchema {
	params, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"a": map[string]any{
				"type":        "number",
				"description": "First operand",
			},
			"b": map[string]any{
				"type":        "number",
				"description": "Second operand",
			},
			"op": map[string]any{
				"type":        "string",
				"description": "Operation: add, sub, mul, div",
				"enum":        []string{"add", "sub", "mul", "div"},
			},
		},
		"required": []string{"a", "b", "op"},
	})

	return types.ToolSchema{
		Name:        "calculator",
		Description: "Perform basic arithmetic operations",
		Parameters:  params,
	}
}

// SearchToolSchema 返回搜索工具定义
func SearchToolSchema() types.ToolSchema {
	params, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "Search query",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "Maximum number of results",
				"default":     10,
			},
		},
		"required": []string{"query"},
	})

	return types.ToolSchema{
		Name:        "search",
		Description: "Search for information",
		Parameters:  params,
	}
}

// WeatherToolSchema 返回天气工具定义
func WeatherToolSchema() types.ToolSchema {
	params, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"location": map[string]any{
				"type":        "string",
				"description": "City name or coordinates",
			},
			"unit": map[string]any{
				"type":        "string",
				"description": "Temperature unit",
				"enum":        []string{"celsius", "fahrenheit"},
				"default":     "celsius",
			},
		},
		"required": []string{"location"},
	})

	return types.ToolSchema{
		Name:        "get_weather",
		Description: "Get current weather for a location",
		Parameters:  params,
	}
}

// DefaultToolSet 返回默认的工具集
func DefaultToolSet() []types.ToolSchema {
	return []types.ToolSchema{
		CalculatorToolSchema(),
		SearchToolSchema(),
		WeatherToolSchema(),
	}
}

// =============================================================================
// 📞 工具调用工厂
// =============================================================================

// CalculatorToolCall 创建计算器工具调用
func CalculatorToolCall(id string, a, b float64, op string) types.ToolCall {
	args, _ := json.Marshal(map[string]any{
		"a":  a,
		"b":  b,
		"op": op,
	})
	return types.ToolCall{
		ID:        id,
		Name:      "calculator",
		Arguments: args,
	}
}

// SearchToolCall 创建搜索工具调用
func SearchToolCall(id, query string, limit int) types.ToolCall {
	args, _ := json.Marshal(map[string]any{
		"query": query,
		"limit": limit,
	})
	return types.ToolCall{
		ID:        id,
		Name:      "search",
		Arguments: args,
	}
}

// WeatherToolCall 创建天气工具调用
func WeatherToolCall(id, location, unit string) types.ToolCall {
	args, _ := json.Marshal(map[string]any{
		"location": location,
		"unit":     unit,
	})
	return types.ToolCall{
		ID:        id,
		Name:      "get_weather",
		Arguments: args,
	}
}
