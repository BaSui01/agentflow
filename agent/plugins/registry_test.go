package plugins

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- mock plugin ---

type mockPlugin struct {
	name          string
	version       string
	initErr       error
	shutdownErr   error
	initCalls     atomic.Int32
	shutdownCalls atomic.Int32
}

func newMockPlugin(name, version string) *mockPlugin {
	return &mockPlugin{name: name, version: version}
}

func (m *mockPlugin) Name() string    { return m.name }
func (m *mockPlugin) Version() string { return m.version }

func (m *mockPlugin) Init(ctx context.Context) error {
	m.initCalls.Add(1)
	return m.initErr
}

func (m *mockPlugin) Shutdown(ctx context.Context) error {
	m.shutdownCalls.Add(1)
	return m.shutdownErr
}

// --- helpers ---

func newTestRegistry(t *testing.T) *InMemoryPluginRegistry {
	t.Helper()
	return NewInMemoryPluginRegistry(nil)
}

func meta(name, version string, tags ...string) PluginMetadata {
	return PluginMetadata{Name: name, Version: version, Tags: tags}
}

// --- interface compliance ---

func TestInMemoryPluginRegistry_ImplementsPluginRegistry(t *testing.T) {
	var _ PluginRegistry = (*InMemoryPluginRegistry)(nil)
}

// --- constructor ---

func TestNewInMemoryPluginRegistry(t *testing.T) {
	tests := []struct {
		name      string
		nilLogger bool
	}{
		{name: "with nil logger", nilLogger: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewInMemoryPluginRegistry(nil)
			require.NotNil(t, r)
			assert.Empty(t, r.List())
		})
	}
}

// --- Register ---

func TestInMemoryPluginRegistry_Register(t *testing.T) {
	tests := []struct {
		name    string
		plugin  Plugin
		meta    PluginMetadata
		wantErr string
	}{
		{
			name:   "success",
			plugin: newMockPlugin("foo", "1.0"),
			meta:   meta("foo", "1.0"),
		},
		{
			name:    "nil plugin",
			plugin:  nil,
			meta:    meta("x", "1.0"),
			wantErr: "plugin must not be nil",
		},
		{
			name:    "empty name",
			plugin:  newMockPlugin("", "1.0"),
			meta:    meta("", "1.0"),
			wantErr: "plugin name must not be empty",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestRegistry(t)
			err := r.Register(tt.plugin, tt.meta)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			info, ok := r.Get("foo")
			require.True(t, ok)
			assert.Equal(t, PluginStateRegistered, info.State)
		})
	}
}

func TestInMemoryPluginRegistry_Register_Duplicate(t *testing.T) {
	r := newTestRegistry(t)
	p := newMockPlugin("dup", "1.0")
	require.NoError(t, r.Register(p, meta("dup", "1.0")))

	err := r.Register(p, meta("dup", "1.0"))
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPluginAlreadyRegistered)
}

// --- Unregister ---

func TestInMemoryPluginRegistry_Unregister(t *testing.T) {
	tests := []struct {
		name         string
		setup        func(*InMemoryPluginRegistry)
		target       string
		wantErr      string
		wantSentinel error
	}{
		{
			name: "unregister registered plugin",
			setup: func(r *InMemoryPluginRegistry) {
				_ = r.Register(newMockPlugin("a", "1.0"), meta("a", "1.0"))
			},
			target: "a",
		},
		{
			name:         "not found",
			setup:        func(r *InMemoryPluginRegistry) {},
			target:       "missing",
			wantErr:      "plugin not found",
			wantSentinel: ErrPluginNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestRegistry(t)
			tt.setup(r)
			err := r.Unregister(context.Background(), tt.target)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantSentinel)
				return
			}
			require.NoError(t, err)
			_, ok := r.Get(tt.target)
			assert.False(t, ok)
		})
	}
}

func TestInMemoryPluginRegistry_Unregister_InitializedPlugin(t *testing.T) {
	r := newTestRegistry(t)
	p := newMockPlugin("x", "1.0")
	require.NoError(t, r.Register(p, meta("x", "1.0")))
	require.NoError(t, r.Init(context.Background(), "x"))

	require.NoError(t, r.Unregister(context.Background(), "x"))
	assert.Equal(t, int32(1), p.shutdownCalls.Load())
	_, ok := r.Get("x")
	assert.False(t, ok)
}

// --- Init ---

func TestInMemoryPluginRegistry_Init(t *testing.T) {
	tests := []struct {
		name      string
		initErr   error
		wantState PluginState
		wantErr   bool
	}{
		{
			name:      "success",
			wantState: PluginStateInitialized,
		},
		{
			name:      "init error sets failed state",
			initErr:   errors.New("boom"),
			wantState: PluginStateFailed,
			wantErr:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestRegistry(t)
			p := newMockPlugin("p", "1.0")
			p.initErr = tt.initErr
			require.NoError(t, r.Register(p, meta("p", "1.0")))

			err := r.Init(context.Background(), "p")
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			info, ok := r.Get("p")
			require.True(t, ok)
			assert.Equal(t, tt.wantState, info.State)
		})
	}
}

func TestInMemoryPluginRegistry_Init_AlreadyInitialized(t *testing.T) {
	r := newTestRegistry(t)
	p := newMockPlugin("p", "1.0")
	require.NoError(t, r.Register(p, meta("p", "1.0")))
	require.NoError(t, r.Init(context.Background(), "p"))

	// Second init should be a no-op
	require.NoError(t, r.Init(context.Background(), "p"))
	assert.Equal(t, int32(1), p.initCalls.Load())
}

func TestInMemoryPluginRegistry_Init_NotFound(t *testing.T) {
	r := newTestRegistry(t)
	err := r.Init(context.Background(), "nope")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPluginNotFound)
}

// --- InitAll ---

func TestInMemoryPluginRegistry_InitAll(t *testing.T) {
	r := newTestRegistry(t)
	p1 := newMockPlugin("a", "1.0")
	p2 := newMockPlugin("b", "1.0")
	p2.initErr = errors.New("fail-b")
	p3 := newMockPlugin("c", "1.0")

	require.NoError(t, r.Register(p1, meta("a", "1.0")))
	require.NoError(t, r.Register(p2, meta("b", "1.0")))
	require.NoError(t, r.Register(p3, meta("c", "1.0")))

	err := r.InitAll(context.Background())
	// Should report the error from p2 but still init p1 and p3
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fail-b")

	info1, _ := r.Get("a")
	assert.Equal(t, PluginStateInitialized, info1.State)
	info2, _ := r.Get("b")
	assert.Equal(t, PluginStateFailed, info2.State)
	info3, _ := r.Get("c")
	assert.Equal(t, PluginStateInitialized, info3.State)
}

func TestInMemoryPluginRegistry_InitAll_AllSuccess(t *testing.T) {
	r := newTestRegistry(t)
	require.NoError(t, r.Register(newMockPlugin("x", "1.0"), meta("x", "1.0")))
	require.NoError(t, r.Register(newMockPlugin("y", "1.0"), meta("y", "1.0")))

	require.NoError(t, r.InitAll(context.Background()))
}

// --- ShutdownAll ---

func TestInMemoryPluginRegistry_ShutdownAll(t *testing.T) {
	r := newTestRegistry(t)
	p1 := newMockPlugin("a", "1.0")
	p2 := newMockPlugin("b", "1.0")
	p2.shutdownErr = errors.New("shutdown-fail")

	require.NoError(t, r.Register(p1, meta("a", "1.0")))
	require.NoError(t, r.Register(p2, meta("b", "1.0")))
	require.NoError(t, r.InitAll(context.Background()))

	err := r.ShutdownAll(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "shutdown-fail")

	info1, _ := r.Get("a")
	assert.Equal(t, PluginStateShutdown, info1.State)
	// p2 stays initialized because shutdown failed
	info2, _ := r.Get("b")
	assert.Equal(t, PluginStateInitialized, info2.State)
}

// --- List ---

func TestInMemoryPluginRegistry_List(t *testing.T) {
	r := newTestRegistry(t)
	require.NoError(t, r.Register(newMockPlugin("charlie", "1.0"), meta("charlie", "1.0")))
	require.NoError(t, r.Register(newMockPlugin("alpha", "1.0"), meta("alpha", "1.0")))
	require.NoError(t, r.Register(newMockPlugin("bravo", "1.0"), meta("bravo", "1.0")))

	list := r.List()
	require.Len(t, list, 3)
	assert.Equal(t, "alpha", list[0].Metadata.Name)
	assert.Equal(t, "bravo", list[1].Metadata.Name)
	assert.Equal(t, "charlie", list[2].Metadata.Name)
}

func TestInMemoryPluginRegistry_List_Empty(t *testing.T) {
	r := newTestRegistry(t)
	assert.Empty(t, r.List())
}

// --- Search ---

func TestInMemoryPluginRegistry_Search(t *testing.T) {
	r := newTestRegistry(t)
	require.NoError(t, r.Register(newMockPlugin("p1", "1.0"), meta("p1", "1.0", "llm", "chat")))
	require.NoError(t, r.Register(newMockPlugin("p2", "1.0"), meta("p2", "1.0", "rag")))
	require.NoError(t, r.Register(newMockPlugin("p3", "1.0"), meta("p3", "1.0", "llm", "rag")))

	tests := []struct {
		name      string
		tags      []string
		wantLen   int
		wantNames []string
	}{
		{name: "single tag", tags: []string{"rag"}, wantLen: 2, wantNames: []string{"p2", "p3"}},
		{name: "multiple tags", tags: []string{"chat", "rag"}, wantLen: 3, wantNames: []string{"p1", "p2", "p3"}},
		{name: "no match", tags: []string{"unknown"}, wantLen: 0},
		{name: "empty tags", tags: nil, wantLen: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := r.Search(tt.tags)
			assert.Len(t, results, tt.wantLen)
			if tt.wantNames != nil {
				names := make([]string, len(results))
				for i, info := range results {
					names[i] = info.Metadata.Name
				}
				assert.Equal(t, tt.wantNames, names)
			}
		})
	}
}

// --- Concurrency ---

func TestInMemoryPluginRegistry_ConcurrentAccess(t *testing.T) {
	r := newTestRegistry(t)
	ctx := context.Background()
	const goroutines = 20

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			p := newMockPlugin(
				"plugin-"+string(rune('a'+id%26)),
				"1.0",
			)
			_ = r.Register(p, meta(p.Name(), "1.0", "concurrent"))
			r.Get(p.Name())
			r.List()
			r.Search([]string{"concurrent"})
			_ = r.Init(ctx, p.Name())
			_ = r.Unregister(ctx, p.Name())
		}(i)
	}

	wg.Wait()
	// No panic or data race is the success criterion (run with -race)
}
