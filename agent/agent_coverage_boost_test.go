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
