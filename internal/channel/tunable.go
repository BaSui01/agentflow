// Package channel provides tunable channel implementations.
package channel

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// TunableConfig configures a tunable channel.
type TunableConfig struct {
	InitialSize  int           `json:"initial_size"`
	MinSize      int           `json:"min_size"`
	MaxSize      int           `json:"max_size"`
	GrowFactor   float64       `json:"grow_factor"`
	ShrinkFactor float64       `json:"shrink_factor"`
	SampleWindow time.Duration `json:"sample_window"`
}

// DefaultTunableConfig returns sensible defaults.
func DefaultTunableConfig() TunableConfig {
	return TunableConfig{
		InitialSize:  64,
		MinSize:      16,
		MaxSize:      4096,
		GrowFactor:   2.0,
		ShrinkFactor: 0.5,
		SampleWindow: 10 * time.Second,
	}
}

// TunableChannel provides a dynamically-sized buffered channel.
type TunableChannel[T any] struct {
	config TunableConfig
	ch     chan T
	mu     sync.RWMutex
	size   int

	// Metrics for auto-tuning
	sends    atomic.Int64
	receives atomic.Int64
	blocks   atomic.Int64
	lastTune time.Time
}

// NewTunableChannel creates a new tunable channel.
func NewTunableChannel[T any](config TunableConfig) *TunableChannel[T] {
	return &TunableChannel[T]{
		config:   config,
		ch:       make(chan T, config.InitialSize),
		size:     config.InitialSize,
		lastTune: time.Now(),
	}
}

// Send sends a value to the channel.
func (tc *TunableChannel[T]) Send(ctx context.Context, v T) error {
	tc.sends.Add(1)

	tc.mu.RLock()
	ch := tc.ch
	tc.mu.RUnlock()

	select {
	case ch <- v:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		tc.blocks.Add(1)
		// Blocking send
		select {
		case ch <- v:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// Receive receives a value from the channel.
func (tc *TunableChannel[T]) Receive(ctx context.Context) (T, error) {
	tc.receives.Add(1)

	tc.mu.RLock()
	ch := tc.ch
	tc.mu.RUnlock()

	select {
	case v := <-ch:
		return v, nil
	case <-ctx.Done():
		var zero T
		return zero, ctx.Err()
	}
}

// TrySend attempts a non-blocking send.
func (tc *TunableChannel[T]) TrySend(v T) bool {
	tc.mu.RLock()
	ch := tc.ch
	tc.mu.RUnlock()

	select {
	case ch <- v:
		tc.sends.Add(1)
		return true
	default:
		tc.blocks.Add(1)
		return false
	}
}

// TryReceive attempts a non-blocking receive.
func (tc *TunableChannel[T]) TryReceive() (T, bool) {
	tc.mu.RLock()
	ch := tc.ch
	tc.mu.RUnlock()

	select {
	case v := <-ch:
		tc.receives.Add(1)
		return v, true
	default:
		var zero T
		return zero, false
	}
}

// Chan returns the underlying channel for select statements.
func (tc *TunableChannel[T]) Chan() <-chan T {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	return tc.ch
}

// Len returns the current number of items in the channel.
func (tc *TunableChannel[T]) Len() int {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	return len(tc.ch)
}

// Cap returns the current capacity.
func (tc *TunableChannel[T]) Cap() int {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	return tc.size
}

// Tune adjusts the channel size based on usage patterns.
func (tc *TunableChannel[T]) Tune() {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	if time.Since(tc.lastTune) < tc.config.SampleWindow {
		return
	}

	blocks := tc.blocks.Swap(0)
	sends := tc.sends.Swap(0)

	if sends == 0 {
		return
	}

	blockRate := float64(blocks) / float64(sends)
	utilization := float64(len(tc.ch)) / float64(tc.size)

	newSize := tc.size

	// Grow if blocking too much
	if blockRate > 0.1 && tc.size < tc.config.MaxSize {
		newSize = int(float64(tc.size) * tc.config.GrowFactor)
		if newSize > tc.config.MaxSize {
			newSize = tc.config.MaxSize
		}
	}

	// Shrink if underutilized
	if utilization < 0.25 && blockRate < 0.01 && tc.size > tc.config.MinSize {
		newSize = int(float64(tc.size) * tc.config.ShrinkFactor)
		if newSize < tc.config.MinSize {
			newSize = tc.config.MinSize
		}
	}

	if newSize != tc.size {
		tc.resize(newSize)
	}

	tc.lastTune = time.Now()
}

func (tc *TunableChannel[T]) resize(newSize int) {
	newCh := make(chan T, newSize)

	// Drain old channel into new
	for {
		select {
		case v := <-tc.ch:
			select {
			case newCh <- v:
			default:
				// New channel full, stop draining
				return
			}
		default:
			tc.ch = newCh
			tc.size = newSize
			return
		}
	}
}

// Stats returns channel statistics.
func (tc *TunableChannel[T]) Stats() TunableChannelStats {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	return TunableChannelStats{
		Size:        tc.size,
		Length:      len(tc.ch),
		Sends:       tc.sends.Load(),
		Receives:    tc.receives.Load(),
		Blocks:      tc.blocks.Load(),
		Utilization: float64(len(tc.ch)) / float64(tc.size),
	}
}

// TunableChannelStats contains channel statistics.
type TunableChannelStats struct {
	Size        int     `json:"size"`
	Length      int     `json:"length"`
	Sends       int64   `json:"sends"`
	Receives    int64   `json:"receives"`
	Blocks      int64   `json:"blocks"`
	Utilization float64 `json:"utilization"`
}

// Close closes the channel.
func (tc *TunableChannel[T]) Close() {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	close(tc.ch)
}
