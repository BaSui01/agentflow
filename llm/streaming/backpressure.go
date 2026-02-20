// 包流为高通量LLM响应提供回压-意识流.
package streaming

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrBufferFull   = errors.New("buffer full, backpressure applied")
	ErrStreamClosed = errors.New("stream closed")
	ErrSlowConsumer = errors.New("consumer too slow")
)

// Token 代表着流体符号 。
type Token struct {
	Content   string    `json:"content"`
	Index     int       `json:"index"`
	Timestamp time.Time `json:"timestamp"`
	Final     bool      `json:"final"`
}

// 后压Config配置后压行为.
type BackpressureConfig struct {
	BufferSize      int           `json:"buffer_size"`
	HighWaterMark   float64       `json:"high_water_mark"` // 0.0-1.0
	LowWaterMark    float64       `json:"low_water_mark"`  // 0.0-1.0
	SlowConsumerTTL time.Duration `json:"slow_consumer_ttl"`
	DropPolicy      DropPolicy    `json:"drop_policy"`
}

// DroppPolicy定义了缓冲器满后要做什么.
type DropPolicy int

const (
	DropPolicyBlock  DropPolicy = iota // Block producer
	DropPolicyOldest                   // Drop oldest tokens
	DropPolicyNewest                   // Drop newest tokens
	DropPolicyError                    // Return error
)

// 默认压缩 Config 返回优化默认值 。
func DefaultBackpressureConfig() BackpressureConfig {
	return BackpressureConfig{
		BufferSize:      1024,
		HighWaterMark:   0.8,
		LowWaterMark:    0.2,
		SlowConsumerTTL: 30 * time.Second,
		DropPolicy:      DropPolicyBlock,
	}
}

// 后压Stream 执行后压-知觉信号流.
type BackpressureStream struct {
	config BackpressureConfig
	buffer chan Token
	done   chan struct{}
	closed atomic.Bool
	mu     sync.RWMutex

	// 计量
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

// NewBackpressure Stream 创造出新的后压-意识流.
func NewBackpressureStream(config BackpressureConfig) *BackpressureStream {
	return &BackpressureStream{
		config:   config,
		buffer:   make(chan Token, config.BufferSize),
		done:     make(chan struct{}),
		pauseCh:  make(chan struct{}, 1),
		resumeCh: make(chan struct{}, 1),
	}
}

// 写给溪流发一个有后压处理的令牌.
func (s *BackpressureStream) Write(ctx context.Context, token Token) error {
	if s.closed.Load() {
		return ErrStreamClosed
	}

	s.lastWrite.Store(time.Now().UnixNano())

	// 检查缓冲级别
	level := float64(len(s.buffer)) / float64(s.config.BufferSize)

	// 高水分时应用回压
	if level >= s.config.HighWaterMark {
		s.paused.Store(true)
		s.blocked.Add(1)

		switch s.config.DropPolicy {
		case DropPolicyBlock:
			// 等待缓冲排出
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
			// 丢弃最旧的符号
			select {
			case <-s.buffer:
				s.dropped.Add(1)
			default:
			}
			s.buffer <- token
			s.produced.Add(1)
			return nil

		case DropPolicyNewest:
			// 丢弃此标志
			s.dropped.Add(1)
			return nil

		case DropPolicyError:
			return ErrBufferFull
		}
	}

	// 低水分恢复状态
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

// 读取从溪来接取符.
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

// ReadChan返回读取符的通道.
func (s *BackpressureStream) ReadChan() <-chan Token {
	return s.buffer
}

// 关上溪口.
func (s *BackpressureStream) Close() error {
	if s.closed.Swap(true) {
		return nil // Already closed
	}
	close(s.done)
	close(s.buffer)
	return nil
}

// IsPaused 返回流是否因后压而暂停 。
func (s *BackpressureStream) IsPaused() bool {
	return s.paused.Load()
}

// 缓冲级返回当前缓冲利用率(0.0-1.0).
func (s *BackpressureStream) BufferLevel() float64 {
	return float64(len(s.buffer)) / float64(s.config.BufferSize)
}

// Stats 返回流统计 。
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

// StreamStats包含流统计数据.
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

// Stream Complexer 粉丝出一流 给多个消费者。
type StreamMultiplexer struct {
	source    *BackpressureStream
	consumers []*BackpressureStream
	mu        sync.RWMutex
	running   atomic.Bool
}

// NewStream多相机创建了新的多相机.
func NewStreamMultiplexer(source *BackpressureStream) *StreamMultiplexer {
	return &StreamMultiplexer{
		source:    source,
		consumers: make([]*BackpressureStream, 0),
	}
}

// 添加Concumer增加了消费流.
func (m *StreamMultiplexer) AddConsumer(config BackpressureConfig) *BackpressureStream {
	m.mu.Lock()
	defer m.mu.Unlock()

	consumer := NewBackpressureStream(config)
	m.consumers = append(m.consumers, consumer)
	return consumer
}

// 开始多路运行 。
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
		// 给每个消费者的无阻信
		select {
		case consumer.buffer <- token:
		default:
			// 消费者行为缓慢,运用其降价政策
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

// rateLimiter为流提供了基于符号的速率限制.
type RateLimiter struct {
	tokensPerSec float64
	bucket       float64
	maxBucket    float64
	lastRefill   time.Time
	mu           sync.Mutex
}

// NewRateLimiter创建了新的速率限制器.
func NewRateLimiter(tokensPerSec float64, burst int) *RateLimiter {
	return &RateLimiter{
		tokensPerSec: tokensPerSec,
		bucket:       float64(burst),
		maxBucket:    float64(burst),
		lastRefill:   time.Now(),
	}
}

// 允许检查是否可以消耗一个令牌 。
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

// 等待区块, 直到一个符号可用 。
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
