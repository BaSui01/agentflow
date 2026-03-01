package agent

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// --- LifecycleManager tests ---

func TestNewLifecycleManager(t *testing.T) {
	ma := &mockAgent{id: "lm-1", state: StateInit}
	lm := NewLifecycleManager(ma, zap.NewNop())
	require.NotNil(t, lm)
	assert.False(t, lm.IsRunning())
	assert.False(t, lm.GetHealthStatus().Healthy)
}

func TestLifecycleManager_StartStop(t *testing.T) {
	ma := &mockAgent{id: "lm-2", state: StateInit}
	lm := NewLifecycleManager(ma, zap.NewNop())

	ctx := context.Background()
	err := lm.Start(ctx)
	require.NoError(t, err)
	assert.True(t, lm.IsRunning())

	// Wait for health check to run
	time.Sleep(50 * time.Millisecond)
	status := lm.GetHealthStatus()
	assert.True(t, status.Healthy)
	assert.Equal(t, StateReady, status.State)

	err = lm.Stop(ctx)
	require.NoError(t, err)
	assert.False(t, lm.IsRunning())
}

func TestLifecycleManager_StartAlreadyRunning(t *testing.T) {
	ma := &mockAgent{id: "lm-3", state: StateInit}
	lm := NewLifecycleManager(ma, zap.NewNop())

	ctx := context.Background()
	require.NoError(t, lm.Start(ctx))
	defer func() {
		require.NoError(t, lm.Stop(ctx))
	}()

	err := lm.Start(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already running")
}

func TestLifecycleManager_StopNotRunning(t *testing.T) {
	ma := &mockAgent{id: "lm-4", state: StateInit}
	lm := NewLifecycleManager(ma, zap.NewNop())

	err := lm.Stop(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not running")
}

func TestLifecycleManager_HealthCheckUnhealthy(t *testing.T) {
	ma := &mockAgent{id: "lm-5", state: StateFailed}
	lm := NewLifecycleManager(ma, zap.NewNop())
	lm.healthCheckInterval = 10 * time.Millisecond

	// Manually perform health check
	lm.performHealthCheck()
	status := lm.GetHealthStatus()
	assert.False(t, status.Healthy)
	assert.Contains(t, status.Message, "unhealthy")
}

// --- PluginRegistry tests ---

type testPlugin struct {
	name        string
	initCalled  bool
	shutCalled  bool
	initErr     error
	shutdownErr error
}

func (p *testPlugin) Name() string                       { return p.name }
func (p *testPlugin) Type() PluginType                   { return PluginTypeExtension }
func (p *testPlugin) Init(ctx context.Context) error     { p.initCalled = true; return p.initErr }
func (p *testPlugin) Shutdown(ctx context.Context) error { p.shutCalled = true; return p.shutdownErr }

func TestPluginRegistry_RegisterAndGet(t *testing.T) {
	r := NewPluginRegistry()
	p := &testPlugin{name: "test-plugin"}

	err := r.Register(p)
	require.NoError(t, err)

	got, ok := r.Get("test-plugin")
	assert.True(t, ok)
	assert.Equal(t, p, got)
}

func TestPluginRegistry_RegisterDuplicate(t *testing.T) {
	r := NewPluginRegistry()
	p := &testPlugin{name: "dup"}

	require.NoError(t, r.Register(p))
	err := r.Register(p)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")
}

func TestPluginRegistry_Get_NotFound(t *testing.T) {
	r := NewPluginRegistry()
	_, ok := r.Get("nonexistent")
	assert.False(t, ok)
}

func TestPluginRegistry_List(t *testing.T) {
	r := NewPluginRegistry()
	require.NoError(t, r.Register(&testPlugin{name: "p1"}))
	require.NoError(t, r.Register(&testPlugin{name: "p2"}))

	plugins := r.List()
	assert.Len(t, plugins, 2)
}

func TestPluginRegistry_Init(t *testing.T) {
	r := NewPluginRegistry()
	p1 := &testPlugin{name: "p1"}
	p2 := &testPlugin{name: "p2"}
	require.NoError(t, r.Register(p1))
	require.NoError(t, r.Register(p2))

	err := r.Init(context.Background())
	require.NoError(t, err)
	assert.True(t, p1.initCalled)
	assert.True(t, p2.initCalled)

	// Second init should be no-op
	p1.initCalled = false
	err = r.Init(context.Background())
	require.NoError(t, err)
	assert.False(t, p1.initCalled)
}

func TestPluginRegistry_Shutdown(t *testing.T) {
	r := NewPluginRegistry()
	p1 := &testPlugin{name: "p1"}
	require.NoError(t, r.Register(p1))
	require.NoError(t, r.Init(context.Background()))

	err := r.Shutdown(context.Background())
	require.NoError(t, err)
	assert.True(t, p1.shutCalled)
}

func TestPluginRegistry_Unregister(t *testing.T) {
	r := NewPluginRegistry()
	p := &testPlugin{name: "unreg"}
	require.NoError(t, r.Register(p))

	err := r.Unregister("unreg")
	require.NoError(t, err)

	_, ok := r.Get("unreg")
	assert.False(t, ok)
}

func TestPluginRegistry_Unregister_NotFound(t *testing.T) {
	r := NewPluginRegistry()

	err := r.Unregister("missing")
	assert.Error(t, err)
}
