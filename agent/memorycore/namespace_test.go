package memorycore

import (
	"context"
	"testing"
)

// inMemoryManager is a simple in-memory MemoryManager for testing.
type inMemoryManager struct {
	records map[string][]MemoryRecord
}

func newInMemoryManager() *inMemoryManager {
	return &inMemoryManager{records: map[string][]MemoryRecord{}}
}

func (m *inMemoryManager) Save(_ context.Context, rec MemoryRecord) error {
	m.records[rec.AgentID] = append(m.records[rec.AgentID], rec)
	return nil
}

func (m *inMemoryManager) Delete(_ context.Context, _ string) error { return nil }
func (m *inMemoryManager) Clear(_ context.Context, agentID string, _ MemoryKind) error {
	delete(m.records, agentID)
	return nil
}

func (m *inMemoryManager) LoadRecent(_ context.Context, agentID string, _ MemoryKind, limit int) ([]MemoryRecord, error) {
	recs := m.records[agentID]
	if len(recs) > limit {
		recs = recs[len(recs)-limit:]
	}
	return recs, nil
}

func (m *inMemoryManager) Search(_ context.Context, agentID string, _ string, topK int) ([]MemoryRecord, error) {
	return m.LoadRecent(nil, agentID, "", topK)
}

func (m *inMemoryManager) Get(_ context.Context, _ string) (*MemoryRecord, error) {
	return nil, nil
}

func TestNamespacedManager_Isolation(t *testing.T) {
	inner := newInMemoryManager()

	ns1 := NewNamespacedManager(inner, "sub-agent-1")
	ns2 := NewNamespacedManager(inner, "sub-agent-2")

	ctx := context.Background()

	// Save to ns1
	if err := ns1.Save(ctx, MemoryRecord{AgentID: "main", Content: "from-sub1", Kind: MemoryShortTerm}); err != nil {
		t.Fatal(err)
	}
	// Save to ns2
	if err := ns2.Save(ctx, MemoryRecord{AgentID: "main", Content: "from-sub2", Kind: MemoryShortTerm}); err != nil {
		t.Fatal(err)
	}

	// ns1 should only see its own records
	recs1, err := ns1.LoadRecent(ctx, "main", MemoryShortTerm, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs1) != 1 || recs1[0].Content != "from-sub1" {
		t.Errorf("ns1 should see only its own record, got %v", recs1)
	}

	// ns2 should only see its own records
	recs2, err := ns2.LoadRecent(ctx, "main", MemoryShortTerm, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs2) != 1 || recs2[0].Content != "from-sub2" {
		t.Errorf("ns2 should see only its own record, got %v", recs2)
	}

	// Direct access to inner should see both under different scoped keys
	allKeys := make([]string, 0)
	for k := range inner.records {
		allKeys = append(allKeys, k)
	}
	if len(allKeys) != 2 {
		t.Errorf("expected 2 scoped keys in inner store, got %d: %v", len(allKeys), allKeys)
	}
}

func TestNamespacedManager_Namespace(t *testing.T) {
	inner := newInMemoryManager()
	ns := NewNamespacedManager(inner, "test-ns")
	if ns.Namespace() != "test-ns" {
		t.Errorf("expected namespace 'test-ns', got %q", ns.Namespace())
	}
}
