package rag

import (
	"container/heap"
	"fmt"
	"math"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// heap helpers to avoid name collision with container/heap
func heap_push(h heap.Interface, x any) { heap.Push(h, x) }
func heap_pop(h heap.Interface) any     { return heap.Pop(h) }

// ============================================================
// DefaultHNSWConfig / AdaptiveHNSWConfig
// ============================================================

func TestDefaultHNSWConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultHNSWConfig()
	assert.Equal(t, 16, cfg.M)
	assert.Equal(t, 200, cfg.EfConstruction)
	assert.Equal(t, 100, cfg.EfSearch)
	assert.Equal(t, 16, cfg.MaxLevel)
	assert.InDelta(t, 1.0/math.Log(2.0), cfg.Ml, 1e-9)
}

func TestAdaptiveHNSWConfig(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		dataSize       int
		expectedM      int
		expectedEfCon  int
		expectedEfSrch int
	}{
		{"tiny dataset", 100, 12, 100, 50},
		{"small dataset", 9999, 12, 100, 50},
		{"medium dataset", 10000, 16, 200, 100},
		{"large dataset", 100000, 24, 300, 150},
		{"very large dataset", 1000000, 32, 400, 200},
		{"huge dataset", 5000000, 32, 400, 200},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := AdaptiveHNSWConfig(tt.dataSize)
			assert.Equal(t, tt.expectedM, cfg.M)
			assert.Equal(t, tt.expectedEfCon, cfg.EfConstruction)
			assert.Equal(t, tt.expectedEfSrch, cfg.EfSearch)
		})
	}
}

// ============================================================
// HNSWIndex — Build / Search / Add / Delete / Size
// ============================================================

func newTestHNSWIndex(t *testing.T) *HNSWIndex {
	t.Helper()
	cfg := HNSWConfig{
		M:              4,
		EfConstruction: 20,
		EfSearch:       20,
		MaxLevel:       4,
		Ml:             1.0 / math.Log(2.0),
	}
	return NewHNSWIndex(cfg, zap.NewNop())
}

func TestHNSWIndex_Build_EmptyInput(t *testing.T) {
	t.Parallel()
	idx := newTestHNSWIndex(t)
	err := idx.Build([][]float64{}, []string{})
	require.NoError(t, err)
	assert.Equal(t, 0, idx.Size())
}

func TestHNSWIndex_Build_MismatchedLengths(t *testing.T) {
	t.Parallel()
	idx := newTestHNSWIndex(t)
	err := idx.Build([][]float64{{1, 0}}, []string{"a", "b"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mismatch")
}

func TestHNSWIndex_Build_AndSearch(t *testing.T) {
	t.Parallel()
	idx := newTestHNSWIndex(t)

	vectors := [][]float64{
		{1, 0, 0},
		{0, 1, 0},
		{0, 0, 1},
		{1, 1, 0},
	}
	ids := []string{"a", "b", "c", "d"}

	err := idx.Build(vectors, ids)
	require.NoError(t, err)
	assert.Equal(t, 4, idx.Size())

	// Search for vector closest to {1, 0, 0} — should return "a"
	results, err := idx.Search([]float64{1, 0, 0}, 2)
	require.NoError(t, err)
	require.NotEmpty(t, results)
	assert.Equal(t, "a", results[0].ID)
	assert.InDelta(t, 0.0, results[0].Distance, 1e-9)
	assert.InDelta(t, 1.0, results[0].Score, 1e-9)
}

func TestHNSWIndex_Search_EmptyIndex(t *testing.T) {
	t.Parallel()
	idx := newTestHNSWIndex(t)
	results, err := idx.Search([]float64{1, 0}, 5)
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestHNSWIndex_Add_Success(t *testing.T) {
	t.Parallel()
	idx := newTestHNSWIndex(t)

	require.NoError(t, idx.Add([]float64{1, 0}, "v1"))
	require.NoError(t, idx.Add([]float64{0, 1}, "v2"))
	assert.Equal(t, 2, idx.Size())
}

func TestHNSWIndex_Add_Duplicate(t *testing.T) {
	t.Parallel()
	idx := newTestHNSWIndex(t)

	require.NoError(t, idx.Add([]float64{1, 0}, "v1"))
	err := idx.Add([]float64{0, 1}, "v1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestHNSWIndex_Delete_Success(t *testing.T) {
	t.Parallel()
	idx := newTestHNSWIndex(t)

	require.NoError(t, idx.Add([]float64{1, 0, 0}, "v1"))
	require.NoError(t, idx.Add([]float64{0, 1, 0}, "v2"))
	require.NoError(t, idx.Add([]float64{0, 0, 1}, "v3"))
	assert.Equal(t, 3, idx.Size())

	require.NoError(t, idx.Delete("v2"))
	assert.Equal(t, 2, idx.Size())

	// Search should still work after deletion
	results, err := idx.Search([]float64{1, 0, 0}, 3)
	require.NoError(t, err)
	for _, r := range results {
		assert.NotEqual(t, "v2", r.ID)
	}
}

func TestHNSWIndex_Delete_NotFound(t *testing.T) {
	t.Parallel()
	idx := newTestHNSWIndex(t)
	err := idx.Delete("nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestHNSWIndex_Delete_EntryPoint(t *testing.T) {
	t.Parallel()
	idx := newTestHNSWIndex(t)

	require.NoError(t, idx.Add([]float64{1, 0}, "ep"))
	require.NoError(t, idx.Add([]float64{0, 1}, "other"))

	// Delete the entry point
	require.NoError(t, idx.Delete("ep"))
	assert.Equal(t, 1, idx.Size())

	// Search should still work
	results, err := idx.Search([]float64{0, 1}, 1)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "other", results[0].ID)
}

func TestHNSWIndex_Delete_AllNodes(t *testing.T) {
	t.Parallel()
	idx := newTestHNSWIndex(t)

	require.NoError(t, idx.Add([]float64{1, 0}, "a"))
	require.NoError(t, idx.Add([]float64{0, 1}, "b"))
	require.NoError(t, idx.Delete("a"))
	require.NoError(t, idx.Delete("b"))
	assert.Equal(t, 0, idx.Size())

	results, err := idx.Search([]float64{1, 0}, 1)
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestHNSWIndex_ConcurrentAddAndSearch(t *testing.T) {
	t.Parallel()
	idx := newTestHNSWIndex(t)

	// Seed with a few vectors
	require.NoError(t, idx.Build([][]float64{{1, 0}, {0, 1}}, []string{"seed1", "seed2"}))

	var wg sync.WaitGroup
	// Concurrent adds
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			v := []float64{float64(i), float64(10 - i)}
			_ = idx.Add(v, fmt.Sprintf("conc-%d", i))
		}(i)
	}
	// Concurrent searches
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = idx.Search([]float64{1, 1}, 3)
		}()
	}
	wg.Wait()
	assert.GreaterOrEqual(t, idx.Size(), 2) // at least the seeds
}

// ============================================================
// distance helper
// ============================================================

func TestHNSWIndex_Distance_DifferentLengths(t *testing.T) {
	t.Parallel()
	idx := newTestHNSWIndex(t)
	d := idx.distance([]float64{1, 0}, []float64{1, 0, 0})
	assert.Equal(t, 1.0, d)
}

func TestHNSWIndex_Distance_ZeroVector(t *testing.T) {
	t.Parallel()
	idx := newTestHNSWIndex(t)
	d := idx.distance([]float64{0, 0}, []float64{1, 0})
	assert.Equal(t, 1.0, d)
}

func TestHNSWIndex_Distance_IdenticalVectors(t *testing.T) {
	t.Parallel()
	idx := newTestHNSWIndex(t)
	d := idx.distance([]float64{0.5, 0.5}, []float64{0.5, 0.5})
	assert.InDelta(t, 0.0, d, 1e-9)
}

// ============================================================
// Heap implementations
// ============================================================

func TestMinHeap_PushPop(t *testing.T) {
	t.Parallel()
	h := &minHeap{}
	heap_push(h, &heapItem{id: "a", dist: 3.0})
	heap_push(h, &heapItem{id: "b", dist: 1.0})
	heap_push(h, &heapItem{id: "c", dist: 2.0})

	assert.Equal(t, 3, h.Len())
	item := heap_pop(h).(*heapItem)
	assert.Equal(t, "b", item.id)
	assert.Equal(t, 1.0, item.dist)
}

func TestMaxHeap_PushPop(t *testing.T) {
	t.Parallel()
	h := &maxHeap{}
	heap_push(h, &heapItem{id: "a", dist: 3.0})
	heap_push(h, &heapItem{id: "b", dist: 1.0})
	heap_push(h, &heapItem{id: "c", dist: 2.0})

	assert.Equal(t, 3, h.Len())
	item := heap_pop(h).(*heapItem)
	assert.Equal(t, "a", item.id)
	assert.Equal(t, 3.0, item.dist)
}



