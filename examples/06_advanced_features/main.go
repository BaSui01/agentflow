package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/yourusername/agentflow/agent"
	"github.com/yourusername/agentflow/llm"
	"go.uber.org/zap"
)

// 演示高级特性：Reflection、动态工具选择、提示词工程

func main() {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	// 示例 1: Reflection 机制
	fmt.Println("=== 示例 1: Reflection 机制 ===")
	demoReflection(logger)

	fmt.Println("\n=== 示例 2: 动态工具选择 ===")
	demoToolSelection(logger)

	fmt.Println("\n=== 示例 3: 提示词工程 ===")
	demoPromptEngineering(logger)
}

func demoReflection(logger *zap.Logger) {
	// 创建 Agent
	config := agent.Config{
		ID:          "reflection-agent",
		Name:        "Reflection Agent",
		Type:        agent.TypeAnalyzer,
		Model:       "gpt-4",
		MaxTokens:   2000,
		Temperature: 0.7,
	}

	// 创建提示词包
	config.PromptBundle = agent.PromptBundle{
		Version: "1.0",
		System: agent.SystemPrompt{
			Role:     "你是一个专业的内容分析专家",
			Identity: "你擅长分析文本质量，发现问题并提供改进建议",
			OutputRules: []string{
				"输出要清晰、结构化",
				"提供具体的改进建议",
			},
		},
	}

	// 假设已有 provider（这里用 nil 演示结构）
	var provider llm.Provider = nil // 实际使用时需要初始化

	baseAgent := agent.NewBaseAgent(config, provider, nil, nil, nil, logger)

	// 配置 Reflection
	reflectionConfig := agent.ReflectionExecutorConfig{
		Enabled:       true,
		MaxIterations: 3,
		MinQuality:    0.7,
	}

	executor := agent.NewReflectionExecutor(baseAgent, reflectionConfig)

	// 执行任务
	input := &agent.Input{
		TraceID: "trace-001",
		Content: "请写一篇关于人工智能的短文",
	}

	ctx := context.Background()
	result, err := executor.ExecuteWithReflection(ctx, input)
	if err != nil {
		log.Printf("Reflection 执行失败: %v", err)
		return
	}

	fmt.Printf("迭代次数: %d\n", result.Iterations)
	fmt.Printf("是否改进: %v\n", result.ImprovedByReflection)
	fmt.Printf("总耗时: %v\n", result.TotalDuration)

	// 打印每次迭代的评审结果
	for i, critique := range result.Critiques {
		fmt.Printf("\n第 %d 次迭代评审:\n", i+1)
		fmt.Printf("  分数: %.2f\n", critique.Score)
		fmt.Printf("  是否达标: %v\n", critique.IsGood)
		if len(critique.Issues) > 0 {
			fmt.Printf("  问题: %v\n", critique.Issues)
		}
		if len(critique.Suggestions) > 0 {
			fmt.Printf("  建议: %v\n", critique.Suggestions)
		}
	}
}

func demoToolSelection(logger *zap.Logger) {
	// 创建 Agent
	config := agent.Config{
		ID:          "tool-agent",
		Name:        "Tool Selection Agent",
		Type:        agent.TypeGeneric,
		Model:       "gpt-4",
		MaxTokens:   2000,
		Temperature: 0.7,
	}

	var provider llm.Provider = nil
	baseAgent := agent.NewBaseAgent(config, provider, nil, nil, nil, logger)

	// 配置动态工具选择
	selectorConfig := agent.DefaultToolSelectionConfig()
	selectorConfig.MaxTools = 3
	selectorConfig.MinScore = 0.4

	selector := agent.NewDynamicToolSelector(baseAgent, selectorConfig)

	// 定义可用工具
	availableTools := []llm.ToolSchema{
		{
			Name:        "web_search",
			Description: "搜索互联网获取最新信息",
		},
		{
			Name:        "calculator",
			Description: "执行数学计算",
		},
		{
			Name:        "code_interpreter",
			Description: "执行 Python 代码",
		},
		{
			Name:        "database_query",
			Description: "查询数据库",
		},
		{
			Name:        "file_reader",
			Description: "读取文件内容",
		},
	}

	// 任务：需要搜索和计算
	task := "查找最新的 GDP 数据并计算增长率"

	ctx := context.Background()
	selectedTools, err := selector.SelectTools(ctx, task, availableTools)
	if err != nil {
		log.Printf("工具选择失败: %v", err)
		return
	}

	fmt.Printf("任务: %s\n", task)
	fmt.Printf("可用工具数: %d\n", len(availableTools))
	fmt.Printf("选择的工具数: %d\n", len(selectedTools))
	fmt.Println("选择的工具:")
	for i, tool := range selectedTools {
		fmt.Printf("  %d. %s - %s\n", i+1, tool.Name, tool.Description)
	}

	// 演示工具评分
	scores, _ := selector.ScoreTools(ctx, task, availableTools)
	fmt.Println("\n工具评分详情:")
	for _, score := range scores {
		fmt.Printf("  %s: 总分=%.2f (语义=%.2f, 成本=%.2f, 可靠性=%.2f)\n",
			score.Tool.Name,
			score.TotalScore,
			score.SemanticSimilarity,
			1.0-score.EstimatedCost*10, // 归一化显示
			score.ReliabilityScore,
		)
	}

	// 更新工具统计
	selector.UpdateToolStats("web_search", true, 500*time.Millisecond, 0.05)
	selector.UpdateToolStats("calculator", true, 100*time.Millisecond, 0.01)
}

func demoPromptEngineering(logger *zap.Logger) {
	// 1. 使用提示词增强器
	fmt.Println("1. 提示词增强器")

	config := agent.DefaultPromptEngineeringConfig()
	enhancer := agent.NewPromptEnhancer(config)

	// 原始提示词包
	bundle := agent.PromptBundle{
		Version: "1.0",
		System: agent.SystemPrompt{
			Role:     "你是一个助手",
			Identity: "帮助用户解决问题",
		},
		Examples: []agent.Example{
			{
				User:      "什么是 AI?",
				Assistant: "AI 是人工智能的缩写...",
			},
			{
				User:      "AI 有什么应用?",
				Assistant: "AI 应用广泛，包括...",
			},
			{
				User:      "AI 的未来如何?",
				Assistant: "AI 的未来充满可能...",
			},
			{
				User:      "如何学习 AI?",
				Assistant: "学习 AI 可以从...",
			},
		},
	}

	// 增强提示词
	enhanced := enhancer.EnhancePromptBundle(bundle)

	fmt.Println("原始系统提示词:")
	fmt.Println(bundle.RenderSystemPrompt())
	fmt.Println("\n增强后的系统提示词:")
	fmt.Println(enhanced.RenderSystemPrompt())
	fmt.Printf("\n示例数量: %d -> %d\n", len(bundle.Examples), len(enhanced.Examples))

	// 2. 使用提示词优化器
	fmt.Println("\n2. 提示词优化器")

	optimizer := agent.NewPromptOptimizer()

	originalPrompt := "写代码"
	optimizedPrompt := optimizer.OptimizePrompt(originalPrompt)

	fmt.Println("原始提示词:")
	fmt.Println(originalPrompt)
	fmt.Println("\n优化后的提示词:")
	fmt.Println(optimizedPrompt)

	// 3. 使用提示词模板库
	fmt.Println("\n3. 提示词模板库")

	library := agent.NewPromptTemplateLibrary()

	// 列出所有模板
	fmt.Println("可用模板:")
	for i, name := range library.ListTemplates() {
		template, _ := library.GetTemplate(name)
		fmt.Printf("  %d. %s - %s\n", i+1, name, template.Description)
	}

	// 使用代码生成模板
	fmt.Println("\n使用代码生成模板:")
	codePrompt, err := library.RenderTemplate("code_generation", map[string]string{
		"language":    "Go",
		"requirement": "实现一个 HTTP 服务器，支持 GET 和 POST 请求",
	})
	if err != nil {
		log.Printf("模板渲染失败: %v", err)
		return
	}
	fmt.Println(codePrompt)

	// 注册自定义模板
	fmt.Println("\n注册自定义模板:")
	customTemplate := agent.PromptTemplate{
		Name:        "bug_fix",
		Description: "Bug 修复模板",
		Template: `请修复以下代码中的 Bug：

代码：
{{.code}}

错误信息：
{{.error}}

要求：
- 找出 Bug 的根本原因
- 提供修复方案
- 解释为什么会出现这个 Bug`,
		Variables: []string{"code", "error"},
	}

	library.RegisterTemplate(customTemplate)
	fmt.Printf("已注册模板: %s\n", customTemplate.Name)

	bugFixPrompt, _ := library.RenderTemplate("bug_fix", map[string]string{
		"code":  "func divide(a, b int) int { return a / b }",
		"error": "panic: runtime error: integer divide by zero",
	})
	fmt.Println("\n渲染的 Bug 修复提示词:")
	fmt.Println(bugFixPrompt)
}
