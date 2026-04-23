package usecase

import (
	"context"
	"encoding/json"
	"testing"

	agent "github.com/BaSui01/agentflow/agent/runtime"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	llmgateway "github.com/BaSui01/agentflow/llm/gateway"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

func testGatewayFromProvider(provider llmcore.Provider) llmcore.Gateway {
	if provider == nil {
		return nil
	}
	return llmgateway.New(llmgateway.Config{
		ChatProvider: provider,
		Logger:       zap.NewNop(),
	})
}

func TestDefaultAgentService_ExecuteAgent_InjectsRuntimeHandoffTargets(t *testing.T) {
	var sawHandoffTool bool

	source := agent.BuildBaseAgent(testAgentConfigForUsecase("source-agent", "Source", "gpt-4o-mini"), testGatewayFromProvider(&usecaseTestProvider{
		name:           "source",
		supportsNative: true,
		completionFn: func(ctx context.Context, req *llmcore.ChatRequest) (*llmcore.ChatResponse, error) {
			for _, tool := range req.Tools {
				if tool.Name == "transfer_to_target_agent" {
					sawHandoffTool = true
				}
			}
			return &llmcore.ChatResponse{
				ID:       "resp-1",
				Provider: "source",
				Model:    "gpt-4o-mini",
				Choices: []llmcore.ChatChoice{{
					Index:        0,
					FinishReason: "stop",
					Message: types.Message{
						Role:    llmcore.RoleAssistant,
						Content: "ok",
					},
				}},
				Usage: llmcore.ChatUsage{TotalTokens: 8},
			}, nil
		},
	}), nil, nil, nil, zap.NewNop(), nil)
	target := agent.BuildBaseAgent(testAgentConfigForUsecase("target-agent", "Target", "gpt-4.1"), testGatewayFromProvider(&usecaseTestProvider{
		name:           "target",
		supportsNative: true,
	}), nil, nil, nil, zap.NewNop(), nil)

	if err := source.Init(context.Background()); err != nil {
		t.Fatalf("source init failed: %v", err)
	}
	if err := target.Init(context.Background()); err != nil {
		t.Fatalf("target init failed: %v", err)
	}

	svc := NewDefaultAgentService(nil, func(ctx context.Context, agentID string) (agent.Agent, error) {
		switch agentID {
		case "source-agent":
			return source, nil
		case "target-agent":
			return target, nil
		default:
			return nil, assertError("not found")
		}
	})

	_, _, err := svc.ExecuteAgent(context.Background(), AgentExecuteRequest{
		AgentID: "source-agent",
		Content: "delegate if needed",
		Context: map[string]any{
			"handoff_agents": []any{"target-agent"},
		},
	}, "trace-1")
	if err != nil {
		t.Fatalf("ExecuteAgent returned error: %v", err)
	}
	if !sawHandoffTool {
		t.Fatalf("expected synthetic handoff tool to be injected into ChatRequest")
	}
}

func TestDefaultAgentService_ExecuteAgent_InjectsConfigLevelHandoffTargets(t *testing.T) {
	var sawHandoffTool bool

	sourceCfg := testAgentConfigForUsecase("source-agent", "Source", "gpt-4o-mini")
	sourceCfg.Runtime.Handoffs = []string{"target-agent", "target-agent"}
	source := agent.BuildBaseAgent(sourceCfg, testGatewayFromProvider(&usecaseTestProvider{
		name:           "source",
		supportsNative: true,
		completionFn: func(ctx context.Context, req *llmcore.ChatRequest) (*llmcore.ChatResponse, error) {
			for _, tool := range req.Tools {
				if tool.Name == "transfer_to_target_agent" {
					sawHandoffTool = true
				}
			}
			return &llmcore.ChatResponse{
				ID:       "resp-2",
				Provider: "source",
				Model:    "gpt-4o-mini",
				Choices: []llmcore.ChatChoice{{
					Index:        0,
					FinishReason: "stop",
					Message:      types.Message{Role: llmcore.RoleAssistant, Content: "ok"},
				}},
				Usage: llmcore.ChatUsage{TotalTokens: 8},
			}, nil
		},
	}), nil, nil, nil, zap.NewNop(), nil)
	target := agent.BuildBaseAgent(testAgentConfigForUsecase("target-agent", "Target", "gpt-4.1"), testGatewayFromProvider(&usecaseTestProvider{
		name:           "target",
		supportsNative: true,
	}), nil, nil, nil, zap.NewNop(), nil)

	if err := source.Init(context.Background()); err != nil {
		t.Fatalf("source init failed: %v", err)
	}
	if err := target.Init(context.Background()); err != nil {
		t.Fatalf("target init failed: %v", err)
	}

	svc := NewDefaultAgentService(nil, func(ctx context.Context, agentID string) (agent.Agent, error) {
		switch agentID {
		case "source-agent":
			return source, nil
		case "target-agent":
			return target, nil
		default:
			return nil, assertError("not found")
		}
	})

	_, _, err := svc.ExecuteAgent(context.Background(), AgentExecuteRequest{
		AgentID: "source-agent",
		Content: "delegate if needed",
	}, "trace-2")
	if err != nil {
		t.Fatalf("ExecuteAgent returned error: %v", err)
	}
	if !sawHandoffTool {
		t.Fatalf("expected config-level handoff target to be injected into ChatRequest")
	}
}

type usecaseTestProvider struct {
	name           string
	completionFn   func(ctx context.Context, req *llmcore.ChatRequest) (*llmcore.ChatResponse, error)
	supportsNative bool
}

func (p *usecaseTestProvider) Completion(ctx context.Context, req *llmcore.ChatRequest) (*llmcore.ChatResponse, error) {
	if p.completionFn != nil {
		return p.completionFn(ctx, req)
	}
	return nil, nil
}

func (p *usecaseTestProvider) Stream(context.Context, *llmcore.ChatRequest) (<-chan llmcore.StreamChunk, error) {
	ch := make(chan llmcore.StreamChunk)
	close(ch)
	return ch, nil
}

func (p *usecaseTestProvider) HealthCheck(context.Context) (*llmcore.HealthStatus, error) {
	return &llmcore.HealthStatus{Healthy: true}, nil
}

func (p *usecaseTestProvider) Name() string { return p.name }
func (p *usecaseTestProvider) SupportsNativeFunctionCalling() bool {
	return p.supportsNative
}
func (p *usecaseTestProvider) ListModels(context.Context) ([]llmcore.Model, error) { return nil, nil }
func (p *usecaseTestProvider) Endpoints() llmcore.ProviderEndpoints {
	return llmcore.ProviderEndpoints{}
}

func testAgentConfigForUsecase(id, name, model string) types.AgentConfig {
	return types.AgentConfig{
		Core: types.CoreConfig{
			ID:   id,
			Name: name,
			Type: string(agent.TypeGeneric),
		},
		LLM: types.LLMConfig{Model: model},
	}
}

type assertError string

func (e assertError) Error() string { return string(e) }

var _ json.Marshaler
