package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/BaSui01/agentflow/llm/providers/openai"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

func main() {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

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
			{Role: llm.RoleSystem, Content: "You are a concise and helpful assistant."},
			{Role: llm.RoleUser, Content: "Hello! What is the capital of France? Please answer in one sentence."},
		},
		MaxTokens:   200,
		Temperature: 0.3,
	}

	resp, err := provider.Completion(ctx, req)
	if err != nil {
		log.Fatalf("Chat completion failed: %v", err)
	}

	if len(resp.Choices) == 0 {
		log.Fatal("chat completion returned no choices")
	}

	fmt.Printf("Base URL: %s\n", baseURL)
	fmt.Printf("Model: %s\n", model)
	fmt.Printf("Response: %s\n", resp.Choices[0].Message.Content)
	fmt.Printf("Token usage: prompt=%d completion=%d total=%d\n",
		resp.Usage.PromptTokens,
		resp.Usage.CompletionTokens,
		resp.Usage.TotalTokens,
	)
}

func envOrDefault(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
