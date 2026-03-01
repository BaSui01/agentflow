package agent

import (
	"context"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/agent/memory"
	"github.com/BaSui01/agentflow/agent/skills"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ============================================================
// Integration tests (integration.go)
// ============================================================

func TestBaseAgent_EnableReflection(t *testing.T) {
	ba := newTestBaseAgent()
	assert.Nil(t, ba.reflectionExecutor)

	runner := &integTestReflectionRunner{}
	ba.EnableReflection(runner)
	assert.Equal(t, runner, ba.reflectionExecutor)
}

func TestBaseAgent_EnableToolSelection(t *testing.T) {
	ba := newTestBaseAgent()
	assert.Nil(t, ba.toolSelector)

	sel := &integTestToolSelectorRunner{}
	ba.EnableToolSelection(sel)
	assert.Equal(t, sel, ba.toolSelector)
}

func TestBaseAgent_EnablePromptEnhancer(t *testing.T) {
	ba := newTestBaseAgent()
	assert.Nil(t, ba.promptEnhancer)

	enh := &integTestPromptEnhancerRunner{}
	ba.EnablePromptEnhancer(enh)
	assert.Equal(t, enh, ba.promptEnhancer)
}

func TestBaseAgent_EnableSkills(t *testing.T) {
	ba := newTestBaseAgent()
	assert.Nil(t, ba.skillManager)

	sm := &integTestSkillDiscoverer{}
	ba.EnableSkills(sm)
	assert.Equal(t, sm, ba.skillManager)
}

func TestBaseAgent_EnableMCP(t *testing.T) {
	ba := newTestBaseAgent()
	assert.Nil(t, ba.mcpServer)

	srv := &integTestMCPServer{}
	ba.EnableMCP(srv)
	assert.Equal(t, srv, ba.mcpServer)
}

func TestBaseAgent_EnableLSP(t *testing.T) {
	ba := newTestBaseAgent()
	assert.Nil(t, ba.lspClient)

	client := &integTestLSPClient{}
	ba.EnableLSP(client)
	assert.Equal(t, client, ba.lspClient)
}

func TestBaseAgent_EnableLSPWithLifecycle(t *testing.T) {
	ba := newTestBaseAgent()

	client := &integTestLSPClient{}
	lifecycle := &integTestLSPLifecycle{}
	ba.EnableLSPWithLifecycle(client, lifecycle)
	assert.Equal(t, client, ba.lspClient)
	assert.Equal(t, lifecycle, ba.lspLifecycle)
}

func TestBaseAgent_EnableEnhancedMemory(t *testing.T) {
	ba := newTestBaseAgent()
	assert.Nil(t, ba.enhancedMemory)

	mem := &integTestEnhancedMemory{}
	ba.EnableEnhancedMemory(mem)
	assert.Equal(t, mem, ba.enhancedMemory)
}

func TestBaseAgent_EnableObservability(t *testing.T) {
	ba := newTestBaseAgent()
	assert.Nil(t, ba.observabilitySystem)

	obs := &integTestObservability{}
	ba.EnableObservability(obs)
	assert.Equal(t, obs, ba.observabilitySystem)
}

func TestBaseAgent_GetFeatureStatus(t *testing.T) {
	ba := newTestBaseAgent()
	status := ba.GetFeatureStatus()

	// All should be false initially
	for key, val := range status {
		assert.False(t, val, "expected %s to be false", key)
	}

	// Enable some features
	ba.EnableReflection(&integTestReflectionRunner{})
	ba.EnableObservability(&integTestObservability{})

	status = ba.GetFeatureStatus()
	assert.True(t, status["reflection"])
	assert.True(t, status["observability"])
	assert.False(t, status["skills"])
}

func TestBaseAgent_PrintFeatureStatus(t *testing.T) {
	ba := newTestBaseAgent()
	// Should not panic
	ba.PrintFeatureStatus()
}

func TestBaseAgent_GetFeatureMetrics(t *testing.T) {
	ba := newTestBaseAgent()
	ba.EnableReflection(&integTestReflectionRunner{})

	metrics := ba.GetFeatureMetrics()
	assert.Equal(t, ba.ID(), metrics["agent_id"])
	assert.Equal(t, ba.Name(), metrics["agent_name"])
	assert.NotNil(t, metrics["features"])
	assert.NotNil(t, metrics["config"])
	assert.Equal(t, 1, metrics["enabled_features_count"])
}

func TestBaseAgent_ExportConfiguration(t *testing.T) {
	ba := newTestBaseAgent()
	exported := ba.ExportConfiguration()

	assert.Equal(t, ba.config.ID, exported["id"])
	assert.Equal(t, ba.config.Name, exported["name"])
	assert.NotNil(t, exported["features"])
}

func TestBaseAgent_ValidateConfiguration_NoProvider(t *testing.T) {
	require.PanicsWithValue(t, "agent.NewBaseAgent: provider must not be nil", func() {
		_ = NewBaseAgent(Config{
			ID:   "test-1",
			Name: "Test",
			Type: TypeGeneric,
		}, nil, nil, nil, nil, zap.NewNop())
	})
}

func TestBaseAgent_ValidateConfiguration_Success(t *testing.T) {
	ba := newTestBaseAgent()
	err := ba.ValidateConfiguration()
	require.NoError(t, err)
}

func TestBaseAgent_ValidateConfiguration_MissingExecutors(t *testing.T) {
	ba := NewBaseAgent(Config{
		ID:                   "test-1",
		Name:                 "Test",
		Type:                 TypeGeneric,
		EnableReflection:     true,
		EnableToolSelection:  true,
		EnablePromptEnhancer: true,
		EnableSkills:         true,
		EnableMCP:            true,
		EnableLSP:            true,
		EnableEnhancedMemory: true,
		EnableObservability:  true,
	}, &testProvider{name: "test"}, nil, nil, nil, zap.NewNop())

	err := ba.ValidateConfiguration()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reflection enabled but executor not set")
	assert.Contains(t, err.Error(), "tool selection enabled but selector not set")
}

func TestBaseAgent_QuickSetup(t *testing.T) {
	ba := newTestBaseAgent()
	opts := DefaultQuickSetupOptions()

	result, err := ba.QuickSetup(context.Background(), opts)
	require.NoError(t, err)
	require.NotNil(t, result)
	// With default options (all features enabled) and no subsystems wired,
	// every feature should appear in RequiredSetups.
	assert.NotEmpty(t, result.RequiredSetups)
}

func TestDefaultQuickSetupOptions(t *testing.T) {
	opts := DefaultQuickSetupOptions()
	assert.True(t, opts.EnableAllFeatures)
	assert.True(t, opts.EnableReflection)
	assert.True(t, opts.EnableToolSelection)
	assert.True(t, opts.EnablePromptEnhancer)
	assert.True(t, opts.EnableSkills)
	assert.False(t, opts.EnableMCP) // MCP needs extra config
	assert.True(t, opts.EnableLSP)
	assert.True(t, opts.EnableEnhancedMemory)
	assert.True(t, opts.EnableObservability)
	assert.Equal(t, 3, opts.ReflectionMaxIterations)
	assert.Equal(t, 5, opts.ToolSelectionMaxTools)
}

func TestDefaultEnhancedExecutionOptions(t *testing.T) {
	opts := DefaultEnhancedExecutionOptions()
	assert.False(t, opts.UseReflection)
	assert.False(t, opts.UseToolSelection)
	assert.True(t, opts.LoadWorkingMemory)
	assert.True(t, opts.LoadShortTermMemory)
	assert.True(t, opts.SaveToMemory)
	assert.True(t, opts.UseObservability)
	assert.True(t, opts.RecordMetrics)
	assert.True(t, opts.RecordTrace)
}

func TestPrependSkillInstructions(t *testing.T) {
	tests := []struct {
		name         string
		prompt       string
		instructions []string
		expected     string
	}{
		{
			name:         "empty instructions",
			prompt:       "hello",
			instructions: nil,
			expected:     "hello",
		},
		{
			name:         "all blank instructions",
			prompt:       "hello",
			instructions: []string{"", "  ", ""},
			expected:     "hello",
		},
		{
			name:         "with instructions",
			prompt:       "hello",
			instructions: []string{"do this", "do that"},
			expected:     "技能执行指令:\n1. do this\n2. do that\n\n用户请求:\nhello",
		},
		{
			name:         "deduplicates instructions",
			prompt:       "hello",
			instructions: []string{"do this", "do this", "do that"},
			expected:     "技能执行指令:\n1. do this\n2. do that\n\n用户请求:\nhello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := prependSkillInstructions(tt.prompt, tt.instructions)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ============================================================
// Adapter tests
// ============================================================

func TestAsReflectionRunner(t *testing.T) {
	// Just verify it wraps without panic
	runner := AsReflectionRunner(&ReflectionExecutor{})
	assert.NotNil(t, runner)
}

func TestAsPromptEnhancerRunner(t *testing.T) {
	enhancer := NewPromptEnhancer(*DefaultPromptEnhancerConfig())
	runner := AsPromptEnhancerRunner(enhancer)
	assert.NotNil(t, runner)

	result, err := runner.EnhanceUserPrompt("hello", "")
	require.NoError(t, err)
	assert.NotEmpty(t, result)
}

// ============================================================
// Test helpers for integration_test.go
// ============================================================

func newTestBaseAgent() *BaseAgent {
	return NewBaseAgent(Config{
		ID:   "test-agent",
		Name: "TestAgent",
		Type: TypeGeneric,
	}, &testProvider{name: "test"}, nil, nil, nil, zap.NewNop())
}

type integTestReflectionRunner struct{}

func (r *integTestReflectionRunner) ExecuteWithReflection(ctx context.Context, input *Input) (any, error) {
	return nil, nil
}

type integTestToolSelectorRunner struct{}

func (r *integTestToolSelectorRunner) SelectTools(ctx context.Context, task string, tools any) (any, error) {
	return nil, nil
}

type integTestPromptEnhancerRunner struct{}

func (r *integTestPromptEnhancerRunner) EnhanceUserPrompt(prompt, ctx string) (string, error) {
	return prompt, nil
}

type integTestSkillDiscoverer struct{}

func (r *integTestSkillDiscoverer) DiscoverSkills(ctx context.Context, task string) ([]*skills.Skill, error) {
	return nil, nil
}

type integTestMCPServer struct{}

type integTestLSPClient struct{}

func (r *integTestLSPClient) Shutdown(ctx context.Context) error { return nil }

type integTestLSPLifecycle struct{}

func (r *integTestLSPLifecycle) Close() error { return nil }

type integTestEnhancedMemory struct{}

func (r *integTestEnhancedMemory) LoadWorking(ctx context.Context, agentID string) ([]any, error) {
	return nil, nil
}
func (r *integTestEnhancedMemory) LoadShortTerm(ctx context.Context, agentID string, limit int) ([]any, error) {
	return nil, nil
}
func (r *integTestEnhancedMemory) SaveShortTerm(ctx context.Context, agentID, content string, metadata map[string]any) error {
	return nil
}
func (r *integTestEnhancedMemory) RecordEpisode(ctx context.Context, event *memory.EpisodicEvent) error {
	return nil
}

type integTestObservability struct{}

func (r *integTestObservability) StartTrace(traceID, agentID string)          {}
func (r *integTestObservability) EndTrace(traceID, status string, err error)  {}
func (r *integTestObservability) RecordTask(agentID string, success bool, d time.Duration, tokens int, cost, quality float64) {
}

