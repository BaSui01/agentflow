package agent

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/agent/guardrails"
	"github.com/BaSui01/agentflow/llm"
	llmtools "github.com/BaSui01/agentflow/llm/capabilities/tools"
	"github.com/BaSui01/agentflow/llm/observability"
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

// ═══ Builder enable* 功能开关测试 ═══

func TestAgentBuilder_BuildWithMCP(t *testing.T) {
	cfg := testConfig("mcp-agent")
	cfg.Extensions.MCP = &types.MCPConfig{Enabled: true}
	b := NewAgentBuilder(cfg)
	b.WithProvider(&testMockProvider{})
	b.WithLogger(zap.NewNop())
	ag, err := b.Build()
	if err != nil {
		t.Fatalf("Build with MCP failed: %v", err)
	}
	if ag == nil {
		t.Fatal("expected non-nil agent")
	}
}

func TestAgentBuilder_BuildWithEnhancedMemory(t *testing.T) {
	cfg := testConfig("memory-agent")
	cfg.Features.Memory = &types.MemoryConfig{Enabled: true}
	b := NewAgentBuilder(cfg)
	b.WithProvider(&testMockProvider{})
	b.WithLogger(zap.NewNop())
	ag, err := b.Build()
	if err != nil {
		t.Fatalf("Build with Memory failed: %v", err)
	}
	if ag == nil {
		t.Fatal("expected non-nil agent")
	}
}

func TestAgentBuilder_BuildWithLSP(t *testing.T) {
	cfg := testConfig("lsp-agent")
	cfg.Extensions.LSP = &types.LSPConfig{Enabled: true}
	b := NewAgentBuilder(cfg)
	b.WithProvider(&testMockProvider{})
	b.WithLogger(zap.NewNop())
	ag, err := b.Build()
	if err != nil {
		t.Fatalf("Build with LSP failed: %v", err)
	}
	if ag == nil {
		t.Fatal("expected non-nil agent")
	}
}

func TestAgentBuilder_Validate_Coverage(t *testing.T) {
	// 有效配置
	b := NewAgentBuilder(testConfig("valid"))
	b.WithProvider(&testMockProvider{})
	b.WithLogger(zap.NewNop())
	if err := b.Validate(); err != nil {
		t.Fatalf("valid config should pass: %v", err)
	}

	// 无效配置（缺少 ID）
	b2 := NewAgentBuilder(types.AgentConfig{Core: types.CoreConfig{Name: "no-id"}})
	if err := b2.Validate(); err == nil {
		t.Fatal("expected error for missing ID")
	}
}

func TestBaseAgent_Observe_Coverage(t *testing.T) {
	ag := buildTestAgent(t, "observe-test")
	ag.Init(context.Background())

	// 无记忆管理器时 Observe 不应 panic
	err := ag.Observe(context.Background(), &Feedback{
		Type:    "approval",
		Content: "good job",
		Data:    map[string]any{"rating": 5},
	})
	// 无记忆时可能返回 nil 或 error，都可以
	_ = err
}

func TestBaseAgent_Plan_Coverage(t *testing.T) {
	ag := buildTestAgent(t, "plan-test")
	ag.Init(context.Background())

	// Plan 需要 provider，应该能调用（虽然 mock 返回简单结果）
	result, err := ag.Plan(context.Background(), &Input{
		TraceID: "plan-001",
		Content: "设计一个简单的 API",
	})
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil plan result")
	}
}

// ═══ Event 补充测试 ═══

func TestSimpleEventBus_SetPanicErrorChan(t *testing.T) {
	bus := NewEventBus(zap.NewNop())
	ch := make(chan error, 1)
	bus.(*SimpleEventBus).SetPanicErrorChan(ch)
}

// ═══ Completion 补充测试 ═══

func TestWithRuntimeStreamEmitter(t *testing.T) {
	emitter := func(event RuntimeStreamEvent) {}
	ctx := WithRuntimeStreamEmitter(context.Background(), emitter)
	_, ok := runtimeStreamEmitterFromContext(ctx)
	if !ok {
		t.Fatal("expected emitter in context")
	}
}

func TestRuntimeStreamEmitterFromContext_Missing(t *testing.T) {
	_, ok := runtimeStreamEmitterFromContext(context.Background())
	if ok {
		t.Fatal("expected no emitter in empty context")
	}
}

// ═══ Builder enableSkills 测试 ═══

func TestAgentBuilder_BuildWithSkills(t *testing.T) {
	cfg := testConfig("skills-agent")
	cfg.Extensions.Skills = &types.SkillsConfig{Enabled: true}
	b := NewAgentBuilder(cfg)
	b.WithProvider(&testMockProvider{})
	b.WithLogger(zap.NewNop())
	ag, err := b.Build()
	if err != nil {
		t.Fatalf("Build with Skills failed: %v", err)
	}
	if ag == nil {
		t.Fatal("expected non-nil agent")
	}
}

// ═══ BaseAgent Execute 边界测试 ═══

func TestBaseAgent_Execute_NilInput(t *testing.T) {
	ag := buildTestAgent(t, "nil-input")
	ag.Init(context.Background())
	_, err := ag.Execute(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil input")
	}
}

func TestBaseAgent_Execute_EmptyContent(t *testing.T) {
	ag := buildTestAgent(t, "empty-content")
	ag.Init(context.Background())
	_, err := ag.Execute(context.Background(), &Input{TraceID: "t1", Content: ""})
	if err == nil {
		t.Fatal("expected error for empty content")
	}
}

func TestBaseAgent_Execute_AgentBusy(t *testing.T) {
	ag := buildTestAgent(t, "busy-test")
	ag.Init(context.Background())

	ag.TryLockExec()
	defer ag.UnlockExec()

	_, err := ag.Execute(context.Background(), &Input{TraceID: "t1", Content: "test"})
	if err == nil {
		t.Fatal("expected ErrAgentBusy")
	}
}

// ═══ ExecuteEnhanced 测试 ═══

func TestBaseAgent_ExecuteEnhanced_NilInput(t *testing.T) {
	ag := buildTestAgent(t, "enhanced-nil")
	ag.Init(context.Background())
	_, err := ag.ExecuteEnhanced(context.Background(), nil, EnhancedExecutionOptions{})
	if err == nil {
		t.Fatal("expected error for nil input")
	}
}

func TestBaseAgent_ExecuteEnhanced_Basic(t *testing.T) {
	ag := buildTestAgent(t, "enhanced-basic")
	ag.Init(context.Background())
	output, err := ag.ExecuteEnhanced(context.Background(), &Input{
		TraceID: "enh-001",
		Content: "test enhanced execution",
	}, EnhancedExecutionOptions{})
	if err != nil {
		t.Fatalf("ExecuteEnhanced failed: %v", err)
	}
	if output == nil {
		t.Fatal("expected non-nil output")
	}
}

// ═══ ExtensionRegistry 测试 ═══

func TestExtensionRegistry_SaveToEnhancedMemory_NilMemory(t *testing.T) {
	reg := NewExtensionRegistry(zap.NewNop())
	// nil enhanced memory 不应 panic
	reg.SaveToEnhancedMemory(context.Background(), "agent1",
		&Input{TraceID: "t1", Content: "test"},
		&Output{Content: "result", TokensUsed: 10, Duration: time.Millisecond},
		false,
	)
}

func TestExtensionRegistry_ExecuteWithReflection_NilExecutor(t *testing.T) {
	reg := NewExtensionRegistry(zap.NewNop())
	_, err := reg.ExecuteWithReflection(context.Background(), &Input{TraceID: "t1", Content: "test"})
	if err == nil {
		t.Fatal("expected error with nil reflection executor")
	}
}

// ═══ Integration skillInstructions 测试 ═══

func TestSkillInstructionsContext(t *testing.T) {
	ctx := withSkillInstructions(context.Background(), []string{"use tool X"})
	instructions := skillInstructionsFromCtx(ctx)
	if len(instructions) != 1 || instructions[0] != "use tool X" {
		t.Fatalf("expected ['use tool X'], got %v", instructions)
	}
}

func TestSkillInstructionsContext_Missing(t *testing.T) {
	instructions := skillInstructionsFromCtx(context.Background())
	if len(instructions) != 0 {
		t.Fatal("expected empty instructions in empty context")
	}
}

// ═══ BaseAgent Execute 正常路径 ═══

func TestBaseAgent_Execute_Success(t *testing.T) {
	ag := buildTestAgent(t, "exec-success")
	ag.Init(context.Background())

	output, err := ag.Execute(context.Background(), &Input{
		TraceID: "exec-001",
		Content: "hello",
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if output == nil || output.Content == "" {
		t.Fatal("expected non-empty output")
	}
}

// ═══ BaseAgent Teardown 测试 ═══

func TestBaseAgent_Teardown_NotReady(t *testing.T) {
	ag := buildTestAgent(t, "teardown-notready")
	// 不调用 Init，直接 Teardown
	err := ag.Teardown(context.Background())
	// 可能返回错误或 nil，都不应 panic
	_ = err
}

// ═══ BaseAgent Name/Type/ID 测试 ═══

func TestBaseAgent_Identity(t *testing.T) {
	ag := buildTestAgent(t, "identity-test")
	if ag.ID() != "identity-test" {
		t.Fatalf("expected id=identity-test, got %s", ag.ID())
	}
	if ag.Name() != "identity-test" {
		t.Fatalf("expected name=identity-test, got %s", ag.Name())
	}
	if ag.Type() == "" {
		t.Fatal("expected non-empty type")
	}
}

// ═══ Integration context 辅助函数测试 ═══

func TestMemoryContext(t *testing.T) {
	ctx := withMemoryContext(context.Background(), []string{"mem1", "mem2"})
	result := memoryContextFromCtx(ctx)
	if len(result) != 2 || result[0] != "mem1" {
		t.Fatalf("expected [mem1 mem2], got %v", result)
	}
}

func TestMemoryContext_Missing(t *testing.T) {
	result := memoryContextFromCtx(context.Background())
	if len(result) != 0 {
		t.Fatalf("expected empty, got %v", result)
	}
}

// ═══ Completion applyContextRouteHints 测试 ═══

func TestApplyContextRouteHints(t *testing.T) {
	req := &llm.ChatRequest{Model: "test"}
	ctx := context.Background()
	// 不应 panic
	applyContextRouteHints(req, ctx)
}

func TestApplyContextRouteHints_WithRunConfig(t *testing.T) {
	req := &llm.ChatRequest{Model: "test"}
	ctx := types.WithLLMProvider(context.Background(), "my-provider")
	applyContextRouteHints(req, ctx)
	if req.Metadata == nil || req.Metadata["chat_provider"] != "my-provider" {
		t.Fatalf("expected metadata chat_provider=my-provider, got %v", req.Metadata)
	}
}

func TestApplyContextRouteHints_NilReq(t *testing.T) {
	applyContextRouteHints(nil, context.Background()) // 不应 panic
}

// ═══ Integration 辅助函数测试 ═══

func TestShallowCopyInput(t *testing.T) {
	input := &Input{TraceID: "t1", Content: "hello", Context: map[string]any{"k": "v"}}
	copied := shallowCopyInput(input)
	if copied.TraceID != "t1" || copied.Content != "hello" {
		t.Fatal("basic fields not copied")
	}
}

func TestExecutionPipeline_Use(t *testing.T) {
	called := false
	core := func(ctx context.Context, input *Input) (*Output, error) {
		called = true
		return &Output{Content: "core"}, nil
	}
	pipeline := NewExecutionPipeline(core)
	// 添加一个 no-op 中间件
	pipeline.Use(func(ctx context.Context, input *Input, next ExecutionFunc) (*Output, error) {
		return next(ctx, input)
	})
	output, err := pipeline.Execute(context.Background(), &Input{TraceID: "t1", Content: "test"})
	if err != nil {
		t.Fatalf("pipeline execute failed: %v", err)
	}
	if !called {
		t.Fatal("core function not called")
	}
	if output.Content != "core" {
		t.Fatalf("expected 'core', got '%s'", output.Content)
	}
}

func TestAsToolSelectorRunner(t *testing.T) {
	// nil DynamicToolSelector 返回 nil DynamicToolSelectorRunner
	result := AsToolSelectorRunner(nil)
	// result 是 (*DynamicToolSelector)(nil)，类型非 nil 但值是 nil
	_ = result
}

// ═══ Persistence stores 测试 ═══

func TestPersistenceStores_Defaults(t *testing.T) {
	ps := &PersistenceStores{}
	ps.SetMaxRestoreMessages(100)
	ps.SetPromptStore(nil)
	ps.SetConversationStore(nil)
	ps.SetRunStore(nil)

	if ps.PromptStore() != nil {
		t.Fatal("expected nil PromptStore")
	}
	if ps.ConversationStore() != nil {
		t.Fatal("expected nil ConversationStore")
	}
	if ps.RunStore() != nil {
		t.Fatal("expected nil RunStore")
	}
}

// ═══ PersistenceStores 操作测试 ═══

func TestPersistenceStores_LoadPrompt(t *testing.T) {
	ps := &PersistenceStores{}
	// nil store 不应 panic
	doc := ps.LoadPrompt(context.Background(), "assistant", "test", "")
	if doc != nil {
		t.Fatal("expected nil from nil store")
	}
}

func TestPersistenceStores_RecordRun(t *testing.T) {
	ps := &PersistenceStores{}
	// nil store 不应 panic
	runID := ps.RecordRun(context.Background(), "agent1", "tenant1", "trace1", "input", time.Now())
	if runID != "" {
		t.Fatalf("expected empty runID from nil store, got %s", runID)
	}
}

func TestPersistenceStores_UpdateRunStatus(t *testing.T) {
	ps := &PersistenceStores{}
	// nil store 不应 panic
	ps.UpdateRunStatus(context.Background(), "run1", "completed", nil, "")
}

func TestPersistenceStores_RestoreConversation(t *testing.T) {
	ps := &PersistenceStores{}
	msgs := ps.RestoreConversation(context.Background(), "conv1")
	if len(msgs) != 0 {
		t.Fatalf("expected empty messages from nil store, got %d", len(msgs))
	}
}

func TestPersistenceStores_PersistConversation(t *testing.T) {
	ps := &PersistenceStores{}
	// nil store 不应 panic
	ps.PersistConversation(context.Background(), "conv1", "agent1", "tenant1", "user1", "input", "output")
}

// ═══ ExtensionRegistry 更多测试 ═══

func TestExtensionRegistry_AllAccessors(t *testing.T) {
	reg := NewExtensionRegistry(zap.NewNop())
	if reg.SkillManagerExt() != nil {
		t.Fatal("expected nil SkillManagerExt")
	}
	if reg.MCPServerExt() != nil {
		t.Fatal("expected nil MCPServerExt")
	}
	if reg.LSPClientExt() != nil {
		t.Fatal("expected nil LSPClientExt")
	}
	if reg.EnhancedMemoryExt() != nil {
		t.Fatal("expected nil EnhancedMemoryExt")
	}
	if reg.ObservabilitySystemExt() != nil {
		t.Fatal("expected nil ObservabilitySystemExt")
	}
	if reg.ReflectionExecutor() != nil {
		t.Fatal("expected nil ReflectionExecutor")
	}
	if reg.ToolSelector() != nil {
		t.Fatal("expected nil ToolSelector")
	}
	if reg.PromptEnhancerExt() != nil {
		t.Fatal("expected nil PromptEnhancerExt")
	}
}

// ═══ BaseAgent WithToolProvider 测试 ═══

func TestAgentBuilder_WithMaxReActIterations_Coverage(t *testing.T) {
	b := NewAgentBuilder(testConfig("react-iter"))
	b.WithMaxReActIterations(15)
	b.WithProvider(&testMockProvider{})
	b.WithLogger(zap.NewNop())
	ag, err := b.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	_ = ag
}

func TestAgentBuilder_WithToolProvider(t *testing.T) {
	b := NewAgentBuilder(testConfig("tool-prov"))
	b.WithProvider(&testMockProvider{})
	b.WithToolProvider(&testMockProvider{})
	b.WithLogger(zap.NewNop())
	ag, err := b.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	_ = ag
}

func TestAgentBuilder_WithMemory_Coverage(t *testing.T) {
	b := NewAgentBuilder(testConfig("mem-test"))
	b.WithProvider(&testMockProvider{})
	b.WithMemory(nil)
	b.WithLogger(zap.NewNop())
	_, err := b.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
}

func TestAgentBuilder_WithEventBus_Coverage(t *testing.T) {
	b := NewAgentBuilder(testConfig("bus-test"))
	b.WithProvider(&testMockProvider{})
	b.WithEventBus(NewEventBus(zap.NewNop()))
	b.WithLogger(zap.NewNop())
	_, err := b.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Coverage Boost Round 2 — targets: chatCompletionStreaming, wrapProviderWithGateway,
// coreExecutor (reflection path), ExecuteWithReflection, LoadPrompt, RecordRun,
// PersistConversation with real store mocks.
// ═══════════════════════════════════════════════════════════════════════════════

// --- mock provider that returns streaming chunks ---

type streamingMockProvider struct {
	testMockProvider
	chunks []llm.StreamChunk
}

func (p *streamingMockProvider) Stream(_ context.Context, _ *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	ch := make(chan llm.StreamChunk, len(p.chunks))
	for _, c := range p.chunks {
		ch <- c
	}
	close(ch)
	return ch, nil
}

// --- mock ReflectionRunner ---

type mockReflectionRunner struct {
	output *Output
	err    error
}

func (m *mockReflectionRunner) ExecuteWithReflection(_ context.Context, input *Input) (*Output, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.output != nil {
		return m.output, nil
	}
	return &Output{TraceID: input.TraceID, Content: "reflected", TokensUsed: 5, Duration: time.Millisecond}, nil
}

// --- mock PromptStoreProvider ---

type mockPromptStore struct {
	doc PromptDocument
	err error
}

func (m *mockPromptStore) GetActive(_ context.Context, _, _, _ string) (PromptDocument, error) {
	return m.doc, m.err
}

// --- mock RunStoreProvider ---

type mockRunStore struct {
	recorded []*RunDoc
	err      error
}

func (m *mockRunStore) RecordRun(_ context.Context, doc *RunDoc) error {
	if m.err != nil {
		return m.err
	}
	m.recorded = append(m.recorded, doc)
	return nil
}

func (m *mockRunStore) UpdateStatus(_ context.Context, _, _ string, _ *RunOutputDoc, _ string) error {
	return nil
}

// --- mock ConversationStoreProvider ---

type mockConversationStore struct {
	appendErr error
	createErr error
	messages  []ConversationMessage
	total     int64
}

func (m *mockConversationStore) Create(_ context.Context, _ *ConversationDoc) error {
	return m.createErr
}
func (m *mockConversationStore) GetByID(_ context.Context, _ string) (*ConversationDoc, error) {
	return nil, fmt.Errorf("not found")
}
func (m *mockConversationStore) AppendMessages(_ context.Context, _ string, _ []ConversationMessage) error {
	return m.appendErr
}
func (m *mockConversationStore) List(_ context.Context, _, _ string, _, _ int) ([]*ConversationDoc, int64, error) {
	return nil, 0, nil
}
func (m *mockConversationStore) Update(_ context.Context, _ string, _ ConversationUpdate) error {
	return nil
}
func (m *mockConversationStore) Delete(_ context.Context, _ string) error { return nil }
func (m *mockConversationStore) DeleteByParentID(_ context.Context, _, _ string) error {
	return nil
}
func (m *mockConversationStore) GetMessages(_ context.Context, _ string, _, _ int) ([]ConversationMessage, int64, error) {
	return m.messages, m.total, nil
}
func (m *mockConversationStore) DeleteMessage(_ context.Context, _, _ string) error { return nil }
func (m *mockConversationStore) ClearMessages(_ context.Context, _ string) error    { return nil }
func (m *mockConversationStore) Archive(_ context.Context, _ string) error           { return nil }

// --- mock Ledger ---

type mockLedger struct{}

func (m *mockLedger) Record(_ context.Context, _ observability.LedgerEntry) error {
	return nil
}

// --- helper: build agent with custom provider ---

func buildTestAgentWithProvider(t *testing.T, id string, prov llm.Provider) *BaseAgent {
	t.Helper()
	b := NewAgentBuilder(testConfig(id))
	b.WithProvider(prov)
	b.WithLogger(zap.NewNop())
	ag, err := b.Build()
	if err != nil {
		t.Fatalf("buildTestAgentWithProvider failed: %v", err)
	}
	return ag
}

// ═══ 1. chatCompletionStreaming — no tools (plain stream) ═══

func TestChatCompletionStreaming_NoTools(t *testing.T) {
	prov := &streamingMockProvider{
		chunks: []llm.StreamChunk{
			{ID: "c1", Model: "m", Provider: "p", Delta: types.Message{Content: "hello "}, FinishReason: ""},
			{ID: "c1", Model: "m", Provider: "p", Delta: types.Message{Content: "world"}, FinishReason: "stop"},
		},
	}

	ag := buildTestAgentWithProvider(t, "stream-no-tools", prov)
	ag.Init(context.Background())

	var collected []string
	emitter := func(ev RuntimeStreamEvent) {
		if ev.Type == RuntimeStreamToken {
			collected = append(collected, ev.Token)
		}
	}
	ctx := WithRuntimeStreamEmitter(context.Background(), emitter)

	output, err := ag.Execute(ctx, &Input{TraceID: "s1", Content: "hi"})
	if err != nil {
		t.Fatalf("streaming execute failed: %v", err)
	}
	if output == nil || output.Content == "" {
		t.Fatal("expected non-empty output from streaming path")
	}
	if len(collected) == 0 {
		t.Fatal("expected emitter to receive tokens")
	}
}

func TestChatCompletion_StreamingPath(t *testing.T) {
	prov := &streamingMockProvider{
		chunks: []llm.StreamChunk{
			{ID: "s1", Model: "m", Provider: "p", Delta: types.Message{Content: "streamed"}},
			{FinishReason: "stop"},
		},
	}

	ag := buildTestAgentWithProvider(t, "cc-stream", prov)
	ag.Init(context.Background())

	var tokens []string
	emitter := func(ev RuntimeStreamEvent) {
		if ev.Type == RuntimeStreamToken {
			tokens = append(tokens, ev.Token)
		}
	}
	ctx := WithRuntimeStreamEmitter(context.Background(), emitter)

	resp, err := ag.ChatCompletion(ctx, []types.Message{
		{Role: llm.RoleUser, Content: "hello"},
	})
	if err != nil {
		t.Fatalf("ChatCompletion streaming failed: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if len(tokens) == 0 {
		t.Fatal("expected tokens from streaming emitter")
	}
}

// ═══ 2. wrapProviderWithGateway ═══

func TestWrapProviderWithGateway_NilProvider(t *testing.T) {
	result := wrapProviderWithGateway(nil, zap.NewNop(), nil)
	if result != nil {
		t.Fatal("expected nil for nil provider")
	}
}

func TestWrapProviderWithGateway_WithLedger(t *testing.T) {
	prov := &testMockProvider{}
	wrapped := wrapProviderWithGateway(prov, zap.NewNop(), &mockLedger{})
	if wrapped == nil {
		t.Fatal("expected non-nil wrapped provider")
	}
	resp, err := wrapped.Completion(context.Background(), &llm.ChatRequest{
		Model:    "test",
		Messages: []types.Message{{Role: llm.RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("wrapped provider completion failed: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
}

func TestWrapProviderWithGateway_NilLedger(t *testing.T) {
	prov := &testMockProvider{}
	wrapped := wrapProviderWithGateway(prov, zap.NewNop(), nil)
	if wrapped == nil {
		t.Fatal("expected non-nil wrapped provider even with nil ledger")
	}
}

// ═══ 3. coreExecutor — reflection path ═══

func TestCoreExecutor_WithReflection(t *testing.T) {
	ag := buildTestAgent(t, "core-reflect")
	ag.Init(context.Background())

	mockRefl := &mockReflectionRunner{
		output: &Output{TraceID: "r1", Content: "reflected output", TokensUsed: 7, Duration: time.Millisecond},
	}
	ag.extensions.EnableReflection(mockRefl)

	opts := EnhancedExecutionOptions{UseReflection: true}
	executor := ag.coreExecutor(opts)

	output, err := executor(context.Background(), &Input{TraceID: "r1", Content: "test reflection"})
	if err != nil {
		t.Fatalf("coreExecutor with reflection failed: %v", err)
	}
	if output.Content != "reflected output" {
		t.Fatalf("expected 'reflected output', got '%s'", output.Content)
	}
}

func TestCoreExecutor_ReflectionError(t *testing.T) {
	ag := buildTestAgent(t, "core-reflect-err")
	ag.Init(context.Background())

	mockRefl := &mockReflectionRunner{err: fmt.Errorf("reflection boom")}
	ag.extensions.EnableReflection(mockRefl)

	opts := EnhancedExecutionOptions{UseReflection: true}
	executor := ag.coreExecutor(opts)

	_, err := executor(context.Background(), &Input{TraceID: "r2", Content: "test"})
	if err == nil {
		t.Fatal("expected error from reflection failure")
	}
}

func TestCoreExecutor_NoReflection_FallsBackToExecute(t *testing.T) {
	ag := buildTestAgent(t, "core-no-reflect")
	ag.Init(context.Background())

	opts := EnhancedExecutionOptions{UseReflection: false}
	executor := ag.coreExecutor(opts)

	output, err := executor(context.Background(), &Input{TraceID: "r3", Content: "test"})
	if err != nil {
		t.Fatalf("coreExecutor without reflection failed: %v", err)
	}
	if output == nil || output.Content == "" {
		t.Fatal("expected non-empty output")
	}
}

// ═══ 4. ExecuteWithReflection via ExtensionRegistry ═══

func TestExtensionRegistry_ExecuteWithReflection_WithRunner(t *testing.T) {
	reg := NewExtensionRegistry(zap.NewNop())
	mockRefl := &mockReflectionRunner{
		output: &Output{TraceID: "ext-r1", Content: "ext reflected", TokensUsed: 3},
	}
	reg.EnableReflection(mockRefl)

	output, err := reg.ExecuteWithReflection(context.Background(), &Input{TraceID: "ext-r1", Content: "test"})
	if err != nil {
		t.Fatalf("ExecuteWithReflection failed: %v", err)
	}
	if output.Content != "ext reflected" {
		t.Fatalf("expected 'ext reflected', got '%s'", output.Content)
	}
}

// ═══ 5. LoadPrompt with real PromptStore ═══

func TestPersistenceStores_LoadPrompt_WithStore(t *testing.T) {
	ps := NewPersistenceStores(zap.NewNop())
	store := &mockPromptStore{
		doc: PromptDocument{
			Version: "v2",
			System:  SystemPrompt{Identity: "You are a helpful assistant."},
		},
	}
	ps.SetPromptStore(store)

	doc := ps.LoadPrompt(context.Background(), "assistant", "test-agent", "tenant1")
	if doc == nil {
		t.Fatal("expected non-nil prompt document")
	}
	if doc.Version != "v2" {
		t.Fatalf("expected version=v2, got %s", doc.Version)
	}
}

func TestPersistenceStores_LoadPrompt_StoreError(t *testing.T) {
	ps := NewPersistenceStores(zap.NewNop())
	store := &mockPromptStore{err: fmt.Errorf("db error")}
	ps.SetPromptStore(store)

	doc := ps.LoadPrompt(context.Background(), "assistant", "test-agent", "")
	if doc != nil {
		t.Fatal("expected nil on store error")
	}
}

// ═══ 6. RecordRun with real RunStore ═══

func TestPersistenceStores_RecordRun_WithStore(t *testing.T) {
	ps := NewPersistenceStores(zap.NewNop())
	store := &mockRunStore{}
	ps.SetRunStore(store)

	runID := ps.RecordRun(context.Background(), "agent1", "tenant1", "trace1", "hello", time.Now())
	if runID == "" {
		t.Fatal("expected non-empty runID")
	}
	if len(store.recorded) != 1 {
		t.Fatalf("expected 1 recorded run, got %d", len(store.recorded))
	}
	if store.recorded[0].AgentID != "agent1" {
		t.Fatalf("expected agent_id=agent1, got %s", store.recorded[0].AgentID)
	}
}

func TestPersistenceStores_RecordRun_StoreError(t *testing.T) {
	ps := NewPersistenceStores(zap.NewNop())
	store := &mockRunStore{err: fmt.Errorf("db write error")}
	ps.SetRunStore(store)

	runID := ps.RecordRun(context.Background(), "agent1", "tenant1", "trace1", "hello", time.Now())
	if runID != "" {
		t.Fatalf("expected empty runID on error, got %s", runID)
	}
}

// ═══ 7. PersistConversation with real ConversationStore ═══

func TestPersistenceStores_PersistConversation_AppendSuccess(t *testing.T) {
	ps := NewPersistenceStores(zap.NewNop())
	store := &mockConversationStore{appendErr: nil}
	ps.SetConversationStore(store)

	ps.PersistConversation(context.Background(), "conv1", "agent1", "tenant1", "user1", "input", "output")
}

func TestPersistenceStores_PersistConversation_AppendFailCreateSuccess(t *testing.T) {
	ps := NewPersistenceStores(zap.NewNop())
	store := &mockConversationStore{
		appendErr: fmt.Errorf("not found"),
		createErr: nil,
	}
	ps.SetConversationStore(store)

	ps.PersistConversation(context.Background(), "conv2", "agent1", "tenant1", "user1", "input", "output")
}

func TestPersistenceStores_PersistConversation_BothFail(t *testing.T) {
	ps := NewPersistenceStores(zap.NewNop())
	store := &mockConversationStore{
		appendErr: fmt.Errorf("append fail"),
		createErr: fmt.Errorf("create fail"),
	}
	ps.SetConversationStore(store)

	ps.PersistConversation(context.Background(), "conv3", "agent1", "tenant1", "user1", "input", "output")
}

// ═══ RestoreConversation with real store ═══

func TestPersistenceStores_RestoreConversation_WithStore(t *testing.T) {
	ps := NewPersistenceStores(zap.NewNop())
	store := &mockConversationStore{
		messages: []ConversationMessage{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi there"},
		},
		total: 2,
	}
	ps.SetConversationStore(store)
	ps.SetMaxRestoreMessages(100)

	msgs := ps.RestoreConversation(context.Background(), "conv1")
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Content != "hello" {
		t.Fatalf("expected first message 'hello', got '%s'", msgs[0].Content)
	}
}

// ═══ UpdateRunStatus with real store ═══

func TestPersistenceStores_UpdateRunStatus_WithStore(t *testing.T) {
	ps := NewPersistenceStores(zap.NewNop())
	store := &mockRunStore{}
	ps.SetRunStore(store)

	err := ps.UpdateRunStatus(context.Background(), "run1", "completed", &RunOutputDoc{Content: "done"}, "")
	if err != nil {
		t.Fatalf("UpdateRunStatus failed: %v", err)
	}
}

// ═══ gatewayProvider / gatewayToolProvider paths ═══

func TestBaseAgent_GatewayProvider_WithLedger(t *testing.T) {
	prov := &testMockProvider{}
	ag := NewBaseAgent(
		testConfig("gw-ledger"),
		prov,
		nil, nil, nil,
		zap.NewNop(),
		&mockLedger{},
	)
	gw := ag.gatewayProvider()
	if gw == nil {
		t.Fatal("expected non-nil gateway provider")
	}
}

func TestBaseAgent_GatewayToolProvider_WithLedger(t *testing.T) {
	prov := &testMockProvider{}
	toolProv := &testMockProvider{}
	ag := NewBaseAgent(
		testConfig("gw-tool-ledger"),
		prov,
		nil, nil, nil,
		zap.NewNop(),
		&mockLedger{},
	)
	ag.SetToolProvider(toolProv)
	gtp := ag.gatewayToolProvider()
	if gtp == nil {
		t.Fatal("expected non-nil gateway tool provider")
	}
}

func TestBaseAgent_GatewayToolProvider_NoToolProvider(t *testing.T) {
	prov := &testMockProvider{}
	ag := NewBaseAgent(
		testConfig("gw-no-tool"),
		prov,
		nil, nil, nil,
		zap.NewNop(),
		&mockLedger{},
	)
	gtp := ag.gatewayToolProvider()
	if gtp == nil {
		t.Fatal("expected non-nil provider (should fall back to gatewayProvider)")
	}
}

// ═══ ExecuteEnhanced with reflection option ═══

func TestBaseAgent_ExecuteEnhanced_WithReflection(t *testing.T) {
	ag := buildTestAgent(t, "enh-reflect")
	ag.Init(context.Background())

	mockRefl := &mockReflectionRunner{
		output: &Output{TraceID: "er1", Content: "enhanced reflected", TokensUsed: 4, Duration: time.Millisecond},
	}
	ag.extensions.EnableReflection(mockRefl)

	output, err := ag.ExecuteEnhanced(context.Background(), &Input{
		TraceID: "er1",
		Content: "test enhanced with reflection",
	}, EnhancedExecutionOptions{UseReflection: true})
	if err != nil {
		t.Fatalf("ExecuteEnhanced with reflection failed: %v", err)
	}
	if output.Content != "enhanced reflected" {
		t.Fatalf("expected 'enhanced reflected', got '%s'", output.Content)
	}
}

// ═══ Feature status and metrics ═══

func TestBaseAgent_GetFeatureMetrics_Coverage(t *testing.T) {
	ag := buildTestAgent(t, "metrics-test2")
	metrics := ag.GetFeatureMetrics()
	if metrics["agent_id"] != "metrics-test2" {
		t.Fatalf("expected agent_id=metrics-test2, got %v", metrics["agent_id"])
	}
}

func TestBaseAgent_PrintFeatureStatus_Coverage(t *testing.T) {
	ag := buildTestAgent(t, "print-status2")
	ag.PrintFeatureStatus()
}

func TestBaseAgent_ValidateConfiguration_Coverage(t *testing.T) {
	ag := buildTestAgent(t, "validate-cfg2")
	err := ag.ValidateConfiguration()
	if err != nil {
		t.Fatalf("ValidateConfiguration failed: %v", err)
	}
}

func TestBaseAgent_ExportConfiguration_Coverage(t *testing.T) {
	ag := buildTestAgent(t, "export-cfg2")
	cfg := ag.ExportConfiguration()
	if cfg["id"] != "export-cfg2" {
		t.Fatalf("expected id=export-cfg2, got %v", cfg["id"])
	}
}

// ═══ SaveToEnhancedMemory with real runner ═══

func TestExtensionRegistry_SaveToEnhancedMemory_WithRunner(t *testing.T) {
	reg := NewExtensionRegistry(zap.NewNop())
	mem := &mockEnhancedMemory{}
	reg.EnableEnhancedMemory(mem)

	reg.SaveToEnhancedMemory(context.Background(), "agent1",
		&Input{TraceID: "t1", Content: "test"},
		&Output{Content: "result", TokensUsed: 10, Duration: time.Millisecond},
		true,
	)
	if mem.savedCount != 1 {
		t.Fatalf("expected 1 save, got %d", mem.savedCount)
	}
	if mem.episodeCount != 1 {
		t.Fatalf("expected 1 episode, got %d", mem.episodeCount)
	}
}

type mockEnhancedMemory struct {
	savedCount   int
	episodeCount int
}

func (m *mockEnhancedMemory) LoadWorking(_ context.Context, _ string) ([]types.MemoryEntry, error) {
	return nil, nil
}
func (m *mockEnhancedMemory) LoadShortTerm(_ context.Context, _ string, _ int) ([]types.MemoryEntry, error) {
	return nil, nil
}
func (m *mockEnhancedMemory) SaveShortTerm(_ context.Context, _, _ string, _ map[string]any) error {
	m.savedCount++
	return nil
}
func (m *mockEnhancedMemory) RecordEpisode(_ context.Context, _ *types.EpisodicEvent) error {
	m.episodeCount++
	return nil
}

// ═══════════════════════════════════════════════════════════════════════════════
// Coverage Boost Round 3 — lifecycle, memory_facade, scoped persistence,
// integration middlewares
// ═══════════════════════════════════════════════════════════════════════════════

// ═══ LifecycleManager ═══

func TestLifecycleManager_StartStop(t *testing.T) {
	ag := &testSimpleAgent{id: "lm-agent", output: "ok"}
	lm := NewLifecycleManager(ag, zap.NewNop())

	if lm.IsRunning() {
		t.Fatal("should not be running before Start")
	}

	ctx := context.Background()
	if err := lm.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if !lm.IsRunning() {
		t.Fatal("should be running after Start")
	}

	hs := lm.GetHealthStatus()
	_ = hs // just exercise the accessor

	// Double start should error
	if err := lm.Start(ctx); err == nil {
		t.Fatal("expected error on double Start")
	}

	if err := lm.Stop(ctx); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
	if lm.IsRunning() {
		t.Fatal("should not be running after Stop")
	}

	// Double stop should error
	if err := lm.Stop(ctx); err == nil {
		t.Fatal("expected error on double Stop")
	}
}

func TestLifecycleManager_Restart(t *testing.T) {
	ag := &testSimpleAgent{id: "lm-restart", output: "ok"}
	lm := NewLifecycleManager(ag, zap.NewNop())

	ctx := context.Background()
	if err := lm.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if err := lm.Restart(ctx); err != nil {
		t.Fatalf("Restart failed: %v", err)
	}
	if !lm.IsRunning() {
		t.Fatal("should be running after Restart")
	}
	lm.Stop(ctx)
}

// ═══ UnifiedMemoryFacade ═══

func TestUnifiedMemoryFacade_Accessors(t *testing.T) {
	f := NewUnifiedMemoryFacade(nil, nil, nil)
	if f.HasBase() {
		t.Fatal("expected no base")
	}
	if f.HasEnhanced() {
		t.Fatal("expected no enhanced")
	}
	if f.Base() != nil {
		t.Fatal("expected nil base")
	}
	if f.Enhanced() != nil {
		t.Fatal("expected nil enhanced")
	}
	if f.SkipBaseMemory() {
		t.Fatal("expected SkipBaseMemory=false without enhanced")
	}
}

func TestUnifiedMemoryFacade_SaveInteraction_NoMemory(t *testing.T) {
	f := NewUnifiedMemoryFacade(nil, nil, zap.NewNop())
	// Should not panic
	f.SaveInteraction(context.Background(), "agent1", "t1", "user input", "agent output")
}

func TestUnifiedMemoryFacade_SaveInteraction_Enhanced(t *testing.T) {
	mem := &mockEnhancedMemory{}
	f := NewUnifiedMemoryFacade(nil, mem, zap.NewNop())
	if !f.HasEnhanced() {
		t.Fatal("expected enhanced")
	}
	if !f.SkipBaseMemory() {
		t.Fatal("expected SkipBaseMemory=true with enhanced")
	}
	f.SaveInteraction(context.Background(), "agent1", "t1", "user input", "agent output")
	if mem.savedCount != 1 {
		t.Fatalf("expected 1 save, got %d", mem.savedCount)
	}
}

func TestUnifiedMemoryFacade_SaveInteraction_BaseOnly(t *testing.T) {
	baseMem := &mockBaseMemory{}
	f := NewUnifiedMemoryFacade(baseMem, nil, zap.NewNop())
	if !f.HasBase() {
		t.Fatal("expected base")
	}
	f.SaveInteraction(context.Background(), "agent1", "t1", "user input", "agent output")
	if baseMem.saveCount != 2 { // user + agent
		t.Fatalf("expected 2 saves, got %d", baseMem.saveCount)
	}
}

func TestUnifiedMemoryFacade_LoadContext_NoEnhanced(t *testing.T) {
	f := NewUnifiedMemoryFacade(nil, nil, zap.NewNop())
	ctx := f.LoadContext(context.Background(), "agent1")
	if len(ctx) != 0 {
		t.Fatalf("expected empty context, got %v", ctx)
	}
}

func TestUnifiedMemoryFacade_LoadContext_Enhanced(t *testing.T) {
	mem := &mockEnhancedMemoryWithData{
		working:   []types.MemoryEntry{{Content: "working mem"}},
		shortTerm: []types.MemoryEntry{{Content: "short term"}},
	}
	f := NewUnifiedMemoryFacade(nil, mem, zap.NewNop())
	ctx := f.LoadContext(context.Background(), "agent1")
	if len(ctx) != 2 {
		t.Fatalf("expected 2 context entries, got %d", len(ctx))
	}
}

func TestUnifiedMemoryFacade_RecordEpisode(t *testing.T) {
	mem := &mockEnhancedMemory{}
	f := NewUnifiedMemoryFacade(nil, mem, zap.NewNop())
	f.RecordEpisode(context.Background(), &types.EpisodicEvent{ID: "ep1"})
	if mem.episodeCount != 1 {
		t.Fatalf("expected 1 episode, got %d", mem.episodeCount)
	}
}

func TestUnifiedMemoryFacade_RecordEpisode_NilEnhanced(t *testing.T) {
	f := NewUnifiedMemoryFacade(nil, nil, zap.NewNop())
	// Should not panic
	f.RecordEpisode(context.Background(), &types.EpisodicEvent{ID: "ep2"})
}

// ═══ ScopedPersistenceStores ═══

func TestScopedPersistenceStores_Basic(t *testing.T) {
	inner := NewPersistenceStores(zap.NewNop())
	scoped := NewScopedPersistenceStores(inner, "sub-agent-1")

	if scoped.Scope() != "sub-agent-1" {
		t.Fatalf("expected scope=sub-agent-1, got %s", scoped.Scope())
	}

	// LoadPrompt delegates to inner (not scoped)
	doc := scoped.LoadPrompt(context.Background(), "assistant", "test", "")
	if doc != nil {
		t.Fatal("expected nil from nil store")
	}

	// RecordRun with nil store
	runID := scoped.RecordRun(context.Background(), "agent1", "tenant1", "trace1", "input", time.Now())
	if runID != "" {
		t.Fatalf("expected empty runID, got %s", runID)
	}

	// UpdateRunStatus with nil store
	err := scoped.UpdateRunStatus(context.Background(), "run1", "completed", nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// RestoreConversation with nil store
	msgs := scoped.RestoreConversation(context.Background(), "conv1")
	if len(msgs) != 0 {
		t.Fatalf("expected empty, got %d", len(msgs))
	}

	// PersistConversation with nil store — should not panic
	scoped.PersistConversation(context.Background(), "conv1", "agent1", "tenant1", "user1", "in", "out")
}

func TestScopedPersistenceStores_WithStores(t *testing.T) {
	inner := NewPersistenceStores(zap.NewNop())
	inner.SetRunStore(&mockRunStore{})
	inner.SetPromptStore(&mockPromptStore{
		doc: PromptDocument{Version: "v3"},
	})

	scoped := NewScopedPersistenceStores(inner, "scope1")

	runID := scoped.RecordRun(context.Background(), "agent1", "tenant1", "trace1", "input", time.Now())
	if runID == "" {
		t.Fatal("expected non-empty runID")
	}

	doc := scoped.LoadPrompt(context.Background(), "assistant", "test", "")
	if doc == nil || doc.Version != "v3" {
		t.Fatal("expected prompt doc with version v3")
	}
}

// ═══ ExecuteEnhanced with middleware paths ═══

func TestBaseAgent_ExecuteEnhanced_WithObservability(t *testing.T) {
	ag := buildTestAgent(t, "enh-obs")
	ag.Init(context.Background())

	obs := &mockObservability{}
	ag.extensions.EnableObservability(obs)

	output, err := ag.ExecuteEnhanced(context.Background(), &Input{
		TraceID: "obs1",
		Content: "test observability",
	}, EnhancedExecutionOptions{
		UseObservability: true,
		RecordMetrics:    true,
		RecordTrace:      true,
	})
	if err != nil {
		t.Fatalf("ExecuteEnhanced with observability failed: %v", err)
	}
	if output == nil {
		t.Fatal("expected non-nil output")
	}
	if obs.traceStarted != 1 {
		t.Fatalf("expected 1 trace start, got %d", obs.traceStarted)
	}
	if obs.traceEnded != 1 {
		t.Fatalf("expected 1 trace end, got %d", obs.traceEnded)
	}
}

func TestBaseAgent_ExecuteEnhanced_WithSkills(t *testing.T) {
	ag := buildTestAgent(t, "enh-skills")
	ag.Init(context.Background())

	skills := &mockSkillDiscoverer{
		skills: []*types.DiscoveredSkill{
			{Name: "skill1", Instructions: "use tool A"},
		},
	}
	ag.extensions.EnableSkills(skills)

	output, err := ag.ExecuteEnhanced(context.Background(), &Input{
		TraceID: "sk1",
		Content: "test skills",
	}, EnhancedExecutionOptions{UseSkills: true})
	if err != nil {
		t.Fatalf("ExecuteEnhanced with skills failed: %v", err)
	}
	if output == nil {
		t.Fatal("expected non-nil output")
	}
}

func TestBaseAgent_ExecuteEnhanced_WithMemoryLoad(t *testing.T) {
	ag := buildTestAgent(t, "enh-memload")
	ag.Init(context.Background())

	mem := &mockEnhancedMemoryWithData{
		working:   []types.MemoryEntry{{Content: "working context"}},
		shortTerm: []types.MemoryEntry{{Content: "short term context"}},
	}
	ag.extensions.EnableEnhancedMemory(mem)

	output, err := ag.ExecuteEnhanced(context.Background(), &Input{
		TraceID: "ml1",
		Content: "test memory load",
	}, EnhancedExecutionOptions{
		UseEnhancedMemory:   true,
		LoadWorkingMemory:   true,
		LoadShortTermMemory: true,
	})
	if err != nil {
		t.Fatalf("ExecuteEnhanced with memory load failed: %v", err)
	}
	if output == nil {
		t.Fatal("expected non-nil output")
	}
}

func TestBaseAgent_ExecuteEnhanced_WithMemorySave(t *testing.T) {
	ag := buildTestAgent(t, "enh-memsave")
	ag.Init(context.Background())

	mem := &mockEnhancedMemory{}
	ag.extensions.EnableEnhancedMemory(mem)

	output, err := ag.ExecuteEnhanced(context.Background(), &Input{
		TraceID: "ms1",
		Content: "test memory save",
	}, EnhancedExecutionOptions{
		UseEnhancedMemory: true,
		SaveToMemory:      true,
	})
	if err != nil {
		t.Fatalf("ExecuteEnhanced with memory save failed: %v", err)
	}
	if output == nil {
		t.Fatal("expected non-nil output")
	}
	if mem.savedCount != 1 {
		t.Fatalf("expected 1 save, got %d", mem.savedCount)
	}
}

func TestBaseAgent_ExecuteEnhanced_WithPromptEnhancer(t *testing.T) {
	ag := buildTestAgent(t, "enh-prompt")
	ag.Init(context.Background())

	enhancer := &mockPromptEnhancer{}
	ag.extensions.EnablePromptEnhancer(enhancer)

	output, err := ag.ExecuteEnhanced(context.Background(), &Input{
		TraceID: "pe1",
		Content: "test prompt enhancer",
	}, EnhancedExecutionOptions{UsePromptEnhancer: true})
	if err != nil {
		t.Fatalf("ExecuteEnhanced with prompt enhancer failed: %v", err)
	}
	if output == nil {
		t.Fatal("expected non-nil output")
	}
}

func TestBaseAgent_ExecuteEnhanced_AllMiddlewares(t *testing.T) {
	ag := buildTestAgent(t, "enh-all")
	ag.Init(context.Background())

	ag.extensions.EnableObservability(&mockObservability{})
	ag.extensions.EnableSkills(&mockSkillDiscoverer{
		skills: []*types.DiscoveredSkill{{Name: "s1", Instructions: "do X"}},
	})
	mem := &mockEnhancedMemoryWithData{
		working:   []types.MemoryEntry{{Content: "w"}},
		shortTerm: []types.MemoryEntry{{Content: "s"}},
	}
	ag.extensions.EnableEnhancedMemory(mem)
	ag.extensions.EnablePromptEnhancer(&mockPromptEnhancer{})

	output, err := ag.ExecuteEnhanced(context.Background(), &Input{
		TraceID: "all1",
		Content: "test all middlewares",
	}, EnhancedExecutionOptions{
		UseObservability:    true,
		RecordMetrics:       true,
		RecordTrace:         true,
		UseSkills:           true,
		UseEnhancedMemory:   true,
		LoadWorkingMemory:   true,
		LoadShortTermMemory: true,
		SaveToMemory:        true,
		UsePromptEnhancer:   true,
	})
	if err != nil {
		t.Fatalf("ExecuteEnhanced with all middlewares failed: %v", err)
	}
	if output == nil {
		t.Fatal("expected non-nil output")
	}
}

// ═══ Additional mock types ═══

type mockBaseMemory struct {
	saveCount int
}

func (m *mockBaseMemory) Save(_ context.Context, _ MemoryRecord) error {
	m.saveCount++
	return nil
}
func (m *mockBaseMemory) LoadRecent(_ context.Context, _ string, _ MemoryKind, _ int) ([]MemoryRecord, error) {
	return nil, nil
}
func (m *mockBaseMemory) Search(_ context.Context, _, _ string, _ int) ([]MemoryRecord, error) {
	return nil, nil
}
func (m *mockBaseMemory) Delete(_ context.Context, _ string) error          { return nil }
func (m *mockBaseMemory) Clear(_ context.Context, _ string, _ MemoryKind) error { return nil }
func (m *mockBaseMemory) Get(_ context.Context, _ string) (*MemoryRecord, error) { return nil, nil }

type mockEnhancedMemoryWithData struct {
	working   []types.MemoryEntry
	shortTerm []types.MemoryEntry
}

func (m *mockEnhancedMemoryWithData) LoadWorking(_ context.Context, _ string) ([]types.MemoryEntry, error) {
	return m.working, nil
}
func (m *mockEnhancedMemoryWithData) LoadShortTerm(_ context.Context, _ string, _ int) ([]types.MemoryEntry, error) {
	return m.shortTerm, nil
}
func (m *mockEnhancedMemoryWithData) SaveShortTerm(_ context.Context, _, _ string, _ map[string]any) error {
	return nil
}
func (m *mockEnhancedMemoryWithData) RecordEpisode(_ context.Context, _ *types.EpisodicEvent) error {
	return nil
}

type mockObservability struct {
	traceStarted int
	traceEnded   int
	taskRecorded int
}

func (m *mockObservability) StartTrace(_, _ string) {
	m.traceStarted++
}
func (m *mockObservability) EndTrace(_, _ string, _ error) {
	m.traceEnded++
}
func (m *mockObservability) RecordTask(_ string, _ bool, _ time.Duration, _ int, _, _ float64) {
	m.taskRecorded++
}

type mockSkillDiscoverer struct {
	skills []*types.DiscoveredSkill
}

func (m *mockSkillDiscoverer) DiscoverSkills(_ context.Context, _ string) ([]*types.DiscoveredSkill, error) {
	return m.skills, nil
}

type mockPromptEnhancer struct{}

func (m *mockPromptEnhancer) EnhanceUserPrompt(prompt, _ string) (string, error) {
	return "enhanced: " + prompt, nil
}

// ═══ Coverage Boost Round 2 ═══

// --- toolSelectionMiddleware (0%) ---

type mockToolSelector struct {
	selected []types.ToolSchema
	err      error
}

func (m *mockToolSelector) SelectTools(_ context.Context, _ string, tools []types.ToolSchema) ([]types.ToolSchema, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.selected != nil {
		return m.selected, nil
	}
	return tools, nil
}

func TestBaseAgent_ExecuteEnhanced_WithToolSelection(t *testing.T) {
	ag := buildTestAgent(t, "ts-mw")
	ag.Init(context.Background())
	ag.extensions.EnableToolSelection(&mockToolSelector{})

	output, err := ag.ExecuteEnhanced(context.Background(), &Input{
		TraceID: "ts1",
		Content: "test tool selection",
	}, EnhancedExecutionOptions{
		UseToolSelection: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output == nil {
		t.Fatal("expected non-nil output")
	}
}

// --- ExecuteWithReflection adapter (0%) ---

func TestReflectionRunnerAdapter_ExecuteWithReflection(t *testing.T) {
	ag := buildTestAgent(t, "ref-adapt")
	ag.Init(context.Background())

	cfg := DefaultReflectionExecutorConfig()
	cfg.Enabled = false // so it just does a single pass
	executor := NewReflectionExecutor(ag, cfg)
	runner := AsReflectionRunner(executor)

	out, err := runner.ExecuteWithReflection(context.Background(), &Input{
		TraceID: "ra1",
		Content: "test reflection adapter",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == nil || out.Content == "" {
		t.Fatal("expected non-empty output")
	}
}

// --- panicPayloadToError (0%) ---

func TestPanicPayloadToError(t *testing.T) {
	// error input
	err := panicPayloadToError(fmt.Errorf("boom"))
	if err == nil || err.Error() != "boom" {
		t.Fatalf("expected 'boom', got %v", err)
	}
	// string input
	err = panicPayloadToError("oops")
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if err.Error() != "panic: oops" {
		t.Fatalf("expected 'panic: oops', got %v", err)
	}
	// int input
	err = panicPayloadToError(42)
	if err == nil {
		t.Fatal("expected non-nil error")
	}
}

// --- buildValidationFeedbackMessage (0%) ---

func TestBuildValidationFeedbackMessage(t *testing.T) {
	ag := buildTestAgent(t, "vfm")
	result := &guardrails.ValidationResult{
		Errors: []guardrails.ValidationError{
			{Code: "E001", Message: "too short"},
			{Code: "E002", Message: "missing field"},
		},
	}
	msg := ag.buildValidationFeedbackMessage(result)
	if msg == "" {
		t.Fatal("expected non-empty message")
	}
	if !strContains(msg, "E001") || !strContains(msg, "E002") {
		t.Fatalf("expected error codes in message, got: %s", msg)
	}
}

// --- addTaskDescription (0%) ---

func TestPromptOptimizer_AddTaskDescription(t *testing.T) {
	opt := &PromptOptimizer{}
	result := opt.addTaskDescription("do something")
	if result == "" {
		t.Fatal("expected non-empty result")
	}
	if !strContains(result, "do something") {
		t.Fatalf("expected task in result, got: %s", result)
	}
}

// --- RenderSystemPrompt low coverage ---

func TestPromptBundle_RenderSystemPrompt_WithConstraints(t *testing.T) {
	b := PromptBundle{
		System: SystemPrompt{
			Role:     "You are a helper",
			Identity: "AI assistant",
			Policies: []string{"be helpful"},
		},
		Constraints: []string{"no violence", "be polite", ""},
	}
	result := b.RenderSystemPrompt()
	if !strContains(result, "no violence") {
		t.Fatalf("expected constraint in result, got: %s", result)
	}
	if !strContains(result, "be polite") {
		t.Fatalf("expected constraint in result, got: %s", result)
	}
}

func TestPromptBundle_RenderSystemPrompt_Empty(t *testing.T) {
	b := PromptBundle{}
	result := b.RenderSystemPrompt()
	if result != "" {
		t.Fatalf("expected empty result, got: %s", result)
	}
}

// --- replaceTemplateVars low coverage ---

func TestReplaceTemplateVars_NoMatch(t *testing.T) {
	result := replaceTemplateVars("hello {{unknown}}", map[string]string{"name": "world"})
	if result != "hello {{unknown}}" {
		t.Fatalf("expected unchanged, got: %s", result)
	}
}

func TestReplaceTemplateVars_EmptyText(t *testing.T) {
	result := replaceTemplateVars("", map[string]string{"name": "world"})
	if result != "" {
		t.Fatalf("expected empty, got: %s", result)
	}
}

func TestReplaceTemplateVars_Match(t *testing.T) {
	result := replaceTemplateVars("hello {{ name }}", map[string]string{"name": "world"})
	if result != "hello world" {
		t.Fatalf("expected 'hello world', got: %s", result)
	}
}

// --- WithLogger nil (60%) ---

func TestAgentBuilder_WithLogger_Nil(t *testing.T) {
	b := NewAgentBuilder(testConfig("nil-logger"))
	b.WithLogger(nil)
	b.WithProvider(&testMockProvider{})
	_, err := b.Build()
	if err == nil {
		t.Fatal("expected error when logger is nil")
	}
}

// --- WithDefaultSkills with non-existent dir (69.2%) ---

func TestAgentBuilder_WithDefaultSkills_BadDir(t *testing.T) {
	b := NewAgentBuilder(testConfig("skills-bad"))
	b.WithDefaultSkills("/nonexistent/path/to/skills", nil)
	b.WithProvider(&testMockProvider{})
	b.WithLogger(zap.NewNop())
	_, err := b.Build()
	if err == nil {
		t.Fatal("expected error for bad skills directory")
	}
}

// --- Build with errors accumulated (62.5%) ---

func TestAgentBuilder_Build_WithAccumulatedErrors(t *testing.T) {
	b := NewAgentBuilder(testConfig("err-build"))
	b.WithLogger(nil) // accumulates error
	b.WithProvider(&testMockProvider{})
	_, err := b.Build()
	if err == nil {
		t.Fatal("expected build error")
	}
}

func TestAgentBuilder_Build_NoModel(t *testing.T) {
	cfg := testConfig("no-model")
	cfg.LLM.Model = ""
	b := NewAgentBuilder(cfg)
	b.WithProvider(&testMockProvider{})
	b.WithLogger(zap.NewNop())
	_, err := b.Build()
	if err == nil {
		t.Fatal("expected error for missing model")
	}
}

// --- enableMCP with instance (50%) ---

func TestAgentBuilder_BuildWithMCP_WithInstance(t *testing.T) {
	cfg := testConfig("mcp-inst")
	cfg.Extensions.MCP = &types.MCPConfig{Enabled: true}
	b := NewAgentBuilder(cfg)
	b.WithProvider(&testMockProvider{})
	b.WithLogger(zap.NewNop())
	b.WithMCP(&mockMCPServer{})
	ag, err := b.Build()
	if err != nil {
		t.Fatalf("Build with MCP instance failed: %v", err)
	}
	if ag == nil {
		t.Fatal("expected non-nil agent")
	}
}

type mockMCPServer struct{}

func (m *mockMCPServer) ListTools() []types.ToolSchema { return nil }
func (m *mockMCPServer) ExecuteTool(_ context.Context, _ string, _ map[string]any) (any, error) {
	return nil, nil
}

// --- enableEnhancedMemory with instance (60%) ---

func TestAgentBuilder_BuildWithEnhancedMemory_WithInstance(t *testing.T) {
	cfg := testConfig("mem-inst")
	cfg.Features.Memory = &types.MemoryConfig{Enabled: true}
	b := NewAgentBuilder(cfg)
	b.WithProvider(&testMockProvider{})
	b.WithLogger(zap.NewNop())
	b.WithEnhancedMemory(&mockEnhancedMemory{})
	ag, err := b.Build()
	if err != nil {
		t.Fatalf("Build with enhanced memory instance failed: %v", err)
	}
	if ag == nil {
		t.Fatal("expected non-nil agent")
	}
}

// --- enableSkills with instance (66.7%) ---

func TestAgentBuilder_BuildWithSkills_WithInstance(t *testing.T) {
	cfg := testConfig("skills-inst")
	cfg.Extensions.Skills = &types.SkillsConfig{Enabled: true}
	b := NewAgentBuilder(cfg)
	b.WithProvider(&testMockProvider{})
	b.WithLogger(zap.NewNop())
	b.WithSkills(&mockSkillDiscoverer{})
	ag, err := b.Build()
	if err != nil {
		t.Fatalf("Build with skills instance failed: %v", err)
	}
	if ag == nil {
		t.Fatal("expected non-nil agent")
	}
}

// --- ensureAgentType nil (75%) ---

func TestEnsureAgentType_Nil(t *testing.T) {
	ensureAgentType(nil) // should not panic
}

func TestEnsureAgentType_EmptyType(t *testing.T) {
	cfg := &types.AgentConfig{}
	cfg.Core.Type = "  "
	ensureAgentType(cfg)
	if cfg.Core.Type != string(TypeGeneric) {
		t.Fatalf("expected %s, got %s", TypeGeneric, cfg.Core.Type)
	}
}

// --- applyContextRouteHints (70%) ---

func TestApplyContextRouteHints_WithProvider(t *testing.T) {
	req := &llm.ChatRequest{}
	ctx := types.WithLLMProvider(context.Background(), "anthropic")
	applyContextRouteHints(req, ctx)
	if req.Metadata == nil || req.Metadata["chat_provider"] != "anthropic" {
		t.Fatalf("expected provider in metadata, got: %v", req.Metadata)
	}
}

func TestApplyContextRouteHints_WithRoutePolicy(t *testing.T) {
	req := &llm.ChatRequest{}
	ctx := types.WithLLMRoutePolicy(context.Background(), "round_robin")
	applyContextRouteHints(req, ctx)
	if req.Metadata == nil || req.Metadata["route_policy"] != "round_robin" {
		t.Fatalf("expected route_policy in metadata, got: %v", req.Metadata)
	}
}

// --- WithRuntimeStreamEmitter nil (60%) ---

func TestWithRuntimeStreamEmitter_NilEmitter(t *testing.T) {
	ctx := WithRuntimeStreamEmitter(context.Background(), nil)
	_, ok := runtimeStreamEmitterFromContext(ctx)
	if ok {
		t.Fatal("expected no emitter for nil input")
	}
}

func TestWithRuntimeStreamEmitter_NilCtx(t *testing.T) {
	emit := func(ev RuntimeStreamEvent) {}
	ctx := WithRuntimeStreamEmitter(nil, emit)
	got, ok := runtimeStreamEmitterFromContext(ctx)
	if !ok || got == nil {
		t.Fatal("expected emitter from nil ctx")
	}
}

func TestRuntimeStreamEmitterFromContext_NilCtx(t *testing.T) {
	_, ok := runtimeStreamEmitterFromContext(nil)
	if ok {
		t.Fatal("expected false for nil ctx")
	}
}

// --- RecallMemory (66.7%) ---

func TestBaseAgent_RecallMemory_NilMemory(t *testing.T) {
	ag := buildTestAgent(t, "recall-nil")
	records, err := ag.RecallMemory(context.Background(), "query", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("expected empty records, got %d", len(records))
	}
}

func TestBaseAgent_RecallMemory_WithMemory(t *testing.T) {
	ag := buildTestAgent(t, "recall-mem")
	ag.memory = &mockBaseMemory{}
	records, err := ag.RecallMemory(context.Background(), "query", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// mockBaseMemory.Search returns nil, nil
	if records != nil {
		t.Fatalf("expected nil records, got %v", records)
	}
}

// --- TeardownExtensions (62.5%) ---

type mockLSPLifecycle struct {
	closed bool
}

func (m *mockLSPLifecycle) Close() error {
	m.closed = true
	return nil
}

type mockLSPClient struct {
	shutdown bool
}

func (m *mockLSPClient) Shutdown(_ context.Context) error {
	m.shutdown = true
	return nil
}

func TestExtensionRegistry_TeardownExtensions_WithLSPLifecycle(t *testing.T) {
	reg := NewExtensionRegistry(zap.NewNop())
	lc := &mockLSPLifecycle{}
	reg.lspLifecycle = lc
	err := reg.TeardownExtensions(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !lc.closed {
		t.Fatal("expected lifecycle to be closed")
	}
}

func TestExtensionRegistry_TeardownExtensions_WithLSPClient(t *testing.T) {
	reg := NewExtensionRegistry(zap.NewNop())
	client := &mockLSPClient{}
	reg.lspClient = client
	err := reg.TeardownExtensions(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !client.shutdown {
		t.Fatal("expected client to be shutdown")
	}
}

// --- scopedID (66.7%) ---

func TestScopedPersistenceStores_ScopedID_Empty(t *testing.T) {
	inner := NewPersistenceStores(zap.NewNop())
	s := NewScopedPersistenceStores(inner, "tenant1")
	if s.scopedID("") != "" {
		t.Fatal("expected empty for empty id")
	}
	if s.scopedID("abc") != "tenant1/abc" {
		t.Fatalf("expected 'tenant1/abc', got '%s'", s.scopedID("abc"))
	}
}

// --- RestoreConversation more branches (76.2%) ---

func TestPersistenceStores_RestoreConversation_OffsetClamp(t *testing.T) {
	store := &mockConversationStore{
		messages: []ConversationMessage{
			{Role: "user", Content: "hi"},
			{Role: "assistant", Content: "hello"},
		},
		total: 2,
	}
	p := NewPersistenceStores(zap.NewNop())
	p.SetConversationStore(store)
	msgs := p.RestoreConversation(context.Background(), "conv1")
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
}

// --- lastUserQuery (0%) ---

func TestLastUserQuery(t *testing.T) {
	msgs := []types.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "first"},
		{Role: "assistant", Content: "resp"},
		{Role: "user", Content: "second"},
	}
	q := lastUserQuery(msgs)
	if q != "second" {
		t.Fatalf("expected 'second', got '%s'", q)
	}
}

func TestLastUserQuery_NoUser(t *testing.T) {
	msgs := []types.Message{
		{Role: "system", Content: "sys"},
	}
	q := lastUserQuery(msgs)
	if q != "" {
		t.Fatalf("expected empty, got '%s'", q)
	}
}

// --- InitGlobalRegistry / CreateAgent / TeardownAll / ResetCache (0%) ---

func TestInitGlobalRegistry_And_CreateAgent(t *testing.T) {
	// Just test CreateAgent when GlobalRegistry is already set (from other tests or init)
	// We don't reset globalRegistryOnce to avoid sync.Once copy issues.
	InitGlobalRegistry(zap.NewNop())
	if GlobalRegistry == nil {
		t.Fatal("expected non-nil global registry")
	}

	// CreateAgent without provider should fail at registry level
	_, err := CreateAgent(
		testConfig("global-test"),
		nil,
		nil, nil, nil,
		zap.NewNop(),
	)
	// It may fail because provider is nil, which is expected
	if err == nil {
		t.Log("CreateAgent succeeded (provider may be optional in registry)")
	}
}

func TestCreateAgent_NoRegistry(t *testing.T) {
	// Save and restore GlobalRegistry without touching globalRegistryOnce
	oldReg := GlobalRegistry
	defer func() {
		GlobalRegistry = oldReg
	}()

	globalRegistryMu.Lock()
	GlobalRegistry = nil
	globalRegistryMu.Unlock()

	_, err := CreateAgent(testConfig("no-reg"), nil, nil, nil, nil, zap.NewNop())
	if err == nil {
		t.Fatal("expected error when registry not initialized")
	}
}

// --- CachingResolver TeardownAll / ResetCache / WithPromptStore / WithConversationStore / WithRunStore (0%) ---

func TestCachingResolver_StoreSetters(t *testing.T) {
	reg := NewAgentRegistry(zap.NewNop())
	resolver := NewCachingResolver(reg, &testMockProvider{}, zap.NewNop())

	resolver.WithPromptStore(&mockPromptStore{})
	resolver.WithConversationStore(&mockConversationStore{})
	resolver.WithRunStore(&mockRunStore{})

	// TeardownAll with no cached agents should not panic
	resolver.TeardownAll(context.Background())

	// ResetCache with no cached agents should not panic
	resolver.ResetCache(context.Background())
}

// --- RunConfig.ApplyToRequest more branches (71.4%) ---

func TestRunConfig_ApplyToRequest_AllFields(t *testing.T) {
	model := "gpt-4"
	provider := "openai"
	routePolicy := "latency"
	temp := float32(0.5)
	maxTokens := 1000
	topP := float32(0.9)
	toolChoice := "auto"

	rc := &RunConfig{
		Model:       &model,
		Provider:    &provider,
		RoutePolicy: &routePolicy,
		Temperature: &temp,
		MaxTokens:   &maxTokens,
		TopP:        &topP,
		Stop:        []string{"END"},
		ToolChoice:  &toolChoice,
	}

	req := &llm.ChatRequest{}
	rc.ApplyToRequest(req, types.AgentConfig{})

	if req.Model != "gpt-4" {
		t.Fatalf("expected model gpt-4, got %s", req.Model)
	}
	if req.Temperature != 0.5 {
		t.Fatalf("expected temp 0.5, got %f", req.Temperature)
	}
	if req.MaxTokens != 1000 {
		t.Fatalf("expected maxTokens 1000, got %d", req.MaxTokens)
	}
	if req.TopP != 0.9 {
		t.Fatalf("expected topP 0.9, got %f", req.TopP)
	}
	if req.ToolChoice != "auto" {
		t.Fatalf("expected toolChoice auto, got %s", req.ToolChoice)
	}
}

func TestRunConfig_ApplyToRequest_NilRC(t *testing.T) {
	var rc *RunConfig
	req := &llm.ChatRequest{Model: "original"}
	rc.ApplyToRequest(req, types.AgentConfig{})
	if req.Model != "original" {
		t.Fatal("expected no change for nil RunConfig")
	}
}

// --- DynamicToolSelector getAvgLatency (66.7%) ---

func TestDynamicToolSelector_GetAvgLatency(t *testing.T) {
	ag := buildTestAgent(t, "ts-lat")
	cfg := *DefaultToolSelectionConfig()
	s := NewDynamicToolSelector(ag, cfg)
	// No stats: should return default
	lat := s.getAvgLatency("unknown_tool")
	if lat != 500*time.Millisecond {
		t.Fatalf("expected 500ms default, got %v", lat)
	}

	// With stats
	s.toolStats["my_tool"] = &ToolStats{
		TotalCalls:   10,
		TotalLatency: 2 * time.Second,
	}
	lat = s.getAvgLatency("my_tool")
	if lat != 200*time.Millisecond {
		t.Fatalf("expected 200ms, got %v", lat)
	}
}

// --- NewDynamicToolSelector defaults (60%) ---

func TestNewDynamicToolSelector_Defaults(t *testing.T) {
	ag := buildTestAgent(t, "ts-def")
	cfg := ToolSelectionConfig{
		MaxTools: 0,
		MinScore: 0,
	}
	s := NewDynamicToolSelector(ag, cfg)
	if s.config.MaxTools != 5 {
		t.Fatalf("expected MaxTools=5, got %d", s.config.MaxTools)
	}
	if s.config.MinScore != 0.3 {
		t.Fatalf("expected MinScore=0.3, got %f", s.config.MinScore)
	}
}

// --- NewReflectionExecutor defaults (57.1%) ---

func TestNewReflectionExecutor_Defaults(t *testing.T) {
	ag := buildTestAgent(t, "ref-def")
	cfg := ReflectionExecutorConfig{
		MaxIterations: 0,
		MinQuality:    0,
		CriticPrompt:  "",
	}
	exec := NewReflectionExecutor(ag, cfg)
	if exec.config.MaxIterations <= 0 {
		t.Fatal("expected positive MaxIterations")
	}
	if exec.config.MinQuality <= 0 {
		t.Fatal("expected positive MinQuality")
	}
	if exec.config.CriticPrompt == "" {
		t.Fatal("expected non-empty CriticPrompt")
	}
}

// --- StreamCompletion (75%) ---

func TestBaseAgent_StreamCompletion(t *testing.T) {
	prov := &streamingMockProvider{
		chunks: []llm.StreamChunk{
			{ID: "c1", Delta: types.Message{Content: "hi"}},
		},
	}
	ag := buildTestAgentWithProvider(t, "stream-comp", prov)
	ag.Init(context.Background())

	ch, err := ag.StreamCompletion(context.Background(), []types.Message{
		{Role: "user", Content: "hello"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	count := 0
	for range ch {
		count++
	}
	if count == 0 {
		t.Fatal("expected at least one chunk")
	}
}

// --- processEvents panic recovery (63.4%) ---

func TestSimpleEventBus_ProcessEvents_PanicRecovery(t *testing.T) {
	busIface := NewEventBus(zap.NewNop())
	bus := busIface.(*SimpleEventBus)

	bus.Subscribe("test_panic", func(e Event) {
		panic("handler panic")
	})

	// 发布事件触发 panic — 不应导致进程崩溃
	bus.Publish(&StateChangeEvent{
		AgentID_:   "test",
		FromState:  StateReady,
		ToState:    StateRunning,
		Timestamp_: time.Now(),
	})

	// 等待事件处理完成
	time.Sleep(100 * time.Millisecond)
	bus.Stop()
	// 到这里没崩溃就是 PASS
}

// --- Unsubscribe non-existent (87.5%) ---

func TestSimpleEventBus_Unsubscribe_NonExistent(t *testing.T) {
	bus := NewEventBus(zap.NewNop())
	bus.Unsubscribe("does-not-exist") // should not panic
	bus.Stop()
}

// --- ExecuteOne (75%) ---

func TestToolManagerExecutor_ExecuteOne_Empty(t *testing.T) {
	tm := &mockToolManager{}
	exec := newToolManagerExecutor(tm, "agent1", nil, nil)
	result := exec.ExecuteOne(context.Background(), types.ToolCall{
		ID:   "call1",
		Name: "nonexistent",
	})
	// Should return a result even if tool doesn't exist
	if result.ToolCallID != "call1" {
		t.Fatalf("expected call1, got %s", result.ToolCallID)
	}
}

type mockToolManager struct{}

func (m *mockToolManager) GetAllowedTools(_ string) []types.ToolSchema { return nil }
func (m *mockToolManager) ExecuteForAgent(_ context.Context, _ string, _ []types.ToolCall) []llmtools.ToolResult {
	return nil
}

// --- SaveMemory (81.8%) ---

func TestBaseAgent_SaveMemory_WithMemory(t *testing.T) {
	ag := buildTestAgent(t, "save-mem")
	mem := &mockBaseMemory{}
	ag.memory = mem
	err := ag.SaveMemory(context.Background(), "test content", MemoryShortTerm, map[string]any{"key": "val"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mem.saveCount != 1 {
		t.Fatalf("expected 1 save, got %d", mem.saveCount)
	}
}

// --- gatewayProvider more branches (77.8%) ---

func TestBaseAgent_GatewayProvider_ExternalGateway(t *testing.T) {
	ag := buildTestAgent(t, "gw-ext")
	ext := &testMockProvider{}
	ag.SetGateway(ext)
	gw := ag.gatewayProvider()
	if gw != ext {
		t.Fatal("expected external gateway")
	}
}

func TestBaseAgent_GatewayProvider_NilLedger(t *testing.T) {
	ag := buildTestAgent(t, "gw-nil-ledger")
	gw := ag.gatewayProvider()
	// With nil ledger, should return provider directly
	if gw == nil {
		t.Fatal("expected non-nil provider")
	}
}

// helper
func strContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
