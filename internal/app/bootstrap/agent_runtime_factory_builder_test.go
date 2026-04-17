package bootstrap

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/testutil/mocks"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
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

func TestRegisterDefaultRuntimeAgentFactory_PreservesConfiguredLoopBudget(t *testing.T) {
	registry := agent.NewAgentRegistry(zap.NewNop())
	provider := mocks.NewSuccessProvider("hello")
	RegisterDefaultRuntimeAgentFactory(registry, provider, nil, nil, nil, zap.NewNop())

	created, err := registry.Create(types.AgentConfig{
		Core: types.CoreConfig{
			ID:   "test-agent",
			Name: "Test",
			Type: string(agent.TypeGeneric),
		},
		LLM: types.LLMConfig{Model: "gpt-4"},
		Runtime: types.RuntimeConfig{
			MaxLoopIterations: 7,
		},
	}, provider, nil, nil, nil, zap.NewNop())
	require.NoError(t, err)

	baseAgent, ok := created.(*agent.BaseAgent)
	require.True(t, ok)
	require.Equal(t, 7, baseAgent.Config().Runtime.MaxLoopIterations)
}

func TestRegisterDefaultRuntimeAgentFactory_PropagatesTaskLoopBudgetRunConfig(t *testing.T) {
	registry := agent.NewAgentRegistry(zap.NewNop())
	provider := &captureBootstrapProvider{content: "hello"}
	RegisterDefaultRuntimeAgentFactory(registry, provider, nil, nil, nil, zap.NewNop())

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

	rc := agent.RunConfigFromInputContext(map[string]any{"max_loop_iterations": 5})
	require.NotNil(t, rc)
	ctx := agent.WithRunConfig(context.Background(), rc)

	_, err = baseAgent.ChatCompletion(ctx, []types.Message{{
		Role:    types.RoleUser,
		Content: "hello",
	}})
	require.NoError(t, err)
	require.NotNil(t, provider.lastRequest)
	require.NotNil(t, provider.lastRequest.Metadata)
	assert.Equal(t, "5", provider.lastRequest.Metadata["max_loop_iterations"])
	_, hasLegacyAlias := provider.lastRequest.Metadata["loop_max_iterations"]
	assert.False(t, hasLegacyAlias)
}

type testEventBus struct{}

func (b *testEventBus) Publish(event agent.Event) {}
func (b *testEventBus) Subscribe(eventType agent.EventType, handler agent.EventHandler) string {
	return "sub"
}
func (b *testEventBus) Unsubscribe(subscriptionID string) {}
func (b *testEventBus) Stop()                             {}

var _ agent.EventBus = (*testEventBus)(nil)

type captureBootstrapProvider struct {
	content     string
	lastRequest *llm.ChatRequest
}

func (p *captureBootstrapProvider) Completion(_ context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	cloned := *req
	if req.Metadata != nil {
		cloned.Metadata = make(map[string]string, len(req.Metadata))
		for key, value := range req.Metadata {
			cloned.Metadata[key] = value
		}
	}
	p.lastRequest = &cloned
	return &llm.ChatResponse{
		Model: req.Model,
		Choices: []llm.ChatChoice{{
			Index: 0,
			Message: types.Message{
				Role:    types.RoleAssistant,
				Content: p.content,
			},
		}},
	}, nil
}

func (p *captureBootstrapProvider) Stream(_ context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	ch := make(chan llm.StreamChunk, 1)
	ch <- llm.StreamChunk{
		Model: req.Model,
		Delta: types.Message{
			Role:    types.RoleAssistant,
			Content: p.content,
		},
	}
	close(ch)
	return ch, nil
}

func (p *captureBootstrapProvider) HealthCheck(context.Context) (*llm.HealthStatus, error) {
	return &llm.HealthStatus{Healthy: true, Latency: time.Millisecond}, nil
}

func (p *captureBootstrapProvider) Name() string                        { return "capture-bootstrap-provider" }
func (p *captureBootstrapProvider) SupportsNativeFunctionCalling() bool { return true }
func (p *captureBootstrapProvider) ListModels(context.Context) ([]llm.Model, error) {
	return []llm.Model{{ID: "test-model"}}, nil
}
func (p *captureBootstrapProvider) Endpoints() llm.ProviderEndpoints { return llm.ProviderEndpoints{} }

func (*captureBootstrapProvider) CountTokens(_ context.Context, req *llm.ChatRequest) (*llm.TokenCountResponse, error) {
	return &llm.TokenCountResponse{
		InputTokens: len(req.Messages) + req.MaxTokens,
	}, nil
}
