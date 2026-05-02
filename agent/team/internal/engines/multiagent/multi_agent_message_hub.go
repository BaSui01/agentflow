package multiagent

import (
	"context"
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/agent/persistence"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// NewMessageHub 创建消息中心
func NewMessageHub(logger *zap.Logger) *MessageHub {
	return &MessageHub{
		channels:    make(map[string]chan *Message),
		logger:      logger.With(zap.String("component", "message_hub")),
		retryConfig: persistence.DefaultRetryConfig(),
	}
}

// NewMessageHubWithStore 创建带持久化的消息中心
func NewMessageHubWithStore(logger *zap.Logger, store persistence.MessageStore) *MessageHub {
	hub := NewMessageHub(logger)
	hub.messageStore = store
	return hub
}

// SetMessageStore 设置消息存储（用于依赖注入）
func (h *MessageHub) SetMessageStore(store persistence.MessageStore) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.messageStore = store
}

// Close 关闭消息中心
// 使用 sync.Once 保护 channel 关闭，防止重复关闭 panic
func (h *MessageHub) Close() error {
	var closeErr error
	h.closeOnce.Do(func() {
		h.mu.Lock()
		defer h.mu.Unlock()

		h.closed = true

		// 关闭所有通道
		for _, ch := range h.channels {
			close(ch)
		}
		h.channels = nil

		// 关闭存储
		if h.messageStore != nil {
			closeErr = h.messageStore.Close()
		}
	})
	return closeErr
}

// CreateChannel 创建通道
func (h *MessageHub) CreateChannel(agentID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.channels[agentID] = make(chan *Message, 100)
}

// Send 发送消息
// 如果配置了持久化存储，消息会先持久化再投递
// 即使 channel 满了，消息也不会丢失
func (h *MessageHub) Send(msg *Message) error {
	return h.SendWithContext(context.Background(), msg)
}

// SendWithContext 发送消息（带上下文）
// 修复竞态条件：使用单次锁定确保操作原子性
func (h *MessageHub) SendWithContext(ctx context.Context, msg *Message) error {
	// 生成消息 ID（无需锁）
	if msg.ID == "" {
		msg.ID = uuid.New().String()
	}
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}

	// 如果有持久化存储，先持久化消息（无需锁）
	if h.messageStore != nil {
		persistMsg := h.toPersistMessage(msg)
		if err := h.messageStore.SaveMessage(ctx, persistMsg); err != nil {
			h.logger.Error("failed to persist message",
				zap.String("msg_id", msg.ID),
				zap.String("from_agent_id", msg.FromID),
				zap.String("to_agent_id", msg.ToID),
				zap.Error(err),
			)
			// 持久化失败不阻止消息投递，但记录错误
		}
	}

	// T-011: 缩小锁范围 — 仅在锁内读取 channels map，不在锁内做 channel 发送
	h.mu.RLock()
	if h.closed {
		h.mu.RUnlock()
		return fmt.Errorf("message hub is closed")
	}

	if msg.ToID == "" {
		// 广播：复制需要发送的 channel 列表
		targets := make(map[string]chan *Message, len(h.channels))
		for id, ch := range h.channels {
			if id != msg.FromID {
				targets[id] = ch
			}
		}
		h.mu.RUnlock()

		for id, ch := range targets {
			select {
			case ch <- msg:
				// 消息投递成功，确认消息
				go h.ackMessage(ctx, msg.ID)
			default:
				// channel 满了，消息已持久化，后续可重试
				h.logger.Debug("channel full, message persisted for retry",
					zap.String("to", id),
					zap.String("msg_id", msg.ID),
				)
			}
		}
	} else {
		// 点对点：读取目标 channel 后立即释放锁
		ch, ok := h.channels[msg.ToID]
		h.mu.RUnlock()

		if !ok {
			return fmt.Errorf("channel not found: %s", msg.ToID)
		}

		select {
		case ch <- msg:
			// 消息投递成功，确认消息
			go h.ackMessage(ctx, msg.ID)
		default:
			// channel 满了，消息已持久化，后续可重试
			h.logger.Debug("channel full, message persisted for retry",
				zap.String("to", msg.ToID),
				zap.String("msg_id", msg.ID),
			)
			// 不返回错误，因为消息已持久化
		}
	}

	return nil
}

// toPersistMessage 转换为持久化消息格式
func (h *MessageHub) toPersistMessage(msg *Message) *persistence.Message {
	body := msg.Body
	if body.Content == "" {
		body.Content = msg.Content
	}
	if body.Timestamp.IsZero() {
		body.Timestamp = msg.Timestamp
	}
	if body.Metadata == nil && len(msg.Metadata) > 0 {
		body.Metadata = toStringMetadata(msg.Metadata)
	}

	pm := persistence.NewMessageDAOFromTypes(
		msg.ToID, // topic
		msg.FromID,
		msg.ToID,
		string(msg.Type),
		body,
	)
	pm.ID = msg.ID
	if len(msg.Metadata) > 0 {
		if pm.Payload == nil {
			pm.Payload = make(map[string]any, 1)
		}
		pm.Payload[payloadAnyMetadataKey] = msg.Metadata
	}
	return pm
}

// fromPersistMessage 从持久化消息格式转换
func (h *MessageHub) fromPersistMessage(pm *persistence.Message) *Message {
	body := pm.ToTypesMessage()
	metadata := pm.Payload
	if raw, ok := pm.Payload[payloadAnyMetadataKey]; ok {
		if m, ok := raw.(map[string]any); ok {
			metadata = m
		}
	}

	return &Message{
		ID:        pm.ID,
		FromID:    pm.FromID,
		ToID:      pm.ToID,
		Type:      MessageType(pm.Type),
		Content:   body.Content,
		Metadata:  metadata,
		Timestamp: body.Timestamp,
		Body:      body,
	}
}

func toStringMetadata(in map[string]any) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ackMessage 确认消息已处理
// H1 FIX: 异步 goroutine 调用前检查 closed 标志，防止 store 关闭后访问
func (h *MessageHub) ackMessage(ctx context.Context, msgID string) {
	// H1 FIX: Close() 可能已关闭 messageStore，提前检查避免访问已关闭的 store
	h.mu.RLock()
	closed := h.closed
	h.mu.RUnlock()
	if closed {
		return
	}

	if h.messageStore == nil {
		return
	}
	if err := h.messageStore.AckMessage(ctx, msgID); err != nil {
		h.logger.Debug("failed to ack message",
			zap.String("msg_id", msgID),
			zap.Error(err),
		)
	}
}

// safeSend safely writes to a channel, recovering from panic if the channel is closed.
func safeSend(ch chan<- *Message, msg *Message) (sent bool) {
	defer func() {
		if r := recover(); r != nil {
			sent = false
		}
	}()
	ch <- msg
	return true
}

// RecoverMessages 恢复未处理的消息（服务重启后调用）
func (h *MessageHub) RecoverMessages(ctx context.Context) error {
	if h.messageStore == nil {
		return nil
	}
	h.logger.Info("recovering unprocessed messages")

	h.mu.RLock()
	agentIDs := make([]string, 0, len(h.channels))
	for id := range h.channels {
		agentIDs = append(agentIDs, id)
	}
	h.mu.RUnlock()

	totalRecovered := 0

	for _, agentID := range agentIDs {
		// 获取该 agent 的未确认消息
		msgs, err := h.messageStore.GetUnackedMessages(ctx, agentID, 5*time.Minute)
		if err != nil {
			h.logger.Warn("failed to get unacked messages",
				zap.String("agent_id", agentID),
				zap.Error(err),
			)
			continue
		}

		for _, pm := range msgs {
			// 检查是否应该重试
			if !pm.ShouldRetry(h.retryConfig) {
				h.logger.Debug("message exceeded max retries",
					zap.String("msg_id", pm.ID),
					zap.Int("retry_count", pm.RetryCount),
				)
				continue
			}

			// 增加重试计数
			if err := h.messageStore.IncrementRetry(ctx, pm.ID); err != nil {
				h.logger.Warn("failed to increment retry count",
					zap.String("msg_id", pm.ID),
					zap.Error(err),
				)
			}

			// 重新投递消息
			msg := h.fromPersistMessage(pm)
			h.mu.RLock()
			ch, ok := h.channels[agentID]
			h.mu.RUnlock()

			if ok {
				if !safeSend(ch, msg) {
					// channel was closed, stop recovery
					h.logger.Debug("message hub closed during send, stopping recovery",
						zap.String("msg_id", msg.ID),
						zap.String("to", agentID),
					)
					break
				}
				totalRecovered++
				h.logger.Debug("message recovered",
					zap.String("msg_id", msg.ID),
					zap.String("to", agentID),
				)
			}
		}
	}

	h.logger.Info("message recovery completed",
		zap.Int("recovered", totalRecovered),
	)

	return nil
}

// StartRetryLoop 启动消息重试循环
func (h *MessageHub) StartRetryLoop(ctx context.Context, interval time.Duration) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				h.logger.Error("panic in StartRetryLoop", zap.Any("panic", r))
			}
		}()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := h.RecoverMessages(ctx); err != nil {
					h.logger.Warn("retry loop error", zap.Error(err))
				}
			}
		}
	}()
}

// Stats 获取消息统计信息
func (h *MessageHub) Stats(ctx context.Context) (*persistence.MessageStoreStats, error) {
	if h.messageStore == nil {
		return nil, fmt.Errorf("no message store configured")
	}
	return h.messageStore.Stats(ctx)
}

// Receive 接收消息
func (h *MessageHub) Receive(agentID string, timeout time.Duration) (*Message, error) {
	h.mu.RLock()
	ch, ok := h.channels[agentID]
	h.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("channel not found: %s", agentID)
	}

	select {
	case msg := <-ch:
		return msg, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("receive timeout")
	}
}
