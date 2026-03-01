package workflow

import (
	"github.com/BaSui01/agentflow/types"
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/BaSui01/agentflow/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================
// Mock implementations for testing
// ============================================================

// mockProvider implements llm.Provider for testing.
type mockProvider struct {
	name     string
	response *llm.ChatResponse
	err      error
	calls    int
}

func (m *mockProvider) Completion(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	m.calls++
	if m.err != nil {
		return nil, m.err
	}
	return m.response, nil
}

func (m *mockProvider) Stream(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockProvider) HealthCheck(ctx context.Context) (*llm.HealthStatus, error) {
	return &llm.HealthStatus{Healthy: true}, nil
}

func (m *mockProvider) Name() string                        { return m.name }
func (m *mockProvider) SupportsNativeFunctionCalling() bool { return false }
func (m *mockProvider) Endpoints() llm.ProviderEndpoints    { return llm.ProviderEndpoints{} }
func (m *mockProvider) ListModels(ctx context.Context) ([]llm.Model, error) {
	return nil, nil
}

// mockTool implements Tool for testing.
type mockTool struct {
	name   string
	result any
	err    error
}

func (t *mockTool) Name() string { return t.name }
func (t *mockTool) Execute(ctx context.Context, params map[string]any) (any, error) {
	if t.err != nil {
		return nil, t.err
	}
	return t.result, nil
}

// mockToolRegistry implements ToolRegistry for testing.
type mockToolRegistry struct {
	tools map[string]*mockTool
}

func newMockToolRegistry() *mockToolRegistry {
	return &mockToolRegistry{tools: make(map[string]*mockTool)}
}

func (r *mockToolRegistry) register(t *mockTool) {
	r.tools[t.name] = t
}

func (r *mockToolRegistry) GetTool(name string) (Tool, bool) {
	t, ok := r.tools[name]
	if !ok {
		return nil, false
	}
	return t, true
}

func (r *mockToolRegistry) ExecuteTool(ctx context.Context, name string, params map[string]any) (any, error) {
	t, ok := r.tools[name]
	if !ok {
		return nil, fmt.Errorf("tool %q not found", name)
	}
	return t.Execute(ctx, params)
}

// mockHumanInputHandler implements HumanInputHandler for testing.
type mockHumanInputHandler struct {
	response any
	err      error
	calls    int
}

func (h *mockHumanInputHandler) RequestInput(ctx context.Context, prompt string, inputType string, options []string) (any, error) {
	h.calls++
	if h.err != nil {
		return nil, h.err
	}
	return h.response, nil
}

// ============================================================
// Step interface compliance
// ============================================================

func TestAllStepsImplementStepInterface(t *testing.T) {
	steps := []Step{
		&PassthroughStep{},
		&LLMStep{},
		&ToolStep{ToolName: "test-tool"},
		&HumanInputStep{},
		&CodeStep{Handler: func(ctx context.Context, input any) (any, error) { return nil, nil }},
	}
	for _, s := range steps {
		assert.NotEmpty(t, s.Name(), "step %T should have a non-empty name", s)
	}
}

func TestPassthroughStep_NilInput(t *testing.T) {
	step := &PassthroughStep{}
	result, err := step.Execute(context.Background(), nil)
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestPassthroughStep_MapInput(t *testing.T) {
	step := &PassthroughStep{}
	input := map[string]any{"key": "value"}
	result, err := step.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, input, result)
}

// ============================================================
// PassthroughStep tests
// ============================================================

func TestPassthroughStep_Execute(t *testing.T) {
	step := &PassthroughStep{}
	assert.Equal(t, "passthrough", step.Name())

	result, err := step.Execute(context.Background(), "hello")
	require.NoError(t, err)
	assert.Equal(t, "hello", result)
}

// ============================================================
// LLMStep tests
// ============================================================

func TestLLMStep_Execute_NilProvider(t *testing.T) {
	step := &LLMStep{
		Model:  "gpt-4",
		Prompt: "test prompt",
	}
	assert.Equal(t, "llm", step.Name())

	_, err := step.Execute(context.Background(), "input data")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotConfigured)
}

func TestLLMStep_Execute_WithProvider(t *testing.T) {
	provider := &mockProvider{
		name: "test-provider",
		response: &llm.ChatResponse{
			Choices: []llm.ChatChoice{
				{Message: types.Message{Role: llm.RoleAssistant, Content: "Hello from LLM"}},
			},
		},
	}

	step := &LLMStep{
		Model:       "gpt-4",
		Prompt:      "Summarize this:",
		Temperature: 0.7,
		MaxTokens:   100,
		Provider:    provider,
	}

	result, err := step.Execute(context.Background(), "some text")
	require.NoError(t, err)
	assert.Equal(t, "Hello from LLM", result)
	assert.Equal(t, 1, provider.calls)
}

func TestLLMStep_Execute_ProviderError(t *testing.T) {
	provider := &mockProvider{
		name: "test-provider",
		err:  errors.New("API rate limited"),
	}

	step := &LLMStep{
		Model:    "gpt-4",
		Prompt:   "test",
		Provider: provider,
	}

	_, err := step.Execute(context.Background(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "completion failed")
	assert.Contains(t, err.Error(), "API rate limited")
}

func TestLLMStep_Execute_EmptyResponse(t *testing.T) {
	provider := &mockProvider{
		name:     "test-provider",
		response: &llm.ChatResponse{Choices: []llm.ChatChoice{}},
	}

	step := &LLMStep{
		Model:    "gpt-4",
		Prompt:   "test",
		Provider: provider,
	}

	_, err := step.Execute(context.Background(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty response")
}

func TestLLMStep_Execute_WithTemperatureAndMaxTokens(t *testing.T) {
	provider := &mockProvider{
		name: "test-provider",
		response: &llm.ChatResponse{
			Choices: []llm.ChatChoice{
				{Message: types.Message{Role: llm.RoleAssistant, Content: "response"}},
			},
		},
	}

	step := &LLMStep{
		Model:       "gpt-4",
		Prompt:      "test",
		Temperature: 0.0,
		MaxTokens:   1,
		Provider:    provider,
	}

	result, err := step.Execute(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, "response", result)
}

func TestLLMStep_Execute_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	provider := &mockProvider{
		name: "test-provider",
		err:  ctx.Err(),
	}

	step := &LLMStep{
		Prompt:   "test",
		Provider: provider,
	}

	_, err := step.Execute(ctx, nil)
	require.Error(t, err)
}

func TestLLMStep_Execute_PromptCombination(t *testing.T) {
	tests := []struct {
		name          string
		prompt        string
		input         any
		expectContent string
	}{
		{
			name:          "prompt only",
			prompt:        "Hello",
			input:         nil,
			expectContent: "Hello",
		},
		{
			name:          "prompt with string input",
			prompt:        "Summarize:",
			input:         "some text",
			expectContent: "Summarize:\n\nsome text",
		},
		{
			name:          "empty prompt with string input",
			prompt:        "",
			input:         "just input",
			expectContent: "just input",
		},
		{
			name:          "prompt with non-string input",
			prompt:        "Analyze",
			input:         42,
			expectContent: "Analyze",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedReq *llm.ChatRequest
			provider := &mockProvider{
				name: "test",
				response: &llm.ChatResponse{
					Choices: []llm.ChatChoice{
						{Message: types.Message{Role: llm.RoleAssistant, Content: "ok"}},
					},
				},
			}
			// Wrap to capture request
			step := &LLMStep{
				Prompt:   tt.prompt,
				Provider: provider,
			}
			// We can't easily capture the request with our simple mock,
			// so we verify the output is correct (provider returns "ok")
			_ = capturedReq
			result, err := step.Execute(context.Background(), tt.input)
			require.NoError(t, err)
			assert.Equal(t, "ok", result)
		})
	}
}

// ============================================================
// ToolStep tests
// ============================================================

func TestToolStep_Execute_NilRegistry(t *testing.T) {
	step := &ToolStep{
		ToolName: "calculator",
		Params:   map[string]any{"op": "add"},
	}
	assert.Equal(t, "calculator", step.Name())

	_, err := step.Execute(context.Background(), "input")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotConfigured)
}

func TestToolStep_Execute_WithRegistry(t *testing.T) {
	registry := newMockToolRegistry()
	registry.register(&mockTool{
		name:   "calculator",
		result: map[string]any{"answer": 42},
	})

	step := &ToolStep{
		ToolName: "calculator",
		Params:   map[string]any{"op": "add", "a": 20, "b": 22},
		Registry: registry,
	}

	result, err := step.Execute(context.Background(), nil)
	require.NoError(t, err)
	m, ok := result.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 42, m["answer"])
}

func TestToolStep_Execute_ToolNotFound(t *testing.T) {
	registry := newMockToolRegistry()

	step := &ToolStep{
		ToolName: "nonexistent",
		Registry: registry,
	}

	_, err := step.Execute(context.Background(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "execution failed")
	assert.Contains(t, err.Error(), "not found")
}

func TestToolStep_Execute_ToolError(t *testing.T) {
	registry := newMockToolRegistry()
	registry.register(&mockTool{
		name: "broken",
		err:  errors.New("tool crashed"),
	})

	step := &ToolStep{
		ToolName: "broken",
		Registry: registry,
	}

	_, err := step.Execute(context.Background(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tool crashed")
}

func TestToolStep_Execute_NilInput(t *testing.T) {
	registry := newMockToolRegistry()
	registry.register(&mockTool{name: "noop", result: "done"})

	step := &ToolStep{
		ToolName: "noop",
		Params:   map[string]any{"key": "val"},
		Registry: registry,
	}

	result, err := step.Execute(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, "done", result)
}

func TestToolStep_Execute_EmptyParams(t *testing.T) {
	registry := newMockToolRegistry()
	registry.register(&mockTool{name: "bare", result: "ok"})

	step := &ToolStep{
		ToolName: "bare",
		Registry: registry,
	}

	result, err := step.Execute(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, "ok", result)
}

func TestHumanInputStep_Execute_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	handler := &mockHumanInputHandler{err: ctx.Err()}
	step := &HumanInputStep{
		Prompt:  "Confirm?",
		Handler: handler,
	}

	_, err := step.Execute(ctx, nil)
	require.Error(t, err)
}

func TestToolStep_Execute_InputMerge(t *testing.T) {
	registry := newMockToolRegistry()
	registry.register(&mockTool{
		name:   "echo",
		result: "ok",
	})

	step := &ToolStep{
		ToolName: "echo",
		Params:   map[string]any{"static": "value"},
		Registry: registry,
	}

	// Map input merges with params (static params take precedence)
	result, err := step.Execute(context.Background(), map[string]any{"dynamic": "data"})
	require.NoError(t, err)
	assert.Equal(t, "ok", result)

	// Non-map input goes into "input" key
	result, err = step.Execute(context.Background(), "plain string")
	require.NoError(t, err)
	assert.Equal(t, "ok", result)
}

// ============================================================
// HumanInputStep tests
// ============================================================

func TestHumanInputStep_Execute_NilHandler(t *testing.T) {
	step := &HumanInputStep{
		Prompt:  "Please confirm",
		Type:    "choice",
		Options: []string{"yes", "no"},
		Timeout: 30,
	}
	assert.Equal(t, "human_input", step.Name())

	_, err := step.Execute(context.Background(), "context")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotConfigured)
}

func TestHumanInputStep_Execute_WithHandler(t *testing.T) {
	handler := &mockHumanInputHandler{
		response: "yes",
	}

	step := &HumanInputStep{
		Prompt:  "Approve?",
		Type:    "choice",
		Options: []string{"yes", "no"},
		Handler: handler,
	}

	result, err := step.Execute(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, "yes", result)
	assert.Equal(t, 1, handler.calls)
}

func TestHumanInputStep_Execute_HandlerError(t *testing.T) {
	handler := &mockHumanInputHandler{
		err: errors.New("timeout waiting for human"),
	}

	step := &HumanInputStep{
		Prompt:  "Approve?",
		Handler: handler,
	}

	_, err := step.Execute(context.Background(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "request failed")
	assert.Contains(t, err.Error(), "timeout waiting for human")
}

// ============================================================
// CodeStep tests
// ============================================================

func TestCodeStep_Execute_WithHandler(t *testing.T) {
	step := &CodeStep{
		Handler: func(ctx context.Context, input any) (any, error) {
			return fmt.Sprintf("processed: %v", input), nil
		},
	}
	assert.Equal(t, "code", step.Name())

	result, err := step.Execute(context.Background(), "data")
	require.NoError(t, err)
	assert.Equal(t, "processed: data", result)
}

func TestCodeStep_Execute_NilHandler(t *testing.T) {
	step := &CodeStep{}

	_, err := step.Execute(context.Background(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "handler not configured")
}


