package agent

import (
	"context"
	"sync"

	"github.com/BaSui01/agentflow/llm"
	llmtools "github.com/BaSui01/agentflow/llm/tools"
)

// testProvider implements llm.Provider for testing
type testProvider struct {
	name           string
	completionFn   func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error)
	streamFn       func(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error)
	healthCheckFn  func(ctx context.Context) (*llm.HealthStatus, error)
	listModelsFn   func(ctx context.Context) ([]llm.Model, error)
	supportsNative bool
}

func (p *testProvider) Name() string                        { return p.name }
func (p *testProvider) SupportsNativeFunctionCalling() bool { return p.supportsNative }
func (p *testProvider) Completion(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	if p.completionFn != nil {
		return p.completionFn(ctx, req)
	}
	return nil, nil
}
func (p *testProvider) Stream(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	if p.streamFn != nil {
		return p.streamFn(ctx, req)
	}
	return nil, nil
}
func (p *testProvider) HealthCheck(ctx context.Context) (*llm.HealthStatus, error) {
	if p.healthCheckFn != nil {
		return p.healthCheckFn(ctx)
	}
	return &llm.HealthStatus{Healthy: true}, nil
}
func (p *testProvider) ListModels(ctx context.Context) ([]llm.Model, error) {
	if p.listModelsFn != nil {
		return p.listModelsFn(ctx)
	}
	return nil, nil
}
func (p *testProvider) Endpoints() llm.ProviderEndpoints { return llm.ProviderEndpoints{} }

// testMemoryManager implements MemoryManager for testing
type testMemoryManager struct {
	saveFn       func(ctx context.Context, rec MemoryRecord) error
	deleteFn     func(ctx context.Context, id string) error
	clearFn      func(ctx context.Context, agentID string, kind MemoryKind) error
	loadRecentFn func(ctx context.Context, agentID string, kind MemoryKind, limit int) ([]MemoryRecord, error)
	searchFn     func(ctx context.Context, agentID string, query string, topK int) ([]MemoryRecord, error)
	getFn        func(ctx context.Context, id string) (*MemoryRecord, error)
}

func (m *testMemoryManager) Save(ctx context.Context, rec MemoryRecord) error {
	if m.saveFn != nil {
		return m.saveFn(ctx, rec)
	}
	return nil
}
func (m *testMemoryManager) Delete(ctx context.Context, id string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}
func (m *testMemoryManager) Clear(ctx context.Context, agentID string, kind MemoryKind) error {
	if m.clearFn != nil {
		return m.clearFn(ctx, agentID, kind)
	}
	return nil
}
func (m *testMemoryManager) LoadRecent(ctx context.Context, agentID string, kind MemoryKind, limit int) ([]MemoryRecord, error) {
	if m.loadRecentFn != nil {
		return m.loadRecentFn(ctx, agentID, kind, limit)
	}
	return nil, nil
}
func (m *testMemoryManager) Search(ctx context.Context, agentID string, query string, topK int) ([]MemoryRecord, error) {
	if m.searchFn != nil {
		return m.searchFn(ctx, agentID, query, topK)
	}
	return nil, nil
}
func (m *testMemoryManager) Get(ctx context.Context, id string) (*MemoryRecord, error) {
	if m.getFn != nil {
		return m.getFn(ctx, id)
	}
	return nil, nil
}

// testToolManager implements ToolManager for testing
type testToolManager struct {
	getAllowedToolsFn func(agentID string) []llm.ToolSchema
	executeForAgentFn func(ctx context.Context, agentID string, calls []llm.ToolCall) []llmtools.ToolResult
}

func (t *testToolManager) GetAllowedTools(agentID string) []llm.ToolSchema {
	if t.getAllowedToolsFn != nil {
		return t.getAllowedToolsFn(agentID)
	}
	return nil
}
func (t *testToolManager) ExecuteForAgent(ctx context.Context, agentID string, calls []llm.ToolCall) []llmtools.ToolResult {
	if t.executeForAgentFn != nil {
		return t.executeForAgentFn(ctx, agentID, calls)
	}
	return nil
}

// testEventBus implements EventBus for testing
type testEventBus struct {
	publishFn     func(event Event)
	subscribeFn   func(eventType EventType, handler EventHandler) string
	unsubscribeFn func(subscriptionID string)
	stopFn        func()
	published     []Event
	mu            sync.Mutex
}

func (b *testEventBus) Publish(event Event) {
	b.mu.Lock()
	b.published = append(b.published, event)
	b.mu.Unlock()
	if b.publishFn != nil {
		b.publishFn(event)
	}
}
func (b *testEventBus) Subscribe(eventType EventType, handler EventHandler) string {
	if b.subscribeFn != nil {
		return b.subscribeFn(eventType, handler)
	}
	return "test-sub"
}
func (b *testEventBus) Unsubscribe(subscriptionID string) {
	if b.unsubscribeFn != nil {
		b.unsubscribeFn(subscriptionID)
	}
}
func (b *testEventBus) Stop() {
	if b.stopFn != nil {
		b.stopFn()
	}
}
