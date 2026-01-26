package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"time"

	llmpkg "github.com/BaSui01/agentflow/llm"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

var ErrCacheMiss = errors.New("cache miss")

// PromptCache Prompt 缓存接口
type PromptCache interface {
	Get(ctx context.Context, key string) (*CacheEntry, error)
	Set(ctx context.Context, key string, entry *CacheEntry) error
	Delete(ctx context.Context, key string) error
	GenerateKey(req any) string
}

// CacheEntry 缓存条目
type CacheEntry struct {
	Response      any       `json:"response"`
	TokensSaved   int       `json:"tokens_saved"`
	PromptVersion string    `json:"prompt_version,omitempty"`
	ModelVersion  string    `json:"model_version,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	ExpiresAt     time.Time `json:"expires_at"`
	HitCount      int       `json:"hit_count"`
}

// CacheConfig 缓存配置
type CacheConfig struct {
	LocalMaxSize    int                // 本地缓存最大条目数
	LocalTTL        time.Duration      // 本地缓存 TTL
	RedisTTL        time.Duration      // Redis 缓存 TTL
	EnableLocal     bool               // 是否启用本地缓存
	EnableRedis     bool               // 是否启用 Redis 缓存
	KeyStrategyType string             // 缓存键策略类型：hash | hierarchical
	CacheableCheck  func(req any) bool // 判断请求是否可缓存
}

// DefaultCacheConfig 默认配置
func DefaultCacheConfig() *CacheConfig {
	return &CacheConfig{
		LocalMaxSize: 1000,
		LocalTTL:     5 * time.Minute,
		RedisTTL:     1 * time.Hour,
		EnableLocal:  true,
		EnableRedis:  true,
		CacheableCheck: func(req any) bool {
			// 默认策略：只缓存“纯文本对话”请求。
			// 若请求包含工具列表（Tools 非空），通常意味着可能触发工具调用或依赖外部状态，
			// 直接缓存响应会导致副作用被跳过或结果不一致，因此默认不缓存。
			v := reflect.ValueOf(req)
			if !v.IsValid() {
				return true
			}
			if v.Kind() == reflect.Pointer {
				if v.IsNil() {
					return true
				}
				v = v.Elem()
			}
			if v.Kind() != reflect.Struct {
				return true
			}

			f := v.FieldByName("Tools")
			if !f.IsValid() || f.Kind() != reflect.Slice {
				return true
			}
			return f.Len() == 0
		},
	}
}

// MultiLevelCache 多级缓存实现
type MultiLevelCache struct {
	local    *LRUCache
	redis    *redis.Client
	config   *CacheConfig
	strategy KeyStrategy // 缓存键生成策略
	logger   *zap.Logger
}

// NewMultiLevelCache 创建多级缓存
func NewMultiLevelCache(rdb *redis.Client, config *CacheConfig, logger *zap.Logger) *MultiLevelCache {
	if config == nil {
		config = DefaultCacheConfig()
	}

	var local *LRUCache
	if config.EnableLocal {
		local = NewLRUCache(config.LocalMaxSize, config.LocalTTL)
	}

	// 根据配置选择缓存键策略
	var strategy KeyStrategy
	switch config.KeyStrategyType {
	case "hierarchical":
		strategy = NewHierarchicalKeyStrategy()
		logger.Info("using hierarchical cache key strategy")
	default:
		strategy = NewHashKeyStrategy()
		logger.Info("using hash cache key strategy")
	}

	return &MultiLevelCache{
		local:    local,
		redis:    rdb,
		config:   config,
		strategy: strategy,
		logger:   logger,
	}
}

// Get 获取缓存
func (c *MultiLevelCache) Get(ctx context.Context, key string) (*CacheEntry, error) {
	// 1. 查本地缓存
	if c.config.EnableLocal && c.local != nil {
		if entry, ok := c.local.Get(key); ok {
			c.logger.Debug("local cache hit", zap.String("key", key))
			return entry, nil
		}
	}

	// 2. 查 Redis 缓存
	if c.config.EnableRedis && c.redis != nil {
		data, err := c.redis.Get(ctx, c.redisKey(key)).Bytes()
		if err == nil {
			var entry CacheEntry
			if err := json.Unmarshal(data, &entry); err == nil {
				// 回填本地缓存
				if c.config.EnableLocal && c.local != nil {
					c.local.Set(key, &entry)
				}
				c.logger.Debug("redis cache hit", zap.String("key", key))
				// 异步更新命中计数
				go c.incrementHitCount(context.Background(), key)
				return &entry, nil
			}
		}
		if !errors.Is(err, redis.Nil) {
			c.logger.Warn("redis get error", zap.Error(err))
		}
	}

	return nil, ErrCacheMiss
}

// Set 设置缓存
func (c *MultiLevelCache) Set(ctx context.Context, key string, entry *CacheEntry) error {
	entry.CreatedAt = time.Now()
	entry.ExpiresAt = time.Now().Add(c.config.RedisTTL)

	// 1. 写本地缓存
	if c.config.EnableLocal && c.local != nil {
		c.local.Set(key, entry)
	}

	// 2. 写 Redis 缓存
	if c.config.EnableRedis && c.redis != nil {
		data, err := json.Marshal(entry)
		if err != nil {
			return err
		}
		if err := c.redis.Set(ctx, c.redisKey(key), data, c.config.RedisTTL).Err(); err != nil {
			c.logger.Warn("redis set error", zap.Error(err))
			return err
		}
	}

	c.logger.Debug("cache set", zap.String("key", key))
	return nil
}

// Delete 删除缓存
func (c *MultiLevelCache) Delete(ctx context.Context, key string) error {
	// 删除本地缓存
	if c.config.EnableLocal && c.local != nil {
		c.local.Delete(key)
	}

	// 删除 Redis 缓存
	if c.config.EnableRedis && c.redis != nil {
		if err := c.redis.Del(ctx, c.redisKey(key)).Err(); err != nil {
			return err
		}
	}

	return nil
}

// GenerateKey 生成缓存键（使用策略模式）
func (c *MultiLevelCache) GenerateKey(req any) string {
	// 尝试转换为 ChatRequest
	chatReq, ok := req.(*llmpkg.ChatRequest)
	if !ok {
		// 回退到默认 Hash 实现
		data, _ := json.Marshal(req)
		hash := sha256.Sum256(data)
		return "llm:cache:" + hex.EncodeToString(hash[:16])
	}

	return c.strategy.GenerateKey(chatReq)
}

// IsCacheable 判断请求是否可缓存
func (c *MultiLevelCache) IsCacheable(req any) bool {
	if c.config.CacheableCheck != nil {
		return c.config.CacheableCheck(req)
	}
	return true
}

func (c *MultiLevelCache) redisKey(key string) string {
	return "llm:prompt_cache:" + key
}

func (c *MultiLevelCache) incrementHitCount(ctx context.Context, key string) {
	if c.redis == nil {
		return
	}
	// 使用 Lua 脚本原子更新
	script := redis.NewScript(`
		local key = KEYS[1]
		local data = redis.call('GET', key)
		if data then
			local entry = cjson.decode(data)
			entry.hit_count = (entry.hit_count or 0) + 1
			local ttl = redis.call('TTL', key)
			if ttl > 0 then
				redis.call('SET', key, cjson.encode(entry), 'EX', ttl)
			end
		end
		return 1
	`)
	script.Run(ctx, c.redis, []string{c.redisKey(key)})
}

// InvalidateByVersion 按版本失效缓存
func (c *MultiLevelCache) InvalidateByVersion(ctx context.Context, promptVersion, modelVersion string) error {
	// 本地缓存全部清空（简单实现）
	if c.local != nil {
		c.local.Clear()
	}

	// Redis 缓存需要扫描删除（生产环境建议使用更高效的方案）
	c.logger.Info("cache invalidated by version",
		zap.String("prompt_version", promptVersion),
		zap.String("model_version", modelVersion))

	return nil
}

// ============================================================
// LRU 本地缓存实现（使用双向链表实现 O(1) 操作）
// ============================================================

type LRUCache struct {
	mu       sync.RWMutex
	capacity int
	ttl      time.Duration
	items    map[string]*lruNode
	head     *lruNode // 最近使用
	tail     *lruNode // 最久未使用
}

type lruNode struct {
	key       string
	entry     *CacheEntry
	expiresAt time.Time
	prev      *lruNode
	next      *lruNode
}

func NewLRUCache(capacity int, ttl time.Duration) *LRUCache {
	return &LRUCache{
		capacity: capacity,
		ttl:      ttl,
		items:    make(map[string]*lruNode),
	}
}

func (c *LRUCache) Get(key string) (*CacheEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	node, ok := c.items[key]
	if !ok {
		return nil, false
	}

	// 检查过期
	if time.Now().After(node.expiresAt) {
		c.removeNode(node)
		delete(c.items, key)
		return nil, false
	}

	// 移动到头部（O(1) 操作）
	c.moveToHead(node)
	node.entry.HitCount++

	return node.entry, true
}

func (c *LRUCache) Set(key string, entry *CacheEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 如果已存在，更新并移动到头部
	if node, ok := c.items[key]; ok {
		node.entry = entry
		node.expiresAt = time.Now().Add(c.ttl)
		c.moveToHead(node)
		return
	}

	// 检查容量，淘汰最久未使用的
	if len(c.items) >= c.capacity {
		c.evictTail()
	}

	// 创建新节点并添加到头部
	node := &lruNode{
		key:       key,
		entry:     entry,
		expiresAt: time.Now().Add(c.ttl),
	}
	c.items[key] = node
	c.addToHead(node)
}

func (c *LRUCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if node, ok := c.items[key]; ok {
		c.removeNode(node)
		delete(c.items, key)
	}
}

func (c *LRUCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[string]*lruNode)
	c.head = nil
	c.tail = nil
}

// addToHead 添加节点到头部 O(1)
func (c *LRUCache) addToHead(node *lruNode) {
	node.prev = nil
	node.next = c.head
	if c.head != nil {
		c.head.prev = node
	}
	c.head = node
	if c.tail == nil {
		c.tail = node
	}
}

// removeNode 从链表中移除节点 O(1)
func (c *LRUCache) removeNode(node *lruNode) {
	if node.prev != nil {
		node.prev.next = node.next
	} else {
		c.head = node.next
	}
	if node.next != nil {
		node.next.prev = node.prev
	} else {
		c.tail = node.prev
	}
}

// moveToHead 移动节点到头部 O(1)
func (c *LRUCache) moveToHead(node *lruNode) {
	if node == c.head {
		return
	}
	c.removeNode(node)
	c.addToHead(node)
}

// evictTail 淘汰尾部节点 O(1)
func (c *LRUCache) evictTail() {
	if c.tail == nil {
		return
	}
	delete(c.items, c.tail.key)
	c.removeNode(c.tail)
}

// Stats 缓存统计
func (c *LRUCache) Stats() (size int, capacity int) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items), c.capacity
}
