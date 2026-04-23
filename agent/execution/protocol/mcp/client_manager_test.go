package mcp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestMCPClientManager_RegisterAndGet(t *testing.T) {
	mgr := NewMCPClientManager(zap.NewNop())
	ctx := context.Background()

	transport := NewInProcessTransport(&echoHandler{})
	require.NoError(t, mgr.Register(ctx, "server-1", transport))

	client, err := mgr.Get("server-1")
	require.NoError(t, err)
	require.NotNil(t, client)

	tools, err := client.ListTools(ctx)
	require.NoError(t, err)
	assert.Len(t, tools, 1)
}

func TestMCPClientManager_DuplicateRegister(t *testing.T) {
	mgr := NewMCPClientManager(zap.NewNop())
	ctx := context.Background()

	require.NoError(t, mgr.Register(ctx, "dup", NewInProcessTransport(&echoHandler{})))
	err := mgr.Register(ctx, "dup", NewInProcessTransport(&echoHandler{}))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")
}

func TestMCPClientManager_GetNotFound(t *testing.T) {
	mgr := NewMCPClientManager(zap.NewNop())
	_, err := mgr.Get("nonexistent")
	assert.Error(t, err)
}

func TestMCPClientManager_ListServers(t *testing.T) {
	mgr := NewMCPClientManager(zap.NewNop())
	ctx := context.Background()

	require.NoError(t, mgr.Register(ctx, "a", NewInProcessTransport(&echoHandler{})))
	require.NoError(t, mgr.Register(ctx, "b", NewInProcessTransport(&echoHandler{})))

	servers := mgr.ListServers()
	assert.Len(t, servers, 2)
}

func TestMCPClientManager_ListAllTools(t *testing.T) {
	mgr := NewMCPClientManager(zap.NewNop())
	ctx := context.Background()

	require.NoError(t, mgr.Register(ctx, "s1", NewInProcessTransport(&echoHandler{})))
	require.NoError(t, mgr.Register(ctx, "s2", NewInProcessTransport(&echoHandler{})))

	allTools, err := mgr.ListAllTools(ctx)
	require.NoError(t, err)
	assert.Len(t, allTools, 2)
	assert.Len(t, allTools["s1"], 1)
	assert.Len(t, allTools["s2"], 1)
}

func TestMCPClientManager_RemoveAndCloseAll(t *testing.T) {
	mgr := NewMCPClientManager(zap.NewNop())
	ctx := context.Background()

	require.NoError(t, mgr.Register(ctx, "x", NewInProcessTransport(&echoHandler{})))
	require.NoError(t, mgr.Remove("x"))

	_, err := mgr.Get("x")
	assert.Error(t, err)

	require.NoError(t, mgr.Register(ctx, "y", NewInProcessTransport(&echoHandler{})))
	require.NoError(t, mgr.CloseAll())
	assert.Len(t, mgr.ListServers(), 0)
}
