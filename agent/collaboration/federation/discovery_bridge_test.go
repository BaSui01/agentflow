package federation

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
)

// mockDiscoveryRegistry is a function-callback mock for DiscoveryRegistry (§30).
type mockDiscoveryRegistry struct {
	registerFn     func(ctx context.Context, info *AgentRegistration) error
	unregisterFn   func(ctx context.Context, agentID string) error
	updateStatusFn func(ctx context.Context, agentID string, status string) error

	mu           sync.Mutex
	registered   map[string]*AgentRegistration
	statuses     map[string]string
	unregistered []string
}

func newMockDiscoveryRegistry() *mockDiscoveryRegistry {
	m := &mockDiscoveryRegistry{
		registered: make(map[string]*AgentRegistration),
		statuses:   make(map[string]string),
	}
	m.registerFn = func(_ context.Context, info *AgentRegistration) error {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.registered[info.ID] = info
		return nil
	}
	m.unregisterFn = func(_ context.Context, agentID string) error {
		m.mu.Lock()
		defer m.mu.Unlock()
		delete(m.registered, agentID)
		m.unregistered = append(m.unregistered, agentID)
		return nil
	}
	m.updateStatusFn = func(_ context.Context, agentID string, status string) error {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.statuses[agentID] = status
		return nil
	}
	return m
}

func (m *mockDiscoveryRegistry) RegisterAgent(ctx context.Context, info *AgentRegistration) error {
	return m.registerFn(ctx, info)
}

func (m *mockDiscoveryRegistry) UnregisterAgent(ctx context.Context, agentID string) error {
	return m.unregisterFn(ctx, agentID)
}

func (m *mockDiscoveryRegistry) UpdateAgentStatus(ctx context.Context, agentID string, status string) error {
	return m.updateStatusFn(ctx, agentID, status)
}

func (m *mockDiscoveryRegistry) getRegistered(id string) (*AgentRegistration, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.registered[id]
	return r, ok
}

func (m *mockDiscoveryRegistry) registeredCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.registered)
}

func (m *mockDiscoveryRegistry) getUnregistered() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, len(m.unregistered))
	copy(out, m.unregistered)
	return out
}

// --- helpers ---

func newBridgeTestOrchestrator(t *testing.T) *Orchestrator {
	t.Helper()
	return NewOrchestrator(FederationConfig{
		NodeID:            "local",
		HeartbeatInterval: 50 * time.Millisecond,
	}, zap.NewNop())
}

func newBridgeTestBridge(t *testing.T, orch *Orchestrator, reg *mockDiscoveryRegistry, cfg BridgeConfig) *DiscoveryBridge {
	t.Helper()
	return NewDiscoveryBridge(orch, reg, cfg, zap.NewNop())
}

func testNode(id, name string, caps ...string) *FederatedNode {
	return &FederatedNode{
		ID:           id,
		Name:         name,
		Endpoint:     "https://" + id + ".example.com",
		Capabilities: caps,
		Metadata:     map[string]string{"env": "test"},
	}
}

// --- tests ---

func TestSyncNode(t *testing.T) {
	orch := newBridgeTestOrchestrator(t)
	reg := newMockDiscoveryRegistry()
	bridge := newBridgeTestBridge(t, orch, reg, DefaultBridgeConfig())

	node := testNode("n1", "node-1", "chat", "search")
	ctx := context.Background()

	if err := bridge.SyncNode(ctx, node); err != nil {
		t.Fatalf("SyncNode: %v", err)
	}

	got, ok := reg.getRegistered("n1")
	if !ok {
		t.Fatal("node n1 not found in registry")
	}
	if got.Name != "node-1" {
		t.Errorf("Name = %q, want %q", got.Name, "node-1")
	}
	if len(got.Capabilities) != 2 {
		t.Errorf("Capabilities len = %d, want 2", len(got.Capabilities))
	}
}

func TestSyncNode_NilNode(t *testing.T) {
	orch := newBridgeTestOrchestrator(t)
	reg := newMockDiscoveryRegistry()
	bridge := newBridgeTestBridge(t, orch, reg, DefaultBridgeConfig())

	if err := bridge.SyncNode(context.Background(), nil); err == nil {
		t.Fatal("expected error for nil node")
	}
}

func TestSyncAllNodes(t *testing.T) {
	orch := newBridgeTestOrchestrator(t)
	orch.RegisterNode(testNode("n1", "node-1", "chat"))
	orch.RegisterNode(testNode("n2", "node-2", "search"))

	reg := newMockDiscoveryRegistry()
	bridge := newBridgeTestBridge(t, orch, reg, DefaultBridgeConfig())

	if err := bridge.SyncAllNodes(context.Background()); err != nil {
		t.Fatalf("SyncAllNodes: %v", err)
	}

	if reg.registeredCount() != 2 {
		t.Errorf("registered count = %d, want 2", reg.registeredCount())
	}
}

func TestAutoSync_OnNodeRegister(t *testing.T) {
	orch := newBridgeTestOrchestrator(t)
	reg := newMockDiscoveryRegistry()
	bridge := newBridgeTestBridge(t, orch, reg, BridgeConfig{
		SyncInterval: time.Hour, // long interval so periodic sync doesn't interfere
		AutoSync:     true,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := bridge.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer bridge.Stop()

	// Register a node after bridge is started; callback should auto-sync.
	orch.RegisterNode(testNode("n3", "node-3", "translate"))

	// Give the async callback a moment.
	time.Sleep(100 * time.Millisecond)

	if _, ok := reg.getRegistered("n3"); !ok {
		t.Error("node n3 was not auto-synced to discovery")
	}
}

func TestAutoSync_OnNodeUnregister(t *testing.T) {
	orch := newBridgeTestOrchestrator(t)
	orch.RegisterNode(testNode("n1", "node-1", "chat"))

	reg := newMockDiscoveryRegistry()
	bridge := newBridgeTestBridge(t, orch, reg, BridgeConfig{
		SyncInterval: time.Hour,
		AutoSync:     true,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := bridge.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer bridge.Stop()

	orch.UnregisterNode("n1")

	time.Sleep(100 * time.Millisecond)

	unreg := reg.getUnregistered()
	found := false
	for _, id := range unreg {
		if id == "n1" {
			found = true
			break
		}
	}
	if !found {
		t.Error("node n1 was not unregistered from discovery")
	}
}

func TestStopIsIdempotent(t *testing.T) {
	orch := newBridgeTestOrchestrator(t)
	reg := newMockDiscoveryRegistry()
	bridge := newBridgeTestBridge(t, orch, reg, DefaultBridgeConfig())

	// Calling Stop multiple times should not panic.
	bridge.Stop()
	bridge.Stop()
	bridge.Stop()
}

func TestPeriodicSync(t *testing.T) {
	orch := newBridgeTestOrchestrator(t)
	orch.RegisterNode(testNode("n1", "node-1", "chat"))

	reg := newMockDiscoveryRegistry()
	bridge := newBridgeTestBridge(t, orch, reg, BridgeConfig{
		SyncInterval: 50 * time.Millisecond,
		AutoSync:     false,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := bridge.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer bridge.Stop()

	// Wait for at least one periodic sync tick.
	time.Sleep(150 * time.Millisecond)

	if _, ok := reg.getRegistered("n1"); !ok {
		t.Error("periodic sync did not register node n1")
	}
}

func TestDefaultBridgeConfig(t *testing.T) {
	cfg := DefaultBridgeConfig()
	if cfg.SyncInterval != 60*time.Second {
		t.Errorf("SyncInterval = %v, want 60s", cfg.SyncInterval)
	}
	if !cfg.AutoSync {
		t.Error("AutoSync should default to true")
	}
}

// TestAutoSync_CallbackCtxPropagatesParentCancellation 验证 GitHub Issue #12：
// Start(ctx) 注册的 auto-sync 回调内派生的 syncCtx 必须从父 ctx 派生，
// 这样父 ctx 取消（或 Stop() 被调用）能立即取消正在飞行中的同步操作。
//
// 旧实现使用 context.WithTimeout(context.Background(), ...)，
// 父 ctx 取消后回调 ctx 仍存活，导致 goroutine 与 IO 调用泄漏。
func TestAutoSync_CallbackCtxPropagatesParentCancellation(t *testing.T) {
	orch := newBridgeTestOrchestrator(t)
	reg := newMockDiscoveryRegistry()

	// capturedCtx 由 register 回调内的 syncCtx 提供，registerStarted 通知主流程
	// 已经进入注册函数（即回调链已经触发并派生出 syncCtx）。
	capturedCh := make(chan context.Context, 1)
	registerEntered := make(chan struct{})
	releaseRegister := make(chan struct{})

	reg.registerFn = func(ctx context.Context, info *AgentRegistration) error {
		select {
		case capturedCh <- ctx:
		default:
		}
		close(registerEntered)
		// 阻塞到主流程取消父 ctx 或测试结束。
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-releaseRegister:
			return nil
		case <-time.After(2 * time.Second):
			return nil
		}
	}

	bridge := newBridgeTestBridge(t, orch, reg, BridgeConfig{
		SyncInterval: time.Hour,
		AutoSync:     true,
	})

	parentCtx, parentCancel := context.WithCancel(context.Background())
	if err := bridge.Start(parentCtx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer bridge.Stop()
	defer close(releaseRegister)

	// 触发 OnNodeRegister 回调。
	orch.RegisterNode(testNode("n1", "node-1", "chat"))

	// 等待回调进入 RegisterAgent。
	select {
	case <-registerEntered:
	case <-time.After(time.Second):
		t.Fatal("register callback was never invoked")
	}

	var capturedCtx context.Context
	select {
	case capturedCtx = <-capturedCh:
	case <-time.After(time.Second):
		t.Fatal("could not capture syncCtx from callback")
	}

	// 取消父 ctx，期望 capturedCtx 也随之 Done。
	t.Logf("captured ctx string: %v", capturedCtx)
	t.Logf("captured ctx err before cancel: %v", capturedCtx.Err())
	parentCancel()
	time.Sleep(50 * time.Millisecond)
	t.Logf("captured ctx err after cancel: %v", capturedCtx.Err())

	select {
	case <-capturedCtx.Done():
		// 期望路径：父 ctx 取消传播到回调派生的 syncCtx。
	case <-time.After(500 * time.Millisecond):
		t.Fatal("auto-sync callback ctx did not propagate parent cancellation " +
			"(syncCtx 应从父 ctx 派生，而非 context.Background())")
	}
}

// TestAutoSync_CallbackCtxCancelsOnStop 验证 Stop() 也能取消正在飞行中的回调 ctx。
// 这是 #12 的另一面：即使没有外部父 ctx 取消，Stop() 也应主动停止后台工作。
func TestAutoSync_CallbackCtxCancelsOnStop(t *testing.T) {
	orch := newBridgeTestOrchestrator(t)
	reg := newMockDiscoveryRegistry()

	capturedCh := make(chan context.Context, 1)
	registerEntered := make(chan struct{})
	releaseRegister := make(chan struct{})

	reg.registerFn = func(ctx context.Context, info *AgentRegistration) error {
		select {
		case capturedCh <- ctx:
		default:
		}
		close(registerEntered)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-releaseRegister:
			return nil
		case <-time.After(2 * time.Second):
			return nil
		}
	}

	bridge := newBridgeTestBridge(t, orch, reg, BridgeConfig{
		SyncInterval: time.Hour,
		AutoSync:     true,
	})

	parentCtx, parentCancel := context.WithCancel(context.Background())
	defer parentCancel()
	if err := bridge.Start(parentCtx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer close(releaseRegister)

	orch.RegisterNode(testNode("n2", "node-2", "translate"))

	select {
	case <-registerEntered:
	case <-time.After(time.Second):
		t.Fatal("register callback was never invoked")
	}

	var capturedCtx context.Context
	select {
	case capturedCtx = <-capturedCh:
	case <-time.After(time.Second):
		t.Fatal("could not capture syncCtx from callback")
	}

	bridge.Stop()

	select {
	case <-capturedCtx.Done():
		// 期望路径：Stop() 触发 lifecycle 取消，传播到 syncCtx。
	case <-time.After(500 * time.Millisecond):
		t.Fatal("auto-sync callback ctx did not cancel after Stop() " +
			"(Stop 应取消所有派生 ctx)")
	}
}
