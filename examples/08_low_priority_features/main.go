package main

import (
	"context"
	"fmt"
	"time"

	agentcore "github.com/BaSui01/agentflow/agent/core"
	"github.com/BaSui01/agentflow/agent/observability/monitoring"
	agent "github.com/BaSui01/agentflow/agent/runtime"
	runtime "github.com/BaSui01/agentflow/agent/runtime"
	"github.com/BaSui01/agentflow/agent/team"
	llm "github.com/BaSui01/agentflow/llm/core"
	llmgateway "github.com/BaSui01/agentflow/llm/gateway"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// 演示低优先级功能：层次化架构、多 Agent 协作、可观测性系统

func main() {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	fmt.Println("=== 低优先级功能演示 ===")

	// 示例 1: 层次化架构
	fmt.Println("=== 示例 1: 层次化架构 (Supervisor-Worker) ===")
	demoHierarchicalArchitecture(logger)

	fmt.Println("\n=== 示例 2: 多 Agent 协作 ===")
	demoMultiAgentCollaboration(logger)

	fmt.Println("\n=== 示例 3: 可观测性系统 ===")
	demoObservabilitySystem(logger)
}

func demoHierarchicalArchitecture(logger *zap.Logger) {
	// 1. 创建 Supervisor Agent
	fmt.Println("1. 创建 Supervisor Agent")
	supervisorConfig := types.AgentConfig{
		Core: types.CoreConfig{
			ID:   "supervisor",
			Name: "Supervisor Agent",
			Type: string(agent.TypeGeneric),
		},
		LLM: types.LLMConfig{
			Model:       "gpt-4",
			MaxTokens:   2000,
			Temperature: 0.7,
		},
	}

	// 注意：实际使用时需要提供真实的 LLM provider，此处传 nil 仅演示结构
	supervisor := mustBuildDemoAgent(context.Background(), supervisorConfig, logger)

	// 2. 创建 Worker Agents
	fmt.Println("2. 创建 Worker Agents")
	workers := []agent.Agent{}

	for i := 1; i <= 3; i++ {
		workerConfig := types.AgentConfig{
			Core: types.CoreConfig{
				ID:   fmt.Sprintf("worker-%d", i),
				Name: fmt.Sprintf("Worker Agent %d", i),
				Type: string(agent.TypeGeneric),
			},
			LLM: types.LLMConfig{
				Model:       "gpt-3.5-turbo",
				MaxTokens:   1000,
				Temperature: 0.7,
			},
		}
		worker := mustBuildDemoAgent(context.Background(), workerConfig, logger)
		workers = append(workers, worker)
	}

	fmt.Printf("创建了 1 个 Supervisor 和 %d 个 Workers\n", len(workers))

	// 3. 通过官方 TeamBuilder 创建层次化团队
	fmt.Println("\n3. 创建层次化 Team")
	builder := team.NewTeamBuilder("hierarchical-demo").
		WithMode(team.ModeSupervisor).
		WithMaxRounds(3).
		WithTimeout(3*time.Minute).
		AddMember(supervisor, "supervisor")
	for _, w := range workers {
		builder.AddMember(w, "worker")
	}
	hierarchicalTeam, err := builder.Build(logger)
	if err != nil {
		fmt.Printf("层次化 Team 创建失败: %v\n", err)
		return
	}

	// 4. 打印实际配置
	fmt.Println("层次化 Team 已创建")
	fmt.Printf("配置:\n")
	fmt.Printf("  - Team ID: %s\n", hierarchicalTeam.ID())
	fmt.Printf("  - 模式: %s\n", team.ModeSupervisor)
	fmt.Printf("  - 成员数: %d\n", len(hierarchicalTeam.Members()))
	fmt.Printf("  - 超时: %v\n", 3*time.Minute)

	// 5. 打印 Worker 实际状态
	fmt.Println("\n5. Worker 实际状态")
	for _, w := range workers {
		fmt.Printf("  - %s: 状态=%s\n", w.ID(), w.State())
	}
}

func demoMultiAgentCollaboration(logger *zap.Logger) {
	// 1. 创建多个 Agent
	fmt.Println("1. 创建协作 Agents")
	agents := []agent.Agent{}

	agentRoles := []string{"Analyst", "Critic", "Synthesizer"}
	for i, role := range agentRoles {
		config := types.AgentConfig{
			Core: types.CoreConfig{
				ID:   fmt.Sprintf("agent-%d", i+1),
				Name: fmt.Sprintf("%s Agent", role),
				Type: string(agent.TypeGeneric),
			},
			LLM: types.LLMConfig{
				Model:       "gpt-4",
				MaxTokens:   1500,
				Temperature: 0.7,
			},
		}
		a := mustBuildDemoAgent(context.Background(), config, logger)
		agents = append(agents, a)
	}

	fmt.Printf("创建了 %d 个协作 Agents\n", len(agents))

	// 2. 展示所有协作模式
	fmt.Println("\n2. 可用协作模式")
	patterns := []struct {
		name string
		mode string
	}{
		{"协作执行", string(team.ExecutionModeCollaboration)},
		{"审议执行", string(team.ExecutionModeDeliberation)},
		{"并行执行", string(team.ExecutionModeParallel)},
		{"Supervisor 团队", string(team.ModeSupervisor)},
		{"Round-robin 团队", string(team.ModeRoundRobin)},
	}

	for i, p := range patterns {
		fmt.Printf("  %d. %s (%s)\n", i+1, p.name, p.mode)
	}

	fmt.Println("\n3. 创建 Supervisor 协作团队")
	supervisorTeam := team.NewTeamBuilder("supervisor-collaboration").
		WithMode(team.ModeSupervisor).
		WithMaxRounds(3).
		WithTimeout(2 * time.Minute)
	for i, a := range agents {
		role := "worker"
		if i == 0 {
			role = "supervisor"
		}
		supervisorTeam.AddMember(a, role)
	}
	builtSupervisorTeam, err := supervisorTeam.Build(logger)
	if err != nil {
		fmt.Printf("  创建失败: %v\n", err)
	} else {
		fmt.Printf("  Team ID: %s, 成员数: %d\n", builtSupervisorTeam.ID(), len(builtSupervisorTeam.Members()))
	}

	fmt.Println("\n4. 创建 Round-robin 协作团队")
	roundRobinTeam := team.NewTeamBuilder("round-robin-collaboration").
		WithMode(team.ModeRoundRobin).
		WithMaxRounds(3).
		WithTimeout(2 * time.Minute)
	for _, a := range agents {
		roundRobinTeam.AddMember(a, "collaborator")
	}
	builtRoundRobinTeam, err := roundRobinTeam.Build(logger)
	if err != nil {
		fmt.Printf("  创建失败: %v\n", err)
	} else {
		fmt.Printf("  Team ID: %s, 成员数: %d\n", builtRoundRobinTeam.ID(), len(builtRoundRobinTeam.Members()))
	}

	fmt.Println("\n5. 执行门面")
	for _, mode := range team.SupportedExecutionModes() {
		fmt.Printf("  - %s\n", mode)
	}
}

func mustBuildDemoAgent(ctx context.Context, cfg types.AgentConfig, logger *zap.Logger) *agent.BaseAgent {
	gateway := llmgateway.New(llmgateway.Config{
		ChatProvider: noopProvider{},
		Logger:       logger,
	})
	ag, err := runtime.NewBuilder(gateway, logger).Build(ctx, cfg)
	if err != nil {
		panic(fmt.Sprintf("build demo agent %s failed: %v", cfg.Core.ID, err))
	}
	return ag
}

type noopProvider struct{}

func (noopProvider) Completion(_ context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	return &llm.ChatResponse{
		Model: req.Model,
		Choices: []llm.ChatChoice{{
			Index: 0,
			Message: types.Message{
				Role:    llm.RoleAssistant,
				Content: "[noop provider]",
			},
		}},
	}, nil
}

func (noopProvider) Stream(_ context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	ch := make(chan llm.StreamChunk, 1)
	ch <- llm.StreamChunk{
		Model: req.Model,
		Delta: types.Message{
			Role:    llm.RoleAssistant,
			Content: "[noop provider]",
		},
		FinishReason: "stop",
	}
	close(ch)
	return ch, nil
}

func (noopProvider) HealthCheck(context.Context) (*llm.HealthStatus, error) {
	return &llm.HealthStatus{Healthy: true}, nil
}

func (noopProvider) Name() string { return "noop" }

func (noopProvider) SupportsNativeFunctionCalling() bool { return false }

func (noopProvider) ListModels(context.Context) ([]llm.Model, error) { return nil, nil }

func (noopProvider) Endpoints() llm.ProviderEndpoints { return llm.ProviderEndpoints{} }

func demoObservabilitySystem(logger *zap.Logger) {
	// 1. 创建可观测性系统的各个组件
	fmt.Println("1. 创建可观测性系统")
	collector := observability.NewMetricsCollector(logger)
	tracer := observability.NewTracer(logger)

	fmt.Println("可观测性系统已创建")

	// 2. 使用 MetricsCollector 记录真实的任务指标
	fmt.Println("\n2. 指标收集（真实 API 调用）")
	testData := []struct {
		success bool
		latency time.Duration
		tokens  int
		cost    float64
		quality float64
	}{
		{true, 120 * time.Millisecond, 500, 0.01, 8.5},
		{true, 150 * time.Millisecond, 600, 0.012, 7.8},
		{false, 200 * time.Millisecond, 400, 0.008, 0},
		{true, 130 * time.Millisecond, 550, 0.011, 9.0},
		{true, 180 * time.Millisecond, 700, 0.014, 8.2},
	}

	for _, d := range testData {
		collector.RecordTask("demo-agent", d.success, d.latency, d.tokens, d.cost, d.quality)
	}

	// 3. 读取真实的指标数据
	metrics := collector.GetMetrics("demo-agent")
	if metrics != nil {
		fmt.Printf("\n指标统计（来自 MetricsCollector）:\n")
		fmt.Printf("  - 总任务数: %d\n", metrics.TotalTasks)
		fmt.Printf("  - 成功任务: %d\n", metrics.SuccessfulTasks)
		fmt.Printf("  - 失败任务: %d\n", metrics.FailedTasks)
		fmt.Printf("  - 成功率: %.2f%%\n", metrics.TaskSuccessRate*100)
		fmt.Printf("  - 平均延迟: %v\n", metrics.AvgLatency)
		fmt.Printf("  - P50 延迟: %v\n", metrics.P50Latency)
		fmt.Printf("  - P95 延迟: %v\n", metrics.P95Latency)
		fmt.Printf("  - 总 Token: %d\n", metrics.TotalTokens)
		fmt.Printf("  - 总成本: $%.3f\n", metrics.TotalCost)
		fmt.Printf("  - 每任务成本: $%.4f\n", metrics.CostPerTask)
		fmt.Printf("  - 平均质量: %.2f\n", metrics.AvgOutputQuality)
	}

	// 4. 使用 Tracer 记录真实的追踪数据
	fmt.Println("\n4. 追踪系统（真实 API 调用）")
	trace := tracer.StartTrace("trace-demo-001", "demo-agent")
	fmt.Printf("  追踪已开始: ID=%s, Agent=%s\n", trace.TraceID, trace.AgentID)

	// 添加 Span
	tracer.AddSpan("trace-demo-001", &observability.Span{
		Name:       "llm_call",
		StartTime:  time.Now(),
		EndTime:    time.Now().Add(150 * time.Millisecond),
		Attributes: map[string]any{"model": "gpt-4", "tokens": 500},
	})

	tracer.EndTrace("trace-demo-001", "completed", nil)

	// 读取追踪结果
	completedTrace := tracer.GetTrace("trace-demo-001")
	if completedTrace != nil {
		fmt.Printf("  追踪完成: 状态=%s, Spans=%d\n", completedTrace.Status, len(completedTrace.Spans))
	}

	// 5. 获取所有 Agent 的指标
	fmt.Println("\n5. 所有 Agent 指标汇总")
	allMetrics := collector.GetAllMetrics()
	fmt.Printf("  已监控 Agent 数: %d\n", len(allMetrics))
	for agentID, m := range allMetrics {
		fmt.Printf("  - %s: 任务=%d, 成功率=%.0f%%, 平均延迟=%v\n",
			agentID, m.TotalTasks, m.TaskSuccessRate*100, m.AvgLatency)
	}

	// 6. Explainability tracker paths
	explainCfg := observability.DefaultExplainabilityConfig()
	explainCfg.MaxTraceAge = time.Minute
	explainCfg.MaxTracesPerAgent = 10
	tracker := observability.NewExplainabilityTracker(explainCfg)
	rTrace := tracker.StartTrace("session-1", "demo-agent")
	if rTrace != nil {
		tracker.AddStep(rTrace.ID, observability.ReasoningStep{
			Type:    "thought",
			Content: "Analyze query and choose tool",
		})
		decision := observability.Decision{
			Type:        observability.DecisionToolSelection,
			Description: "choose search tool",
			Reasoning:   "query needs external facts",
			Confidence:  0.9,
			Alternatives: []observability.Alternative{
				{Option: "search", Score: 0.9, Reason: "has latest info", WasChosen: true},
				{Option: "local_cache", Score: 0.4, Reason: "may be stale"},
			},
			Factors: []observability.Factor{
				{Name: "freshness", Weight: 0.8, Impact: "positive", Explanation: "latest data required"},
			},
		}
		tracker.RecordDecision(rTrace.ID, decision)
		tracker.EndTrace(rTrace.ID, true, "completed", "")
		_ = tracker.GetTrace(rTrace.ID)
		_ = tracker.GetAgentTraces("demo-agent")
		_ = tracker.ExplainDecision(decision)
		report, repErr := tracker.GenerateAuditReport(rTrace.ID)
		if repErr == nil && report != nil {
			_, _ = report.Export()
		}
	}

	// 7. Simple evaluation strategy path
	eval := &observability.SimpleEvaluationStrategy{}
	_, _ = eval.Evaluate(context.Background(), &agentcore.Input{
		TraceID: "trace-eval",
		Content: "summarize this incident",
	}, &agentcore.Output{
		TraceID: "trace-eval",
		Content: "incident summary output",
	})
}
