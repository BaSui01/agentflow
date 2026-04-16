package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers/vendor"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

func main() {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	ctx := context.Background()

	fmt.Println("=== Testing Mistral AI ===")
	testCompatProvider(ctx, logger, compatProviderExample{
		ProviderCode: "mistral",
		APIKeyEnv:    "MISTRAL_API_KEY",
		Model:        "mistral-large-latest",
		Prompt:       "What is the capital of France?",
	})

	fmt.Println("\n=== Testing Tencent Hunyuan ===")
	testCompatProvider(ctx, logger, compatProviderExample{
		ProviderCode: "hunyuan",
		APIKeyEnv:    "HUNYUAN_API_KEY",
		Model:        "hunyuan-lite",
		Prompt:       "介绍一下北京",
	})

	fmt.Println("\n=== Testing Moonshot Kimi ===")
	testCompatProvider(ctx, logger, compatProviderExample{
		ProviderCode: "kimi",
		APIKeyEnv:    "KIMI_API_KEY",
		Model:        "moonshot-v1-8k",
		Prompt:       "什么是月之暗面？",
	})

	fmt.Println("\n=== Testing Meta Llama ===")
	testCompatProvider(ctx, logger, compatProviderExample{
		ProviderCode: "llama",
		APIKeyEnv:    "TOGETHER_API_KEY",
		Model:        "meta-llama/Llama-3.3-70B-Instruct-Turbo",
		Prompt:       "What is Meta's Llama model?",
		Extra: map[string]any{
			"provider": "together",
		},
	})
}

type compatProviderExample struct {
	ProviderCode string
	APIKeyEnv    string
	Model        string
	Prompt       string
	Extra        map[string]any
}

func testCompatProvider(ctx context.Context, logger *zap.Logger, cfg compatProviderExample) {
	apiKey := os.Getenv(cfg.APIKeyEnv)
	if apiKey == "" {
		log.Printf("%s not set, skipping\n", cfg.APIKeyEnv)
		return
	}

	provider, err := vendor.NewChatProviderFromConfig(cfg.ProviderCode, vendor.ChatProviderConfig{
		APIKey:  apiKey,
		Model:   cfg.Model,
		Timeout: 30 * time.Second,
		Extra:   cfg.Extra,
	}, logger)
	if err != nil {
		log.Printf("create provider failed: %v\n", err)
		return
	}

	status, err := provider.HealthCheck(ctx)
	if err != nil {
		log.Printf("Health check failed: %v\n", err)
		return
	}
	fmt.Printf("Provider: %s, Health: %v, Latency: %v\n", provider.Name(), status.Healthy, status.Latency)

	resp, err := provider.Completion(ctx, &llm.ChatRequest{
		Model: cfg.Model,
		Messages: []types.Message{
			{Role: llm.RoleUser, Content: cfg.Prompt},
		},
		MaxTokens:   100,
		Temperature: 0.7,
	})
	if err != nil {
		log.Printf("Completion failed: %v\n", err)
		return
	}

	fmt.Printf("Response: %s\n", resp.Choices[0].Message.Content)
	fmt.Printf("Usage: %d tokens\n", resp.Usage.TotalTokens)
}
