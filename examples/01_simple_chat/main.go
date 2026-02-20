package main

import (
	"context"
	"fmt"
	"log"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/BaSui01/agentflow/llm/providers/openai"
	"go.uber.org/zap"
)

func main() {
	// 1. 创建 Logger
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	// 2. 配置 OpenAI Provider
	cfg := providers.OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			APIKey:  "your-api-key-here", // 替换为你的 API Key
			BaseURL: "https://api.openai.com/v1",
			Model:   "gpt-3.5-turbo",
		},
	}

	// 3. 创建 Provider
	provider := openai.NewOpenAIProvider(cfg, logger)

	// 4. 发起对话
	ctx := context.Background()
	req := &llm.ChatRequest{
		Model: "gpt-3.5-turbo",
		Messages: []llm.Message{
			{
				Role:    llm.RoleUser,
				Content: "Hello! What is the capital of France?",
			},
		},
		MaxTokens:   100,
		Temperature: 0.7,
	}

	resp, err := provider.Completion(ctx, req)
	if err != nil {
		log.Fatalf("Completion failed: %v", err)
	}

	// 5. 打印响应
	fmt.Printf("Response: %s\n", resp.Choices[0].Message.Content)
	fmt.Printf("Tokens used: %d (input: %d, output: %d)\n",
		resp.Usage.TotalTokens,
		resp.Usage.PromptTokens,
		resp.Usage.CompletionTokens,
	)
}
