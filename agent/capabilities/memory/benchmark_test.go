package memory

import (
	"context"
	"fmt"
	"testing"

	"go.uber.org/zap"
)

// BenchmarkCoordinator_Save measures memory save performance.
func BenchmarkCoordinator_Save(b *testing.B) {
	logger := zap.NewNop()
	mgr := newTestMM()
	coord := NewCoordinator("bench-agent", mgr, logger)

	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = coord.Save(ctx, fmt.Sprintf("message content %d", i), MemoryWorking, map[string]any{
			"role": "user",
		})
	}
}

// BenchmarkCoordinator_GetRecentMessages measures message retrieval performance.
func BenchmarkCoordinator_GetRecentMessages(b *testing.B) {
	logger := zap.NewNop()
	mgr := newTestMM()
	coord := NewCoordinator("bench-agent", mgr, logger)

	// Pre-populate cache with records
	ctx := context.Background()
	for i := 0; i < MaxRecentMemory; i++ {
		_ = coord.Save(ctx, fmt.Sprintf("message %d", i), MemoryWorking, map[string]any{
			"role": "user",
		})
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = coord.GetRecentMessages()
	}
}
