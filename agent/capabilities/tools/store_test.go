package tools

import (
	"context"
	"testing"

	"github.com/BaSui01/agentflow/agent/execution/protocol/a2a"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInMemoryRegistryStore_SaveAndLoad(t *testing.T) {
	store := NewInMemoryRegistryStore()
	ctx := context.Background()

	card := a2a.NewAgentCard("store-agent", "Store Agent", "http://localhost:8080", "1.0.0")
	info := &AgentInfo{Card: card, Status: AgentStatusOnline}

	err := store.Save(ctx, info)
	require.NoError(t, err)

	loaded, err := store.Load(ctx, "store-agent")
	require.NoError(t, err)
	assert.Equal(t, "store-agent", loaded.Card.Name)
}

func TestInMemoryRegistryStore_Save_NilAgent(t *testing.T) {
	store := NewInMemoryRegistryStore()
	ctx := context.Background()

	err := store.Save(ctx, nil)
	assert.Error(t, err)

	err = store.Save(ctx, &AgentInfo{Card: nil})
	assert.Error(t, err)
}

func TestInMemoryRegistryStore_Load_NotFound(t *testing.T) {
	store := NewInMemoryRegistryStore()
	ctx := context.Background()

	_, err := store.Load(ctx, "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestInMemoryRegistryStore_LoadAll(t *testing.T) {
	store := NewInMemoryRegistryStore()
	ctx := context.Background()

	// Empty store
	all, err := store.LoadAll(ctx)
	require.NoError(t, err)
	assert.Empty(t, all)

	// Add agents
	for _, name := range []string{"a1", "a2"} {
		card := a2a.NewAgentCard(name, name, "http://localhost:8080", "1.0.0")
		require.NoError(t, store.Save(ctx, &AgentInfo{Card: card}))
	}

	all, err = store.LoadAll(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 2)
}

func TestInMemoryRegistryStore_Delete(t *testing.T) {
	store := NewInMemoryRegistryStore()
	ctx := context.Background()

	card := a2a.NewAgentCard("del-agent", "Del Agent", "http://localhost:8080", "1.0.0")
	require.NoError(t, store.Save(ctx, &AgentInfo{Card: card}))

	err := store.Delete(ctx, "del-agent")
	require.NoError(t, err)

	_, err = store.Load(ctx, "del-agent")
	assert.Error(t, err)
}

func TestInMemoryRegistryStore_Delete_NotFound(t *testing.T) {
	store := NewInMemoryRegistryStore()
	ctx := context.Background()

	err := store.Delete(ctx, "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestInMemoryRegistryStore_SaveOverwrite(t *testing.T) {
	store := NewInMemoryRegistryStore()
	ctx := context.Background()

	card := a2a.NewAgentCard("overwrite-agent", "V1", "http://localhost:8080", "1.0.0")
	require.NoError(t, store.Save(ctx, &AgentInfo{Card: card, Status: AgentStatusOnline}))

	card2 := a2a.NewAgentCard("overwrite-agent", "V2", "http://localhost:9090", "2.0.0")
	require.NoError(t, store.Save(ctx, &AgentInfo{Card: card2, Status: AgentStatusBusy}))

	loaded, err := store.Load(ctx, "overwrite-agent")
	require.NoError(t, err)
	assert.Equal(t, AgentStatusBusy, loaded.Status)
}
