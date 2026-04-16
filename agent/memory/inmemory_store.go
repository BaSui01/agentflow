package memory

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

type InMemoryMemoryStoreConfig struct {
	// Max Entries是本商店条目的全球封顶.
	// 0表示无限.
	MaxEntries int

	// 现在用于测试。 默认时间 。 现在。
	Now func() time.Time
}

type inMemoryEntry struct {
	value     any
	createdAt time.Time
	expiresAt time.Time
}

// InMemory MemoryStore是一款在TTL支持下的简单记忆Store执行.
// 它用于地方发展、测试和小规模部署。
type InMemoryMemoryStore struct {
	mu      sync.RWMutex
	entries map[string]inMemoryEntry

	maxEntries int
	now        func() time.Time
	logger     *zap.Logger
}

func NewInMemoryMemoryStore(config InMemoryMemoryStoreConfig, logger *zap.Logger) *InMemoryMemoryStore {
	if logger == nil {
		logger = zap.NewNop()
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	return &InMemoryMemoryStore{
		entries:    make(map[string]inMemoryEntry),
		maxEntries: config.MaxEntries,
		now:        now,
		logger:     logger.With(zap.String("component", "memory_store_inmemory")),
	}
}

func (s *InMemoryMemoryStore) Save(ctx context.Context, key string, value any, ttl time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if key == "" {
		return fmt.Errorf("key is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	var expiresAt time.Time
	if ttl > 0 {
		expiresAt = now.Add(ttl)
	}

	s.entries[key] = inMemoryEntry{
		value:     value,
		createdAt: now,
		expiresAt: expiresAt,
	}

	s.cleanupExpiredLocked(now)
	s.evictIfNeededLocked()
	return nil
}

func (s *InMemoryMemoryStore) Load(ctx context.Context, key string) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if key == "" {
		return nil, fmt.Errorf("key is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	s.cleanupExpiredLocked(now)

	ent, ok := s.entries[key]
	if !ok {
		return nil, fmt.Errorf("key %q not found", key)
	}
	return ent.value, nil
}

func (s *InMemoryMemoryStore) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if key == "" {
		return fmt.Errorf("key is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.entries, key)
	return nil
}

func (s *InMemoryMemoryStore) List(ctx context.Context, pattern string, limit int) ([]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	s.cleanupExpiredLocked(now)

	type item struct {
		createdAt time.Time
		value     any
	}
	items := make([]item, 0, len(s.entries))

	for k, ent := range s.entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if pattern == "" || matchWildcard(pattern, k) {
			items = append(items, item{createdAt: ent.createdAt, value: ent.value})
		}
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].createdAt.After(items[j].createdAt)
	})

	if limit <= 0 || limit > len(items) {
		limit = len(items)
	}

	out := make([]any, 0, limit)
	for i := 0; i < limit; i++ {
		out = append(out, items[i].value)
	}
	return out, nil
}

func (s *InMemoryMemoryStore) Clear(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	cleared := len(s.entries)
	s.entries = make(map[string]inMemoryEntry)
	s.logger.Info("memory store cleared", zap.Int("cleared", cleared))
	return nil
}

func (s *InMemoryMemoryStore) cleanupExpiredLocked(now time.Time) {
	if len(s.entries) == 0 {
		return
	}
	for k, ent := range s.entries {
		if !ent.expiresAt.IsZero() && !now.Before(ent.expiresAt) {
			delete(s.entries, k)
		}
	}
}

func (s *InMemoryMemoryStore) evictIfNeededLocked() {
	if s.maxEntries <= 0 || len(s.entries) <= s.maxEntries {
		return
	}

	type kv struct {
		key       string
		createdAt time.Time
	}
	all := make([]kv, 0, len(s.entries))
	for k, ent := range s.entries {
		all = append(all, kv{key: k, createdAt: ent.createdAt})
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].createdAt.Before(all[j].createdAt)
	})

	toEvict := len(s.entries) - s.maxEntries
	for i := 0; i < toEvict && i < len(all); i++ {
		delete(s.entries, all[i].key)
	}
}

func matchWildcard(pattern, s string) bool {
	if pattern == "*" {
		return true
	}
	if !strings.Contains(pattern, "*") {
		return pattern == s
	}

	parts := strings.Split(pattern, "*")
	idx := 0
	for _, p := range parts {
		if p == "" {
			continue
		}
		pos := strings.Index(s[idx:], p)
		if pos < 0 {
			return false
		}
		idx += pos + len(p)
	}
	return true
}
