package runtime

import (
	"context"
	"testing"

	"github.com/BaSui01/agentflow/rag"
	"github.com/BaSui01/agentflow/rag/core"
	"go.uber.org/zap"
)

func TestNewSemanticCache(t *testing.T) {
	store := rag.NewInMemoryVectorStore(zap.NewNop())
	if _, err := NewSemanticCache(store, SemanticCacheConfig{SimilarityThreshold: 0.95}, zap.NewNop()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := NewSemanticCache(nil, SemanticCacheConfig{SimilarityThreshold: 0.95}, zap.NewNop()); err == nil {
		t.Fatal("expected nil store error")
	}
	if _, err := NewSemanticCache(store, SemanticCacheConfig{SimilarityThreshold: 1.2}, zap.NewNop()); err == nil {
		t.Fatal("expected invalid threshold error")
	}
}

func TestSemanticCacheSetGetClear(t *testing.T) {
	ctx := context.Background()
	store := rag.NewInMemoryVectorStore(zap.NewNop())
	cache, err := NewSemanticCache(store, SemanticCacheConfig{SimilarityThreshold: 0.9}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewSemanticCache failed: %v", err)
	}

	doc := core.Document{
		ID:        "q1",
		Content:   "cached answer",
		Embedding: []float64{1, 0},
	}
	if err := cache.Set(ctx, doc); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	got, ok := cache.Get(ctx, []float64{1, 0})
	if !ok || got == nil {
		t.Fatal("expected cache hit")
	}
	if got.ID != "q1" {
		t.Fatalf("unexpected doc id: %s", got.ID)
	}

	if err := cache.Clear(ctx); err != nil {
		t.Fatalf("Clear failed: %v", err)
	}
	if _, ok := cache.Get(ctx, []float64{1, 0}); ok {
		t.Fatal("expected cache miss after clear")
	}
}
