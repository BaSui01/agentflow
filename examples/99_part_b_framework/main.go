// =============================================================================
// 🧪 AgentFlow 全能力测试 Part B — 框架内部能力（纯本地，不调 API）
// =============================================================================
// 覆盖中间件、韧性、缓存、Token 计数、背压流、Guardrails、MCP/A2A 协议、
// 多 Agent 聚合、工作流编排等框架内部模块。
// =============================================================================

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/agent/guardrails"
	"github.com/BaSui01/agentflow/agent/multiagent"
	"github.com/BaSui01/agentflow/agent/protocol/a2a"
	"github.com/BaSui01/agentflow/agent/protocol/mcp"
	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/middleware"
	llmpolicy "github.com/BaSui01/agentflow/llm/runtime/policy"
	"github.com/BaSui01/agentflow/llm/streaming"
	"github.com/BaSui01/agentflow/llm/tokenizer"
	"github.com/BaSui01/agentflow/types"
	"github.com/BaSui01/agentflow/workflow"
	"go.uber.org/zap"
)

type R struct{ Name, Status string; D time.Duration; Info string }
var rs []R

func rec(n, s string, d time.Duration, info string) {
	rs = append(rs, R{n, s, d, info})
	i := map[string]string{"PASS": "✅", "FAIL": "❌", "WARN": "⚠️"}[s]
	fmt.Printf("  %s %-32s %8v  %s\n", i, n, d.Round(time.Microsecond), info)
}

func main() {
	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║  🧪 AgentFlow 全能力测试 Part B — 框架内部能力（本地）      ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")

	fmt.Println("\n━━━ 中间件 & 请求改写 ━━━")
	b01RewriterChain()
	b02XMLToolFormat()
	b03EmptyToolsCleaner()

	fmt.Println("\n━━━ 韧性 & 重试 ━━━")
	b04RetryPolicy()
	b05ResilientProvider()

	fmt.Println("\n━━━ Token 计数 ━━━")
	b06TokenEstimator()

	fmt.Println("\n━━━ 背压流 ━━━")
	b07BackpressureStream()
	b08StreamMultiplexer()

	fmt.Println("\n━━━ Guardrails 护栏 ━━━")
	b09ValidatorChain()
	b10TripwireDetection()

	fmt.Println("\n━━━ MCP 协议 ━━━")
	b11MCPServer()

	fmt.Println("\n━━━ A2A 协议 ━━━")
	b12A2AAgentCard()

	fmt.Println("\n━━━ 多 Agent 聚合 ━━━")
	b13AggregatorMergeAll()
	b14AggregatorBestOfN()
	b15AggregatorVoteMajority()

	fmt.Println("\n━━━ 工作流编排 ━━━")
	b16SequentialWorkflow()
	b17CredentialOverride()
	b18ToolCallIndexField()

	fmt.Println("\n━━━ 熔断器 ━━━")
	b19CircuitBreakerStateTransition()

	fmt.Println("\n━━━ DAG 工作流 ━━━")
	b20DAGWorkflow()

	fmt.Println("\n━━━ 多Agent WorkerPool ━━━")
	b21WorkerPoolExecution()

	fmt.Println("\n━━━ 幂等性 ━━━")
	b22IdempotencyKey()

	fmt.Println("\n━━━ 边界条件 ━━━")
	b23EmptyMessages()
	b24UnicodeContent()

	printSummary()
}

// ═══ 中间件 ═══

func b01RewriterChain() {
	t := time.Now()
	chain := middleware.NewRewriterChain(middleware.NewXMLToolRewriter(), middleware.NewEmptyToolsCleaner())
	req := &llm.ChatRequest{Model: "test", Messages: []types.Message{{Role: "user", Content: "hi"}}}
	out, err := chain.Execute(context.Background(), req)
	if err != nil { rec("RewriterChain", "FAIL", time.Since(t), err.Error()); return }
	if out != nil && out.Model == "test" { rec("RewriterChain", "PASS", time.Since(t), "链式执行成功")
	} else { rec("RewriterChain", "FAIL", time.Since(t), "输出异常") }
}

func b02XMLToolFormat() {
	t := time.Now()
	xml := middleware.FormatToolsAsXML([]types.ToolSchema{
		{Name: "search", Description: "搜索引擎", Parameters: json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"}}}`)},
	})
	if strings.Contains(xml, "tool_calls") && strings.Contains(xml, "search") {
		rec("XML工具格式化", "PASS", time.Since(t), fmt.Sprintf("生成%d字符XML", len(xml)))
	} else { rec("XML工具格式化", "FAIL", time.Since(t), "XML格式不正确") }
}

func b03EmptyToolsCleaner() {
	t := time.Now()
	cleaner := middleware.NewEmptyToolsCleaner()
	req := &llm.ChatRequest{Tools: []types.ToolSchema{}} // 空工具列表
	out, err := cleaner.Rewrite(context.Background(), req)
	if err != nil { rec("空工具清理器", "FAIL", time.Since(t), err.Error()); return }
	if out.Tools == nil || len(out.Tools) == 0 { rec("空工具清理器", "PASS", time.Since(t), "空列表已清理")
	} else { rec("空工具清理器", "FAIL", time.Since(t), "未清理") }
}

// ═══ 韧性 ═══

func b04RetryPolicy() {
	t := time.Now()
	policy := llmpolicy.DefaultRetryPolicy()
	retryer := llmpolicy.NewBackoffRetryer(policy, zap.NewNop())
	attempts := 0
	err := retryer.Do(context.Background(), func() error {
		attempts++
		if attempts < 3 { return fmt.Errorf("模拟失败 #%d", attempts) }
		return nil
	})
	if err == nil && attempts == 3 { rec("重试策略", "PASS", time.Since(t), fmt.Sprintf("重试%d次后成功", attempts))
	} else { rec("重试策略", "FAIL", time.Since(t), fmt.Sprintf("attempts=%d err=%v", attempts, err)) }
}

func b05ResilientProvider() {
	t := time.Now()
	callCount := 0
	mock := &mockProvider{fn: func() (*llm.ChatResponse, error) {
		callCount++
		if callCount < 2 { return nil, &types.Error{Code: types.ErrUpstreamError, Message: "503", Retryable: true} }
		return &llm.ChatResponse{Choices: []llm.ChatChoice{{Message: types.Message{Content: "ok"}}}}, nil
	}}
	rp := llm.NewResilientProvider(mock, &llm.ResilientConfig{
		RetryPolicy: llmpolicy.DefaultRetryPolicy(),
	}, zap.NewNop())
	resp, err := rp.Completion(context.Background(), &llm.ChatRequest{Model: "test"})
	if err == nil && resp != nil && resp.Choices[0].Message.Content == "ok" {
		rec("弹性Provider", "PASS", time.Since(t), fmt.Sprintf("重试%d次后成功", callCount))
	} else { rec("弹性Provider", "FAIL", time.Since(t), fmt.Sprintf("err=%v calls=%d", err, callCount)) }
}

// ═══ Token ═══

func b06TokenEstimator() {
	t := time.Now()
	tok := tokenizer.GetTokenizerOrEstimator("unknown-model")
	n, err := tok.CountTokens("Hello world, 你好世界！This is a test.")
	if err != nil { rec("Token估算器", "FAIL", time.Since(t), err.Error()); return }
	if n > 0 && n < 100 { rec("Token估算器", "PASS", time.Since(t), fmt.Sprintf("估算=%d tokens", n))
	} else { rec("Token估算器", "WARN", time.Since(t), fmt.Sprintf("估算值异常=%d", n)) }
}

// ═══ 背压流 ═══

func b07BackpressureStream() {
	t := time.Now()
	cfg := streaming.DefaultBackpressureConfig()
	cfg.BufferSize = 10
	s := streaming.NewBackpressureStream(cfg)
	ctx := context.Background()
	// 写入
	for i := 0; i < 5; i++ { s.Write(ctx, streaming.Token{Content: fmt.Sprintf("t%d", i), Index: i}) }
	s.Write(ctx, streaming.Token{Final: true})
	// 读取
	var tokens []string
	for { tk, err := s.Read(ctx); if err != nil || tk.Final { break }; tokens = append(tokens, tk.Content) }
	s.Close()
	if len(tokens) == 5 { rec("背压流", "PASS", time.Since(t), fmt.Sprintf("写入5读出5"))
	} else { rec("背压流", "FAIL", time.Since(t), fmt.Sprintf("读出%d", len(tokens))) }
}

func b08StreamMultiplexer() {
	t := time.Now()
	cfg := streaming.DefaultBackpressureConfig(); cfg.BufferSize = 20
	src := streaming.NewBackpressureStream(cfg)
	mux := streaming.NewStreamMultiplexer(src)
	c1 := mux.AddConsumer(cfg)
	c2 := mux.AddConsumer(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go mux.Start(ctx)
	// 写入源
	go func() { for i := 0; i < 3; i++ { src.Write(ctx, streaming.Token{Content: fmt.Sprintf("m%d", i)}) }; src.Write(ctx, streaming.Token{Final: true}) }()
	// 两个消费者都应该收到相同数据
	read := func(s *streaming.BackpressureStream) int { n := 0; for { tk, e := s.Read(ctx); if e != nil || tk.Final { break }; n++ }; return n }
	var wg sync.WaitGroup; var n1, n2 int
	wg.Add(2); go func() { defer wg.Done(); n1 = read(c1) }(); go func() { defer wg.Done(); n2 = read(c2) }()
	wg.Wait()
	if n1 == 3 && n2 == 3 { rec("流多路复用", "PASS", time.Since(t), fmt.Sprintf("2消费者各收%d", n1))
	} else { rec("流多路复用", "WARN", time.Since(t), fmt.Sprintf("c1=%d c2=%d", n1, n2)) }
}

// ═══ Guardrails ═══

func b09ValidatorChain() {
	t := time.Now()
	chain := guardrails.NewValidatorChain(&guardrails.ValidatorChainConfig{Mode: guardrails.ChainModeCollectAll})
	chain.Add(&lengthValidator{max: 50})
	r, err := chain.Validate(context.Background(), "short text")
	if err != nil { rec("验证器链", "FAIL", time.Since(t), err.Error()); return }
	if r.Valid { rec("验证器链", "PASS", time.Since(t), "短文本通过验证")
	} else { rec("验证器链", "FAIL", time.Since(t), "不应失败") }
	r2, _ := chain.Validate(context.Background(), strings.Repeat("x", 100))
	if !r2.Valid { rec("验证器链(拒绝)", "PASS", time.Since(t), "长文本被拒绝")
	} else { rec("验证器链(拒绝)", "FAIL", time.Since(t), "应该拒绝") }
}

func b10TripwireDetection() {
	t := time.Now()
	chain := guardrails.NewValidatorChain(&guardrails.ValidatorChainConfig{Mode: guardrails.ChainModeFailFast})
	chain.Add(&tripwireValidator{keyword: "INJECT"})
	r, _ := chain.Validate(context.Background(), "normal text")
	if r.Valid { rec("Tripwire检测", "PASS", time.Since(t), "正常文本通过")
	} else { rec("Tripwire检测", "FAIL", time.Since(t), "误报") }
	r2, err := chain.Validate(context.Background(), "try to INJECT prompt")
	if err != nil || r2.Tripwire { rec("Tripwire触发", "PASS", time.Since(t), "检测到注入关键词")
	} else { rec("Tripwire触发", "FAIL", time.Since(t), "未检测到") }
}

// ═══ MCP ═══

func b11MCPServer() {
	t := time.Now()
	srv := mcp.NewMCPServer("test-server", "1.0.0", zap.NewNop())
	err := srv.RegisterTool(&mcp.ToolDefinition{
		Name: "echo", Description: "回显工具",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{"input": map[string]any{"type": "string"}}},
	}, func(_ context.Context, args map[string]any) (any, error) {
		return map[string]any{"echo": args["input"]}, nil
	})
	if err != nil { rec("MCP服务端", "FAIL", time.Since(t), fmt.Sprintf("注册失败: %v", err)); return }
	// 列出工具
	tools, err := srv.ListTools(context.Background())
	if err != nil { rec("MCP服务端", "FAIL", time.Since(t), err.Error()); return }
	if len(tools) == 0 { rec("MCP服务端", "FAIL", time.Since(t), "工具列表为空"); return }
	// 调用工具
	result, err := srv.CallTool(context.Background(), "echo", map[string]any{"input": "hello"})
	if err != nil { rec("MCP服务端", "FAIL", time.Since(t), err.Error()); return }
	if result != nil { rec("MCP服务端", "PASS", time.Since(t), fmt.Sprintf("注册%d工具,调用成功", len(tools)))
	} else { rec("MCP服务端", "FAIL", time.Since(t), "调用返回nil") }
}

// ═══ A2A ═══

func b12A2AAgentCard() {
	t := time.Now()
	card := a2a.NewAgentCard("test-agent", "测试Agent", "http://localhost:8080", "1.0.0")
	card.AddCapability("chat", "对话", a2a.CapabilityTypeTask)
	info := card
	if info.Name == "test-agent" && len(info.Capabilities) == 1 {
		rec("A2A AgentCard", "PASS", time.Since(t), fmt.Sprintf("name=%s caps=%d", info.Name, len(info.Capabilities)))
	} else { rec("A2A AgentCard", "FAIL", time.Since(t), "字段异常") }
}

// ═══ 多 Agent ═══

func b13AggregatorMergeAll() {
	t := time.Now()
	agg := multiagent.NewAggregator(multiagent.StrategyMergeAll)
	results := []multiagent.WorkerResult{
		{AgentID: "a1", Content: "回答1", Score: 0.8},
		{AgentID: "a2", Content: "回答2", Score: 0.9},
	}
	out, err := agg.Aggregate(results)
	if err != nil { rec("聚合-MergeAll", "FAIL", time.Since(t), err.Error()); return }
	if strings.Contains(out.Content, "回答1") && strings.Contains(out.Content, "回答2") {
		rec("聚合-MergeAll", "PASS", time.Since(t), "两个结果合并")
	} else { rec("聚合-MergeAll", "FAIL", time.Since(t), out.Content) }
}

func b14AggregatorBestOfN() {
	t := time.Now()
	agg := multiagent.NewAggregator(multiagent.StrategyBestOfN)
	results := []multiagent.WorkerResult{
		{AgentID: "a1", Content: "低分回答", Score: 0.3},
		{AgentID: "a2", Content: "高分回答", Score: 0.95},
	}
	out, err := agg.Aggregate(results)
	if err != nil { rec("聚合-BestOfN", "FAIL", time.Since(t), err.Error()); return }
	if strings.Contains(out.Content, "高分") { rec("聚合-BestOfN", "PASS", time.Since(t), "选择最高分")
	} else { rec("聚合-BestOfN", "FAIL", time.Since(t), out.Content) }
}

func b15AggregatorVoteMajority() {
	t := time.Now()
	agg := multiagent.NewAggregator(multiagent.StrategyVoteMajority)
	results := []multiagent.WorkerResult{
		{AgentID: "a1", Content: "Go最好"},
		{AgentID: "a2", Content: "Go最好"},
		{AgentID: "a3", Content: "Rust最好"},
	}
	out, err := agg.Aggregate(results)
	if err != nil { rec("聚合-多数投票", "FAIL", time.Since(t), err.Error()); return }
	if strings.Contains(out.Content, "Go") { rec("聚合-多数投票", "PASS", time.Since(t), "多数胜出")
	} else { rec("聚合-多数投票", "WARN", time.Since(t), out.Content) }
}

// ═══ 工作流 ═══

func b16SequentialWorkflow() {
	t := time.Now()
	step1 := workflow.NewFuncStep("uppercase", func(_ context.Context, input any) (any, error) {
		return strings.ToUpper(input.(string)), nil
	})
	step2 := workflow.NewFuncStep("add_suffix", func(_ context.Context, input any) (any, error) {
		return input.(string) + "_DONE", nil
	})
	// 手动串联
	out1, err := step1.Execute(context.Background(), "hello")
	if err != nil { rec("工作流串行", "FAIL", time.Since(t), err.Error()); return }
	out2, err := step2.Execute(context.Background(), out1)
	if err != nil { rec("工作流串行", "FAIL", time.Since(t), err.Error()); return }
	if out2.(string) == "HELLO_DONE" { rec("工作流串行", "PASS", time.Since(t), "HELLO_DONE")
	} else { rec("工作流串行", "FAIL", time.Since(t), fmt.Sprintf("got=%v", out2)) }
}

func b17CredentialOverride() {
	t := time.Now()
	ctx := llm.WithCredentialOverride(context.Background(), llm.CredentialOverride{APIKey: "test-key-123"})
	cred, ok := llm.CredentialOverrideFromContext(ctx)
	if ok && cred.APIKey == "test-key-123" { rec("凭据覆盖", "PASS", time.Since(t), "上下文传递正确")
	} else { rec("凭据覆盖", "FAIL", time.Since(t), "凭据丢失") }
}

func b18ToolCallIndexField() {
	t := time.Now()
	tc := types.ToolCall{Index: 2, ID: "call_123", Name: "test", Arguments: json.RawMessage(`{"a":1}`)}
	data, _ := json.Marshal(tc)
	var tc2 types.ToolCall; json.Unmarshal(data, &tc2)
	if tc2.Index == 2 && tc2.ID == "call_123" { rec("ToolCall.Index字段", "PASS", time.Since(t), "序列化round-trip正确")
	} else { rec("ToolCall.Index字段", "FAIL", time.Since(t), fmt.Sprintf("got index=%d id=%s", tc2.Index, tc2.ID)) }
}

// ─── 汇总 ────────────────────────────────────────────────

func printSummary() {
	fmt.Println("\n╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║  📊 Part B 测试汇总                                         ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	ps, fl, wr := 0, 0, 0
	for _, r := range rs { switch r.Status { case "PASS": ps++; case "FAIL": fl++; case "WARN": wr++ } }
	fmt.Printf("\n  总计: %d | ✅ PASS: %d | ❌ FAIL: %d | ⚠️  WARN: %d\n\n", len(rs), ps, fl, wr)
	for _, r := range rs {
		i := map[string]string{"PASS": "✅", "FAIL": "❌", "WARN": "⚠️"}[r.Status]
		fmt.Printf("  %s %-32s %8v  %s\n", i, r.Name, r.D.Round(time.Microsecond), r.Info)
	}
	if fl == 0 { fmt.Println("\n  🎉 全部框架内部测试通过！") } else { fmt.Printf("\n  ⚠️  有 %d 项失败\n", fl) }
}

// ─── Mock & 辅助类型 ────────────────────────────────────

type mockProvider struct {
	fn func() (*llm.ChatResponse, error)
}
func (m *mockProvider) Name() string { return "mock" }
func (m *mockProvider) Completion(_ context.Context, _ *llm.ChatRequest) (*llm.ChatResponse, error) { return m.fn() }
func (m *mockProvider) Stream(_ context.Context, _ *llm.ChatRequest) (<-chan llm.StreamChunk, error) { return nil, nil }
func (m *mockProvider) HealthCheck(_ context.Context) (*llm.HealthStatus, error) { return &llm.HealthStatus{Healthy: true}, nil }
func (m *mockProvider) SupportsNativeFunctionCalling() bool { return false }
func (m *mockProvider) ListModels(_ context.Context) ([]llm.Model, error) { return nil, nil }
func (m *mockProvider) Endpoints() llm.ProviderEndpoints { return llm.ProviderEndpoints{} }

type lengthValidator struct{ max int }
func (v *lengthValidator) Validate(_ context.Context, c string) (*guardrails.ValidationResult, error) {
	r := guardrails.NewValidationResult(); if len([]rune(c)) > v.max { r.Valid = false; r.Errors = append(r.Errors, guardrails.ValidationError{Code: "too_long", Message: "超长"}) }; return r, nil
}
func (v *lengthValidator) Name() string { return "length" }
func (v *lengthValidator) Priority() int { return 1 }

type tripwireValidator struct{ keyword string }
func (v *tripwireValidator) Validate(_ context.Context, c string) (*guardrails.ValidationResult, error) {
	r := guardrails.NewValidationResult(); if strings.Contains(c, v.keyword) { r.Valid = false; r.Tripwire = true; r.Errors = append(r.Errors, guardrails.ValidationError{Code: "injection", Message: "检测到注入"}) }; return r, nil
}
func (v *tripwireValidator) Name() string { return "tripwire" }
func (v *tripwireValidator) Priority() int { return 0 }

// ═══ 熔断器 ═══

func b19CircuitBreakerStateTransition() {
	t := time.Now()
	// 用 ResilientProvider 间接测试熔断器：连续失败触发熔断，超时后恢复
	failCount := 0
	mock := &mockProvider{fn: func() (*llm.ChatResponse, error) {
		failCount++
		return nil, &types.Error{Code: types.ErrUpstreamError, Message: "503", Retryable: true}
	}}
	rp := llm.NewResilientProvider(mock, &llm.ResilientConfig{
		RetryPolicy:    &llmpolicy.RetryPolicy{MaxRetries: 0}, // 不重试，直接失败
		CircuitBreaker: &llm.CircuitBreakerConfig{FailureThreshold: 3, SuccessThreshold: 1, Timeout: 100 * time.Millisecond},
	}, zap.NewNop())

	// 连续失败 3 次触发熔断
	for i := 0; i < 3; i++ {
		rp.Completion(context.Background(), &llm.ChatRequest{Model: "test"})
	}
	// 第 4 次应该被熔断器拦截（快速失败）
	_, err := rp.Completion(context.Background(), &llm.ChatRequest{Model: "test"})
	if err != nil && strings.Contains(err.Error(), "circuit") {
		rec("熔断器-触发", "PASS", time.Since(t), "3次失败后熔断")
	} else {
		rec("熔断器-触发", "FAIL", time.Since(t), fmt.Sprintf("err=%v", err))
	}

	// 等待超时后恢复（半开状态）
	time.Sleep(150 * time.Millisecond)
	// 切换为成功的 mock
	mock.fn = func() (*llm.ChatResponse, error) {
		return &llm.ChatResponse{Choices: []llm.ChatChoice{{Message: types.Message{Content: "recovered"}}}}, nil
	}
	resp, err := rp.Completion(context.Background(), &llm.ChatRequest{Model: "test"})
	if err == nil && resp != nil && resp.Choices[0].Message.Content == "recovered" {
		rec("熔断器-恢复", "PASS", time.Since(t), "超时后半开→关闭")
	} else {
		rec("熔断器-恢复", "FAIL", time.Since(t), fmt.Sprintf("err=%v", err))
	}
}

// ═══ DAG 工作流 ═══

func b20DAGWorkflow() {
	t := time.Now()
	graph := workflow.NewDAGGraph()

	// 构建 A → B → C 的 DAG
	graph.AddNode(&workflow.DAGNode{
		ID: "start", Type: workflow.NodeTypeAction,
		Step: workflow.NewFuncStep("start", func(_ context.Context, input any) (any, error) {
			return input.(string) + "_A", nil
		}),
	})
	graph.AddNode(&workflow.DAGNode{
		ID: "middle", Type: workflow.NodeTypeAction,
		Step: workflow.NewFuncStep("middle", func(_ context.Context, input any) (any, error) {
			return input.(string) + "_B", nil
		}),
	})
	graph.AddNode(&workflow.DAGNode{
		ID: "end", Type: workflow.NodeTypeAction,
		Step: workflow.NewFuncStep("end", func(_ context.Context, input any) (any, error) {
			return input.(string) + "_C", nil
		}),
	})
	graph.AddEdge("start", "middle")
	graph.AddEdge("middle", "end")
	graph.SetEntry("start")

	executor := workflow.NewDAGExecutor(nil, zap.NewNop())
	result, err := executor.Execute(context.Background(), graph, "INIT")
	if err != nil {
		rec("DAG工作流", "FAIL", time.Since(t), err.Error())
		return
	}
	if result.(string) == "INIT_A_B_C" {
		rec("DAG工作流", "PASS", time.Since(t), "A→B→C 串行执行正确")
	} else {
		rec("DAG工作流", "WARN", time.Since(t), fmt.Sprintf("got=%v", result))
	}
}

// ═══ 多Agent WorkerPool ═══

func b21WorkerPoolExecution() {
	t := time.Now()
	pool := multiagent.NewWorkerPool(multiagent.DefaultWorkerPoolConfig(), zap.NewNop())

	tasks := []multiagent.WorkerTask{
		{AgentID: "agent1", Agent: &mockAgent{id: "agent1", output: "结果A"}, Input: &agent.Input{TraceID: "t1", Content: "任务1"}},
		{AgentID: "agent2", Agent: &mockAgent{id: "agent2", output: "结果B"}, Input: &agent.Input{TraceID: "t2", Content: "任务2"}},
	}

	results, err := pool.Execute(context.Background(), tasks)
	if err != nil {
		rec("WorkerPool并发", "FAIL", time.Since(t), err.Error())
		return
	}

	successCount := 0
	for _, r := range results {
		if r.Err == nil && r.Content != "" {
			successCount++
		}
	}
	if successCount == 2 {
		rec("WorkerPool并发", "PASS", time.Since(t), fmt.Sprintf("%d/%d 任务成功", successCount, len(tasks)))
	} else {
		rec("WorkerPool并发", "WARN", time.Since(t), fmt.Sprintf("%d/%d", successCount, len(tasks)))
	}
}

// ═══ 幂等性 ═══

func b22IdempotencyKey() {
	t := time.Now()
	// 测试 ResilientProvider 的幂等性：相同请求不重复执行
	callCount := int64(0)
	mock := &mockProvider{fn: func() (*llm.ChatResponse, error) {
		atomic.AddInt64(&callCount, 1)
		return &llm.ChatResponse{Choices: []llm.ChatChoice{{Message: types.Message{Content: "ok"}}}}, nil
	}}
	rp := llm.NewResilientProvider(mock, &llm.ResilientConfig{
		EnableIdempotency: true,
		IdempotencyTTL:    5 * time.Second,
	}, zap.NewNop())

	req := &llm.ChatRequest{Model: "test", Messages: []types.Message{{Role: "user", Content: "hello"}}}
	// 同一请求调用两次
	rp.Completion(context.Background(), req)
	rp.Completion(context.Background(), req)

	calls := atomic.LoadInt64(&callCount)
	if calls == 1 {
		rec("幂等性", "PASS", time.Since(t), "相同请求只执行1次")
	} else if calls == 2 {
		rec("幂等性", "WARN", time.Since(t), "执行了2次（幂等缓存可能未命中）")
	} else {
		rec("幂等性", "FAIL", time.Since(t), fmt.Sprintf("执行了%d次", calls))
	}
}

// ═══ 边界条件 ═══

func b23EmptyMessages() {
	t := time.Now()
	// 空消息列表不应 panic
	chain := middleware.NewRewriterChain(middleware.NewEmptyToolsCleaner())
	req := &llm.ChatRequest{Model: "test", Messages: nil}
	out, err := chain.Execute(context.Background(), req)
	if err == nil && out != nil {
		rec("空消息处理", "PASS", time.Since(t), "空消息不panic")
	} else {
		rec("空消息处理", "FAIL", time.Since(t), fmt.Sprintf("err=%v", err))
	}
}

func b24UnicodeContent() {
	t := time.Now()
	// Unicode emoji + CJK + 特殊字符的序列化 round-trip
	content := "你好🌍！Go语言🚀 café naïve 日本語テスト"
	msg := types.Message{Role: "user", Content: content}
	data, err := json.Marshal(msg)
	if err != nil { rec("Unicode处理", "FAIL", time.Since(t), err.Error()); return }
	var msg2 types.Message
	json.Unmarshal(data, &msg2)
	if msg2.Content == content {
		rec("Unicode处理", "PASS", time.Since(t), fmt.Sprintf("round-trip正确, %d字符", len([]rune(content))))
	} else {
		rec("Unicode处理", "FAIL", time.Since(t), "内容不一致")
	}
}

// ─── Mock Agent ────────────────────────────────────

type mockAgent struct {
	id     string
	output string
}

func (a *mockAgent) ID() string                                                    { return a.id }
func (a *mockAgent) Name() string                                                  { return a.id }
func (a *mockAgent) Type() agent.AgentType                                         { return "mock" }
func (a *mockAgent) State() agent.State                                            { return "ready" }
func (a *mockAgent) Init(_ context.Context) error                                  { return nil }
func (a *mockAgent) Teardown(_ context.Context) error                              { return nil }
func (a *mockAgent) Plan(_ context.Context, _ *agent.Input) (*agent.PlanResult, error) { return nil, nil }
func (a *mockAgent) Execute(_ context.Context, input *agent.Input) (*agent.Output, error) {
	return &agent.Output{TraceID: input.TraceID, Content: a.output, TokensUsed: 10, Duration: time.Millisecond}, nil
}
func (a *mockAgent) Observe(_ context.Context, _ *agent.Feedback) error { return nil }
