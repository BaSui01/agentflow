package main

import (
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/agent/collaboration"
	"github.com/BaSui01/agentflow/agent/hierarchical"
	"github.com/BaSui01/agentflow/agent/observability"
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
	supervisorConfig := agent.Config{
		ID:          "supervisor",
		Name:        "Supervisor Agent",
		Type:        agent.TypeGeneric,
		Model:       "gpt-4",
		MaxTokens:   2000,
		Temperature: 0.7,
	}

	// 注意：实际使用时需要提供真实的 LLM provider，此处传 nil 仅演示结构
	supervisor := agent.NewBaseAgent(supervisorConfig, nil, nil, nil, nil, logger)

	// 2. 创建 Worker Agents
	fmt.Println("2. 创建 Worker Agents")
	workers := []agent.Agent{}

	for i := 1; i <= 3; i++ {
		workerConfig := agent.Config{
			ID:          fmt.Sprintf("worker-%d", i),
			Name:        fmt.Sprintf("Worker Agent %d", i),
			Type:        agent.TypeGeneric,
			Model:       "gpt-3.5-turbo",
			MaxTokens:   1000,
			Temperature: 0.7,
		}
		worker := agent.NewBaseAgent(workerConfig, nil, nil, nil, nil, logger)
		workers = append(workers, worker)
	}

	fmt.Printf("创建了 1 个 Supervisor 和 %d 个 Workers\n", len(workers))

	// 3. 创建层次化 Agent
	fmt.Println("\n3. 创建层次化 Agent")
	hierarchicalConfig := hierarchical.DefaultHierarchicalConfig()
	hierarchicalConfig.MaxWorkers = 3
	hierarchicalConfig.WorkerSelection = "round_robin"

	hierarchicalAgent := hierarchical.NewHierarchicalAgent(
		supervisor,
		supervisor,
		workers,
		hierarchicalConfig,
		logger,
	)

	// 4. 打印实际配置（从对象读取，而非硬编码字符串）
	fmt.Println("层次化 Agent 已创建")
	fmt.Printf("配置:\n")
	fmt.Printf("  - 最大 Workers: %d\n", hierarchicalConfig.MaxWorkers)
	fmt.Printf("  - 任务超时: %v\n", hierarchicalConfig.TaskTimeout)
	fmt.Printf("  - 工作者选择策略: %s\n", hierarchicalConfig.WorkerSelection)
	fmt.Printf("  - 启用重试: %v\n", hierarchicalConfig.EnableRetry)

	// 5. 打印 Worker 实际状态
	fmt.Println("\n5. Worker 实际状态")
	for _, w := range workers {
		fmt.Printf("  - %s: 状态=%s\n", w.ID(), w.State())
	}

	_ = hierarchicalAgent
}

func demoMultiAgentCollaboration(logger *zap.Logger) {
	// 1. 创建多个 Agent
	fmt.Println("1. 创建协作 Agents")
	agents := []agent.Agent{}

	agentRoles := []string{"Analyst", "Critic", "Synthesizer"}
	for i, role := range agentRoles {
		config := agent.Config{
			ID:          fmt.Sprintf("agent-%d", i+1),
			Name:        fmt.Sprintf("%s Agent", role),
			Type:        agent.TypeGeneric,
			Model:       "gpt-4",
			MaxTokens:   1500,
			Temperature: 0.7,
		}
		a := agent.NewBaseAgent(config, nil, nil, nil, nil, logger)
		agents = append(agents, a)
	}

	fmt.Printf("创建了 %d 个协作 Agents\n", len(agents))

	// 2. 展示所有协作模式
	fmt.Println("\n2. 可用协作模式")
	patterns := []struct {
		name    string
		pattern collaboration.CollaborationPattern
	}{
		{"辩论模式", collaboration.PatternDebate},
		{"共识模式", collaboration.PatternConsensus},
		{"流水线模式", collaboration.PatternPipeline},
		{"广播模式", collaboration.PatternBroadcast},
		{"网络模式", collaboration.PatternNetwork},
	}

	for i, p := range patterns {
		fmt.Printf("  %d. %s (%s)\n", i+1, p.name, p.pattern)
	}

	// 3. 创建辩论模式系统并打印实际配置
	fmt.Println("\n3. 创建辩论模式系统")
	debateConfig := collaboration.DefaultMultiAgentConfig()
	debateConfig.Pattern = collaboration.PatternDebate
	debateConfig.MaxRounds = 3

	debateSystem := collaboration.NewMultiAgentSystem(agents, debateConfig, logger)

	fmt.Printf("配置:\n")
	fmt.Printf("  - 模式: %s\n", debateConfig.Pattern)
	fmt.Printf("  - 最大轮次: %d\n", debateConfig.MaxRounds)
	fmt.Printf("  - 共识阈值: %.2f\n", debateConfig.ConsensusThreshold)

	// 4. 创建流水线模式系统
	fmt.Println("\n4. 创建流水线模式系统")
	pipelineConfig := collaboration.DefaultMultiAgentConfig()
	pipelineConfig.Pattern = collaboration.PatternPipeline

	pipelineSystem := collaboration.NewMultiAgentSystem(agents, pipelineConfig, logger)
	fmt.Printf("  模式: %s\n", pipelineConfig.Pattern)

	// 5. 创建广播模式系统
	fmt.Println("\n5. 创建广播模式系统")
	broadcastConfig := collaboration.DefaultMultiAgentConfig()
	broadcastConfig.Pattern = collaboration.PatternBroadcast

	broadcastSystem := collaboration.NewMultiAgentSystem(agents, broadcastConfig, logger)
	fmt.Printf("  模式: %s\n", broadcastConfig.Pattern)

	// 注意：实际执行需要真实的 LLM provider
	// 调用 debateSystem.Execute(ctx, input) 即可启动辩论

	_ = debateSystem
	_ = pipelineSystem
	_ = broadcastSystem
}

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
}
