package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ====== ToolResultCache Tests ======

func TestToolResultCache_SetGet(t *testing.T) {
	cache := NewToolResultCache(DefaultToolCacheConfig(), nil)

	args := json.RawMessage(`{"query":"test"}`)
	result := json.RawMessage(`{"answer":"42"}`)

	cache.Set("search", args, result, "")

	cached, ok := cache.Get("search", args)
	assert.True(t, ok)
	assert.True(t, cached.FromCache)
	assert.JSONEq(t, `{"answer":"42"}`, string(cached.Result))
}

func TestToolResultCache_Miss(t *testing.T) {
	cache := NewToolResultCache(DefaultToolCacheConfig(), nil)

	_, ok := cache.Get("search", json.RawMessage(`{"q":"test"}`))
	assert.False(t, ok)

	stats := cache.Stats()
	assert.Equal(t, int64(1), stats.Misses)
}

func TestToolResultCache_Expiry(t *testing.T) {
	cfg := DefaultToolCacheConfig()
	cfg.DefaultTTL = 1 * time.Millisecond
	cache := NewToolResultCache(cfg, nil)

	args := json.RawMessage(`{"q":"test"}`)
	cache.Set("search", args, json.RawMessage(`{"r":"ok"}`), "")

	time.Sleep(5 * time.Millisecond)

	_, ok := cache.Get("search", args)
	assert.False(t, ok)
}

func TestToolResultCache_ExcludedTool(t *testing.T) {
	cfg := DefaultToolCacheConfig()
	cfg.ExcludedTools = []string{"dangerous"}
	cache := NewToolResultCache(cfg, nil)

	args := json.RawMessage(`{"x":1}`)
	cache.Set("dangerous", args, json.RawMessage(`{"r":"ok"}`), "")

	_, ok := cache.Get("dangerous", args)
	assert.False(t, ok)
}

func TestToolResultCache_ToolTTLOverride(t *testing.T) {
	cfg := DefaultToolCacheConfig()
	cfg.DefaultTTL = time.Hour
	cfg.ToolTTLOverrides = map[string]time.Duration{
		"volatile": 1 * time.Millisecond,
	}
	cache := NewToolResultCache(cfg, nil)

	args := json.RawMessage(`{"x":1}`)
	cache.Set("volatile", args, json.RawMessage(`{"r":"ok"}`), "")

	time.Sleep(5 * time.Millisecond)

	_, ok := cache.Get("volatile", args)
	assert.False(t, ok)
}

func TestToolResultCache_Eviction(t *testing.T) {
	cfg := DefaultToolCacheConfig()
	cfg.MaxEntries = 3
	cache := NewToolResultCache(cfg, nil)

	for i := 0; i < 5; i++ {
		args := json.RawMessage(fmt.Sprintf(`{"i":%d}`, i))
		cache.Set("tool", args, json.RawMessage(`{"r":"ok"}`), "")
	}

	stats := cache.Stats()
	assert.LessOrEqual(t, stats.Size, 3)
	assert.True(t, stats.Evictions > 0)
}

func TestToolResultCache_Invalidate(t *testing.T) {
	cache := NewToolResultCache(DefaultToolCacheConfig(), nil)

	args := json.RawMessage(`{"q":"test"}`)
	cache.Set("search", args, json.RawMessage(`{"r":"ok"}`), "")

	cache.Invalidate("search", args)

	_, ok := cache.Get("search", args)
	assert.False(t, ok)
}

func TestToolResultCache_InvalidateTool(t *testing.T) {
	cache := NewToolResultCache(DefaultToolCacheConfig(), nil)

	cache.Set("search", json.RawMessage(`{"q":"a"}`), json.RawMessage(`{"r":"1"}`), "")
	cache.Set("search", json.RawMessage(`{"q":"b"}`), json.RawMessage(`{"r":"2"}`), "")
	cache.Set("calc", json.RawMessage(`{"x":1}`), json.RawMessage(`{"r":"3"}`), "")

	cache.InvalidateTool("search")

	_, ok := cache.Get("search", json.RawMessage(`{"q":"a"}`))
	assert.False(t, ok)

	_, ok = cache.Get("calc", json.RawMessage(`{"x":1}`))
	assert.True(t, ok)
}

func TestToolResultCache_Clear(t *testing.T) {
	cache := NewToolResultCache(DefaultToolCacheConfig(), nil)

	cache.Set("a", json.RawMessage(`{}`), json.RawMessage(`{}`), "")
	cache.Set("b", json.RawMessage(`{}`), json.RawMessage(`{}`), "")

	cache.Clear()

	stats := cache.Stats()
	assert.Equal(t, 0, stats.Size)
}

func TestToolResultCache_Stats(t *testing.T) {
	cache := NewToolResultCache(DefaultToolCacheConfig(), nil)

	args := json.RawMessage(`{"q":"test"}`)
	cache.Set("search", args, json.RawMessage(`{"r":"ok"}`), "")

	cache.Get("search", args) // hit
	cache.Get("search", args) // hit
	cache.Get("other", args)  // miss

	stats := cache.Stats()
	assert.Equal(t, int64(2), stats.Hits)
	assert.Equal(t, int64(1), stats.Misses)
}

func TestToolResultCache_CacheError(t *testing.T) {
	cache := NewToolResultCache(DefaultToolCacheConfig(), nil)

	args := json.RawMessage(`{"q":"test"}`)
	cache.Set("search", args, nil, "tool failed")

	cached, ok := cache.Get("search", args)
	assert.True(t, ok)
	assert.Equal(t, "tool failed", cached.Error)
}

// ====== HierarchicalKeyStrategy Additional Tests ======

func TestHierarchicalKeyStrategy_EmptyMessages(t *testing.T) {
	s := NewHierarchicalKeyStrategy()

	key := s.GenerateKey(&llm.ChatRequest{
		TenantID: "t1",
		Model:    "gpt-4o",
		Messages: nil,
	})

	assert.Contains(t, key, "llm:cache:t1:gpt-4o:initial")
}

func TestHierarchicalKeyStrategy_SameHistoryProducesSameKey(t *testing.T) {
	s := NewHierarchicalKeyStrategy()

	msgs := []llm.Message{
		{Role: llm.RoleSystem, Content: "You are helpful"},
		{Role: llm.RoleUser, Content: "Hello"},
	}

	key1 := s.GenerateKey(&llm.ChatRequest{
		TenantID: "t1",
		Model:    "gpt-4o",
		Messages: append(msgs, llm.Message{Role: llm.RoleUser, Content: "Q1"}),
	})

	key2 := s.GenerateKey(&llm.ChatRequest{
		TenantID: "t1",
		Model:    "gpt-4o",
		Messages: append(msgs, llm.Message{Role: llm.RoleUser, Content: "Q2"}),
	})

	// Same prefix (history), different last message doesn't affect key
	assert.Equal(t, key1, key2)
}

// ====== CachingToolExecutor Tests ======

type mockToolExecutor struct {
	executeFn func(ctx context.Context, calls []llm.ToolCall) []tools.ToolResult
}

func (m *mockToolExecutor) Execute(ctx context.Context, calls []llm.ToolCall) []tools.ToolResult {
	if m.executeFn != nil {
		return m.executeFn(ctx, calls)
	}
	results := make([]tools.ToolResult, len(calls))
	for i, c := range calls {
		results[i] = tools.ToolResult{
			ToolCallID: c.ID,
			Name:       c.Name,
			Result:     json.RawMessage(`{"computed":true}`),
		}
	}
	return results
}

func (m *mockToolExecutor) ExecuteOne(ctx context.Context, call llm.ToolCall) tools.ToolResult {
	results := m.Execute(ctx, []llm.ToolCall{call})
	return results[0]
}

func TestCachingToolExecutor_CacheHit(t *testing.T) {
	cache := NewToolResultCache(DefaultToolCacheConfig(), nil)
	execCount := 0
	executor := &mockToolExecutor{
		executeFn: func(ctx context.Context, calls []llm.ToolCall) []tools.ToolResult {
			execCount++
			results := make([]tools.ToolResult, len(calls))
			for i, c := range calls {
				results[i] = tools.ToolResult{
					ToolCallID: c.ID,
					Name:       c.Name,
					Result:     json.RawMessage(`{"r":"computed"}`),
				}
			}
			return results
		},
	}

	cachingExec := NewCachingToolExecutor(executor, cache, nil)

	call := llm.ToolCall{ID: "c1", Name: "search", Arguments: json.RawMessage(`{"q":"test"}`)}

	// First call: cache miss, executes
	r1 := cachingExec.ExecuteOne(context.Background(), call)
	assert.False(t, r1.FromCache)
	assert.Equal(t, 1, execCount)

	// Second call: cache hit
	r2 := cachingExec.ExecuteOne(context.Background(), call)
	assert.True(t, r2.FromCache)
	assert.Equal(t, 1, execCount) // not called again
}

func TestCachingToolExecutor_MixedCacheHitMiss(t *testing.T) {
	cache := NewToolResultCache(DefaultToolCacheConfig(), nil)
	executor := &mockToolExecutor{}

	cachingExec := NewCachingToolExecutor(executor, cache, nil)

	// Pre-populate cache for one call
	cache.Set("search", json.RawMessage(`{"q":"cached"}`), json.RawMessage(`{"r":"from_cache"}`), "")

	calls := []llm.ToolCall{
		{ID: "c1", Name: "search", Arguments: json.RawMessage(`{"q":"cached"}`)},
		{ID: "c2", Name: "calc", Arguments: json.RawMessage(`{"x":1}`)},
	}

	results := cachingExec.Execute(context.Background(), calls)
	require.Len(t, results, 2)

	assert.True(t, results[0].FromCache)
	assert.False(t, results[1].FromCache)
}

// ====== HashKeyStrategy Additional Tests ======

func TestHashKeyStrategy_DifferentRequestsDifferentKeys(t *testing.T) {
	s := NewHashKeyStrategy()

	key1 := s.GenerateKey(&llm.ChatRequest{
		Model:    "gpt-4o",
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hello"}},
	})

	key2 := s.GenerateKey(&llm.ChatRequest{
		Model:    "gpt-4o",
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "World"}},
	})

	assert.NotEqual(t, key1, key2)
}

func TestHashKeyStrategy_DifferentModels(t *testing.T) {
	s := NewHashKeyStrategy()

	key1 := s.GenerateKey(&llm.ChatRequest{
		Model:    "gpt-4o",
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hello"}},
	})

	key2 := s.GenerateKey(&llm.ChatRequest{
		Model:    "claude-3",
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hello"}},
	})

	assert.NotEqual(t, key1, key2)
}
