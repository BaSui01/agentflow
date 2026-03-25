package agent

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/agent/reasoning"
	"github.com/BaSui01/agentflow/llm"
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
	ba := NewBaseAgent(testAgentConfig("test-1", "Test", ""), nil, nil, nil, nil, zap.NewNop(), nil)

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
	ba := NewBaseAgent(cfg, &testProvider{name: "test"}, nil, nil, nil, zap.NewNop(), nil)

	err := ba.ValidateConfiguration()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reflection enabled but executor not set")
	assert.Contains(t, err.Error(), "tool selection enabled but selector not set")
}

func TestBaseAgent_Execute_DefaultClosedLoopInvokesPlanAndObserve(t *testing.T) {
	logger := zap.NewNop()
	var completionCalls int
	bus := &testEventBus{}
	provider := &testProvider{
		name: "mock",
		completionFn: func(_ context.Context, _ *llm.ChatRequest) (*llm.ChatResponse, error) {
			completionCalls++
			switch completionCalls {
			case 1:
				return &llm.ChatResponse{
					Provider: "mock",
					Model:    "gpt-4",
					Choices: []llm.ChatChoice{{
						Message: types.Message{Role: llm.RoleAssistant, Content: "1. inspect\n2. answer"},
					}},
				}, nil
			case 2:
				return &llm.ChatResponse{
					Provider: "mock",
					Model:    "gpt-4",
					Choices: []llm.ChatChoice{{
						FinishReason: "stop",
						Message:      types.Message{Role: llm.RoleAssistant, Content: "closed-loop output"},
					}},
					Usage: llm.ChatUsage{TotalTokens: 11},
				}, nil
			default:
				return nil, fmt.Errorf("unexpected completion call %d", completionCalls)
			}
		},
	}

	ag := NewBaseAgent(testAgentConfig("loop-agent", "LoopAgent", "gpt-4"), provider, &testMemoryManager{}, &testToolManager{}, bus, logger, nil)
	require.NoError(t, ag.Init(context.Background()))
	ag.SetCompletionJudge(NewDefaultCompletionJudge())
	ag.SetReasoningModeSelector(NewDefaultReasoningModeSelector())

	output, err := ag.Execute(context.Background(), &Input{TraceID: "trace-loop", Content: "solve this"})
	require.NoError(t, err)
	require.NotNil(t, output)
	assert.Equal(t, 2, completionCalls)
	assert.Equal(t, "closed-loop output", output.Content)
	assert.Equal(t, 1, output.IterationCount)
	assert.Equal(t, string(LoopStageEvaluate), output.CurrentStage)
	assert.Equal(t, ReasoningModeReact, output.SelectedReasoningMode)
	require.NotNil(t, output.Metadata)
	assert.Equal(t, 1, output.Metadata["loop_iteration_count"])
	assert.Equal(t, StopReasonSolved, output.Metadata["loop_stop_reason"])

	var feedbackSeen bool
	for _, event := range bus.published {
		if _, ok := event.(*FeedbackEvent); ok {
			feedbackSeen = true
			break
		}
	}
	assert.True(t, feedbackSeen, "expected Observe to publish a feedback event in the default closed loop")
}

func TestBaseAgent_Execute_DefaultClosedLoopConsumesReasoningRegistry(t *testing.T) {
	logger := zap.NewNop()
	var completionCalls int
	provider := &testProvider{
		name: "mock",
		completionFn: func(_ context.Context, _ *llm.ChatRequest) (*llm.ChatResponse, error) {
			completionCalls++
			return &llm.ChatResponse{
				Provider: "mock",
				Model:    "gpt-4",
				Choices: []llm.ChatChoice{{
					Message: types.Message{Role: llm.RoleAssistant, Content: "1. inspect\n2. execute"},
				}},
			}, nil
		},
	}

	registry := reasoning.NewPatternRegistry()
	require.NoError(t, registry.Register(integrationReasoningPatternStub{name: ReasoningModePlanAndExecute}))

	ag := NewBaseAgent(testAgentConfig("reasoning-agent", "ReasoningAgent", "gpt-4"), provider, &testMemoryManager{}, &testToolManager{}, &testEventBus{}, logger, nil)
	require.NoError(t, ag.Init(context.Background()))
	ag.SetReasoningRegistry(registry)
	ag.SetReasoningModeSelector(NewDefaultReasoningModeSelector())
	ag.SetCompletionJudge(NewDefaultCompletionJudge())

	output, err := ag.Execute(context.Background(), &Input{
		TraceID: "trace-reasoning",
		Content: "break down and execute this workflow",
		Context: map[string]any{"complex_task": true},
	})
	require.NoError(t, err)
	require.NotNil(t, output)
	assert.Equal(t, 1, completionCalls, "planner should consume the provider once and act should be handled by the reasoning registry")
	assert.Equal(t, ReasoningModePlanAndExecute, output.SelectedReasoningMode)
	assert.Equal(t, "pattern:plan_and_execute", output.Content)
	require.NotNil(t, output.Metadata)
	assert.Equal(t, "plan_and_execute", output.Metadata["reasoning_pattern"])
}

func TestBaseAgent_Execute_DefaultClosedLoopReflectsWithinMainChain(t *testing.T) {
	logger := zap.NewNop()
	var completionCalls int
	provider := &testProvider{
		name: "mock",
		completionFn: func(_ context.Context, _ *llm.ChatRequest) (*llm.ChatResponse, error) {
			completionCalls++
			switch completionCalls {
			case 1:
				return &llm.ChatResponse{
					Provider: "mock",
					Model:    "gpt-4",
					Choices: []llm.ChatChoice{{
						Message: types.Message{Role: llm.RoleAssistant, Content: "1. diagnose\n2. fix"},
					}},
				}, nil
			case 2:
				return &llm.ChatResponse{
					Provider: "mock",
					Model:    "gpt-4",
					Choices: []llm.ChatChoice{{
						FinishReason: "stop",
						Message:      types.Message{Role: llm.RoleAssistant, Content: "draft answer"},
					}},
				}, nil
			case 3:
				return &llm.ChatResponse{
					Provider: "mock",
					Model:    "gpt-4",
					Choices: []llm.ChatChoice{{
						Message: types.Message{Role: llm.RoleAssistant, Content: "1. verify\n2. finalize"},
					}},
				}, nil
			case 4:
				return &llm.ChatResponse{
					Provider: "mock",
					Model:    "gpt-4",
					Choices: []llm.ChatChoice{{
						FinishReason: "stop",
						Message:      types.Message{Role: llm.RoleAssistant, Content: "refined answer"},
					}},
					Usage: llm.ChatUsage{TotalTokens: 21},
				}, nil
			default:
				return nil, fmt.Errorf("unexpected completion call %d", completionCalls)
			}
		},
	}

	judge := &integrationCompletionJudgeStub{decisions: []*CompletionDecision{
		{Decision: LoopDecisionReflect, NeedReflection: true, StopReason: StopReasonBlocked, Reason: "reflect before finalizing"},
		{Decision: LoopDecisionDone, Solved: true, StopReason: StopReasonSolved, Reason: "resolved"},
	}}

	cfg := testAgentConfig("reflect-agent", "ReflectAgent", "gpt-4")
	cfg.Runtime.MaxReActIterations = 1
	ag := NewBaseAgent(cfg, provider, &testMemoryManager{}, &testToolManager{}, &testEventBus{}, logger, nil)
	require.NoError(t, ag.Init(context.Background()))
	ag.EnableReflection(&integrationLoopReflectionRunner{})
	ag.SetCompletionJudge(judge)
	ag.SetReasoningModeSelector(NewDefaultReasoningModeSelector())
	ag.config.Features.Reflection = &types.ReflectionConfig{Enabled: true, MaxIterations: 2}

	output, err := ag.Execute(context.Background(), &Input{TraceID: "trace-reflect", Content: "improve this answer"})
	require.NoError(t, err)
	require.NotNil(t, output)
	assert.Equal(t, 4, completionCalls)
	assert.Equal(t, "refined answer", output.Content)
	require.NotNil(t, output.Metadata)
	assert.Equal(t, 1, output.Metadata["reflection_iterations"])
	assert.Equal(t, StopReasonSolved, output.Metadata["loop_stop_reason"])
}

func TestBaseAgent_Execute_DefaultClosedLoopNeedsValidationAndToolVerificationBeforeSolved(t *testing.T) {
	logger := zap.NewNop()
	provider := &testProvider{
		name: "mock",
		completionFn: func(_ context.Context, _ *llm.ChatRequest) (*llm.ChatResponse, error) {
			return &llm.ChatResponse{
				Provider: "mock",
				Model:    "gpt-4",
				Choices: []llm.ChatChoice{{
					FinishReason: "stop",
					Message:      types.Message{Role: llm.RoleAssistant, Content: "candidate answer"},
				}},
			}, nil
		},
		streamFn: func(_ context.Context, _ *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
			ch := make(chan llm.StreamChunk, 1)
			ch <- llm.StreamChunk{
				Delta:        types.Message{Role: llm.RoleAssistant, Content: "candidate answer"},
				FinishReason: "stop",
			}
			close(ch)
			return ch, nil
		},
	}

	ag := NewBaseAgent(testAgentConfig("validate-agent", "ValidateAgent", "gpt-4"), provider, &testMemoryManager{}, &testToolManager{}, &testEventBus{}, logger, nil)
	require.NoError(t, ag.Init(context.Background()))
	ag.SetCompletionJudge(NewDefaultCompletionJudge())
	ag.SetReasoningModeSelector(NewDefaultReasoningModeSelector())

	output, err := ag.Execute(context.Background(), &Input{
		TraceID: "trace-validate",
		Content: "respond only after acceptance criteria are validated",
		Context: map[string]any{
			"acceptance_criteria":        []string{"must be verified"},
			"tool_verification_required": true,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, output)
	if output.StopReason == string(StopReasonSolved) {
		t.Fatalf("expected default closed loop to require validation/acceptance before solved")
	}
	if got := output.Metadata["loop_stop_reason"]; got == StopReasonSolved || got == string(StopReasonSolved) {
		t.Fatalf("expected default closed loop metadata to remain unsolved before validation")
	}
}

func TestBaseAgent_Execute_DefaultClosedLoopKeepsCodeTaskOpenWithoutVerificationEvidence(t *testing.T) {
	logger := zap.NewNop()
	provider := &testProvider{
		name: "mock",
		completionFn: func(_ context.Context, _ *llm.ChatRequest) (*llm.ChatResponse, error) {
			return &llm.ChatResponse{
				Provider: "mock",
				Model:    "gpt-4",
				Choices: []llm.ChatChoice{{
					FinishReason: "stop",
					Message:      types.Message{Role: llm.RoleAssistant, Content: "implemented the fix"},
				}},
			}, nil
		},
	}

	ag := NewBaseAgent(testAgentConfig("code-validate-agent", "CodeValidateAgent", "gpt-4"), provider, &testMemoryManager{}, &testToolManager{}, &testEventBus{}, logger, nil)
	require.NoError(t, ag.Init(context.Background()))
	ag.SetCompletionJudge(NewDefaultCompletionJudge())
	ag.SetReasoningModeSelector(NewDefaultReasoningModeSelector())

	output, err := ag.Execute(context.Background(), &Input{
		TraceID: "trace-code-validate",
		Content: "fix the Go bug and verify the result",
		Context: map[string]any{
			"task_type": "code",
		},
	})
	require.NoError(t, err)
	require.NotNil(t, output)
	assert.Equal(t, string(StopReasonMaxIterations), output.StopReason)
	assert.Equal(t, "pending", output.Metadata["validation_status"])
}

func TestBaseAgent_Execute_DefaultClosedLoopHonorsRunConfigMaxLoopIterations(t *testing.T) {
	logger := zap.NewNop()
	var completionCalls int
	provider := &testProvider{
		name: "mock",
		completionFn: func(_ context.Context, _ *llm.ChatRequest) (*llm.ChatResponse, error) {
			completionCalls++
			return &llm.ChatResponse{
				Provider: "mock",
				Model:    "gpt-4",
				Choices: []llm.ChatChoice{{
					FinishReason: "stop",
					Message:      types.Message{Role: llm.RoleAssistant, Content: "still working"},
				}},
			}, nil
		},
	}

	ag := NewBaseAgent(testAgentConfig("budget-agent", "BudgetAgent", "gpt-4"), provider, &testMemoryManager{}, &testToolManager{}, &testEventBus{}, logger, nil)
	require.NoError(t, ag.Init(context.Background()))
	ag.SetCompletionJudge(&integrationCompletionJudgeStub{decisions: []*CompletionDecision{
		{Decision: LoopDecisionContinue, Reason: "need another pass"},
		{Decision: LoopDecisionContinue, Reason: "need another pass"},
	}})
	ag.SetReasoningModeSelector(NewDefaultReasoningModeSelector())

	output, err := ag.Execute(context.Background(), &Input{
		TraceID: "trace-budget",
		Content: "bounded task",
		Overrides: &RunConfig{
			MaxLoopIterations: IntPtr(1),
		},
	})
	require.NoError(t, err)
	require.NotNil(t, output)
	assert.Equal(t, 1, output.IterationCount)
	assert.Equal(t, string(StopReasonMaxIterations), output.StopReason)
	assert.Equal(t, 2, completionCalls)
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
	return NewBaseAgent(testAgentConfig("test-agent", "TestAgent", ""), &testProvider{name: "test"}, nil, nil, nil, zap.NewNop(), nil)
}

type integTestReflectionRunner struct{}

func (r *integTestReflectionRunner) ExecuteWithReflection(ctx context.Context, input *Input) (*Output, error) {
	return nil, nil
}

type integrationLoopReflectionRunner struct{}

func (r *integrationLoopReflectionRunner) ExecuteWithReflection(ctx context.Context, input *Input) (*Output, error) {
	return nil, nil
}

func (r *integrationLoopReflectionRunner) ReflectStep(_ context.Context, _ *Input, _ *Output, _ *LoopState) (*LoopReflectionResult, error) {
	return &LoopReflectionResult{
		NextInput: &Input{TraceID: "trace-reflect", Content: "refined prompt"},
		Observation: &LoopObservation{
			Metadata: map[string]any{
				"reflection_critique": Critique{Score: 0.45, IsGood: false},
			},
		},
	}, nil
}

type integrationCompletionJudgeStub struct {
	decisions []*CompletionDecision
	index     int
}

func (s *integrationCompletionJudgeStub) Judge(_ context.Context, _ *LoopState, _ *Output, _ error) (*CompletionDecision, error) {
	if s.index >= len(s.decisions) {
		return &CompletionDecision{Decision: LoopDecisionDone, Solved: true, StopReason: StopReasonSolved}, nil
	}
	decision := s.decisions[s.index]
	s.index++
	return decision, nil
}

type integTestToolSelectorRunner struct{}

func (r *integTestToolSelectorRunner) SelectTools(ctx context.Context, task string, tools []types.ToolSchema) ([]types.ToolSchema, error) {
	return nil, nil
}

type integTestPromptEnhancerRunner struct{}

func (r *integTestPromptEnhancerRunner) EnhanceUserPrompt(prompt, ctx string) (string, error) {
	return prompt, nil
}

type integTestSkillDiscoverer struct{}

func (r *integTestSkillDiscoverer) DiscoverSkills(ctx context.Context, task string) ([]*types.DiscoveredSkill, error) {
	return nil, nil
}

type integTestMCPServer struct{}

type integTestLSPClient struct{}

func (r *integTestLSPClient) Shutdown(ctx context.Context) error { return nil }

type integTestLSPLifecycle struct{}

func (r *integTestLSPLifecycle) Close() error { return nil }

type integTestEnhancedMemory struct{}

func (r *integTestEnhancedMemory) LoadWorking(ctx context.Context, agentID string) ([]types.MemoryEntry, error) {
	return nil, nil
}
func (r *integTestEnhancedMemory) LoadShortTerm(ctx context.Context, agentID string, limit int) ([]types.MemoryEntry, error) {
	return nil, nil
}
func (r *integTestEnhancedMemory) SaveShortTerm(ctx context.Context, agentID, content string, metadata map[string]any) error {
	return nil
}
func (r *integTestEnhancedMemory) RecordEpisode(ctx context.Context, event *types.EpisodicEvent) error {
	return nil
}

type integTestObservability struct{}

func (r *integTestObservability) StartTrace(traceID, agentID string)         {}
func (r *integTestObservability) EndTrace(traceID, status string, err error) {}
func (r *integTestObservability) RecordTask(agentID string, success bool, d time.Duration, tokens int, cost, quality float64) {
}

type integrationReasoningPatternStub struct {
	name string
}

func (s integrationReasoningPatternStub) Execute(context.Context, string) (*reasoning.ReasoningResult, error) {
	return &reasoning.ReasoningResult{
		Pattern:     s.name,
		Task:        "integration-test",
		FinalAnswer: "pattern:" + s.name,
		Confidence:  0.88,
		Steps: []reasoning.ReasoningStep{
			{StepID: "step-1", Type: "plan", Content: "use reasoning registry"},
		},
		Metadata: map[string]any{
			"source": "integration-test",
		},
	}, nil
}

func (s integrationReasoningPatternStub) Name() string {
	return s.name
}
