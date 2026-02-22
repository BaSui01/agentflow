package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/BaSui01/agentflow/llm/providers/openai"
	"go.uber.org/zap"
)

// Demonstrates advanced features: Reflection, Dynamic Tool Selection, Prompt Engineering

func main() {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	// Example 1: Reflection Mechanism
	fmt.Println("=== Example 1: Reflection Mechanism ===")
	demoReflection(logger)

	fmt.Println("\n=== Example 2: Dynamic Tool Selection ===")
	demoToolSelection(logger)

	fmt.Println("\n=== Example 3: Prompt Engineering ===")
	demoPromptEngineering(logger)
}

// createProvider creates an OpenAI provider from environment variables.
// Returns nil if OPENAI_API_KEY is not set.
func createProvider(logger *zap.Logger) llm.Provider {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil
	}
	baseURL := os.Getenv("OPENAI_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	cfg := providers.OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			APIKey:  apiKey,
			BaseURL: baseURL,
			Model:   "gpt-4",
		},
	}
	return openai.NewOpenAIProvider(cfg, logger)
}

func demoReflection(logger *zap.Logger) {
	provider := createProvider(logger)
	if provider == nil {
		fmt.Println("  Skipped: set OPENAI_API_KEY to run this demo")
		return
	}

	// Create Agent
	config := agent.Config{
		ID:          "reflection-agent",
		Name:        "Reflection Agent",
		Type:        agent.TypeAnalyzer,
		Model:       "gpt-4",
		MaxTokens:   2000,
		Temperature: 0.7,
	}

	// Create prompt bundle
	config.PromptBundle = agent.PromptBundle{
		Version: "1.0",
		System: agent.SystemPrompt{
			Role:     "You are a professional content analysis expert",
			Identity: "You excel at analyzing text quality, identifying issues, and providing improvement suggestions",
			OutputRules: []string{
				"Output should be clear and structured",
				"Provide specific improvement suggestions",
			},
		},
	}

	baseAgent := agent.NewBaseAgent(config, provider, nil, nil, nil, logger)

	// Configure Reflection
	reflectionConfig := agent.ReflectionExecutorConfig{
		Enabled:       true,
		MaxIterations: 3,
		MinQuality:    0.7,
	}

	executor := agent.NewReflectionExecutor(baseAgent, reflectionConfig)

	// Execute task
	input := &agent.Input{
		TraceID: "trace-001",
		Content: "Please write a short article about artificial intelligence",
	}

	ctx := context.Background()
	result, err := executor.ExecuteWithReflection(ctx, input)
	if err != nil {
		log.Printf("Reflection execution failed: %v", err)
		return
	}

	fmt.Printf("Iterations: %d\n", result.Iterations)
	fmt.Printf("Improved: %v\n", result.ImprovedByReflection)
	fmt.Printf("Total duration: %v\n", result.TotalDuration)

	// Print critique results for each iteration
	for i, critique := range result.Critiques {
		fmt.Printf("\nIteration %d critique:\n", i+1)
		fmt.Printf("  Score: %.2f\n", critique.Score)
		fmt.Printf("  Is good: %v\n", critique.IsGood)
		if len(critique.Issues) > 0 {
			fmt.Printf("  Issues: %v\n", critique.Issues)
		}
		if len(critique.Suggestions) > 0 {
			fmt.Printf("  Suggestions: %v\n", critique.Suggestions)
		}
	}
}

func demoToolSelection(logger *zap.Logger) {
	// Create Agent with provider if available, otherwise use nil
	// (tool scoring is keyword-based and works without a provider)
	provider := createProvider(logger)

	config := agent.Config{
		ID:          "tool-agent",
		Name:        "Tool Selection Agent",
		Type:        agent.TypeGeneric,
		Model:       "gpt-4",
		MaxTokens:   2000,
		Temperature: 0.7,
	}

	baseAgent := agent.NewBaseAgent(config, provider, nil, nil, nil, logger)

	// Configure dynamic tool selection
	selectorConfig := agent.DefaultToolSelectionConfig()
	selectorConfig.MaxTools = 3
	selectorConfig.MinScore = 0.4
	// Disable LLM ranking when no provider is available
	if provider == nil {
		selectorConfig.UseLLMRanking = false
	}

	selector := agent.NewDynamicToolSelector(baseAgent, *selectorConfig)

	// Define available tools
	availableTools := []llm.ToolSchema{
		{
			Name:        "web_search",
			Description: "Search the internet for latest information",
		},
		{
			Name:        "calculator",
			Description: "Perform mathematical calculations",
		},
		{
			Name:        "code_interpreter",
			Description: "Execute Python code",
		},
		{
			Name:        "database_query",
			Description: "Query database",
		},
		{
			Name:        "file_reader",
			Description: "Read file contents",
		},
	}

	// Task: requires search and calculation
	task := "Find the latest GDP data and calculate growth rate"

	ctx := context.Background()
	selectedTools, err := selector.SelectTools(ctx, task, availableTools)
	if err != nil {
		log.Printf("Tool selection failed: %v", err)
		return
	}

	fmt.Printf("Task: %s\n", task)
	fmt.Printf("Available tools: %d\n", len(availableTools))
	fmt.Printf("Selected tools: %d\n", len(selectedTools))
	fmt.Println("Selected tools:")
	for i, tool := range selectedTools {
		fmt.Printf("  %d. %s - %s\n", i+1, tool.Name, tool.Description)
	}

	// Demonstrate tool scoring
	scores, _ := selector.ScoreTools(ctx, task, availableTools)
	fmt.Println("\nTool scoring details:")
	for _, score := range scores {
		fmt.Printf("  %s: Total=%.2f (Semantic=%.2f, Cost=%.2f, Reliability=%.2f)\n",
			score.Tool.Name,
			score.TotalScore,
			score.SemanticSimilarity,
			1.0-score.EstimatedCost*10, // Normalized display
			score.ReliabilityScore,
		)
	}

	// Update tool statistics
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
