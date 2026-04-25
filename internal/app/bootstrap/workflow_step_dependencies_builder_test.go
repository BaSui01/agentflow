package bootstrap

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/agent/capabilities/planning"
	"github.com/BaSui01/agentflow/agent/integration/hosted"
	"github.com/BaSui01/agentflow/agent/observability/hitl"
	agent "github.com/BaSui01/agentflow/agent/runtime"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
	"github.com/BaSui01/agentflow/workflow/core"
	"github.com/BaSui01/agentflow/workflow/engine"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type workflowDepsTestAgent struct {
	id string
}

func (a *workflowDepsTestAgent) ID() string                     { return a.id }
func (a *workflowDepsTestAgent) Name() string                   { return a.id }
func (a *workflowDepsTestAgent) Type() agent.AgentType          { return agent.TypeGeneric }
func (a *workflowDepsTestAgent) State() agent.State             { return agent.StateReady }
func (a *workflowDepsTestAgent) Init(context.Context) error     { return nil }
func (a *workflowDepsTestAgent) Teardown(context.Context) error { return nil }
func (a *workflowDepsTestAgent) Plan(context.Context, *agent.Input) (*agent.PlanResult, error) {
	return nil, nil
}
func (a *workflowDepsTestAgent) Observe(context.Context, *agent.Feedback) error { return nil }
func (a *workflowDepsTestAgent) Execute(context.Context, *agent.Input) (*agent.Output, error) {
	return &agent.Output{Content: "ok"}, nil
}

type workflowDepsTestGateway struct {
	streamChunks []llmcore.UnifiedChunk
}

type workflowDepsAuthorizationService struct {
	decision *types.AuthorizationDecision
	requests []types.AuthorizationRequest
}

func (s *workflowDepsAuthorizationService) Authorize(_ context.Context, req types.AuthorizationRequest) (*types.AuthorizationDecision, error) {
	s.requests = append(s.requests, req)
	if s.decision != nil {
		return s.decision, nil
	}
	return &types.AuthorizationDecision{Decision: types.DecisionAllow, Reason: "test allow"}, nil
}

type workflowDepsHostedTool struct {
	name     string
	typ      hosted.HostedToolType
	executed *bool
}

func (t workflowDepsHostedTool) Type() hosted.HostedToolType {
	return t.typ
}

func (t workflowDepsHostedTool) Name() string {
	return t.name
}

func (t workflowDepsHostedTool) Description() string {
	return "test tool"
}

func (t workflowDepsHostedTool) Schema() types.ToolSchema {
	return types.ToolSchema{Name: t.name, Description: t.Description(), Parameters: json.RawMessage(`{"type":"object"}`)}
}

func (t workflowDepsHostedTool) Execute(context.Context, json.RawMessage) (json.RawMessage, error) {
	if t.executed != nil {
		*t.executed = true
	}
	return json.Marshal(map[string]any{"ok": true})
}

type workflowDepsCodeExecutor struct {
	executed *bool
	language *string
	code     *string
	timeout  *time.Duration
}

func (e workflowDepsCodeExecutor) Execute(_ context.Context, language string, code string, timeout time.Duration) (*hosted.CodeExecOutput, error) {
	if e.executed != nil {
		*e.executed = true
	}
	if e.language != nil {
		*e.language = language
	}
	if e.code != nil {
		*e.code = code
	}
	if e.timeout != nil {
		*e.timeout = timeout
	}
	return &hosted.CodeExecOutput{Stdout: "ok"}, nil
}

type workflowDepsInterruptRequester struct {
	called *bool
}

func (r workflowDepsInterruptRequester) RequestApproval(context.Context, planning.ApprovalRequest) (*planning.ApprovalResponse, error) {
	if r.called != nil {
		*r.called = true
	}
	return &planning.ApprovalResponse{Action: "approve", Feedback: "ok"}, nil
}

func (g workflowDepsTestGateway) Invoke(context.Context, *llmcore.UnifiedRequest) (*llmcore.UnifiedResponse, error) {
	return &llmcore.UnifiedResponse{
		Output: &llmcore.ChatResponse{
			Model: "test-model",
			Choices: []llmcore.ChatChoice{{
				Message: types.Message{Content: "ok"},
			}},
			Usage: types.ChatUsage{PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2},
		},
	}, nil
}

func (g workflowDepsTestGateway) Stream(context.Context, *llmcore.UnifiedRequest) (<-chan llmcore.UnifiedChunk, error) {
	ch := make(chan llmcore.UnifiedChunk, len(g.streamChunks))
	for _, chunk := range g.streamChunks {
		ch <- chunk
	}
	close(ch)
	return ch, nil
}

func TestBuildStepDependencies_InjectsAgentResolverForOrchestration(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	expected := &workflowDepsTestAgent{id: "agent-orch"}
	deps := buildStepDependencies(WorkflowRuntimeOptions{
		AgentResolver: func(ctx context.Context, agentID string) (agent.Agent, error) {
			require.Equal(t, "agent-orch", agentID)
			return expected, nil
		},
	}, zap.NewNop())

	require.NotNil(t, deps.AgentExecutor)
	require.NotNil(t, deps.AgentResolver)

	resolved, err := deps.AgentResolver.ResolveAgent(ctx, "agent-orch")
	require.NoError(t, err)
	require.Same(t, expected, resolved)

	node, err := engine.BuildExecutionNode(engine.StepSpec{
		ID:                     "orch-step",
		Type:                   core.StepTypeOrchestration,
		OrchestrationMode:      "round_robin",
		OrchestrationAgents:    []string{"agent-orch"},
		OrchestrationMaxRounds: 1,
	}, deps)
	require.NoError(t, err)
	require.NoError(t, node.Step.Validate())
}

func TestWorkflowGatewayAdapter_StreamMapsGatewayChunks(t *testing.T) {
	t.Parallel()

	reasoning := "plan"
	adapter := newWorkflowGatewayAdapter(workflowDepsTestGateway{
		streamChunks: []llmcore.UnifiedChunk{
			{
				Output: &llmcore.StreamChunk{
					Model: "test-model",
					Delta: types.Message{
						Content:          "hi",
						ReasoningContent: &reasoning,
					},
				},
			},
			{
				Usage: &llmcore.Usage{
					PromptTokens:     1,
					CompletionTokens: 2,
					TotalTokens:      3,
				},
			},
		},
	}, "fallback-model")

	stream, err := adapter.Stream(context.Background(), &core.LLMRequest{Model: "test-model", Prompt: "hello"})
	require.NoError(t, err)

	var chunks []core.LLMStreamChunk
	for chunk := range stream {
		chunks = append(chunks, chunk)
	}

	require.Len(t, chunks, 3)
	require.Equal(t, "hi", chunks[0].Delta)
	require.NotNil(t, chunks[0].ReasoningContent)
	require.Equal(t, "plan", *chunks[0].ReasoningContent)
	require.Equal(t, 3, chunks[2].Usage.TotalTokens)
	require.True(t, chunks[2].Done)
}

func TestHostedToolRegistryAdapter_AuthorizesBeforeToolExecution(t *testing.T) {
	t.Parallel()

	executed := false
	registry := hosted.NewToolRegistry(zap.NewNop())
	registry.Register(workflowDepsHostedTool{name: "run_command", typ: hosted.ToolTypeShell, executed: &executed})
	auth := &workflowDepsAuthorizationService{
		decision: &types.AuthorizationDecision{Decision: types.DecisionDeny, Reason: "blocked"},
	}
	adapter := hostedToolRegistryAdapter{registry: registry, authorization: auth}

	ctx := types.WithRunID(types.WithTraceID(types.WithAgentID(context.Background(), "agent-1"), "trace-1"), "run-1")
	_, err := adapter.ExecuteTool(ctx, "run_command", map[string]any{"cmd": "pwd"})

	require.ErrorContains(t, err, "authorization denied")
	require.False(t, executed)
	require.Len(t, auth.requests, 1)
	require.Equal(t, types.ResourceShell, auth.requests[0].ResourceKind)
	require.Equal(t, types.ActionExecute, auth.requests[0].Action)
	require.Equal(t, types.RiskExecution, auth.requests[0].RiskTier)
	require.Equal(t, "run-1", auth.requests[0].Context["run_id"])
}

func TestHostedToolRegistryAdapter_AuthorizesRawChainExecution(t *testing.T) {
	t.Parallel()

	executed := false
	registry := hosted.NewToolRegistry(zap.NewNop())
	registry.Register(workflowDepsHostedTool{name: "retrieval", typ: hosted.ToolTypeRetrieval, executed: &executed})
	auth := &workflowDepsAuthorizationService{}
	adapter := hostedToolRegistryAdapter{registry: registry, authorization: auth}

	raw, err := adapter.Execute(context.Background(), "retrieval", json.RawMessage(`{"query":"q"}`))

	require.NoError(t, err)
	require.JSONEq(t, `{"ok":true}`, string(raw))
	require.True(t, executed)
	require.Len(t, auth.requests, 1)
	require.Equal(t, types.ResourceTool, auth.requests[0].ResourceKind)
	require.Equal(t, types.RiskSafeRead, auth.requests[0].RiskTier)
}

func TestHostedCodeHandler_AuthorizesWithoutRawCode(t *testing.T) {
	t.Parallel()

	executed := false
	codeTool := hosted.NewCodeExecTool(hosted.CodeExecConfig{
		Executor: workflowDepsCodeExecutor{executed: &executed},
		Logger:   zap.NewNop(),
	})
	auth := &workflowDepsAuthorizationService{
		decision: &types.AuthorizationDecision{Decision: types.DecisionDeny, Reason: "code blocked"},
	}
	handler := hostedCodeHandler{tool: codeTool, authorization: auth}

	_, err := handler.Execute(context.Background(), core.StepInput{
		Data: map[string]any{
			"language": "python",
			"code":     "print('secret')",
		},
	})

	require.ErrorContains(t, err, "authorization denied")
	require.False(t, executed)
	require.Len(t, auth.requests, 1)
	req := auth.requests[0]
	require.Equal(t, types.ResourceCodeExec, req.ResourceKind)
	require.Equal(t, types.ActionExecute, req.Action)
	args, ok := req.Context["arguments"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "python", args["language"])
	require.NotEmpty(t, args["code_fingerprint"])
	require.NotContains(t, args, "code")
}

func TestHostedCodeHandler_RejectsOversizedCodeBeforeAuthorization(t *testing.T) {
	t.Parallel()

	executed := false
	codeTool := hosted.NewCodeExecTool(hosted.CodeExecConfig{
		Executor: workflowDepsCodeExecutor{executed: &executed},
		Logger:   zap.NewNop(),
	})
	auth := &workflowDepsAuthorizationService{}
	handler := hostedCodeHandler{
		tool:          codeTool,
		authorization: auth,
		policy: workflowCodeExecutionPolicy{
			MaxCodeBytes: 8,
		},
	}

	_, err := handler.Execute(context.Background(), core.StepInput{
		Data: map[string]any{
			"language": "python",
			"code":     strings.Repeat("x", 9),
		},
	})

	require.ErrorContains(t, err, "exceeds max size")
	require.False(t, executed)
	require.Empty(t, auth.requests)
}

func TestHostedCodeHandler_RejectsOversizedTimeoutBeforeAuthorization(t *testing.T) {
	t.Parallel()

	executed := false
	codeTool := hosted.NewCodeExecTool(hosted.CodeExecConfig{
		Executor: workflowDepsCodeExecutor{executed: &executed},
		Logger:   zap.NewNop(),
	})
	auth := &workflowDepsAuthorizationService{}
	handler := hostedCodeHandler{
		tool:          codeTool,
		authorization: auth,
		policy: workflowCodeExecutionPolicy{
			MaxTimeout: 2 * time.Second,
		},
	}

	_, err := handler.Execute(context.Background(), core.StepInput{
		Data: map[string]any{
			"language":        "python",
			"code":            "print('ok')",
			"timeout_seconds": float64(3),
		},
	})

	require.ErrorContains(t, err, "timeout_seconds exceeds max")
	require.False(t, executed)
	require.Empty(t, auth.requests)
}

func TestHostedCodeHandler_ForwardsTimeoutAndAuditsPolicyLimits(t *testing.T) {
	t.Parallel()

	executed := false
	var gotLanguage string
	var gotCode string
	var gotTimeout time.Duration
	codeTool := hosted.NewCodeExecTool(hosted.CodeExecConfig{
		Executor: workflowDepsCodeExecutor{
			executed: &executed,
			language: &gotLanguage,
			code:     &gotCode,
			timeout:  &gotTimeout,
		},
		Logger: zap.NewNop(),
	})
	auth := &workflowDepsAuthorizationService{}
	handler := hostedCodeHandler{
		tool:          codeTool,
		authorization: auth,
		policy: workflowCodeExecutionPolicy{
			MaxCodeBytes:        128,
			MaxTimeout:          7 * time.Second,
			DefaultTimeout:      4 * time.Second,
			MaxOutputBytes:      2048,
			AllowedLanguageTags: []string{"python"},
		},
	}

	out, err := handler.Execute(context.Background(), core.StepInput{
		Data: map[string]any{
			"language":        "python",
			"code":            "print('ok')",
			"timeout_seconds": json.Number("5"),
		},
	})

	require.NoError(t, err)
	require.True(t, executed)
	require.Equal(t, "python", gotLanguage)
	require.Equal(t, "print('ok')", gotCode)
	require.Equal(t, 5*time.Second, gotTimeout)
	require.Equal(t, "ok", out["stdout"])
	require.Len(t, auth.requests, 1)
	args, ok := auth.requests[0].Context["arguments"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, 5, args["timeout_seconds"])
	require.Equal(t, 128, args["max_code_bytes"])
	require.Equal(t, 7, args["max_timeout_seconds"])
	require.Equal(t, 2048, args["max_output_bytes"])
	require.Equal(t, []string{"python"}, args["allowed_languages"])
	require.NotContains(t, args, "code")
}

func TestHITLHumanInputHandler_AuthorizesBeforeRequestingInput(t *testing.T) {
	t.Parallel()

	called := false
	auth := &workflowDepsAuthorizationService{
		decision: &types.AuthorizationDecision{Decision: types.DecisionDeny, Reason: "human input blocked"},
	}
	handler := hitlHumanInputHandler{
		requester:     workflowDepsInterruptRequester{called: &called},
		authorization: auth,
	}

	_, err := handler.RequestInput(context.Background(), "approve?", "approval", []string{"yes"})

	require.ErrorContains(t, err, "authorization denied")
	require.False(t, called)
	require.Len(t, auth.requests, 1)
	require.Equal(t, types.ResourceWorkflow, auth.requests[0].ResourceKind)
	require.Equal(t, types.ActionApprove, auth.requests[0].Action)
}

func TestBuildStepDependencies_DoesNotRegisterWorkflowAutoApprove(t *testing.T) {
	t.Parallel()

	manager := hitl.NewInterruptManager(hitl.NewInMemoryInterruptStore(), zap.NewNop())
	_ = buildStepDependencies(WorkflowRuntimeOptions{HITLManager: manager}, zap.NewNop())

	resp, err := manager.CreateInterrupt(context.Background(), hitl.InterruptOptions{
		Type:    hitl.InterruptTypeApproval,
		Title:   "manual approval",
		Timeout: 5 * time.Millisecond,
	})

	require.Nil(t, resp)
	require.ErrorContains(t, err, "interrupt timeout")
}
