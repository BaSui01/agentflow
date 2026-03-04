package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/BaSui01/agentflow"
	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/BaSui01/agentflow/llm/providers/openai"
	"go.uber.org/zap"
)

func main() {
	// 1. 创建 Logger
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	// 2. 从环境变量读取 API Key
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("请设置环境变量 OPENAI_API_KEY，例如: export OPENAI_API_KEY=sk-xxx")
	}

	// 3. 配置 OpenAI Provider
	cfg := providers.OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			APIKey:  apiKey,
			BaseURL: "https://api.openai.com",
			Model:   "gpt-3.5-turbo",
		},
	}

	// 3. 创建 Provider
	provider := openai.NewOpenAIProvider(cfg, logger)

	// 4. 通过顶层便捷入口创建 Agent
	options := buildOptionSamples(logger, provider, apiKey)
	chatAgent, err := agentflow.New(options...)
	if err != nil {
		log.Fatalf("Create agent failed: %v", err)
	}

	// 5. 发起对话
	ctx := context.Background()
	out, err := chatAgent.Execute(ctx, &agent.Input{
		Content: "Hello! What is the capital of France?",
	})
	if err != nil {
		log.Fatalf("Agent execution failed: %v", err)
	}

	// 6. 打印响应
	fmt.Printf("Response: %s\n", out.Content)
}

func buildOptionSamples(logger *zap.Logger, provider llm.Provider, apiKey string) []agentflow.Option {
	return []agentflow.Option{
		agentflow.WithProvider(provider),
		agentflow.WithToolProvider(provider),
		agentflow.WithOpenAI("gpt-3.5-turbo"),
		agentflow.WithAnthropic("claude-sonnet-4-20250514"),
		agentflow.WithDeepSeek("deepseek-chat"),
		agentflow.WithModel("gpt-3.5-turbo"),
		agentflow.WithName("simple-chat-agent"),
		agentflow.WithSystemPrompt("You are a helpful assistant."),
		agentflow.WithLogger(logger),
		agentflow.WithAPIKey(apiKey),
	}
}
