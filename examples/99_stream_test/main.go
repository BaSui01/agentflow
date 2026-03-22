// =============================================================================
// 🧪 AgentFlow 全栈测试 — 流式 + 工具调用 + ReAct 循环 + 循环智能体
// =============================================================================
// 测试目标：
// 1. 流式解析（Stream SSE）           ← 已验证 ✅
// 2. 同步调用（Completion）            ← 已验证 ✅
// 3. 工具调用（Tool Call）解析         ← 本次新增
// 4. ReAct 循环（LLM→Tool→LLM→...）  ← 本次新增
// 5. 循环智能体（Loop Agent 多轮迭代） ← 本次新增
// =============================================================================

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/capabilities/tools"
	"github.com/BaSui01/agentflow/llm/providers/openaicompat"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// =============================================================================
// 配置
// =============================================================================

const (
	apiKey  = "sk-W2WaPZdpC6iOoP6bMY8P7XwRfIZua7c2tkTuQ0DIPGPZKQwJ"
	baseURL = "https://ai.xoooox.xyz"
	model   = "glm-5"
)

// =============================================================================
// 🛠️ 模拟工具函数（供 ReAct 循环调用）
// =============================================================================

// getWeather 模拟天气查询工具
func getWeather(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		City string `json:"city"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("参数解析失败: %w", err)
	}
	city := params.City
	if city == "" {
		city = "未知城市"
	}

	result := map[string]any{
		"city":        city,
		"temperature": 22,
		"condition":   "晴朗",
		"humidity":    65,
		"wind":        "东南风3级",
	}
	return json.Marshal(result)
}

// calculate 模拟计算器工具
func calculate(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		Expression string `json:"expression"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("参数解析失败: %w", err)
	}

	// 简单计算器：支持基础运算
	result := evaluateSimple(params.Expression)
	return json.Marshal(map[string]any{
		"expression": params.Expression,
		"result":     result,
	})
}

// searchKnowledge 模拟知识搜索工具
func searchKnowledge(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("参数解析失败: %w", err)
	}

	results := []map[string]string{
		{"title": "Go 语言并发编程", "snippet": "Go 通过 goroutine 和 channel 实现轻量级并发..."},
		{"title": "Rust vs Go 性能对比", "snippet": "在内存安全和并发场景下，两者各有优势..."},
	}
	return json.Marshal(map[string]any{
		"query":   params.Query,
		"results": results,
		"count":   len(results),
	})
}

// evaluateSimple 超简单表达式求值（仅支持两数运算）
func evaluateSimple(expr string) string {
	expr = strings.TrimSpace(expr)
	for _, op := range []string{"+", "-", "*", "/"} {
		if idx := strings.LastIndex(expr, op); idx > 0 {
			left, err1 := strconv.ParseFloat(strings.TrimSpace(expr[:idx]), 64)
			right, err2 := strconv.ParseFloat(strings.TrimSpace(expr[idx+1:]), 64)
			if err1 != nil || err2 != nil {
				continue
			}
			var result float64
			switch op {
			case "+":
				result = left + right
			case "-":
				result = left - right
			case "*":
				result = left * right
			case "/":
				if right == 0 {
					return "错误: 除以零"
				}
				result = left / right
			}
			if result == math.Trunc(result) {
				return fmt.Sprintf("%.0f", result)
			}
			return fmt.Sprintf("%.2f", result)
		}
	}
	return "无法计算: " + expr
}

// =============================================================================
// 入口
// =============================================================================

func main() {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	provider := openaicompat.New(openaicompat.Config{
		ProviderName:  "xoooox-glm",
		APIKey:        apiKey,
		BaseURL:       baseURL,
		DefaultModel:  model,
		FallbackModel: model,
		Timeout:       120 * time.Second,
	}, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║  🚀 AgentFlow 全栈测试 — Tool Call + ReAct + Loop Agent    ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	fmt.Println()

	// 测试 1: 带工具的 Completion（检测 LLM 是否返回 tool_calls）
	test1ToolCallCompletion(ctx, provider, logger)

	// 测试 2: ReAct 循环（LLM → Tool → LLM → ... → 最终回答）
	test2ReActLoop(ctx, provider, logger)

	// 测试 3: ReAct 流式循环
	test3ReActStream(ctx, provider, logger)

	// 测试 4: 循环智能体（手动实现 Loop Agent 模式）
	test4LoopAgent(ctx, provider, logger)

	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║  ✅ 全部测试完成                                            ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
}

// =============================================================================
// 测试 1: 带工具定义的 Completion — 检测 LLM 是否返回 tool_calls
// =============================================================================

func test1ToolCallCompletion(ctx context.Context, provider llm.Provider, logger *zap.Logger) {
	fmt.Println("🔧 [测试1] 带工具定义的 Completion — 检测 tool_calls 返回")
	fmt.Println(strings.Repeat("─", 60))

	// 定义工具 Schema（OpenAI 函数调用格式）
	toolSchemas := []types.ToolSchema{
		{
			Name:        "get_weather",
			Description: "查询指定城市的天气信息",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"city": {
						"type": "string",
						"description": "城市名称，如北京、上海"
					}
				},
				"required": ["city"]
			}`),
		},
		{
			Name:        "calculate",
			Description: "计算数学表达式",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"expression": {
						"type": "string",
						"description": "数学表达式，如 2+3、100*0.85"
					}
				},
				"required": ["expression"]
			}`),
		},
	}

	req := &llm.ChatRequest{
		Model: model,
		Messages: []types.Message{
			{Role: llm.RoleSystem, Content: "你是一个智能助手，请使用提供的工具来回答问题。需要查天气时调用 get_weather，需要计算时调用 calculate。"},
			{Role: llm.RoleUser, Content: "北京今天天气怎么样？另外帮我算一下 256 * 3.14"},
		},
		Tools:       toolSchemas,
		MaxTokens:   1024,
		Temperature: 0.1, // 低温度让模型更倾向调用工具
	}

	start := time.Now()
	resp, err := provider.Completion(ctx, req)
	elapsed := time.Since(start)

	if err != nil {
		fmt.Printf("  ❌ 调用失败: %v\n\n", err)
		return
	}

	choice := resp.Choices[0]
	fmt.Printf("  ⏱️  耗时: %v\n", elapsed)
	fmt.Printf("  🏷️  FinishReason: %s\n", choice.FinishReason)

	if len(choice.Message.ToolCalls) > 0 {
		fmt.Printf("  ✅ 检测到 %d 个工具调用！\n", len(choice.Message.ToolCalls))
		for i, tc := range choice.Message.ToolCalls {
			fmt.Printf("    [%d] ID=%s 工具=%s 参数=%s\n", i+1, tc.ID, tc.Name, string(tc.Arguments))
		}
	} else {
		fmt.Printf("  ⚠️  模型未返回 tool_calls（直接回复了文本）\n")
		if choice.Message.Content != "" {
			fmt.Printf("  📝 回复: %s\n", truncate(choice.Message.Content, 200))
		}
	}

	// 打印推理内容
	if choice.Message.ReasoningContent != nil {
		fmt.Printf("  💭 推理: %s\n", truncate(*choice.Message.ReasoningContent, 200))
	}

	fmt.Printf("  📊 Token: prompt=%d completion=%d total=%d\n",
		resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.TotalTokens)
	fmt.Println()
}

// =============================================================================
// 测试 2: ReAct 循环 — LLM → Tool → LLM → ... → 最终回答
// =============================================================================

func test2ReActLoop(ctx context.Context, provider llm.Provider, logger *zap.Logger) {
	fmt.Println("🔄 [测试2] ReAct 循环 — LLM → Tool → LLM → 最终回答")
	fmt.Println(strings.Repeat("─", 60))

	// 注册工具
	registry := tools.NewDefaultRegistry(logger)

	registry.Register("get_weather", getWeather, tools.ToolMetadata{
		Schema: types.ToolSchema{
			Name:        "get_weather",
			Description: "查询指定城市的实时天气信息，返回温度、天气状况、湿度等",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"city": {"type": "string", "description": "城市名称"}
				},
				"required": ["city"]
			}`),
		},
		Timeout: 10 * time.Second,
	})

	registry.Register("calculate", calculate, tools.ToolMetadata{
		Schema: types.ToolSchema{
			Name:        "calculate",
			Description: "计算数学表达式并返回结果",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"expression": {"type": "string", "description": "数学表达式"}
				},
				"required": ["expression"]
			}`),
		},
		Timeout: 5 * time.Second,
	})

	// 创建工具执行器
	executor := tools.NewDefaultExecutor(registry, logger)

	// 创建 ReAct 执行器
	reactExecutor := tools.NewReActExecutor(provider, executor, tools.ReActConfig{
		MaxIterations: 5,
		StopOnError:   false,
	}, logger)

	// 构造请求
	req := &llm.ChatRequest{
		Model: model,
		Messages: []types.Message{
			{Role: llm.RoleSystem, Content: "你是一个智能助手。请使用工具来获取信息并回答用户问题。先查询需要的信息，然后给出完整回答。"},
			{Role: llm.RoleUser, Content: "上海今天天气如何？顺便帮我算一下 1024 / 8 等于多少？"},
		},
		Tools:       registry.List(),
		MaxTokens:   1024,
		Temperature: 0.1,
	}

	start := time.Now()
	resp, steps, err := reactExecutor.Execute(ctx, req)
	elapsed := time.Since(start)

	if err != nil {
		fmt.Printf("  ❌ ReAct 执行失败: %v\n", err)
		// 即使失败也打印已完成的步骤
		if len(steps) > 0 {
			printReActSteps(steps)
		}
		fmt.Println()
		return
	}

	fmt.Printf("  ⏱️  总耗时: %v\n", elapsed)
	fmt.Printf("  🔄 迭代次数: %d\n", len(steps))

	// 打印每步详情
	printReActSteps(steps)

	// 打印最终回答
	if resp != nil && len(resp.Choices) > 0 {
		fmt.Printf("  📝 最终回答:\n    %s\n", truncate(resp.Choices[0].Message.Content, 500))
		fmt.Printf("  📊 Token: prompt=%d completion=%d total=%d\n",
			resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.TotalTokens)
	}
	fmt.Println()
}

// =============================================================================
// 测试 3: ReAct 流式循环 — 实时打印 LLM 输出 + 工具调用
// =============================================================================

func test3ReActStream(ctx context.Context, provider llm.Provider, logger *zap.Logger) {
	fmt.Println("📡 [测试3] ReAct 流式循环 — 实时打印推理+工具+回答")
	fmt.Println(strings.Repeat("─", 60))

	registry := tools.NewDefaultRegistry(logger)

	registry.Register("search_knowledge", searchKnowledge, tools.ToolMetadata{
		Schema: types.ToolSchema{
			Name:        "search_knowledge",
			Description: "搜索知识库获取技术信息",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"query": {"type": "string", "description": "搜索关键词"}
				},
				"required": ["query"]
			}`),
		},
		Timeout: 10 * time.Second,
	})

	executor := tools.NewDefaultExecutor(registry, logger)

	reactExecutor := tools.NewReActExecutor(provider, executor, tools.ReActConfig{
		MaxIterations: 3,
		StopOnError:   false,
	}, logger)

	req := &llm.ChatRequest{
		Model: model,
		Messages: []types.Message{
			{Role: llm.RoleSystem, Content: "你是一个技术顾问。使用 search_knowledge 工具搜索信息，然后给出建议。"},
			{Role: llm.RoleUser, Content: "Go语言的并发编程有什么特点？"},
		},
		Tools:       registry.List(),
		MaxTokens:   1024,
		Temperature: 0.1,
	}

	start := time.Now()
	eventCh, err := reactExecutor.ExecuteStream(ctx, req)
	if err != nil {
		fmt.Printf("  ❌ ReAct 流式调用失败: %v\n\n", err)
		return
	}

	iterCount := 0
	for event := range eventCh {
		switch event.Type {
		case tools.ReActEventIterationStart:
			iterCount++
			fmt.Printf("\n  ── 迭代 #%d ──\n", iterCount)

		case tools.ReActEventLLMChunk:
			if event.Chunk != nil {
				if event.Chunk.Delta.Content != "" {
					fmt.Print(event.Chunk.Delta.Content)
				}
				if event.Chunk.Delta.ReasoningContent != nil && *event.Chunk.Delta.ReasoningContent != "" {
					// 推理内容用灰色标识（只打一次前缀）
				}
			}

		case tools.ReActEventToolsStart:
			fmt.Printf("\n  🔧 开始执行工具: ")
			for _, tc := range event.ToolCalls {
				fmt.Printf("[%s(%s)] ", tc.Name, string(tc.Arguments))
			}
			fmt.Println()

		case tools.ReActEventToolProgress:
			fmt.Printf("  ⏳ 工具进度: %v\n", event.ProgressData)

		case tools.ReActEventToolsEnd:
			fmt.Printf("  ✅ 工具执行完成，返回 %d 个结果\n", len(event.ToolResults))
			for _, r := range event.ToolResults {
				fmt.Printf("    └─ %s: %s\n", r.Name, truncate(string(r.Result), 100))
			}

		case tools.ReActEventCompleted:
			fmt.Printf("\n  🎉 ReAct 完成！迭代: %d\n", iterCount)

		case tools.ReActEventError:
			fmt.Printf("\n  ❌ 错误: %v\n", event.Error)
		}
	}

	fmt.Printf("  ⏱️  总耗时: %v\n", time.Since(start))
	fmt.Println()
}

// =============================================================================
// 测试 4: 循环智能体 — 手动实现 Loop Agent 多轮迭代模式
// =============================================================================

func test4LoopAgent(ctx context.Context, provider llm.Provider, logger *zap.Logger) {
	fmt.Println("🔁 [测试4] 循环智能体 — 多轮自我迭代（Loop Agent 模式）")
	fmt.Println(strings.Repeat("─", 60))

	// Loop Agent 核心思路：
	// 1. 每轮 LLM 调用的输出作为下一轮的输入
	// 2. 检测停止关键词或达到最大迭代次数时退出
	// 3. 每轮可以进行自我改进/反思/迭代优化

	const (
		maxIterations = 3
		stopKeyword   = "FINAL_ANSWER"
	)

	systemPrompt := `你是一个迭代优化助手。你的工作模式是多轮自我改进：

规则：
1. 每一轮你需要审视上一轮的输出，找出可以改进的地方
2. 进行改进并输出更好的版本
3. 当你认为答案已经足够好时，在回答的最开头加上 "FINAL_ANSWER" 标记
4. 回答要简洁，每轮不超过100字

请严格遵守以上规则。`

	currentInput := "请用3句话总结微服务架构的优缺点。"

	fmt.Printf("  📋 初始问题: %s\n", currentInput)
	fmt.Printf("  🔢 最大迭代: %d | 停止关键词: %s\n\n", maxIterations, stopKeyword)

	var (
		totalTokens int
		allOutputs  []string
	)

	messages := []types.Message{
		{Role: llm.RoleSystem, Content: systemPrompt},
		{Role: llm.RoleUser, Content: currentInput},
	}

	start := time.Now()

	for iter := 1; iter <= maxIterations; iter++ {
		fmt.Printf("  ── 🔄 迭代 #%d ──\n", iter)

		req := &llm.ChatRequest{
			Model:       model,
			Messages:    messages,
			MaxTokens:   512,
			Temperature: 0.7,
		}

		iterStart := time.Now()
		resp, err := provider.Completion(ctx, req)
		iterElapsed := time.Since(iterStart)

		if err != nil {
			fmt.Printf("    ❌ 迭代 %d 失败: %v\n", iter, err)
			break
		}

		if len(resp.Choices) == 0 {
			fmt.Printf("    ❌ 迭代 %d 无响应\n", iter)
			break
		}

		output := resp.Choices[0].Message.Content
		allOutputs = append(allOutputs, output)
		totalTokens += resp.Usage.TotalTokens

		fmt.Printf("    ⏱️  耗时: %v | Token: %d\n", iterElapsed, resp.Usage.TotalTokens)

		// 显示推理内容（如果有）
		if resp.Choices[0].Message.ReasoningContent != nil {
			fmt.Printf("    💭 推理: %s\n", truncate(*resp.Choices[0].Message.ReasoningContent, 150))
		}

		fmt.Printf("    📝 输出: %s\n", truncate(output, 300))

		// 检测停止条件
		if strings.Contains(output, stopKeyword) {
			fmt.Printf("\n  🎯 检测到停止关键词 '%s'，在第 %d 轮停止\n", stopKeyword, iter)
			break
		}

		if iter == maxIterations {
			fmt.Printf("\n  ⚠️  达到最大迭代次数 %d，强制停止\n", maxIterations)
			break
		}

		// 将本轮输出作为下一轮上下文
		messages = append(messages,
			types.Message{Role: llm.RoleAssistant, Content: output},
			types.Message{Role: llm.RoleUser, Content: fmt.Sprintf("这是你第%d轮的回答。请审视它，找出可以改进的地方，输出更好的版本。如果你认为已经足够好了，在回答开头加上 FINAL_ANSWER。", iter)},
		)
		fmt.Println()
	}

	elapsed := time.Since(start)

	fmt.Println()
	fmt.Println("  ═══════ 循环智能体总结 ═══════")
	fmt.Printf("  ├─ 总迭代次数: %d\n", len(allOutputs))
	fmt.Printf("  ├─ 总耗时: %v\n", elapsed)
	fmt.Printf("  ├─ 总 Token: %d\n", totalTokens)
	if len(allOutputs) > 0 {
		fmt.Printf("  └─ 最终输出:\n    %s\n", allOutputs[len(allOutputs)-1])
	}
	fmt.Println()
}

// =============================================================================
// 辅助函数
// =============================================================================

// printReActSteps 格式化打印 ReAct 步骤
func printReActSteps(steps []tools.ReActStep) {
	for _, step := range steps {
		fmt.Printf("  ── 步骤 #%d ──\n", step.StepNumber)
		if step.Thought != "" {
			fmt.Printf("    💭 思考: %s\n", truncate(step.Thought, 200))
		}
		if len(step.Actions) > 0 {
			fmt.Printf("    🔧 工具调用:\n")
			for _, tc := range step.Actions {
				fmt.Printf("      └─ %s(%s)\n", tc.Name, truncate(string(tc.Arguments), 80))
			}
		}
		if len(step.Observations) > 0 {
			fmt.Printf("    👁️  工具结果:\n")
			for _, obs := range step.Observations {
				if obs.Error != "" {
					fmt.Printf("      └─ %s: ❌ %s\n", obs.Name, obs.Error)
				} else {
					fmt.Printf("      └─ %s: %s\n", obs.Name, truncate(string(obs.Result), 100))
				}
			}
		}
		fmt.Printf("    📊 Token: %d\n", step.TokensUsed)
	}
}

// truncate 截断字符串并加省略号
func truncate(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
