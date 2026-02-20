package llm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// ErrCacheMiss 显示缓存丢失 。
var ErrCacheMiss = errors.New("cache miss")

// 快取 Entry 代表缓存响应 。
type CacheEntry struct {
	Response    *ChatResponse `json:"response"`
	TokensSaved int           `json:"tokens_saved"`
	CreatedAt   time.Time     `json:"created_at"`
	ExpiresAt   time.Time     `json:"expires_at"`
	HitCount    int           `json:"hit_count"`
}

// 快取Config 配置缓存 。
type CacheConfig struct {
	LocalMaxSize int           `json:"local_max_size"`
	LocalTTL     time.Duration `json:"local_ttl"`
	RedisTTL     time.Duration `json:"redis_ttl"`
	EnableLocal  bool          `json:"enable_local"`
	EnableRedis  bool          `json:"enable_redis"`
}

// 默认CacheConfig 返回合理的默认值 。
func DefaultCacheConfig() *CacheConfig {
	return &CacheConfig{
		LocalMaxSize: 1000,
		LocalTTL:     5 * time.Minute,
		RedisTTL:     1 * time.Hour,
		EnableLocal:  true,
		EnableRedis:  true,
	}
}

// 多级缓存提供本地 + Redis 缓存.
type MultiLevelCache struct {
	local  *LRUCache
	redis  *redis.Client
	config *CacheConfig
	logger *zap.Logger
}

// 新建多级缓存创建多级缓存 。
func NewMultiLevelCache(rdb *redis.Client, config *CacheConfig, logger *zap.Logger) *MultiLevelCache {
	if config == nil {
		config = DefaultCacheConfig()
	}

	var local *LRUCache
	if config.EnableLocal {
		local = NewLRUCache(config.LocalMaxSize, config.LocalTTL)
	}

	return &MultiLevelCache{
		local:  local,
		redis:  rdb,
		config: config,
		logger: logger,
	}
}

// 从缓存取来
func (c *MultiLevelCache) Get(ctx context.Context, key string) (*CacheEntry, error) {
	if c.config.EnableLocal && c.local != nil {
		if entry, ok := c.local.Get(key); ok {
			return entry, nil
		}
	}

	if c.config.EnableRedis && c.redis != nil {
		data, err := c.redis.Get(ctx, c.redisKey(key)).Bytes()
		if err == nil {
			var entry CacheEntry
			if err := json.Unmarshal(data, &entry); err == nil {
				if c.config.EnableLocal && c.local != nil {
					c.local.Set(key, &entry)
				}
				return &entry, nil
			}
		}
	}

	return nil, ErrCacheMiss
}

// 设置缓存存储 。
func (c *MultiLevelCache) Set(ctx context.Context, key string, entry *CacheEntry) error {
	entry.CreatedAt = time.Now()
	entry.ExpiresAt = time.Now().Add(c.config.RedisTTL)

	if c.config.EnableLocal && c.local != nil {
		c.local.Set(key, entry)
	}

	if c.config.EnableRedis && c.redis != nil {
		data, err := json.Marshal(entry)
		if err != nil {
			return fmt.Errorf("marshal cache entry: %w", err)
		}
		return c.redis.Set(ctx, c.redisKey(key), data, c.config.RedisTTL).Err()
	}

	return nil
}

// 生成 Key 从请求生成缓存密钥 。
func (c *MultiLevelCache) GenerateKey(req *ChatRequest) string {
	data, _ := json.Marshal(struct {
		Model    string `json:"model"`
		Messages any    `json:"messages"`
	}{
		Model:    req.Model,
		Messages: req.Messages,
	})
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:16])
}

// 如果请求是可缓存的, 则可以缓存 。
func (c *MultiLevelCache) IsCacheable(req *ChatRequest) bool {
	return len(req.Tools) == 0
}

func (c *MultiLevelCache) redisKey(key string) string {
	return "llm:cache:" + key
}

// LRUCache是一个简单的LRU缓存.
type LRUCache struct {
	mu       sync.RWMutex
	capacity int
	ttl      time.Duration
	items    map[string]*lruNode
	head     *lruNode
	tail     *lruNode
}

type lruNode struct {
	key       string
	entry     *CacheEntry
	expiresAt time.Time
	prev      *lruNode
	next      *lruNode
}

// NewLRUCAche创建了新的LRU缓存.
func NewLRUCache(capacity int, ttl time.Duration) *LRUCache {
	return &LRUCache{
		capacity: capacity,
		ttl:      ttl,
		items:    make(map[string]*lruNode),
	}
}

// 从缓存取来
func (c *LRUCache) Get(key string) (*CacheEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	node, ok := c.items[key]
	if !ok {
		return nil, false
	}

	if time.Now().After(node.expiresAt) {
		c.removeNode(node)
		delete(c.items, key)
		return nil, false
	}

	c.moveToHead(node)
	node.entry.HitCount++
	return node.entry, true
}

// 设置缓存存储 。
func (c *LRUCache) Set(key string, entry *CacheEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if node, ok := c.items[key]; ok {
		node.entry = entry
		node.expiresAt = time.Now().Add(c.ttl)
		c.moveToHead(node)
		return
	}

	if len(c.items) >= c.capacity {
		c.evictTail()
	}

	node := &lruNode{
		key:       key,
		entry:     entry,
		expiresAt: time.Now().Add(c.ttl),
	}
	c.items[key] = node
	c.addToHead(node)
}

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

func (c *LRUCache) moveToHead(node *lruNode) {
	if node == c.head {
		return
	}
	c.removeNode(node)
	c.addToHead(node)
}

func (c *LRUCache) evictTail() {
	if c.tail == nil {
		return
	}
	delete(c.items, c.tail.key)
	c.removeNode(c.tail)
}
