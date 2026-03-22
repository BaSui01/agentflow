package agent

import (
	"context"
	"testing"
	"time"

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
