package runtime

import (
	"context"
	"testing"

	llmcore "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestCachingResolver_DefaultModelUsesCatalogAlias(t *testing.T) {
	logger := zap.NewNop()
	registry := NewAgentRegistry(logger)
	var created types.AgentConfig
	registry.Register(TypeGeneric, func(cfg types.AgentConfig, gateway llmcore.Gateway, memory MemoryManager, toolManager ToolManager, bus EventBus, logger *zap.Logger) (Agent, error) {
		created = cfg
		return &resolverAgentStub{id: cfg.Core.ID}, nil
	})
	gateway := testGateway(&captureRuntimeProvider{content: "ok"})
	catalog := types.NewModelCatalog([]types.ModelDescriptor{{
		Provider: "capture-runtime-provider",
		ID:       "gpt-canonical",
		Aliases:  []string{"default-agent-model"},
	}})

	ag, err := NewCachingResolver(registry, gateway, logger).
		WithDefaultModel("default-agent-model").
		WithModelCatalog(catalog).
		Resolve(context.Background(), "agent-a")
	require.NoError(t, err)
	require.NotNil(t, ag)
	assert.Equal(t, "capture-runtime-provider", created.LLM.Provider)
	assert.Equal(t, "gpt-canonical", created.LLM.Model)
	assert.Equal(t, "capture-runtime-provider", created.Model.Provider)
	assert.Equal(t, "gpt-canonical", created.Model.Model)
}

type resolverAgentStub struct{ id string }

func (a *resolverAgentStub) ID() string                     { return a.id }
func (a *resolverAgentStub) Name() string                   { return a.id }
func (a *resolverAgentStub) Type() AgentType                { return TypeGeneric }
func (a *resolverAgentStub) State() State                   { return StateReady }
func (a *resolverAgentStub) Init(context.Context) error     { return nil }
func (a *resolverAgentStub) Teardown(context.Context) error { return nil }
func (a *resolverAgentStub) Plan(context.Context, *Input) (*PlanResult, error) {
	return &PlanResult{}, nil
}
func (a *resolverAgentStub) Execute(context.Context, *Input) (*Output, error) { return &Output{}, nil }
func (a *resolverAgentStub) Observe(context.Context, *Feedback) error         { return nil }
