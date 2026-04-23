package benchmarks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/agent/capabilities/tools"
	"github.com/BaSui01/agentflow/agent/execution/protocol/a2a"
	agent "github.com/BaSui01/agentflow/agent/execution/runtime"
	agentruntime "github.com/BaSui01/agentflow/agent/execution/runtime"
	"github.com/BaSui01/agentflow/api/handlers"
	"github.com/BaSui01/agentflow/api/routes"
	"github.com/BaSui01/agentflow/internal/usecase"
	llmgateway "github.com/BaSui01/agentflow/llm/gateway"
	"github.com/BaSui01/agentflow/testutil/mocks"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

type benchRegistry struct {
	mu     sync.RWMutex
	agents map[string]*tools.AgentInfo
}

func newBenchRegistry() *benchRegistry {
	return &benchRegistry{agents: make(map[string]*tools.AgentInfo)}
}

func (r *benchRegistry) RegisterAgent(_ context.Context, info *tools.AgentInfo) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.agents[info.Card.Name] = info
	return nil
}
func (r *benchRegistry) UnregisterAgent(_ context.Context, agentID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.agents, agentID)
	return nil
}
func (r *benchRegistry) UpdateAgent(_ context.Context, info *tools.AgentInfo) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.agents[info.Card.Name] = info
	return nil
}
func (r *benchRegistry) GetAgent(_ context.Context, agentID string) (*tools.AgentInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	info, ok := r.agents[agentID]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return info, nil
}
func (r *benchRegistry) ListAgents(_ context.Context) ([]*tools.AgentInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*tools.AgentInfo, 0, len(r.agents))
	for _, info := range r.agents {
		out = append(out, info)
	}
	return out, nil
}
func (r *benchRegistry) RegisterCapability(_ context.Context, _ string, _ *tools.CapabilityInfo) error {
	return nil
}
func (r *benchRegistry) UnregisterCapability(_ context.Context, _, _ string) error { return nil }
func (r *benchRegistry) UpdateCapability(_ context.Context, _ string, _ *tools.CapabilityInfo) error {
	return nil
}
func (r *benchRegistry) GetCapability(_ context.Context, _, _ string) (*tools.CapabilityInfo, error) {
	return nil, nil
}
func (r *benchRegistry) ListCapabilities(_ context.Context, _ string) ([]tools.CapabilityInfo, error) {
	return nil, nil
}
func (r *benchRegistry) FindCapabilities(_ context.Context, _ string) ([]tools.CapabilityInfo, error) {
	return nil, nil
}
func (r *benchRegistry) UpdateAgentStatus(_ context.Context, _ string, _ tools.AgentStatus) error {
	return nil
}
func (r *benchRegistry) UpdateAgentLoad(_ context.Context, _ string, _ float64) error { return nil }
func (r *benchRegistry) RecordExecution(_ context.Context, _ string, _ string, _ bool, _ time.Duration) error {
	return nil
}
func (r *benchRegistry) Subscribe(_ tools.DiscoveryEventHandler) string { return "" }
func (r *benchRegistry) Unsubscribe(_ string)                           {}
func (r *benchRegistry) Close() error                                   { return nil }

var _ tools.Registry = (*benchRegistry)(nil)

func buildBenchAgent(agentID string, maxConcurrency int) agent.Agent {
	logger := zap.NewNop()
	provider := mocks.NewSuccessProvider("benchmark reply")
	cfg := types.AgentConfig{
		Core: types.CoreConfig{ID: agentID, Name: "bench-agent"},
		LLM:  types.LLMConfig{Model: "test-model"},
	}
	gateway := llmgateway.New(llmgateway.Config{ChatProvider: provider, Logger: logger})
	ag, _ := agentruntime.NewBuilder(gateway, logger).WithOptions(agentruntime.BuildOptions{
		MaxConcurrency: maxConcurrency,
	}).Build(context.Background(), cfg)
	_ = ag.Init(context.Background())
	return ag
}

func setupBenchServer(resolver usecase.AgentResolver) *httptest.Server {
	logger := zap.NewNop()
	registry := newBenchRegistry()
	agentHandler := handlers.NewAgentHandlerWithService(usecase.NewDefaultAgentService(registry, resolver), nil, logger)
	healthHandler := handlers.NewHealthHandler(logger)
	mux := http.NewServeMux()
	routes.RegisterSystem(mux, healthHandler, "bench", time.Now().Format(time.RFC3339), "HEAD")
	routes.RegisterAgent(mux, agentHandler, logger)
	return httptest.NewServer(mux)
}

func BenchmarkAgentExecute_Serial(b *testing.B) {
	agentID := "bench-serial"
	ag := buildBenchAgent(agentID, 1)
	_ = newBenchRegistry().RegisterAgent(context.Background(), &tools.AgentInfo{
		Card: &a2a.AgentCard{Name: agentID},
	})

	resolver := func(ctx context.Context, id string) (agent.Agent, error) {
		if id == agentID {
			return ag, nil
		}
		return nil, fmt.Errorf("not found")
	}
	server := setupBenchServer(resolver)
	defer server.Close()

	reqBody, _ := json.Marshal(usecase.AgentExecuteRequest{AgentID: agentID, Content: "hello"})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, _ := http.Post(server.URL+"/api/v1/agents/execute", "application/json", bytes.NewReader(reqBody))
		if resp != nil {
			resp.Body.Close()
		}
	}
}

func BenchmarkAgentExecute_Concurrent5(b *testing.B) {
	agentID := "bench-concurrent-5"
	ag := buildBenchAgent(agentID, 5)
	_ = newBenchRegistry().RegisterAgent(context.Background(), &tools.AgentInfo{
		Card: &a2a.AgentCard{Name: agentID},
	})

	resolver := func(ctx context.Context, id string) (agent.Agent, error) {
		if id == agentID {
			return ag, nil
		}
		return nil, fmt.Errorf("not found")
	}
	server := setupBenchServer(resolver)
	defer server.Close()

	reqBody, _ := json.Marshal(usecase.AgentExecuteRequest{AgentID: agentID, Content: "hello"})

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			resp, _ := http.Post(server.URL+"/api/v1/agents/execute", "application/json", bytes.NewReader(reqBody))
			if resp != nil {
				resp.Body.Close()
			}
		}
	})
}

func BenchmarkAgentExecute_Concurrent10(b *testing.B) {
	agentID := "bench-concurrent-10"
	ag := buildBenchAgent(agentID, 10)
	_ = newBenchRegistry().RegisterAgent(context.Background(), &tools.AgentInfo{
		Card: &a2a.AgentCard{Name: agentID},
	})

	resolver := func(ctx context.Context, id string) (agent.Agent, error) {
		if id == agentID {
			return ag, nil
		}
		return nil, fmt.Errorf("not found")
	}
	server := setupBenchServer(resolver)
	defer server.Close()

	reqBody, _ := json.Marshal(usecase.AgentExecuteRequest{AgentID: agentID, Content: "hello"})

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			resp, _ := http.Post(server.URL+"/api/v1/agents/execute", "application/json", bytes.NewReader(reqBody))
			if resp != nil {
				resp.Body.Close()
			}
		}
	})
}
