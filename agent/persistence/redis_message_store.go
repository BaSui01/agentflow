package persistence

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// RedisMessageStore是一个基于Redis的MessageStore执行.
// 适合分布式生产部署.
// 使用 Redis Streams 在消费组的支持下存储消息.
type RedisMessageStore struct {
	client    *redis.Client
	keyPrefix string
	config    StoreConfig
}

// NewRedisMessageStore 创建一个新的基于 Redis 的信息存储
func NewRedisMessageStore(config StoreConfig) (*RedisMessageStore, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", config.Redis.Host, config.Redis.Port),
		Password: config.Redis.Password,
		DB:       config.Redis.DB,
		PoolSize: config.Redis.PoolSize,
	})

	// 测试连接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	keyPrefix := config.Redis.KeyPrefix
	if keyPrefix == "" {
		keyPrefix = "agentflow:"
	}

	store := &RedisMessageStore{
		client:    client,
		keyPrefix: keyPrefix + "msg:",
		config:    config,
	}

	return store, nil
}

// 关闭商店
func (s *RedisMessageStore) Close() error {
	return s.client.Close()
}

// 平平检查,如果商店是健康的
func (s *RedisMessageStore) Ping(ctx context.Context) error {
	return s.client.Ping(ctx).Err()
}

// 消息Key 返回消息的 Redis 密钥
func (s *RedisMessageStore) messageKey(msgID string) string {
	return s.keyPrefix + "data:" + msgID
}

// 主题Key 返回主题信件列表的 Redis 密钥
func (s *RedisMessageStore) topicKey(topic string) string {
	return s.keyPrefix + "topic:" + topic
}

// 待决 Key 返回待决信件的 Redis 密钥
func (s *RedisMessageStore) pendingKey(topic string) string {
	return s.keyPrefix + "pending:" + topic
}

// 保存信件坚持一个消息
func (s *RedisMessageStore) SaveMessage(ctx context.Context, msg *Message) error {
	if msg == nil {
		return ErrInvalidInput
	}

	// 如果没有设定则生成 ID
	if msg.ID == "" {
		msg.ID = uuid.New().String()
	}

	// 设定未设定的创建时间
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now()
	}

	// 序列化信件
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	pipe := s.client.Pipeline()

	// 存储信件数据
	pipe.Set(ctx, s.messageKey(msg.ID), data, 0)

	// 添加到主题列表
	if msg.Topic != "" {
		pipe.RPush(ctx, s.topicKey(msg.Topic), msg.ID)
		// 添加到待定的设定中, 有分数 = 创建时间戳
		pipe.ZAdd(ctx, s.pendingKey(msg.Topic), redis.Z{
			Score:  float64(msg.CreatedAt.UnixNano()),
			Member: msg.ID,
		})
	}

	_, err = pipe.Exec(ctx)
	return err
}

// 保存消息在解剖上持续了多个消息
func (s *RedisMessageStore) SaveMessages(ctx context.Context, msgs []*Message) error {
	if len(msgs) == 0 {
		return nil
	}

	pipe := s.client.Pipeline()

	for _, msg := range msgs {
		if msg == nil {
			continue
		}

		if msg.ID == "" {
			msg.ID = uuid.New().String()
		}
		if msg.CreatedAt.IsZero() {
			msg.CreatedAt = time.Now()
		}

		data, err := json.Marshal(msg)
		if err != nil {
			return fmt.Errorf("failed to marshal message: %w", err)
		}

		pipe.Set(ctx, s.messageKey(msg.ID), data, 0)

		if msg.Topic != "" {
			pipe.RPush(ctx, s.topicKey(msg.Topic), msg.ID)
			pipe.ZAdd(ctx, s.pendingKey(msg.Topic), redis.Z{
				Score:  float64(msg.CreatedAt.UnixNano()),
				Member: msg.ID,
			})
		}
	}

	_, err := pipe.Exec(ctx)
	return err
}

// 通过 ID 获取信件
func (s *RedisMessageStore) GetMessage(ctx context.Context, msgID string) (*Message, error) {
	data, err := s.client.Get(ctx, s.messageKey(msgID)).Bytes()
	if err == redis.Nil {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}

	return &msg, nil
}

// GetMessages 获取带有 pagination 主题的信息
func (s *RedisMessageStore) GetMessages(ctx context.Context, topic string, cursor string, limit int) ([]*Message, string, error) {
	if limit <= 0 {
		limit = 100
	}

	// 从主题列表获取消息ID
	start := int64(0)
	if cursor != "" {
		// 查找光标位置
		pos, err := s.client.LPos(ctx, s.topicKey(topic), cursor, redis.LPosArgs{}).Result()
		if err == nil {
			start = pos + 1
		}
	}

	msgIDs, err := s.client.LRange(ctx, s.topicKey(topic), start, start+int64(limit)-1).Result()
	if err != nil {
		return nil, "", err
	}

	if len(msgIDs) == 0 {
		return []*Message{}, "", nil
	}

	// 获取消息数据
	result := make([]*Message, 0, len(msgIDs))
	for _, msgID := range msgIDs {
		msg, err := s.GetMessage(ctx, msgID)
		if err != nil {
			continue
		}
		result = append(result, msg)
	}

	// 确定下一个光标
	nextCursor := ""
	if len(msgIDs) == limit {
		nextCursor = msgIDs[len(msgIDs)-1]
	}

	return result, nextCursor, nil
}

// AckMessage 是一个被承认的信息
func (s *RedisMessageStore) AckMessage(ctx context.Context, msgID string) error {
	msg, err := s.GetMessage(ctx, msgID)
	if err != nil {
		return err
	}

	now := time.Now()
	msg.AckedAt = &now

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	pipe := s.client.Pipeline()

	// 更新消息数据
	pipe.Set(ctx, s.messageKey(msgID), data, 0)

	// 删除待办集
	if msg.Topic != "" {
		pipe.ZRem(ctx, s.pendingKey(msg.Topic), msgID)
	}

	_, err = pipe.Exec(ctx)
	return err
}

// 获取未保存的邮件获取未确认的比指定时间长的信件
func (s *RedisMessageStore) GetUnackedMessages(ctx context.Context, topic string, olderThan time.Duration) ([]*Message, error) {
	cutoff := time.Now().Add(-olderThan).UnixNano()

	// 从待定的设定中获取消息ID, 并设定分数 < cutoffee
	msgIDs, err := s.client.ZRangeByScore(ctx, s.pendingKey(topic), &redis.ZRangeBy{
		Min: "-inf",
		Max: strconv.FormatInt(cutoff, 10),
	}).Result()
	if err != nil {
		return nil, err
	}

	result := make([]*Message, 0, len(msgIDs))
	for _, msgID := range msgIDs {
		msg, err := s.GetMessage(ctx, msgID)
		if err != nil {
			continue
		}
		if msg.AckedAt == nil {
			result = append(result, msg)
		}
	}

	return result, nil
}

// GetPendingMessages 检索需要发送的信件
func (s *RedisMessageStore) GetPendingMessages(ctx context.Context, topic string, limit int) ([]*Message, error) {
	if limit <= 0 {
		limit = 100
	}

	now := time.Now()

	// 获取所有待处理信件ID
	msgIDs, err := s.client.ZRange(ctx, s.pendingKey(topic), 0, int64(limit*2)).Result()
	if err != nil {
		return nil, err
	}

	result := make([]*Message, 0)
	for _, msgID := range msgIDs {
		msg, err := s.GetMessage(ctx, msgID)
		if err != nil {
			continue
		}

		// 跳过已锁定或已过期的信件
		if msg.AckedAt != nil || msg.IsExpired() {
			continue
		}

		// 检查是否准备好重试
		if msg.RetryCount > 0 {
			nextRetry := msg.NextRetryTime(s.config.Retry)
			if now.Before(nextRetry) {
				continue
			}
		}

		// 检查最大重试
		if msg.RetryCount >= s.config.Retry.MaxRetries {
			continue
		}

		result = append(result, msg)

		if len(result) >= limit {
			break
		}
	}

	return result, nil
}

// 递增
func (s *RedisMessageStore) IncrementRetry(ctx context.Context, msgID string) error {
	msg, err := s.GetMessage(ctx, msgID)
	if err != nil {
		return err
	}

	msg.RetryCount++
	now := time.Now()
	msg.LastRetryAt = &now

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	return s.client.Set(ctx, s.messageKey(msgID), data, 0).Err()
}

// 删除信件从存储处删除
func (s *RedisMessageStore) DeleteMessage(ctx context.Context, msgID string) error {
	msg, err := s.GetMessage(ctx, msgID)
	if err != nil {
		return err
	}

	pipe := s.client.Pipeline()

	// 删除信件数据
	pipe.Del(ctx, s.messageKey(msgID))

	// 从议题列表中删除并待办集
	if msg.Topic != "" {
		pipe.LRem(ctx, s.topicKey(msg.Topic), 1, msgID)
		pipe.ZRem(ctx, s.pendingKey(msg.Topic), msgID)
	}

	_, err = pipe.Exec(ctx)
	return err
}

// 清理删除旧消息
func (s *RedisMessageStore) Cleanup(ctx context.Context, olderThan time.Duration) (int, error) {
	// 获取全部话题密钥
	topicKeys, err := s.client.Keys(ctx, s.keyPrefix+"topic:*").Result()
	if err != nil {
		return 0, err
	}

	cutoff := time.Now().Add(-olderThan)
	count := 0

	for _, topicKey := range topicKeys {
		msgIDs, err := s.client.LRange(ctx, topicKey, 0, -1).Result()
		if err != nil {
			continue
		}

		for _, msgID := range msgIDs {
			msg, err := s.GetMessage(ctx, msgID)
			if err != nil {
				continue
			}

			shouldDelete := false

			// 删除比截取时间长的被敲击的信件
			if msg.AckedAt != nil && msg.AckedAt.Before(cutoff) {
				shouldDelete = true
			}

			// 同时删除已过期的信件
			if msg.IsExpired() {
				shouldDelete = true
			}

			if shouldDelete {
				if err := s.DeleteMessage(ctx, msgID); err == nil {
					count++
				}
			}
		}
	}

	return count, nil
}

// Stats 返回关于消息库的统计数据
func (s *RedisMessageStore) Stats(ctx context.Context) (*MessageStoreStats, error) {
	stats := &MessageStoreStats{
		TopicCounts: make(map[string]int64),
	}

	// 获取全部话题密钥
	topicKeys, err := s.client.Keys(ctx, s.keyPrefix+"topic:*").Result()
	if err != nil {
		return nil, err
	}

	var oldestPending time.Time

	for _, topicKey := range topicKeys {
		topic := topicKey[len(s.keyPrefix+"topic:"):]

		// 获取话题信息计数
		count, err := s.client.LLen(ctx, topicKey).Result()
		if err != nil {
			continue
		}
		stats.TopicCounts[topic] = count
		stats.TotalMessages += count

		// 获取待计数
		pendingCount, err := s.client.ZCard(ctx, s.pendingKey(topic)).Result()
		if err == nil {
			stats.PendingMessages += pendingCount
		}

		// 获得最年长的等待
		oldest, err := s.client.ZRangeWithScores(ctx, s.pendingKey(topic), 0, 0).Result()
		if err == nil && len(oldest) > 0 {
			ts := time.Unix(0, int64(oldest[0].Score))
			if oldestPending.IsZero() || ts.Before(oldestPending) {
				oldestPending = ts
			}
		}
	}

	stats.AckedMessages = stats.TotalMessages - stats.PendingMessages

	if !oldestPending.IsZero() {
		stats.OldestPendingAge = time.Since(oldestPending)
	}

	return stats, nil
}

// 确保 RedisMessageStore 执行信件系统
var _ MessageStore = (*RedisMessageStore)(nil)
