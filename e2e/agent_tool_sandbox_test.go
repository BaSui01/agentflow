package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/agent/discovery"
	"github.com/BaSui01/agentflow/agent/execution"
	"github.com/BaSui01/agentflow/agent/protocol/a2a"
	"github.com/BaSui01/agentflow/api"
	"github.com/BaSui01/agentflow/api/handlers"
	"github.com/BaSui01/agentflow/api/routes"
	"github.com/BaSui01/agentflow/internal/usecase"
	"github.com/BaSui01/agentflow/llm"
	llmtools "github.com/BaSui01/agentflow/llm/capabilities/tools"
	"github.com/BaSui01/agentflow/testutil/mocks"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// toolCallProvider returns a tool call response for the first turn, then a final answer.
type toolCallProvider struct {
	turn int
}

func (p *toolCallProvider) Completion(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	p.turn++
	if p.turn == 1 {
		return &llm.ChatResponse{
			Model: req.Model,
			Choices: []llm.ChatChoice{
				{
					Index: 0,
					Message: types.Message{
						Role: types.RoleAssistant,
						ToolCalls: []types.ToolCall{
							{
								ID:        "call-1",
								Name:      "code_execution",
								Arguments: []byte(`{"language":"python","code":"print('hello from sandbox')"}`),
							},
						},
					},
				},
			},
		}, nil
	}
	return &llm.ChatResponse{
		Model: req.Model,
		Choices: []llm.ChatChoice{
			{
				Index: 0,
				Message: types.Message{
					Role:    types.RoleAssistant,
					Content: "sandbox execution completed",
				},
			},
		},
	}, nil
}

func (p *toolCallProvider) Stream(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	ch := make(chan llm.StreamChunk, 1)
	close(ch)
	return ch, nil
}
func (p *toolCallProvider) HealthCheck(ctx context.Context) (*llm.HealthStatus, error) {
	return &llm.HealthStatus{Healthy: true}, nil
}
func (p *toolCallProvider) Name() string                        { return "tool-call-provider" }
func (p *toolCallProvider) SupportsNativeFunctionCalling() bool { return true }
func (p *toolCallProvider) ListModels(ctx context.Context) ([]llm.Model, error) {
	return []llm.Model{{ID: "test-model"}}, nil
}
func (p *toolCallProvider) Endpoints() llm.ProviderEndpoints { return llm.ProviderEndpoints{} }

// simpleToolManager wraps a single sandbox tool for e2e tests.
type simpleToolManager struct {
	tool *execution.SandboxTool
}

func (m *simpleToolManager) GetAllowedTools(agentID string) []types.ToolSchema {
	return []types.ToolSchema{
		{
			Name:        "code_execution",
			Description: "Execute code in a sandbox",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"language":{"type":"string"},"code":{"type":"string"}},"required":["language","code"]}`),
		},
	}
}

func (m *simpleToolManager) ExecuteForAgent(ctx context.Context, agentID string, calls []types.ToolCall) []llmtools.ToolResult {
	results := make([]llmtools.ToolResult, len(calls))
	for i, call := range calls {
		raw, err := m.tool.Execute(ctx, call.Arguments)
		results[i] = llmtools.ToolResult{
			ToolCallID: call.ID,
			Name:       call.Name,
			Result:     raw,
		}
		if err != nil {
			results[i].Error = err.Error()
		}
	}
	return results
}

func buildSandboxE2EAgent(t *testing.T, agentID string) agent.Agent {
	t.Helper()
	logger := zap.NewNop()
	provider := &toolCallProvider{}

	cfg := types.AgentConfig{
		Core: types.CoreConfig{
			ID:   agentID,
			Name: "e2e-sandbox-agent",
		},
		LLM: types.LLMConfig{
			Model: "test-model",
		},
	}

	// Use process backend so the test can execute without Docker.
	backend := execution.NewProcessBackendWithConfig(nil, execution.ProcessBackendConfig{
		Enabled: true,
		CustomInterpreters: map[execution.Language]string{
			execution.LangPython: "python",
		},
	})
	exec := execution.NewSandboxExecutor(execution.DefaultSandboxConfig(), backend, logger)
	tool := execution.NewSandboxTool(exec, logger)

	ag, err := agent.NewAgentBuilder(cfg).
		WithProvider(provider).
		WithToolProvider(mocks.NewSuccessProvider("tool reasoning")).
		WithToolManager(&simpleToolManager{tool: tool}).
		WithLogger(logger).
		Build()
	require.NoError(t, err)
	require.NoError(t, ag.Init(context.Background()))
	return ag
}

func TestE2E_AgentWithSandboxTool_Execute(t *testing.T) {
	agentID := "e2e-sandbox-agent"
	ag := buildSandboxE2EAgent(t, agentID)

	ctx := context.Background()
	reg := newE2ERegistry()
	_ = reg.RegisterAgent(ctx, &discovery.AgentInfo{
		Card: &a2a.AgentCard{Name: agentID},
	})

	resolver := func(ctx context.Context, id string) (agent.Agent, error) {
		if id == agentID {
			return ag, nil
		}
		return nil, fmt.Errorf("agent not found")
	}

	logger := zap.NewNop()
	agentRegistry := agent.NewAgentRegistry(logger)
	agentHandler := handlers.NewAgentHandler(reg, agentRegistry, logger, resolver)
	healthHandler := handlers.NewHealthHandler(logger)

	mux := http.NewServeMux()
	routes.RegisterSystem(mux, healthHandler, "e2e", "", "")
	routes.RegisterAgent(mux, agentHandler, logger)
	server := httptest.NewServer(mux)
	defer server.Close()

	reqBody, _ := json.Marshal(usecase.AgentExecuteRequest{
		AgentID: agentID,
		Content: "run python code",
	})

	resp, err := http.Post(server.URL+"/api/v1/agents/execute", "application/json", bytes.NewReader(reqBody))
	require.NoError(t, err)
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	require.Equal(t, http.StatusOK, resp.StatusCode, "response body: %s", string(bodyBytes))

	var apiResp api.Response
	require.NoError(t, json.Unmarshal(bodyBytes, &apiResp))
	assert.True(t, apiResp.Success)

	data, ok := apiResp.Data.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "sandbox execution completed", data["content"])
}
