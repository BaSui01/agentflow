package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const defaultRedisMemoryKeyPrefix = "agentflow:memory:"

// RedisMemoryStoreConfig configures Redis-backed short-term/working memory.
type RedisMemoryStoreConfig struct {
	// KeyPrefix namespaces all Redis keys owned by this store.
	// Defaults to "agentflow:memory:".
	KeyPrefix string
}

// RedisMemoryStore is a durable MemoryStore backed by Redis.
//
// It is intended for short-term and working memory layers that need TTL,
// restart survival, and cross-process sharing without changing the
// EnhancedMemorySystem MemoryStore interface.
type RedisMemoryStore struct {
	client redis.UniversalClient
	prefix string
	logger *zap.Logger
}

type redisMemoryEnvelope struct {
	CreatedAt time.Time       `json:"created_at"`
	Value     json.RawMessage `json:"value"`
}

// NewRedisMemoryStore creates a Redis-backed MemoryStore.
func NewRedisMemoryStore(client redis.UniversalClient, config RedisMemoryStoreConfig, logger *zap.Logger) (*RedisMemoryStore, error) {
	if client == nil {
		return nil, fmt.Errorf("redis client is required")
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	prefix := config.KeyPrefix
	if prefix == "" {
		prefix = defaultRedisMemoryKeyPrefix
	}
	return &RedisMemoryStore{
		client: client,
		prefix: prefix,
		logger: logger.With(zap.String("component", "memory_store_redis")),
	}, nil
}

func (s *RedisMemoryStore) Save(ctx context.Context, key string, value any, ttl time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if key == "" {
		return fmt.Errorf("key is required")
	}

	rawValue, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal memory value: %w", err)
	}
	rawEnvelope, err := json.Marshal(redisMemoryEnvelope{
		CreatedAt: time.Now().UTC(),
		Value:     rawValue,
	})
	if err != nil {
		return fmt.Errorf("marshal memory envelope: %w", err)
	}

	if err := s.client.Set(ctx, s.redisKey(key), rawEnvelope, ttl).Err(); err != nil {
		return fmt.Errorf("redis set memory %q: %w", key, err)
	}
	return nil
}

func (s *RedisMemoryStore) Load(ctx context.Context, key string) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if key == "" {
		return nil, fmt.Errorf("key is required")
	}

	raw, err := s.client.Get(ctx, s.redisKey(key)).Bytes()
	if err != nil {
		return nil, fmt.Errorf("key %q not found: %w", key, err)
	}
	value, _, err := decodeRedisMemoryEnvelope(raw)
	if err != nil {
		return nil, fmt.Errorf("decode memory %q: %w", key, err)
	}
	return value, nil
}

func (s *RedisMemoryStore) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if key == "" {
		return fmt.Errorf("key is required")
	}
	if err := s.client.Del(ctx, s.redisKey(key)).Err(); err != nil {
		return fmt.Errorf("redis delete memory %q: %w", key, err)
	}
	return nil
}

func (s *RedisMemoryStore) List(ctx context.Context, pattern string, limit int) ([]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	keys, err := s.client.Keys(ctx, s.redisKey(patternOrAll(pattern))).Result()
	if err != nil {
		return nil, fmt.Errorf("redis list memory keys: %w", err)
	}

	type item struct {
		value     any
		createdAt time.Time
	}
	items := make([]item, 0, len(keys))
	for _, key := range keys {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		raw, err := s.client.Get(ctx, key).Bytes()
		if err != nil {
			continue
		}
		value, createdAt, err := decodeRedisMemoryEnvelope(raw)
		if err != nil {
			s.logger.Warn("skipping corrupt redis memory value",
				zap.String("key", strings.TrimPrefix(key, s.prefix)),
				zap.Error(err))
			continue
		}
		items = append(items, item{value: value, createdAt: createdAt})
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

func (s *RedisMemoryStore) Clear(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	keys, err := s.client.Keys(ctx, s.redisKey("*")).Result()
	if err != nil {
		return fmt.Errorf("redis list memory keys: %w", err)
	}
	if len(keys) == 0 {
		return nil
	}
	if err := s.client.Del(ctx, keys...).Err(); err != nil {
		return fmt.Errorf("redis clear memory keys: %w", err)
	}
	return nil
}

func (s *RedisMemoryStore) redisKey(key string) string {
	if key == "" {
		key = "*"
	}
	return s.prefix + key
}

func patternOrAll(pattern string) string {
	if pattern == "" {
		return "*"
	}
	return pattern
}

func decodeRedisMemoryEnvelope(raw []byte) (any, time.Time, error) {
	var env redisMemoryEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, time.Time{}, err
	}

	dec := json.NewDecoder(bytes.NewReader(env.Value))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return nil, time.Time{}, err
	}
	return normalizeRedisJSONValue(value, ""), env.CreatedAt, nil
}

func normalizeRedisJSONValue(value any, key string) any {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for childKey, item := range v {
			out[childKey] = normalizeRedisJSONValue(item, childKey)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = normalizeRedisJSONValue(item, key)
		}
		return out
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return i
		}
		if f, err := v.Float64(); err == nil {
			return f
		}
		return v.String()
	case string:
		if key == "timestamp" || strings.HasSuffix(key, "_at") {
			if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
				return t
			}
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				return t
			}
		}
		return v
	default:
		return value
	}
}
