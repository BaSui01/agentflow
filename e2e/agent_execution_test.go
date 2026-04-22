package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/agent/capabilities/tools"
	"github.com/BaSui01/agentflow/agent/execution/protocol/a2a"
	agentruntime "github.com/BaSui01/agentflow/agent/execution/runtime"
	"github.com/BaSui01/agentflow/api"
	"github.com/BaSui01/agentflow/api/handlers"
	"github.com/BaSui01/agentflow/api/routes"
	"github.com/BaSui01/agentflow/internal/usecase"
	llmgateway "github.com/BaSui01/agentflow/llm/gateway"
	"github.com/BaSui01/agentflow/testutil/mocks"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// e2eRegistry is a simple in-memory discovery registry for end-to-end tests.
type e2eRegistry struct {
	mu     sync.RWMutex
	agents map[string]*tools.AgentInfo
}

func newE2ERegistry() *e2eRegistry {
	return &e2eRegistry{agents: make(map[string]*tools.AgentInfo)}
}

func (r *e2eRegistry) RegisterAgent(_ context.Context, info *tools.AgentInfo) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.agents[info.Card.Name] = info
	return nil
}

func (r *e2eRegistry) UnregisterAgent(_ context.Context, agentID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.agents, agentID)
	return nil
}

func (r *e2eRegistry) UpdateAgent(_ context.Context, info *tools.AgentInfo) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.agents[info.Card.Name] = info
	return nil
}

func (r *e2eRegistry) GetAgent(_ context.Context, agentID string) (*tools.AgentInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	info, ok := r.agents[agentID]
	if !ok {
		return nil, assert.AnError
	}
	return info, nil
}

func (r *e2eRegistry) ListAgents(_ context.Context) ([]*tools.AgentInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*tools.AgentInfo, 0, len(r.agents))
	for _, info := range r.agents {
		out = append(out, info)
	}
	return out, nil
}

func (r *e2eRegistry) RegisterCapability(_ context.Context, _ string, _ *tools.CapabilityInfo) error {
	return nil
}
func (r *e2eRegistry) UnregisterCapability(_ context.Context, _, _ string) error { return nil }
func (r *e2eRegistry) UpdateCapability(_ context.Context, _ string, _ *tools.CapabilityInfo) error {
	return nil
}
func (r *e2eRegistry) GetCapability(_ context.Context, _, _ string) (*tools.CapabilityInfo, error) {
	return nil, nil
}
func (r *e2eRegistry) ListCapabilities(_ context.Context, _ string) ([]tools.CapabilityInfo, error) {
	return nil, nil
}
func (r *e2eRegistry) FindCapabilities(_ context.Context, _ string) ([]tools.CapabilityInfo, error) {
	return nil, nil
}
func (r *e2eRegistry) UpdateAgentStatus(_ context.Context, _ string, _ tools.AgentStatus) error {
	return nil
}
func (r *e2eRegistry) UpdateAgentLoad(_ context.Context, _ string, _ float64) error { return nil }
func (r *e2eRegistry) RecordExecution(_ context.Context, _ string, _ string, _ bool, _ time.Duration) error {
	return nil
}
func (r *e2eRegistry) Subscribe(_ tools.DiscoveryEventHandler) string { return "" }
func (r *e2eRegistry) Unsubscribe(_ string)                           {}
func (r *e2eRegistry) Close() error                                   { return nil }

var _ tools.Registry = (*e2eRegistry)(nil)

// buildE2EAgent creates a real BaseAgent using the fluent builder.
func buildE2EAgent(t *testing.T, agentID string, maxConcurrency int) agent.Agent {
	t.Helper()
	logger := zap.NewNop()
	provider := mocks.NewSuccessProvider("hello from e2e agent")

	cfg := types.AgentConfig{
		Core: types.CoreConfig{
			ID:   agentID,
			Name: "e2e-test-agent",
		},
		LLM: types.LLMConfig{
			Model: "test-model",
		},
	}
	gateway := llmgateway.New(llmgateway.Config{ChatProvider: provider, Logger: logger})
	ag, err := agentruntime.NewBuilder(gateway, logger).WithOptions(agentruntime.BuildOptions{
		MaxConcurrency: maxConcurrency,
	}).Build(context.Background(), cfg)
	require.NoError(t, err)
	require.NoError(t, ag.Init(context.Background()))
	return ag
}

// setupE2EServer wires handlers and returns an httptest.Server.
func setupE2EServer(t *testing.T, resolver usecase.AgentResolver) *httptest.Server {
	t.Helper()
	logger := zap.NewNop()
	registry := newE2ERegistry()
	agentHandler := handlers.NewAgentHandlerWithService(usecase.NewDefaultAgentService(registry, resolver), nil, logger)
	healthHandler := handlers.NewHealthHandler(logger)

	mux := http.NewServeMux()
	routes.RegisterSystem(mux, healthHandler, "e2e", time.Now().Format(time.RFC3339), "HEAD")
	routes.RegisterAgent(mux, agentHandler, logger)

	return httptest.NewServer(mux)
}

func TestE2E_AgentExecute_Success(t *testing.T) {
	agentID := "e2e-agent-1"
	ag := buildE2EAgent(t, agentID, 1)

	// Register agent in discovery registry
	ctx := context.Background()
	reg := newE2ERegistry()
	_ = reg.RegisterAgent(ctx, &tools.AgentInfo{
		Card: &a2a.AgentCard{Name: agentID},
	})

	resolver := func(ctx context.Context, id string) (agent.Agent, error) {
		if id == agentID {
			return ag, nil
		}
		return nil, assert.AnError
	}

	server := setupE2EServer(t, resolver)
	defer server.Close()

	reqBody, _ := json.Marshal(usecase.AgentExecuteRequest{
		AgentID: agentID,
		Content: "say hello",
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
	assert.Equal(t, "hello from e2e agent", data["content"])
}

func TestE2E_AgentExecute_ConcurrentRequests(t *testing.T) {
	agentID := "e2e-agent-concurrent"
	ag := buildE2EAgent(t, agentID, 5)

	ctx := context.Background()
	reg := newE2ERegistry()
	_ = reg.RegisterAgent(ctx, &tools.AgentInfo{
		Card: &a2a.AgentCard{Name: agentID},
	})

	resolver := func(ctx context.Context, id string) (agent.Agent, error) {
		if id == agentID {
			return ag, nil
		}
		return nil, assert.AnError
	}

	server := setupE2EServer(t, resolver)
	defer server.Close()

	const n = 10
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() {
			reqBody, _ := json.Marshal(usecase.AgentExecuteRequest{
				AgentID: agentID,
				Content: "concurrent request",
			})
			resp, err := http.Post(server.URL+"/api/v1/agents/execute", "application/json", bytes.NewReader(reqBody))
			if err != nil {
				errs <- err
				return
			}
			bodyBytes, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				errs <- fmt.Errorf("status %d: %s", resp.StatusCode, string(bodyBytes))
				return
			}
			errs <- nil
		}()
	}

	for i := 0; i < n; i++ {
		require.NoError(t, <-errs)
	}
}
