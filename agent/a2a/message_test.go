package a2a

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestA2AMessageType_IsValid(t *testing.T) {
	tests := []struct {
		name     string
		msgType  A2AMessageType
		expected bool
	}{
		{"task type", A2AMessageTypeTask, true},
		{"result type", A2AMessageTypeResult, true},
		{"error type", A2AMessageTypeError, true},
		{"status type", A2AMessageTypeStatus, true},
		{"cancel type", A2AMessageTypeCancel, true},
		{"invalid type", A2AMessageType("invalid"), false},
		{"empty type", A2AMessageType(""), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.msgType.IsValid())
		})
	}
}

func TestA2AMessageType_String(t *testing.T) {
	assert.Equal(t, "task", A2AMessageTypeTask.String())
	assert.Equal(t, "result", A2AMessageTypeResult.String())
	assert.Equal(t, "error", A2AMessageTypeError.String())
	assert.Equal(t, "status", A2AMessageTypeStatus.String())
	assert.Equal(t, "cancel", A2AMessageTypeCancel.String())
}

func TestNewA2AMessage(t *testing.T) {
	payload := map[string]string{"key": "value"}
	msg := NewA2AMessage(A2AMessageTypeTask, "agent-a", "agent-b", payload)

	assert.NotEmpty(t, msg.ID)
	assert.Equal(t, A2AMessageTypeTask, msg.Type)
	assert.Equal(t, "agent-a", msg.From)
	assert.Equal(t, "agent-b", msg.To)
	assert.Equal(t, payload, msg.Payload)
	assert.False(t, msg.Timestamp.IsZero())
	assert.Empty(t, msg.ReplyTo)
}

func TestNewTaskMessage(t *testing.T) {
	msg := NewTaskMessage("sender", "receiver", "task payload")

	assert.Equal(t, A2AMessageTypeTask, msg.Type)
	assert.Equal(t, "sender", msg.From)
	assert.Equal(t, "receiver", msg.To)
	assert.Equal(t, "task payload", msg.Payload)
}

func TestNewResultMessage(t *testing.T) {
	msg := NewResultMessage("sender", "receiver", "result payload", "original-msg-id")

	assert.Equal(t, A2AMessageTypeResult, msg.Type)
	assert.Equal(t, "sender", msg.From)
	assert.Equal(t, "receiver", msg.To)
	assert.Equal(t, "result payload", msg.Payload)
	assert.Equal(t, "original-msg-id", msg.ReplyTo)
}

func TestNewErrorMessage(t *testing.T) {
	msg := NewErrorMessage("sender", "receiver", "error payload", "original-msg-id")

	assert.Equal(t, A2AMessageTypeError, msg.Type)
	assert.Equal(t, "original-msg-id", msg.ReplyTo)
}

func TestNewStatusMessage(t *testing.T) {
	msg := NewStatusMessage("sender", "receiver", "status payload", "original-msg-id")

	assert.Equal(t, A2AMessageTypeStatus, msg.Type)
	assert.Equal(t, "original-msg-id", msg.ReplyTo)
}

func TestNewCancelMessage(t *testing.T) {
	msg := NewCancelMessage("sender", "receiver", "task-123")

	assert.Equal(t, A2AMessageTypeCancel, msg.Type)
	payload, ok := msg.Payload.(map[string]string)
	require.True(t, ok)
	assert.Equal(t, "task-123", payload["task_id"])
}

func TestA2AMessage_Validate(t *testing.T) {
	tests := []struct {
		name        string
		msg         *A2AMessage
		expectedErr error
	}{
		{
			name: "valid message",
			msg: &A2AMessage{
				ID:        "msg-123",
				Type:      A2AMessageTypeTask,
				From:      "agent-a",
				To:        "agent-b",
				Timestamp: time.Now(),
			},
			expectedErr: nil,
		},
		{
			name: "missing ID",
			msg: &A2AMessage{
				Type:      A2AMessageTypeTask,
				From:      "agent-a",
				To:        "agent-b",
				Timestamp: time.Now(),
			},
			expectedErr: ErrMessageMissingID,
		},
		{
			name: "invalid type",
			msg: &A2AMessage{
				ID:        "msg-123",
				Type:      A2AMessageType("invalid"),
				From:      "agent-a",
				To:        "agent-b",
				Timestamp: time.Now(),
			},
			expectedErr: ErrMessageInvalidType,
		},
		{
			name: "missing from",
			msg: &A2AMessage{
				ID:        "msg-123",
				Type:      A2AMessageTypeTask,
				To:        "agent-b",
				Timestamp: time.Now(),
			},
			expectedErr: ErrMessageMissingFrom,
		},
		{
			name: "missing to",
			msg: &A2AMessage{
				ID:        "msg-123",
				Type:      A2AMessageTypeTask,
				From:      "agent-a",
				Timestamp: time.Now(),
			},
			expectedErr: ErrMessageMissingTo,
		},
		{
			name: "missing timestamp",
			msg: &A2AMessage{
				ID:   "msg-123",
				Type: A2AMessageTypeTask,
				From: "agent-a",
				To:   "agent-b",
			},
			expectedErr: ErrMessageMissingTimestamp,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.msg.Validate()
			if tt.expectedErr != nil {
				assert.ErrorIs(t, err, tt.expectedErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestA2AMessage_TypeChecks(t *testing.T) {
	tests := []struct {
		name     string
		msgType  A2AMessageType
		isTask   bool
		isResult bool
		isError  bool
		isStatus bool
		isCancel bool
	}{
		{"task", A2AMessageTypeTask, true, false, false, false, false},
		{"result", A2AMessageTypeResult, false, true, false, false, false},
		{"error", A2AMessageTypeError, false, false, true, false, false},
		{"status", A2AMessageTypeStatus, false, false, false, true, false},
		{"cancel", A2AMessageTypeCancel, false, false, false, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := NewA2AMessage(tt.msgType, "a", "b", nil)
			assert.Equal(t, tt.isTask, msg.IsTask())
			assert.Equal(t, tt.isResult, msg.IsResult())
			assert.Equal(t, tt.isError, msg.IsError())
			assert.Equal(t, tt.isStatus, msg.IsStatus())
			assert.Equal(t, tt.isCancel, msg.IsCancel())
		})
	}
}

func TestA2AMessage_IsReply(t *testing.T) {
	msg := NewTaskMessage("a", "b", nil)
	assert.False(t, msg.IsReply())

	replyMsg := NewResultMessage("b", "a", nil, msg.ID)
	assert.True(t, replyMsg.IsReply())
}

func TestA2AMessage_CreateReply(t *testing.T) {
	original := NewTaskMessage("agent-a", "agent-b", "task")
	reply := original.CreateReply(A2AMessageTypeResult, "result")

	assert.NotEqual(t, original.ID, reply.ID)
	assert.Equal(t, A2AMessageTypeResult, reply.Type)
	assert.Equal(t, "agent-b", reply.From)
	assert.Equal(t, "agent-a", reply.To)
	assert.Equal(t, "result", reply.Payload)
	assert.Equal(t, original.ID, reply.ReplyTo)
}

func TestA2AMessage_JSONSerialization(t *testing.T) {
	original := &A2AMessage{
		ID:        "msg-123",
		Type:      A2AMessageTypeTask,
		From:      "agent-a",
		To:        "agent-b",
		Payload:   map[string]any{"key": "value", "number": float64(42)},
		Timestamp: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		ReplyTo:   "prev-msg",
	}

	// Serialize
	data, err := original.ToJSON()
	require.NoError(t, err)

	// Deserialize
	parsed, err := FromJSON(data)
	require.NoError(t, err)

	// Verify
	assert.Equal(t, original.ID, parsed.ID)
	assert.Equal(t, original.Type, parsed.Type)
	assert.Equal(t, original.From, parsed.From)
	assert.Equal(t, original.To, parsed.To)
	assert.Equal(t, original.ReplyTo, parsed.ReplyTo)
	assert.Equal(t, original.Timestamp.UTC(), parsed.Timestamp.UTC())

	// Payload comparison (JSON unmarshals to map[string]any)
	payloadMap, ok := parsed.Payload.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "value", payloadMap["key"])
	assert.Equal(t, float64(42), payloadMap["number"])
}

func TestA2AMessage_MarshalUnmarshal(t *testing.T) {
	original := NewTaskMessage("sender", "receiver", map[string]string{"task": "do something"})

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var parsed A2AMessage
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)

	assert.Equal(t, original.ID, parsed.ID)
	assert.Equal(t, original.Type, parsed.Type)
	assert.Equal(t, original.From, parsed.From)
	assert.Equal(t, original.To, parsed.To)
}

func TestA2AMessage_Clone(t *testing.T) {
	original := &A2AMessage{
		ID:        "msg-123",
		Type:      A2AMessageTypeTask,
		From:      "agent-a",
		To:        "agent-b",
		Payload:   map[string]any{"key": "value"},
		Timestamp: time.Now().UTC(),
		ReplyTo:   "prev-msg",
	}

	clone := original.Clone()

	// Verify values are equal
	assert.Equal(t, original.ID, clone.ID)
	assert.Equal(t, original.Type, clone.Type)
	assert.Equal(t, original.From, clone.From)
	assert.Equal(t, original.To, clone.To)
	assert.Equal(t, original.ReplyTo, clone.ReplyTo)
	assert.Equal(t, original.Timestamp, clone.Timestamp)

	// Verify it's a deep copy (modifying clone doesn't affect original)
	if payloadMap, ok := clone.Payload.(map[string]any); ok {
		payloadMap["key"] = "modified"
	}
	originalPayload := original.Payload.(map[string]any)
	assert.Equal(t, "value", originalPayload["key"])
}

func TestParseA2AMessage(t *testing.T) {
	t.Run("valid message", func(t *testing.T) {
		validJSON := `{
			"id": "msg-123",
			"type": "task",
			"from": "agent-a",
			"to": "agent-b",
			"payload": {"key": "value"},
			"timestamp": "2024-01-15T10:30:00Z"
		}`

		msg, err := ParseA2AMessage([]byte(validJSON))
		require.NoError(t, err)
		assert.Equal(t, "msg-123", msg.ID)
		assert.Equal(t, A2AMessageTypeTask, msg.Type)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		invalidJSON := `{invalid json}`
		_, err := ParseA2AMessage([]byte(invalidJSON))
		assert.ErrorIs(t, err, ErrInvalidMessage)
	})

	t.Run("missing required field", func(t *testing.T) {
		missingID := `{
			"type": "task",
			"from": "agent-a",
			"to": "agent-b",
			"timestamp": "2024-01-15T10:30:00Z"
		}`
		_, err := ParseA2AMessage([]byte(missingID))
		assert.ErrorIs(t, err, ErrMessageMissingID)
	})
}

func TestFromJSON(t *testing.T) {
	t.Run("valid JSON", func(t *testing.T) {
		validJSON := `{
			"id": "msg-123",
			"type": "result",
			"from": "agent-b",
			"to": "agent-a",
			"payload": "success",
			"timestamp": "2024-01-15T10:30:00Z",
			"reply_to": "original-msg"
		}`

		msg, err := FromJSON([]byte(validJSON))
		require.NoError(t, err)
		assert.Equal(t, "msg-123", msg.ID)
		assert.Equal(t, A2AMessageTypeResult, msg.Type)
		assert.Equal(t, "original-msg", msg.ReplyTo)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		_, err := FromJSON([]byte(`not json`))
		assert.Error(t, err)
	})
}
