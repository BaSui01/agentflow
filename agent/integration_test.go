package agent

import (
	"context"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/agent/memory"
	"github.com/BaSui01/agentflow/agent/skills"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ============================================================
// Integration tests (integration.go)
// ============================================================

func TestBaseAgent_EnableReflection(t *testing.T) {
	ba := newTestBaseAgent()
	assert.Nil(t, ba.extensions.ReflectionExecutor())

	runner := &integTestReflectionRunner{}
	ba.EnableReflection(runner)
	assert.Equal(t, runner, ba.extensions.ReflectionExecutor())
}

func TestBaseAgent_EnableToolSelection(t *testing.T) {
	ba := newTestBaseAgent()
	assert.Nil(t, ba.extensions.ToolSelector())

	sel := &integTestToolSelectorRunner{}
	ba.EnableToolSelection(sel)
	assert.Equal(t, sel, ba.extensions.ToolSelector())
}

func TestBaseAgent_EnablePromptEnhancer(t *testing.T) {
	ba := newTestBaseAgent()
	assert.Nil(t, ba.extensions.PromptEnhancerExt())

	enh := &integTestPromptEnhancerRunner{}
	ba.EnablePromptEnhancer(enh)
	assert.Equal(t, enh, ba.extensions.PromptEnhancerExt())
}

func TestBaseAgent_EnableSkills(t *testing.T) {
	ba := newTestBaseAgent()
	assert.Nil(t, ba.extensions.SkillManagerExt())

	sm := &integTestSkillDiscoverer{}
	ba.EnableSkills(sm)
	assert.Equal(t, sm, ba.extensions.SkillManagerExt())
}

func TestBaseAgent_EnableMCP(t *testing.T) {
	ba := newTestBaseAgent()
	assert.Nil(t, ba.extensions.MCPServerExt())

	srv := &integTestMCPServer{}
	ba.EnableMCP(srv)
	assert.Equal(t, srv, ba.extensions.MCPServerExt())
}

func TestBaseAgent_EnableLSP(t *testing.T) {
	ba := newTestBaseAgent()
	assert.Nil(t, ba.extensions.LSPClientExt())

	client := &integTestLSPClient{}
	ba.EnableLSP(client)
	assert.Equal(t, client, ba.extensions.LSPClientExt())
}

func TestBaseAgent_EnableLSPWithLifecycle(t *testing.T) {
	ba := newTestBaseAgent()

	client := &integTestLSPClient{}
	lifecycle := &integTestLSPLifecycle{}
	ba.EnableLSPWithLifecycle(client, lifecycle)
	assert.Equal(t, client, ba.extensions.LSPClientExt())
	assert.Equal(t, lifecycle, ba.extensions.LSPLifecycleExt())
}

func TestBaseAgent_EnableEnhancedMemory(t *testing.T) {
	ba := newTestBaseAgent()
	assert.Nil(t, ba.extensions.EnhancedMemoryExt())

	mem := &integTestEnhancedMemory{}
	ba.EnableEnhancedMemory(mem)
	assert.Equal(t, mem, ba.extensions.EnhancedMemoryExt())
}

func TestBaseAgent_EnableObservability(t *testing.T) {
	ba := newTestBaseAgent()
	assert.Nil(t, ba.extensions.ObservabilitySystemExt())

	obs := &integTestObservability{}
	ba.EnableObservability(obs)
	assert.Equal(t, obs, ba.extensions.ObservabilitySystemExt())
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

	assert.Equal(t, ba.config.Core.ID, exported["id"])
	assert.Equal(t, ba.config.Core.Name, exported["name"])
	assert.NotNil(t, exported["features"])
}

func TestBaseAgent_ValidateConfiguration_NoProvider(t *testing.T) {
	ba := NewBaseAgent(testAgentConfig("test-1", "Test", ""), nil, nil, nil, nil, zap.NewNop())

	err := ba.ValidateConfiguration()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider not set")
}

func TestBaseAgent_ValidateConfiguration_Success(t *testing.T) {
	ba := newTestBaseAgent()
	err := ba.ValidateConfiguration()
	require.NoError(t, err)
}

func TestBaseAgent_ValidateConfiguration_MissingExecutors(t *testing.T) {
	cfg := testAgentConfig("test-1", "Test", "")
	cfg.Features.Reflection = &types.ReflectionConfig{Enabled: true}
	cfg.Features.ToolSelection = &types.ToolSelectionConfig{Enabled: true}
	cfg.Features.PromptEnhancer = &types.PromptEnhancerConfig{Enabled: true}
	cfg.Extensions.Skills = &types.SkillsConfig{Enabled: true}
	cfg.Extensions.MCP = &types.MCPConfig{Enabled: true}
	cfg.Extensions.LSP = &types.LSPConfig{Enabled: true}
	cfg.Features.Memory = &types.MemoryConfig{Enabled: true}
	cfg.Extensions.Observability = &types.ObservabilityConfig{Enabled: true}
	ba := NewBaseAgent(cfg, &testProvider{name: "test"}, nil, nil, nil, zap.NewNop())

	err := ba.ValidateConfiguration()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reflection enabled but executor not set")
	assert.Contains(t, err.Error(), "tool selection enabled but selector not set")
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
	return NewBaseAgent(testAgentConfig("test-agent", "TestAgent", ""), &testProvider{name: "test"}, nil, nil, nil, zap.NewNop())
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

func (r *integTestObservability) StartTrace(traceID, agentID string)         {}
func (r *integTestObservability) EndTrace(traceID, status string, err error) {}
func (r *integTestObservability) RecordTask(agentID string, success bool, d time.Duration, tokens int, cost, quality float64) {
}
