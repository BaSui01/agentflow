package a2a

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

// 特性:代理-框架-2026-增强,财产 10: A2A 信件回合-特里普
// ** 参数:要求6.1**
// 对于任何有效的A2AMessage,序列化到JSON,然后去序列化应产生
// 一个保留了所有字段值的等效消息对象。

// genA2AMessageType生成一个随机有效的A2AMessageType.
func genA2AMessageType() *rapid.Generator[A2AMessageType] {
	return rapid.SampledFrom([]A2AMessageType{
		A2AMessageTypeTask,
		A2AMessageTypeResult,
		A2AMessageTypeError,
		A2AMessageTypeStatus,
		A2AMessageTypeCancel,
	})
}

// genAgentiID生成有效的代理标识符。
func genAgentID() *rapid.Generator[string] {
	return rapid.StringMatching(`[a-z][a-z0-9-]{2,30}`)
}

// genMessageID生成一个有效的消息标识符(UUID-like).
func genMessageID() *rapid.Generator[string] {
	return rapid.StringMatching(`[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}`)
}

// genTimestamp在合理范围内生成一个有效的时间戳.
func genTimestamp() *rapid.Generator[time.Time] {
	return rapid.Custom(func(t *rapid.T) time.Time {
		// 生成2020年至2030年的时间戳
		year := rapid.IntRange(2020, 2030).Draw(t, "year")
		month := rapid.IntRange(1, 12).Draw(t, "month")
		day := rapid.IntRange(1, 28).Draw(t, "day")
		hour := rapid.IntRange(0, 23).Draw(t, "hour")
		minute := rapid.IntRange(0, 59).Draw(t, "minute")
		second := rapid.IntRange(0, 59).Draw(t, "second")
		return time.Date(year, time.Month(month), day, hour, minute, second, 0, time.UTC)
	})
}

// genSimplePayload 生成简单的JSON可连载荷.
func genSimplePayload() *rapid.Generator[any] {
	return rapid.Custom(func(t *rapid.T) any {
		payloadType := rapid.IntRange(0, 4).Draw(t, "payloadType")
		switch payloadType {
		case 0:
			// 字符串有效载荷
			return rapid.StringMatching(`[a-zA-Z0-9 ]{1,100}`).Draw(t, "stringPayload")
		case 1:
			// 载荷数
			return rapid.Float64Range(-1e10, 1e10).Draw(t, "numberPayload")
		case 2:
			// 布尔有效载荷
			return rapid.Bool().Draw(t, "boolPayload")
		case 3:
			// 地图有效载荷
			numKeys := rapid.IntRange(1, 5).Draw(t, "numKeys")
			m := make(map[string]any)
			for i := range numKeys {
				key := rapid.StringMatching(`[a-z][a-z_]{1,10}`).Draw(t, "mapKey")
				// 使用简单的值来避免嵌入式复杂
				valueType := rapid.IntRange(0, 2).Draw(t, "valueType")
				switch valueType {
				case 0:
					m[key] = rapid.StringMatching(`[a-zA-Z0-9]{1,20}`).Draw(t, "mapStringValue")
				case 1:
					m[key] = rapid.Float64Range(-1000, 1000).Draw(t, "mapNumberValue")
				case 2:
					m[key] = rapid.Bool().Draw(t, "mapBoolValue")
				}
				// 使用索引后缀来避免重复密钥
				if _, exists := m[key]; exists {
					m[key+"_"+string(rune('0'+i))] = m[key]
				}
			}
			return m
		case 4:
			// 阵列有效载荷
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

// genA2AMessage生成有效的A2AMessage.
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

// TestProperty A2AMessage RundTrip测试A2A消息能活到JSON来回.
func TestProperty_A2AMessage_RoundTrip(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// 生成一个随机有效的 A2AMessage
		original := genA2AMessage().Draw(rt, "message")

		// 序列化为 JSON
		data, err := json.Marshal(original)
		require.NoError(t, err, "Should serialize message to JSON")

		// 从 JSON 切入
		var deserialized A2AMessage
		err = json.Unmarshal(data, &deserialized)
		require.NoError(t, err, "Should deserialize message from JSON")

		// 财产:ID应保留
		assert.Equal(t, original.ID, deserialized.ID, "ID should be preserved after round-trip")

		// 财产:应保留类型
		assert.Equal(t, original.Type, deserialized.Type, "Type should be preserved after round-trip")

		// 财产:应当保留
		assert.Equal(t, original.From, deserialized.From, "From should be preserved after round-trip")

		// 财产:应予保留
		assert.Equal(t, original.To, deserialized.To, "To should be preserved after round-trip")

		// 属性: 时间戳应当保留(在协调世界时进行比较)
		assert.Equal(t, original.Timestamp.UTC(), deserialized.Timestamp.UTC(),
			"Timestamp should be preserved after round-trip")

		// 属性: 应保存
		assert.Equal(t, original.ReplyTo, deserialized.ReplyTo,
			"ReplyTo should be preserved after round-trip")

		// 财产:有效载荷应等同(JSON比较)
		originalPayloadJSON, err := json.Marshal(original.Payload)
		require.NoError(t, err)
		deserializedPayloadJSON, err := json.Marshal(deserialized.Payload)
		require.NoError(t, err)
		assert.JSONEq(t, string(originalPayloadJSON), string(deserializedPayloadJSON),
			"Payload should be equivalent after round-trip")
	})
}

// TestProperty A2AMessage RundTrip ToJSON 使用ToJSON/FromJSON方法进行往返测试.
func TestProperty_A2AMessage_RoundTrip_ToJSON(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// 生成一个随机有效的 A2AMessage
		original := genA2AMessage().Draw(rt, "message")

		// 使用 ToJSON 方法进行序列化
		data, err := original.ToJSON()
		require.NoError(t, err, "ToJSON should succeed")

		// 使用 FromJSON 函数去序列化
		deserialized, err := FromJSON(data)
		require.NoError(t, err, "FromJSON should succeed")

		// 财产:应保留所有田地
		assert.Equal(t, original.ID, deserialized.ID, "ID should be preserved")
		assert.Equal(t, original.Type, deserialized.Type, "Type should be preserved")
		assert.Equal(t, original.From, deserialized.From, "From should be preserved")
		assert.Equal(t, original.To, deserialized.To, "To should be preserved")
		assert.Equal(t, original.Timestamp.UTC(), deserialized.Timestamp.UTC(), "Timestamp should be preserved")
		assert.Equal(t, original.ReplyTo, deserialized.ReplyTo, "ReplyTo should be preserved")

		// 属性: 取消序列化消息应有效
		err = deserialized.Validate()
		assert.NoError(t, err, "Deserialized message should be valid")
	})
}

// TestProperty A2AMessage RundTrip AllMessageTypes 测试每条消息类型的回程.
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
				// 用特定类型生成信件
				msg := &A2AMessage{
					ID:        genMessageID().Draw(rt, "id"),
					Type:      msgType,
					From:      genAgentID().Draw(rt, "from"),
					To:        genAgentID().Draw(rt, "to"),
					Payload:   genSimplePayload().Draw(rt, "payload"),
					Timestamp: genTimestamp().Draw(rt, "timestamp"),
				}

				// 往返旅行
				data, err := json.Marshal(msg)
				require.NoError(t, err)

				var deserialized A2AMessage
				err = json.Unmarshal(data, &deserialized)
				require.NoError(t, err)

				// 财产: 类型应准确保留
				assert.Equal(t, msgType, deserialized.Type,
					"Message type %s should be preserved", msgType)
			})
		})
	}
}

// 测试Property A2AMessage RundTrip With VariousPayloads 以不同有效载荷类型进行往返试验.
func TestProperty_A2AMessage_RoundTrip_WithVariousPayloads(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		baseMsg := &A2AMessage{
			ID:        genMessageID().Draw(rt, "id"),
			Type:      A2AMessageTypeTask,
			From:      genAgentID().Draw(rt, "from"),
			To:        genAgentID().Draw(rt, "to"),
			Timestamp: genTimestamp().Draw(rt, "timestamp"),
		}

		// 无有效载荷试验
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

		// 用字符串有效载荷进行测试
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

		// 用地图有效载荷进行测试
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

// 测试Property A2AMessage RundTrip PreservesValidation测试,有效消息在往返后仍然有效.
func TestProperty_A2AMessage_RoundTrip_PreservesValidation(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// 生成有效的信件
		original := genA2AMessage().Draw(rt, "message")

		// 校验正本是有效的
		err := original.Validate()
		require.NoError(t, err, "Original message should be valid")

		// 往返旅行
		data, err := json.Marshal(original)
		require.NoError(t, err)

		var deserialized A2AMessage
		err = json.Unmarshal(data, &deserialized)
		require.NoError(t, err)

		// 属性:去序消息也应有效
		err = deserialized.Validate()
		assert.NoError(t, err, "Deserialized message should remain valid after round-trip")
	})
}

// TestProperty A2AMessage RundTrip Reply Tooptional 测试可选的 ReferenceTo 字段得到正确处理.
func TestProperty_A2AMessage_RoundTrip_ReplyToOptional(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// 以回覆方式测试
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

		// 没有回复的测试
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

// 测试Property A2AMessage RundTrip Timestamp 精度测试 时间戳精度保存.
func TestProperty_A2AMessage_RoundTrip_TimestampPrecision(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// 第二精度生成时间戳( JSON 通常使用 RFC3339)
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

		// 属性: 时间戳在协调世界时应当相等
		assert.True(t, msg.Timestamp.UTC().Equal(deserialized.Timestamp.UTC()),
			"Timestamp should be preserved with correct precision")
	})
}
