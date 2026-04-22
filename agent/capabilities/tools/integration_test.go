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

// --- mockAgentCapabilityProvider ---

type mockAgentCapabilityProvider struct {
	id           string
	name         string
	capabilities []a2a.Capability
	card         *a2a.AgentCard
}

func (m *mockAgentCapabilityProvider) ID() string                        { return m.id }
func (m *mockAgentCapabilityProvider) Name() string                      { return m.name }
func (m *mockAgentCapabilityProvider) GetCapabilities() []a2a.Capability { return m.capabilities }
func (m *mockAgentCapabilityProvider) GetAgentCard() *a2a.AgentCard      { return m.card }

// --- AgentDiscoveryIntegration tests ---

func newTestIntegration(t *testing.T) (*DiscoveryService, *AgentDiscoveryIntegration) {
	t.Helper()
	cfg := DefaultServiceConfig()
	cfg.Registry.EnableHealthCheck = false
	cfg.Protocol.EnableHTTP = false
	cfg.Protocol.EnableMulticast = false
	cfg.EnableAutoRegistration = false

	svc := NewDiscoveryService(cfg, zap.NewNop())
	ctx := context.Background()
	require.NoError(t, svc.Start(ctx))
	t.Cleanup(func() { require.NoError(t, svc.Stop(ctx)) })

	intCfg := DefaultIntegrationConfig()
	intCfg.AutoUnregister = false
	integration := NewAgentDiscoveryIntegration(svc, intCfg, nil)
	return svc, integration
}

func TestNewAgentDiscoveryIntegration_Defaults(t *testing.T) {
	svc := NewDiscoveryService(nil, nil)
	integration := NewAgentDiscoveryIntegration(svc, nil, nil)
	require.NotNil(t, integration)
	assert.NotNil(t, integration.config)
	assert.NotNil(t, integration.agents)
	assert.NotNil(t, integration.loadReporters)
}

func TestDefaultIntegrationConfig(t *testing.T) {
	cfg := DefaultIntegrationConfig()
	assert.True(t, cfg.AutoRegister)
	assert.True(t, cfg.AutoUnregister)
	assert.Equal(t, 10*time.Second, cfg.LoadReportInterval)
	assert.Equal(t, "http://localhost:8080", cfg.DefaultEndpoint)
	assert.Equal(t, "1.0.0", cfg.DefaultVersion)
}

func TestAgentDiscoveryIntegration_RegisterAgent(t *testing.T) {
	_, integration := newTestIntegration(t)
	ctx := context.Background()

	agent := &mockAgentCapabilityProvider{
		id:   "agent-1",
		name: "Agent One",
		capabilities: []a2a.Capability{
			{Name: "code_review", Description: "Review code", Type: a2a.CapabilityTypeTask},
		},
		card: nil, // will use default card creation
	}

	err := integration.RegisterAgent(ctx, agent)
	require.NoError(t, err)
	assert.True(t, integration.IsAgentRegistered("agent-1"))

	// Duplicate registration should fail
	err = integration.RegisterAgent(ctx, agent)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")
}

func TestAgentDiscoveryIntegration_RegisterAgent_NilAgent(t *testing.T) {
	_, integration := newTestIntegration(t)
	ctx := context.Background()

	err := integration.RegisterAgent(ctx, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "agent is nil")
}

func TestAgentDiscoveryIntegration_RegisterAgent_WithCard(t *testing.T) {
	_, integration := newTestIntegration(t)
	ctx := context.Background()

	card := a2a.NewAgentCard("card-agent", "Card Agent", "http://localhost:9090", "2.0.0")
	card.AddCapability("deploy", "Deploy", a2a.CapabilityTypeTask)

	agent := &mockAgentCapabilityProvider{
		id:   "card-agent",
		name: "Card Agent",
		capabilities: []a2a.Capability{
			{Name: "deploy", Description: "Deploy", Type: a2a.CapabilityTypeTask},
		},
		card: card,
	}

	err := integration.RegisterAgent(ctx, agent)
	require.NoError(t, err)
	assert.True(t, integration.IsAgentRegistered("card-agent"))
}

func TestAgentDiscoveryIntegration_UnregisterAgent(t *testing.T) {
	_, integration := newTestIntegration(t)
	ctx := context.Background()

	agent := &mockAgentCapabilityProvider{
		id:           "unreg-agent",
		name:         "Unreg Agent",
		capabilities: []a2a.Capability{},
	}

	require.NoError(t, integration.RegisterAgent(ctx, agent))
	assert.True(t, integration.IsAgentRegistered("unreg-agent"))

	err := integration.UnregisterAgent(ctx, "unreg-agent")
	require.NoError(t, err)
	assert.False(t, integration.IsAgentRegistered("unreg-agent"))
}

func TestAgentDiscoveryIntegration_GetRegisteredAgents(t *testing.T) {
	_, integration := newTestIntegration(t)
	ctx := context.Background()

	assert.Empty(t, integration.GetRegisteredAgents())

	for _, id := range []string{"a1", "a2", "a3"} {
		agent := &mockAgentCapabilityProvider{id: id, name: id}
		require.NoError(t, integration.RegisterAgent(ctx, agent))
	}

	agents := integration.GetRegisteredAgents()
	assert.Len(t, agents, 3)
}

func TestAgentDiscoveryIntegration_UpdateAgentCapabilities(t *testing.T) {
	_, integration := newTestIntegration(t)
	ctx := context.Background()

	agent := &mockAgentCapabilityProvider{
		id:   "update-agent",
		name: "Update Agent",
		capabilities: []a2a.Capability{
			{Name: "search", Description: "Search", Type: a2a.CapabilityTypeQuery},
		},
	}

	require.NoError(t, integration.RegisterAgent(ctx, agent))

	err := integration.UpdateAgentCapabilities(ctx, "update-agent")
	require.NoError(t, err)

	// Non-existent agent
	err = integration.UpdateAgentCapabilities(ctx, "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not registered")
}

func TestAgentDiscoveryIntegration_SetLoadReporter(t *testing.T) {
	_, integration := newTestIntegration(t)

	called := false
	integration.SetLoadReporter("agent-1", func() float64 {
		called = true
		return 0.5
	})

	integration.loadReportersMu.RLock()
	reporter, ok := integration.loadReporters["agent-1"]
	integration.loadReportersMu.RUnlock()

	assert.True(t, ok)
	val := reporter()
	assert.True(t, called)
	assert.Equal(t, 0.5, val)
}

func TestAgentDiscoveryIntegration_RecordExecution(t *testing.T) {
	svc, integration := newTestIntegration(t)
	ctx := context.Background()

	card := a2a.NewAgentCard("exec-agent", "Exec Agent", "http://localhost:8080", "1.0.0")
	card.AddCapability("build", "Build", a2a.CapabilityTypeTask)
	info := AgentInfoFromCard(card, true)
	require.NoError(t, svc.RegisterAgent(ctx, info))

	err := integration.RecordExecution(ctx, "exec-agent", "build", true, 50*time.Millisecond)
	require.NoError(t, err)
}

func TestAgentDiscoveryIntegration_FindAgentForTask(t *testing.T) {
	svc, integration := newTestIntegration(t)
	ctx := context.Background()

	card := a2a.NewAgentCard("find-agent", "Find Agent", "http://localhost:8080", "1.0.0")
	card.AddCapability("analyze", "Analyze", a2a.CapabilityTypeQuery)
	info := AgentInfoFromCard(card, true)
	require.NoError(t, svc.RegisterAgent(ctx, info))

	agent, err := integration.FindAgentForTask(ctx, "analyze code", []string{"analyze"})
	require.NoError(t, err)
	assert.Equal(t, "find-agent", agent.Card.Name)
}

func TestAgentDiscoveryIntegration_ComposeAgentsForTask(t *testing.T) {
	svc, integration := newTestIntegration(t)
	ctx := context.Background()

	for _, pair := range []struct{ name, cap string }{
		{"compose-a", "lint"},
		{"compose-b", "format"},
	} {
		card := a2a.NewAgentCard(pair.name, "Test", "http://localhost:8080", "1.0.0")
		card.AddCapability(pair.cap, pair.cap, a2a.CapabilityTypeTask)
		info := AgentInfoFromCard(card, true)
		require.NoError(t, svc.RegisterAgent(ctx, info))
	}

	result, err := integration.ComposeAgentsForTask(ctx, &CompositionRequest{
		RequiredCapabilities: []string{"lint", "format"},
	})
	require.NoError(t, err)
	assert.True(t, result.Complete)
}

func TestAgentDiscoveryIntegration_StartStop(t *testing.T) {
	_, integration := newTestIntegration(t)
	ctx := context.Background()

	err := integration.Start(ctx)
	require.NoError(t, err)
	assert.True(t, integration.running)

	// Double start
	err = integration.Start(ctx)
	assert.Error(t, err)

	err = integration.Stop(ctx)
	require.NoError(t, err)
	assert.False(t, integration.running)

	// Double stop is no-op
	err = integration.Stop(ctx)
	require.NoError(t, err)
}

func TestAgentDiscoveryIntegration_StopWithAutoUnregister(t *testing.T) {
	cfg := DefaultServiceConfig()
	cfg.Registry.EnableHealthCheck = false
	cfg.Protocol.EnableHTTP = false
	cfg.Protocol.EnableMulticast = false
	cfg.EnableAutoRegistration = false

	svc := NewDiscoveryService(cfg, zap.NewNop())
	ctx := context.Background()
	require.NoError(t, svc.Start(ctx))
	defer func() { require.NoError(t, svc.Stop(ctx)) }()

	intCfg := DefaultIntegrationConfig()
	intCfg.AutoUnregister = true
	integration := NewAgentDiscoveryIntegration(svc, intCfg, nil)

	agent := &mockAgentCapabilityProvider{id: "auto-unreg", name: "Auto Unreg"}
	require.NoError(t, integration.RegisterAgent(ctx, agent))
	require.NoError(t, integration.Start(ctx))

	err := integration.Stop(ctx)
	require.NoError(t, err)
	assert.False(t, integration.IsAgentRegistered("auto-unreg"))
}

func TestAgentDiscoveryIntegration_DiscoveryService(t *testing.T) {
	svc, integration := newTestIntegration(t)
	assert.Equal(t, svc, integration.DiscoveryService())
}

func TestSetGlobalIntegration(t *testing.T) {
	svc := NewDiscoveryService(nil, nil)
	integration := NewAgentDiscoveryIntegration(svc, nil, nil)
	SetGlobalIntegration(integration)
	got := GetGlobalIntegration()
	assert.Equal(t, integration, got)
	SetGlobalIntegration(nil)
}
