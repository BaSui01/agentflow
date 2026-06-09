package core

import (
	"errors"
	"testing"

	llmcore "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type minimalAgentStub struct {
	id        string
	name      string
	agentType string
}

func (a minimalAgentStub) ID() string   { return a.id }
func (a minimalAgentStub) Name() string { return a.name }
func (a minimalAgentStub) Type() string { return a.agentType }

func TestAgentRegistryRegisterCreateAndUnregister(t *testing.T) {
	registry := NewAgentRegistry(zap.NewNop())
	customType := AgentType("custom")
	created := false

	registry.Register(customType, func(config types.AgentConfig, _ llmcore.Gateway, _ MemoryManager, _ ToolManager, _ EventBus, _ *zap.Logger) (MinimalAgent, error) {
		created = true
		return minimalAgentStub{id: config.Core.ID, name: config.Core.Name, agentType: config.Core.Type}, nil
	})

	agent, err := registry.Create(types.AgentConfig{Core: types.CoreConfig{ID: "agent-1", Name: "Agent One", Type: string(customType)}}, nil, nil, nil, nil, zap.NewNop())
	require.NoError(t, err)
	assert.True(t, created)
	assert.Equal(t, "agent-1", agent.ID())
	assert.Equal(t, "Agent One", agent.Name())
	assert.Equal(t, string(customType), agent.Type())
	assert.True(t, registry.IsRegistered(customType))
	assert.NotNil(t, registry.GetFactory(customType))

	registry.Unregister(customType)
	assert.False(t, registry.IsRegistered(customType))
	assert.Nil(t, registry.GetFactory(customType))
}

func TestAgentRegistryCreateReportsMissingAndFactoryErrors(t *testing.T) {
	registry := NewAgentRegistry(zap.NewNop())

	agent, err := registry.Create(types.AgentConfig{Core: types.CoreConfig{Type: "missing"}}, nil, nil, nil, nil, zap.NewNop())
	require.Error(t, err)
	assert.Nil(t, agent)
	assert.Contains(t, err.Error(), `agent type "missing" not registered`)

	boom := errors.New("boom")
	registry.Register("broken", func(types.AgentConfig, llmcore.Gateway, MemoryManager, ToolManager, EventBus, *zap.Logger) (MinimalAgent, error) {
		return nil, boom
	})
	agent, err = registry.Create(types.AgentConfig{Core: types.CoreConfig{Type: "broken"}}, nil, nil, nil, nil, zap.NewNop())
	require.Error(t, err)
	assert.Nil(t, agent)
	assert.ErrorIs(t, err, boom)
	assert.Contains(t, err.Error(), `failed to create agent of type "broken"`)
}

func TestAgentRegistryInitializesBuiltInTypes(t *testing.T) {
	registry := NewAgentRegistry(zap.NewNop())
	for _, agentType := range []AgentType{TypeGeneric, TypeAssistant, TypeAnalyzer, TypeTranslator, TypeSummarizer, TypeReviewer} {
		assert.Truef(t, registry.IsRegistered(agentType), "%s should be registered", agentType)
	}
	assert.GreaterOrEqual(t, len(registry.ListTypes()), 6)
}
