package persistence

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

// 记忆MessageStore是MessageStore的内在执行.
// 适合开发和测试。 数据在重新启动时丢失 。
type MemoryMessageStore struct {
	messages map[string]*Message // msgID -> Message
	topics   map[string][]string // topic -> []msgID
	mu       sync.RWMutex
	closed   bool
	config   StoreConfig
}

// 新记忆MessageStore 创建了新的记忆信息存储器
func NewMemoryMessageStore(config StoreConfig) *MemoryMessageStore {
	store := &MemoryMessageStore{
		messages: make(map[string]*Message),
		topics:   make(map[string][]string),
		config:   config,
	}

	// 启用后开始清理 goroutine
	if config.Cleanup.Enabled {
		go store.cleanupLoop(config.Cleanup.Interval)
	}

	return store
}

// 关闭商店
func (s *MemoryMessageStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

// 平平检查,如果商店是健康的
func (s *MemoryMessageStore) Ping(ctx context.Context) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return ErrStoreClosed
	}
	return nil
}

// 保存信件坚持一个消息
func (s *MemoryMessageStore) SaveMessage(ctx context.Context, msg *Message) error {
	if msg == nil {
		return ErrInvalidInput
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrStoreClosed
	}

	// 如果没有设定则生成 ID
	if msg.ID == "" {
		msg.ID = uuid.New().String()
	}

	// 设定未设定的创建时间
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now()
	}

	// 存储信件
	s.messages[msg.ID] = msg

	// 添加到主题索引
	if msg.Topic != "" {
		s.topics[msg.Topic] = append(s.topics[msg.Topic], msg.ID)
	}

	return nil
}

// 保存消息在解剖上持续了多个消息
func (s *MemoryMessageStore) SaveMessages(ctx context.Context, msgs []*Message) error {
	if len(msgs) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrStoreClosed
	}

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

		s.messages[msg.ID] = msg

		if msg.Topic != "" {
			s.topics[msg.Topic] = append(s.topics[msg.Topic], msg.ID)
		}
	}

	return nil
}

// 通过 ID 获取信件
func (s *MemoryMessageStore) GetMessage(ctx context.Context, msgID string) (*Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrStoreClosed
	}

	msg, ok := s.messages[msgID]
	if !ok {
		return nil, ErrNotFound
	}

	return msg, nil
}

// GetMessages 获取带有 pagination 主题的信息
func (s *MemoryMessageStore) GetMessages(ctx context.Context, topic string, cursor string, limit int) ([]*Message, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, "", ErrStoreClosed
	}

	msgIDs, ok := s.topics[topic]
	if !ok {
		return []*Message{}, "", nil
	}

	// 根据光标查找启动索引
	startIdx := 0
	if cursor != "" {
		for i, id := range msgIDs {
			if id == cursor {
				startIdx = i + 1
				break
			}
		}
	}

	// 应用限制
	if limit <= 0 {
		limit = 100
	}

	endIdx := startIdx + limit
	if endIdx > len(msgIDs) {
		endIdx = len(msgIDs)
	}

	// 收集信件
	result := make([]*Message, 0, endIdx-startIdx)
	for i := startIdx; i < endIdx; i++ {
		if msg, ok := s.messages[msgIDs[i]]; ok {
			result = append(result, msg)
		}
	}

	// 确定下一个光标
	nextCursor := ""
	if endIdx < len(msgIDs) {
		nextCursor = msgIDs[endIdx-1]
	}

	return result, nextCursor, nil
}

// AckMessage 是一个被承认的信息
func (s *MemoryMessageStore) AckMessage(ctx context.Context, msgID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrStoreClosed
	}

	msg, ok := s.messages[msgID]
	if !ok {
		return ErrNotFound
	}

	now := time.Now()
	msg.AckedAt = &now

	return nil
}

// 获取未保存的邮件获取未确认的比指定时间长的信件
func (s *MemoryMessageStore) GetUnackedMessages(ctx context.Context, topic string, olderThan time.Duration) ([]*Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrStoreClosed
	}

	cutoff := time.Now().Add(-olderThan)
	result := make([]*Message, 0)

	msgIDs, ok := s.topics[topic]
	if !ok {
		return result, nil
	}

	for _, msgID := range msgIDs {
		msg, ok := s.messages[msgID]
		if !ok {
			continue
		}

		// 检查是否未打开和足够老
		if msg.AckedAt == nil && msg.CreatedAt.Before(cutoff) {
			result = append(result, msg)
		}
	}

	return result, nil
}

// GetPendingMessages 检索需要发送的信件
func (s *MemoryMessageStore) GetPendingMessages(ctx context.Context, topic string, limit int) ([]*Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrStoreClosed
	}

	result := make([]*Message, 0)
	now := time.Now()

	msgIDs, ok := s.topics[topic]
	if !ok {
		return result, nil
	}

	for _, msgID := range msgIDs {
		msg, ok := s.messages[msgID]
		if !ok {
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

		if limit > 0 && len(result) >= limit {
			break
		}
	}

	return result, nil
}

// 递增
func (s *MemoryMessageStore) IncrementRetry(ctx context.Context, msgID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrStoreClosed
	}

	msg, ok := s.messages[msgID]
	if !ok {
		return ErrNotFound
	}

	msg.RetryCount++
	now := time.Now()
	msg.LastRetryAt = &now

	return nil
}

// 删除信件从存储处删除
func (s *MemoryMessageStore) DeleteMessage(ctx context.Context, msgID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrStoreClosed
	}

	msg, ok := s.messages[msgID]
	if !ok {
		return ErrNotFound
	}

	// 从主题索引中删除
	if msg.Topic != "" {
		msgIDs := s.topics[msg.Topic]
		for i, id := range msgIDs {
			if id == msgID {
				s.topics[msg.Topic] = append(msgIDs[:i], msgIDs[i+1:]...)
				break
			}
		}
	}

	delete(s.messages, msgID)

	return nil
}

// 清理删除旧消息
func (s *MemoryMessageStore) Cleanup(ctx context.Context, olderThan time.Duration) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return 0, ErrStoreClosed
	}

	cutoff := time.Now().Add(-olderThan)
	count := 0

	for msgID, msg := range s.messages {
		// 删除比截取时间长的被敲击的信件
		if msg.AckedAt != nil && msg.AckedAt.Before(cutoff) {
			// 从主题索引中删除
			if msg.Topic != "" {
				msgIDs := s.topics[msg.Topic]
				for i, id := range msgIDs {
					if id == msgID {
						s.topics[msg.Topic] = append(msgIDs[:i], msgIDs[i+1:]...)
						break
					}
				}
			}
			delete(s.messages, msgID)
			count++
		}

		// 同时删除已过期的信件
		if msg.IsExpired() {
			if msg.Topic != "" {
				msgIDs := s.topics[msg.Topic]
				for i, id := range msgIDs {
					if id == msgID {
						s.topics[msg.Topic] = append(msgIDs[:i], msgIDs[i+1:]...)
						break
					}
				}
			}
			delete(s.messages, msgID)
			count++
		}
	}

	return count, nil
}

// Stats 返回关于消息库的统计数据
func (s *MemoryMessageStore) Stats(ctx context.Context) (*MessageStoreStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrStoreClosed
	}

	stats := &MessageStoreStats{
		TopicCounts: make(map[string]int64),
	}

	var oldestPending time.Time

	for _, msg := range s.messages {
		stats.TotalMessages++

		if msg.AckedAt != nil {
			stats.AckedMessages++
		} else if msg.IsExpired() {
			stats.ExpiredMessages++
		} else {
			stats.PendingMessages++
			if oldestPending.IsZero() || msg.CreatedAt.Before(oldestPending) {
				oldestPending = msg.CreatedAt
			}
		}

		if msg.Topic != "" {
			stats.TopicCounts[msg.Topic]++
		}
	}

	if !oldestPending.IsZero() {
		stats.OldestPendingAge = time.Since(oldestPending)
	}

	return stats, nil
}

// 清理Loop 运行定期清理
func (s *MemoryMessageStore) cleanupLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		s.mu.RLock()
		closed := s.closed
		s.mu.RUnlock()

		if closed {
			return
		}

		_, _ = s.Cleanup(context.Background(), s.config.Cleanup.MessageRetention)
	}
}

// 确保内存MessageStore执行信件Store
var _ MessageStore = (*MemoryMessageStore)(nil)
