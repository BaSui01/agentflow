package runtime

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"sync"
	"sync/atomic"
	"time"

	"github.com/BaSui01/agentflow/types"
)

// =============================================================================
// SteeringChannel (merged from steering.go)
// =============================================================================

// 类型别名：方便包内使用，避免到处写 types.SteeringXxx
type SteeringMessage = types.SteeringMessage
type SteeringMessageType = types.SteeringMessageType

// 常量别名
const (
	SteeringTypeGuide       = types.SteeringTypeGuide
	SteeringTypeStopAndSend = types.SteeringTypeStopAndSend
)

// SteeringChannel 双向通信通道，用于在流式生成过程中注入用户指令
type SteeringChannel struct {
	ch     chan SteeringMessage
	closed atomic.Bool
}

// NewSteeringChannel 创建一个带缓冲的 steering 通道
func NewSteeringChannel(bufSize int) *SteeringChannel {
	if bufSize <= 0 {
		bufSize = 1
	}
	return &SteeringChannel{
		ch: make(chan SteeringMessage, bufSize),
	}
}

var (
	// ErrSteeringChannelClosed 通道已关闭
	ErrSteeringChannelClosed = errors.New("steering channel is closed")
	// ErrSteeringChannelFull 通道已满（非阻塞发送失败）
	ErrSteeringChannelFull = errors.New("steering channel is full")
)

// Send 向通道发送一条 steering 消息（非阻塞，panic-safe）
func (sc *SteeringChannel) Send(msg SteeringMessage) (err error) {
	if sc.closed.Load() {
		return ErrSteeringChannelClosed
	}
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}
	// recover 防止 Send 和 Close 之间的 TOCTOU 竞态导致 send-on-closed-channel panic
	defer func() {
		if r := recover(); r != nil {
			err = ErrSteeringChannelClosed
		}
	}()
	select {
	case sc.ch <- msg:
		return nil
	default:
		return ErrSteeringChannelFull
	}
}

// Receive 返回底层 channel，用于 select 监听
func (sc *SteeringChannel) Receive() <-chan SteeringMessage {
	return sc.ch
}

// Close 关闭通道
func (sc *SteeringChannel) Close() {
	if sc.closed.CompareAndSwap(false, true) {
		close(sc.ch)
	}
}

// IsClosed 检查通道是否已关闭
func (sc *SteeringChannel) IsClosed() bool {
	return sc.closed.Load()
}

// --- context 注入/提取 ---

type steeringChannelKey struct{}

// WithSteeringChannel 将 SteeringChannel 注入 context
func WithSteeringChannel(ctx context.Context, ch *SteeringChannel) context.Context {
	if ch == nil {
		return ctx
	}
	return context.WithValue(ctx, steeringChannelKey{}, ch)
}

// SteeringChannelFromContext 从 context 中提取 SteeringChannel
func SteeringChannelFromContext(ctx context.Context) (*SteeringChannel, bool) {
	if ctx == nil {
		return nil, false
	}
	ch, ok := ctx.Value(steeringChannelKey{}).(*SteeringChannel)
	return ch, ok && ch != nil
}

// steerChOrNil 返回 steering channel 的接收端，如果不存在则返回 nil（select 中永远不会触发）
func steerChOrNil(ch *SteeringChannel) <-chan SteeringMessage {
	if ch == nil {
		return nil
	}
	return ch.Receive()
}

// ExecutionSession 跟踪一个活跃的流式执行。
// SteeringChannel 是唯一的运行状态源，避免额外状态字段分叉。
type ExecutionSession struct {
	ID         string           `json:"id"`
	AgentID    string           `json:"agent_id"`
	SteeringCh *SteeringChannel `json:"-"`
	CreatedAt  time.Time        `json:"created_at"`
}

// Status 返回当前会话状态（单一状态源：SteeringChannel.IsClosed）。
func (s *ExecutionSession) Status() string {
	if s.SteeringCh.IsClosed() {
		return "completed"
	}
	return "running"
}

// Complete 标记会话为已完成（关闭 steering channel）。
func (s *ExecutionSession) Complete() {
	s.SteeringCh.Close()
}

// IsRunning 检查会话是否仍在运行。
func (s *ExecutionSession) IsRunning() bool {
	return !s.SteeringCh.IsClosed()
}

// SessionManager 管理活跃的流式执行会话（内存 map + 自动过期清理）。
type SessionManager struct {
	sessions sync.Map
	stopOnce sync.Once
	stopCh   chan struct{}
}

// NewSessionManager 创建会话管理器并启动后台清理 goroutine。
func NewSessionManager() *SessionManager {
	m := &SessionManager{
		stopCh: make(chan struct{}),
	}
	go m.cleanupLoop()
	return m
}

// Create 创建一个新的执行会话。
func (m *SessionManager) Create(agentID string) *ExecutionSession {
	sess := &ExecutionSession{
		ID:         fmt.Sprintf("exec_%s", uuid.New().String()[:12]),
		AgentID:    agentID,
		SteeringCh: NewSteeringChannel(4),
		CreatedAt:  time.Now(),
	}
	m.sessions.Store(sess.ID, sess)
	return sess
}

// Get 根据 ID 获取会话。
func (m *SessionManager) Get(id string) (*ExecutionSession, bool) {
	v, ok := m.sessions.Load(id)
	if !ok {
		return nil, false
	}
	return v.(*ExecutionSession), true
}

// Remove 移除会话并关闭其 steering channel。
func (m *SessionManager) Remove(id string) {
	if v, loaded := m.sessions.LoadAndDelete(id); loaded {
		v.(*ExecutionSession).Complete()
	}
}

// Cleanup 清理过期会话：已完成的超过 maxAge 清理，活跃的不强制终止。
func (m *SessionManager) Cleanup(maxAge time.Duration) {
	cutoff := time.Now().Add(-maxAge)
	m.sessions.Range(func(key, value any) bool {
		sess := value.(*ExecutionSession)
		if sess.CreatedAt.Before(cutoff) && !sess.IsRunning() {
			m.sessions.Delete(key)
		}
		return true
	})
}

// Stop 停止后台清理 goroutine。
func (m *SessionManager) Stop() {
	m.stopOnce.Do(func() {
		close(m.stopCh)
	})
}

// cleanupLoop 每 60s 清理超过 30min 的过期会话。
func (m *SessionManager) cleanupLoop() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.Cleanup(30 * time.Minute)
		case <-m.stopCh:
			return
		}
	}
}
