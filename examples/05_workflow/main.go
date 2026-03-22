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
	"github.com/BaSui01/agentflow/workflow"
	"go.uber.org/zap"
)

func main() {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENAI_API_KEY environment variable is required")
	}

	baseURL := envOrDefault("OPENAI_BASE_URL", "https://api.openai.com")
	model := envOrDefault("OPENAI_MODEL", "gpt-4o-mini")

	provider := openai.NewOpenAIProvider(providers.OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			APIKey:  apiKey,
			BaseURL: baseURL,
			Model:   model,
		},
	}, logger)

	ctx := context.Background()
	if err := runDAGWorkflow(ctx, provider, baseURL, model); err != nil {
		log.Fatalf("workflow execution failed: %v", err)
	}
}

func runDAGWorkflow(ctx context.Context, provider llm.Provider, baseURL, model string) error {
	fmt.Println("=== AgentFlow DAG Workflow 示例 ===")
	fmt.Printf("Base URL: %s\n", baseURL)
	fmt.Printf("Model: %s\n", model)

	translateStep := workflow.NewFuncStep("translate", func(ctx context.Context, input any) (any, error) {
		text := input.(string)
		resp, err := provider.Completion(ctx, &llm.ChatRequest{
			Model: model,
			Messages: []types.Message{
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

	summarizeStep := workflow.NewFuncStep("summarize", func(ctx context.Context, input any) (any, error) {
		text := input.(string)
		resp, err := provider.Completion(ctx, &llm.ChatRequest{
			Model: model,
			Messages: []types.Message{
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

	graph := workflow.NewDAGGraph()
	graph.AddNode(&workflow.DAGNode{ID: "translate", Type: workflow.NodeTypeAction, Step: translateStep})
	graph.AddNode(&workflow.DAGNode{ID: "summarize", Type: workflow.NodeTypeAction, Step: summarizeStep})
	graph.AddEdge("translate", "summarize")
	graph.SetEntry("translate")

	wf := workflow.NewDAGWorkflow("translate-and-summarize", "翻译并总结文章", graph)

	input := "Artificial Intelligence is transforming the world. Machine learning algorithms can now recognize patterns, make predictions, and even create art. The future of AI is both exciting and challenging."
	fmt.Printf("原文：\n%s\n\n", input)

	result, err := wf.Execute(ctx, input)
	if err != nil {
		return err
	}

	fmt.Printf("结果：\n%s\n", result)
	return nil
}

func envOrDefault(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
