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

func TestDiscoveryProtocol_SubscribeUnsubscribe(t *testing.T) {
	proto := NewDiscoveryProtocol(&ProtocolConfig{EnableLocal: true}, nil, nil)

	called := false
	id := proto.Subscribe(func(info *AgentInfo) {
		called = true
	})
	assert.NotEmpty(t, id)
	_ = called

	proto.Unsubscribe(id)
}

func TestDiscoveryProtocol_Discover(t *testing.T) {
	reg := newCovTestRegistry(t)
	config := &ProtocolConfig{EnableLocal: true}
	proto := NewDiscoveryProtocol(config, reg, zap.NewNop())

	ctx := context.Background()

	// Announce a local agent
	card := a2a.NewAgentCard("test-agent", "Test", "http://localhost", "1.0")
	info := &AgentInfo{Card: card, Status: AgentStatusOnline, IsLocal: true}
	require.NoError(t, proto.Announce(ctx, info))

	// Discover
	agents, err := proto.Discover(ctx, nil)
	require.NoError(t, err)
	assert.NotEmpty(t, agents)
}

func TestDiscoveryProtocol_Discover_WithFilter(t *testing.T) {
	config := &ProtocolConfig{EnableLocal: true}
	proto := NewDiscoveryProtocol(config, nil, zap.NewNop())
	ctx := context.Background()

	card := a2a.NewAgentCard("agent1", "Agent1", "http://localhost", "1.0")
	info := &AgentInfo{
		Card:    card,
		Status:  AgentStatusOnline,
		IsLocal: true,
		Capabilities: []CapabilityInfo{
			{Capability: a2a.Capability{Name: "search"}, Status: CapabilityStatusActive},
		},
	}
	require.NoError(t, proto.Announce(ctx, info))

	t.Run("filter by capability", func(t *testing.T) {
		agents, err := proto.Discover(ctx, &DiscoveryFilter{
			Capabilities: []string{"search"},
		})
		require.NoError(t, err)
		assert.Len(t, agents, 1)
	})

	t.Run("filter by missing capability", func(t *testing.T) {
		agents, err := proto.Discover(ctx, &DiscoveryFilter{
			Capabilities: []string{"nonexistent"},
		})
		require.NoError(t, err)
		assert.Empty(t, agents)
	})

	t.Run("filter by status", func(t *testing.T) {
		agents, err := proto.Discover(ctx, &DiscoveryFilter{
			Status: []AgentStatus{AgentStatusOnline},
		})
		require.NoError(t, err)
		assert.Len(t, agents, 1)
	})
}

func TestDiscoveryProtocol_StartStop_LocalOnly(t *testing.T) {
	config := &ProtocolConfig{
		EnableLocal: true,
		EnableHTTP:  false,
	}
	proto := NewDiscoveryProtocol(config, nil, zap.NewNop())

	ctx := context.Background()
	require.NoError(t, proto.Start(ctx))

	// Start again should fail
	err := proto.Start(ctx)
	assert.Error(t, err)

	require.NoError(t, proto.Stop(ctx))
}

func TestDiscoveryProtocol_StartStop_WithHTTP(t *testing.T) {
	config := &ProtocolConfig{
		EnableLocal: true,
		EnableHTTP:  true,
		HTTPHost:    "127.0.0.1",
		HTTPPort:    0, // random port
	}
	proto := NewDiscoveryProtocol(config, nil, zap.NewNop())

	ctx := context.Background()
	require.NoError(t, proto.Start(ctx))
	time.Sleep(50 * time.Millisecond)
	require.NoError(t, proto.Stop(ctx))
}

func TestInitGlobalIntegration(t *testing.T) {
	// Reset global state for test
	SetGlobalIntegration(nil)

	service := NewDiscoveryService(nil, zap.NewNop())
	InitGlobalIntegration(service, nil, zap.NewNop())

	integration := GetGlobalIntegration()
	assert.NotNil(t, integration)
}

func TestDiscoveryProtocol_MatchesFilter_LocalRemote(t *testing.T) {
	proto := NewDiscoveryProtocol(&ProtocolConfig{EnableLocal: true}, nil, zap.NewNop())
	ctx := context.Background()

	localCard := a2a.NewAgentCard("local-agent", "Local", "http://localhost", "1.0")
	localInfo := &AgentInfo{Card: localCard, Status: AgentStatusOnline, IsLocal: true}
	require.NoError(t, proto.Announce(ctx, localInfo))

	boolTrue := true
	boolFalse := false

	t.Run("local filter true", func(t *testing.T) {
		agents, err := proto.Discover(ctx, &DiscoveryFilter{Local: &boolTrue})
		require.NoError(t, err)
		assert.Len(t, agents, 1)
	})

	t.Run("remote filter true excludes local", func(t *testing.T) {
		agents, err := proto.Discover(ctx, &DiscoveryFilter{Remote: &boolTrue})
		require.NoError(t, err)
		assert.Empty(t, agents)
	})

	t.Run("local filter false", func(t *testing.T) {
		agents, err := proto.Discover(ctx, &DiscoveryFilter{Local: &boolFalse})
		require.NoError(t, err)
		// Local=false doesn't filter out local agents (it means "don't require local")
		assert.NotEmpty(t, agents)
	})
}

func TestDiscoveryProtocol_MatchesFilter_Tags(t *testing.T) {
	proto := NewDiscoveryProtocol(&ProtocolConfig{EnableLocal: true}, nil, zap.NewNop())
	ctx := context.Background()

	card := a2a.NewAgentCard("tagged", "Tagged", "http://localhost", "1.0")
	info := &AgentInfo{
		Card:    card,
		Status:  AgentStatusOnline,
		IsLocal: true,
		Capabilities: []CapabilityInfo{
			{
				Capability: a2a.Capability{Name: "search"},
				Tags:       []string{"fast", "reliable"},
			},
		},
	}
	require.NoError(t, proto.Announce(ctx, info))

	t.Run("matching tag", func(t *testing.T) {
		agents, err := proto.Discover(ctx, &DiscoveryFilter{Tags: []string{"fast"}})
		require.NoError(t, err)
		assert.Len(t, agents, 1)
	})

	t.Run("missing tag", func(t *testing.T) {
		agents, err := proto.Discover(ctx, &DiscoveryFilter{Tags: []string{"slow"}})
		require.NoError(t, err)
		assert.Empty(t, agents)
	})
}

func TestDiscoveryProtocol_NotifyHandlers(t *testing.T) {
	proto := NewDiscoveryProtocol(&ProtocolConfig{EnableLocal: true}, nil, zap.NewNop())

	ch := make(chan string, 1)
	proto.Subscribe(func(info *AgentInfo) {
		ch <- info.Card.Name
	})

	card := a2a.NewAgentCard("test", "Test", "http://localhost", "1.0")
	info := &AgentInfo{Card: card, Status: AgentStatusOnline, IsLocal: true}
	require.NoError(t, proto.Announce(context.Background(), info))

	select {
	case name := <-ch:
		assert.Equal(t, "test", name)
	case <-time.After(time.Second):
		t.Fatal("handler not called")
	}
}

