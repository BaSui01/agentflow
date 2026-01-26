// Package streaming provides backpressure-aware streaming for high-throughput LLM responses.
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

// Token represents a streaming token.
type Token struct {
	Content   string    `json:"content"`
	Index     int       `json:"index"`
	Timestamp time.Time `json:"timestamp"`
	Final     bool      `json:"final"`
}

// BackpressureConfig configures backpressure behavior.
type BackpressureConfig struct {
	BufferSize      int           `json:"buffer_size"`
	HighWaterMark   float64       `json:"high_water_mark"` // 0.0-1.0
	LowWaterMark    float64       `json:"low_water_mark"`  // 0.0-1.0
	SlowConsumerTTL time.Duration `json:"slow_consumer_ttl"`
	DropPolicy      DropPolicy    `json:"drop_policy"`
}

// DropPolicy defines what to do when buffer is full.
type DropPolicy int

const (
	DropPolicyBlock  DropPolicy = iota // Block producer
	DropPolicyOldest                   // Drop oldest tokens
	DropPolicyNewest                   // Drop newest tokens
	DropPolicyError                    // Return error
)

// DefaultBackpressureConfig returns optimized defaults.
func DefaultBackpressureConfig() BackpressureConfig {
	return BackpressureConfig{
		BufferSize:      1024,
		HighWaterMark:   0.8,
		LowWaterMark:    0.2,
		SlowConsumerTTL: 30 * time.Second,
		DropPolicy:      DropPolicyBlock,
	}
}

// BackpressureStream implements backpressure-aware token streaming.
type BackpressureStream struct {
	config BackpressureConfig
	buffer chan Token
	done   chan struct{}
	closed atomic.Bool
	mu     sync.RWMutex

	// Metrics
	produced  atomic.Int64
	consumed  atomic.Int64
	dropped   atomic.Int64
	blocked   atomic.Int64
	lastWrite atomic.Int64
	lastRead  atomic.Int64

	// Flow control
	paused   atomic.Bool
	pauseCh  chan struct{}
	resumeCh chan struct{}
}

// NewBackpressureStream creates a new backpressure-aware stream.
func NewBackpressureStream(config BackpressureConfig) *BackpressureStream {
	return &BackpressureStream{
		config:   config,
		buffer:   make(chan Token, config.BufferSize),
		done:     make(chan struct{}),
		pauseCh:  make(chan struct{}, 1),
		resumeCh: make(chan struct{}, 1),
	}
}

// Write sends a token to the stream with backpressure handling.
func (s *BackpressureStream) Write(ctx context.Context, token Token) error {
	if s.closed.Load() {
		return ErrStreamClosed
	}

	s.lastWrite.Store(time.Now().UnixNano())

	// Check buffer level
	level := float64(len(s.buffer)) / float64(s.config.BufferSize)

	// Apply backpressure at high water mark
	if level >= s.config.HighWaterMark {
		s.paused.Store(true)
		s.blocked.Add(1)

		switch s.config.DropPolicy {
		case DropPolicyBlock:
			// Wait for buffer to drain
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
			// Drop oldest token
			select {
			case <-s.buffer:
				s.dropped.Add(1)
			default:
			}
			s.buffer <- token
			s.produced.Add(1)
			return nil

		case DropPolicyNewest:
			// Drop this token
			s.dropped.Add(1)
			return nil

		case DropPolicyError:
			return ErrBufferFull
		}
	}

	// Resume at low water mark
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

// Read receives tokens from the stream.
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

// ReadChan returns a channel for reading tokens.
func (s *BackpressureStream) ReadChan() <-chan Token {
	return s.buffer
}

// Close closes the stream.
func (s *BackpressureStream) Close() error {
	if s.closed.Swap(true) {
		return nil // Already closed
	}
	close(s.done)
	close(s.buffer)
	return nil
}

// IsPaused returns whether the stream is paused due to backpressure.
func (s *BackpressureStream) IsPaused() bool {
	return s.paused.Load()
}

// BufferLevel returns the current buffer utilization (0.0-1.0).
func (s *BackpressureStream) BufferLevel() float64 {
	return float64(len(s.buffer)) / float64(s.config.BufferSize)
}

// Stats returns stream statistics.
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

// StreamStats contains stream statistics.
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

// StreamMultiplexer fans out a single stream to multiple consumers.
type StreamMultiplexer struct {
	source    *BackpressureStream
	consumers []*BackpressureStream
	mu        sync.RWMutex
	running   atomic.Bool
}

// NewStreamMultiplexer creates a new multiplexer.
func NewStreamMultiplexer(source *BackpressureStream) *StreamMultiplexer {
	return &StreamMultiplexer{
		source:    source,
		consumers: make([]*BackpressureStream, 0),
	}
}

// AddConsumer adds a consumer stream.
func (m *StreamMultiplexer) AddConsumer(config BackpressureConfig) *BackpressureStream {
	m.mu.Lock()
	defer m.mu.Unlock()

	consumer := NewBackpressureStream(config)
	m.consumers = append(m.consumers, consumer)
	return consumer
}

// Start begins multiplexing.
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
		// Non-blocking write to each consumer
		select {
		case consumer.buffer <- token:
		default:
			// Consumer is slow, apply its drop policy
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

// RateLimiter provides token-based rate limiting for streams.
type RateLimiter struct {
	tokensPerSec float64
	bucket       float64
	maxBucket    float64
	lastRefill   time.Time
	mu           sync.Mutex
}

// NewRateLimiter creates a new rate limiter.
func NewRateLimiter(tokensPerSec float64, burst int) *RateLimiter {
	return &RateLimiter{
		tokensPerSec: tokensPerSec,
		bucket:       float64(burst),
		maxBucket:    float64(burst),
		lastRefill:   time.Now(),
	}
}

// Allow checks if a token can be consumed.
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

// Wait blocks until a token is available.
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
