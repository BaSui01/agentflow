package agent

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/BaSui01/agentflow/types"
)

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
