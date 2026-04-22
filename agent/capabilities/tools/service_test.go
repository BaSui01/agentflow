package tools

import (
	"context"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/agent/execution/protocol/a2a"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// --- DiscoveryService tests ---

func TestNewDiscoveryService_NilConfig(t *testing.T) {
	svc := NewDiscoveryService(nil, nil)
	require.NotNil(t, svc)
	assert.NotNil(t, svc.registry)
	assert.NotNil(t, svc.matcher)
	assert.NotNil(t, svc.composer)
	assert.NotNil(t, svc.protocol)
}

func TestDiscoveryService_StartStop(t *testing.T) {
	cfg := DefaultServiceConfig()
	cfg.Registry.EnableHealthCheck = false
	cfg.Protocol.EnableHTTP = false
	cfg.Protocol.EnableMulticast = false
	cfg.EnableAutoRegistration = false

	svc := NewDiscoveryService(cfg, zap.NewNop())
	ctx := context.Background()

	err := svc.Start(ctx)
	require.NoError(t, err)
	assert.True(t, svc.running)

	// Double start should fail
	err = svc.Start(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already running")

	err = svc.Stop(ctx)
	require.NoError(t, err)
	assert.False(t, svc.running)

	// Restart should work after stop.
	err = svc.Start(ctx)
	require.NoError(t, err)
	assert.True(t, svc.running)
	err = svc.Stop(ctx)
	require.NoError(t, err)
	assert.False(t, svc.running)

	// Double stop is a no-op
	err = svc.Stop(ctx)
	require.NoError(t, err)
}

func TestDiscoveryService_RegisterLocalAgent(t *testing.T) {
	cfg := DefaultServiceConfig()
	cfg.Registry.EnableHealthCheck = false
	cfg.Protocol.EnableHTTP = false
	cfg.Protocol.EnableMulticast = false
	cfg.EnableAutoRegistration = false

	svc := NewDiscoveryService(cfg, zap.NewNop())
	ctx := context.Background()
	require.NoError(t, svc.Start(ctx))
	defer func() { require.NoError(t, svc.Stop(ctx)) }()

	card := a2a.NewAgentCard("local-agent", "Local Agent", "http://localhost:9090", "1.0.0")
	card.AddCapability("code_review", "Review code", a2a.CapabilityTypeTask)
	info := AgentInfoFromCard(card, true)

	err := svc.RegisterLocalAgent(info)
	require.NoError(t, err)

	// Verify agent is registered
	agent, err := svc.GetAgent(ctx, "local-agent")
	require.NoError(t, err)
	assert.Equal(t, "local-agent", agent.Card.Name)
	assert.True(t, agent.IsLocal)
}

func TestDiscoveryService_UpdateLocalAgentLoad(t *testing.T) {
	cfg := DefaultServiceConfig()
	cfg.Registry.EnableHealthCheck = false
	cfg.Protocol.EnableHTTP = false
	cfg.Protocol.EnableMulticast = false
	cfg.EnableAutoRegistration = false

	svc := NewDiscoveryService(cfg, zap.NewNop())
	ctx := context.Background()
	require.NoError(t, svc.Start(ctx))
	defer func() { require.NoError(t, svc.Stop(ctx)) }()

	// No local agent registered yet
	err := svc.UpdateLocalAgentLoad(0.5)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no local agent")

	// Register local agent
	card := a2a.NewAgentCard("load-agent", "Load Agent", "http://localhost:9090", "1.0.0")
	info := AgentInfoFromCard(card, true)
	require.NoError(t, svc.RegisterLocalAgent(info))

	// Now update load
	err = svc.UpdateLocalAgentLoad(0.75)
	require.NoError(t, err)
}

func TestDiscoveryService_FindAgents(t *testing.T) {
	cfg := DefaultServiceConfig()
	cfg.Registry.EnableHealthCheck = false
	cfg.Protocol.EnableHTTP = false
	cfg.Protocol.EnableMulticast = false
	cfg.EnableAutoRegistration = false

	svc := NewDiscoveryService(cfg, zap.NewNop())
	ctx := context.Background()
	require.NoError(t, svc.Start(ctx))
	defer func() { require.NoError(t, svc.Stop(ctx)) }()

	// Register agents
	for _, name := range []string{"agent-a", "agent-b"} {
		card := a2a.NewAgentCard(name, "Test", "http://localhost:8080", "1.0.0")
		card.AddCapability("search", "Search", a2a.CapabilityTypeQuery)
		info := AgentInfoFromCard(card, true)
		require.NoError(t, svc.RegisterAgent(ctx, info))
	}

	results, err := svc.FindAgents(ctx, &MatchRequest{
		RequiredCapabilities: []string{"search"},
		Strategy:             MatchStrategyBestMatch,
	})
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestDiscoveryService_ListAgents(t *testing.T) {
	cfg := DefaultServiceConfig()
	cfg.Registry.EnableHealthCheck = false
	cfg.Protocol.EnableHTTP = false
	cfg.Protocol.EnableMulticast = false
	cfg.EnableAutoRegistration = false

	svc := NewDiscoveryService(cfg, zap.NewNop())
	ctx := context.Background()
	require.NoError(t, svc.Start(ctx))
	defer func() { require.NoError(t, svc.Stop(ctx)) }()

	agents, err := svc.ListAgents(ctx)
	require.NoError(t, err)
	assert.Empty(t, agents)

	card := a2a.NewAgentCard("list-agent", "Test", "http://localhost:8080", "1.0.0")
	info := AgentInfoFromCard(card, true)
	require.NoError(t, svc.RegisterAgent(ctx, info))

	agents, err = svc.ListAgents(ctx)
	require.NoError(t, err)
	assert.Len(t, agents, 1)
}

func TestDiscoveryService_FindCapabilities(t *testing.T) {
	cfg := DefaultServiceConfig()
	cfg.Registry.EnableHealthCheck = false
	cfg.Protocol.EnableHTTP = false
	cfg.Protocol.EnableMulticast = false
	cfg.EnableAutoRegistration = false

	svc := NewDiscoveryService(cfg, zap.NewNop())
	ctx := context.Background()
	require.NoError(t, svc.Start(ctx))
	defer func() { require.NoError(t, svc.Stop(ctx)) }()

	card := a2a.NewAgentCard("cap-agent", "Test", "http://localhost:8080", "1.0.0")
	card.AddCapability("deploy", "Deploy", a2a.CapabilityTypeTask)
	info := AgentInfoFromCard(card, true)
	require.NoError(t, svc.RegisterAgent(ctx, info))

	caps, err := svc.FindCapabilities(ctx, "deploy")
	require.NoError(t, err)
	assert.Len(t, caps, 1)

	caps, err = svc.FindCapabilities(ctx, "nonexistent")
	require.NoError(t, err)
	assert.Empty(t, caps)
}

func TestDiscoveryService_ComposeCapabilities(t *testing.T) {
	cfg := DefaultServiceConfig()
	cfg.Registry.EnableHealthCheck = false
	cfg.Protocol.EnableHTTP = false
	cfg.Protocol.EnableMulticast = false
	cfg.EnableAutoRegistration = false

	svc := NewDiscoveryService(cfg, zap.NewNop())
	ctx := context.Background()
	require.NoError(t, svc.Start(ctx))
	defer func() { require.NoError(t, svc.Stop(ctx)) }()

	// Register agents with different capabilities
	for _, pair := range []struct{ name, cap string }{
		{"agent-x", "build"},
		{"agent-y", "test"},
	} {
		card := a2a.NewAgentCard(pair.name, "Test", "http://localhost:8080", "1.0.0")
		card.AddCapability(pair.cap, pair.cap, a2a.CapabilityTypeTask)
		info := AgentInfoFromCard(card, true)
		require.NoError(t, svc.RegisterAgent(ctx, info))
	}

	result, err := svc.ComposeCapabilities(ctx, &CompositionRequest{
		RequiredCapabilities: []string{"build", "test"},
	})
	require.NoError(t, err)
	assert.True(t, result.Complete)
	assert.Len(t, result.Agents, 2)
}

func TestDiscoveryService_RegisterDependency(t *testing.T) {
	cfg := DefaultServiceConfig()
	cfg.Registry.EnableHealthCheck = false
	cfg.Protocol.EnableHTTP = false
	cfg.Protocol.EnableMulticast = false
	cfg.EnableAutoRegistration = false

	svc := NewDiscoveryService(cfg, zap.NewNop())
	svc.RegisterDependency("deploy", []string{"build", "test"})
	svc.RegisterExclusiveGroup([]string{"gpu", "cpu"})
	// No panic means success
}

func TestDiscoveryService_Subscribe(t *testing.T) {
	cfg := DefaultServiceConfig()
	cfg.Registry.EnableHealthCheck = false
	cfg.Protocol.EnableHTTP = false
	cfg.Protocol.EnableMulticast = false
	cfg.EnableAutoRegistration = false

	svc := NewDiscoveryService(cfg, zap.NewNop())
	ctx := context.Background()
	require.NoError(t, svc.Start(ctx))
	defer func() { require.NoError(t, svc.Stop(ctx)) }()

	eventCh := make(chan *DiscoveryEvent, 10)
	subID := svc.Subscribe(func(e *DiscoveryEvent) {
		eventCh <- e
	})
	defer svc.Unsubscribe(subID)

	card := a2a.NewAgentCard("sub-agent", "Test", "http://localhost:8080", "1.0.0")
	info := AgentInfoFromCard(card, true)
	require.NoError(t, svc.RegisterAgent(ctx, info))

	select {
	case e := <-eventCh:
		// RegisterAgent fires registered, then Announce may fire updated
		assert.Contains(t, []DiscoveryEventType{DiscoveryEventAgentRegistered, DiscoveryEventAgentUpdated}, e.Type)
	case <-time.After(1 * time.Second):
		t.Error("timeout waiting for event")
	}
}

func TestDiscoveryService_Accessors(t *testing.T) {
	cfg := DefaultServiceConfig()
	cfg.Registry.EnableHealthCheck = false
	cfg.Protocol.EnableHTTP = false
	cfg.Protocol.EnableMulticast = false

	svc := NewDiscoveryService(cfg, zap.NewNop())
	assert.NotNil(t, svc.Registry())
	assert.NotNil(t, svc.Matcher())
	assert.NotNil(t, svc.Composer())
	assert.NotNil(t, svc.Protocol())
}

func TestAgentInfoFromCard_NilCard(t *testing.T) {
	info := AgentInfoFromCard(nil, true)
	assert.Nil(t, info)
}

func TestCreateAgentCard(t *testing.T) {
	caps := []a2a.Capability{
		{Name: "search", Description: "Search", Type: a2a.CapabilityTypeQuery},
		{Name: "deploy", Description: "Deploy", Type: a2a.CapabilityTypeTask},
	}
	card := CreateAgentCard("my-agent", "My Agent", "http://localhost:8080", "2.0.0", caps)
	require.NotNil(t, card)
	assert.Equal(t, "my-agent", card.Name)
	assert.Len(t, card.Capabilities, 2)
}

func TestSetGlobalDiscoveryService(t *testing.T) {
	cfg := DefaultServiceConfig()
	cfg.Registry.EnableHealthCheck = false
	cfg.Protocol.EnableHTTP = false
	cfg.Protocol.EnableMulticast = false

	svc := NewDiscoveryService(cfg, zap.NewNop())
	SetGlobalDiscoveryService(svc)
	got := GetGlobalDiscoveryService()
	assert.Equal(t, svc, got)

	// Cleanup
	SetGlobalDiscoveryService(nil)
}

func TestDefaultServiceConfig(t *testing.T) {
	cfg := DefaultServiceConfig()
	assert.NotNil(t, cfg.Registry)
	assert.NotNil(t, cfg.Matcher)
	assert.NotNil(t, cfg.Composer)
	assert.NotNil(t, cfg.Protocol)
	assert.True(t, cfg.EnableAutoRegistration)
	assert.Equal(t, 15*time.Second, cfg.HeartbeatInterval)
	assert.True(t, cfg.EnableMetrics)
}
