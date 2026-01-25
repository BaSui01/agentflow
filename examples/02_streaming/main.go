package main

import (
	"context"
	"fmt"
	"log"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/providers"
	"github.com/BaSui01/agentflow/providers/openai"
	"go.uber.org/zap"
)

func main() {
	// 1. 创建 Logger
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	// 2. 配置 OpenAI Provider
	cfg := providers.OpenAIConfig{
		APIKey:  "your-api-key-here", // 替换为你的 API Key
		BaseURL: "https://api.openai.com/v1",
		Model:   "gpt-3.5-turbo",
	}

	// 3. 创建 Provider
	provider := openai.NewOpenAIProvider(cfg, logger)

	// 4. 发起流式对话
	ctx := context.Background()
	req := &llm.ChatRequest{
		Model: "gpt-3.5-turbo",
		Messages: []llm.Message{
			{
				Role:    llm.RoleUser,
				Content: "Write a short poem about Go programming language.",
			},
		},
		MaxTokens:   200,
		Temperature: 0.8,
	}

	stream, err := provider.Stream(ctx, req)
	if err != nil {
		log.Fatalf("Stream failed: %v", err)
	}

	// 5. 逐块打印响应
	fmt.Println("Streaming response:")
	fmt.Println("---")
	for chunk := range stream {
		if chunk.Err != nil {
			log.Fatalf("Stream error: %v", chunk.Err)
		}
		fmt.Print(chunk.Delta.Content)
	}
	fmt.Println("\n---")
}
