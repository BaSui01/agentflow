package a2a

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// A2AMessageType represents the type of A2A message.
type A2AMessageType string

const (
	// A2AMessageTypeTask indicates a task request message.
	A2AMessageTypeTask A2AMessageType = "task"
	// A2AMessageTypeResult indicates a task result message.
	A2AMessageTypeResult A2AMessageType = "result"
	// A2AMessageTypeError indicates an error message.
	A2AMessageTypeError A2AMessageType = "error"
	// A2AMessageTypeStatus indicates a status update message.
	A2AMessageTypeStatus A2AMessageType = "status"
	// A2AMessageTypeCancel indicates a task cancellation message.
	A2AMessageTypeCancel A2AMessageType = "cancel"
)

// IsValid checks if the message type is a valid A2A message type.
func (t A2AMessageType) IsValid() bool {
	switch t {
	case A2AMessageTypeTask, A2AMessageTypeResult, A2AMessageTypeError,
		A2AMessageTypeStatus, A2AMessageTypeCancel:
		return true
	default:
		return false
	}
}

// String returns the string representation of the message type.
func (t A2AMessageType) String() string {
	return string(t)
}

// A2AMessage represents an A2A standard message for agent-to-agent communication.
type A2AMessage struct {
	// ID is the unique identifier for this message.
	ID string `json:"id"`
	// Type indicates the message type (task, result, error, status, cancel).
	Type A2AMessageType `json:"type"`
	// From is the identifier of the sending agent.
	From string `json:"from"`
	// To is the identifier of the receiving agent.
	To string `json:"to"`
	// Payload contains the message data.
	Payload any `json:"payload"`
	// Timestamp is when the message was created.
	Timestamp time.Time `json:"timestamp"`
	// ReplyTo is the ID of the message this is replying to (optional).
	ReplyTo string `json:"reply_to,omitempty"`
}

// NewA2AMessage creates a new A2AMessage with a generated ID and current timestamp.
func NewA2AMessage(msgType A2AMessageType, from, to string, payload any) *A2AMessage {
	return &A2AMessage{
		ID:        uuid.New().String(),
		Type:      msgType,
		From:      from,
		To:        to,
		Payload:   payload,
		Timestamp: time.Now().UTC(),
	}
}

// NewTaskMessage creates a new task request message.
func NewTaskMessage(from, to string, payload any) *A2AMessage {
	return NewA2AMessage(A2AMessageTypeTask, from, to, payload)
}

// NewResultMessage creates a new result message in reply to a task.
func NewResultMessage(from, to string, payload any, replyTo string) *A2AMessage {
	msg := NewA2AMessage(A2AMessageTypeResult, from, to, payload)
	msg.ReplyTo = replyTo
	return msg
}

// NewErrorMessage creates a new error message in reply to a task.
func NewErrorMessage(from, to string, payload any, replyTo string) *A2AMessage {
	msg := NewA2AMessage(A2AMessageTypeError, from, to, payload)
	msg.ReplyTo = replyTo
	return msg
}

// NewStatusMessage creates a new status update message.
func NewStatusMessage(from, to string, payload any, replyTo string) *A2AMessage {
	msg := NewA2AMessage(A2AMessageTypeStatus, from, to, payload)
	msg.ReplyTo = replyTo
	return msg
}

// NewCancelMessage creates a new cancellation message.
func NewCancelMessage(from, to string, taskID string) *A2AMessage {
	return NewA2AMessage(A2AMessageTypeCancel, from, to, map[string]string{"task_id": taskID})
}

// Validate checks if the A2AMessage has all required fields and valid values.
func (m *A2AMessage) Validate() error {
	if m.ID == "" {
		return ErrMessageMissingID
	}
	if !m.Type.IsValid() {
		return ErrMessageInvalidType
	}
	if m.From == "" {
		return ErrMessageMissingFrom
	}
	if m.To == "" {
		return ErrMessageMissingTo
	}
	if m.Timestamp.IsZero() {
		return ErrMessageMissingTimestamp
	}
	return nil
}

// IsReply checks if this message is a reply to another message.
func (m *A2AMessage) IsReply() bool {
	return m.ReplyTo != ""
}

// IsTask checks if this is a task request message.
func (m *A2AMessage) IsTask() bool {
	return m.Type == A2AMessageTypeTask
}

// IsResult checks if this is a result message.
func (m *A2AMessage) IsResult() bool {
	return m.Type == A2AMessageTypeResult
}

// IsError checks if this is an error message.
func (m *A2AMessage) IsError() bool {
	return m.Type == A2AMessageTypeError
}

// IsStatus checks if this is a status message.
func (m *A2AMessage) IsStatus() bool {
	return m.Type == A2AMessageTypeStatus
}

// IsCancel checks if this is a cancellation message.
func (m *A2AMessage) IsCancel() bool {
	return m.Type == A2AMessageTypeCancel
}

// MarshalJSON implements json.Marshaler for A2AMessage.
func (m *A2AMessage) MarshalJSON() ([]byte, error) {
	type Alias A2AMessage
	return json.Marshal(&struct {
		*Alias
	}{
		Alias: (*Alias)(m),
	})
}

// UnmarshalJSON implements json.Unmarshaler for A2AMessage.
func (m *A2AMessage) UnmarshalJSON(data []byte) error {
	type Alias A2AMessage
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(m),
	}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	return nil
}

// Clone creates a deep copy of the message.
func (m *A2AMessage) Clone() *A2AMessage {
	clone := &A2AMessage{
		ID:        m.ID,
		Type:      m.Type,
		From:      m.From,
		To:        m.To,
		Timestamp: m.Timestamp,
		ReplyTo:   m.ReplyTo,
	}

	// Deep copy payload if it's a map or slice
	if m.Payload != nil {
		data, err := json.Marshal(m.Payload)
		if err == nil {
			var payload any
			if err := json.Unmarshal(data, &payload); err == nil {
				clone.Payload = payload
			} else {
				clone.Payload = m.Payload
			}
		} else {
			clone.Payload = m.Payload
		}
	}

	return clone
}

// CreateReply creates a reply message to this message.
func (m *A2AMessage) CreateReply(msgType A2AMessageType, payload any) *A2AMessage {
	return &A2AMessage{
		ID:        uuid.New().String(),
		Type:      msgType,
		From:      m.To,
		To:        m.From,
		Payload:   payload,
		Timestamp: time.Now().UTC(),
		ReplyTo:   m.ID,
	}
}

// ToJSON serializes the message to JSON bytes.
func (m *A2AMessage) ToJSON() ([]byte, error) {
	return json.Marshal(m)
}

// FromJSON deserializes a message from JSON bytes.
func FromJSON(data []byte) (*A2AMessage, error) {
	var msg A2AMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// ParseA2AMessage parses JSON data into an A2AMessage and validates it.
func ParseA2AMessage(data []byte) (*A2AMessage, error) {
	msg, err := FromJSON(data)
	if err != nil {
		return nil, ErrInvalidMessage
	}
	if err := msg.Validate(); err != nil {
		return nil, err
	}
	return msg, nil
}
