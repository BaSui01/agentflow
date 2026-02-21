package workflow

import (
	"fmt"
	"sync"
)

// Reducer defines how to merge state updates from multiple nodes.
type Reducer[T any] func(current T, update T) T

// ChannelReader is a non-generic interface for reading channel values.
// This is needed because Go's type system does not allow matching
// generic method signatures like Get() T via interface{ Get() any }.
type ChannelReader interface {
	GetAny() any
	Version() uint64
}

// Channel represents a state channel with optional reducer.
type Channel[T any] struct {
	name    string
	value   T
	reducer Reducer[T]
	mu      sync.RWMutex
	version uint64
	history []T
	maxHist int
}

// ChannelOption configures a channel.
type ChannelOption[T any] func(*Channel[T])

// WithReducer sets a custom reducer for the channel.
func WithReducer[T any](r Reducer[T]) ChannelOption[T] {
	return func(c *Channel[T]) {
		c.reducer = r
	}
}

// WithHistory enables history tracking with max entries.
func WithHistory[T any](max int) ChannelOption[T] {
	return func(c *Channel[T]) {
		c.maxHist = max
		c.history = make([]T, 0, max)
	}
}

// NewChannel creates a new state channel.
func NewChannel[T any](name string, initial T, opts ...ChannelOption[T]) *Channel[T] {
	c := &Channel[T]{
		name:  name,
		value: initial,
		reducer: func(_, update T) T {
			return update // Default: last-write-wins
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Get returns the current value.
func (c *Channel[T]) Get() T {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.value
}

// Update applies an update using the reducer.
func (c *Channel[T]) Update(update T) T {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.maxHist > 0 {
		if len(c.history) >= c.maxHist {
			c.history = c.history[1:]
		}
		c.history = append(c.history, c.value)
	}

	c.value = c.reducer(c.value, update)
	c.version++
	return c.value
}

// GetAny returns the current value as any, implementing ChannelReader.
func (c *Channel[T]) GetAny() any {
	return c.Get()
}

// Version returns the current version number.
func (c *Channel[T]) Version() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.version
}

// History returns the value history.
func (c *Channel[T]) History() []T {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]T, len(c.history))
	copy(result, c.history)
	return result
}

// Built-in reducers

// LastValueReducer returns the most recent value (default).
func LastValueReducer[T any]() Reducer[T] {
	return func(_, update T) T {
		return update
	}
}

// AppendReducer appends slices together.
func AppendReducer[T any]() Reducer[[]T] {
	return func(current, update []T) []T {
		result := make([]T, 0, len(current)+len(update))
		result = append(result, current...)
		result = append(result, update...)
		return result
	}
}

// MergeMapReducer merges maps, with update values taking precedence.
func MergeMapReducer[K comparable, V any]() Reducer[map[K]V] {
	return func(current, update map[K]V) map[K]V {
		result := make(map[K]V, len(current)+len(update))
		for k, v := range current {
			result[k] = v
		}
		for k, v := range update {
			result[k] = v
		}
		return result
	}
}

// SumReducer sums numeric values.
func SumReducer[T ~int | ~int64 | ~float64]() Reducer[T] {
	return func(current, update T) T {
		return current + update
	}
}

// MaxReducer keeps the maximum value.
func MaxReducer[T ~int | ~int64 | ~float64]() Reducer[T] {
	return func(current, update T) T {
		if update > current {
			return update
		}
		return current
	}
}

// StateGraph manages multiple channels as a unified state.
type StateGraph struct {
	channels map[string]any
	mu       sync.RWMutex
}

// NewStateGraph creates a new state graph.
func NewStateGraph() *StateGraph {
	return &StateGraph{
		channels: make(map[string]any),
	}
}

// RegisterChannel registers a channel with the state graph.
func RegisterChannel[T any](sg *StateGraph, channel *Channel[T]) {
	sg.mu.Lock()
	defer sg.mu.Unlock()
	sg.channels[channel.name] = channel
}

// GetChannel retrieves a typed channel by name.
func GetChannel[T any](sg *StateGraph, name string) (*Channel[T], error) {
	sg.mu.RLock()
	defer sg.mu.RUnlock()

	ch, ok := sg.channels[name]
	if !ok {
		return nil, fmt.Errorf("channel not found: %s", name)
	}

	typed, ok := ch.(*Channel[T])
	if !ok {
		return nil, fmt.Errorf("channel type mismatch: %s", name)
	}

	return typed, nil
}

// Annotation provides type-safe state definition.
type Annotation[T any] struct {
	Name    string
	Default T
	Reducer Reducer[T]
}

// NewAnnotation creates a new annotation.
func NewAnnotation[T any](name string, defaultVal T, reducer Reducer[T]) Annotation[T] {
	return Annotation[T]{
		Name:    name,
		Default: defaultVal,
		Reducer: reducer,
	}
}

// CreateChannel creates a channel from an annotation.
func (a Annotation[T]) CreateChannel() *Channel[T] {
	opts := []ChannelOption[T]{}
	if a.Reducer != nil {
		opts = append(opts, WithReducer(a.Reducer))
	}
	return NewChannel(a.Name, a.Default, opts...)
}

// StateSnapshot captures the current state of all channels.
type StateSnapshot struct {
	Values   map[string]any    `json:"values"`
	Versions map[string]uint64 `json:"versions"`
}

// Snapshot creates a snapshot of the current state.
func (sg *StateGraph) Snapshot() StateSnapshot {
	sg.mu.RLock()
	defer sg.mu.RUnlock()

	snapshot := StateSnapshot{
		Values:   make(map[string]any),
		Versions: make(map[string]uint64),
	}

	for name, ch := range sg.channels {
		if cr, ok := ch.(ChannelReader); ok {
			snapshot.Values[name] = cr.GetAny()
			snapshot.Versions[name] = cr.Version()
		}
	}

	return snapshot
}

// NodeOutput represents output from a graph node.
type NodeOutput struct {
	Updates map[string]any `json:"updates"`
	NodeID  string         `json:"node_id"`
}

// ApplyNodeOutput applies a node's output to the state graph.
func (sg *StateGraph) ApplyNodeOutput(output NodeOutput) error {
	sg.mu.Lock()
	defer sg.mu.Unlock()

	for name, update := range output.Updates {
		ch, ok := sg.channels[name]
		if !ok {
			continue // Skip unknown channels
		}

		// Use type switch for common types
		switch c := ch.(type) {
		case *Channel[string]:
			if v, ok := update.(string); ok {
				c.Update(v)
			}
		case *Channel[int]:
			if v, ok := update.(int); ok {
				c.Update(v)
			}
		case *Channel[[]string]:
			if v, ok := update.([]string); ok {
				c.Update(v)
			}
		case *Channel[map[string]any]:
			if v, ok := update.(map[string]any); ok {
				c.Update(v)
			}
		}
	}

	return nil
}
