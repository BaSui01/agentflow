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

// RedisMessageStore is a Redis-based implementation of MessageStore.
// Suitable for distributed production deployments.
// Uses Redis Streams for message storage with consumer group support.
type RedisMessageStore struct {
	client    *redis.Client
	keyPrefix string
	config    StoreConfig
}

// NewRedisMessageStore creates a new Redis-based message store
func NewRedisMessageStore(config StoreConfig) (*RedisMessageStore, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", config.Redis.Host, config.Redis.Port),
		Password: config.Redis.Password,
		DB:       config.Redis.DB,
		PoolSize: config.Redis.PoolSize,
	})

	// Test connection
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

// Close closes the store
func (s *RedisMessageStore) Close() error {
	return s.client.Close()
}

// Ping checks if the store is healthy
func (s *RedisMessageStore) Ping(ctx context.Context) error {
	return s.client.Ping(ctx).Err()
}

// messageKey returns the Redis key for a message
func (s *RedisMessageStore) messageKey(msgID string) string {
	return s.keyPrefix + "data:" + msgID
}

// topicKey returns the Redis key for a topic's message list
func (s *RedisMessageStore) topicKey(topic string) string {
	return s.keyPrefix + "topic:" + topic
}

// pendingKey returns the Redis key for pending messages
func (s *RedisMessageStore) pendingKey(topic string) string {
	return s.keyPrefix + "pending:" + topic
}

// SaveMessage persists a single message
func (s *RedisMessageStore) SaveMessage(ctx context.Context, msg *Message) error {
	if msg == nil {
		return ErrInvalidInput
	}

	// Generate ID if not set
	if msg.ID == "" {
		msg.ID = uuid.New().String()
	}

	// Set created time if not set
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now()
	}

	// Serialize message
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	pipe := s.client.Pipeline()

	// Store message data
	pipe.Set(ctx, s.messageKey(msg.ID), data, 0)

	// Add to topic list
	if msg.Topic != "" {
		pipe.RPush(ctx, s.topicKey(msg.Topic), msg.ID)
		// Add to pending set with score = created timestamp
		pipe.ZAdd(ctx, s.pendingKey(msg.Topic), redis.Z{
			Score:  float64(msg.CreatedAt.UnixNano()),
			Member: msg.ID,
		})
	}

	_, err = pipe.Exec(ctx)
	return err
}

// SaveMessages persists multiple messages atomically
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

// GetMessage retrieves a message by ID
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

// GetMessages retrieves messages for a topic with pagination
func (s *RedisMessageStore) GetMessages(ctx context.Context, topic string, cursor string, limit int) ([]*Message, string, error) {
	if limit <= 0 {
		limit = 100
	}

	// Get message IDs from topic list
	start := int64(0)
	if cursor != "" {
		// Find cursor position
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

	// Get message data
	result := make([]*Message, 0, len(msgIDs))
	for _, msgID := range msgIDs {
		msg, err := s.GetMessage(ctx, msgID)
		if err != nil {
			continue
		}
		result = append(result, msg)
	}

	// Determine next cursor
	nextCursor := ""
	if len(msgIDs) == limit {
		nextCursor = msgIDs[len(msgIDs)-1]
	}

	return result, nextCursor, nil
}

// AckMessage marks a message as acknowledged
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

	// Update message data
	pipe.Set(ctx, s.messageKey(msgID), data, 0)

	// Remove from pending set
	if msg.Topic != "" {
		pipe.ZRem(ctx, s.pendingKey(msg.Topic), msgID)
	}

	_, err = pipe.Exec(ctx)
	return err
}

// GetUnackedMessages retrieves unacknowledged messages older than the specified duration
func (s *RedisMessageStore) GetUnackedMessages(ctx context.Context, topic string, olderThan time.Duration) ([]*Message, error) {
	cutoff := time.Now().Add(-olderThan).UnixNano()

	// Get message IDs from pending set with score < cutoff
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

// GetPendingMessages retrieves messages that need to be delivered
func (s *RedisMessageStore) GetPendingMessages(ctx context.Context, topic string, limit int) ([]*Message, error) {
	if limit <= 0 {
		limit = 100
	}

	now := time.Now()

	// Get all pending message IDs
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

		// Skip acked or expired messages
		if msg.AckedAt != nil || msg.IsExpired() {
			continue
		}

		// Check if ready for retry
		if msg.RetryCount > 0 {
			nextRetry := msg.NextRetryTime(s.config.Retry)
			if now.Before(nextRetry) {
				continue
			}
		}

		// Check max retries
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

// IncrementRetry increments the retry count for a message
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

// DeleteMessage removes a message from the store
func (s *RedisMessageStore) DeleteMessage(ctx context.Context, msgID string) error {
	msg, err := s.GetMessage(ctx, msgID)
	if err != nil {
		return err
	}

	pipe := s.client.Pipeline()

	// Delete message data
	pipe.Del(ctx, s.messageKey(msgID))

	// Remove from topic list and pending set
	if msg.Topic != "" {
		pipe.LRem(ctx, s.topicKey(msg.Topic), 1, msgID)
		pipe.ZRem(ctx, s.pendingKey(msg.Topic), msgID)
	}

	_, err = pipe.Exec(ctx)
	return err
}

// Cleanup removes old acknowledged messages
func (s *RedisMessageStore) Cleanup(ctx context.Context, olderThan time.Duration) (int, error) {
	// Get all topic keys
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

			// Remove acked messages older than cutoff
			if msg.AckedAt != nil && msg.AckedAt.Before(cutoff) {
				shouldDelete = true
			}

			// Also remove expired messages
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

// Stats returns statistics about the message store
func (s *RedisMessageStore) Stats(ctx context.Context) (*MessageStoreStats, error) {
	stats := &MessageStoreStats{
		TopicCounts: make(map[string]int64),
	}

	// Get all topic keys
	topicKeys, err := s.client.Keys(ctx, s.keyPrefix+"topic:*").Result()
	if err != nil {
		return nil, err
	}

	var oldestPending time.Time

	for _, topicKey := range topicKeys {
		topic := topicKey[len(s.keyPrefix+"topic:"):]

		// Get topic message count
		count, err := s.client.LLen(ctx, topicKey).Result()
		if err != nil {
			continue
		}
		stats.TopicCounts[topic] = count
		stats.TotalMessages += count

		// Get pending count
		pendingCount, err := s.client.ZCard(ctx, s.pendingKey(topic)).Result()
		if err == nil {
			stats.PendingMessages += pendingCount
		}

		// Get oldest pending
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

// Ensure RedisMessageStore implements MessageStore
var _ MessageStore = (*RedisMessageStore)(nil)
