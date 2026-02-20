package persistence

import (
	"context"
	"encoding/json"
	"time"
)

// MessageStore定义了信件持久性的界面.
// 它提供可靠的信息传送,并附有确认和重新测试支持。
type MessageStore interface {
	Store

	// 保存Message 坚持给商店的单个消息
	SaveMessage(ctx context.Context, msg *Message) error

	// 保存消息在解剖上持续了多个消息
	SaveMessages(ctx context.Context, msgs []*Message) error

	// 通过 ID 获取信件
	GetMessage(ctx context.Context, msgID string) (*Message, error)

	// GetMessages 获取带有 pagination 主题的信息
	// 返回信件、 下个光标和错误
	GetMessages(ctx context.Context, topic string, cursor string, limit int) ([]*Message, string, error)

	// AckMessage 标记已确认/处理的信息
	AckMessage(ctx context.Context, msgID string) error

	// 获取未保存的邮件获取未确认的比指定时间长的信件
	// 这些留言是重试的候选人
	GetUnackedMessages(ctx context.Context, topic string, olderThan time.Duration) ([]*Message, error)

	// GetPendingMessages 检索需要发送的信件
	// 这包括需要重试的新信件和信件
	GetPendingMessages(ctx context.Context, topic string, limit int) ([]*Message, error)

	// 递增
	IncrementRetry(ctx context.Context, msgID string) error

	// 删除信件从存储处删除
	DeleteMessage(ctx context.Context, msgID string) error

	// 清理删除旧消息
	Cleanup(ctx context.Context, olderThan time.Duration) (int, error)

	// Stats 返回关于消息库的统计数据
	Stats(ctx context.Context) (*MessageStoreStats, error)
}

// 信件代表系统中的持久信息
type Message struct {
	// ID 是信件的唯一标识符
	ID string `json:"id"`

	// 题目是信息主题/频道
	Topic string `json:"topic"`

	// FromID 是发送代理 ID
	FromID string `json:"from_id"`

	// ToID 是接收代理ID( 空来播放)
	ToID string `json:"to_id,omitempty"`

	// 类型是信件类型(提议、回应、表决等)
	Type string `json:"type"`

	// 内容是信件内容
	Content string `json:"content"`

	// 有效载荷包含额外的结构化数据
	Payload map[string]any `json:"payload,omitempty"`

	// 元数据包含信件元数据
	Metadata map[string]string `json:"metadata,omitempty"`

	// 创建到信件创建时
	CreatedAt time.Time `json:"created_at"`

	// AckedAt是消息被承认的时候(即使没有被承认也没有)
	AckedAt *time.Time `json:"acked_at,omitempty"`

	// 重试( Rettry Count) 是送货尝试的次数
	RetryCount int `json:"retry_count"`

	// Last RetryAt 是上次尝试重试时
	LastRetryAt *time.Time `json:"last_retry_at,omitempty"`

	// 过期是信件过期时( 可选)
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// JSON警长执行JSON。 元目录
func (m *Message) MarshalJSON() ([]byte, error) {
	type Alias Message
	return json.Marshal(&struct {
		*Alias
	}{
		Alias: (*Alias)(m),
	})
}

// UnmarshalJSON 执行json。 解马沙勒
func (m *Message) UnmarshalJSON(data []byte) error {
	type Alias Message
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(m),
	}
	return json.Unmarshal(data, aux)
}

// 如果信件已过期, 检查已过期
func (m *Message) IsExpired() bool {
	if m.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*m.ExpiresAt)
}

// 如果信件已被确认, 将会被检查
func (m *Message) IsAcked() bool {
	return m.AckedAt != nil
}

// 是否应根据重试配置重试信件
func (m *Message) ShouldRetry(config RetryConfig) bool {
	if m.IsAcked() || m.IsExpired() {
		return false
	}
	return m.RetryCount < config.MaxRetries
}

// 下次重试时计算
func (m *Message) NextRetryTime(config RetryConfig) time.Time {
	backoff := config.CalculateBackoff(m.RetryCount)
	if m.LastRetryAt != nil {
		return m.LastRetryAt.Add(backoff)
	}
	return m.CreatedAt.Add(backoff)
}

// 信件Stats 包含关于信件存储的统计数据
type MessageStoreStats struct {
	// TotalMessages 为商店中信件的总数
	TotalMessages int64 `json:"total_messages"`

	// 未决信件是未确认信件的数量
	PendingMessages int64 `json:"pending_messages"`

	// AckedMessages 是确认消息的数量
	AckedMessages int64 `json:"acked_messages"`

	// 过期信件是过期信件的数量
	ExpiredMessages int64 `json:"expired_messages"`

	// 主题计数为每个主题的信息数
	TopicCounts map[string]int64 `json:"topic_counts"`

	// 最老的PendingAge是最老的待发消息的年龄
	OldestPendingAge time.Duration `json:"oldest_pending_age"`
}

// MessageFilter 定义过滤信件的标准
type MessageFilter struct {
	// 按主题划分的专题过滤器
	Topic string `json:"topic,omitempty"`

	// 发送者从ID中过滤
	FromID string `json:"from_id,omitempty"`

	// 收件人的 ToID 过滤器
	ToID string `json:"to_id,omitempty"`

	// 按信件类型输入过滤器
	Type string `json:"type,omitempty"`

	// 通过承认状态进行状态过滤
	Status MessageStatus `json:"status,omitempty"`

	// 在此时间之后创建过滤信件
	CreatedAfter *time.Time `json:"created_after,omitempty"`

	// 在此之前创建过滤信件
	CreatedBefore *time.Time `json:"created_before,omitempty"`

	// 限定要返回的信件的最大数量
	Limit int `json:"limit,omitempty"`

	// 偏移为要跳过的信件数量
	Offset int `json:"offset,omitempty"`
}

// 信件状态代表信件状态
type MessageStatus string

const (
	// 信件状态显示信件正在等待处理
	MessageStatusPending MessageStatus = "pending"

	// 信件状态显示信件已被确认
	MessageStatusAcked MessageStatus = "acked"

	// 信件状态已过期 。
	MessageStatusExpired MessageStatus = "expired"

	// 信件状态失败, 表示信件在最大重试后失败
	MessageStatusFailed MessageStatus = "failed"
)
