package team

import (
	"context"

	"github.com/BaSui01/agentflow/agent/team/internal/engines/multiagent"
	"go.uber.org/zap"
)

// SharedState is the public state contract accepted by team execution modes
// that exchange intermediate results through input.Context["shared_state"].
type SharedState interface {
	Get(ctx context.Context, key string) (any, bool)
	Set(ctx context.Context, key string, value any) error
	Watch(ctx context.Context, key string) <-chan any
	Snapshot(ctx context.Context) map[string]any
}

// InMemorySharedState is the official in-memory SharedState implementation.
type InMemorySharedState struct {
	inner *multiagent.InMemorySharedState
}

var (
	_ SharedState            = (*InMemorySharedState)(nil)
	_ multiagent.SharedState = (*InMemorySharedState)(nil)
)

func NewInMemorySharedState() *InMemorySharedState {
	return NewInMemorySharedStateWithLogger(zap.NewNop())
}

func NewInMemorySharedStateWithLogger(logger *zap.Logger) *InMemorySharedState {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &InMemorySharedState{inner: multiagent.NewInMemorySharedStateWithLogger(logger)}
}

func (s *InMemorySharedState) Get(ctx context.Context, key string) (any, bool) {
	return s.inner.Get(ctx, key)
}

func (s *InMemorySharedState) Set(ctx context.Context, key string, value any) error {
	return s.inner.Set(ctx, key, value)
}

func (s *InMemorySharedState) Watch(ctx context.Context, key string) <-chan any {
	return s.inner.Watch(ctx, key)
}

func (s *InMemorySharedState) Snapshot(ctx context.Context) map[string]any {
	return s.inner.Snapshot(ctx)
}
