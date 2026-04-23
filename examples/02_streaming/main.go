package main

import (
	"context"
	"fmt"
	"log"
	"os"

	llm "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/BaSui01/agentflow/llm/providers/openai"
	"github.com/BaSui01/agentflow/types"
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

	baseURL := envOrDefault("OPENAI_BASE_URL", "https://api.openai.com")
	model := envOrDefault("OPENAI_MODEL", "gpt-4o-mini")

	cfg := providers.OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			APIKey:  apiKey,
			BaseURL: baseURL,
			Model:   model,
		},
	}

	provider := openai.NewOpenAIProvider(cfg, logger)

	ctx := context.Background()
	req := &llm.ChatRequest{
		Model: model,
		Messages: []types.Message{
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

	fmt.Printf("Base URL: %s\n", baseURL)
	fmt.Printf("Model: %s\n", model)
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

func envOrDefault(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
