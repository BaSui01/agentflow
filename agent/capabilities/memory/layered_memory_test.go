package memory

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// --- EpisodicMemory Tests ---

func TestEpisodicMemory_Store_AutoID(t *testing.T) {
	t.Parallel()
	mem := NewEpisodicMemory(10, zap.NewNop())

	ep := &Episode{Context: "test context", Action: "test action"}
	mem.Store(ep)

	assert.NotEmpty(t, ep.ID)
	assert.False(t, ep.Timestamp.IsZero())
	assert.Len(t, mem.Recall(0), 1)
}

func TestEpisodicMemory_Store_Eviction(t *testing.T) {
	t.Parallel()
	mem := NewEpisodicMemory(3, zap.NewNop())

	for i := 0; i < 5; i++ {
		mem.Store(&Episode{Context: "ctx", Action: "act"})
	}

	episodes := mem.Recall(0)
	assert.Len(t, episodes, 3, "should evict oldest episodes when exceeding maxSize")
}

func TestEpisodicMemory_Recall_Limit(t *testing.T) {
	t.Parallel()
	mem := NewEpisodicMemory(100, zap.NewNop())

	for i := 0; i < 10; i++ {
		mem.Store(&Episode{Context: "ctx", Action: "act"})
	}

	episodes := mem.Recall(3)
	assert.Len(t, episodes, 3, "should return only requested number of episodes")

	all := mem.Recall(0)
	assert.Len(t, all, 10, "limit 0 should return all")
}

func TestEpisodicMemory_Search(t *testing.T) {
	t.Parallel()
	mem := NewEpisodicMemory(100, zap.NewNop())

	mem.Store(&Episode{Context: "golang testing", Action: "write tests"})
	mem.Store(&Episode{Context: "python scripting", Action: "run script"})
	mem.Store(&Episode{Context: "golang debugging", Action: "fix bug"})

	results := mem.Search("golang", 10)
	assert.Len(t, results, 2)

	results = mem.Search("golang", 1)
	assert.Len(t, results, 1)

	results = mem.Search("nonexistent", 10)
	assert.Empty(t, results)
}

func TestEpisodicMemory_Search_ByAction(t *testing.T) {
	t.Parallel()
	mem := NewEpisodicMemory(100, zap.NewNop())

	mem.Store(&Episode{Context: "ctx1", Action: "deploy service"})
	mem.Store(&Episode{Context: "ctx2", Action: "test service"})

	results := mem.Search("deploy", 10)
	assert.Len(t, results, 1)
}

// --- SemanticMemory Tests ---

type mockEmbedder struct {
	embedFn func(ctx context.Context, text string) ([]float32, error)
}

func (m *mockEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	if m.embedFn != nil {
		return m.embedFn(ctx, text)
	}
	return []float32{0.1, 0.2, 0.3}, nil
}

func TestSemanticMemory_StoreFact_AutoID(t *testing.T) {
	t.Parallel()
	mem := NewSemanticMemory(nil, zap.NewNop())

	err := mem.StoreFact(context.Background(), &Fact{
		Subject:   "Go",
		Predicate: "is",
		Object:    "a programming language",
	})
	require.NoError(t, err)

	facts := mem.Query("Go")
	assert.Len(t, facts, 1)
	assert.NotEmpty(t, facts[0].ID)
	assert.False(t, facts[0].CreatedAt.IsZero())
}

func TestSemanticMemory_StoreFact_WithEmbedder(t *testing.T) {
	t.Parallel()
	embedder := &mockEmbedder{
		embedFn: func(_ context.Context, _ string) ([]float32, error) {
			return []float32{1.0, 2.0}, nil
		},
	}
	mem := NewSemanticMemory(embedder, zap.NewNop())

	err := mem.StoreFact(context.Background(), &Fact{
		Subject:   "Go",
		Predicate: "is",
		Object:    "fast",
	})
	require.NoError(t, err)

	facts := mem.Query("Go")
	require.Len(t, facts, 1)
	assert.Equal(t, []float32{1.0, 2.0}, facts[0].Embedding)
}

func TestSemanticMemory_GetFact(t *testing.T) {
	t.Parallel()
	mem := NewSemanticMemory(nil, zap.NewNop())

	fact := &Fact{ID: "fact-1", Subject: "Go", Predicate: "is", Object: "fast"}
	require.NoError(t, mem.StoreFact(context.Background(), fact))

	got, ok := mem.GetFact("fact-1")
	assert.True(t, ok)
	assert.Equal(t, "Go", got.Subject)

	_, ok = mem.GetFact("nonexistent")
	assert.False(t, ok)
}

func TestSemanticMemory_Query_NoMatch(t *testing.T) {
	t.Parallel()
	mem := NewSemanticMemory(nil, zap.NewNop())

	require.NoError(t, mem.StoreFact(context.Background(), &Fact{
		Subject: "Go", Predicate: "is", Object: "fast",
	}))

	facts := mem.Query("Python")
	assert.Empty(t, facts)
}

// --- WorkingMemory Tests ---

func TestWorkingMemory_SetGet(t *testing.T) {
	t.Parallel()
	mem := NewWorkingMemory(10, time.Hour, zap.NewNop())

	mem.Set("key1", "value1", 5)
	val, ok := mem.Get("key1")
	assert.True(t, ok)
	assert.Equal(t, "value1", val)
}

func TestWorkingMemory_Get_Expired(t *testing.T) {
	t.Parallel()
	mem := NewWorkingMemory(10, time.Millisecond, zap.NewNop())

	mem.Set("key1", "value1", 5)
	time.Sleep(5 * time.Millisecond)

	_, ok := mem.Get("key1")
	assert.False(t, ok, "expired item should not be returned")
}

func TestWorkingMemory_Set_OverwriteExisting(t *testing.T) {
	t.Parallel()
	mem := NewWorkingMemory(10, time.Hour, zap.NewNop())

	mem.Set("key1", "old", 5)
	mem.Set("key1", "new", 10)

	val, ok := mem.Get("key1")
	assert.True(t, ok)
	assert.Equal(t, "new", val)

	all := mem.GetAll()
	assert.Len(t, all, 1, "should not duplicate keys")
}

func TestWorkingMemory_EvictLowestPriority(t *testing.T) {
	t.Parallel()
	mem := NewWorkingMemory(3, time.Hour, zap.NewNop())

	mem.Set("low", "v", 1)
	mem.Set("mid", "v", 5)
	mem.Set("high", "v", 10)
	mem.Set("new", "v", 7) // should evict "low"

	_, ok := mem.Get("low")
	assert.False(t, ok, "lowest priority item should be evicted")

	_, ok = mem.Get("mid")
	assert.True(t, ok)
	_, ok = mem.Get("high")
	assert.True(t, ok)
	_, ok = mem.Get("new")
	assert.True(t, ok)
}

func TestWorkingMemory_Clear(t *testing.T) {
	t.Parallel()
	mem := NewWorkingMemory(10, time.Millisecond, zap.NewNop())

	mem.Set("key1", "v1", 5)
	mem.Set("key2", "v2", 5)
	time.Sleep(5 * time.Millisecond)

	mem.Clear()
	all := mem.GetAll()
	assert.Empty(t, all, "Clear should remove expired items")
}

func TestWorkingMemory_GetAll_FiltersExpired(t *testing.T) {
	t.Parallel()
	mem := NewWorkingMemory(10, time.Millisecond, zap.NewNop())

	mem.Set("expire", "v", 5)
	time.Sleep(5 * time.Millisecond)
	mem2 := NewWorkingMemory(10, time.Hour, zap.NewNop())
	mem2.Set("keep", "v", 5)

	all := mem.GetAll()
	assert.Empty(t, all)

	all2 := mem2.GetAll()
	assert.Len(t, all2, 1)
}

// --- ProceduralMemory Tests ---

func TestProceduralMemory_Store_AutoID(t *testing.T) {
	t.Parallel()
	mem := NewProceduralMemory(ProceduralConfig{MaxProcedures: 10}, zap.NewNop())

	proc := &Procedure{Name: "deploy", Steps: []string{"build", "push", "deploy"}}
	mem.Store(proc)

	assert.NotEmpty(t, proc.ID)
	got, ok := mem.Get(proc.ID)
	assert.True(t, ok)
	assert.Equal(t, "deploy", got.Name)
}

func TestProceduralMemory_FindByTrigger(t *testing.T) {
	t.Parallel()
	mem := NewProceduralMemory(ProceduralConfig{MaxProcedures: 10}, zap.NewNop())

	mem.Store(&Procedure{ID: "p1", Name: "deploy", Triggers: []string{"push", "merge"}})
	mem.Store(&Procedure{ID: "p2", Name: "test", Triggers: []string{"push"}})
	mem.Store(&Procedure{ID: "p3", Name: "lint", Triggers: []string{"save"}})

	results := mem.FindByTrigger("push")
	assert.Len(t, results, 2)

	results = mem.FindByTrigger("save")
	assert.Len(t, results, 1)

	results = mem.FindByTrigger("nonexistent")
	assert.Empty(t, results)
}

// --- LayeredMemory Tests ---

func TestNewLayeredMemory(t *testing.T) {
	t.Parallel()
	lm := NewLayeredMemory(LayeredMemoryConfig{
		EpisodicMaxSize: 100,
		WorkingCapacity: 10,
		WorkingTTL:      time.Hour,
	}, zap.NewNop())

	assert.NotNil(t, lm.Episodic)
	assert.NotNil(t, lm.Semantic)
	assert.NotNil(t, lm.Working)
	assert.NotNil(t, lm.Procedural)
}

func TestLayeredMemory_Export(t *testing.T) {
	t.Parallel()
	lm := NewLayeredMemory(LayeredMemoryConfig{
		EpisodicMaxSize: 100,
		WorkingCapacity: 10,
		WorkingTTL:      time.Hour,
	}, zap.NewNop())

	lm.Episodic.Store(&Episode{Context: "test", Action: "export"})
	lm.Working.Set("key", "value", 5)

	data, err := lm.Export()
	require.NoError(t, err)
	assert.Contains(t, string(data), "episodic")
	assert.Contains(t, string(data), "working")
}

// --- Concurrency Tests ---

func TestEpisodicMemory_Concurrent(t *testing.T) {
	t.Parallel()
	mem := NewEpisodicMemory(1000, zap.NewNop())

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mem.Store(&Episode{Context: "concurrent", Action: "test"})
			mem.Recall(5)
			mem.Search("concurrent", 3)
		}()
	}
	wg.Wait()

	episodes := mem.Recall(0)
	assert.Equal(t, 50, len(episodes))
}

func TestWorkingMemory_Concurrent(t *testing.T) {
	t.Parallel()
	mem := NewWorkingMemory(100, time.Hour, zap.NewNop())

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			key := "key"
			mem.Set(key, idx, idx)
			mem.Get(key)
			mem.GetAll()
		}(i)
	}
	wg.Wait()
}
