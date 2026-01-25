package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/providers"
	"github.com/BaSui01/agentflow/providers/hunyuan"
	"github.com/BaSui01/agentflow/providers/kimi"
	"github.com/BaSui01/agentflow/providers/llama"
	"github.com/BaSui01/agentflow/providers/mistral"
	"go.uber.org/zap"
)

func main() {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	ctx := context.Background()

	// Example 1: Mistral AI
	fmt.Println("=== Testing Mistral AI ===")
	testMistral(ctx, logger)

	// Example 2: Tencent Hunyuan
	fmt.Println("\n=== Testing Tencent Hunyuan ===")
	testHunyuan(ctx, logger)

	// Example 3: Moonshot Kimi
	fmt.Println("\n=== Testing Moonshot Kimi ===")
	testKimi(ctx, logger)

	// Example 4: Meta Llama (via Together AI)
	fmt.Println("\n=== Testing Meta Llama ===")
	testLlama(ctx, logger)
}

func testMistral(ctx context.Context, logger *zap.Logger) {
	apiKey := os.Getenv("MISTRAL_API_KEY")
	if apiKey == "" {
		log.Println("MISTRAL_API_KEY not set, skipping")
		return
	}

	provider := mistral.NewMistralProvider(providers.MistralConfig{
		APIKey:  apiKey,
		Model:   "mistral-large-latest",
		Timeout: 30 * time.Second,
	}, logger)

	// Health check
	status, err := provider.HealthCheck(ctx)
	if err != nil {
		log.Printf("Health check failed: %v\n", err)
		return
	}
	fmt.Printf("Health: %v, Latency: %v\n", status.Healthy, status.Latency)

	// Chat completion
	req := &llm.ChatRequest{
		Model: "mistral-large-latest",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "What is the capital of France?"},
		},
		MaxTokens:   100,
		Temperature: 0.7,
	}

	resp, err := provider.Completion(ctx, req)
	if err != nil {
		log.Printf("Completion failed: %v\n", err)
		return
	}

	fmt.Printf("Response: %s\n", resp.Choices[0].Message.Content)
	fmt.Printf("Usage: %d tokens\n", resp.Usage.TotalTokens)
}

func testHunyuan(ctx context.Context, logger *zap.Logger) {
	apiKey := os.Getenv("HUNYUAN_API_KEY")
	if apiKey == "" {
		log.Println("HUNYUAN_API_KEY not set, skipping")
		return
	}

	provider := hunyuan.NewHunyuanProvider(providers.HunyuanConfig{
		APIKey:  apiKey,
		Model:   "hunyuan-lite",
		Timeout: 30 * time.Second,
	}, logger)

	// Health check
	status, err := provider.HealthCheck(ctx)
	if err != nil {
		log.Printf("Health check failed: %v\n", err)
		return
	}
	fmt.Printf("Health: %v, Latency: %v\n", status.Healthy, status.Latency)

	// Chat completion
	req := &llm.ChatRequest{
		Model: "hunyuan-lite",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "介绍一下北京"},
		},
		MaxTokens:   100,
		Temperature: 0.7,
	}

	resp, err := provider.Completion(ctx, req)
	if err != nil {
		log.Printf("Completion failed: %v\n", err)
		return
	}

	fmt.Printf("Response: %s\n", resp.Choices[0].Message.Content)
	fmt.Printf("Usage: %d tokens\n", resp.Usage.TotalTokens)
}

func testKimi(ctx context.Context, logger *zap.Logger) {
	apiKey := os.Getenv("KIMI_API_KEY")
	if apiKey == "" {
		log.Println("KIMI_API_KEY not set, skipping")
		return
	}

	provider := kimi.NewKimiProvider(providers.KimiConfig{
		APIKey:  apiKey,
		Model:   "moonshot-v1-8k",
		Timeout: 30 * time.Second,
	}, logger)

	// Health check
	status, err := provider.HealthCheck(ctx)
	if err != nil {
		log.Printf("Health check failed: %v\n", err)
		return
	}
	fmt.Printf("Health: %v, Latency: %v\n", status.Healthy, status.Latency)

	// Chat completion
	req := &llm.ChatRequest{
		Model: "moonshot-v1-8k",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "什么是月之暗面？"},
		},
		MaxTokens:   100,
		Temperature: 0.7,
	}

	resp, err := provider.Completion(ctx, req)
	if err != nil {
		log.Printf("Completion failed: %v\n", err)
		return
	}

	fmt.Printf("Response: %s\n", resp.Choices[0].Message.Content)
	fmt.Printf("Usage: %d tokens\n", resp.Usage.TotalTokens)
}

func testLlama(ctx context.Context, logger *zap.Logger) {
	apiKey := os.Getenv("TOGETHER_API_KEY")
	if apiKey == "" {
		log.Println("TOGETHER_API_KEY not set, skipping")
		return
	}

	provider := llama.NewLlamaProvider(providers.LlamaConfig{
		APIKey:   apiKey,
		Model:    "meta-llama/Llama-3.3-70B-Instruct-Turbo",
		Provider: "together",
		Timeout:  30 * time.Second,
	}, logger)

	// Health check
	status, err := provider.HealthCheck(ctx)
	if err != nil {
		log.Printf("Health check failed: %v\n", err)
		return
	}
	fmt.Printf("Health: %v, Latency: %v\n", status.Healthy, status.Latency)

	// Chat completion
	req := &llm.ChatRequest{
		Model: "meta-llama/Llama-3.3-70B-Instruct-Turbo",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "What is Meta's Llama model?"},
		},
		MaxTokens:   100,
		Temperature: 0.7,
	}

	resp, err := provider.Completion(ctx, req)
	if err != nil {
		log.Printf("Completion failed: %v\n", err)
		return
	}

	fmt.Printf("Response: %s\n", resp.Choices[0].Message.Content)
	fmt.Printf("Usage: %d tokens\n", resp.Usage.TotalTokens)
}
