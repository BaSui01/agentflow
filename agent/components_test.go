package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ============================================================
// AgentIdentity tests
// ============================================================

func TestAgentIdentity_NewAndGetters(t *testing.T) {
	id := NewAgentIdentity("agent-1", "TestAgent", TypeAssistant)

	assert.Equal(t, "agent-1", id.ID())
	assert.Equal(t, "TestAgent", id.Name())
	assert.Equal(t, TypeAssistant, id.Type())
	assert.Equal(t, "", id.Description())
}

func TestAgentIdentity_SetDescription(t *testing.T) {
	id := NewAgentIdentity("agent-1", "TestAgent", TypeGeneric)
	id.SetDescription("A test agent")
	assert.Equal(t, "A test agent", id.Description())
}

// ============================================================
// StateManager tests
// ============================================================

func TestStateManager_InitialState(t *testing.T) {
	bus := &testEventBus{}
	sm := NewStateManager("agent-1", bus, zap.NewNop())
	assert.Equal(t, StateInit, sm.State())
}

func TestStateManager_Transition_Valid(t *testing.T) {
	bus := &testEventBus{}
	sm := NewStateManager("agent-1", bus, zap.NewNop())

	err := sm.Transition(context.Background(), StateReady)
	require.NoError(t, err)
	assert.Equal(t, StateReady, sm.State())
	// Should have published a state change event
	assert.Len(t, bus.published, 1)
}

func TestStateManager_Transition_Invalid(t *testing.T) {
	bus := &testEventBus{}
	sm := NewStateManager("agent-1", bus, zap.NewNop())

	// init -> completed is not valid
	err := sm.Transition(context.Background(), StateCompleted)
	require.Error(t, err)
	assert.Equal(t, StateInit, sm.State())
	assert.Len(t, bus.published, 0)
}

func TestStateManager_Transition_NilBus(t *testing.T) {
	sm := NewStateManager("agent-1", nil, zap.NewNop())

	err := sm.Transition(context.Background(), StateReady)
	require.NoError(t, err)
	assert.Equal(t, StateReady, sm.State())
}

func TestStateManager_TryLockExec(t *testing.T) {
	bus := &testEventBus{}
	sm := NewStateManager("agent-1", bus, zap.NewNop())

	assert.True(t, sm.TryLockExec())
	// Second attempt should fail
	assert.False(t, sm.TryLockExec())
	sm.UnlockExec()
	// After unlock, should succeed again
	assert.True(t, sm.TryLockExec())
	sm.UnlockExec()
}

func TestStateManager_EnsureReady(t *testing.T) {
	bus := &testEventBus{}
	sm := NewStateManager("agent-1", bus, zap.NewNop())

	// Not ready yet (init state)
	err := sm.EnsureReady()
	require.Error(t, err)
	assert.Equal(t, ErrAgentNotReady, err)

	// Transition to ready
	require.NoError(t, sm.Transition(context.Background(), StateReady))
	err = sm.EnsureReady()
	require.NoError(t, err)
}

// ============================================================
// LLMExecutor tests
// ============================================================

func TestLLMExecutor_NewAndProvider(t *testing.T) {
	provider := &testProvider{name: "test-provider"}
	config := LLMExecutorConfig{
		Model:       "gpt-4",
		MaxTokens:   1000,
		Temperature: 0.7,
	}
	executor := NewLLMExecutor(provider, config, zap.NewNop())

	assert.Equal(t, provider, executor.Provider())
}

func TestLLMExecutor_Complete_NilProvider(t *testing.T) {
	config := LLMExecutorConfig{Model: "gpt-4"}
	executor := NewLLMExecutor(nil, config, zap.NewNop())

	_, err := executor.Complete(context.Background(), []llm.Message{
		{Role: llm.RoleUser, Content: "hello"},
	})
	require.Error(t, err)
	assert.Equal(t, ErrProviderNotSet, err)
}

func TestLLMExecutor_Complete_Success(t *testing.T) {
	provider := &testProvider{
		name: "test",
		completionFn: func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
			return &llm.ChatResponse{
				Choices: []llm.ChatChoice{
					{Message: llm.Message{Content: "response"}, FinishReason: "stop"},
				},
			}, nil
		},
	}
	config := LLMExecutorConfig{Model: "gpt-4", MaxTokens: 100, Temperature: 0.5}
	executor := NewLLMExecutor(provider, config, zap.NewNop())

	resp, err := executor.Complete(context.Background(), []llm.Message{
		{Role: llm.RoleUser, Content: "hello"},
	})
	require.NoError(t, err)
	require.Len(t, resp.Choices, 1)
	assert.Equal(t, "response", resp.Choices[0].Message.Content)
}

func TestLLMExecutor_Complete_WithContextManager(t *testing.T) {
	provider := &testProvider{
		name: "test",
		completionFn: func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
			return &llm.ChatResponse{
				Choices: []llm.ChatChoice{
					{Message: llm.Message{Content: "optimized response"}},
				},
			}, nil
		},
	}
	config := LLMExecutorConfig{Model: "gpt-4"}
	executor := NewLLMExecutor(provider, config, zap.NewNop())

	cm := &testContextManager{
		prepareFn: func(ctx context.Context, msgs []llm.Message, query string) ([]llm.Message, error) {
			// Return optimized messages
			return msgs[:1], nil
		},
	}
	executor.SetContextManager(cm)

	msgs := []llm.Message{
		{Role: llm.RoleSystem, Content: "system"},
		{Role: llm.RoleUser, Content: "hello"},
	}
	resp, err := executor.Complete(context.Background(), msgs)
	require.NoError(t, err)
	assert.Equal(t, "optimized response", resp.Choices[0].Message.Content)
}

func TestLLMExecutor_Stream_NilProvider(t *testing.T) {
	config := LLMExecutorConfig{Model: "gpt-4"}
	executor := NewLLMExecutor(nil, config, zap.NewNop())

	_, err := executor.Stream(context.Background(), []llm.Message{
		{Role: llm.RoleUser, Content: "hello"},
	})
	require.Error(t, err)
	assert.Equal(t, ErrProviderNotSet, err)
}

func TestLLMExecutor_Stream_Success(t *testing.T) {
	ch := make(chan llm.StreamChunk, 1)
	ch <- llm.StreamChunk{Delta: llm.Message{Content: "streamed"}}
	close(ch)

	provider := &testProvider{
		name: "test",
		streamFn: func(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
			return ch, nil
		},
	}
	config := LLMExecutorConfig{Model: "gpt-4"}
	executor := NewLLMExecutor(provider, config, zap.NewNop())

	result, err := executor.Stream(context.Background(), []llm.Message{
		{Role: llm.RoleUser, Content: "hello"},
	})
	require.NoError(t, err)
	chunk := <-result
	assert.Equal(t, "streamed", chunk.Delta.Content)
}

func TestExtractLastUserQuery(t *testing.T) {
	tests := []struct {
		name     string
		messages []llm.Message
		expected string
	}{
		{
			name:     "empty messages",
			messages: nil,
			expected: "",
		},
		{
			name: "single user message",
			messages: []llm.Message{
				{Role: llm.RoleUser, Content: "hello"},
			},
			expected: "hello",
		},
		{
			name: "multiple messages returns last user",
			messages: []llm.Message{
				{Role: llm.RoleUser, Content: "first"},
				{Role: llm.RoleAssistant, Content: "response"},
				{Role: llm.RoleUser, Content: "second"},
			},
			expected: "second",
		},
		{
			name: "no user messages",
			messages: []llm.Message{
				{Role: llm.RoleSystem, Content: "system"},
				{Role: llm.RoleAssistant, Content: "response"},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractLastUserQuery(tt.messages)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ============================================================
// ExtensionManager tests
// ============================================================

func TestExtensionManager_New(t *testing.T) {
	em := NewExtensionManager(zap.NewNop())
	assert.NotNil(t, em)
	assert.False(t, em.HasReflection())
	assert.False(t, em.HasToolSelection())
	assert.False(t, em.HasGuardrails())
	assert.False(t, em.HasObservability())
}

func TestExtensionManager_SetAndGetAll(t *testing.T) {
	em := NewExtensionManager(zap.NewNop())

	reflection := &testReflectionRunner{}
	toolSel := &testToolSelectorRunner{}
	promptEnh := &testPromptEnhancerRunner{}
	skillDisc := &testSkillDiscoverer{}
	mcpSrv := &testMCPServer{}
	enhMem := &testEnhancedMemoryRunner{}
	obs := &testObservabilityRunner{}
	guard := &testGuardrailsExtension{}

	em.SetReflection(reflection)
	em.SetToolSelection(toolSel)
	em.SetPromptEnhancer(promptEnh)
	em.SetSkills(skillDisc)
	em.SetMCP(mcpSrv)
	em.SetEnhancedMemory(enhMem)
	em.SetObservability(obs)
	em.SetGuardrails(guard)

	assert.True(t, em.HasReflection())
	assert.True(t, em.HasToolSelection())
	assert.True(t, em.HasGuardrails())
	assert.True(t, em.HasObservability())

	assert.Equal(t, reflection, em.Reflection())
	assert.Equal(t, toolSel, em.ToolSelection())
	assert.Equal(t, promptEnh, em.PromptEnhancer())
	assert.Equal(t, skillDisc, em.Skills())
	assert.Equal(t, mcpSrv, em.MCP())
	assert.Equal(t, enhMem, em.EnhancedMemory())
	assert.Equal(t, obs, em.Observability())
	assert.Equal(t, guard, em.Guardrails())
}

// ============================================================
// ModularAgent tests
// ============================================================

func TestModularAgent_NewAndIdentity(t *testing.T) {
	config := ModularAgentConfig{
		ID:          "mod-1",
		Name:        "ModAgent",
		Type:        TypeAnalyzer,
		Description: "A modular agent",
	}
	provider := &testProvider{name: "test"}
	mem := &testMemoryManager{}
	tools := &testToolManager{}
	bus := &testEventBus{}

	agent := NewModularAgent(config, provider, mem, tools, bus, zap.NewNop())

	assert.Equal(t, "mod-1", agent.ID())
	assert.Equal(t, "ModAgent", agent.Name())
	assert.Equal(t, TypeAnalyzer, agent.Type())
	assert.Equal(t, StateInit, agent.State())
	assert.NotNil(t, agent.Extensions())
	assert.NotNil(t, agent.LLM())
	assert.Equal(t, mem, agent.Memory())
	assert.Equal(t, tools, agent.Tools())
}

func TestModularAgent_NilLogger(t *testing.T) {
	config := ModularAgentConfig{ID: "mod-2", Name: "Test", Type: TypeGeneric}
	agent := NewModularAgent(config, nil, nil, nil, nil, nil)
	assert.Equal(t, "mod-2", agent.ID())
}

func TestModularAgent_Init(t *testing.T) {
	mem := &testMemoryManager{
		loadRecentFn: func(ctx context.Context, agentID string, kind MemoryKind, limit int) ([]MemoryRecord, error) {
			return []MemoryRecord{{Content: "past"}}, nil
		},
	}
	config := ModularAgentConfig{ID: "mod-3", Name: "Test", Type: TypeGeneric}
	agent := NewModularAgent(config, nil, mem, nil, &testEventBus{}, zap.NewNop())

	err := agent.Init(context.Background())
	require.NoError(t, err)
	assert.Equal(t, StateReady, agent.State())
}

func TestModularAgent_Init_NilMemory(t *testing.T) {
	config := ModularAgentConfig{ID: "mod-4", Name: "Test", Type: TypeGeneric}
	agent := NewModularAgent(config, nil, nil, nil, &testEventBus{}, zap.NewNop())

	err := agent.Init(context.Background())
	require.NoError(t, err)
	assert.Equal(t, StateReady, agent.State())
}

func TestModularAgent_Init_MemoryError(t *testing.T) {
	mem := &testMemoryManager{
		loadRecentFn: func(ctx context.Context, agentID string, kind MemoryKind, limit int) ([]MemoryRecord, error) {
			return nil, errors.New("memory error")
		},
	}
	config := ModularAgentConfig{ID: "mod-5", Name: "Test", Type: TypeGeneric}
	agent := NewModularAgent(config, nil, mem, nil, &testEventBus{}, zap.NewNop())

	// Should still succeed even with memory error
	err := agent.Init(context.Background())
	require.NoError(t, err)
	assert.Equal(t, StateReady, agent.State())
}

func TestModularAgent_Teardown(t *testing.T) {
	config := ModularAgentConfig{ID: "mod-6", Name: "Test", Type: TypeGeneric}
	agent := NewModularAgent(config, nil, nil, nil, nil, zap.NewNop())

	err := agent.Teardown(context.Background())
	require.NoError(t, err)
}

func TestModularAgent_Execute_NotReady(t *testing.T) {
	config := ModularAgentConfig{ID: "mod-7", Name: "Test", Type: TypeGeneric}
	agent := NewModularAgent(config, nil, nil, nil, &testEventBus{}, zap.NewNop())

	_, err := agent.Execute(context.Background(), &Input{Content: "hello"})
	require.Error(t, err)
}

func TestModularAgent_Execute_Success(t *testing.T) {
	provider := &testProvider{
		name: "test",
		completionFn: func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
			return &llm.ChatResponse{
				Choices: []llm.ChatChoice{
					{Message: llm.Message{Content: "response"}, FinishReason: "stop"},
				},
				Usage: llm.ChatUsage{TotalTokens: 42},
			}, nil
		},
	}
	config := ModularAgentConfig{ID: "mod-8", Name: "Test", Type: TypeGeneric}
	agent := NewModularAgent(config, provider, nil, nil, &testEventBus{}, zap.NewNop())
	require.NoError(t, agent.Init(context.Background()))

	output, err := agent.Execute(context.Background(), &Input{TraceID: "t1", Content: "hello"})
	require.NoError(t, err)
	assert.Equal(t, "response", output.Content)
	assert.Equal(t, "t1", output.TraceID)
	assert.Equal(t, 42, output.TokensUsed)
	assert.Equal(t, "stop", output.FinishReason)
}

func TestModularAgent_Execute_NilProvider(t *testing.T) {
	config := ModularAgentConfig{ID: "mod-9", Name: "Test", Type: TypeGeneric}
	agent := NewModularAgent(config, nil, nil, nil, &testEventBus{}, zap.NewNop())
	require.NoError(t, agent.Init(context.Background()))

	_, err := agent.Execute(context.Background(), &Input{Content: "hello"})
	require.Error(t, err)
}

func TestModularAgent_Plan(t *testing.T) {
	provider := &testProvider{
		name: "test",
		completionFn: func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
			return &llm.ChatResponse{
				Choices: []llm.ChatChoice{
					{Message: llm.Message{Content: "1. Step one\n2. Step two"}},
				},
			}, nil
		},
	}
	config := ModularAgentConfig{ID: "mod-10", Name: "Test", Type: TypeGeneric}
	agent := NewModularAgent(config, provider, nil, nil, nil, zap.NewNop())

	result, err := agent.Plan(context.Background(), &Input{Content: "build a house"})
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotEmpty(t, result.Steps)
}

func TestModularAgent_Observe(t *testing.T) {
	config := ModularAgentConfig{ID: "mod-11", Name: "Test", Type: TypeGeneric}
	agent := NewModularAgent(config, nil, nil, nil, nil, zap.NewNop())

	err := agent.Observe(context.Background(), &Feedback{Type: "approval", Content: "looks good"})
	require.NoError(t, err)
}

// ============================================================
// Test helpers for components_test.go
// ============================================================

type testContextManager struct {
	prepareFn func(ctx context.Context, msgs []llm.Message, query string) ([]llm.Message, error)
}

func (m *testContextManager) PrepareMessages(ctx context.Context, msgs []llm.Message, query string) ([]llm.Message, error) {
	if m.prepareFn != nil {
		return m.prepareFn(ctx, msgs, query)
	}
	return msgs, nil
}
func (m *testContextManager) GetStatus(msgs []llm.Message) any    { return nil }
func (m *testContextManager) EstimateTokens(msgs []llm.Message) int { return 0 }

type testReflectionRunner struct{}

func (r *testReflectionRunner) ExecuteWithReflection(ctx context.Context, input any) (any, error) {
	return nil, nil
}
func (r *testReflectionRunner) IsEnabled() bool { return true }

type testToolSelectorRunner struct{}

func (r *testToolSelectorRunner) SelectTools(ctx context.Context, query string, candidates []types.ToolSchema) ([]types.ToolSchema, error) {
	return nil, nil
}
func (r *testToolSelectorRunner) GetSelectionStrategy() string { return "test" }

type testPromptEnhancerRunner struct{}

func (r *testPromptEnhancerRunner) Enhance(ctx context.Context, prompt string, context map[string]any) (string, error) {
	return prompt, nil
}
func (r *testPromptEnhancerRunner) GetEnhancementMode() string { return "test" }

type testSkillDiscoverer struct{}

func (r *testSkillDiscoverer) LoadSkill(ctx context.Context, name string) error { return nil }
func (r *testSkillDiscoverer) ExecuteSkill(ctx context.Context, name string, input any) (any, error) {
	return nil, nil
}
func (r *testSkillDiscoverer) ListSkills() []string { return nil }

type testMCPServer struct{}

func (r *testMCPServer) Connect(ctx context.Context, endpoint string) error { return nil }
func (r *testMCPServer) Disconnect(ctx context.Context) error               { return nil }
func (r *testMCPServer) SendMessage(ctx context.Context, message any) (any, error) {
	return nil, nil
}
func (r *testMCPServer) IsConnected() bool { return false }

type testEnhancedMemoryRunner struct{}

func (r *testEnhancedMemoryRunner) StoreWithImportance(ctx context.Context, content string, importance float64) error {
	return nil
}
func (r *testEnhancedMemoryRunner) RetrieveByRelevance(ctx context.Context, query string, topK int) ([]types.MemoryRecord, error) {
	return nil, nil
}
func (r *testEnhancedMemoryRunner) Consolidate(ctx context.Context) error { return nil }
func (r *testEnhancedMemoryRunner) Decay(ctx context.Context) error       { return nil }

type testObservabilityRunner struct{}

func (r *testObservabilityRunner) RecordMetric(name string, value float64, tags map[string]string) {}
func (r *testObservabilityRunner) StartSpan(ctx context.Context, name string) (context.Context, types.SpanHandle) {
	return ctx, nil
}
func (r *testObservabilityRunner) LogEvent(level string, message string, fields map[string]any) {}

type testGuardrailsExtension struct{}

func (r *testGuardrailsExtension) ValidateInput(ctx context.Context, input string) (*types.ValidationResult, error) {
	return &types.ValidationResult{Valid: true}, nil
}
func (r *testGuardrailsExtension) ValidateOutput(ctx context.Context, output string) (*types.ValidationResult, error) {
	return &types.ValidationResult{Valid: true}, nil
}
func (r *testGuardrailsExtension) FilterOutput(ctx context.Context, output string) (string, error) {
	return output, nil
}
