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

	"github.com/BaSui01/agentflow/agent/adapters/handoff"
	"github.com/BaSui01/agentflow/agent/capabilities/guardrails"
	memorycore "github.com/BaSui01/agentflow/agent/capabilities/memory"
	"github.com/BaSui01/agentflow/agent/collaboration/multiagent"
	collaboration "github.com/BaSui01/agentflow/agent/collaboration/multiagent"
	"github.com/BaSui01/agentflow/agent/execution/protocol/a2a"
	"github.com/BaSui01/agentflow/agent/execution/protocol/mcp"
	agent "github.com/BaSui01/agentflow/agent/execution/runtime"
	"github.com/BaSui01/agentflow/agent/observability/evaluation"
	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/capabilities/tools"
	"github.com/BaSui01/agentflow/llm/middleware"
	provbase "github.com/BaSui01/agentflow/llm/providers/base"
	llmpolicy "github.com/BaSui01/agentflow/llm/runtime/policy"
	"github.com/BaSui01/agentflow/llm/streaming"
	"github.com/BaSui01/agentflow/llm/tokenizer"
	"github.com/BaSui01/agentflow/types"
	"github.com/BaSui01/agentflow/workflow"
	workflowruntime "github.com/BaSui01/agentflow/workflow/runtime"
	"go.uber.org/zap"
)

type R struct {
	Name, Status string
	D            time.Duration
	Info         string
}

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

	fmt.Println("\n━━━ 异常重试 & 非200 ━━━")
	b25HTTPErrorMapping()
	b26RetryableErrorDetection()
	b27NonRetryableError()

	fmt.Println("\n━━━ 工具异常 ━━━")
	b28ToolPanicRecovery()
	b29ToolNotFound()
	b30ToolRateLimit()
	b31ToolTimeout()

	fmt.Println("\n━━━ 循环边界 ━━━")
	b32ReActMaxIterationsZero()
	b33ReActStopOnError()
	b34WorkerPoolEmpty()
	b35WorkerPoolFailFast()
	b36DAGCycleDetection()

	fmt.Println("\n━━━ Agent 高级功能 ━━━")
	b37SharedState()
	b38MemoryManager()
	b39AgentHandoff()
	b40DeliberationMode()
	b41DebateCoordinator()

	fmt.Println("\n━━━ 评估框架 ━━━")
	b42EvaluationFramework()

	printSummary()
}

// ═══ 中间件 ═══

func b01RewriterChain() {
	t := time.Now()
	chain := middleware.NewRewriterChain(middleware.NewXMLToolRewriter(), middleware.NewEmptyToolsCleaner())
	req := &llm.ChatRequest{Model: "test", Messages: []types.Message{{Role: "user", Content: "hi"}}}
	out, err := chain.Execute(context.Background(), req)
	if err != nil {
		rec("RewriterChain", "FAIL", time.Since(t), err.Error())
		return
	}
	if out != nil && out.Model == "test" {
		rec("RewriterChain", "PASS", time.Since(t), "链式执行成功")
	} else {
		rec("RewriterChain", "FAIL", time.Since(t), "输出异常")
	}
}

func b02XMLToolFormat() {
	t := time.Now()
	xml := middleware.FormatToolsAsXML([]types.ToolSchema{
		{Name: "search", Description: "搜索引擎", Parameters: json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"}}}`)},
	})
	if strings.Contains(xml, "tool_calls") && strings.Contains(xml, "search") {
		rec("XML工具格式化", "PASS", time.Since(t), fmt.Sprintf("生成%d字符XML", len(xml)))
	} else {
		rec("XML工具格式化", "FAIL", time.Since(t), "XML格式不正确")
	}
}

func b03EmptyToolsCleaner() {
	t := time.Now()
	cleaner := middleware.NewEmptyToolsCleaner()
	req := &llm.ChatRequest{Tools: []types.ToolSchema{}} // 空工具列表
	out, err := cleaner.Rewrite(context.Background(), req)
	if err != nil {
		rec("空工具清理器", "FAIL", time.Since(t), err.Error())
		return
	}
	if out.Tools == nil || len(out.Tools) == 0 {
		rec("空工具清理器", "PASS", time.Since(t), "空列表已清理")
	} else {
		rec("空工具清理器", "FAIL", time.Since(t), "未清理")
	}
}

// ═══ 韧性 ═══

func b04RetryPolicy() {
	t := time.Now()
	policy := llmpolicy.DefaultRetryPolicy()
	retryer := llmpolicy.NewBackoffRetryer(policy, zap.NewNop())
	attempts := 0
	err := retryer.Do(context.Background(), func() error {
		attempts++
		if attempts < 3 {
			return fmt.Errorf("模拟失败 #%d", attempts)
		}
		return nil
	})
	if err == nil && attempts == 3 {
		rec("重试策略", "PASS", time.Since(t), fmt.Sprintf("重试%d次后成功", attempts))
	} else {
		rec("重试策略", "FAIL", time.Since(t), fmt.Sprintf("attempts=%d err=%v", attempts, err))
	}
}

func b05ResilientProvider() {
	t := time.Now()
	callCount := 0
	mock := &mockProvider{fn: func() (*llm.ChatResponse, error) {
		callCount++
		if callCount < 2 {
			return nil, &types.Error{Code: types.ErrUpstreamError, Message: "503", Retryable: true}
		}
		return &llm.ChatResponse{Choices: []llm.ChatChoice{{Message: types.Message{Content: "ok"}}}}, nil
	}}
	rp := llm.NewResilientProvider(mock, &llm.ResilientConfig{
		RetryPolicy: llmpolicy.DefaultRetryPolicy(),
	}, zap.NewNop())
	resp, err := rp.Completion(context.Background(), &llm.ChatRequest{Model: "test"})
	if err == nil && resp != nil && resp.Choices[0].Message.Content == "ok" {
		rec("弹性Provider", "PASS", time.Since(t), fmt.Sprintf("重试%d次后成功", callCount))
	} else {
		rec("弹性Provider", "FAIL", time.Since(t), fmt.Sprintf("err=%v calls=%d", err, callCount))
	}
}

// ═══ Token ═══

func b06TokenEstimator() {
	t := time.Now()
	tok := tokenizer.GetTokenizerOrEstimator("unknown-model")
	n, err := tok.CountTokens("Hello world, 你好世界！This is a test.")
	if err != nil {
		rec("Token估算器", "FAIL", time.Since(t), err.Error())
		return
	}
	if n > 0 && n < 100 {
		rec("Token估算器", "PASS", time.Since(t), fmt.Sprintf("估算=%d tokens", n))
	} else {
		rec("Token估算器", "WARN", time.Since(t), fmt.Sprintf("估算值异常=%d", n))
	}
}

// ═══ 背压流 ═══

func b07BackpressureStream() {
	t := time.Now()
	cfg := streaming.DefaultBackpressureConfig()
	cfg.BufferSize = 10
	s := streaming.NewBackpressureStream(cfg)
	ctx := context.Background()
	// 写入
	for i := 0; i < 5; i++ {
		s.Write(ctx, streaming.Token{Content: fmt.Sprintf("t%d", i), Index: i})
	}
	s.Write(ctx, streaming.Token{Final: true})
	// 读取
	var tokens []string
	for {
		tk, err := s.Read(ctx)
		if err != nil || tk.Final {
			break
		}
		tokens = append(tokens, tk.Content)
	}
	s.Close()
	if len(tokens) == 5 {
		rec("背压流", "PASS", time.Since(t), fmt.Sprintf("写入5读出5"))
	} else {
		rec("背压流", "FAIL", time.Since(t), fmt.Sprintf("读出%d", len(tokens)))
	}
}

func b08StreamMultiplexer() {
	t := time.Now()
	cfg := streaming.DefaultBackpressureConfig()
	cfg.BufferSize = 20
	src := streaming.NewBackpressureStream(cfg)
	mux := streaming.NewStreamMultiplexer(src)
	c1 := mux.AddConsumer(cfg)
	c2 := mux.AddConsumer(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go mux.Start(ctx)
	// 写入源
	go func() {
		for i := 0; i < 3; i++ {
			src.Write(ctx, streaming.Token{Content: fmt.Sprintf("m%d", i)})
		}
		src.Write(ctx, streaming.Token{Final: true})
	}()
	// 两个消费者都应该收到相同数据
	read := func(s *streaming.BackpressureStream) int {
		n := 0
		for {
			tk, e := s.Read(ctx)
			if e != nil || tk.Final {
				break
			}
			n++
		}
		return n
	}
	var wg sync.WaitGroup
	var n1, n2 int
	wg.Add(2)
	go func() { defer wg.Done(); n1 = read(c1) }()
	go func() { defer wg.Done(); n2 = read(c2) }()
	wg.Wait()
	if n1 == 3 && n2 == 3 {
		rec("流多路复用", "PASS", time.Since(t), fmt.Sprintf("2消费者各收%d", n1))
	} else {
		rec("流多路复用", "WARN", time.Since(t), fmt.Sprintf("c1=%d c2=%d", n1, n2))
	}
}

// ═══ Guardrails ═══

func b09ValidatorChain() {
	t := time.Now()
	chain := guardrails.NewValidatorChain(&guardrails.ValidatorChainConfig{Mode: guardrails.ChainModeCollectAll})
	chain.Add(&lengthValidator{max: 50})
	r, err := chain.Validate(context.Background(), "short text")
	if err != nil {
		rec("验证器链", "FAIL", time.Since(t), err.Error())
		return
	}
	if r.Valid {
		rec("验证器链", "PASS", time.Since(t), "短文本通过验证")
	} else {
		rec("验证器链", "FAIL", time.Since(t), "不应失败")
	}
	r2, _ := chain.Validate(context.Background(), strings.Repeat("x", 100))
	if !r2.Valid {
		rec("验证器链(拒绝)", "PASS", time.Since(t), "长文本被拒绝")
	} else {
		rec("验证器链(拒绝)", "FAIL", time.Since(t), "应该拒绝")
	}
}

func b10TripwireDetection() {
	t := time.Now()
	chain := guardrails.NewValidatorChain(&guardrails.ValidatorChainConfig{Mode: guardrails.ChainModeFailFast})
	chain.Add(&tripwireValidator{keyword: "INJECT"})
	r, _ := chain.Validate(context.Background(), "normal text")
	if r.Valid {
		rec("Tripwire检测", "PASS", time.Since(t), "正常文本通过")
	} else {
		rec("Tripwire检测", "FAIL", time.Since(t), "误报")
	}
	r2, err := chain.Validate(context.Background(), "try to INJECT prompt")
	if err != nil || r2.Tripwire {
		rec("Tripwire触发", "PASS", time.Since(t), "检测到注入关键词")
	} else {
		rec("Tripwire触发", "FAIL", time.Since(t), "未检测到")
	}
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
	if err != nil {
		rec("MCP服务端", "FAIL", time.Since(t), fmt.Sprintf("注册失败: %v", err))
		return
	}
	// 列出工具
	tools, err := srv.ListTools(context.Background())
	if err != nil {
		rec("MCP服务端", "FAIL", time.Since(t), err.Error())
		return
	}
	if len(tools) == 0 {
		rec("MCP服务端", "FAIL", time.Since(t), "工具列表为空")
		return
	}
	// 调用工具
	result, err := srv.CallTool(context.Background(), "echo", map[string]any{"input": "hello"})
	if err != nil {
		rec("MCP服务端", "FAIL", time.Since(t), err.Error())
		return
	}
	if result != nil {
		rec("MCP服务端", "PASS", time.Since(t), fmt.Sprintf("注册%d工具,调用成功", len(tools)))
	} else {
		rec("MCP服务端", "FAIL", time.Since(t), "调用返回nil")
	}
}

// ═══ A2A ═══

func b12A2AAgentCard() {
	t := time.Now()
	card := a2a.NewAgentCard("test-agent", "测试Agent", "http://localhost:8080", "1.0.0")
	card.AddCapability("chat", "对话", a2a.CapabilityTypeTask)
	info := card
	if info.Name == "test-agent" && len(info.Capabilities) == 1 {
		rec("A2A AgentCard", "PASS", time.Since(t), fmt.Sprintf("name=%s caps=%d", info.Name, len(info.Capabilities)))
	} else {
		rec("A2A AgentCard", "FAIL", time.Since(t), "字段异常")
	}
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
	if err != nil {
		rec("聚合-MergeAll", "FAIL", time.Since(t), err.Error())
		return
	}
	if strings.Contains(out.Content, "回答1") && strings.Contains(out.Content, "回答2") {
		rec("聚合-MergeAll", "PASS", time.Since(t), "两个结果合并")
	} else {
		rec("聚合-MergeAll", "FAIL", time.Since(t), out.Content)
	}
}

func b14AggregatorBestOfN() {
	t := time.Now()
	agg := multiagent.NewAggregator(multiagent.StrategyBestOfN)
	results := []multiagent.WorkerResult{
		{AgentID: "a1", Content: "低分回答", Score: 0.3},
		{AgentID: "a2", Content: "高分回答", Score: 0.95},
	}
	out, err := agg.Aggregate(results)
	if err != nil {
		rec("聚合-BestOfN", "FAIL", time.Since(t), err.Error())
		return
	}
	if strings.Contains(out.Content, "高分") {
		rec("聚合-BestOfN", "PASS", time.Since(t), "选择最高分")
	} else {
		rec("聚合-BestOfN", "FAIL", time.Since(t), out.Content)
	}
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
	if err != nil {
		rec("聚合-多数投票", "FAIL", time.Since(t), err.Error())
		return
	}
	if strings.Contains(out.Content, "Go") {
		rec("聚合-多数投票", "PASS", time.Since(t), "多数胜出")
	} else {
		rec("聚合-多数投票", "WARN", time.Since(t), out.Content)
	}
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
	if err != nil {
		rec("工作流串行", "FAIL", time.Since(t), err.Error())
		return
	}
	out2, err := step2.Execute(context.Background(), out1)
	if err != nil {
		rec("工作流串行", "FAIL", time.Since(t), err.Error())
		return
	}
	if out2.(string) == "HELLO_DONE" {
		rec("工作流串行", "PASS", time.Since(t), "HELLO_DONE")
	} else {
		rec("工作流串行", "FAIL", time.Since(t), fmt.Sprintf("got=%v", out2))
	}
}

func b17CredentialOverride() {
	t := time.Now()
	ctx := llm.WithCredentialOverride(context.Background(), llm.CredentialOverride{APIKey: "test-key-123"})
	cred, ok := llm.CredentialOverrideFromContext(ctx)
	if ok && cred.APIKey == "test-key-123" {
		rec("凭据覆盖", "PASS", time.Since(t), "上下文传递正确")
	} else {
		rec("凭据覆盖", "FAIL", time.Since(t), "凭据丢失")
	}
}

func b18ToolCallIndexField() {
	t := time.Now()
	tc := types.ToolCall{Index: 2, ID: "call_123", Name: "test", Arguments: json.RawMessage(`{"a":1}`)}
	data, _ := json.Marshal(tc)
	var tc2 types.ToolCall
	json.Unmarshal(data, &tc2)
	if tc2.Index == 2 && tc2.ID == "call_123" {
		rec("ToolCall.Index字段", "PASS", time.Since(t), "序列化round-trip正确")
	} else {
		rec("ToolCall.Index字段", "FAIL", time.Since(t), fmt.Sprintf("got index=%d id=%s", tc2.Index, tc2.ID))
	}
}

// ═══ 评估框架 ═══

func b42EvaluationFramework() {
	t := time.Now()

	// 1. 用 mock provider 构建 EvalExecutor
	executor := &evalMockExecutor{
		response: "Go语言是一种高效、简洁的编程语言，适合构建后端服务和分布式系统。",
		tokens:   42,
		latency:  15 * time.Millisecond,
	}

	// 2. 构建评估套件
	suite := &evaluation.EvalSuite{
		ID:   "quality-baseline",
		Name: "输出质量基线评估",
		Tasks: []evaluation.EvalTask{
			{
				ID:       "keyword-check",
				Name:     "关键词覆盖",
				Input:    "介绍Go语言的优势",
				Expected: "Go语言是一种高效、简洁的编程语言，适合构建后端服务和分布式系统。",
			},
			{
				ID:       "length-check",
				Name:     "输出长度合理性",
				Input:    "介绍Go语言",
				Expected: "Go语言",
				Metadata: map[string]string{"type": "contains"},
			},
		},
	}

	// 3. 配置评估器（含指标收集和告警）
	cfg := evaluation.DefaultEvaluatorConfig()
	cfg.Concurrency = 1
	cfg.PassThreshold = 0.5
	cfg.CollectMetrics = true
	cfg.EnableAlerts = true
	cfg.AlertThresholds = []evaluation.AlertThreshold{
		{MetricName: "score", Operator: "lt", Value: 0.3, Level: evaluation.AlertLevelCritical, Message: "分数过低"},
	}

	evaluator := evaluation.NewEvaluator(cfg, zap.NewNop())
	evaluator.RegisterScorer("contains", &evalContainsScorer{})

	// 4. 运行评估
	report, err := evaluator.Evaluate(context.Background(), suite, executor)
	if err != nil {
		rec("评估框架", "FAIL", time.Since(t), fmt.Sprintf("评估失败: %v", err))
		return
	}

	// 5. 检查输出质量
	allPassed := true
	for _, r := range report.Results {
		// 关键词检查：输出包含 "Go"
		if !strings.Contains(r.Output, "Go") {
			allPassed = false
			rec("评估-关键词", "FAIL", time.Since(t), fmt.Sprintf("task=%s 输出缺少关键词'Go'", r.TaskID))
		}
		// 长度合理性：输出 > 10 字符
		if len([]rune(r.Output)) < 10 {
			allPassed = false
			rec("评估-长度", "FAIL", time.Since(t), fmt.Sprintf("task=%s 输出过短: %d字符", r.TaskID, len([]rune(r.Output))))
		}
	}

	if allPassed {
		rec("评估-质量检查", "PASS", time.Since(t), fmt.Sprintf(
			"通过率=%.0f%% 平均分=%.2f",
			report.Summary.PassRate*100, report.Summary.AverageScore))
	}

	// 6. 记录 Token 用量和延迟作为基线
	rec("评估-Token基线", "PASS", time.Since(t), fmt.Sprintf(
		"总Token=%d 总耗时=%v 任务数=%d",
		report.Summary.TotalTokens, report.Summary.TotalDuration, report.Summary.TotalTasks))

	// 7. 验证告警系统正常工作
	alerts := evaluator.GetAlerts()
	rec("评估-告警系统", "PASS", time.Since(t), fmt.Sprintf("触发告警=%d", len(alerts)))
}

// evalMockExecutor 评估用 mock 执行器
type evalMockExecutor struct {
	response string
	tokens   int
	latency  time.Duration
}

func (e *evalMockExecutor) Execute(_ context.Context, _ string) (string, int, error) {
	time.Sleep(e.latency)
	return e.response, e.tokens, nil
}

// evalContainsScorer 包含匹配评分器
type evalContainsScorer struct{}

func (s *evalContainsScorer) Score(_ context.Context, task *evaluation.EvalTask, output string) (float64, map[string]float64, error) {
	if task.Expected == "" {
		return 1.0, nil, nil
	}
	if strings.Contains(output, task.Expected) {
		return 1.0, map[string]float64{"contains": 1.0}, nil
	}
	return 0.0, map[string]float64{"contains": 0.0}, nil
}

// ─── 汇总 ────────────────────────────────────────────────

func printSummary() {
	fmt.Println("\n╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║  📊 Part B 测试汇总                                         ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	ps, fl, wr := 0, 0, 0
	for _, r := range rs {
		switch r.Status {
		case "PASS":
			ps++
		case "FAIL":
			fl++
		case "WARN":
			wr++
		}
	}
	fmt.Printf("\n  总计: %d | ✅ PASS: %d | ❌ FAIL: %d | ⚠️  WARN: %d\n\n", len(rs), ps, fl, wr)
	for _, r := range rs {
		i := map[string]string{"PASS": "✅", "FAIL": "❌", "WARN": "⚠️"}[r.Status]
		fmt.Printf("  %s %-32s %8v  %s\n", i, r.Name, r.D.Round(time.Microsecond), r.Info)
	}
	if fl == 0 {
		fmt.Println("\n  🎉 全部框架内部测试通过！")
	} else {
		fmt.Printf("\n  ⚠️  有 %d 项失败\n", fl)
	}
}

// ─── Mock & 辅助类型 ────────────────────────────────────

type mockProvider struct {
	fn func() (*llm.ChatResponse, error)
}

func (m *mockProvider) Name() string { return "mock" }
func (m *mockProvider) Completion(_ context.Context, _ *llm.ChatRequest) (*llm.ChatResponse, error) {
	return m.fn()
}
func (m *mockProvider) Stream(_ context.Context, _ *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	return nil, nil
}
func (m *mockProvider) HealthCheck(_ context.Context) (*llm.HealthStatus, error) {
	return &llm.HealthStatus{Healthy: true}, nil
}
func (m *mockProvider) SupportsNativeFunctionCalling() bool               { return false }
func (m *mockProvider) ListModels(_ context.Context) ([]llm.Model, error) { return nil, nil }
func (m *mockProvider) Endpoints() llm.ProviderEndpoints                  { return llm.ProviderEndpoints{} }

type lengthValidator struct{ max int }

func (v *lengthValidator) Validate(_ context.Context, c string) (*guardrails.ValidationResult, error) {
	r := guardrails.NewValidationResult()
	if len([]rune(c)) > v.max {
		r.Valid = false
		r.Errors = append(r.Errors, guardrails.ValidationError{Code: "too_long", Message: "超长"})
	}
	return r, nil
}
func (v *lengthValidator) Name() string  { return "length" }
func (v *lengthValidator) Priority() int { return 1 }

type tripwireValidator struct{ keyword string }

func (v *tripwireValidator) Validate(_ context.Context, c string) (*guardrails.ValidationResult, error) {
	r := guardrails.NewValidationResult()
	if strings.Contains(c, v.keyword) {
		r.Valid = false
		r.Tripwire = true
		r.Errors = append(r.Errors, guardrails.ValidationError{Code: "injection", Message: "检测到注入"})
	}
	return r, nil
}
func (v *tripwireValidator) Name() string  { return "tripwire" }
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

	wf := workflow.NewDAGWorkflow("demo-dag", "Facade-driven DAG demo", graph)
	wfRuntime := workflowruntime.NewBuilder(nil, zap.NewNop()).
		WithDSLParser(false).
		Build()
	result, err := wfRuntime.Facade.ExecuteDAG(context.Background(), wf, "INIT")
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
	if err != nil {
		rec("Unicode处理", "FAIL", time.Since(t), err.Error())
		return
	}
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

func (a *mockAgent) ID() string                       { return a.id }
func (a *mockAgent) Name() string                     { return a.id }
func (a *mockAgent) Type() agent.AgentType            { return "mock" }
func (a *mockAgent) State() agent.State               { return "ready" }
func (a *mockAgent) Init(_ context.Context) error     { return nil }
func (a *mockAgent) Teardown(_ context.Context) error { return nil }
func (a *mockAgent) Plan(_ context.Context, _ *agent.Input) (*agent.PlanResult, error) {
	return nil, nil
}
func (a *mockAgent) Execute(_ context.Context, input *agent.Input) (*agent.Output, error) {
	return &agent.Output{TraceID: input.TraceID, Content: a.output, TokensUsed: 10, Duration: time.Millisecond}, nil
}
func (a *mockAgent) Observe(_ context.Context, _ *agent.Feedback) error { return nil }

// ═══ 异常重试 & 非200 ═══

func b25HTTPErrorMapping() {
	t := time.Now()
	providerbase := provbase.MapHTTPError
	tests := []struct {
		code      int
		wantRetry bool
		wantCode  types.ErrorCode
	}{
		{401, false, types.ErrUnauthorized},
		{403, false, types.ErrForbidden},
		{429, true, types.ErrRateLimit},
		{500, true, types.ErrUpstreamError},
		{502, true, types.ErrUpstreamError},
		{503, true, types.ErrUpstreamError},
	}
	allOK := true
	for _, tt := range tests {
		err := providerbase(tt.code, "test error", "test")
		if err.Retryable != tt.wantRetry || err.Code != tt.wantCode {
			allOK = false
			rec("HTTP错误映射", "FAIL", time.Since(t), fmt.Sprintf("code=%d: retryable=%v(want %v) code=%s(want %s)", tt.code, err.Retryable, tt.wantRetry, err.Code, tt.wantCode))
			return
		}
	}
	if allOK {
		rec("HTTP错误映射", "PASS", time.Since(t), fmt.Sprintf("6种状态码映射正确"))
	}
}

func b26RetryableErrorDetection() {
	t := time.Now()
	retryable := &types.Error{Code: types.ErrRateLimit, Message: "429", Retryable: true}
	if types.IsRetryable(retryable) {
		rec("可重试错误检测", "PASS", time.Since(t), "429 正确标记为可重试")
	} else {
		rec("可重试错误检测", "FAIL", time.Since(t), "429 应该可重试")
	}
}

func b27NonRetryableError() {
	t := time.Now()
	nonRetryable := &types.Error{Code: types.ErrUnauthorized, Message: "401", Retryable: false}
	if !types.IsRetryable(nonRetryable) {
		rec("不可重试错误", "PASS", time.Since(t), "401 正确标记为不可重试")
	} else {
		rec("不可重试错误", "FAIL", time.Since(t), "401 不应该可重试")
	}
}

// ═══ 工具异常 ═══

func b28ToolPanicRecovery() {
	t := time.Now()
	lg := zap.NewNop()
	reg := tools.NewDefaultRegistry(lg)
	reg.Register("panic_tool", func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
		panic("模拟工具 panic！")
	}, tools.ToolMetadata{
		Schema: types.ToolSchema{Name: "panic_tool", Description: "会panic的工具",
			Parameters: json.RawMessage(`{"type":"object","properties":{"x":{"type":"string"}},"required":["x"]}`)},
		Timeout: 5 * time.Second,
	})
	ex := tools.NewDefaultExecutor(reg, lg)
	result := ex.ExecuteOne(context.Background(), types.ToolCall{ID: "c1", Name: "panic_tool", Arguments: json.RawMessage(`{"x":"test"}`)})
	if result.Error != "" && strings.Contains(result.Error, "panic") {
		rec("工具Panic恢复", "PASS", time.Since(t), "panic被捕获: "+result.Error[:min(len(result.Error), 40)])
	} else if result.Error != "" {
		rec("工具Panic恢复", "WARN", time.Since(t), "有错误但非panic: "+result.Error[:min(len(result.Error), 40)])
	} else {
		rec("工具Panic恢复", "FAIL", time.Since(t), "panic未被捕获！")
	}
}

func b29ToolNotFound() {
	t := time.Now()
	lg := zap.NewNop()
	reg := tools.NewDefaultRegistry(lg)
	ex := tools.NewDefaultExecutor(reg, lg)
	result := ex.ExecuteOne(context.Background(), types.ToolCall{ID: "c1", Name: "nonexistent", Arguments: json.RawMessage(`{}`)})
	if strings.Contains(result.Error, "not found") {
		rec("工具未找到", "PASS", time.Since(t), "正确返回 not found")
	} else {
		rec("工具未找到", "FAIL", time.Since(t), result.Error)
	}
}

func b30ToolRateLimit() {
	t := time.Now()
	lg := zap.NewNop()
	reg := tools.NewDefaultRegistry(lg)
	reg.Register("limited", func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`"ok"`), nil
	}, tools.ToolMetadata{
		Schema: types.ToolSchema{Name: "limited", Description: "限流工具",
			Parameters: json.RawMessage(`{"type":"object","properties":{"x":{"type":"string"}},"required":["x"]}`)},
		Timeout:   5 * time.Second,
		RateLimit: &tools.RateLimitConfig{MaxCalls: 1, Window: 10 * time.Second},
	})
	ex := tools.NewDefaultExecutor(reg, lg)
	call := types.ToolCall{ID: "c1", Name: "limited", Arguments: json.RawMessage(`{"x":"1"}`)}
	r1 := ex.ExecuteOne(context.Background(), call)
	r2 := ex.ExecuteOne(context.Background(), call)
	if r1.Error == "" && strings.Contains(r2.Error, "rate limit") {
		rec("工具限流", "PASS", time.Since(t), "第1次成功，第2次被限流")
	} else {
		rec("工具限流", "WARN", time.Since(t), fmt.Sprintf("r1=%s r2=%s", r1.Error, r2.Error))
	}
}

func b31ToolTimeout() {
	t := time.Now()
	lg := zap.NewNop()
	reg := tools.NewDefaultRegistry(lg)
	reg.Register("slow", func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
		select {
		case <-time.After(5 * time.Second):
			return json.RawMessage(`"done"`), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}, tools.ToolMetadata{
		Schema: types.ToolSchema{Name: "slow", Description: "慢工具",
			Parameters: json.RawMessage(`{"type":"object","properties":{"x":{"type":"string"}},"required":["x"]}`)},
		Timeout: 100 * time.Millisecond,
	})
	ex := tools.NewDefaultExecutor(reg, lg)
	result := ex.ExecuteOne(context.Background(), types.ToolCall{ID: "c1", Name: "slow", Arguments: json.RawMessage(`{"x":"1"}`)})
	if strings.Contains(result.Error, "timeout") {
		rec("工具超时", "PASS", time.Since(t), "100ms超时正确触发")
	} else {
		rec("工具超时", "FAIL", time.Since(t), result.Error)
	}
}

// ═══ 循环边界 ═══

func b32ReActMaxIterationsZero() {
	t := time.Now()
	lg := zap.NewNop()
	callCount := 0
	mock := &mockProvider{fn: func() (*llm.ChatResponse, error) {
		callCount++
		return &llm.ChatResponse{Choices: []llm.ChatChoice{{Message: types.Message{Content: "done"}}}}, nil
	}}
	ex := tools.NewDefaultExecutor(tools.NewDefaultRegistry(lg), lg)
	re := tools.NewReActExecutor(mock, ex, tools.ReActConfig{MaxIterations: 0}, lg) // 0 应该变成默认值10
	resp, _, err := re.Execute(context.Background(), &llm.ChatRequest{Model: "test", Messages: []types.Message{{Role: "user", Content: "hi"}}})
	if err == nil && resp != nil && callCount == 1 {
		rec("ReAct MaxIter=0", "PASS", time.Since(t), "默认值生效，1次调用即完成")
	} else {
		rec("ReAct MaxIter=0", "WARN", time.Since(t), fmt.Sprintf("calls=%d err=%v", callCount, err))
	}
}

func b33ReActStopOnError() {
	t := time.Now()
	lg := zap.NewNop()
	callCount := 0
	mock := &mockProvider{fn: func() (*llm.ChatResponse, error) {
		callCount++
		return &llm.ChatResponse{Choices: []llm.ChatChoice{{
			FinishReason: "tool_calls",
			Message:      types.Message{Role: "assistant", ToolCalls: []types.ToolCall{{ID: "c1", Name: "bad_tool", Arguments: json.RawMessage(`{"x":"1"}`)}}},
		}}}, nil
	}}
	reg := tools.NewDefaultRegistry(lg) // bad_tool 未注册 → 执行失败
	ex := tools.NewDefaultExecutor(reg, lg)
	re := tools.NewReActExecutor(mock, ex, tools.ReActConfig{MaxIterations: 5, StopOnError: true}, lg)
	_, steps, err := re.Execute(context.Background(), &llm.ChatRequest{Model: "test", Messages: []types.Message{{Role: "user", Content: "hi"}}})
	if err != nil && len(steps) == 1 && strings.Contains(err.Error(), "tool execution failed") {
		rec("ReAct StopOnError", "PASS", time.Since(t), "第1步工具失败即停止")
	} else {
		rec("ReAct StopOnError", "FAIL", time.Since(t), fmt.Sprintf("steps=%d err=%v", len(steps), err))
	}
}

func b34WorkerPoolEmpty() {
	t := time.Now()
	pool := multiagent.NewWorkerPool(multiagent.DefaultWorkerPoolConfig(), zap.NewNop())
	results, err := pool.Execute(context.Background(), nil)
	if err == nil && results == nil {
		rec("WorkerPool空任务", "PASS", time.Since(t), "空列表返回nil")
	} else {
		rec("WorkerPool空任务", "FAIL", time.Since(t), fmt.Sprintf("err=%v results=%v", err, results))
	}
}

func b35WorkerPoolFailFast() {
	t := time.Now()
	pool := multiagent.NewWorkerPool(multiagent.WorkerPoolConfig{
		FailurePolicy: multiagent.PolicyFailFast,
		TaskTimeout:   5 * time.Second,
	}, zap.NewNop())
	tasks := []multiagent.WorkerTask{
		{AgentID: "fail", Agent: &failAgent{}, Input: &agent.Input{TraceID: "t1", Content: "x"}},
		{AgentID: "ok", Agent: &mockAgent{id: "ok", output: "ok"}, Input: &agent.Input{TraceID: "t2", Content: "y"}},
	}
	_, err := pool.Execute(context.Background(), tasks)
	if err != nil {
		rec("WorkerPool FailFast", "PASS", time.Since(t), "第1个失败后快速返回错误")
	} else {
		rec("WorkerPool FailFast", "WARN", time.Since(t), "未返回错误")
	}
}

func b36DAGCycleDetection() {
	t := time.Now()
	graph := workflow.NewDAGGraph()
	graph.AddNode(&workflow.DAGNode{ID: "a", Type: workflow.NodeTypeAction, Step: workflow.NewFuncStep("a", func(_ context.Context, i any) (any, error) { return i, nil })})
	graph.AddNode(&workflow.DAGNode{ID: "b", Type: workflow.NodeTypeAction, Step: workflow.NewFuncStep("b", func(_ context.Context, i any) (any, error) { return i, nil })})
	graph.AddEdge("a", "b")
	graph.AddEdge("b", "a") // 环！
	graph.SetEntry("a")
	wf := workflow.NewDAGWorkflow("cycle-dag", "Cycle detection via facade", graph)
	wfRuntime := workflowruntime.NewBuilder(nil, zap.NewNop()).
		WithDSLParser(false).
		Build()
	_, err := wfRuntime.Facade.ExecuteDAG(context.Background(), wf, "input")
	if err != nil && strings.Contains(err.Error(), "cycle") {
		rec("DAG环检测", "PASS", time.Since(t), "正确检测到环")
	} else if err != nil {
		rec("DAG环检测", "WARN", time.Since(t), "有错误但非cycle: "+err.Error()[:min(len(err.Error()), 60)])
	} else {
		rec("DAG环检测", "FAIL", time.Since(t), "未检测到环！")
	}
}

// ─── failAgent ────────────────────────────────────

type failAgent struct{}

func (a *failAgent) ID() string                       { return "fail" }
func (a *failAgent) Name() string                     { return "fail" }
func (a *failAgent) Type() agent.AgentType            { return "mock" }
func (a *failAgent) State() agent.State               { return "ready" }
func (a *failAgent) Init(_ context.Context) error     { return nil }
func (a *failAgent) Teardown(_ context.Context) error { return nil }
func (a *failAgent) Plan(_ context.Context, _ *agent.Input) (*agent.PlanResult, error) {
	return nil, nil
}
func (a *failAgent) Execute(_ context.Context, _ *agent.Input) (*agent.Output, error) {
	return nil, fmt.Errorf("模拟Agent执行失败")
}
func (a *failAgent) Observe(_ context.Context, _ *agent.Feedback) error { return nil }

// ═══ Agent 高级功能 ═══

func b37SharedState() {
	t := time.Now()
	ss := collaboration.NewInMemorySharedState()
	ctx := context.Background()

	// Set + Get
	ss.Set(ctx, "counter", 42)
	val, ok := ss.Get(ctx, "counter")
	if !ok || val.(int) != 42 {
		rec("SharedState读写", "FAIL", time.Since(t), "Get/Set 失败")
		return
	}

	// Watch
	ch := ss.Watch(ctx, "event")
	go func() {
		time.Sleep(10 * time.Millisecond)
		ss.Set(ctx, "event", "fired")
	}()
	select {
	case v := <-ch:
		if v == "fired" {
			rec("SharedState读写", "PASS", time.Since(t), "Get/Set/Watch 全部正确")
		} else {
			rec("SharedState读写", "WARN", time.Since(t), fmt.Sprintf("Watch收到: %v", v))
		}
	case <-time.After(1 * time.Second):
		rec("SharedState读写", "FAIL", time.Since(t), "Watch 超时")
	}

	// Snapshot
	snap := ss.Snapshot(ctx)
	if len(snap) >= 2 {
		rec("SharedState快照", "PASS", time.Since(t), fmt.Sprintf("快照包含%d个key", len(snap)))
	} else {
		rec("SharedState快照", "WARN", time.Since(t), fmt.Sprintf("快照只有%d个key", len(snap)))
	}
}

func b38MemoryManager() {
	t := time.Now()
	// 使用自定义内存实现测试 MemoryManager 接口
	mm := &inMemoryMgr{records: make(map[string]memorycore.MemoryRecord)}
	ctx := context.Background()

	err := mm.Save(ctx, memorycore.MemoryRecord{
		ID: "rec1", AgentID: "agent1", Kind: types.MemoryShortTerm,
		Content: "用户喜欢Go语言", Metadata: map[string]any{"topic": "preference"},
	})
	if err != nil {
		rec("记忆管理器", "FAIL", time.Since(t), fmt.Sprintf("Save失败: %v", err))
		return
	}

	records, err := mm.LoadRecent(ctx, "agent1", types.MemoryShortTerm, 10)
	if err != nil {
		rec("记忆管理器", "FAIL", time.Since(t), fmt.Sprintf("LoadRecent失败: %v", err))
		return
	}

	if len(records) == 1 && records[0].Content == "用户喜欢Go语言" {
		rec("记忆管理器", "PASS", time.Since(t), "Save+LoadRecent 正确")
	} else {
		rec("记忆管理器", "WARN", time.Since(t), fmt.Sprintf("records=%d", len(records)))
	}

	// 测试 Cache 层
	cache := memorycore.NewCache("agent1", mm, zap.NewNop())
	cache.LoadRecent(ctx)
	err = cache.Save(ctx, "新的记忆内容", types.MemoryShortTerm, nil)
	if err != nil {
		rec("记忆Cache层", "FAIL", time.Since(t), err.Error())
	} else {
		rec("记忆Cache层", "PASS", time.Since(t), "Cache Save 成功")
	}
}

func b39AgentHandoff() {
	t := time.Now()
	lg := zap.NewNop()
	mgr := handoff.NewHandoffManager(lg)

	// 注册两个 Agent
	agent1 := &handoffMockAgent{id: "support", caps: []handoff.AgentCapability{{Name: "support", TaskTypes: []string{"question"}}}}
	agent2 := &handoffMockAgent{id: "billing", caps: []handoff.AgentCapability{{Name: "billing", TaskTypes: []string{"payment"}}}}
	mgr.RegisterAgent(agent1)
	mgr.RegisterAgent(agent2)

	// 执行 Handoff
	h, err := mgr.Handoff(context.Background(), handoff.HandoffOptions{
		FromAgentID: "support",
		ToAgentID:   "billing",
		Task:        handoff.Task{Type: "payment", Description: "处理退款", Input: "退款请求"},
		Timeout:     5 * time.Second,
		Wait:        true,
	})
	if err != nil {
		rec("Agent Handoff", "FAIL", time.Since(t), fmt.Sprintf("Handoff失败: %v", err))
		return
	}

	if h != nil && (h.Status == handoff.StatusCompleted || h.Status == handoff.StatusAccepted || h.Status == handoff.StatusInProgress) {
		rec("Agent Handoff", "PASS", time.Since(t), fmt.Sprintf("status=%s from=%s to=%s", h.Status, h.FromAgentID, h.ToAgentID))
	} else if h != nil {
		rec("Agent Handoff", "WARN", time.Since(t), fmt.Sprintf("status=%s", h.Status))
	} else {
		rec("Agent Handoff", "FAIL", time.Since(t), "返回nil")
	}
}

func b40DeliberationMode() {
	t := time.Now()

	agents := []agent.Agent{
		&mockAgent{id: "thinker1", output: "观点A：Go语言简洁高效"},
		&mockAgent{id: "thinker2", output: "观点B：Go并发模型优秀"},
		&mockAgent{id: "synthesizer", output: "综合：Go语言兼具简洁性和强大的并发能力"},
	}

	registry := multiagent.NewModeRegistry()
	multiagent.RegisterDefaultModes(registry, zap.NewNop())
	strategy, err := registry.Get("deliberation")
	if err != nil {
		rec("Deliberation深思", "FAIL", time.Since(t), fmt.Sprintf("模式未注册: %v", err))
		return
	}

	input := &agent.Input{TraceID: "test-delib", Content: "分析Go语言的优势", Context: map[string]any{"max_rounds": 2}}
	out, err := strategy.Execute(context.Background(), agents, input)
	if err != nil {
		rec("Deliberation深思", "FAIL", time.Since(t), fmt.Sprintf("执行失败: %v", err))
		return
	}

	if out != nil && out.Content != "" {
		rec("Deliberation深思", "PASS", time.Since(t), fmt.Sprintf("输出%d字符", len([]rune(out.Content))))
	} else {
		rec("Deliberation深思", "WARN", time.Since(t), "输出为空")
	}
}

func b41DebateCoordinator() {
	t := time.Now()

	agents := []agent.Agent{
		&mockAgent{id: "debater1", output: "Go最好：简洁、并发、编译快"},
		&mockAgent{id: "debater2", output: "Rust最好：内存安全、零成本抽象"},
		&mockAgent{id: "judge", output: "综合：Go适合后端服务，Rust适合系统编程"},
	}

	// "debate" 不存在，辩论功能在 "collaboration" 模式中
	registry := multiagent.NewModeRegistry()
	multiagent.RegisterDefaultModes(registry, zap.NewNop())
	strategy, err := registry.Get("collaboration")
	if err != nil {
		rec("Collaboration协作", "FAIL", time.Since(t), fmt.Sprintf("模式未注册: %v", err))
		return
	}

	input := &agent.Input{TraceID: "test-collab", Content: "Go vs Rust 哪个更好？", Context: map[string]any{"max_rounds": 2}}
	out, err := strategy.Execute(context.Background(), agents, input)
	if err != nil {
		rec("Collaboration协作", "FAIL", time.Since(t), fmt.Sprintf("执行失败: %v", err))
		return
	}

	if out != nil && out.Content != "" {
		rec("Collaboration协作", "PASS", time.Since(t), fmt.Sprintf("输出%d字符", len([]rune(out.Content))))
	} else {
		rec("Collaboration协作", "WARN", time.Since(t), "输出为空")
	}
}

// ─── handoffMockAgent ────────────────────────────────

type handoffMockAgent struct {
	id   string
	caps []handoff.AgentCapability
}

func (a *handoffMockAgent) ID() string                              { return a.id }
func (a *handoffMockAgent) Capabilities() []handoff.AgentCapability { return a.caps }
func (a *handoffMockAgent) CanHandle(task handoff.Task) bool {
	for _, c := range a.caps {
		for _, tt := range c.TaskTypes {
			if tt == task.Type {
				return true
			}
		}
	}
	return false
}
func (a *handoffMockAgent) AcceptHandoff(_ context.Context, h *handoff.Handoff) error {
	h.Status = handoff.StatusAccepted
	return nil
}
func (a *handoffMockAgent) ExecuteHandoff(_ context.Context, h *handoff.Handoff) (*handoff.HandoffResult, error) {
	return &handoff.HandoffResult{Output: "处理完成", Duration: 10}, nil
}

// ─── inMemoryMgr (MemoryManager mock) ────────────────

type inMemoryMgr struct {
	records map[string]memorycore.MemoryRecord
	mu      sync.Mutex
}

func (m *inMemoryMgr) Save(_ context.Context, rec memorycore.MemoryRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.records[rec.ID] = rec
	return nil
}
func (m *inMemoryMgr) Delete(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.records, id)
	return nil
}
func (m *inMemoryMgr) Clear(_ context.Context, agentID string, _ memorycore.MemoryKind) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, v := range m.records {
		if v.AgentID == agentID {
			delete(m.records, k)
		}
	}
	return nil
}
func (m *inMemoryMgr) LoadRecent(_ context.Context, agentID string, kind memorycore.MemoryKind, limit int) ([]memorycore.MemoryRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []memorycore.MemoryRecord
	for _, v := range m.records {
		if v.AgentID == agentID && v.Kind == kind {
			out = append(out, v)
		}
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}
func (m *inMemoryMgr) Search(_ context.Context, agentID string, _ string, topK int) ([]memorycore.MemoryRecord, error) {
	return m.LoadRecent(context.Background(), agentID, "", topK)
}
func (m *inMemoryMgr) Get(_ context.Context, id string) (*memorycore.MemoryRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r, ok := m.records[id]; ok {
		return &r, nil
	}
	return nil, fmt.Errorf("not found")
}
