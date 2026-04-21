package discovery

import (
	"context"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/agent/protocol/a2a"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func newTestRegistry(t *testing.T) *CapabilityRegistry {
	t.Helper()
	cfg := DefaultRegistryConfig()
	cfg.EnableHealthCheck = false
	return NewCapabilityRegistry(cfg, zap.NewNop())
}

func registerTestAgent(t *testing.T, reg *CapabilityRegistry, name string, caps []string) {
	t.Helper()
	card := a2a.NewAgentCard(name, "Test", "http://localhost:8080", "1.0.0")
	capInfos := make([]CapabilityInfo, len(caps))
	for i, c := range caps {
		capInfos[i] = CapabilityInfo{
			Capability: a2a.Capability{Name: c, Description: c, Type: a2a.CapabilityTypeTask},
			Status:     CapabilityStatusActive,
			Score:      50.0,
		}
	}
	info := &AgentInfo{Card: card, Status: AgentStatusOnline, IsLocal: true, Capabilities: capInfos}
	require.NoError(t, reg.RegisterAgent(context.Background(), info))
}

func TestCapabilityRegistry_RegisterAgent_NilInfo(t *testing.T) {
	reg := newTestRegistry(t)
	err := reg.RegisterAgent(context.Background(), nil)
	assert.Error(t, err)
}

func TestCapabilityRegistry_RegisterAgent_NilCard(t *testing.T) {
	reg := newTestRegistry(t)
	err := reg.RegisterAgent(context.Background(), &AgentInfo{Card: nil})
	assert.Error(t, err)
}

func TestCapabilityRegistry_RegisterAgent_EmptyName(t *testing.T) {
	reg := newTestRegistry(t)
	card := &a2a.AgentCard{Name: ""}
	err := reg.RegisterAgent(context.Background(), &AgentInfo{Card: card})
	assert.Error(t, err)
}

func TestCapabilityRegistry_WithStore(t *testing.T) {
	store := NewInMemoryRegistryStore()
	cfg := DefaultRegistryConfig()
	cfg.EnableHealthCheck = false
	reg := NewCapabilityRegistry(cfg, zap.NewNop(), WithStore(store))

	ctx := context.Background()
	card := a2a.NewAgentCard("stored-agent", "Stored", "http://localhost:8080", "1.0.0")
	info := &AgentInfo{Card: card, Status: AgentStatusOnline}
	require.NoError(t, reg.RegisterAgent(ctx, info))

	// Verify persisted to store
	loaded, err := store.Load(ctx, "stored-agent")
	require.NoError(t, err)
	assert.Equal(t, "stored-agent", loaded.Card.Name)

	// Unregister should also delete from store
	require.NoError(t, reg.UnregisterAgent(ctx, "stored-agent"))
	_, err = store.Load(ctx, "stored-agent")
	assert.Error(t, err)
}

func TestCapabilityRegistry_UpdateAgent(t *testing.T) {
	reg := newTestRegistry(t)
	ctx := context.Background()
	registerTestAgent(t, reg, "upd-agent", []string{"cap1"})

	// Update with new capabilities
	card := a2a.NewAgentCard("upd-agent", "Updated", "http://localhost:9090", "2.0.0")
	newInfo := &AgentInfo{
		Card:   card,
		Status: AgentStatusOnline,
		Capabilities: []CapabilityInfo{
			{Capability: a2a.Capability{Name: "cap2", Description: "cap2", Type: a2a.CapabilityTypeTask}},
		},
	}
	err := reg.UpdateAgent(ctx, newInfo)
	require.NoError(t, err)

	// Old capability should be gone
	caps, err := reg.FindCapabilities(ctx, "cap1")
	require.NoError(t, err)
	assert.Empty(t, caps)

	// New capability should exist
	caps, err = reg.FindCapabilities(ctx, "cap2")
	require.NoError(t, err)
	assert.Len(t, caps, 1)
}

func TestCapabilityRegistry_UpdateAgent_NotFound(t *testing.T) {
	reg := newTestRegistry(t)
	card := a2a.NewAgentCard("ghost", "Ghost", "http://localhost:8080", "1.0.0")
	err := reg.UpdateAgent(context.Background(), &AgentInfo{Card: card})
	assert.Error(t, err)
}

func TestCapabilityRegistry_UpdateAgent_NilInfo(t *testing.T) {
	reg := newTestRegistry(t)
	err := reg.UpdateAgent(context.Background(), nil)
	assert.Error(t, err)
}

func TestCapabilityRegistry_RegisterCapability(t *testing.T) {
	reg := newTestRegistry(t)
	ctx := context.Background()
	registerTestAgent(t, reg, "cap-agent", []string{})

	cap := &CapabilityInfo{
		Capability: a2a.Capability{Name: "new_cap", Description: "New", Type: a2a.CapabilityTypeTask},
	}
	err := reg.RegisterCapability(ctx, "cap-agent", cap)
	require.NoError(t, err)

	// Duplicate
	err = reg.RegisterCapability(ctx, "cap-agent", cap)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")
}

func TestCapabilityRegistry_RegisterCapability_NilCap(t *testing.T) {
	reg := newTestRegistry(t)
	err := reg.RegisterCapability(context.Background(), "agent", nil)
	assert.Error(t, err)
}

func TestCapabilityRegistry_RegisterCapability_AgentNotFound(t *testing.T) {
	reg := newTestRegistry(t)
	cap := &CapabilityInfo{Capability: a2a.Capability{Name: "x"}}
	err := reg.RegisterCapability(context.Background(), "nonexistent", cap)
	assert.Error(t, err)
}

func TestCapabilityRegistry_UnregisterCapability(t *testing.T) {
	reg := newTestRegistry(t)
	ctx := context.Background()
	registerTestAgent(t, reg, "unreg-cap-agent", []string{"removable"})

	err := reg.UnregisterCapability(ctx, "unreg-cap-agent", "removable")
	require.NoError(t, err)

	caps, err := reg.ListCapabilities(ctx, "unreg-cap-agent")
	require.NoError(t, err)
	assert.Empty(t, caps)
}

func TestCapabilityRegistry_UnregisterCapability_NotFound(t *testing.T) {
	reg := newTestRegistry(t)
	ctx := context.Background()
	registerTestAgent(t, reg, "unreg-cap-agent2", []string{})

	err := reg.UnregisterCapability(ctx, "unreg-cap-agent2", "nonexistent")
	assert.Error(t, err)
}

func TestCapabilityRegistry_UpdateCapability(t *testing.T) {
	reg := newTestRegistry(t)
	ctx := context.Background()
	registerTestAgent(t, reg, "upd-cap-agent", []string{"updatable"})

	cap := &CapabilityInfo{
		Capability: a2a.Capability{Name: "updatable", Description: "Updated", Type: a2a.CapabilityTypeTask},
		Score:      99.0,
	}
	err := reg.UpdateCapability(ctx, "upd-cap-agent", cap)
	require.NoError(t, err)

	got, err := reg.GetCapability(ctx, "upd-cap-agent", "updatable")
	require.NoError(t, err)
	assert.Equal(t, 99.0, got.Score)
}

func TestCapabilityRegistry_UpdateCapability_NotFound(t *testing.T) {
	reg := newTestRegistry(t)
	ctx := context.Background()
	registerTestAgent(t, reg, "upd-cap-agent2", []string{})

	cap := &CapabilityInfo{Capability: a2a.Capability{Name: "ghost"}}
	err := reg.UpdateCapability(ctx, "upd-cap-agent2", cap)
	assert.Error(t, err)
}

func TestCapabilityRegistry_UpdateCapability_NilCap(t *testing.T) {
	reg := newTestRegistry(t)
	err := reg.UpdateCapability(context.Background(), "agent", nil)
	assert.Error(t, err)
}

func TestCapabilityRegistry_ListCapabilities(t *testing.T) {
	reg := newTestRegistry(t)
	ctx := context.Background()
	registerTestAgent(t, reg, "list-cap-agent", []string{"a", "b", "c"})

	caps, err := reg.ListCapabilities(ctx, "list-cap-agent")
	require.NoError(t, err)
	assert.Len(t, caps, 3)
}

func TestCapabilityRegistry_ListCapabilities_AgentNotFound(t *testing.T) {
	reg := newTestRegistry(t)
	_, err := reg.ListCapabilities(context.Background(), "nonexistent")
	assert.Error(t, err)
}

func TestCapabilityRegistry_UpdateAgentStatus(t *testing.T) {
	reg := newTestRegistry(t)
	ctx := context.Background()
	registerTestAgent(t, reg, "status-agent", []string{})

	err := reg.UpdateAgentStatus(ctx, "status-agent", AgentStatusBusy)
	require.NoError(t, err)

	agent, err := reg.GetAgent(ctx, "status-agent")
	require.NoError(t, err)
	assert.Equal(t, AgentStatusBusy, agent.Status)
}

func TestCapabilityRegistry_UpdateAgentStatus_NotFound(t *testing.T) {
	reg := newTestRegistry(t)
	err := reg.UpdateAgentStatus(context.Background(), "ghost", AgentStatusOnline)
	assert.Error(t, err)
}

func TestCapabilityRegistry_UpdateAgentLoad(t *testing.T) {
	reg := newTestRegistry(t)
	ctx := context.Background()
	registerTestAgent(t, reg, "load-agent", []string{"cap1"})

	err := reg.UpdateAgentLoad(ctx, "load-agent", 0.8)
	require.NoError(t, err)
}

func TestCapabilityRegistry_UpdateAgentLoad_NotFound(t *testing.T) {
	reg := newTestRegistry(t)
	err := reg.UpdateAgentLoad(context.Background(), "ghost", 0.5)
	assert.Error(t, err)
}

func TestCapabilityRegistry_GetAgentsByCapability(t *testing.T) {
	reg := newTestRegistry(t)
	ctx := context.Background()
	registerTestAgent(t, reg, "by-cap-1", []string{"shared"})
	registerTestAgent(t, reg, "by-cap-2", []string{"shared"})
	registerTestAgent(t, reg, "by-cap-3", []string{"other"})

	agents, err := reg.GetAgentsByCapability(ctx, "shared")
	require.NoError(t, err)
	assert.Len(t, agents, 2)

	agents, err = reg.GetAgentsByCapability(ctx, "nonexistent")
	require.NoError(t, err)
	assert.Empty(t, agents)
}

func TestCapabilityRegistry_GetActiveAgents(t *testing.T) {
	reg := newTestRegistry(t)
	ctx := context.Background()
	registerTestAgent(t, reg, "active-1", []string{})
	registerTestAgent(t, reg, "active-2", []string{})

	// Mark one as unhealthy
	require.NoError(t, reg.UpdateAgentStatus(ctx, "active-2", AgentStatusUnhealthy))

	agents, err := reg.GetActiveAgents(ctx)
	require.NoError(t, err)
	assert.Len(t, agents, 1)
	assert.Equal(t, "active-1", agents[0].Card.Name)
}

func TestCapabilityRegistry_Heartbeat(t *testing.T) {
	reg := newTestRegistry(t)
	ctx := context.Background()
	registerTestAgent(t, reg, "hb-agent", []string{})

	err := reg.Heartbeat(ctx, "hb-agent")
	require.NoError(t, err)

	err = reg.Heartbeat(ctx, "nonexistent")
	assert.Error(t, err)
}

func TestCapabilityRegistry_RecordExecution_NotFoundCapability(t *testing.T) {
	reg := newTestRegistry(t)
	ctx := context.Background()
	registerTestAgent(t, reg, "rec-agent", []string{"existing"})

	err := reg.RecordExecution(ctx, "rec-agent", "nonexistent", true, 10*time.Millisecond)
	assert.Error(t, err)
}

func TestCapabilityRegistry_RecordExecution_NotFoundAgent(t *testing.T) {
	reg := newTestRegistry(t)
	err := reg.RecordExecution(context.Background(), "ghost", "cap", true, 10*time.Millisecond)
	assert.Error(t, err)
}

func TestCapabilityRegistry_Close(t *testing.T) {
	reg := newTestRegistry(t)
	err := reg.Close()
	require.NoError(t, err)

	// Double close should not panic
	err = reg.Close()
	require.NoError(t, err)
}

func TestCapabilityRegistry_Start(t *testing.T) {
	reg := newTestRegistry(t)
	err := reg.Start(context.Background())
	require.NoError(t, err)
}

func TestMustMarshal(t *testing.T) {
	data := mustMarshal(map[string]string{"key": "value"})
	assert.NotNil(t, data)

	// Unmarshalable value
	data = mustMarshal(make(chan int))
	assert.Nil(t, data)
}
