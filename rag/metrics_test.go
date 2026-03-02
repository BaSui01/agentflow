package rag

import (
	"context"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/types"
)

func TestCollectRetrievalMetrics(t *testing.T) {
	ctx := context.Background()
	ctx = types.WithTraceID(ctx, "trace-123")
	ctx = types.WithRunID(ctx, "run-456")

	start := time.Now().Add(-50 * time.Millisecond)
	m := collectRetrievalMetrics(ctx, start, 10*time.Millisecond, 5, 3, 200)

	if m.TraceID != "trace-123" {
		t.Fatalf("expected trace_id trace-123, got %s", m.TraceID)
	}
	if m.RunID != "run-456" {
		t.Fatalf("expected run_id run-456, got %s", m.RunID)
	}
	if m.RetrievalLatency < 50*time.Millisecond {
		t.Fatalf("expected retrieval_latency >= 50ms, got %v", m.RetrievalLatency)
	}
	if m.RerankLatency != 10*time.Millisecond {
		t.Fatalf("expected rerank_latency 10ms, got %v", m.RerankLatency)
	}
	if m.TopK != 5 {
		t.Fatalf("expected topk 5, got %d", m.TopK)
	}
	if m.HitCount != 3 {
		t.Fatalf("expected hit_count 3, got %d", m.HitCount)
	}
	if m.ContextTokens != 200 {
		t.Fatalf("expected context_tokens 200, got %d", m.ContextTokens)
	}
}

func TestCollectRetrievalMetrics_NoContext(t *testing.T) {
	m := collectRetrievalMetrics(context.Background(), time.Now(), 0, 10, 0, 0)
	if m.TraceID != "" {
		t.Fatalf("expected empty trace_id, got %s", m.TraceID)
	}
	if m.RunID != "" {
		t.Fatalf("expected empty run_id, got %s", m.RunID)
	}
}

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"", 0},
		{"hi", 1},
		{"hello world test", 4},
	}
	for _, tt := range tests {
		got := estimateTokens(tt.input)
		if got != tt.expected {
			t.Errorf("estimateTokens(%q) = %d, want %d", tt.input, got, tt.expected)
		}
	}
}
