package agent

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- test helpers for plugin tests ---

// stubAgent is a minimal Agent implementation for testing.
type stubAgent struct {
	executeFn func(ctx context.Context, input *Input) (*Output, error)
}

func (s *stubAgent) ID() string        { return "stub" }
func (s *stubAgent) Name() string      { return "stub-agent" }
func (s *stubAgent) Type() AgentType   { return TypeGeneric }
func (s *stubAgent) State() State      { return StateReady }
func (s *stubAgent) Init(context.Context) error { return nil }
func (s *stubAgent) Teardown(context.Context) error { return nil }
func (s *stubAgent) Plan(context.Context, *Input) (*PlanResult, error) { return nil, nil }
func (s *stubAgent) Execute(ctx context.Context, input *Input) (*Output, error) {
	if s.executeFn != nil {
		return s.executeFn(ctx, input)
	}
	return &Output{Content: "base"}, nil
}
func (s *stubAgent) Observe(context.Context, *Feedback) error { return nil }

// stubMiddlewarePlugin is a minimal MiddlewarePlugin for testing.
type stubMiddlewarePlugin struct {
	name    string
	wrapFn  func(next func(ctx context.Context, input *Input) (*Output, error)) func(ctx context.Context, input *Input) (*Output, error)
}

func (p *stubMiddlewarePlugin) Name() string       { return p.name }
func (p *stubMiddlewarePlugin) Type() PluginType   { return PluginTypeMiddleware }
func (p *stubMiddlewarePlugin) Init(context.Context) error { return nil }
func (p *stubMiddlewarePlugin) Shutdown(context.Context) error { return nil }
func (p *stubMiddlewarePlugin) Wrap(next func(ctx context.Context, input *Input) (*Output, error)) func(ctx context.Context, input *Input) (*Output, error) {
	if p.wrapFn != nil {
		return p.wrapFn(next)
	}
	return next
}

// TestPluginEnabledAgent_Execute_MiddlewareCached verifies that the
// middleware slice is captured once, not re-fetched on every iteration.
// Before the fix, each loop iteration called MiddlewarePlugins() again,
// which could return a different slice under concurrent mutation.
func TestPluginEnabledAgent_Execute_MiddlewareCached(t *testing.T) {
	t.Parallel()

	var callCount atomic.Int32

	mw := &stubMiddlewarePlugin{
		name: "counter",
		wrapFn: func(next func(ctx context.Context, input *Input) (*Output, error)) func(ctx context.Context, input *Input) (*Output, error) {
			callCount.Add(1)
			return next
		},
	}

	registry := NewPluginRegistry()
	require.NoError(t, registry.Register(mw))

	agent := &stubAgent{}
	pea := NewPluginEnabledAgent(agent, registry)

	ctx := context.Background()
	out, err := pea.Execute(ctx, &Input{Content: "hello"})
	require.NoError(t, err)
	assert.Equal(t, "base", out.Content)
	assert.Equal(t, int32(1), callCount.Load(), "middleware Wrap should be called exactly once")
}

// TestPluginEnabledAgent_Execute_ConcurrentRegistration exercises the
// scenario where plugins are registered/unregistered concurrently with
// Execute. Before the fix, this could cause an index-out-of-range panic
// because MiddlewarePlugins() was called twice per iteration.
func TestPluginEnabledAgent_Execute_ConcurrentRegistration(t *testing.T) {
	t.Parallel()

	registry := NewPluginRegistry()
	agent := &stubAgent{}
	pea := NewPluginEnabledAgent(agent, registry)

	ctx := context.Background()

	// Pre-register some middleware
	for i := 0; i < 5; i++ {
		mw := &stubMiddlewarePlugin{name: "mw-" + string(rune('A'+i))}
		require.NoError(t, registry.Register(mw))
	}

	var wg sync.WaitGroup

	// Concurrent executions
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = pea.Execute(ctx, &Input{Content: "test"})
		}()
	}

	// Concurrent registrations/unregistrations
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			mw := &stubMiddlewarePlugin{name: "dynamic-" + string(rune('0'+idx))}
			_ = registry.Register(mw)
			_ = registry.Unregister(mw.Name())
		}(i)
	}

	wg.Wait()
}

// TestPluginEnabledAgent_Execute_MiddlewareOrder verifies that middleware
// wrapping order is correct (last registered wraps outermost).
func TestPluginEnabledAgent_Execute_MiddlewareOrder(t *testing.T) {
	t.Parallel()

	var order []string
	var mu sync.Mutex

	makeMW := func(name string) *stubMiddlewarePlugin {
		return &stubMiddlewarePlugin{
			name: name,
			wrapFn: func(next func(ctx context.Context, input *Input) (*Output, error)) func(ctx context.Context, input *Input) (*Output, error) {
				return func(ctx context.Context, input *Input) (*Output, error) {
					mu.Lock()
					order = append(order, name)
					mu.Unlock()
					return next(ctx, input)
				}
			},
		}
	}

	registry := NewPluginRegistry()
	require.NoError(t, registry.Register(makeMW("first")))
	require.NoError(t, registry.Register(makeMW("second")))
	require.NoError(t, registry.Register(makeMW("third")))

	agent := &stubAgent{}
	pea := NewPluginEnabledAgent(agent, registry)

	ctx := context.Background()
	out, err := pea.Execute(ctx, &Input{Content: "hello"})
	require.NoError(t, err)
	assert.Equal(t, "base", out.Content)

	// Middleware is built from the end: the loop wraps from i=len-1 down to 0.
	// So "first" (index 0) is wrapped last, making it the outermost layer.
	// Execution order: first -> second -> third -> base agent.
	assert.Equal(t, []string{"first", "second", "third"}, order)
}
