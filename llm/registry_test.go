package llm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProviderRegistry_RegisterAndGet(t *testing.T) {
	r := NewProviderRegistry()
	p := &testProvider{name: "openai"}

	r.Register("openai", p)
	got, ok := r.Get("openai")
	assert.True(t, ok)
	assert.Equal(t, "openai", got.Name())

	_, ok = r.Get("nonexistent")
	assert.False(t, ok)
}

func TestProviderRegistry_DefaultAndSetDefault(t *testing.T) {
	r := NewProviderRegistry()

	// No default set
	_, err := r.Default()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no default provider set")

	// Set default for unregistered provider
	err = r.SetDefault("missing")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not registered")

	// Register and set default
	r.Register("p1", &testProvider{name: "p1"})
	err = r.SetDefault("p1")
	require.NoError(t, err)

	p, err := r.Default()
	require.NoError(t, err)
	assert.Equal(t, "p1", p.Name())
}

func TestProviderRegistry_List(t *testing.T) {
	r := NewProviderRegistry()
	r.Register("charlie", &testProvider{name: "charlie"})
	r.Register("alpha", &testProvider{name: "alpha"})
	r.Register("bravo", &testProvider{name: "bravo"})

	names := r.List()
	assert.Equal(t, []string{"alpha", "bravo", "charlie"}, names)
}

func TestProviderRegistry_Unregister(t *testing.T) {
	r := NewProviderRegistry()
	r.Register("p1", &testProvider{name: "p1"})
	r.Register("p2", &testProvider{name: "p2"})
	require.NoError(t, r.SetDefault("p1"))

	r.Unregister("p1")
	assert.Equal(t, 1, r.Len())

	// Default should be cleared
	_, err := r.Default()
	require.Error(t, err)

	// Unregister non-default doesn't clear default
	require.NoError(t, r.SetDefault("p2"))
	r.Unregister("nonexistent") // no-op
	p, err := r.Default()
	require.NoError(t, err)
	assert.Equal(t, "p2", p.Name())
}

func TestProviderRegistry_Len(t *testing.T) {
	r := NewProviderRegistry()
	assert.Equal(t, 0, r.Len())
	r.Register("a", &testProvider{name: "a"})
	assert.Equal(t, 1, r.Len())
	r.Register("b", &testProvider{name: "b"})
	assert.Equal(t, 2, r.Len())
}

func TestProviderRegistry_ReplaceExisting(t *testing.T) {
	r := NewProviderRegistry()
	r.Register("p1", &testProvider{name: "old"})
	r.Register("p1", &testProvider{name: "new"})

	p, ok := r.Get("p1")
	assert.True(t, ok)
	assert.Equal(t, "new", p.Name())
	assert.Equal(t, 1, r.Len())
}

