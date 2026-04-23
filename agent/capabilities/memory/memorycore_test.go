package memory

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// ─── testMemoryManager mock ──────────────────────────

type testMemoryManager struct {
	mu      sync.Mutex
	records map[string]MemoryRecord
	failOn  string // 设置后对应方法返回错误
}

func newTestMM() *testMemoryManager {
	return &testMemoryManager{records: make(map[string]MemoryRecord)}
}

func (m *testMemoryManager) Save(_ context.Context, rec MemoryRecord) error {
	if m.failOn == "save" {
		return fmt.Errorf("mock save error")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if rec.ID == "" {
		rec.ID = fmt.Sprintf("rec_%d", len(m.records)+1)
	}
	m.records[rec.ID] = rec
	return nil
}

func (m *testMemoryManager) Delete(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.records, id)
	return nil
}

func (m *testMemoryManager) Clear(_ context.Context, agentID string, _ MemoryKind) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, v := range m.records {
		if v.AgentID == agentID {
			delete(m.records, k)
		}
	}
	return nil
}

func (m *testMemoryManager) LoadRecent(_ context.Context, agentID string, kind MemoryKind, limit int) ([]MemoryRecord, error) {
	if m.failOn == "load" {
		return nil, fmt.Errorf("mock load error")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []MemoryRecord
	for _, v := range m.records {
		if v.AgentID == agentID && v.Kind == kind {
			out = append(out, v)
		}
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (m *testMemoryManager) Search(_ context.Context, agentID string, _ string, topK int) ([]MemoryRecord, error) {
	if m.failOn == "search" {
		return nil, fmt.Errorf("mock search error")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []MemoryRecord
	for _, v := range m.records {
		if v.AgentID == agentID {
			out = append(out, v)
		}
		if len(out) >= topK {
			break
		}
	}
	return out, nil
}

func (m *testMemoryManager) Get(_ context.Context, id string) (*MemoryRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r, ok := m.records[id]; ok {
		return &r, nil
	}
	return nil, fmt.Errorf("not found: %s", id)
}

// ═══════════════════════════════════════════════════════
// Cache 测试
// ═══════════════════════════════════════════════════════

func TestCache_NewCache(t *testing.T) {
	mm := newTestMM()
	c := NewCache("agent1", mm, zap.NewNop())
	if c == nil {
		t.Fatal("NewCache returned nil")
	}
	if c.Manager() != mm {
		t.Fatal("Manager() returned wrong instance")
	}
	if !c.HasMemory() {
		t.Fatal("HasMemory should be true")
	}
	if c.HasRecentMemory() {
		t.Fatal("HasRecentMemory should be false initially")
	}
}

func TestCache_NilMemory(t *testing.T) {
	c := NewCache("agent1", nil, zap.NewNop())
	if c.HasMemory() {
		t.Fatal("HasMemory should be false with nil")
	}
	// Save with nil memory should not error
	if err := c.Save(context.Background(), "test", MemoryShortTerm, nil); err != nil {
		t.Fatalf("Save with nil memory should return nil, got %v", err)
	}
	// LoadRecent with nil memory should not panic
	c.LoadRecent(context.Background())
	// Recall with nil memory should return empty
	recs, err := c.Recall(context.Background(), "query", 5)
	if err != nil {
		t.Fatalf("Recall with nil memory should not error, got %v", err)
	}
	if len(recs) != 0 {
		t.Fatalf("expected 0 records, got %d", len(recs))
	}
}

func TestCache_SaveAndLoadRecent(t *testing.T) {
	mm := newTestMM()
	c := NewCache("agent1", mm, zap.NewNop())
	ctx := context.Background()

	// Save 3 records
	for i := 0; i < 3; i++ {
		if err := c.Save(ctx, fmt.Sprintf("content_%d", i), MemoryShortTerm, nil); err != nil {
			t.Fatalf("Save failed: %v", err)
		}
	}

	if !c.HasRecentMemory() {
		t.Fatal("HasRecentMemory should be true after save")
	}

	// Verify in-memory cache
	msgs := c.GetRecentMessages()
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}

	// LoadRecent from backend
	c2 := NewCache("agent1", mm, zap.NewNop())
	c2.LoadRecent(ctx)
	if !c2.HasRecentMemory() {
		t.Fatal("HasRecentMemory should be true after LoadRecent")
	}
}

func TestCache_SaveError(t *testing.T) {
	mm := newTestMM()
	mm.failOn = "save"
	c := NewCache("agent1", mm, zap.NewNop())
	err := c.Save(context.Background(), "test", MemoryShortTerm, nil)
	if err == nil {
		t.Fatal("expected error from Save")
	}
}

func TestCache_LoadRecentError(t *testing.T) {
	mm := newTestMM()
	mm.failOn = "load"
	c := NewCache("agent1", mm, zap.NewNop())
	// LoadRecent should not panic, just log warning
	c.LoadRecent(context.Background())
	if c.HasRecentMemory() {
		t.Fatal("should have no recent memory after load error")
	}
}

func TestCache_MaxRecentMemory(t *testing.T) {
	mm := newTestMM()
	c := NewCache("agent1", mm, zap.NewNop())
	ctx := context.Background()

	// Save more than MaxRecentMemory
	for i := 0; i < MaxRecentMemory+10; i++ {
		c.Save(ctx, fmt.Sprintf("content_%d", i), MemoryShortTerm, nil)
	}

	msgs := c.GetRecentMessages()
	if len(msgs) > MaxRecentMemory {
		t.Fatalf("expected at most %d messages, got %d", MaxRecentMemory, len(msgs))
	}
}

func TestCache_GetRecentMessages_RoleMetadata(t *testing.T) {
	mm := newTestMM()
	c := NewCache("agent1", mm, zap.NewNop())
	ctx := context.Background()

	c.Save(ctx, "user msg", MemoryShortTerm, map[string]any{"role": "user"})
	c.Save(ctx, "assistant msg", MemoryShortTerm, map[string]any{"role": "assistant"})
	c.Save(ctx, "no role", MemoryShortTerm, nil) // default to assistant

	msgs := c.GetRecentMessages()
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}
	if msgs[0].Role != "user" {
		t.Fatalf("expected role=user, got %s", msgs[0].Role)
	}
	if msgs[1].Role != "assistant" {
		t.Fatalf("expected role=assistant, got %s", msgs[1].Role)
	}
	if msgs[2].Role != "assistant" {
		t.Fatalf("expected default role=assistant, got %s", msgs[2].Role)
	}
}

func TestCache_GetRecentMessages_FilterKind(t *testing.T) {
	mm := newTestMM()
	c := NewCache("agent1", mm, zap.NewNop())
	ctx := context.Background()

	c.Save(ctx, "short term", MemoryShortTerm, nil)
	c.Save(ctx, "long term", MemoryLongTerm, nil)

	msgs := c.GetRecentMessages()
	// Only MemoryShortTerm should be returned
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message (short term only), got %d", len(msgs))
	}
	if msgs[0].Content != "short term" {
		t.Fatalf("expected 'short term', got '%s'", msgs[0].Content)
	}
}

func TestCache_Recall(t *testing.T) {
	mm := newTestMM()
	c := NewCache("agent1", mm, zap.NewNop())
	ctx := context.Background()

	c.Save(ctx, "Go is great", MemoryShortTerm, nil)
	c.Save(ctx, "Rust is fast", MemoryShortTerm, nil)

	results, err := c.Recall(ctx, "Go", 5)
	if err != nil {
		t.Fatalf("Recall failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}
}

func TestCache_ConcurrentAccess(t *testing.T) {
	mm := newTestMM()
	c := NewCache("agent1", mm, zap.NewNop())
	ctx := context.Background()

	var wg sync.WaitGroup
	// 10 concurrent writers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			c.Save(ctx, fmt.Sprintf("concurrent_%d", n), MemoryShortTerm, nil)
		}(i)
	}
	// 10 concurrent readers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.GetRecentMessages()
			c.HasRecentMemory()
		}()
	}
	wg.Wait()

	if !c.HasRecentMemory() {
		t.Fatal("should have recent memory after concurrent writes")
	}
}

// ═══════════════════════════════════════════════════════
// Coordinator 测试
// ═══════════════════════════════════════════════════════

func TestCoordinator_NewCoordinator(t *testing.T) {
	mm := newTestMM()
	co := NewCoordinator("agent1", mm, zap.NewNop())
	if co == nil {
		t.Fatal("NewCoordinator returned nil")
	}
	if !co.HasMemory() {
		t.Fatal("HasMemory should be true")
	}
	if co.GetMemoryManager() != mm {
		t.Fatal("GetMemoryManager returned wrong instance")
	}
}

func TestCoordinator_NilMemory(t *testing.T) {
	co := NewCoordinator("agent1", nil, zap.NewNop())
	if co.HasMemory() {
		t.Fatal("HasMemory should be false")
	}
	if err := co.LoadRecent(context.Background(), MemoryShortTerm, 10); err != nil {
		t.Fatalf("LoadRecent with nil should not error: %v", err)
	}
	if err := co.Save(context.Background(), "test", MemoryShortTerm, nil); err != nil {
		t.Fatalf("Save with nil should not error: %v", err)
	}
	recs, err := co.Search(context.Background(), "query", 5)
	if err != nil || len(recs) != 0 {
		t.Fatalf("Search with nil should return empty: err=%v len=%d", err, len(recs))
	}
	if err := co.SaveConversation(context.Background(), "hi", "hello"); err != nil {
		t.Fatalf("SaveConversation with nil should not error: %v", err)
	}
	recs, err = co.RecallRelevant(context.Background(), "query", 5)
	if err != nil || len(recs) != 0 {
		t.Fatalf("RecallRelevant with nil should return empty: err=%v len=%d", err, len(recs))
	}
}

func TestCoordinator_SaveAndLoadRecent(t *testing.T) {
	mm := newTestMM()
	co := NewCoordinator("agent1", mm, zap.NewNop())
	ctx := context.Background()

	co.Save(ctx, "memory1", MemoryShortTerm, nil)
	co.Save(ctx, "memory2", MemoryShortTerm, nil)

	recent := co.GetRecentMemory()
	if len(recent) != 2 {
		t.Fatalf("expected 2 recent, got %d", len(recent))
	}

	// LoadRecent from backend
	co2 := NewCoordinator("agent1", mm, zap.NewNop())
	if err := co2.LoadRecent(ctx, MemoryShortTerm, 10); err != nil {
		t.Fatalf("LoadRecent failed: %v", err)
	}
	recent2 := co2.GetRecentMemory()
	if len(recent2) != 2 {
		t.Fatalf("expected 2 from backend, got %d", len(recent2))
	}
}

func TestCoordinator_SaveError(t *testing.T) {
	mm := newTestMM()
	mm.failOn = "save"
	co := NewCoordinator("agent1", mm, zap.NewNop())
	err := co.Save(context.Background(), "test", MemoryShortTerm, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCoordinator_LoadRecentError(t *testing.T) {
	mm := newTestMM()
	mm.failOn = "load"
	co := NewCoordinator("agent1", mm, zap.NewNop())
	err := co.LoadRecent(context.Background(), MemoryShortTerm, 10)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCoordinator_MaxRecentMemory(t *testing.T) {
	mm := newTestMM()
	co := NewCoordinator("agent1", mm, zap.NewNop())
	ctx := context.Background()

	for i := 0; i < MaxRecentMemory+10; i++ {
		co.Save(ctx, fmt.Sprintf("mem_%d", i), MemoryShortTerm, nil)
	}

	recent := co.GetRecentMemory()
	if len(recent) > MaxRecentMemory {
		t.Fatalf("expected at most %d, got %d", MaxRecentMemory, len(recent))
	}
}

func TestCoordinator_ClearRecentMemory(t *testing.T) {
	mm := newTestMM()
	co := NewCoordinator("agent1", mm, zap.NewNop())
	ctx := context.Background()

	co.Save(ctx, "test", MemoryShortTerm, nil)
	if len(co.GetRecentMemory()) == 0 {
		t.Fatal("should have memory")
	}

	co.ClearRecentMemory()
	if len(co.GetRecentMemory()) != 0 {
		t.Fatal("should be empty after clear")
	}
}

func TestCoordinator_SaveConversation(t *testing.T) {
	mm := newTestMM()
	co := NewCoordinator("agent1", mm, zap.NewNop())
	ctx := context.Background()

	err := co.SaveConversation(ctx, "用户说你好", "助手回复你好")
	if err != nil {
		t.Fatalf("SaveConversation failed: %v", err)
	}

	recent := co.GetRecentMemory()
	if len(recent) != 2 {
		t.Fatalf("expected 2 records (user+assistant), got %d", len(recent))
	}
	if recent[0].Content != "用户说你好" {
		t.Fatalf("expected user content, got %s", recent[0].Content)
	}
	if recent[1].Content != "助手回复你好" {
		t.Fatalf("expected assistant content, got %s", recent[1].Content)
	}
}

func TestCoordinator_RecallRelevant(t *testing.T) {
	mm := newTestMM()
	co := NewCoordinator("agent1", mm, zap.NewNop())
	ctx := context.Background()

	co.Save(ctx, "Go语言很棒", MemoryShortTerm, nil)
	co.Save(ctx, "Rust也不错", MemoryShortTerm, nil)

	results, err := co.RecallRelevant(ctx, "Go", 5)
	if err != nil {
		t.Fatalf("RecallRelevant failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}
}

func TestCoordinator_RecallRelevantError(t *testing.T) {
	mm := newTestMM()
	mm.failOn = "search"
	co := NewCoordinator("agent1", mm, zap.NewNop())
	_, err := co.RecallRelevant(context.Background(), "query", 5)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCoordinator_ConcurrentAccess(t *testing.T) {
	mm := newTestMM()
	co := NewCoordinator("agent1", mm, zap.NewNop())
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			co.Save(ctx, fmt.Sprintf("concurrent_%d", n), MemoryShortTerm, nil)
		}(i)
	}
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			co.GetRecentMemory()
			co.HasMemory()
		}()
	}
	wg.Wait()

	recent := co.GetRecentMemory()
	if len(recent) != 10 {
		t.Fatalf("expected 10 records, got %d", len(recent))
	}
}

// ═══════════════════════════════════════════════════════
// NamespacedManager 补充测试
// ═══════════════════════════════════════════════════════

func TestNamespacedManager_SaveAndGet(t *testing.T) {
	mm := newTestMM()
	ns := NewNamespacedManager(mm, "ns1")

	ctx := context.Background()
	rec := MemoryRecord{ID: "r1", AgentID: "agent1", Kind: types.MemoryShortTerm, Content: "test", CreatedAt: time.Now()}
	if err := ns.Save(ctx, rec); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// AgentID should be prefixed
	mm.mu.Lock()
	found := false
	for _, v := range mm.records {
		if v.AgentID == "ns1/agent1" {
			found = true
		}
	}
	mm.mu.Unlock()
	if !found {
		t.Fatal("expected agentID to be prefixed with namespace")
	}
}

func TestNamespacedManager_Delete(t *testing.T) {
	mm := newTestMM()
	ns := NewNamespacedManager(mm, "ns1")
	ctx := context.Background()

	rec := MemoryRecord{ID: "r1", AgentID: "agent1", Kind: types.MemoryShortTerm, Content: "test"}
	ns.Save(ctx, rec)
	if err := ns.Delete(ctx, "r1"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
}

func TestNamespacedManager_Clear(t *testing.T) {
	mm := newTestMM()
	ns := NewNamespacedManager(mm, "ns1")
	ctx := context.Background()

	ns.Save(ctx, MemoryRecord{ID: "r1", AgentID: "agent1", Kind: types.MemoryShortTerm, Content: "test"})
	if err := ns.Clear(ctx, "agent1", types.MemoryShortTerm); err != nil {
		t.Fatalf("Clear failed: %v", err)
	}
}

func TestNamespacedManager_NamespaceMethod(t *testing.T) {
	ns := NewNamespacedManager(newTestMM(), "my_ns")
	if ns.Namespace() != "my_ns" {
		t.Fatalf("expected 'my_ns', got '%s'", ns.Namespace())
	}
}
