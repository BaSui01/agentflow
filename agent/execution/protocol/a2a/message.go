package a2a

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// A2AMessageType代表A2A消息的类型.
type A2AMessageType string

const (
	// A2AMessage TypeTask 表示任务请求消息 。
	A2AMessageTypeTask A2AMessageType = "task"
	// A2AMessageTypeResult表示任务结果消息.
	A2AMessageTypeResult A2AMessageType = "result"
	// A2AMessage TypeError 表示错误消息 。
	A2AMessageTypeError A2AMessageType = "error"
	// A2AMessage Type Status 表示状态更新消息.
	A2AMessageTypeStatus A2AMessageType = "status"
	// A2AMessage TypeCancel 表示任务取消消息.
	A2AMessageTypeCancel A2AMessageType = "cancel"
)

// IsValid 检查信件类型是否为有效的 A2A 信件类型 。
func (t A2AMessageType) IsValid() bool {
	switch t {
	case A2AMessageTypeTask, A2AMessageTypeResult, A2AMessageTypeError,
		A2AMessageTypeStatus, A2AMessageTypeCancel:
		return true
	default:
		return false
	}
}

// 字符串返回消息类型的字符串表示。
func (t A2AMessageType) String() string {
	return string(t)
}

// A2AMessage代表了代理对代理通信的A2A标准消息.
type A2AMessage struct {
	// ID是此消息的唯一标识符 。
	ID string `json:"id"`
	// 类型表示信件类型(任务,结果,出错,状态,取消).
	Type A2AMessageType `json:"type"`
	// 从是发件人的标识符.
	From string `json:"from"`
	// 为接收代理的识别符.
	To string `json:"to"`
	// 有效载荷包含消息数据.
	Payload any `json:"payload"`
	// 时间戳是消息创建时.
	Timestamp time.Time `json:"timestamp"`
	// PresidentTo 是此回复信件的ID( 可选) 。
	ReplyTo string `json:"reply_to,omitempty"`
}

// 新建A2 AMessage创建了一个新的A2AMessage,并带有生成的ID和当前时间戳.
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

// NewTaskMessage 创建了新任务请求消息.
func NewTaskMessage(from, to string, payload any) *A2AMessage {
	return NewA2AMessage(A2AMessageTypeTask, from, to, payload)
}

// NewResultMessage在回复任务时创建了一个新的结果信息.
func NewResultMessage(from, to string, payload any, replyTo string) *A2AMessage {
	msg := NewA2AMessage(A2AMessageTypeResult, from, to, payload)
	msg.ReplyTo = replyTo
	return msg
}

// NewErrorMessage 创建新错误消息以响应任务 。
func NewErrorMessage(from, to string, payload any, replyTo string) *A2AMessage {
	msg := NewA2AMessage(A2AMessageTypeError, from, to, payload)
	msg.ReplyTo = replyTo
	return msg
}

// 新状态消息创建新状态更新消息 。
func NewStatusMessage(from, to string, payload any, replyTo string) *A2AMessage {
	msg := NewA2AMessage(A2AMessageTypeStatus, from, to, payload)
	msg.ReplyTo = replyTo
	return msg
}

// NewCancelMessage创建了新的取消消息.
func NewCancelMessage(from, to string, taskID string) *A2AMessage {
	return NewA2AMessage(A2AMessageTypeCancel, from, to, map[string]string{"task_id": taskID})
}

// 验证 A2AMessage 是否有所有所需的字段和有效值 。
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

// IsReply 检查此信件是否是对另一个信件的回复 。
func (m *A2AMessage) IsReply() bool {
	return m.ReplyTo != ""
}

// 如果这是任务请求信件, 请检查任务 。
func (m *A2AMessage) IsTask() bool {
	return m.Type == A2AMessageTypeTask
}

// 是否是结果信息 。
func (m *A2AMessage) IsResult() bool {
	return m.Type == A2AMessageTypeResult
}

// IsError 检查是否为错误消息 。
func (m *A2AMessage) IsError() bool {
	return m.Type == A2AMessageTypeError
}

// 如果这是状态消息, 请检查状态 。
func (m *A2AMessage) IsStatus() bool {
	return m.Type == A2AMessageTypeStatus
}

// IsCancel 检查是否是取消消息 。
func (m *A2AMessage) IsCancel() bool {
	return m.Type == A2AMessageTypeCancel
}

// JSON警长执行JSON。 A2AMessage的元帅
func (m *A2AMessage) MarshalJSON() ([]byte, error) {
	type Alias A2AMessage
	return json.Marshal(&struct {
		*Alias
	}{
		Alias: (*Alias)(m),
	})
}

// UnmarshalJSON 执行json。 A2AMessage的解马沙勒.
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

// 克隆人创建了消息的深层拷贝.
func (m *A2AMessage) Clone() *A2AMessage {
	clone := &A2AMessage{
		ID:        m.ID,
		Type:      m.Type,
		From:      m.From,
		To:        m.To,
		Timestamp: m.Timestamp,
		ReplyTo:   m.ReplyTo,
	}

	// 深复制有效载荷,如果它是地图或切片
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

// CreatReply 创建此信件的回覆信件 。
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

// ToJSON将消息序列化给JSON字节.
func (m *A2AMessage) ToJSON() ([]byte, error) {
	return json.Marshal(m)
}

// JSON将JSON字节中的信息解析出来.
func FromJSON(data []byte) (*A2AMessage, error) {
	var msg A2AMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// ParseA2AMessage将JSON数据分解为A2AMessage并验证.
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
