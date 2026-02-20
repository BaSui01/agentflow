package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"go.uber.org/zap"
)

// ToolResultCache 缓存工具执行结果以避免冗余调用.
type ToolResultCache struct {
	entries map[string]*toolCacheEntry
	mu      sync.RWMutex
	config  ToolCacheConfig
	logger  *zap.Logger
	stats   CacheStats
}

// ToolCacheConfig 配置工具结果缓存.
type ToolCacheConfig struct {
	MaxEntries          int                      `json:"max_entries"`
	DefaultTTL          time.Duration            `json:"default_ttl"`
	EnableSemantic      bool                     `json:"enable_semantic"` // Enable semantic similarity matching
	SimilarityThreshold float64                  `json:"similarity_threshold"`
	ToolTTLOverrides    map[string]time.Duration `json:"tool_ttl_overrides"` // Per-tool TTL
	ExcludedTools       []string                 `json:"excluded_tools"`     // Tools to never cache
}

// DefaultToolCacheConfig 返回合理的默认值.
func DefaultToolCacheConfig() ToolCacheConfig {
	return ToolCacheConfig{
		MaxEntries:          10000,
		DefaultTTL:          15 * time.Minute,
		EnableSemantic:      false,
		SimilarityThreshold: 0.95,
		ToolTTLOverrides:    make(map[string]time.Duration),
		ExcludedTools:       []string{},
	}
}

// CacheStats 跟踪缓存性能.
type CacheStats struct {
	Hits      int64 `json:"hits"`
	Misses    int64 `json:"misses"`
	Evictions int64 `json:"evictions"`
	Size      int   `json:"size"`
}

type toolCacheEntry struct {
	Key       string          `json:"key"`
	ToolName  string          `json:"tool_name"`
	Arguments json.RawMessage `json:"arguments"`
	Result    json.RawMessage `json:"result"`
	Error     string          `json:"error,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
	ExpiresAt time.Time       `json:"expires_at"`
	HitCount  int             `json:"hit_count"`
}

// NewToolResultCache 创建新的工具结果缓存.
func NewToolResultCache(config ToolCacheConfig, logger *zap.Logger) *ToolResultCache {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &ToolResultCache{
		entries: make(map[string]*toolCacheEntry),
		config:  config,
		logger:  logger,
	}
}

// Get 获取工具调用的缓存结果.
func (c *ToolResultCache) Get(toolName string, arguments json.RawMessage) (*CachedToolResult, bool) {
	// 检查是否排除工具
	if c.isExcluded(toolName) {
		return nil, false
	}

	key := c.buildKey(toolName, arguments)

	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()

	if !ok {
		c.mu.Lock()
		c.stats.Misses++
		c.mu.Unlock()
		return nil, false
	}

	// 检查过期
	if time.Now().After(entry.ExpiresAt) {
		c.mu.Lock()
		delete(c.entries, key)
		c.stats.Misses++
		c.stats.Size = len(c.entries)
		c.mu.Unlock()
		return nil, false
	}

	c.mu.Lock()
	entry.HitCount++
	c.stats.Hits++
	c.mu.Unlock()

	c.logger.Debug("cache hit",
		zap.String("tool", toolName),
		zap.Int("hit_count", entry.HitCount))

	return &CachedToolResult{
		Result:    entry.Result,
		Error:     entry.Error,
		CachedAt:  entry.CreatedAt,
		FromCache: true,
	}, true
}

// Set 在缓存中设置一个工具结果.
func (c *ToolResultCache) Set(toolName string, arguments json.RawMessage, result json.RawMessage, err string) {
	// 检查是否排除工具
	if c.isExcluded(toolName) {
		return
	}

	key := c.buildKey(toolName, arguments)
	ttl := c.getTTL(toolName)

	c.mu.Lock()
	defer c.mu.Unlock()

	// 如果已满则淘汰
	if len(c.entries) >= c.config.MaxEntries {
		c.evictOldest()
	}

	c.entries[key] = &toolCacheEntry{
		Key:       key,
		ToolName:  toolName,
		Arguments: arguments,
		Result:    result,
		Error:     err,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(ttl),
		HitCount:  0,
	}
	c.stats.Size = len(c.entries)

	c.logger.Debug("cached tool result",
		zap.String("tool", toolName),
		zap.Duration("ttl", ttl))
}

// Invalidate 删除特定缓存项.
func (c *ToolResultCache) Invalidate(toolName string, arguments json.RawMessage) {
	key := c.buildKey(toolName, arguments)
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
	c.stats.Size = len(c.entries)
}

// InvalidateTool 删除指定工具的所有缓存项.
func (c *ToolResultCache) InvalidateTool(toolName string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for key, entry := range c.entries {
		if entry.ToolName == toolName {
			delete(c.entries, key)
		}
	}
	c.stats.Size = len(c.entries)
}

// Clear 清除所有缓存条目.
func (c *ToolResultCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*toolCacheEntry)
	c.stats.Size = 0
}

// Stats 返回缓存统计信息.
func (c *ToolResultCache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.stats
}

// CachedToolResult 表示缓存的工具执行结果.
type CachedToolResult struct {
	Result    json.RawMessage `json:"result"`
	Error     string          `json:"error,omitempty"`
	CachedAt  time.Time       `json:"cached_at"`
	FromCache bool            `json:"from_cache"`
}

func (c *ToolResultCache) buildKey(toolName string, arguments json.RawMessage) string {
	// 对参数进行规范化以保证一致的哈希
	var normalized interface{}
	if err := json.Unmarshal(arguments, &normalized); err == nil {
		if sortedArgs, err := json.Marshal(normalized); err == nil {
			arguments = sortedArgs
		}
	}

	data := fmt.Sprintf("%s:%s", toolName, string(arguments))
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

func (c *ToolResultCache) getTTL(toolName string) time.Duration {
	if ttl, ok := c.config.ToolTTLOverrides[toolName]; ok {
		return ttl
	}
	return c.config.DefaultTTL
}

func (c *ToolResultCache) isExcluded(toolName string) bool {
	for _, excluded := range c.config.ExcludedTools {
		if excluded == toolName {
			return true
		}
	}
	return false
}

func (c *ToolResultCache) evictOldest() {
	var oldestKey string
	var oldestTime time.Time

	for key, entry := range c.entries {
		if oldestKey == "" || entry.CreatedAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.CreatedAt
		}
	}

	if oldestKey != "" {
		delete(c.entries, oldestKey)
		c.stats.Evictions++
	}
}

// CachingToolExecutor 将工具执行器用缓存包裹.
type CachingToolExecutor struct {
	executor ToolExecutor
	cache    *ToolResultCache
	logger   *zap.Logger
}

// ToolExecutor 定义工具执行接口.
type ToolExecutor interface {
	Execute(ctx context.Context, calls []llm.ToolCall) []ToolResult
}

// ToolResult 表示工具执行结果.
type ToolResult struct {
	ToolCallID string          `json:"tool_call_id"`
	Name       string          `json:"name"`
	Result     json.RawMessage `json:"result"`
	Error      string          `json:"error,omitempty"`
	Duration   time.Duration   `json:"duration"`
	FromCache  bool            `json:"from_cache"`
}

// NewCachingToolExecutor 创建缓存工具执行器.
func NewCachingToolExecutor(executor ToolExecutor, cache *ToolResultCache, logger *zap.Logger) *CachingToolExecutor {
	return &CachingToolExecutor{
		executor: executor,
		cache:    cache,
		logger:   logger,
	}
}

// Execute 使用缓存执行工具调用.
func (e *CachingToolExecutor) Execute(ctx context.Context, calls []llm.ToolCall) []ToolResult {
	results := make([]ToolResult, len(calls))
	var uncachedCalls []llm.ToolCall
	var uncachedIndices []int

	// 检查每次调用的缓存
	for i, call := range calls {
		if cached, ok := e.cache.Get(call.Name, call.Arguments); ok {
			results[i] = ToolResult{
				ToolCallID: call.ID,
				Name:       call.Name,
				Result:     cached.Result,
				Error:      cached.Error,
				FromCache:  true,
			}
		} else {
			uncachedCalls = append(uncachedCalls, call)
			uncachedIndices = append(uncachedIndices, i)
		}
	}

	// 执行未缓存的调用
	if len(uncachedCalls) > 0 {
		execResults := e.executor.Execute(ctx, uncachedCalls)
		for j, execResult := range execResults {
			idx := uncachedIndices[j]
			results[idx] = ToolResult{
				ToolCallID: execResult.ToolCallID,
				Name:       execResult.Name,
				Result:     execResult.Result,
				Error:      execResult.Error,
				Duration:   execResult.Duration,
				FromCache:  false,
			}

			// 缓存结果
			e.cache.Set(execResult.Name, uncachedCalls[j].Arguments, execResult.Result, execResult.Error)
		}
	}

	return results
}
