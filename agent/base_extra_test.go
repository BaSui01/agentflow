package agent

import (
	"context"
	"github.com/BaSui01/agentflow/types"
	"testing"

	"github.com/BaSui01/agentflow/agent/guardrails"
	llmtools "github.com/BaSui01/agentflow/llm/capabilities/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ============================================================
// toolManagerExecutor tests
// ============================================================

func TestToolManagerExecutor_IsAllowed(t *testing.T) {
	exec := newToolManagerExecutor(nil, "agent-1", []string{"calc", "search"}, nil)

	assert.True(t, exec.isAllowed("calc"))
	assert.True(t, exec.isAllowed("search"))
	assert.False(t, exec.isAllowed("unknown"))
	assert.False(t, exec.isAllowed(""))
	assert.False(t, exec.isAllowed("  "))
}

func TestToolManagerExecutor_IsAllowed_TrimSpaces(t *testing.T) {
	exec := newToolManagerExecutor(nil, "agent-1", []string{" calc ", "search"}, nil)

	assert.True(t, exec.isAllowed("calc"))
	assert.True(t, exec.isAllowed("search"))
}

func TestToolManagerExecutor_Execute_NilManager(t *testing.T) {
	exec := newToolManagerExecutor(nil, "agent-1", []string{"calc"}, nil)

	calls := []types.ToolCall{
		{ID: "call-1", Name: "calc", Arguments: []byte(`{"x":1}`)},
	}
	results := exec.Execute(context.Background(), calls)
	require.Len(t, results, 1)
	assert.Equal(t, "tool manager not configured", results[0].Error)
}

func TestToolManagerExecutor_Execute_NotAllowed(t *testing.T) {
	tm := &testToolManager{}
	exec := newToolManagerExecutor(tm, "agent-1", []string{"calc"}, nil)

	calls := []types.ToolCall{
		{ID: "call-1", Name: "unknown", Arguments: []byte(`{}`)},
	}
	results := exec.Execute(context.Background(), calls)
	require.Len(t, results, 1)
	assert.Contains(t, results[0].Error, "not allowed")
}

func TestToolManagerExecutor_Execute_AllowedCalls(t *testing.T) {
	tm := &testToolManager{
		executeForAgentFn: func(ctx context.Context, agentID string, calls []types.ToolCall) []llmtools.ToolResult {
			results := make([]llmtools.ToolResult, len(calls))
			for i, c := range calls {
				results[i] = llmtools.ToolResult{
					ToolCallID: c.ID,
					Name:       c.Name,
					Result:     []byte(`"ok"`),
				}
			}
			return results
		},
	}
	exec := newToolManagerExecutor(tm, "agent-1", []string{"calc", "search"}, nil)

	calls := []types.ToolCall{
		{ID: "call-1", Name: "calc"},
		{ID: "call-2", Name: "unknown"},
		{ID: "call-3", Name: "search"},
	}
	results := exec.Execute(context.Background(), calls)
	require.Len(t, results, 3)
	assert.Empty(t, results[0].Error)
	assert.Contains(t, results[1].Error, "not allowed")
	assert.Empty(t, results[2].Error)
}

func TestToolManagerExecutor_Execute_WithEventBus(t *testing.T) {
	tm := &testToolManager{
		executeForAgentFn: func(ctx context.Context, agentID string, calls []types.ToolCall) []llmtools.ToolResult {
			return []llmtools.ToolResult{{ToolCallID: calls[0].ID, Name: calls[0].Name}}
		},
	}
	bus := &testEventBus{}
	exec := newToolManagerExecutor(tm, "agent-1", []string{"calc"}, bus)

	calls := []types.ToolCall{{ID: "call-1", Name: "calc"}}
	results := exec.Execute(context.Background(), calls)
	require.Len(t, results, 1)
	// Should have published start and end events
	assert.GreaterOrEqual(t, len(bus.published), 2)
}

func TestToolManagerExecutor_ExecuteOne(t *testing.T) {
	tm := &testToolManager{
		executeForAgentFn: func(ctx context.Context, agentID string, calls []types.ToolCall) []llmtools.ToolResult {
			return []llmtools.ToolResult{{ToolCallID: calls[0].ID, Name: calls[0].Name, Result: []byte(`"42"`)}}
		},
	}
	exec := newToolManagerExecutor(tm, "agent-1", []string{"calc"}, nil)

	result := exec.ExecuteOne(context.Background(), types.ToolCall{ID: "call-1", Name: "calc"})
	assert.Equal(t, "call-1", result.ToolCallID)
	assert.Empty(t, result.Error)
}

// ============================================================
// BaseAgent accessor tests
// ============================================================

func TestBaseAgent_Accessors(t *testing.T) {
	mem := &testMemoryManager{}
	tm := &testToolManager{}
	provider := &testProvider{name: "test"}

	ba := NewBaseAgent(testAgentConfig("ba-1", "TestBA", "gpt-4"), provider, mem, tm, nil, zap.NewNop(), nil)

	assert.Equal(t, mem, ba.Memory())
	assert.Equal(t, tm, ba.Tools())
	assert.Equal(t, "ba-1", ba.Config().Core.ID)
	assert.NotNil(t, ba.Logger())
	assert.Equal(t, provider, ba.Provider())
	assert.Nil(t, ba.ToolProvider())
	assert.False(t, ba.ContextEngineEnabled())
}

func TestBaseAgent_SetContextManager(t *testing.T) {
	ba := NewBaseAgent(testAgentConfig("ba-1", "", ""), &testProvider{name: "test"}, nil, nil, nil, zap.NewNop(), nil)

	assert.False(t, ba.ContextEngineEnabled())

	cm := &testContextManager{}
	ba.SetContextManager(cm)
	assert.True(t, ba.ContextEngineEnabled())

	ba.SetContextManager(nil)
	assert.False(t, ba.ContextEngineEnabled())
}

func TestBaseAgent_SetToolProvider(t *testing.T) {
	ba := NewBaseAgent(testAgentConfig("ba-1", "", ""), &testProvider{name: "test"}, nil, nil, nil, zap.NewNop(), nil)

	tp := &testProvider{name: "tool-provider"}
	ba.SetToolProvider(tp)
	assert.Equal(t, tp, ba.ToolProvider())
}

func TestBaseAgent_MaxReActIterations(t *testing.T) {
	ba := NewBaseAgent(testAgentConfig("ba-1", "", ""), &testProvider{name: "test"}, nil, nil, nil, zap.NewNop(), nil)
	assert.Equal(t, 10, ba.maxReActIterations())

	cfg2 := testAgentConfig("ba-2", "", "")
	cfg2.Runtime.MaxReActIterations = 5
	ba2 := NewBaseAgent(cfg2, &testProvider{name: "test"}, nil, nil, nil, zap.NewNop(), nil)
	assert.Equal(t, 5, ba2.maxReActIterations())
}

// ============================================================
// GuardrailsError tests
// ============================================================

func TestGuardrailsError_NoErrors(t *testing.T) {
	err := &GuardrailsError{
		Type:    GuardrailsErrorTypeInput,
		Message: "validation failed",
	}
	assert.Contains(t, err.Error(), "guardrails input validation failed")
	assert.Contains(t, err.Error(), "validation failed")
}

func TestGuardrailsError_WithErrors(t *testing.T) {
	err := &GuardrailsError{
		Type:    GuardrailsErrorTypeOutput,
		Message: "output check",
		Errors: []guardrails.ValidationError{
			{Code: "TOO_LONG", Message: "content too long"},
			{Code: "PII", Message: "contains PII"},
		},
	}
	errStr := err.Error()
	assert.Contains(t, errStr, "guardrails output validation failed")
	assert.Contains(t, errStr, "TOO_LONG")
	assert.Contains(t, errStr, "PII")
}

// ============================================================
// BaseAgent Guardrails tests
// ============================================================

func TestBaseAgent_SetGuardrails(t *testing.T) {
	ba := NewBaseAgent(testAgentConfig("ba-1", "", ""), &testProvider{name: "test"}, nil, nil, nil, zap.NewNop(), nil)

	assert.False(t, ba.GuardrailsEnabled())

	cfg := &guardrails.GuardrailsConfig{
		MaxInputLength: 1000,
	}
	ba.SetGuardrails(cfg)
	assert.True(t, ba.GuardrailsEnabled())

	ba.SetGuardrails(nil)
	assert.False(t, ba.GuardrailsEnabled())
}

func TestBaseAgent_AddInputValidator(t *testing.T) {
	ba := NewBaseAgent(testAgentConfig("ba-1", "", ""), &testProvider{name: "test"}, nil, nil, nil, zap.NewNop(), nil)

	assert.False(t, ba.GuardrailsEnabled())

	v := guardrails.NewLengthValidator(&guardrails.LengthValidatorConfig{
		MaxLength: 100,
		Action:    guardrails.LengthActionReject,
	})
	ba.AddInputValidator(v)
	assert.True(t, ba.GuardrailsEnabled())
}

func TestBaseAgent_AddOutputValidator(t *testing.T) {
	ba := NewBaseAgent(testAgentConfig("ba-1", "", ""), &testProvider{name: "test"}, nil, nil, nil, zap.NewNop(), nil)

	v := guardrails.NewLengthValidator(&guardrails.LengthValidatorConfig{
		MaxLength: 100,
		Action:    guardrails.LengthActionReject,
	})
	ba.AddOutputValidator(v)
	assert.True(t, ba.GuardrailsEnabled())
}

func TestBaseAgent_AddOutputFilter(t *testing.T) {
	ba := NewBaseAgent(testAgentConfig("ba-1", "", ""), &testProvider{name: "test"}, nil, nil, nil, zap.NewNop(), nil)

	f := &testFilter{name: "test-filter"}
	ba.AddOutputFilter(f)
	assert.True(t, ba.GuardrailsEnabled())
}

// testFilter implements guardrails.Filter for testing
type testFilter struct {
	name string
}

func (f *testFilter) Filter(ctx context.Context, content string) (string, error) {
	return content, nil
}
func (f *testFilter) Name() string { return f.name }

func TestBaseAgent_InitGuardrails(t *testing.T) {
	cfg := &guardrails.GuardrailsConfig{
		MaxInputLength:      500,
		BlockedKeywords:     []string{"bad", "evil"},
		InjectionDetection:  true,
		PIIDetectionEnabled: true,
	}

	ac := testAgentConfig("ba-1", "", "")
	ac.Features.Guardrails = &types.GuardrailsConfig{
		Enabled:            true,
		MaxInputLength:     cfg.MaxInputLength,
		BlockedKeywords:    append([]string(nil), cfg.BlockedKeywords...),
		PIIDetection:       cfg.PIIDetectionEnabled,
		InjectionDetection: cfg.InjectionDetection,
		MaxRetries:         cfg.MaxRetries,
		OnInputFailure:     string(cfg.OnInputFailure),
		OnOutputFailure:    string(cfg.OnOutputFailure),
	}
	ba := NewBaseAgent(ac, &testProvider{name: "test"}, nil, nil, nil, zap.NewNop(), nil)

	assert.True(t, ba.GuardrailsEnabled())
	assert.NotNil(t, ba.inputValidatorChain)
	assert.NotNil(t, ba.outputValidator)
}
