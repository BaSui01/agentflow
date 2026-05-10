package memory

import (
	"context"
	"fmt"
	"sync"
)

// testMemoryManager is a minimal in-memory MemoryManager for testing.
type testMemoryManager struct {
	mu      sync.Mutex
	records map[string]MemoryRecord
	failOn  string // when set, the corresponding method returns an error
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

// compile-time check that testMemoryManager implements MemoryManager.
var _ MemoryManager = (*testMemoryManager)(nil)
