package reasoning

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockPattern implements ReasoningPattern for testing.
type mockPattern struct {
	name string
}

func (m *mockPattern) Name() string { return m.name }

func (m *mockPattern) Execute(ctx context.Context, task string) (*ReasoningResult, error) {
	return &ReasoningResult{
		Pattern:     m.name,
		Task:        task,
		FinalAnswer: "mock answer",
		Confidence:  1.0,
	}, nil
}

func TestNewPatternRegistry(t *testing.T) {
	t.Parallel()

	reg := NewPatternRegistry()
	require.NotNil(t, reg)
	assert.Empty(t, reg.List(), "new registry should have no patterns")
}

func TestPatternRegistry_Register(t *testing.T) {
	t.Parallel()

	t.Run("registers a pattern successfully", func(t *testing.T) {
		t.Parallel()
		reg := NewPatternRegistry()

		err := reg.Register(&mockPattern{name: "alpha"})
		require.NoError(t, err)

		p, ok := reg.Get("alpha")
		assert.True(t, ok)
		assert.Equal(t, "alpha", p.Name())
	})

	t.Run("duplicate registration returns error", func(t *testing.T) {
		t.Parallel()
		reg := NewPatternRegistry()

		err := reg.Register(&mockPattern{name: "dup"})
		require.NoError(t, err)

		err = reg.Register(&mockPattern{name: "dup"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already registered")
	})
}

func TestPatternRegistry_Get(t *testing.T) {
	t.Parallel()

	reg := NewPatternRegistry()
	_ = reg.Register(&mockPattern{name: "exists"})

	t.Run("returns registered pattern", func(t *testing.T) {
		t.Parallel()
		p, ok := reg.Get("exists")
		assert.True(t, ok)
		assert.Equal(t, "exists", p.Name())
	})

	t.Run("returns false for missing pattern", func(t *testing.T) {
		t.Parallel()
		_, ok := reg.Get("nope")
		assert.False(t, ok)
	})
}

func TestPatternRegistry_List(t *testing.T) {
	t.Parallel()

	reg := NewPatternRegistry()
	_ = reg.Register(&mockPattern{name: "charlie"})
	_ = reg.Register(&mockPattern{name: "alpha"})
	_ = reg.Register(&mockPattern{name: "bravo"})

	names := reg.List()
	assert.Equal(t, []string{"alpha", "bravo", "charlie"}, names, "List should return sorted names")
}

func TestPatternRegistry_Unregister(t *testing.T) {
	t.Parallel()

	t.Run("removes existing pattern", func(t *testing.T) {
		t.Parallel()
		reg := NewPatternRegistry()
		_ = reg.Register(&mockPattern{name: "removeme"})

		ok := reg.Unregister("removeme")
		assert.True(t, ok)

		_, found := reg.Get("removeme")
		assert.False(t, found)
	})

	t.Run("returns false for missing pattern", func(t *testing.T) {
		t.Parallel()
		reg := NewPatternRegistry()

		ok := reg.Unregister("ghost")
		assert.False(t, ok)
	})
}

func TestPatternRegistry_MustGet(t *testing.T) {
	t.Parallel()

	t.Run("returns pattern when present", func(t *testing.T) {
		t.Parallel()
		reg := NewPatternRegistry()
		_ = reg.Register(&mockPattern{name: "safe"})

		p := reg.MustGet("safe")
		assert.Equal(t, "safe", p.Name())
	})

	t.Run("panics on missing pattern", func(t *testing.T) {
		t.Parallel()
		reg := NewPatternRegistry()

		assert.PanicsWithValue(t,
			`reasoning pattern "missing" not registered`,
			func() { reg.MustGet("missing") },
		)
	})
}
