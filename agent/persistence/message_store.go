package persistence

import (
	"context"
	"encoding/json"
	"time"
)

// MessageStore defines the interface for message persistence.
// It provides reliable message delivery with acknowledgment and retry support.
type MessageStore interface {
	Store

	// SaveMessage persists a single message to the store
	SaveMessage(ctx context.Context, msg *Message) error

	// SaveMessages persists multiple messages atomically
	SaveMessages(ctx context.Context, msgs []*Message) error

	// GetMessage retrieves a message by ID
	GetMessage(ctx context.Context, msgID string) (*Message, error)

	// GetMessages retrieves messages for a topic with pagination
	// Returns messages, next cursor, and error
	GetMessages(ctx context.Context, topic string, cursor string, limit int) ([]*Message, string, error)

	// AckMessage marks a message as acknowledged/processed
	AckMessage(ctx context.Context, msgID string) error

	// GetUnackedMessages retrieves unacknowledged messages older than the specified duration
	// These messages are candidates for retry
	GetUnackedMessages(ctx context.Context, topic string, olderThan time.Duration) ([]*Message, error)

	// GetPendingMessages retrieves messages that need to be delivered
	// This includes new messages and messages that need retry
	GetPendingMessages(ctx context.Context, topic string, limit int) ([]*Message, error)

	// IncrementRetry increments the retry count for a message
	IncrementRetry(ctx context.Context, msgID string) error

	// DeleteMessage removes a message from the store
	DeleteMessage(ctx context.Context, msgID string) error

	// Cleanup removes old acknowledged messages
	Cleanup(ctx context.Context, olderThan time.Duration) (int, error)

	// Stats returns statistics about the message store
	Stats(ctx context.Context) (*MessageStoreStats, error)
}

// Message represents a persistent message in the system
type Message struct {
	// ID is the unique identifier for the message
	ID string `json:"id"`

	// Topic is the message topic/channel
	Topic string `json:"topic"`

	// FromID is the sender agent ID
	FromID string `json:"from_id"`

	// ToID is the recipient agent ID (empty for broadcast)
	ToID string `json:"to_id,omitempty"`

	// Type is the message type (proposal, response, vote, etc.)
	Type string `json:"type"`

	// Content is the message content
	Content string `json:"content"`

	// Payload contains additional structured data
	Payload map[string]interface{} `json:"payload,omitempty"`

	// Metadata contains message metadata
	Metadata map[string]string `json:"metadata,omitempty"`

	// CreatedAt is when the message was created
	CreatedAt time.Time `json:"created_at"`

	// AckedAt is when the message was acknowledged (nil if not acked)
	AckedAt *time.Time `json:"acked_at,omitempty"`

	// RetryCount is the number of delivery attempts
	RetryCount int `json:"retry_count"`

	// LastRetryAt is when the last retry was attempted
	LastRetryAt *time.Time `json:"last_retry_at,omitempty"`

	// ExpiresAt is when the message expires (optional)
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// MarshalJSON implements json.Marshaler
func (m *Message) MarshalJSON() ([]byte, error) {
	type Alias Message
	return json.Marshal(&struct {
		*Alias
	}{
		Alias: (*Alias)(m),
	})
}

// UnmarshalJSON implements json.Unmarshaler
func (m *Message) UnmarshalJSON(data []byte) error {
	type Alias Message
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(m),
	}
	return json.Unmarshal(data, aux)
}

// IsExpired checks if the message has expired
func (m *Message) IsExpired() bool {
	if m.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*m.ExpiresAt)
}

// IsAcked checks if the message has been acknowledged
func (m *Message) IsAcked() bool {
	return m.AckedAt != nil
}

// ShouldRetry checks if the message should be retried based on the retry config
func (m *Message) ShouldRetry(config RetryConfig) bool {
	if m.IsAcked() || m.IsExpired() {
		return false
	}
	return m.RetryCount < config.MaxRetries
}

// NextRetryTime calculates when the next retry should occur
func (m *Message) NextRetryTime(config RetryConfig) time.Time {
	backoff := config.CalculateBackoff(m.RetryCount)
	if m.LastRetryAt != nil {
		return m.LastRetryAt.Add(backoff)
	}
	return m.CreatedAt.Add(backoff)
}

// MessageStoreStats contains statistics about the message store
type MessageStoreStats struct {
	// TotalMessages is the total number of messages in the store
	TotalMessages int64 `json:"total_messages"`

	// PendingMessages is the number of unacknowledged messages
	PendingMessages int64 `json:"pending_messages"`

	// AckedMessages is the number of acknowledged messages
	AckedMessages int64 `json:"acked_messages"`

	// ExpiredMessages is the number of expired messages
	ExpiredMessages int64 `json:"expired_messages"`

	// TopicCounts is the message count per topic
	TopicCounts map[string]int64 `json:"topic_counts"`

	// OldestPendingAge is the age of the oldest pending message
	OldestPendingAge time.Duration `json:"oldest_pending_age"`
}

// MessageFilter defines criteria for filtering messages
type MessageFilter struct {
	// Topic filters by topic
	Topic string `json:"topic,omitempty"`

	// FromID filters by sender
	FromID string `json:"from_id,omitempty"`

	// ToID filters by recipient
	ToID string `json:"to_id,omitempty"`

	// Type filters by message type
	Type string `json:"type,omitempty"`

	// Status filters by acknowledgment status
	Status MessageStatus `json:"status,omitempty"`

	// CreatedAfter filters messages created after this time
	CreatedAfter *time.Time `json:"created_after,omitempty"`

	// CreatedBefore filters messages created before this time
	CreatedBefore *time.Time `json:"created_before,omitempty"`

	// Limit is the maximum number of messages to return
	Limit int `json:"limit,omitempty"`

	// Offset is the number of messages to skip
	Offset int `json:"offset,omitempty"`
}

// MessageStatus represents the status of a message
type MessageStatus string

const (
	// MessageStatusPending indicates the message is waiting to be processed
	MessageStatusPending MessageStatus = "pending"

	// MessageStatusAcked indicates the message has been acknowledged
	MessageStatusAcked MessageStatus = "acked"

	// MessageStatusExpired indicates the message has expired
	MessageStatusExpired MessageStatus = "expired"

	// MessageStatusFailed indicates the message failed after max retries
	MessageStatusFailed MessageStatus = "failed"
)
