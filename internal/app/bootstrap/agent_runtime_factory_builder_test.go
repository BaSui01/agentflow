package bootstrap

import (
	"reflect"
	"testing"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/testutil/mocks"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestRegisterDefaultRuntimeAgentFactory_InjectsRuntimeDefaults(t *testing.T) {
	registry := agent.NewAgentRegistry(zap.NewNop())
	provider := mocks.NewSuccessProvider("hello")
	checkpointManager := &agent.CheckpointManager{}

	RegisterDefaultRuntimeAgentFactory(registry, provider, nil, checkpointManager, nil, zap.NewNop())

	created, err := registry.Create(types.AgentConfig{
		Core: types.CoreConfig{
			ID:   "test-agent",
			Name: "Test",
			Type: string(agent.TypeGeneric),
		},
		LLM: types.LLMConfig{Model: "gpt-4"},
	}, provider, nil, nil, nil, zap.NewNop())
	require.NoError(t, err)

	baseAgent, ok := created.(*agent.BaseAgent)
	require.True(t, ok)
	require.NotNil(t, baseAgent.ReasoningRegistry())
	require.Equal(t, []string{
		"dynamic_planner",
		"plan_and_execute",
		"reflexion",
		"rewoo",
		"tree_of_thought",
	}, baseAgent.ReasoningRegistry().List())

	field := reflect.ValueOf(baseAgent).Elem().FieldByName("checkpointManager")
	require.True(t, field.IsValid())
	require.Equal(t, reflect.ValueOf(checkpointManager).Pointer(), field.Pointer())
}

func TestRegisterDefaultRuntimeAgentFactory_PreservesEventBusPassThrough(t *testing.T) {
	registry := agent.NewAgentRegistry(zap.NewNop())
	provider := mocks.NewSuccessProvider("hello")
	RegisterDefaultRuntimeAgentFactory(registry, provider, nil, nil, nil, zap.NewNop())

	bus := &testEventBus{}
	created, err := registry.Create(types.AgentConfig{
		Core: types.CoreConfig{
			ID:   "test-agent",
			Name: "Test",
			Type: string(agent.TypeGeneric),
		},
		LLM: types.LLMConfig{Model: "gpt-4"},
	}, provider, nil, nil, bus, zap.NewNop())
	require.NoError(t, err)

	baseAgent, ok := created.(*agent.BaseAgent)
	require.True(t, ok)

	busField := reflect.ValueOf(baseAgent).Elem().FieldByName("bus")
	require.True(t, busField.IsValid())
	require.Equal(t, reflect.ValueOf(bus).Pointer(), busField.Elem().Pointer())
}

type testEventBus struct{}

func (b *testEventBus) Publish(event agent.Event) {}
func (b *testEventBus) Subscribe(eventType agent.EventType, handler agent.EventHandler) string {
	return "sub"
}
func (b *testEventBus) Unsubscribe(subscriptionID string) {}
func (b *testEventBus) Stop()                             {}

var _ agent.EventBus = (*testEventBus)(nil)
