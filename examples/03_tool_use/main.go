package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/BaSui01/agentflow/llm/providers/openai"
	"github.com/BaSui01/agentflow/llm/tools"
	"go.uber.org/zap"
)

// Tool Use / Function Calling 示例
//
// 本示例展示 AgentFlow 框架的核心功能：Tool（工具）调用。
// 流程：
//   1. 定义工具函数和 JSON Schema
//   2. 注册工具到 ToolRegistry
//   3. 发送带 Tools 的 ChatRequest 给 LLM
//   4. LLM 返回 ToolCall，由 ToolExecutor 执行
//   5. 将工具结果回传 LLM，获取最终回答

// ========== 1. 定义工具函数 ==========

// getWeather 模拟获取天气信息的工具函数
// 签名必须符合 tools.ToolFunc: func(ctx, json.RawMessage) (json.RawMessage, error)
func getWeather(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	// 解析参数
	var params struct {
		Location string `json:"location"`
		Unit     string `json:"unit,omitempty"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	if params.Unit == "" {
		params.Unit = "celsius"
	}

	// 模拟天气数据（实际项目中调用真实天气 API）
	weather := map[string]any{
		"location":    params.Location,
		"temperature": 22,
		"unit":        params.Unit,
		"condition":   "sunny",
		"humidity":    65,
	}

	return json.Marshal(weather)
}

// calculate 模拟一个简单的计算器工具
func calculate(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		Expression string `json:"expression"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	// 简单演示：仅返回表达式描述（实际项目中可接入计算引擎）
	result := map[string]any{
		"expression": params.Expression,
		"result":     42,
		"note":       "This is a demo calculator",
	}

	return json.Marshal(result)
}

// ========== 2. 定义工具的 JSON Schema ==========

// weatherSchema 描述 get_weather 工具的参数格式，供 LLM 理解如何调用
var weatherSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"location": {
			"type": "string",
			"description": "The city name, e.g. Beijing, Tokyo, New York"
		},
		"unit": {
			"type": "string",
			"enum": ["celsius", "fahrenheit"],
			"description": "Temperature unit, defaults to celsius"
		}
	},
	"required": ["location"]
}`)

var calculateSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"expression": {
			"type": "string",
			"description": "The math expression to evaluate, e.g. 2+2, 100*3.14"
		}
	},
	"required": ["expression"]
}`)

func main() {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	fmt.Println("=== AgentFlow Tool Use / Function Calling 示例 ===")

	// ========== 3. 从环境变量读取 API Key ==========
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("请设置环境变量 OPENAI_API_KEY，例如: export OPENAI_API_KEY=sk-xxx")
	}

	// ========== 4. 创建 Provider ==========
	cfg := providers.OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			APIKey:  apiKey,
			BaseURL: "https://api.openai.com/v1",
			Model:   "gpt-3.5-turbo",
		},
	}
	provider := openai.NewOpenAIProvider(cfg, logger)

	// ========== 5. 注册工具到 Registry ==========
	registry := tools.NewDefaultRegistry(logger)

	// 注册 get_weather 工具
	if err := registry.Register("get_weather", getWeather, tools.ToolMetadata{
		Schema: llm.ToolSchema{
			Name:        "get_weather",
			Description: "Get the current weather for a given location",
			Parameters:  weatherSchema,
		},
		Description: "查询指定城市的当前天气信息",
	}); err != nil {
		log.Fatalf("注册 get_weather 失败: %v", err)
	}

	// 注册 calculate 工具
	if err := registry.Register("calculate", calculate, tools.ToolMetadata{
		Schema: llm.ToolSchema{
			Name:        "calculate",
			Description: "Evaluate a math expression",
			Parameters:  calculateSchema,
		},
		Description: "计算数学表达式",
	}); err != nil {
		log.Fatalf("注册 calculate 失败: %v", err)
	}

	fmt.Printf("已注册 %d 个工具\n", len(registry.List()))

	// ========== 6. 创建 ToolExecutor ==========
	executor := tools.NewDefaultExecutor(registry, logger)

	// ========== 7. 构建带 Tools 的 ChatRequest ==========
	ctx := context.Background()
	messages := []llm.Message{
		{Role: llm.RoleUser, Content: "What's the weather like in Beijing today?"},
	}

	req := &llm.ChatRequest{
		Model:       "gpt-3.5-turbo",
		Messages:    messages,
		Tools:       registry.List(), // 将所有已注册工具的 Schema 传给 LLM
		ToolChoice:  "auto",          // 让 LLM 自动决定是否调用工具
		MaxTokens:   500,
		Temperature: 0.7,
	}

	// ========== 8. Tool Calling 循环 ==========
	// LLM 可能返回 ToolCall 而非直接回答，需要循环处理
	fmt.Println("\n--- 开始对话 ---")
	fmt.Printf("User: %s\n", messages[0].Content)

	for i := 0; i < 5; i++ { // 最多 5 轮工具调用，防止无限循环
		resp, err := provider.Completion(ctx, req)
		if err != nil {
			log.Fatalf("LLM 调用失败: %v", err)
		}

		assistantMsg := resp.Choices[0].Message

		// 如果 LLM 没有请求工具调用，说明已经生成最终回答
		if len(assistantMsg.ToolCalls) == 0 {
			fmt.Printf("Assistant: %s\n", assistantMsg.Content)
			break
		}

		// LLM 请求了工具调用，执行它们
		fmt.Printf("\n[第 %d 轮] LLM 请求调用 %d 个工具:\n", i+1, len(assistantMsg.ToolCalls))
		for _, tc := range assistantMsg.ToolCalls {
			fmt.Printf("  → %s(%s)\n", tc.Name, string(tc.Arguments))
		}

		// 将 assistant 消息（含 ToolCalls）加入对话历史
		req.Messages = append(req.Messages, assistantMsg)

		// 执行所有工具调用
		results := executor.Execute(ctx, assistantMsg.ToolCalls)

		// 将每个工具结果作为 tool 消息加入对话历史
		for _, result := range results {
			fmt.Printf("  ← %s 返回: %s\n", result.Name, string(result.Result))
			req.Messages = append(req.Messages, llm.Message{
				Role:       llm.RoleTool,
				Content:    string(result.Result),
				Name:       result.Name,
				ToolCallID: result.ToolCallID,
			})
		}
	}

	fmt.Println("--- 对话结束 ---")
	fmt.Printf("总消息数: %d\n", len(req.Messages))
}
