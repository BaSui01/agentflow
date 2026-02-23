package agent

import (
	"testing"

	"github.com/BaSui01/agentflow/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ============================================================
// Container tests
// ============================================================

func TestContainer_New(t *testing.T) {
	c := NewContainer()
	assert.NotNil(t, c)
	assert.Nil(t, c.Provider())
	assert.Nil(t, c.Memory())
	assert.Nil(t, c.ToolManager())
	assert.Nil(t, c.EventBus())
}

func TestContainer_WithProvider(t *testing.T) {
	provider := &testProvider{name: "test"}
	c := NewContainer().WithProvider(provider)
	assert.Equal(t, provider, c.Provider())
}

func TestContainer_WithMemory(t *testing.T) {
	mem := &testMemoryManager{}
	c := NewContainer().WithMemory(mem)
	assert.Equal(t, mem, c.Memory())
}

func TestContainer_WithToolManager(t *testing.T) {
	tm := &testToolManager{}
	c := NewContainer().WithToolManager(tm)
	assert.Equal(t, tm, c.ToolManager())
}

func TestContainer_WithEventBus(t *testing.T) {
	bus := &testEventBus{}
	c := NewContainer().WithEventBus(bus)
	assert.Equal(t, bus, c.EventBus())
}

func TestContainer_WithLogger(t *testing.T) {
	logger := zap.NewNop()
	c := NewContainer().WithLogger(logger)
	assert.Equal(t, logger, c.Logger())
}

func TestContainer_Logger_Default(t *testing.T) {
	c := NewContainer()
	// Should return a nop logger when none is set
	assert.NotNil(t, c.Logger())
}

func TestContainer_WithFactories(t *testing.T) {
	c := NewContainer().
		WithReflectionFactory(func() any { return "reflection" }).
		WithToolSelectionFactory(func() any { return "tool_selection" }).
		WithGuardrailsFactory(func() any { return "guardrails" })

	assert.NotNil(t, c.reflectionFactory)
	assert.NotNil(t, c.toolSelectionFactory)
	assert.NotNil(t, c.guardrailsFactory)
}

func TestContainer_Chaining(t *testing.T) {
	provider := &testProvider{name: "test"}
	mem := &testMemoryManager{}
	tm := &testToolManager{}
	bus := &testEventBus{}
	logger := zap.NewNop()

	c := NewContainer().
		WithProvider(provider).
		WithMemory(mem).
		WithToolManager(tm).
		WithEventBus(bus).
		WithLogger(logger)

	assert.Equal(t, provider, c.Provider())
	assert.Equal(t, mem, c.Memory())
	assert.Equal(t, tm, c.ToolManager())
	assert.Equal(t, bus, c.EventBus())
	assert.Equal(t, logger, c.Logger())
}

func TestContainer_CreateBaseAgent(t *testing.T) {
	provider := &testProvider{name: "test"}
	c := NewContainer().
		WithProvider(provider).
		WithLogger(zap.NewNop())

	config := Config{
		ID:   "agent-1",
		Name: "TestAgent",
		Type: TypeGeneric,
	}

	agent, err := c.CreateBaseAgent(config)
	require.NoError(t, err)
	assert.Equal(t, "agent-1", agent.ID())
	assert.Equal(t, "TestAgent", agent.Name())
	assert.Equal(t, TypeGeneric, agent.Type())
}

func TestContainer_CreateModularAgent(t *testing.T) {
	provider := &testProvider{name: "test"}
	c := NewContainer().
		WithProvider(provider).
		WithLogger(zap.NewNop())

	config := ModularAgentConfig{
		ID:   "mod-1",
		Name: "ModAgent",
		Type: TypeAssistant,
	}

	agent, err := c.CreateModularAgent(config)
	require.NoError(t, err)
	assert.Equal(t, "mod-1", agent.ID())
	assert.Equal(t, "ModAgent", agent.Name())
	assert.Equal(t, TypeAssistant, agent.Type())
}

// ============================================================
// AgentFactoryFunc tests
// ============================================================

func TestAgentFactoryFunc_CreateAgent(t *testing.T) {
	c := NewContainer().
		WithProvider(&testProvider{name: "test"}).
		WithLogger(zap.NewNop())

	factory := NewAgentFactoryFunc(c)

	agent, err := factory.CreateAgent(Config{
		ID:   "factory-1",
		Name: "FactoryAgent",
		Type: TypeGeneric,
	})
	require.NoError(t, err)
	assert.Equal(t, "factory-1", agent.ID())
}

func TestAgentFactoryFunc_CreateModular(t *testing.T) {
	c := NewContainer().
		WithProvider(&testProvider{name: "test"}).
		WithLogger(zap.NewNop())

	factory := NewAgentFactoryFunc(c)

	agent, err := factory.CreateModular(ModularAgentConfig{
		ID:   "factory-mod-1",
		Name: "FactoryModAgent",
		Type: TypeAnalyzer,
	})
	require.NoError(t, err)
	assert.Equal(t, "factory-mod-1", agent.ID())
}

// ============================================================
// ServiceLocator tests
// ============================================================

func TestServiceLocator_RegisterAndGet(t *testing.T) {
	sl := NewServiceLocator()
	sl.Register("key", "value")

	val, ok := sl.Get("key")
	assert.True(t, ok)
	assert.Equal(t, "value", val)
}

func TestServiceLocator_Get_NotFound(t *testing.T) {
	sl := NewServiceLocator()
	_, ok := sl.Get("nonexistent")
	assert.False(t, ok)
}

func TestServiceLocator_MustGet_Success(t *testing.T) {
	sl := NewServiceLocator()
	sl.Register("key", "value")

	val := sl.MustGet("key")
	assert.Equal(t, "value", val)
}

func TestServiceLocator_MustGet_Panics(t *testing.T) {
	sl := NewServiceLocator()
	assert.Panics(t, func() {
		sl.MustGet("nonexistent")
	})
}

func TestServiceLocator_GetProvider(t *testing.T) {
	sl := NewServiceLocator()

	// Not registered
	_, ok := sl.GetProvider()
	assert.False(t, ok)

	// Register wrong type
	sl.Register(ServiceProvider, "not a provider")
	_, ok = sl.GetProvider()
	assert.False(t, ok)

	// Register correct type
	provider := &testProvider{name: "test"}
	sl.Register(ServiceProvider, provider)
	p, ok := sl.GetProvider()
	assert.True(t, ok)
	assert.Equal(t, provider, p)
}

func TestServiceLocator_GetMemory(t *testing.T) {
	sl := NewServiceLocator()

	_, ok := sl.GetMemory()
	assert.False(t, ok)

	mem := &testMemoryManager{}
	sl.Register(ServiceMemory, mem)
	m, ok := sl.GetMemory()
	assert.True(t, ok)
	assert.Equal(t, mem, m)
}

func TestServiceLocator_GetToolManager(t *testing.T) {
	sl := NewServiceLocator()

	_, ok := sl.GetToolManager()
	assert.False(t, ok)

	tm := &testToolManager{}
	sl.Register(ServiceToolManager, tm)
	got, ok := sl.GetToolManager()
	assert.True(t, ok)
	assert.Equal(t, tm, got)
}

func TestServiceLocator_GetEventBus(t *testing.T) {
	sl := NewServiceLocator()

	_, ok := sl.GetEventBus()
	assert.False(t, ok)

	bus := &testEventBus{}
	sl.Register(ServiceEventBus, bus)
	got, ok := sl.GetEventBus()
	assert.True(t, ok)
	assert.Equal(t, bus, got)
}

func TestServiceLocator_GetLogger(t *testing.T) {
	sl := NewServiceLocator()

	_, ok := sl.GetLogger()
	assert.False(t, ok)

	logger := zap.NewNop()
	sl.Register(ServiceLogger, logger)
	got, ok := sl.GetLogger()
	assert.True(t, ok)
	assert.Equal(t, logger, got)
}

func TestServiceConstants(t *testing.T) {
	assert.Equal(t, "provider", ServiceProvider)
	assert.Equal(t, "memory", ServiceMemory)
	assert.Equal(t, "tool_manager", ServiceToolManager)
	assert.Equal(t, "event_bus", ServiceEventBus)
	assert.Equal(t, "logger", ServiceLogger)
}

// Verify GetProvider returns false for wrong type assertion
func TestServiceLocator_GetTyped_WrongType(t *testing.T) {
	sl := NewServiceLocator()
	sl.Register(ServiceMemory, "not a memory manager")

	_, ok := sl.GetMemory()
	assert.False(t, ok)

	sl.Register(ServiceToolManager, 42)
	_, ok = sl.GetToolManager()
	assert.False(t, ok)

	sl.Register(ServiceEventBus, "not a bus")
	_, ok = sl.GetEventBus()
	assert.False(t, ok)

	sl.Register(ServiceLogger, 123)
	_, ok = sl.GetLogger()
	assert.False(t, ok)
}

// Suppress unused import warning
var _ llm.Provider = (*testProvider)(nil)
