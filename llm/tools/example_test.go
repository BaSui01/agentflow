package tools_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	llmpkg "github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/tools"
	"go.uber.org/zap"
)

// 示例：定义一个简单的 GetWeather 工具
func Example_getWeatherTool() {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	// 1. 创建工具注册表
	registry := tools.NewDefaultRegistry(logger)

	// 2. 定义 GetWeather 工具函数
	getWeatherFunc := func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
		// 解析参数
		var params struct {
			Location string `json:"location"`
			Unit     string `json:"unit,omitempty"`
		}
		if err := json.Unmarshal(args, &params); err != nil {
			return nil, fmt.Errorf("invalid parameters: %w", err)
		}

		// 模拟获取天气
		weather := map[string]any{
			"location":    params.Location,
			"temperature": 22,
			"unit":        "celsius",
			"condition":   "sunny",
			"humidity":    60,
		}

		if params.Unit == "fahrenheit" {
			weather["temperature"] = 72
			weather["unit"] = "fahrenheit"
		}

		result, _ := json.Marshal(weather)
		return result, nil
	}

	// 3. 定义工具元数据
	metadata := tools.ToolMetadata{
		Schema: llmpkg.ToolSchema{
			Name:        "get_weather",
			Description: "Get the current weather for a location",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"location": {
						"type": "string",
						"description": "The city and state, e.g. San Francisco, CA"
					},
					"unit": {
						"type": "string",
						"enum": ["celsius", "fahrenheit"],
						"description": "The temperature unit"
					}
				},
				"required": ["location"]
			}`),
		},
		Timeout: 5 * time.Second,
		RateLimit: &tools.RateLimitConfig{
			MaxCalls: 10,
			Window:   time.Minute,
		},
	}

	// 4. 注册工具
	if err := registry.Register("get_weather", getWeatherFunc, metadata); err != nil {
		logger.Fatal("failed to register tool", zap.Error(err))
	}

	// 5. 创建工具执行器
	executor := tools.NewDefaultExecutor(registry, logger)

	// 6. 模拟 LLM 返回的 ToolCalls
	toolCalls := []llmpkg.ToolCall{
		{
			ID:   "call_123",
			Name: "get_weather",
			Arguments: json.RawMessage(`{
				"location": "San Francisco, CA",
				"unit": "fahrenheit"
			}`),
		},
	}

	// 7. 执行工具调用
	ctx := context.Background()
	results := executor.Execute(ctx, toolCalls)

	// 8. 打印结果
	for _, result := range results {
		fmt.Printf("Tool: %s\n", result.Name)
		if result.Error != "" {
			fmt.Printf("Error: %s\n", result.Error)
		} else {
			fmt.Printf("Result: %s\n", string(result.Result))
		}
	}

	// 输出 :
	// 工具: get weather
	// 结果:{"条件":"生","湿":"地":"旧金山,CA""温":72"单位":"平"]
}

// 示例：ReAct 循环集成（伪代码，需要真实的 Provider）
func Example_reActLoop() {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	// 1. 创建工具注册表并注册工具
	registry := tools.NewDefaultRegistry(logger)
	// ... 注册工具（同上）

	// 2. 创建工具执行器
	toolExecutor := tools.NewDefaultExecutor(registry, logger)

	// 3. 假设我们有一个 LLM Provider（这里需要真实实现）
	// 提供者:= (OpenAI, Claude等)

	// 4. 创建 ReAct 执行器
	config := tools.ReActConfig{
		MaxIterations: 5,
		StopOnError:   false,
	}
	// 反应执行器:=工具. NewReAct执行器(提供器,工具执行器,配置器,日志)

	// 5. 准备请求
	req := &llmpkg.ChatRequest{
		TraceID: "trace_123",
		Model:   "gpt-4",
		Messages: []llmpkg.Message{
			{
				Role:    llmpkg.RoleSystem,
				Content: "You are a helpful assistant that can get weather information.",
			},
			{
				Role:    llmpkg.RoleUser,
				Content: "What's the weather like in San Francisco?",
			},
		},
		Tools: registry.List(), // 传递所有可用工具
	}

	// 6. 执行 ReAct 循环
	// resp, 步骤, 错误 : = 反应Executor.Execute(context.Background (, req))
	// 如果错误 ! = 无 {
	//     logger. Error ("ReAct执行失败", zap. Error( err)) :
	//     返回时
	// }

	// 7. 打印结果
	// fmt.Printf ("最后反应: %s\n", resp.Choices [0]. 传言. 内容)
	// fmt.Printf ("总步数:%d\n", len( 步数))

	_ = toolExecutor // 避免未使用变量错误
	_ = config       // 避免未使用变量错误
	_ = req          // 避免未使用变量错误
}

// TestToolRegistry 测试工具注册表
func TestToolRegistry(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	registry := tools.NewDefaultRegistry(logger)

	// 测试注册工具
	testFunc := func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{"success": true}`), nil
	}

	metadata := tools.ToolMetadata{
		Schema: llmpkg.ToolSchema{
			Name:        "test_tool",
			Description: "A test tool",
		},
		Timeout: 10 * time.Second,
	}

	err := registry.Register("test_tool", testFunc, metadata)
	if err != nil {
		t.Fatalf("failed to register tool: %v", err)
	}

	// 测试查询工具
	if !registry.Has("test_tool") {
		t.Error("tool should be registered")
	}

	// 测试列出工具
	schemas := registry.List()
	if len(schemas) != 1 {
		t.Errorf("expected 1 schema, got %d", len(schemas))
	}

	// 测试注销工具
	err = registry.Unregister("test_tool")
	if err != nil {
		t.Fatalf("failed to unregister tool: %v", err)
	}

	if registry.Has("test_tool") {
		t.Error("tool should be unregistered")
	}
}

// TestToolExecutor 测试工具执行器
func TestToolExecutor(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	registry := tools.NewDefaultRegistry(logger)
	executor := tools.NewDefaultExecutor(registry, logger)

	// 注册测试工具
	testFunc := func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
		var input struct {
			Value int `json:"value"`
		}
		json.Unmarshal(args, &input)

		result := map[string]int{"doubled": input.Value * 2}
		return json.Marshal(result)
	}

	metadata := tools.ToolMetadata{
		Schema: llmpkg.ToolSchema{
			Name:        "double",
			Description: "Double a number",
		},
	}

	registry.Register("double", testFunc, metadata)

	// 执行工具
	call := llmpkg.ToolCall{
		ID:        "call_1",
		Name:      "double",
		Arguments: json.RawMessage(`{"value": 5}`),
	}

	result := executor.ExecuteOne(context.Background(), call)

	if result.Error != "" {
		t.Errorf("unexpected error: %s", result.Error)
	}

	var output struct {
		Doubled int `json:"doubled"`
	}
	json.Unmarshal(result.Result, &output)

	if output.Doubled != 10 {
		t.Errorf("expected 10, got %d", output.Doubled)
	}
}

// TestRateLimit 测试速率限制
func TestRateLimit(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	registry := tools.NewDefaultRegistry(logger)
	executor := tools.NewDefaultExecutor(registry, logger)

	// 注册带速率限制的工具
	testFunc := func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{"ok": true}`), nil
	}

	metadata := tools.ToolMetadata{
		Schema: llmpkg.ToolSchema{
			Name: "limited_tool",
		},
		RateLimit: &tools.RateLimitConfig{
			MaxCalls: 2,
			Window:   time.Second,
		},
	}

	registry.Register("limited_tool", testFunc, metadata)

	// 执行工具调用
	call := llmpkg.ToolCall{
		ID:        "call_1",
		Name:      "limited_tool",
		Arguments: json.RawMessage(`{}`),
	}

	// 前两次应该成功
	for i := 0; i < 2; i++ {
		result := executor.ExecuteOne(context.Background(), call)
		if result.Error != "" {
			t.Errorf("call %d should succeed, got error: %s", i+1, result.Error)
		}
	}

	// 第三次应该触发速率限制
	result := executor.ExecuteOne(context.Background(), call)
	if result.Error == "" || result.Error[:10] != "rate limit" {
		t.Errorf("call 3 should be rate limited, got: %s", result.Error)
	}
}

// TestToolTimeout 测试超时
func TestToolTimeout(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	registry := tools.NewDefaultRegistry(logger)
	executor := tools.NewDefaultExecutor(registry, logger)

	// 注册一个会超时的工具
	slowFunc := func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
		time.Sleep(2 * time.Second) // 模拟慢操作
		return json.RawMessage(`{"done": true}`), nil
	}

	metadata := tools.ToolMetadata{
		Schema: llmpkg.ToolSchema{
			Name: "slow_tool",
		},
		Timeout: 100 * time.Millisecond, // 很短的超时
	}

	registry.Register("slow_tool", slowFunc, metadata)

	// 执行工具
	call := llmpkg.ToolCall{
		ID:   "call_1",
		Name: "slow_tool",
	}

	result := executor.ExecuteOne(context.Background(), call)

	// 应该超时
	if result.Error == "" || result.Error[:9] != "execution" {
		t.Errorf("expected timeout error, got: %s", result.Error)
	}
}
