package llm

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// NoOp extension implementations
// =============================================================================

func TestNoOpSecurityProvider(t *testing.T) {
	sp := &NoOpSecurityProvider{}

	t.Run("Authenticate returns anonymous", func(t *testing.T) {
		id, err := sp.Authenticate(context.Background(), nil)
		require.NoError(t, err)
		assert.Equal(t, "anonymous", id.ID)
		assert.Equal(t, "user", id.Type)
	})

	t.Run("Authorize always succeeds", func(t *testing.T) {
		err := sp.Authorize(context.Background(), &Identity{ID: "x"}, "res", "act")
		assert.NoError(t, err)
	})
}

func TestNoOpAuditLogger(t *testing.T) {
	al := &NoOpAuditLogger{}

	t.Run("Log succeeds", func(t *testing.T) {
		err := al.Log(context.Background(), AuditEvent{EventType: "test"})
		assert.NoError(t, err)
	})

	t.Run("Query returns empty", func(t *testing.T) {
		events, err := al.Query(context.Background(), AuditFilter{})
		require.NoError(t, err)
		assert.Empty(t, events)
	})
}

func TestNoOpRateLimiter(t *testing.T) {
	rl := &NoOpRateLimiter{}

	t.Run("Allow returns true", func(t *testing.T) {
		ok, err := rl.Allow(context.Background(), "key")
		require.NoError(t, err)
		assert.True(t, ok)
	})

	t.Run("AllowN returns true", func(t *testing.T) {
		ok, err := rl.AllowN(context.Background(), "key", 100)
		require.NoError(t, err)
		assert.True(t, ok)
	})

	t.Run("Reset succeeds", func(t *testing.T) {
		err := rl.Reset(context.Background(), "key")
		assert.NoError(t, err)
	})
}

func TestNoOpTracer(t *testing.T) {
	tracer := &NoOpTracer{}
	ctx, span := tracer.StartSpan(context.Background(), "test-span")
	assert.NotNil(t, ctx)
	assert.NotNil(t, span)

	// NoOpSpan methods should not panic
	span.SetAttribute("key", "value")
	span.AddEvent("event", map[string]any{"k": "v"})
	span.SetError(fmt.Errorf("test"))
	span.End()
}

// =============================================================================
// ProviderMiddleware / ProviderMiddlewareFunc / ChainProviderMiddleware
// =============================================================================

func TestProviderMiddlewareFunc_Wrap(t *testing.T) {
	called := false
	mw := ProviderMiddlewareFunc(func(p Provider) Provider {
		called = true
		return p
	})

	inner := &testProvider{name: "inner"}
	result := mw.Wrap(inner)
	assert.True(t, called)
	assert.Equal(t, "inner", result.Name())
}

func TestChainProviderMiddleware(t *testing.T) {
	var order []string

	mw1 := ProviderMiddlewareFunc(func(p Provider) Provider {
		order = append(order, "mw1")
		return &testProvider{name: "mw1-" + p.Name()}
	})
	mw2 := ProviderMiddlewareFunc(func(p Provider) Provider {
		order = append(order, "mw2")
		return &testProvider{name: "mw2-" + p.Name()}
	})

	inner := &testProvider{name: "base"}
	result := ChainProviderMiddleware(inner, mw1, mw2)

	// Middlewares applied in reverse order: mw2 first, then mw1
	assert.Equal(t, []string{"mw2", "mw1"}, order)
	assert.Equal(t, "mw1-mw2-base", result.Name())
}

func TestChainProviderMiddleware_Empty(t *testing.T) {
	inner := &testProvider{name: "base"}
	result := ChainProviderMiddleware(inner)
	assert.Equal(t, "base", result.Name())
}


