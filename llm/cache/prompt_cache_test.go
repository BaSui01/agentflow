package cache

import (
	"testing"
	"time"

	llmpkg "github.com/BaSui01/agentflow/llm"
	"go.uber.org/zap"
)

func TestLRUCache_Basic(t *testing.T) {
	cache := NewLRUCache(3, time.Minute)

	// 测试 Set 和 Get
	entry := &CacheEntry{TokensSaved: 100}
	cache.Set("key1", entry)

	got, ok := cache.Get("key1")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got.TokensSaved != 100 {
		t.Errorf("expected TokensSaved=100, got %d", got.TokensSaved)
	}
}

func TestLRUCache_Eviction(t *testing.T) {
	cache := NewLRUCache(2, time.Minute)

	cache.Set("key1", &CacheEntry{TokensSaved: 1})
	cache.Set("key2", &CacheEntry{TokensSaved: 2})
	cache.Set("key3", &CacheEntry{TokensSaved: 3}) // 应该驱逐 key1

	if _, ok := cache.Get("key1"); ok {
		t.Error("key1 should have been evicted")
	}
	if _, ok := cache.Get("key2"); !ok {
		t.Error("key2 should exist")
	}
	if _, ok := cache.Get("key3"); !ok {
		t.Error("key3 should exist")
	}
}

func TestLRUCache_TTL(t *testing.T) {
	cache := NewLRUCache(10, 10*time.Millisecond)

	cache.Set("key1", &CacheEntry{TokensSaved: 1})

	// 立即获取应该成功
	if _, ok := cache.Get("key1"); !ok {
		t.Error("expected cache hit")
	}

	// 等待过期
	time.Sleep(20 * time.Millisecond)

	if _, ok := cache.Get("key1"); ok {
		t.Error("expected cache miss after TTL")
	}
}

func TestMultiLevelCache_GenerateKey(t *testing.T) {
	cache := NewMultiLevelCache(nil, nil, zap.NewNop())

	req1 := &llmpkg.ChatRequest{
		Model:    "gpt-4",
		Messages: []llmpkg.Message{{Role: llmpkg.RoleUser, Content: "hello"}},
	}
	req2 := &llmpkg.ChatRequest{
		Model:    "gpt-4",
		Messages: []llmpkg.Message{{Role: llmpkg.RoleUser, Content: "hello"}},
	}
	req3 := &llmpkg.ChatRequest{
		Model:    "gpt-4",
		Messages: []llmpkg.Message{{Role: llmpkg.RoleUser, Content: "world"}},
	}

	key1 := cache.GenerateKey(req1)
	key2 := cache.GenerateKey(req2)
	key3 := cache.GenerateKey(req3)

	if key1 != key2 {
		t.Error("same requests should have same key")
	}
	if key1 == key3 {
		t.Error("different requests should have different keys")
	}
}

func TestMultiLevelCache_IsCacheable(t *testing.T) {
	cache := NewMultiLevelCache(nil, nil, zap.NewNop())

	// 无工具调用的请求可缓存
	req1 := &llmpkg.ChatRequest{Model: "gpt-4"}
	if !cache.IsCacheable(req1) {
		t.Error("request without tools should be cacheable")
	}

	// 有工具调用的请求不可缓存
	req2 := &llmpkg.ChatRequest{
		Model: "gpt-4",
		Tools: []llmpkg.ToolSchema{{Name: "test"}},
	}
	if cache.IsCacheable(req2) {
		t.Error("request with tools should not be cacheable")
	}
}
