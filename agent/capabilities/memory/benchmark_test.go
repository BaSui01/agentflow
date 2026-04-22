package memory

import (
	"context"
	"fmt"
	"testing"

	"go.uber.org/zap"
)

// stubMemoryManager is a minimal in-memory MemoryManager for benchmarking.
type stubMemoryManager struct {
	records []MemoryRecord
}

func (s *stubMemoryManager) Save(_ context.Context, rec MemoryRecord) error {
	s.records = append(s.records, rec)
	return nil
}

func (s *stubMemoryManager) Delete(_ context.Context, _ string) error { return nil }

func (s *stubMemoryManager) Clear(_ context.Context, _ string, _ MemoryKind) error { return nil }

func (s *stubMemoryManager) LoadRecent(_ context.Context, _ string, _ MemoryKind, limit int) ([]MemoryRecord, error) {
	if limit > len(s.records) {
		limit = len(s.records)
	}
	return s.records[:limit], nil
}

func (s *stubMemoryManager) Search(_ context.Context, _ string, _ string, topK int) ([]MemoryRecord, error) {
	if topK > len(s.records) {
		topK = len(s.records)
	}
	return s.records[:topK], nil
}

func (s *stubMemoryManager) Get(_ context.Context, _ string) (*MemoryRecord, error) {
	return nil, fmt.Errorf("not found")
}

// BenchmarkCache_Save measures memory save performance.
func BenchmarkCache_Save(b *testing.B) {
	logger := zap.NewNop()
	mgr := &stubMemoryManager{}
	cache := NewCache("bench-agent", mgr, logger)

	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = cache.Save(ctx, fmt.Sprintf("message content %d", i), MemoryShortTerm, map[string]any{
			"role": "user",
		})
	}
}

// BenchmarkCache_GetRecentMessages measures message retrieval performance.
func BenchmarkCache_GetRecentMessages(b *testing.B) {
	logger := zap.NewNop()
	mgr := &stubMemoryManager{}
	cache := NewCache("bench-agent", mgr, logger)

	// Pre-populate cache with records
	ctx := context.Background()
	for i := 0; i < MaxRecentMemory; i++ {
		_ = cache.Save(ctx, fmt.Sprintf("message %d", i), MemoryShortTerm, map[string]any{
			"role": "user",
		})
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = cache.GetRecentMessages()
	}
}

// Compile-time check: stubMemoryManager implements MemoryManager.
var _ MemoryManager = (*stubMemoryManager)(nil)
