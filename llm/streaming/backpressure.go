package streaming

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrBufferFull   = errors.New("buffer full, backpressure applied")
	ErrStreamClosed = errors.New("stream closed")
	ErrSlowConsumer = errors.New("consumer too slow")
)

// Token 表示流式传输的 token.
type Token struct {
	Content   string    `json:"content"`
	Index     int       `json:"index"`
	Timestamp time.Time `json:"timestamp"`
	Final     bool      `json:"final"`
}

// BackpressureConfig 配置背压行为.
type BackpressureConfig struct {
	BufferSize      int           `json:"buffer_size"`
	HighWaterMark   float64       `json:"high_water_mark"` // 0.0-1.0
	LowWaterMark    float64       `json:"low_water_mark"`  // 0.0-1.0
	SlowConsumerTTL time.Duration `json:"slow_consumer_ttl"`
	DropPolicy      DropPolicy    `json:"drop_policy"`
}

// DropPolicy 定义缓冲区满后的处理策略.
type DropPolicy int

const (
	DropPolicyBlock  DropPolicy = iota // Block producer
	DropPolicyOldest                   // Drop oldest tokens
	DropPolicyNewest                   // Drop newest tokens
	DropPolicyError                    // Return error
)

// String returns the string representation of DropPolicy.
func (d DropPolicy) String() string {
	switch d {
	case DropPolicyBlock:
		return "block"
	case DropPolicyOldest:
		return "oldest"
	case DropPolicyNewest:
		return "newest"
	case DropPolicyError:
		return "error"
	default:
		return fmt.Sprintf("DropPolicy(%d)", d)
	}
}

// DefaultBackpressureConfig 返回优化的默认值.
func DefaultBackpressureConfig() BackpressureConfig {
	return BackpressureConfig{
		BufferSize:      1024,
		HighWaterMark:   0.8,
		LowWaterMark:    0.2,
		SlowConsumerTTL: 30 * time.Second,
		DropPolicy:      DropPolicyBlock,
	}
}

// BackpressureStream 实现支持背压的流.
type BackpressureStream struct {
	config BackpressureConfig
	buffer chan Token
	done   chan struct{}
	closed atomic.Bool
	closeOnce sync.Once
	mu     sync.RWMutex

	// 指标
	produced  atomic.Int64
	consumed  atomic.Int64
	dropped   atomic.Int64
	blocked   atomic.Int64
	lastWrite atomic.Int64
	lastRead  atomic.Int64

	// 流量控制
	paused   atomic.Bool
	pauseCh  chan struct{}
	resumeCh chan struct{}
}

// NewBackpressureStream 创建新的支持背压的流.
func NewBackpressureStream(config BackpressureConfig) *BackpressureStream {
	return &BackpressureStream{
		config:   config,
		buffer:   make(chan Token, config.BufferSize),
		done:     make(chan struct{}),
		pauseCh:  make(chan struct{}, 1),
		resumeCh: make(chan struct{}, 1),
	}
}

// Write 向流发送一个带背压处理的 token.
// 使用 RLock 防止与 Close() 并发执行时向已关闭 channel 发送导致 panic。
func (s *BackpressureStream) Write(ctx context.Context, token Token) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed.Load() {
		return ErrStreamClosed
	}

	s.lastWrite.Store(time.Now().UnixNano())

	// 检查缓冲区级别
	level := float64(len(s.buffer)) / float64(s.config.BufferSize)

	// 高水位时应用背压
	if level >= s.config.HighWaterMark {
		s.paused.Store(true)
		s.blocked.Add(1)

		switch s.config.DropPolicy {
		case DropPolicyBlock:
			// 等待缓冲区排空
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-s.done:
				return ErrStreamClosed
			case s.buffer <- token:
				s.produced.Add(1)
				return nil
			}

		case DropPolicyOldest:
			// 丢弃最旧的 token
			select {
			case <-s.buffer:
				s.dropped.Add(1)
			default:
			}
			// 使用 select 保护写入，防止并发 Write 填满 buffer 导致永久阻塞
			select {
			case s.buffer <- token:
				s.produced.Add(1)
				return nil
			case <-ctx.Done():
				return ctx.Err()
			case <-s.done:
				return ErrStreamClosed
			}

		case DropPolicyNewest:
			// 丢弃此 token
			s.dropped.Add(1)
			return nil

		case DropPolicyError:
			return ErrBufferFull
		}
	}

	// 低水位时恢复
	if level <= s.config.LowWaterMark {
		s.paused.Store(false)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.done:
		return ErrStreamClosed
	case s.buffer <- token:
		s.produced.Add(1)
		return nil
	}
}

// Read 从流中读取 token.
func (s *BackpressureStream) Read(ctx context.Context) (Token, error) {
	if s.closed.Load() && len(s.buffer) == 0 {
		return Token{}, ErrStreamClosed
	}

	s.lastRead.Store(time.Now().UnixNano())

	select {
	case <-ctx.Done():
		return Token{}, ctx.Err()
	case token, ok := <-s.buffer:
		if !ok {
			return Token{}, ErrStreamClosed
		}
		s.consumed.Add(1)
		return token, nil
	}
}

// ReadChan 返回用于读取 token 的通道.
func (s *BackpressureStream) ReadChan() <-chan Token {
	return s.buffer
}

// Close 关闭流.
// 使用写锁(Lock)确保与 Write() 的 RLock 互斥，
// 防止在 Write 发送到 buffer channel 的同时关闭 channel 导致 panic。
func (s *BackpressureStream) Close() error {
	if s.closed.Swap(true) {
		return nil // Already closed
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closeOnce.Do(func() { close(s.done); close(s.buffer) })
	return nil
}

// IsPaused 返回流是否因背压而暂停.
func (s *BackpressureStream) IsPaused() bool {
	return s.paused.Load()
}

// BufferLevel 返回当前缓冲区利用率 (0.0-1.0).
func (s *BackpressureStream) BufferLevel() float64 {
	return float64(len(s.buffer)) / float64(s.config.BufferSize)
}

// Stats 返回流统计信息.
func (s *BackpressureStream) Stats() StreamStats {
	return StreamStats{
		Produced:   s.produced.Load(),
		Consumed:   s.consumed.Load(),
		Dropped:    s.dropped.Load(),
		Blocked:    s.blocked.Load(),
		BufferSize: len(s.buffer),
		BufferCap:  s.config.BufferSize,
		IsPaused:   s.paused.Load(),
		LastWrite:  time.Unix(0, s.lastWrite.Load()),
		LastRead:   time.Unix(0, s.lastRead.Load()),
	}
}

// StreamStats 包含流统计数据.
type StreamStats struct {
	Produced   int64     `json:"produced"`
	Consumed   int64     `json:"consumed"`
	Dropped    int64     `json:"dropped"`
	Blocked    int64     `json:"blocked"`
	BufferSize int       `json:"buffer_size"`
	BufferCap  int       `json:"buffer_cap"`
	IsPaused   bool      `json:"is_paused"`
	LastWrite  time.Time `json:"last_write"`
	LastRead   time.Time `json:"last_read"`
}

// StreamMultiplexer 将一个流扇出给多个消费者.
type StreamMultiplexer struct {
	source    *BackpressureStream
	consumers []*BackpressureStream
	mu        sync.RWMutex
	running   atomic.Bool
}

// NewStreamMultiplexer 创建新的多路复用器.
func NewStreamMultiplexer(source *BackpressureStream) *StreamMultiplexer {
	return &StreamMultiplexer{
		source:    source,
		consumers: make([]*BackpressureStream, 0),
	}
}

// AddConsumer 添加一个消费流.
func (m *StreamMultiplexer) AddConsumer(config BackpressureConfig) *BackpressureStream {
	m.mu.Lock()
	defer m.mu.Unlock()

	consumer := NewBackpressureStream(config)
	m.consumers = append(m.consumers, consumer)
	return consumer
}

// Start 启动多路复用.
func (m *StreamMultiplexer) Start(ctx context.Context) {
	if m.running.Swap(true) {
		return
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				m.closeAll()
				return
			case token, ok := <-m.source.ReadChan():
				if !ok {
					m.closeAll()
					return
				}
				m.broadcast(ctx, token)
			}
		}
	}()
}

func (m *StreamMultiplexer) broadcast(ctx context.Context, token Token) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, consumer := range m.consumers {
		// 通过 Write() 方法发送 token，而非直接写 consumer.buffer。
		// Write() 内部持有 RLock，与 Close() 的 Lock 互斥，
		// 消除了 closed.Load() 与 channel 发送之间的 TOCTOU 窗口。
		if err := consumer.Write(ctx, token); err != nil {
			// consumer 已关闭或 ctx 取消 — 安全忽略
		}
	}
}

func (m *StreamMultiplexer) closeAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, consumer := range m.consumers {
		consumer.Close()
	}
	m.running.Store(false)
}

// RateLimiter 为流提供基于 token 的速率限制.
type RateLimiter struct {
	tokensPerSec float64
	bucket       float64
	maxBucket    float64
	lastRefill   time.Time
	mu           sync.Mutex
}

// NewRateLimiter 创建新的速率限制器.
func NewRateLimiter(tokensPerSec float64, burst int) *RateLimiter {
	return &RateLimiter{
		tokensPerSec: tokensPerSec,
		bucket:       float64(burst),
		maxBucket:    float64(burst),
		lastRefill:   time.Now(),
	}
}

// Allow 检查是否可以消耗一个 token.
func (r *RateLimiter) Allow() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.refill()

	if r.bucket >= 1 {
		r.bucket--
		return true
	}
	return false
}

// Wait 阻塞直到一个 token 可用.
func (r *RateLimiter) Wait(ctx context.Context) error {
	for {
		if r.Allow() {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(1000/r.tokensPerSec) * time.Millisecond):
		}
	}
}

func (r *RateLimiter) refill() {
	now := time.Now()
	elapsed := now.Sub(r.lastRefill).Seconds()
	r.bucket += elapsed * r.tokensPerSec
	if r.bucket > r.maxBucket {
		r.bucket = r.maxBucket
	}
	r.lastRefill = now
}
