package a2a

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

// Feature: agent-framework-2026-enhancements, Property 10: A2A Message Round-Trip
// **Validates: Requirements 6.1**
// For any valid A2AMessage, serializing to JSON and then deserializing should produce
// an equivalent message object with all field values preserved.

// genA2AMessageType generates a random valid A2AMessageType.
func genA2AMessageType() *rapid.Generator[A2AMessageType] {
	return rapid.SampledFrom([]A2AMessageType{
		A2AMessageTypeTask,
		A2AMessageTypeResult,
		A2AMessageTypeError,
		A2AMessageTypeStatus,
		A2AMessageTypeCancel,
	})
}

// genAgentID generates a valid agent identifier.
func genAgentID() *rapid.Generator[string] {
	return rapid.StringMatching(`[a-z][a-z0-9-]{2,30}`)
}

// genMessageID generates a valid message identifier (UUID-like).
func genMessageID() *rapid.Generator[string] {
	return rapid.StringMatching(`[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}`)
}

// genTimestamp generates a valid timestamp within a reasonable range.
func genTimestamp() *rapid.Generator[time.Time] {
	return rapid.Custom(func(t *rapid.T) time.Time {
		// Generate timestamps between 2020 and 2030
		year := rapid.IntRange(2020, 2030).Draw(t, "year")
		month := rapid.IntRange(1, 12).Draw(t, "month")
		day := rapid.IntRange(1, 28).Draw(t, "day")
		hour := rapid.IntRange(0, 23).Draw(t, "hour")
		minute := rapid.IntRange(0, 59).Draw(t, "minute")
		second := rapid.IntRange(0, 59).Draw(t, "second")
		return time.Date(year, time.Month(month), day, hour, minute, second, 0, time.UTC)
	})
}

// genSimplePayload generates simple JSON-serializable payloads.
func genSimplePayload() *rapid.Generator[any] {
	return rapid.Custom(func(t *rapid.T) any {
		payloadType := rapid.IntRange(0, 4).Draw(t, "payloadType")
		switch payloadType {
		case 0:
			// String payload
			return rapid.StringMatching(`[a-zA-Z0-9 ]{1,100}`).Draw(t, "stringPayload")
		case 1:
			// Number payload
			return rapid.Float64Range(-1e10, 1e10).Draw(t, "numberPayload")
		case 2:
			// Boolean payload
			return rapid.Bool().Draw(t, "boolPayload")
		case 3:
			// Map payload
			numKeys := rapid.IntRange(1, 5).Draw(t, "numKeys")
			m := make(map[string]any)
			for i := range numKeys {
				key := rapid.StringMatching(`[a-z][a-z_]{1,10}`).Draw(t, "mapKey")
				// Use simple values to avoid nested complexity
				valueType := rapid.IntRange(0, 2).Draw(t, "valueType")
				switch valueType {
				case 0:
					m[key] = rapid.StringMatching(`[a-zA-Z0-9]{1,20}`).Draw(t, "mapStringValue")
				case 1:
					m[key] = rapid.Float64Range(-1000, 1000).Draw(t, "mapNumberValue")
				case 2:
					m[key] = rapid.Bool().Draw(t, "mapBoolValue")
				}
				// Avoid duplicate keys by using index suffix
				if _, exists := m[key]; exists {
					m[key+"_"+string(rune('0'+i))] = m[key]
				}
			}
			return m
		case 4:
			// Array payload
			numItems := rapid.IntRange(1, 5).Draw(t, "numItems")
			arr := make([]any, numItems)
			for i := range numItems {
				arr[i] = rapid.StringMatching(`[a-zA-Z0-9]{1,20}`).Draw(t, "arrayItem")
			}
			return arr
		default:
			return nil
		}
	})
}

// genA2AMessage generates a valid A2AMessage.
func genA2AMessage() *rapid.Generator[*A2AMessage] {
	return rapid.Custom(func(t *rapid.T) *A2AMessage {
		hasReplyTo := rapid.Bool().Draw(t, "hasReplyTo")
		replyTo := ""
		if hasReplyTo {
			replyTo = genMessageID().Draw(t, "replyTo")
		}

		return &A2AMessage{
			ID:        genMessageID().Draw(t, "id"),
			Type:      genA2AMessageType().Draw(t, "type"),
			From:      genAgentID().Draw(t, "from"),
			To:        genAgentID().Draw(t, "to"),
			Payload:   genSimplePayload().Draw(t, "payload"),
			Timestamp: genTimestamp().Draw(t, "timestamp"),
			ReplyTo:   replyTo,
		}
	})
}

// TestProperty_A2AMessage_RoundTrip tests that A2A messages survive JSON round-trip.
func TestProperty_A2AMessage_RoundTrip(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Generate a random valid A2AMessage
		original := genA2AMessage().Draw(rt, "message")

		// Serialize to JSON
		data, err := json.Marshal(original)
		require.NoError(t, err, "Should serialize message to JSON")

		// Deserialize from JSON
		var deserialized A2AMessage
		err = json.Unmarshal(data, &deserialized)
		require.NoError(t, err, "Should deserialize message from JSON")

		// Property: ID should be preserved
		assert.Equal(t, original.ID, deserialized.ID, "ID should be preserved after round-trip")

		// Property: Type should be preserved
		assert.Equal(t, original.Type, deserialized.Type, "Type should be preserved after round-trip")

		// Property: From should be preserved
		assert.Equal(t, original.From, deserialized.From, "From should be preserved after round-trip")

		// Property: To should be preserved
		assert.Equal(t, original.To, deserialized.To, "To should be preserved after round-trip")

		// Property: Timestamp should be preserved (compare in UTC)
		assert.Equal(t, original.Timestamp.UTC(), deserialized.Timestamp.UTC(),
			"Timestamp should be preserved after round-trip")

		// Property: ReplyTo should be preserved
		assert.Equal(t, original.ReplyTo, deserialized.ReplyTo,
			"ReplyTo should be preserved after round-trip")

		// Property: Payload should be equivalent (JSON comparison)
		originalPayloadJSON, err := json.Marshal(original.Payload)
		require.NoError(t, err)
		deserializedPayloadJSON, err := json.Marshal(deserialized.Payload)
		require.NoError(t, err)
		assert.JSONEq(t, string(originalPayloadJSON), string(deserializedPayloadJSON),
			"Payload should be equivalent after round-trip")
	})
}

// TestProperty_A2AMessage_RoundTrip_ToJSON tests round-trip using ToJSON/FromJSON methods.
func TestProperty_A2AMessage_RoundTrip_ToJSON(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Generate a random valid A2AMessage
		original := genA2AMessage().Draw(rt, "message")

		// Serialize using ToJSON method
		data, err := original.ToJSON()
		require.NoError(t, err, "ToJSON should succeed")

		// Deserialize using FromJSON function
		deserialized, err := FromJSON(data)
		require.NoError(t, err, "FromJSON should succeed")

		// Property: All fields should be preserved
		assert.Equal(t, original.ID, deserialized.ID, "ID should be preserved")
		assert.Equal(t, original.Type, deserialized.Type, "Type should be preserved")
		assert.Equal(t, original.From, deserialized.From, "From should be preserved")
		assert.Equal(t, original.To, deserialized.To, "To should be preserved")
		assert.Equal(t, original.Timestamp.UTC(), deserialized.Timestamp.UTC(), "Timestamp should be preserved")
		assert.Equal(t, original.ReplyTo, deserialized.ReplyTo, "ReplyTo should be preserved")

		// Property: Deserialized message should be valid
		err = deserialized.Validate()
		assert.NoError(t, err, "Deserialized message should be valid")
	})
}

// TestProperty_A2AMessage_RoundTrip_AllMessageTypes tests round-trip for each message type.
func TestProperty_A2AMessage_RoundTrip_AllMessageTypes(t *testing.T) {
	messageTypes := []A2AMessageType{
		A2AMessageTypeTask,
		A2AMessageTypeResult,
		A2AMessageTypeError,
		A2AMessageTypeStatus,
		A2AMessageTypeCancel,
	}

	for _, msgType := range messageTypes {
		t.Run(string(msgType), func(t *testing.T) {
			rapid.Check(t, func(rt *rapid.T) {
				// Generate message with specific type
				msg := &A2AMessage{
					ID:        genMessageID().Draw(rt, "id"),
					Type:      msgType,
					From:      genAgentID().Draw(rt, "from"),
					To:        genAgentID().Draw(rt, "to"),
					Payload:   genSimplePayload().Draw(rt, "payload"),
					Timestamp: genTimestamp().Draw(rt, "timestamp"),
				}

				// Round-trip
				data, err := json.Marshal(msg)
				require.NoError(t, err)

				var deserialized A2AMessage
				err = json.Unmarshal(data, &deserialized)
				require.NoError(t, err)

				// Property: Type should be preserved exactly
				assert.Equal(t, msgType, deserialized.Type,
					"Message type %s should be preserved", msgType)
			})
		})
	}
}

// TestProperty_A2AMessage_RoundTrip_WithVariousPayloads tests round-trip with different payload types.
func TestProperty_A2AMessage_RoundTrip_WithVariousPayloads(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		baseMsg := &A2AMessage{
			ID:        genMessageID().Draw(rt, "id"),
			Type:      A2AMessageTypeTask,
			From:      genAgentID().Draw(rt, "from"),
			To:        genAgentID().Draw(rt, "to"),
			Timestamp: genTimestamp().Draw(rt, "timestamp"),
		}

		// Test with nil payload
		t.Run("nil payload", func(t *testing.T) {
			msg := *baseMsg
			msg.Payload = nil

			data, err := json.Marshal(&msg)
			require.NoError(t, err)

			var deserialized A2AMessage
			err = json.Unmarshal(data, &deserialized)
			require.NoError(t, err)

			assert.Nil(t, deserialized.Payload, "Nil payload should be preserved")
		})

		// Test with string payload
		t.Run("string payload", func(t *testing.T) {
			msg := *baseMsg
			msg.Payload = rapid.StringMatching(`[a-zA-Z0-9 ]{1,50}`).Draw(rt, "stringPayload")

			data, err := json.Marshal(&msg)
			require.NoError(t, err)

			var deserialized A2AMessage
			err = json.Unmarshal(data, &deserialized)
			require.NoError(t, err)

			assert.Equal(t, msg.Payload, deserialized.Payload, "String payload should be preserved")
		})

		// Test with map payload
		t.Run("map payload", func(t *testing.T) {
			msg := *baseMsg
			msg.Payload = map[string]any{
				"key1": rapid.StringMatching(`[a-z]{1,10}`).Draw(rt, "value1"),
				"key2": rapid.Float64Range(0, 100).Draw(rt, "value2"),
			}

			data, err := json.Marshal(&msg)
			require.NoError(t, err)

			var deserialized A2AMessage
			err = json.Unmarshal(data, &deserialized)
			require.NoError(t, err)

			originalJSON, _ := json.Marshal(msg.Payload)
			deserializedJSON, _ := json.Marshal(deserialized.Payload)
			assert.JSONEq(t, string(originalJSON), string(deserializedJSON),
				"Map payload should be equivalent")
		})
	})
}

// TestProperty_A2AMessage_RoundTrip_PreservesValidation tests that valid messages remain valid after round-trip.
func TestProperty_A2AMessage_RoundTrip_PreservesValidation(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Generate a valid message
		original := genA2AMessage().Draw(rt, "message")

		// Verify original is valid
		err := original.Validate()
		require.NoError(t, err, "Original message should be valid")

		// Round-trip
		data, err := json.Marshal(original)
		require.NoError(t, err)

		var deserialized A2AMessage
		err = json.Unmarshal(data, &deserialized)
		require.NoError(t, err)

		// Property: Deserialized message should also be valid
		err = deserialized.Validate()
		assert.NoError(t, err, "Deserialized message should remain valid after round-trip")
	})
}

// TestProperty_A2AMessage_RoundTrip_ReplyToOptional tests that optional ReplyTo field is handled correctly.
func TestProperty_A2AMessage_RoundTrip_ReplyToOptional(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Test with ReplyTo
		t.Run("with ReplyTo", func(t *testing.T) {
			msg := &A2AMessage{
				ID:        genMessageID().Draw(rt, "id"),
				Type:      A2AMessageTypeResult,
				From:      genAgentID().Draw(rt, "from"),
				To:        genAgentID().Draw(rt, "to"),
				Payload:   "result",
				Timestamp: genTimestamp().Draw(rt, "timestamp"),
				ReplyTo:   genMessageID().Draw(rt, "replyTo"),
			}

			data, err := json.Marshal(msg)
			require.NoError(t, err)

			var deserialized A2AMessage
			err = json.Unmarshal(data, &deserialized)
			require.NoError(t, err)

			assert.Equal(t, msg.ReplyTo, deserialized.ReplyTo, "ReplyTo should be preserved")
			assert.True(t, deserialized.IsReply(), "Should be identified as reply")
		})

		// Test without ReplyTo
		t.Run("without ReplyTo", func(t *testing.T) {
			msg := &A2AMessage{
				ID:        genMessageID().Draw(rt, "id2"),
				Type:      A2AMessageTypeTask,
				From:      genAgentID().Draw(rt, "from2"),
				To:        genAgentID().Draw(rt, "to2"),
				Payload:   "task",
				Timestamp: genTimestamp().Draw(rt, "timestamp2"),
				ReplyTo:   "", // Empty
			}

			data, err := json.Marshal(msg)
			require.NoError(t, err)

			var deserialized A2AMessage
			err = json.Unmarshal(data, &deserialized)
			require.NoError(t, err)

			assert.Empty(t, deserialized.ReplyTo, "Empty ReplyTo should be preserved")
			assert.False(t, deserialized.IsReply(), "Should not be identified as reply")
		})
	})
}

// TestProperty_A2AMessage_RoundTrip_TimestampPrecision tests that timestamp precision is preserved.
func TestProperty_A2AMessage_RoundTrip_TimestampPrecision(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Generate timestamp with second precision (JSON typically uses RFC3339)
		ts := genTimestamp().Draw(rt, "timestamp")

		msg := &A2AMessage{
			ID:        genMessageID().Draw(rt, "id"),
			Type:      A2AMessageTypeTask,
			From:      genAgentID().Draw(rt, "from"),
			To:        genAgentID().Draw(rt, "to"),
			Payload:   nil,
			Timestamp: ts,
		}

		data, err := json.Marshal(msg)
		require.NoError(t, err)

		var deserialized A2AMessage
		err = json.Unmarshal(data, &deserialized)
		require.NoError(t, err)

		// Property: Timestamps should be equal when compared in UTC
		assert.True(t, msg.Timestamp.UTC().Equal(deserialized.Timestamp.UTC()),
			"Timestamp should be preserved with correct precision")
	})
}
