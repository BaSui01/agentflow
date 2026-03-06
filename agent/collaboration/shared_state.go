package collaboration

import (
	"context"
	"sync"

	"go.uber.org/zap"
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
	logger   *zap.Logger
}

func NewInMemorySharedState() *InMemorySharedState {
	return NewInMemorySharedStateWithLogger(zap.NewNop())
}

// NewInMemorySharedStateWithLogger 创建带日志的共享状态，用于记录被跳过的 watcher 通知。
func NewInMemorySharedStateWithLogger(logger *zap.Logger) *InMemorySharedState {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &InMemorySharedState{
		data:     make(map[string]any),
		watchers: make(map[string][]chan any),
		logger:   logger,
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
			s.logger.Warn("watcher channel full, update skipped",
				zap.String("key", key),
				zap.Int("channel_cap", cap(ch)),
			)
		}
	}
	return nil
}

func (s *InMemorySharedState) Watch(ctx context.Context, key string) <-chan any {
	// T-007: buffer 1 避免 Set 时 default 分支跳过导致漏更新
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
