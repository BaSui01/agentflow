package core

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// RedisCheckpointStore Redis 检查点存储
type RedisCheckpointStore struct {
	client RedisClient
	prefix string
	ttl    time.Duration
	logger *zap.Logger
}

// RedisClient Redis 客户端接口
type RedisClient interface {
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Get(ctx context.Context, key string) ([]byte, error)
	Delete(ctx context.Context, key string) error
	Keys(ctx context.Context, pattern string) ([]string, error)
	ZAdd(ctx context.Context, key string, score float64, member string) error
	ZRevRange(ctx context.Context, key string, start, stop int64) ([]string, error)
	ZRemRangeByScore(ctx context.Context, key string, min, max string) error
}

// NewRedisCheckpointStore 创建 Redis 检查点存储
func NewRedisCheckpointStore(client RedisClient, prefix string, ttl time.Duration, logger *zap.Logger) *RedisCheckpointStore {
	return &RedisCheckpointStore{
		client: client,
		prefix: prefix,
		ttl:    ttl,
		logger: logger.With(zap.String("store", "redis_checkpoint")),
	}
}

// Save 保存检查点
func (s *RedisCheckpointStore) Save(ctx context.Context, checkpoint *Checkpoint) error {
	if checkpoint.Version == 0 {
		versions, err := s.ListVersions(ctx, checkpoint.ThreadID)
		if err == nil && len(versions) > 0 {
			maxVersion := 0
			for _, v := range versions {
				if v.Version > maxVersion {
					maxVersion = v.Version
				}
			}
			checkpoint.Version = maxVersion + 1
		} else {
			checkpoint.Version = 1
		}
	}

	data, err := json.Marshal(checkpoint)
	if err != nil {
		return fmt.Errorf("failed to marshal checkpoint: %w", err)
	}

	key := s.checkpointKey(checkpoint.ID)
	if err := s.client.Set(ctx, key, data, s.ttl); err != nil {
		return fmt.Errorf("save checkpoint data to redis: %w", err)
	}

	threadKey := s.threadKey(checkpoint.ThreadID)
	score := float64(checkpoint.CreatedAt.Unix())
	if err := s.client.ZAdd(ctx, threadKey, score, checkpoint.ID); err != nil {
		return fmt.Errorf("add checkpoint to thread index: %w", err)
	}

	s.logger.Debug("checkpoint saved to redis",
		zap.String("checkpoint_id", checkpoint.ID),
		zap.String("thread_id", checkpoint.ThreadID),
		zap.Int("version", checkpoint.Version),
	)

	return nil
}

// Load 加载检查点
func (s *RedisCheckpointStore) Load(ctx context.Context, checkpointID string) (*Checkpoint, error) {
	key := s.checkpointKey(checkpointID)
	data, err := s.client.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("get checkpoint from redis: %w", err)
	}

	var checkpoint Checkpoint
	if err := json.Unmarshal(data, &checkpoint); err != nil {
		return nil, fmt.Errorf("failed to unmarshal checkpoint: %w", err)
	}

	return &checkpoint, nil
}

// LoadLatest 加载最新检查点
func (s *RedisCheckpointStore) LoadLatest(ctx context.Context, threadID string) (*Checkpoint, error) {
	threadKey := s.threadKey(threadID)

	ids, err := s.client.ZRevRange(ctx, threadKey, 0, 0)
	if err != nil {
		return nil, fmt.Errorf("get latest checkpoint ID: %w", err)
	}

	if len(ids) == 0 {
		return nil, fmt.Errorf("no checkpoints found for thread: %s", threadID)
	}

	return s.Load(ctx, ids[0])
}

// List 列出检查点
func (s *RedisCheckpointStore) List(ctx context.Context, threadID string, limit int) ([]*Checkpoint, error) {
	threadKey := s.threadKey(threadID)

	ids, err := s.client.ZRevRange(ctx, threadKey, 0, int64(limit-1))
	if err != nil {
		return nil, fmt.Errorf("list checkpoint IDs: %w", err)
	}

	checkpoints := make([]*Checkpoint, 0, len(ids))
	for _, id := range ids {
		checkpoint, err := s.Load(ctx, id)
		if err != nil {
			s.logger.Warn("failed to load checkpoint",
				zap.String("checkpoint_id", id),
				zap.String("thread_id", threadID),
				zap.String("operation", "list"),
				zap.Error(err),
			)
			continue
		}
		checkpoints = append(checkpoints, checkpoint)
	}

	return checkpoints, nil
}

// Delete 删除检查点
func (s *RedisCheckpointStore) Delete(ctx context.Context, checkpointID string) error {
	key := s.checkpointKey(checkpointID)
	return s.client.Delete(ctx, key)
}

// DeleteThread 删除线程
func (s *RedisCheckpointStore) DeleteThread(ctx context.Context, threadID string) error {
	checkpoints, err := s.List(ctx, threadID, 1000)
	if err != nil {
		return fmt.Errorf("list checkpoints for thread deletion: %w", err)
	}

	for _, checkpoint := range checkpoints {
		if err := s.Delete(ctx, checkpoint.ID); err != nil {
			s.logger.Warn("failed to delete checkpoint",
				zap.String("checkpoint_id", checkpoint.ID),
				zap.String("thread_id", threadID),
				zap.String("operation", "delete_thread"),
				zap.Error(err),
			)
		}
	}

	threadKey := s.threadKey(threadID)
	return s.client.Delete(ctx, threadKey)
}

func (s *RedisCheckpointStore) checkpointKey(id string) string {
	return fmt.Sprintf("%s:checkpoint:%s", s.prefix, id)
}

func (s *RedisCheckpointStore) threadKey(threadID string) string {
	return fmt.Sprintf("%s:thread:%s", s.prefix, threadID)
}

// LoadVersion 加载指定版本的检查点
func (s *RedisCheckpointStore) LoadVersion(ctx context.Context, threadID string, version int) (*Checkpoint, error) {
	versions, err := s.ListVersions(ctx, threadID)
	if err != nil {
		return nil, fmt.Errorf("list versions for load: %w", err)
	}

	for _, v := range versions {
		if v.Version == version {
			return s.Load(ctx, v.ID)
		}
	}

	return nil, fmt.Errorf("version %d not found for thread %s", version, threadID)
}

// ListVersions 列出线程的所有版本
func (s *RedisCheckpointStore) ListVersions(ctx context.Context, threadID string) ([]CheckpointVersion, error) {
	checkpoints, err := s.List(ctx, threadID, 1000)
	if err != nil {
		return nil, fmt.Errorf("list checkpoints for versions: %w", err)
	}

	versions := make([]CheckpointVersion, 0, len(checkpoints))
	for _, cp := range checkpoints {
		versions = append(versions, CheckpointVersion{
			Version:   cp.Version,
			ID:        cp.ID,
			CreatedAt: cp.CreatedAt,
			State:     cp.State,
			Summary:   fmt.Sprintf("Checkpoint at %s", cp.CreatedAt.Format(time.RFC3339)),
		})
	}

	return versions, nil
}

// Rollback 回滚到指定版本
func (s *RedisCheckpointStore) Rollback(ctx context.Context, threadID string, version int) error {
	checkpoint, err := s.LoadVersion(ctx, threadID, version)
	if err != nil {
		return fmt.Errorf("load version %d for rollback: %w", version, err)
	}

	newCheckpoint := *checkpoint
	newCheckpoint.ID = generateCheckpointID()
	newCheckpoint.CreatedAt = time.Now()
	newCheckpoint.ParentID = checkpoint.ID

	versions, err := s.ListVersions(ctx, threadID)
	if err != nil {
		return fmt.Errorf("list versions for rollback: %w", err)
	}

	maxVersion := 0
	for _, v := range versions {
		if v.Version > maxVersion {
			maxVersion = v.Version
		}
	}

	newCheckpoint.Version = maxVersion + 1

	if newCheckpoint.Metadata == nil {
		newCheckpoint.Metadata = make(map[string]any)
	}
	newCheckpoint.Metadata["rollback_from_version"] = version

	return s.Save(ctx, &newCheckpoint)
}
