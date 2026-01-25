package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/yourusername/agentflow/llm"
	"github.com/yourusername/agentflow/providers"
	"github.com/yourusername/agentflow/providers/openai"
	"github.com/yourusername/agentflow/workflow"
	"go.uber.org/zap"
)

func main() {
	// 初始化日志
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	// 获取 API Key
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENAI_API_KEY environment variable is required")
	}

	// 创建 Provider
	provider := openai.NewOpenAIProvider(providers.OpenAIConfig{
		APIKey:  apiKey,
		BaseURL: "https://api.openai.com/v1",
		Model:   "gpt-3.5-turbo",
	}, logger)

	ctx := context.Background()

	fmt.Println("=== AgentFlow Workflow 示例 ===")

	// 示例 1: Prompt Chaining（提示词链）
	fmt.Println("1. Prompt Chaining 示例")
	fmt.Println("场景：将一篇文章翻译成中文，然后总结要点")
	runPromptChaining(ctx, provider, logger)

	fmt.Println(strings.Repeat("-", 60))

	// 示例 2: Routing（路由）
	fmt.Println("2. Routing 示例")
	fmt.Println("场景：根据用户问题类型，路由到不同的专家 Agent")
	runRouting(ctx, provider, logger)

	fmt.Println(strings.Repeat("-", 60))

	// 示例 3: Parallelization（并行化）
	fmt.Println("3. Parallelization 示例")
	fmt.Println("场景：并行分析文章的多个方面（情感、主题、关键词）")
	runParallelization(ctx, provider, logger)
}

// runPromptChaining 演示提示词链工作流
func runPromptChaining(ctx context.Context, provider llm.Provider, logger *zap.Logger) {
	// 步骤 1: 翻译
	translateStep := workflow.NewFuncStep("translate", func(ctx context.Context, input interface{}) (interface{}, error) {
		text := input.(string)
		
		resp, err := provider.Completion(ctx, &llm.ChatRequest{
			Model: "gpt-3.5-turbo",
			Messages: []llm.Message{
				{Role: llm.RoleSystem, Content: "你是一个专业的翻译助手。"},
				{Role: llm.RoleUser, Content: fmt.Sprintf("请将以下英文翻译成中文：\n\n%s", text)},
			},
			MaxTokens:   500,
			Temperature: 0.3,
		})
		if err != nil {
			return nil, err
		}
		
		return resp.Choices[0].Message.Content, nil
	})

	// 步骤 2: 总结
	summarizeStep := workflow.NewFuncStep("summarize", func(ctx context.Context, input interface{}) (interface{}, error) {
		text := input.(string)
		
		resp, err := provider.Completion(ctx, &llm.ChatRequest{
			Model: "gpt-3.5-turbo",
			Messages: []llm.Message{
				{Role: llm.RoleSystem, Content: "你是一个专业的内容总结助手。"},
				{Role: llm.RoleUser, Content: fmt.Sprintf("请总结以下文章的要点（3-5 条）：\n\n%s", text)},
			},
			MaxTokens:   300,
			Temperature: 0.5,
		})
		if err != nil {
			return nil, err
		}
		
		return resp.Choices[0].Message.Content, nil
	})

	// 创建工作流
	chainWorkflow := workflow.NewChainWorkflow(
		"translate-and-summarize",
		"翻译并总结文章",
		translateStep,
		summarizeStep,
	)

	// 执行
	input := "Artificial Intelligence is transforming the world. Machine learning algorithms can now recognize patterns, make predictions, and even create art. The future of AI is both exciting and challenging."

	fmt.Printf("原文：\n%s\n\n", input)

	result, err := chainWorkflow.Execute(ctx, input)
	if err != nil {
		log.Printf("工作流执行失败: %v", err)
		return
	}

	fmt.Printf("结果：\n%s\n", result)
}

// runRouting 演示路由工作流
func runRouting(ctx context.Context, provider llm.Provider, logger *zap.Logger) {
	// 创建路由器（使用 LLM 进行分类）
	router := workflow.NewFuncRouter(func(ctx context.Context, input interface{}) (string, error) {
		question := input.(string)
		
		resp, err := provider.Completion(ctx, &llm.ChatRequest{
			Model: "gpt-3.5-turbo",
			Messages: []llm.Message{
				{Role: llm.RoleSystem, Content: "你是一个问题分类器。请将用户问题分类为：technical（技术问题）、business（商业问题）或 general（一般问题）。只返回分类名称。"},
				{Role: llm.RoleUser, Content: question},
			},
			MaxTokens:   10,
			Temperature: 0.1,
		})
		if err != nil {
			return "", err
		}
		
		category := strings.ToLower(strings.TrimSpace(resp.Choices[0].Message.Content))
		logger.Info("问题分类", zap.String("category", category))
		
		return category, nil
	})

	// 创建处理器
	technicalHandler := workflow.NewFuncHandler("technical", func(ctx context.Context, input interface{}) (interface{}, error) {
		question := input.(string)
		
		resp, err := provider.Completion(ctx, &llm.ChatRequest{
			Model: "gpt-3.5-turbo",
			Messages: []llm.Message{
				{Role: llm.RoleSystem, Content: "你是一个技术专家。请用专业的技术语言回答问题。"},
				{Role: llm.RoleUser, Content: question},
			},
			MaxTokens:   300,
			Temperature: 0.7,
		})
		if err != nil {
			return nil, err
		}
		
		return resp.Choices[0].Message.Content, nil
	})

	businessHandler := workflow.NewFuncHandler("business", func(ctx context.Context, input interface{}) (interface{}, error) {
		question := input.(string)
		
		resp, err := provider.Completion(ctx, &llm.ChatRequest{
			Model: "gpt-3.5-turbo",
			Messages: []llm.Message{
				{Role: llm.RoleSystem, Content: "你是一个商业顾问。请从商业角度回答问题。"},
				{Role: llm.RoleUser, Content: question},
			},
			MaxTokens:   300,
			Temperature: 0.7,
		})
		if err != nil {
			return nil, err
		}
		
		return resp.Choices[0].Message.Content, nil
	})

	generalHandler := workflow.NewFuncHandler("general", func(ctx context.Context, input interface{}) (interface{}, error) {
		question := input.(string)
		
		resp, err := provider.Completion(ctx, &llm.ChatRequest{
			Model: "gpt-3.5-turbo",
			Messages: []llm.Message{
				{Role: llm.RoleSystem, Content: "你是一个友好的助手。请用简单易懂的语言回答问题。"},
				{Role: llm.RoleUser, Content: question},
			},
			MaxTokens:   300,
			Temperature: 0.7,
		})
		if err != nil {
			return nil, err
		}
		
		return resp.Choices[0].Message.Content, nil
	})

	// 创建工作流
	routingWorkflow := workflow.NewRoutingWorkflow(
		"question-routing",
		"根据问题类型路由到专家",
		router,
	)
	routingWorkflow.RegisterHandler("technical", technicalHandler)
	routingWorkflow.RegisterHandler("business", businessHandler)
	routingWorkflow.RegisterHandler("general", generalHandler)
	routingWorkflow.SetDefaultRoute("general")

	// 测试问题
	questions := []string{
		"如何优化 Go 程序的性能？",
		"如何提高公司的市场份额？",
		"今天天气怎么样？",
	}

	for i, question := range questions {
		fmt.Printf("问题 %d: %s\n", i+1, question)
		
		result, err := routingWorkflow.Execute(ctx, question)
		if err != nil {
			log.Printf("工作流执行失败: %v", err)
			continue
		}
		
		fmt.Printf("回答: %s\n\n", result)
	}
}

// runParallelization 演示并行工作流
func runParallelization(ctx context.Context, provider llm.Provider, logger *zap.Logger) {
	// 任务 1: 情感分析
	sentimentTask := workflow.NewFuncTask("sentiment", func(ctx context.Context, input interface{}) (interface{}, error) {
		text := input.(string)
		
		resp, err := provider.Completion(ctx, &llm.ChatRequest{
			Model: "gpt-3.5-turbo",
			Messages: []llm.Message{
				{Role: llm.RoleSystem, Content: "你是一个情感分析专家。请分析文本的情感倾向（积极/消极/中性）。"},
				{Role: llm.RoleUser, Content: text},
			},
			MaxTokens:   100,
			Temperature: 0.3,
		})
		if err != nil {
			return nil, err
		}
		
		return resp.Choices[0].Message.Content, nil
	})

	// 任务 2: 主题提取
	topicTask := workflow.NewFuncTask("topic", func(ctx context.Context, input interface{}) (interface{}, error) {
		text := input.(string)
		
		resp, err := provider.Completion(ctx, &llm.ChatRequest{
			Model: "gpt-3.5-turbo",
			Messages: []llm.Message{
				{Role: llm.RoleSystem, Content: "你是一个主题提取专家。请提取文本的主要主题。"},
				{Role: llm.RoleUser, Content: text},
			},
			MaxTokens:   100,
			Temperature: 0.3,
		})
		if err != nil {
			return nil, err
		}
		
		return resp.Choices[0].Message.Content, nil
	})

	// 任务 3: 关键词提取
	keywordTask := workflow.NewFuncTask("keyword", func(ctx context.Context, input interface{}) (interface{}, error) {
		text := input.(string)
		
		resp, err := provider.Completion(ctx, &llm.ChatRequest{
			Model: "gpt-3.5-turbo",
			Messages: []llm.Message{
				{Role: llm.RoleSystem, Content: "你是一个关键词提取专家。请提取文本的关键词（3-5 个）。"},
				{Role: llm.RoleUser, Content: text},
			},
			MaxTokens:   100,
			Temperature: 0.3,
		})
		if err != nil {
			return nil, err
		}
		
		return resp.Choices[0].Message.Content, nil
	})

	// 创建聚合器
	aggregator := workflow.NewFuncAggregator(func(ctx context.Context, results []workflow.TaskResult) (interface{}, error) {
		report := "=== 文本分析报告 ===\n\n"
		
		for _, r := range results {
			report += fmt.Sprintf("%s:\n%s\n\n", r.TaskName, r.Result)
		}
		
		return report, nil
	})

	// 创建工作流
	parallelWorkflow := workflow.NewParallelWorkflow(
		"text-analysis",
		"并行分析文本的多个方面",
		aggregator,
		sentimentTask,
		topicTask,
		keywordTask,
	)

	// 执行
	input := "AgentFlow 是一个强大的 Go 语言 AI Agent 框架。它提供了完整的 LLM 抽象层、工具调用系统和工作流引擎。开发者可以轻松构建复杂的 AI 应用。"

	fmt.Printf("输入文本：\n%s\n\n", input)

	result, err := parallelWorkflow.Execute(ctx, input)
	if err != nil {
		log.Printf("工作流执行失败: %v", err)
		return
	}

	fmt.Println(result)
}
