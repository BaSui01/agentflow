package collaboration

import (
	"context"
	"sync"
)

type SharedState interface {
	Get(ctx context.Context, key string) (any, bool)
	Set(ctx context.Context, key string, value any) error
	Watch(ctx context.Context, key string) <-chan any
	Snapshot(ctx context.Context) map[string]any
}

type InMemorySharedState struct {
	data     map[string]any
	watchers map[string][]chan any
	mu       sync.RWMutex
}

func NewInMemorySharedState() *InMemorySharedState {
	return &InMemorySharedState{
		data:     make(map[string]any),
		watchers: make(map[string][]chan any),
	}
}

func (s *InMemorySharedState) Get(ctx context.Context, key string) (any, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.data[key]
	return v, ok
}

func (s *InMemorySharedState) Set(ctx context.Context, key string, value any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = value
	for _, ch := range s.watchers[key] {
		select {
		case ch <- value:
		case <-ctx.Done():
		default:
		}
	}
	return nil
}

func (s *InMemorySharedState) Watch(ctx context.Context, key string) <-chan any {
	ch := make(chan any, 1)
	s.mu.Lock()
	s.watchers[key] = append(s.watchers[key], ch)
	if v, ok := s.data[key]; ok {
		select {
		case ch <- v:
		default:
		}
	}
	s.mu.Unlock()
	return ch
}

func (s *InMemorySharedState) Snapshot(ctx context.Context) map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]any, len(s.data))
	for k, v := range s.data {
		out[k] = v
	}
	return out
}
