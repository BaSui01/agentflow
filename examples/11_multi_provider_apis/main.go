package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/providers"
	"github.com/BaSui01/agentflow/providers/anthropic"
	"github.com/BaSui01/agentflow/providers/gemini"
	"github.com/BaSui01/agentflow/providers/openai"
	"go.uber.org/zap"
)

func main() {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	ctx := context.Background()

	fmt.Println("=== 多提供商 API 集成示例 ===")

	// 1. OpenAI - 使用新的 Responses API (2025)
	fmt.Println("1. OpenAI Responses API (2025)")
	testOpenAIResponsesAPI(ctx, logger)

	// 2. OpenAI - 传统 Chat Completions API
	fmt.Println("\n2. OpenAI Chat Completions API (传统)")
	testOpenAIChatCompletions(ctx, logger)

	// 3. Claude - Messages API
	fmt.Println("\n3. Claude Messages API")
	testClaudeMessagesAPI(ctx, logger)

	// 4. Gemini - Generate Content API
	fmt.Println("\n4. Gemini Generate Content API")
	testGeminiGenerateContent(ctx, logger)

	// 5. 工具调用对比
	fmt.Println("\n5. 工具调用对比")
	testToolCalling(ctx, logger)
}

// testOpenAIResponsesAPI 测试 OpenAI 新的 Responses API
func testOpenAIResponsesAPI(ctx context.Context, logger *zap.Logger) {
	cfg := providers.OpenAIConfig{
		APIKey:          os.Getenv("OPENAI_API_KEY"),
		BaseURL:         "https://api.openai.com",
		Model:           "gpt-4o-mini",
		UseResponsesAPI: true, // 启用新 API
	}

	provider := openai.NewOpenAIProvider(cfg, logger)

	req := &llm.ChatRequest{
		Model: "gpt-4o-mini",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "用一句话解释什么是 AI"},
		},
	}

	resp, err := provider.Completion(ctx, req)
	if err != nil {
		log.Printf("错误: %v", err)
		return
	}

	fmt.Printf("响应: %s\n", resp.Choices[0].Message.Content)
	fmt.Printf("Token 使用: %d (输入) + %d (输出) = %d (总计)\n",
		resp.Usage.PromptTokens,
		resp.Usage.CompletionTokens,
		resp.Usage.TotalTokens)
}

// testOpenAIChatCompletions 测试 OpenAI 传统 API
func testOpenAIChatCompletions(ctx context.Context, logger *zap.Logger) {
	cfg := providers.OpenAIConfig{
		APIKey:          os.Getenv("OPENAI_API_KEY"),
		BaseURL:         "https://api.openai.com",
		Model:           "gpt-4o-mini",
		UseResponsesAPI: false, // 使用传统 API
	}

	provider := openai.NewOpenAIProvider(cfg, logger)

	req := &llm.ChatRequest{
		Model: "gpt-4o-mini",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "用一句话解释什么是机器学习"},
		},
	}

	resp, err := provider.Completion(ctx, req)
	if err != nil {
		log.Printf("错误: %v", err)
		return
	}

	fmt.Printf("响应: %s\n", resp.Choices[0].Message.Content)
	fmt.Printf("Token 使用: %d (输入) + %d (输出) = %d (总计)\n",
		resp.Usage.PromptTokens,
		resp.Usage.CompletionTokens,
		resp.Usage.TotalTokens)
}

// testClaudeMessagesAPI 测试 Claude Messages API
func testClaudeMessagesAPI(ctx context.Context, logger *zap.Logger) {
	cfg := providers.ClaudeConfig{
		APIKey:  os.Getenv("ANTHROPIC_API_KEY"),
		BaseURL: "https://api.anthropic.com",
		Model:   "claude-3-5-sonnet-20241022",
	}

	provider := claude.NewClaudeProvider(cfg, logger)

	req := &llm.ChatRequest{
		Model: "claude-3-5-sonnet-20241022",
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: "你是一个专业的技术顾问"},
			{Role: llm.RoleUser, Content: "用一句话解释什么是深度学习"},
		},
	}

	resp, err := provider.Completion(ctx, req)
	if err != nil {
		log.Printf("错误: %v", err)
		return
	}

	fmt.Printf("响应: %s\n", resp.Choices[0].Message.Content)
	fmt.Printf("Token 使用: %d (输入) + %d (输出) = %d (总计)\n",
		resp.Usage.PromptTokens,
		resp.Usage.CompletionTokens,
		resp.Usage.TotalTokens)
}

// testGeminiGenerateContent 测试 Gemini Generate Content API
func testGeminiGenerateContent(ctx context.Context, logger *zap.Logger) {
	cfg := providers.GeminiConfig{
		APIKey:  os.Getenv("GEMINI_API_KEY"),
		BaseURL: "https://generativelanguage.googleapis.com",
		Model:   "gemini-2.5-flash",
	}

	provider := gemini.NewGeminiProvider(cfg, logger)

	req := &llm.ChatRequest{
		Model: "gemini-2.5-flash",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "用一句话解释什么是神经网络"},
		},
	}

	resp, err := provider.Completion(ctx, req)
	if err != nil {
		log.Printf("错误: %v", err)
		return
	}

	fmt.Printf("响应: %s\n", resp.Choices[0].Message.Content)
	fmt.Printf("Token 使用: %d (输入) + %d (输出) = %d (总计)\n",
		resp.Usage.PromptTokens,
		resp.Usage.CompletionTokens,
		resp.Usage.TotalTokens)
}

// testToolCalling 测试工具调用
func testToolCalling(ctx context.Context, logger *zap.Logger) {
	// 定义工具
	tools := []llm.ToolSchema{
		{
			Name:        "get_weather",
			Description: "获取指定城市的天气信息",
			Parameters: []byte(`{
				"type": "object",
				"properties": {
					"city": {
						"type": "string",
						"description": "城市名称"
					}
				},
				"required": ["city"]
			}`),
		},
	}

	// OpenAI 工具调用
	fmt.Println("\nOpenAI 工具调用:")
	testProviderToolCalling(ctx, logger, "openai", tools)

	// Claude 工具调用
	fmt.Println("\nClaude 工具调用:")
	testProviderToolCalling(ctx, logger, "claude", tools)

	// Gemini 工具调用
	fmt.Println("\nGemini 工具调用:")
	testProviderToolCalling(ctx, logger, "gemini", tools)
}

func testProviderToolCalling(ctx context.Context, logger *zap.Logger, providerName string, tools []llm.ToolSchema) {
	var provider llm.Provider

	switch providerName {
	case "openai":
		cfg := providers.OpenAIConfig{
			APIKey:  os.Getenv("OPENAI_API_KEY"),
			BaseURL: "https://api.openai.com",
			Model:   "gpt-4o-mini",
		}
		provider = openai.NewOpenAIProvider(cfg, logger)

	case "claude":
		cfg := providers.ClaudeConfig{
			APIKey:  os.Getenv("ANTHROPIC_API_KEY"),
			BaseURL: "https://api.anthropic.com",
			Model:   "claude-3-5-sonnet-20241022",
		}
		provider = claude.NewClaudeProvider(cfg, logger)

	case "gemini":
		cfg := providers.GeminiConfig{
			APIKey:  os.Getenv("GEMINI_API_KEY"),
			BaseURL: "https://generativelanguage.googleapis.com",
			Model:   "gemini-2.5-flash",
		}
		provider = gemini.NewGeminiProvider(cfg, logger)

	default:
		log.Printf("未知的提供商: %s", providerName)
		return
	}

	req := &llm.ChatRequest{
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "北京今天天气怎么样？"},
		},
		Tools: tools,
	}

	resp, err := provider.Completion(ctx, req)
	if err != nil {
		log.Printf("错误: %v", err)
		return
	}

	if len(resp.Choices) > 0 && len(resp.Choices[0].Message.ToolCalls) > 0 {
		for _, tc := range resp.Choices[0].Message.ToolCalls {
			fmt.Printf("工具调用: %s\n", tc.Name)
			fmt.Printf("参数: %s\n", string(tc.Arguments))
		}
	} else {
		fmt.Printf("响应: %s\n", resp.Choices[0].Message.Content)
	}
}
