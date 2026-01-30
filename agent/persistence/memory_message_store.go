package persistence

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

// MemoryMessageStore is an in-memory implementation of MessageStore.
// Suitable for development and testing. Data is lost on restart.
type MemoryMessageStore struct {
	messages map[string]*Message // msgID -> Message
	topics   map[string][]string // topic -> []msgID
	mu       sync.RWMutex
	closed   bool
	config   StoreConfig
}

// NewMemoryMessageStore creates a new in-memory message store
func NewMemoryMessageStore(config StoreConfig) *MemoryMessageStore {
	store := &MemoryMessageStore{
		messages: make(map[string]*Message),
		topics:   make(map[string][]string),
		config:   config,
	}

	// Start cleanup goroutine if enabled
	if config.Cleanup.Enabled {
		go store.cleanupLoop(config.Cleanup.Interval)
	}

	return store
}

// Close closes the store
func (s *MemoryMessageStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

// Ping checks if the store is healthy
func (s *MemoryMessageStore) Ping(ctx context.Context) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return ErrStoreClosed
	}
	return nil
}

// SaveMessage persists a single message
func (s *MemoryMessageStore) SaveMessage(ctx context.Context, msg *Message) error {
	if msg == nil {
		return ErrInvalidInput
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrStoreClosed
	}

	// Generate ID if not set
	if msg.ID == "" {
		msg.ID = uuid.New().String()
	}

	// Set created time if not set
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now()
	}

	// Store message
	s.messages[msg.ID] = msg

	// Add to topic index
	if msg.Topic != "" {
		s.topics[msg.Topic] = append(s.topics[msg.Topic], msg.ID)
	}

	return nil
}

// SaveMessages persists multiple messages atomically
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

// GetMessage retrieves a message by ID
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

// GetMessages retrieves messages for a topic with pagination
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

	// Find start index based on cursor
	startIdx := 0
	if cursor != "" {
		for i, id := range msgIDs {
			if id == cursor {
				startIdx = i + 1
				break
			}
		}
	}

	// Apply limit
	if limit <= 0 {
		limit = 100
	}

	endIdx := startIdx + limit
	if endIdx > len(msgIDs) {
		endIdx = len(msgIDs)
	}

	// Collect messages
	result := make([]*Message, 0, endIdx-startIdx)
	for i := startIdx; i < endIdx; i++ {
		if msg, ok := s.messages[msgIDs[i]]; ok {
			result = append(result, msg)
		}
	}

	// Determine next cursor
	nextCursor := ""
	if endIdx < len(msgIDs) {
		nextCursor = msgIDs[endIdx-1]
	}

	return result, nextCursor, nil
}

// AckMessage marks a message as acknowledged
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

// GetUnackedMessages retrieves unacknowledged messages older than the specified duration
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

		// Check if unacked and old enough
		if msg.AckedAt == nil && msg.CreatedAt.Before(cutoff) {
			result = append(result, msg)
		}
	}

	return result, nil
}

// GetPendingMessages retrieves messages that need to be delivered
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

		if limit > 0 && len(result) >= limit {
			break
		}
	}

	return result, nil
}

// IncrementRetry increments the retry count for a message
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

// DeleteMessage removes a message from the store
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

	// Remove from topic index
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

// Cleanup removes old acknowledged messages
func (s *MemoryMessageStore) Cleanup(ctx context.Context, olderThan time.Duration) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return 0, ErrStoreClosed
	}

	cutoff := time.Now().Add(-olderThan)
	count := 0

	for msgID, msg := range s.messages {
		// Remove acked messages older than cutoff
		if msg.AckedAt != nil && msg.AckedAt.Before(cutoff) {
			// Remove from topic index
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

		// Also remove expired messages
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

// Stats returns statistics about the message store
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

// cleanupLoop runs periodic cleanup
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

// Ensure MemoryMessageStore implements MessageStore
var _ MessageStore = (*MemoryMessageStore)(nil)
