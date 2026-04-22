package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/agent"
	mcpproto "github.com/BaSui01/agentflow/agent/execution/protocol/mcp"
	agentruntime "github.com/BaSui01/agentflow/agent/execution/runtime"
	"github.com/BaSui01/agentflow/llm"
	llmgateway "github.com/BaSui01/agentflow/llm/gateway"
	"github.com/BaSui01/agentflow/rag"
	"github.com/BaSui01/agentflow/rag/core"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

type faultProvider struct {
	name       string
	completion func(context.Context, *llm.ChatRequest) (*llm.ChatResponse, error)
	listModels func(context.Context) ([]llm.Model, error)
}

func (p *faultProvider) Completion(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	if p.completion != nil {
		return p.completion(ctx, req)
	}
	return mockChatResponse(req.Model, "OK"), nil
}

func (p *faultProvider) Stream(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	ch := make(chan llm.StreamChunk)
	go func() {
		defer close(ch)
		select {
		case <-ctx.Done():
		default:
		}
	}()
	return ch, nil
}

func (p *faultProvider) HealthCheck(ctx context.Context) (*llm.HealthStatus, error) {
	return &llm.HealthStatus{Healthy: true, Message: "fault-provider"}, nil
}

func (p *faultProvider) Name() string {
	if strings.TrimSpace(p.name) == "" {
		return "fault-provider"
	}
	return p.name
}

func (p *faultProvider) SupportsNativeFunctionCalling() bool {
	return true
}

func (p *faultProvider) ListModels(ctx context.Context) ([]llm.Model, error) {
	if p.listModels != nil {
		return p.listModels(ctx)
	}
	return []llm.Model{{ID: "fault-model"}}, nil
}

func (p *faultProvider) Endpoints() llm.ProviderEndpoints {
	return llm.ProviderEndpoints{
		BaseURL:    "fault://provider",
		Completion: "fault://provider/completions",
		Models:     "fault://provider/models",
	}
}

func mockChatResponse(model string, content string) *llm.ChatResponse {
	if strings.TrimSpace(model) == "" {
		model = "fault-model"
	}
	return &llm.ChatResponse{
		ID:       "fault-response",
		Provider: "fault-provider",
		Model:    model,
		Choices: []llm.ChatChoice{
			{
				Index:        0,
				FinishReason: "stop",
				Message: types.Message{
					Role:    llm.RoleAssistant,
					Content: content,
				},
			},
		},
		Usage: llm.ChatUsage{
			PromptTokens:     1,
			CompletionTokens: 1,
			TotalTokens:      2,
		},
		CreatedAt: time.Now(),
	}
}

func runFaultTimeout(ctx context.Context, logger *zap.Logger) error {
	logger.Info("fault X1: timeout start")

	provider := &faultProvider{
		name: "fault-timeout-provider",
		completion: func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(2 * time.Second):
				return mockChatResponse(req.Model, "late-response"), nil
			}
		},
	}

	cfg := types.AgentConfig{
		Core: types.CoreConfig{
			ID:   "fault-timeout-agent",
			Name: "fault-timeout-agent",
			Type: "assistant",
		},
		LLM: types.LLMConfig{
			Model:       "fault-model",
			MaxTokens:   64,
			Temperature: 0,
		},
		Runtime: types.RuntimeConfig{
			SystemPrompt: "Return one short answer.",
		},
	}

	gateway := llmgateway.New(llmgateway.Config{ChatProvider: provider, Logger: logger})
	ag, err := agentruntime.NewBuilder(gateway, logger).Build(ctx, cfg)
	if err != nil {
		return fmt.Errorf("build timeout fault agent: %w", err)
	}
	defer ag.Teardown(context.Background())

	if err := ag.Init(ctx); err != nil {
		return fmt.Errorf("init timeout fault agent: %w", err)
	}

	execCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	_, err = ag.Execute(execCtx, &agent.Input{
		TraceID: "fault-timeout-trace",
		Content: "reply ok",
	})
	if err == nil {
		return fmt.Errorf("expected timeout error, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) && !strings.Contains(strings.ToLower(err.Error()), "deadline") {
		return fmt.Errorf("expected timeout-related error, got: %w", err)
	}

	logger.Info("fault X1: timeout validated", zap.Error(err))
	return nil
}

func runFaultEmptyModelList(ctx context.Context, logger *zap.Logger) error {
	logger.Info("fault X2: empty model list start")

	provider := &faultProvider{
		name: "fault-empty-model-provider",
		listModels: func(ctx context.Context) ([]llm.Model, error) {
			return []llm.Model{}, nil
		},
	}

	models, err := provider.ListModels(ctx)
	if err != nil {
		return fmt.Errorf("ListModels returned unexpected error: %w", err)
	}
	if len(models) != 0 {
		return fmt.Errorf("expected empty model list, got %d", len(models))
	}

	_, err = pickChatModel(models)
	if err == nil {
		return fmt.Errorf("expected model selection error for empty model list, got nil")
	}

	logger.Info("fault X2: empty model list validated", zap.Error(err))
	return nil
}

func runFaultMCPToolError(ctx context.Context, logger *zap.Logger) error {
	logger.Info("fault X3: MCP tool error start")

	mcpServer := mcpproto.NewMCPServer("fault-mcp", "0.1.0", logger)
	defer mcpServer.Close()

	if err := mcpServer.RegisterTool(&mcpproto.ToolDefinition{
		Name:        "always_fail",
		Description: "Always returns an injected error",
		InputSchema: map[string]any{
			"type": "object",
		},
	}, func(ctx context.Context, args map[string]any) (any, error) {
		return nil, errors.New("injected mcp tool failure")
	}); err != nil {
		return fmt.Errorf("register fault MCP tool: %w", err)
	}

	resp, err := mcpServer.HandleMessage(ctx, mcpproto.NewMCPRequest("fault-call-1", "tools/call", map[string]any{
		"name":      "always_fail",
		"arguments": map[string]any{},
	}))
	if err != nil {
		return fmt.Errorf("mcp tools/call transport error: %w", err)
	}
	if resp == nil {
		return fmt.Errorf("expected mcp error response, got nil")
	}
	if resp.Error == nil {
		return fmt.Errorf("expected mcp error response, got success result: %+v", resp.Result)
	}

	logger.Info("fault X3: MCP tool error validated",
		zap.Int("code", resp.Error.Code),
		zap.String("message", resp.Error.Message),
	)
	return nil
}

func runFaultRAGDimensionMismatch(ctx context.Context, logger *zap.Logger) error {
	logger.Info("fault X4: RAG dimension mismatch start")

	store := rag.NewMilvusStore(rag.MilvusConfig{
		Collection: "fault_dimension_check",
	}, logger)

	err := store.AddDocuments(ctx, []core.Document{
		{
			ID:        "doc-1",
			Content:   "ok embedding",
			Embedding: []float64{0.1, 0.2, 0.3, 0.4},
		},
		{
			ID:        "doc-2",
			Content:   "bad embedding",
			Embedding: []float64{0.1, 0.2, 0.3},
		},
	})
	if err == nil {
		return fmt.Errorf("expected dimension mismatch error, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "dimension mismatch") {
		return fmt.Errorf("expected dimension mismatch error, got: %w", err)
	}

	logger.Info("fault X4: dimension mismatch validated", zap.Error(err))
	return nil
}
