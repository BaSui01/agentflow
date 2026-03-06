package storage

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const (
	defaultReferenceStoreKeyPrefix = "agentflow:mm:ref"
	redisStoreOpTimeout           = 5 * time.Second
)

type redisReferenceAsset struct {
	ID        string    `json:"id"`
	FileName  string    `json:"file_name"`
	MimeType  string    `json:"mime_type"`
	Size      int       `json:"size"`
	CreatedAt time.Time `json:"created_at"`
	Data      []byte    `json:"data"`
}

// RedisReferenceStore implements ReferenceStore using Redis.
type RedisReferenceStore struct {
	client    *redis.Client
	keyPrefix string
	ttl       time.Duration
	logger    *zap.Logger
}

// NewRedisReferenceStore creates a Redis-backed reference store.
func NewRedisReferenceStore(client *redis.Client, keyPrefix string, ttl time.Duration, logger *zap.Logger) *RedisReferenceStore {
	if logger == nil {
		logger = zap.NewNop()
	}
	keyPrefix = strings.TrimSpace(keyPrefix)
	if keyPrefix == "" {
		keyPrefix = defaultReferenceStoreKeyPrefix
	}
	if ttl <= 0 {
		ttl = 2 * time.Hour
	}
	return &RedisReferenceStore{
		client:    client,
		keyPrefix: strings.TrimSuffix(keyPrefix, ":"),
		ttl:       ttl,
		logger:    logger.With(zap.String("component", "multimodal_reference_store"), zap.String("backend", "redis")),
	}
}

func (s *RedisReferenceStore) Save(asset *ReferenceAsset) error {
	if asset == nil || s.client == nil {
		return nil
	}
	payload, err := json.Marshal(redisReferenceAsset{
		ID:        asset.ID,
		FileName:  asset.FileName,
		MimeType:  asset.MimeType,
		Size:      asset.Size,
		CreatedAt: asset.CreatedAt,
		Data:      asset.Data,
	})
	if err != nil {
		s.logger.Error("failed to marshal reference asset", zap.String("reference_id", asset.ID), zap.Error(err))
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), redisStoreOpTimeout)
	defer cancel()
	if err := s.client.Set(ctx, s.keyFor(asset.ID), payload, s.ttl).Err(); err != nil {
		s.logger.Error("failed to save reference asset to redis", zap.String("reference_id", asset.ID), zap.Error(err))
		return err
	}
	return nil
}

func (s *RedisReferenceStore) Get(id string) (*ReferenceAsset, bool) {
	if s.client == nil {
		return nil, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), redisStoreOpTimeout)
	defer cancel()

	raw, err := s.client.Get(ctx, s.keyFor(id)).Bytes()
	if err == redis.Nil {
		return nil, false
	}
	if err != nil {
		s.logger.Error("failed to get reference asset from redis", zap.String("reference_id", id), zap.Error(err))
		return nil, false
	}

	var stored redisReferenceAsset
	if err := json.Unmarshal(raw, &stored); err != nil {
		s.logger.Error("failed to unmarshal reference asset", zap.String("reference_id", id), zap.Error(err))
		return nil, false
	}

	return &ReferenceAsset{
		ID:        stored.ID,
		FileName:  stored.FileName,
		MimeType:  stored.MimeType,
		Size:      stored.Size,
		CreatedAt: stored.CreatedAt,
		Data:      append([]byte(nil), stored.Data...),
	}, true
}

func (s *RedisReferenceStore) Delete(id string) {
	if s.client == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), redisStoreOpTimeout)
	defer cancel()
	if err := s.client.Del(ctx, s.keyFor(id)).Err(); err != nil {
		s.logger.Error("failed to delete reference asset from redis", zap.String("reference_id", id), zap.Error(err))
	}
}

func (s *RedisReferenceStore) Cleanup(expireBefore time.Time) {
	// Redis TTL handles key expiration automatically.
	_ = expireBefore
}

func (s *RedisReferenceStore) keyFor(id string) string {
	return s.keyPrefix + ":" + id
}
