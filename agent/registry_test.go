package agent

import (
	"context"
	"testing"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewAgentRegistry(t *testing.T) {
	r := NewAgentRegistry(zap.NewNop())
	require.NotNil(t, r)

	// Should have builtin types registered
	assert.True(t, r.IsRegistered(TypeGeneric))
	assert.True(t, r.IsRegistered(TypeAssistant))
	assert.True(t, r.IsRegistered(TypeAnalyzer))
	assert.True(t, r.IsRegistered(TypeTranslator))
	assert.True(t, r.IsRegistered(TypeSummarizer))
	assert.True(t, r.IsRegistered(TypeReviewer))
}

func TestAgentRegistry_ListTypes(t *testing.T) {
	r := NewAgentRegistry(zap.NewNop())
	types := r.ListTypes()
	assert.GreaterOrEqual(t, len(types), 6)
}

func TestAgentRegistry_RegisterAndUnregister(t *testing.T) {
	r := NewAgentRegistry(zap.NewNop())
	customType := AgentType("custom")

	assert.False(t, r.IsRegistered(customType))

	r.Register(customType, func(config types.AgentConfig, provider llm.Provider, memory MemoryManager, toolManager ToolManager, bus EventBus, logger *zap.Logger) (Agent, error) {
		return NewBaseAgent(config, provider, memory, toolManager, bus, logger, nil), nil
	})
	assert.True(t, r.IsRegistered(customType))

	r.Unregister(customType)
	assert.False(t, r.IsRegistered(customType))
}

func TestAgentRegistry_Create(t *testing.T) {
	r := NewAgentRegistry(zap.NewNop())

	cfg := testAgentConfig("a1", "test", "gpt-4")
	cfg.Core.Type = string(TypeAssistant)

	agent, err := r.Create(cfg, &testProvider{name: "test"}, nil, nil, nil, zap.NewNop())
	require.NoError(t, err)
	require.NotNil(t, agent)
	assert.Equal(t, "a1", agent.ID())
}

func TestAgentRegistry_Create_UnknownType(t *testing.T) {
	r := NewAgentRegistry(zap.NewNop())

	cfg := testAgentConfig("", "", "")
	cfg.Core.Type = "unknown"
	_, err := r.Create(cfg, &testProvider{name: "test"}, nil, nil, nil, zap.NewNop())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not registered")
}

func TestDefaultPromptBundleForType(t *testing.T) {
	tests := []struct {
		agentType AgentType
		wantZero  bool
	}{
		{TypeGeneric, true},
		{TypeAssistant, false},
		{TypeAnalyzer, false},
		{TypeTranslator, false},
		{TypeSummarizer, false},
		{TypeReviewer, false},
		{AgentType("unknown"), true},
	}

	for _, tt := range tests {
		t.Run(string(tt.agentType), func(t *testing.T) {
			b := defaultPromptBundleForType(tt.agentType)
			assert.Equal(t, tt.wantZero, b.IsZero())
		})
	}
}

func TestDefaultSkillCategoriesForType(t *testing.T) {
	assert.NotEmpty(t, defaultSkillCategoriesForType(TypeAssistant))
	assert.NotEmpty(t, defaultSkillCategoriesForType(TypeAnalyzer))
	assert.NotEmpty(t, defaultSkillCategoriesForType(TypeTranslator))
	assert.NotEmpty(t, defaultSkillCategoriesForType(TypeSummarizer))
	assert.NotEmpty(t, defaultSkillCategoriesForType(TypeReviewer))
	assert.Nil(t, defaultSkillCategoriesForType(TypeGeneric))
}

func TestJoinSkillCategories(t *testing.T) {
	cats := defaultSkillCategoriesForType(TypeAssistant)
	result := joinSkillCategories(cats)
	assert.Contains(t, result, ",")
}

func TestCachingResolver_WithMemory(t *testing.T) {
	logger := zap.NewNop()
	registry := NewAgentRegistry(logger)
	provider := &testProvider{name: "mock"}
	mem := &testMemoryManager{
		loadRecentFn: func(_ context.Context, _ string, _ MemoryKind, _ int) ([]MemoryRecord, error) {
			return nil, nil
		},
	}

	resolver := NewCachingResolver(registry, provider, logger).WithMemory(mem)

	// Resolve should create an agent with memory capabilities.
	ctx := context.Background()
	ag, err := resolver.Resolve(ctx, "test-with-mem")
	require.NoError(t, err)
	require.NotNil(t, ag)

	// The underlying BaseAgent should have a non-nil memory field.
	ba, ok := ag.(*BaseAgent)
	require.True(t, ok)
	assert.NotNil(t, ba.memory)
}

func TestCachingResolver_WithoutMemory(t *testing.T) {
	logger := zap.NewNop()
	registry := NewAgentRegistry(logger)
	provider := &testProvider{name: "mock"}

	resolver := NewCachingResolver(registry, provider, logger)

	ctx := context.Background()
	ag, err := resolver.Resolve(ctx, "test-no-mem")
	require.NoError(t, err)

	ba, ok := ag.(*BaseAgent)
	require.True(t, ok)
	assert.Nil(t, ba.memory)
}

func TestCachingResolver_WithEnhancedMemory(t *testing.T) {
	logger := zap.NewNop()
	registry := NewAgentRegistry(logger)
	provider := &testProvider{name: "mock"}
	enhanced := &mockEnhancedMemory{}

	resolver := NewCachingResolver(registry, provider, logger).WithEnhancedMemory(enhanced)

	ag, err := resolver.Resolve(context.Background(), "test-with-enhanced-mem")
	require.NoError(t, err)

	ba, ok := ag.(*BaseAgent)
	require.True(t, ok)
	assert.Equal(t, enhanced, ba.extensions.EnhancedMemoryExt())
	require.NotNil(t, ba.memoryFacade)
	assert.True(t, ba.memoryFacade.HasEnhanced())
}

func TestCachingResolver_WithToolManagerAndDerivedTools(t *testing.T) {
	logger := zap.NewNop()
	registry := NewAgentRegistry(logger)
	provider := &testProvider{name: "mock"}
	toolManager := &testToolManager{
		getAllowedToolsFn: func(agentID string) []types.ToolSchema {
			assert.Equal(t, "agent-tools", agentID)
			return []types.ToolSchema{
				{Name: "retrieval"},
				{Name: "web_search"},
			}
		},
	}

	resolver := NewCachingResolver(registry, provider, logger).WithToolManager(toolManager)
	ag, err := resolver.Resolve(context.Background(), "agent-tools")
	require.NoError(t, err)

	ba, ok := ag.(*BaseAgent)
	require.True(t, ok)
	require.NotNil(t, ba.toolManager)
	assert.ElementsMatch(t, []string{"retrieval", "web_search"}, ba.config.Runtime.Tools)
}

func TestCachingResolver_WithRuntimeToolsOverride(t *testing.T) {
	logger := zap.NewNop()
	registry := NewAgentRegistry(logger)
	provider := &testProvider{name: "mock"}
	toolManager := &testToolManager{
		getAllowedToolsFn: func(agentID string) []types.ToolSchema {
			return []types.ToolSchema{
				{Name: "retrieval"},
				{Name: "web_search"},
			}
		},
	}

	resolver := NewCachingResolver(registry, provider, logger).
		WithToolManager(toolManager).
		WithRuntimeTools([]string{"retrieval", "retrieval", "  web_search  ", ""})
	ag, err := resolver.Resolve(context.Background(), "agent-tools-override")
	require.NoError(t, err)

	ba, ok := ag.(*BaseAgent)
	require.True(t, ok)
	assert.ElementsMatch(t, []string{"retrieval", "web_search"}, ba.config.Runtime.Tools)
}

func TestCachingResolver_WithDefaultModel(t *testing.T) {
	logger := zap.NewNop()
	registry := NewAgentRegistry(logger)
	provider := &testProvider{name: "mock"}

	resolver := NewCachingResolver(registry, provider, logger).WithDefaultModel("gpt-4o-mini")
	ag, err := resolver.Resolve(context.Background(), "agent-model-default")
	require.NoError(t, err)

	ba, ok := ag.(*BaseAgent)
	require.True(t, ok)
	assert.Equal(t, "gpt-4o-mini", ba.config.LLM.Model)
}
