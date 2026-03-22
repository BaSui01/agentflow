package agent

import (
	"context"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// ═══ AsyncExecutor + SubagentManager 补充测试 ═══

func TestSubagentManager_Close(t *testing.T) {
	mgr := NewSubagentManager(zap.NewNop())
	// Close 应该不 panic，可以多次调用
	mgr.Close()
	mgr.Close() // 第二次调用不应 panic
}

func TestAsyncExecutor_ExecuteWithSubagents_AllFail(t *testing.T) {
	mainAgent := &testSimpleAgent{id: "main", output: "main result"}
	exec := NewAsyncExecutor(mainAgent, zap.NewNop())

	failAgent1 := &testFailAgent{id: "fail1"}
	failAgent2 := &testFailAgent{id: "fail2"}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := exec.ExecuteWithSubagents(ctx, &Input{TraceID: "t1", Content: "test"}, []Agent{failAgent1, failAgent2})
	if err == nil {
		t.Fatal("expected error when all subagents fail")
	}
}

func TestAsyncExecutor_ExecuteWithSubagents_Success(t *testing.T) {
	mainAgent := &testSimpleAgent{id: "main", output: "main"}
	exec := NewAsyncExecutor(mainAgent, zap.NewNop())

	sub1 := &testSimpleAgent{id: "sub1", output: "result1"}
	sub2 := &testSimpleAgent{id: "sub2", output: "result2"}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	output, err := exec.ExecuteWithSubagents(ctx, &Input{TraceID: "t1", Content: "test"}, []Agent{sub1, sub2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output == nil || output.Content == "" {
		t.Fatal("expected non-empty output")
	}
	if output.TokensUsed != 20 { // 10 + 10
		t.Fatalf("expected 20 tokens, got %d", output.TokensUsed)
	}
}

func TestAsyncExecutor_ExecuteWithSubagents_PartialFail(t *testing.T) {
	mainAgent := &testSimpleAgent{id: "main", output: "main"}
	exec := NewAsyncExecutor(mainAgent, zap.NewNop())

	sub1 := &testSimpleAgent{id: "sub1", output: "ok"}
	sub2 := &testFailAgent{id: "fail1"}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	output, err := exec.ExecuteWithSubagents(ctx, &Input{TraceID: "t1", Content: "test"}, []Agent{sub1, sub2})
	if err != nil {
		t.Fatalf("partial fail should still return result: %v", err)
	}
	if output == nil {
		t.Fatal("expected output from successful subagent")
	}
}

func TestCopyInput_WithMaps(t *testing.T) {
	input := &Input{
		TraceID:   "t1",
		Content:   "hello",
		Context:   map[string]any{"key": "value"},
		Variables: map[string]string{"var": "val"},
	}
	copied := copyInput(input)
	if copied.TraceID != "t1" || copied.Content != "hello" {
		t.Fatal("basic fields not copied")
	}
	if copied.Context["key"] != "value" {
		t.Fatal("context not copied")
	}
	if copied.Variables["var"] != "val" {
		t.Fatal("variables not copied")
	}
	// 修改原始不应影响副本
	input.Context["key"] = "changed"
	if copied.Context["key"] == "changed" {
		t.Fatal("context should be deep copied")
	}
}

// ═══ Builder 补充测试 ═══

func TestAgentBuilder_WithLedger(t *testing.T) {
	b := NewAgentBuilder(testConfig("ledger-test"))
	b.WithLedger(nil) // nil ledger 不应 panic
	if len(b.errors) > 0 {
		t.Fatalf("unexpected errors: %v", b.errors)
	}
}

func TestAgentBuilder_WithSkills(t *testing.T) {
	b := NewAgentBuilder(testConfig("skills-test"))
	b.WithSkills(nil) // nil skills 不应 panic
}

func TestAgentBuilder_WithPromptStore(t *testing.T) {
	b := NewAgentBuilder(testConfig("prompt-test"))
	b.WithPromptStore(nil)
}

func TestAgentBuilder_WithConversationStore(t *testing.T) {
	b := NewAgentBuilder(testConfig("conv-test"))
	b.WithConversationStore(nil)
}

func TestAgentBuilder_WithRunStore(t *testing.T) {
	b := NewAgentBuilder(testConfig("run-test"))
	b.WithRunStore(nil)
}

func TestAgentBuilder_WithOrchestrator(t *testing.T) {
	b := NewAgentBuilder(testConfig("orch-test"))
	b.WithOrchestrator(nil)
	if b.Orchestrator() != nil {
		t.Fatal("expected nil orchestrator")
	}
}

func TestAgentBuilder_WithReasoning(t *testing.T) {
	b := NewAgentBuilder(testConfig("reason-test"))
	b.WithReasoning(nil)
	if b.ReasoningRegistry() != nil {
		t.Fatal("expected nil reasoning registry")
	}
}

// ═══ 辅助类型 ═══

func testConfig(id string) types.AgentConfig {
	return types.AgentConfig{
		Core: types.CoreConfig{ID: id, Name: id, Type: "assistant"},
		LLM:  types.LLMConfig{Model: "test"},
	}
}

type testSimpleAgent struct {
	id     string
	output string
}

func (a *testSimpleAgent) ID() string        { return a.id }
func (a *testSimpleAgent) Name() string      { return a.id }
func (a *testSimpleAgent) Type() AgentType   { return "test" }
func (a *testSimpleAgent) State() State      { return "ready" }
func (a *testSimpleAgent) Init(_ context.Context) error { return nil }
func (a *testSimpleAgent) Teardown(_ context.Context) error { return nil }
func (a *testSimpleAgent) Plan(_ context.Context, _ *Input) (*PlanResult, error) { return nil, nil }
func (a *testSimpleAgent) Execute(_ context.Context, input *Input) (*Output, error) {
	return &Output{TraceID: input.TraceID, Content: a.output, TokensUsed: 10, Duration: time.Millisecond}, nil
}
func (a *testSimpleAgent) Observe(_ context.Context, _ *Feedback) error { return nil }

type testFailAgent struct {
	id string
}

func (a *testFailAgent) ID() string        { return a.id }
func (a *testFailAgent) Name() string      { return a.id }
func (a *testFailAgent) Type() AgentType   { return "test" }
func (a *testFailAgent) State() State      { return "ready" }
func (a *testFailAgent) Init(_ context.Context) error { return nil }
func (a *testFailAgent) Teardown(_ context.Context) error { return nil }
func (a *testFailAgent) Plan(_ context.Context, _ *Input) (*PlanResult, error) { return nil, nil }
func (a *testFailAgent) Execute(_ context.Context, _ *Input) (*Output, error) {
	return nil, NewError("EXEC_FAILED", "模拟执行失败")
}
func (a *testFailAgent) Observe(_ context.Context, _ *Feedback) error { return nil }

// ═══ Builder enable* 方法补充测试 ═══

func TestAgentBuilder_WithDefaultSkills(t *testing.T) {
	b := NewAgentBuilder(testConfig("skills-default"))
	b.WithDefaultSkills("", nil)
}

func TestAgentBuilder_BuildWithoutProvider(t *testing.T) {
	b := NewAgentBuilder(testConfig("no-provider"))
	b.WithLogger(zap.NewNop())
	_, err := b.Build()
	if err == nil {
		t.Fatal("expected error without provider")
	}
}

func TestAgentBuilder_BuildWithoutLogger(t *testing.T) {
	b := NewAgentBuilder(testConfig("no-logger"))
	_, err := b.Build()
	if err == nil {
		t.Fatal("expected error without logger")
	}
}

func TestAgentBuilder_BuildMinimal(t *testing.T) {
	b := NewAgentBuilder(testConfig("minimal"))
	b.WithProvider(&testMockProvider{})
	b.WithLogger(zap.NewNop())
	ag, err := b.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if ag == nil {
		t.Fatal("expected non-nil agent")
	}
	if ag.ID() != "minimal" {
		t.Fatalf("expected id=minimal, got %s", ag.ID())
	}
}

// ═══ BaseAgent 状态和生命周期测试 ═══

func TestBaseAgent_InitAndTeardown(t *testing.T) {
	ag := buildTestAgent(t, "lifecycle")
	ctx := context.Background()

	if err := ag.Init(ctx); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	if ag.State() != StateReady {
		t.Fatalf("expected state=ready after Init, got %s", ag.State())
	}

	if err := ag.Teardown(ctx); err != nil {
		t.Fatalf("Teardown failed: %v", err)
	}
}

func TestBaseAgent_TryLockExec(t *testing.T) {
	ag := buildTestAgent(t, "lock-test")
	ag.Init(context.Background())

	// 第一次锁应该成功
	if !ag.TryLockExec() {
		t.Fatal("first lock should succeed")
	}
	// 第二次锁应该失败（已锁定）
	if ag.TryLockExec() {
		t.Fatal("second lock should fail")
	}
	ag.UnlockExec()
	// 解锁后应该能再次锁定
	if !ag.TryLockExec() {
		t.Fatal("lock after unlock should succeed")
	}
	ag.UnlockExec()
}

func TestBaseAgent_SetGateway(t *testing.T) {
	ag := buildTestAgent(t, "gateway-test")
	ag.SetGateway(nil) // nil 不应 panic
}

func TestBaseAgent_Tools(t *testing.T) {
	ag := buildTestAgent(t, "tools-test")
	// 无 ToolManager 时返回 nil
	tm := ag.Tools()
	if tm != nil {
		t.Fatal("expected nil ToolManager")
	}
}

func TestBaseAgent_Logger(t *testing.T) {
	ag := buildTestAgent(t, "logger-test")
	if ag.Logger() == nil {
		t.Fatal("expected non-nil logger")
	}
}

// ═══ 辅助构建函数 ═══

func buildTestAgent(t *testing.T, id string) *BaseAgent {
	t.Helper()
	b := NewAgentBuilder(testConfig(id))
	b.WithProvider(&testMockProvider{})
	b.WithLogger(zap.NewNop())
	ag, err := b.Build()
	if err != nil {
		t.Fatalf("buildTestAgent failed: %v", err)
	}
	return ag
}

type testMockProvider struct{}

func (p *testMockProvider) Name() string { return "test-mock" }
func (p *testMockProvider) Completion(_ context.Context, _ *llm.ChatRequest) (*llm.ChatResponse, error) {
	return &llm.ChatResponse{Choices: []llm.ChatChoice{{Message: types.Message{Content: "mock"}}}}, nil
}
func (p *testMockProvider) Stream(_ context.Context, _ *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	ch := make(chan llm.StreamChunk, 1); close(ch); return ch, nil
}
func (p *testMockProvider) HealthCheck(_ context.Context) (*llm.HealthStatus, error) {
	return &llm.HealthStatus{Healthy: true}, nil
}
func (p *testMockProvider) SupportsNativeFunctionCalling() bool { return true }
func (p *testMockProvider) ListModels(_ context.Context) ([]llm.Model, error) { return nil, nil }
func (p *testMockProvider) Endpoints() llm.ProviderEndpoints { return llm.ProviderEndpoints{} }
