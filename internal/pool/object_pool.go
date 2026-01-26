// Package pool provides high-performance object pooling using sync.Pool.
package pool

import (
	"bytes"
	"sync"
	"sync/atomic"
)

// Pool is a generic object pool.
type Pool[T any] struct {
	pool    sync.Pool
	newFunc func() T
	reset   func(*T)

	// Metrics
	gets   atomic.Int64
	puts   atomic.Int64
	news   atomic.Int64
	resets atomic.Int64
}

// NewPool creates a new object pool.
func NewPool[T any](newFunc func() T, resetFunc func(*T)) *Pool[T] {
	p := &Pool[T]{
		newFunc: newFunc,
		reset:   resetFunc,
	}
	p.pool.New = func() any {
		p.news.Add(1)
		return newFunc()
	}
	return p
}

// Get retrieves an object from the pool.
func (p *Pool[T]) Get() T {
	p.gets.Add(1)
	return p.pool.Get().(T)
}

// Put returns an object to the pool.
func (p *Pool[T]) Put(obj T) {
	p.puts.Add(1)
	if p.reset != nil {
		p.resets.Add(1)
		p.reset(&obj)
	}
	p.pool.Put(obj)
}

// Stats returns pool statistics.
func (p *Pool[T]) Stats() PoolStats {
	return PoolStats{
		Gets:   p.gets.Load(),
		Puts:   p.puts.Load(),
		News:   p.news.Load(),
		Resets: p.resets.Load(),
	}
}

// PoolStats contains pool statistics.
type PoolStats struct {
	Gets   int64 `json:"gets"`
	Puts   int64 `json:"puts"`
	News   int64 `json:"news"`
	Resets int64 `json:"resets"`
}

// HitRate returns the cache hit rate.
func (s PoolStats) HitRate() float64 {
	if s.Gets == 0 {
		return 0
	}
	return float64(s.Gets-s.News) / float64(s.Gets)
}

// Pre-configured pools for common types

// ByteBufferPool provides pooled byte buffers.
var ByteBufferPool = NewPool(
	func() *bytes.Buffer {
		return bytes.NewBuffer(make([]byte, 0, 4096))
	},
	func(b **bytes.Buffer) {
		(*b).Reset()
	},
)

// SlicePool provides pooled slices.
type SlicePool[T any] struct {
	pool     sync.Pool
	initSize int
}

// NewSlicePool creates a new slice pool.
func NewSlicePool[T any](initSize int) *SlicePool[T] {
	return &SlicePool[T]{
		initSize: initSize,
		pool: sync.Pool{
			New: func() any {
				return make([]T, 0, initSize)
			},
		},
	}
}

// Get retrieves a slice from the pool.
func (p *SlicePool[T]) Get() []T {
	return p.pool.Get().([]T)
}

// Put returns a slice to the pool.
func (p *SlicePool[T]) Put(s []T) {
	s = s[:0] // Reset length but keep capacity
	p.pool.Put(s)
}

// MapPool provides pooled maps.
type MapPool[K comparable, V any] struct {
	pool     sync.Pool
	initSize int
}

// NewMapPool creates a new map pool.
func NewMapPool[K comparable, V any](initSize int) *MapPool[K, V] {
	return &MapPool[K, V]{
		initSize: initSize,
		pool: sync.Pool{
			New: func() any {
				return make(map[K]V, initSize)
			},
		},
	}
}

// Get retrieves a map from the pool.
func (p *MapPool[K, V]) Get() map[K]V {
	return p.pool.Get().(map[K]V)
}

// Put returns a map to the pool.
func (p *MapPool[K, V]) Put(m map[K]V) {
	clear(m)
	p.pool.Put(m)
}

// MessagePool provides pooled LLM message objects.
type MessagePool struct {
	pool sync.Pool
}

// Message represents a poolable LLM message.
type Message struct {
	Role       string         `json:"role"`
	Content    string         `json:"content"`
	ToolCalls  []ToolCall     `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// ToolCall represents a tool call in a message.
type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// NewMessagePool creates a new message pool.
func NewMessagePool() *MessagePool {
	return &MessagePool{
		pool: sync.Pool{
			New: func() any {
				return &Message{
					ToolCalls: make([]ToolCall, 0, 4),
					Metadata:  make(map[string]any, 4),
				}
			},
		},
	}
}

// Get retrieves a message from the pool.
func (p *MessagePool) Get() *Message {
	return p.pool.Get().(*Message)
}

// Put returns a message to the pool.
func (p *MessagePool) Put(m *Message) {
	m.Role = ""
	m.Content = ""
	m.ToolCalls = m.ToolCalls[:0]
	m.ToolCallID = ""
	clear(m.Metadata)
	p.pool.Put(m)
}

// RequestPool provides pooled LLM request objects.
type RequestPool struct {
	pool    sync.Pool
	msgPool *MessagePool
}

// Request represents a poolable LLM request.
type Request struct {
	Model       string     `json:"model"`
	Messages    []*Message `json:"messages"`
	Temperature float64    `json:"temperature,omitempty"`
	MaxTokens   int        `json:"max_tokens,omitempty"`
	Stream      bool       `json:"stream,omitempty"`
}

// NewRequestPool creates a new request pool.
func NewRequestPool(msgPool *MessagePool) *RequestPool {
	return &RequestPool{
		msgPool: msgPool,
		pool: sync.Pool{
			New: func() any {
				return &Request{
					Messages: make([]*Message, 0, 16),
				}
			},
		},
	}
}

// Get retrieves a request from the pool.
func (p *RequestPool) Get() *Request {
	return p.pool.Get().(*Request)
}

// Put returns a request to the pool.
func (p *RequestPool) Put(r *Request) {
	// Return messages to their pool
	for _, msg := range r.Messages {
		p.msgPool.Put(msg)
	}
	r.Model = ""
	r.Messages = r.Messages[:0]
	r.Temperature = 0
	r.MaxTokens = 0
	r.Stream = false
	p.pool.Put(r)
}

// Global pools for common use
var (
	GlobalMessagePool = NewMessagePool()
	GlobalRequestPool = NewRequestPool(GlobalMessagePool)
	GlobalStringSlice = NewSlicePool[string](16)
	GlobalAnyMap      = NewMapPool[string, any](8)
)
