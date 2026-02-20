package main

import (
	"context"
	"fmt"

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
	_ = context.Background()

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

	// 注意：实际使用时需要提供真实的 provider
	var provider any = nil
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
		supervisor, // supervisor implements agent.Agent interface
		workers,
		hierarchicalConfig,
		logger,
	)

	fmt.Println("层次化 Agent 已创建")
	fmt.Printf("配置:\n")
	fmt.Printf("  - 最大 Workers: %d\n", hierarchicalConfig.MaxWorkers)
	fmt.Printf("  - 任务超时: %v\n", hierarchicalConfig.TaskTimeout)
	fmt.Printf("  - 工作者选择策略: %s\n", hierarchicalConfig.WorkerSelection)
	fmt.Printf("  - 启用重试: %v\n", hierarchicalConfig.EnableRetry)

	// 4. 演示任务分解和执行流程
	fmt.Println("\n4. 任务执行流程")
	fmt.Println("  ┌─────────────────┐")
	fmt.Println("  │   Supervisor    │ ← 接收任务")
	fmt.Println("  └────────┬────────┘")
	fmt.Println("           │ 分解任务")
	fmt.Println("  ┌────────▼────────┐")
	fmt.Println("  │  Task Coordinator│ ← 任务协调")
	fmt.Println("  └────────┬────────┘")
	fmt.Println("           │ 分配任务")
	fmt.Println("     ┌─────┼─────┐")
	fmt.Println("  ┌──▼──┐ ┌▼───┐ ┌▼───┐")
	fmt.Println("  │ W-1 │ │W-2 │ │W-3 │ ← 并行执行")
	fmt.Println("  └──┬──┘ └┬───┘ └┬───┘")
	fmt.Println("     └─────┼─────┘")
	fmt.Println("           │ 返回结果")
	fmt.Println("  ┌────────▼────────┐")
	fmt.Println("  │   Supervisor    │ ← 聚合结果")
	fmt.Println("  └─────────────────┘")

	// 5. 任务分配策略
	fmt.Println("\n5. 任务分配策略")
	fmt.Println("  a) Round Robin: 轮询分配")
	fmt.Println("     - Worker 1 -> Worker 2 -> Worker 3 -> Worker 1 ...")
	fmt.Println("  b) Least Loaded: 最少负载")
	fmt.Println("     - 选择当前负载最低的 Worker")
	fmt.Println("  c) Random: 随机分配")
	fmt.Println("     - 随机选择空闲的 Worker")

	// 6. Worker 状态监控
	fmt.Println("\n6. Worker 状态监控")
	fmt.Println("Worker 状态:")
	fmt.Println("  - worker-1: 状态=ready, 完成=0, 失败=0, 负载=0.00")
	fmt.Println("  - worker-2: 状态=ready, 完成=0, 失败=0, 负载=0.00")
	fmt.Println("  - worker-3: 状态=ready, 完成=0, 失败=0, 负载=0.00")

	// 7. 优势
	fmt.Println("\n7. 层次化架构优势")
	fmt.Println("  - 任务分解: 复杂任务拆分为简单子任务")
	fmt.Println("  - 并行执行: 多个 Worker 同时工作")
	fmt.Println("  - 负载均衡: 智能分配任务")
	fmt.Println("  - 容错能力: 单个 Worker 失败不影响整体")
	fmt.Println("  - 可扩展性: 轻松添加更多 Workers")

	_ = hierarchicalAgent
	_ = provider
}

func demoMultiAgentCollaboration(logger *zap.Logger) {
	ctx := context.Background()

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

	// 2. 协作模式
	fmt.Println("\n2. 协作模式")
	patterns := []struct {
		name        string
		pattern     collaboration.CollaborationPattern
		description string
	}{
		{"辩论模式", collaboration.PatternDebate, "多个 Agent 辩论达成最佳答案"},
		{"共识模式", collaboration.PatternConsensus, "通过投票达成共识"},
		{"流水线模式", collaboration.PatternPipeline, "顺序执行，前一个输出是后一个输入"},
		{"广播模式", collaboration.PatternBroadcast, "并行执行，合并所有结果"},
		{"网络模式", collaboration.PatternNetwork, "Agent 之间自由通信"},
	}

	for i, p := range patterns {
		fmt.Printf("  %d. %s (%s)\n     %s\n", i+1, p.name, p.pattern, p.description)
	}

	// 3. 创建辩论模式系统
	fmt.Println("\n3. 创建辩论模式系统")
	debateConfig := collaboration.DefaultMultiAgentConfig()
	debateConfig.Pattern = collaboration.PatternDebate
	debateConfig.MaxRounds = 3

	debateSystem := collaboration.NewMultiAgentSystem(agents, debateConfig, logger)

	fmt.Println("辩论系统已创建")
	fmt.Printf("配置:\n")
	fmt.Printf("  - 模式: %s\n", debateConfig.Pattern)
	fmt.Printf("  - 最大轮次: %d\n", debateConfig.MaxRounds)
	fmt.Printf("  - 共识阈值: %.2f\n", debateConfig.ConsensusThreshold)

	// 4. 辩论流程
	fmt.Println("\n4. 辩论模式流程")
	fmt.Println("  第 1 轮:")
	fmt.Println("    Agent 1: 提出观点 A")
	fmt.Println("    Agent 2: 提出观点 B")
	fmt.Println("    Agent 3: 提出观点 C")
	fmt.Println("  第 2 轮:")
	fmt.Println("    Agent 1: 评论 B 和 C，改进 A")
	fmt.Println("    Agent 2: 评论 A 和 C，改进 B")
	fmt.Println("    Agent 3: 评论 A 和 B，改进 C")
	fmt.Println("  第 3 轮:")
	fmt.Println("    继续辩论和改进...")
	fmt.Println("  最终:")
	fmt.Println("    选择最佳答案或达成共识")

	// 5. 流水线模式
	fmt.Println("\n5. 流水线模式示例")
	pipelineConfig := collaboration.DefaultMultiAgentConfig()
	pipelineConfig.Pattern = collaboration.PatternPipeline

	pipelineSystem := collaboration.NewMultiAgentSystem(agents, pipelineConfig, logger)

	fmt.Println("流水线流程:")
	fmt.Println("  输入 → Agent 1 (分析) → Agent 2 (批评) → Agent 3 (综合) → 输出")
	fmt.Println("  每个 Agent 的输出成为下一个 Agent 的输入")

	// 6. 广播模式
	fmt.Println("\n6. 广播模式示例")
	broadcastConfig := collaboration.DefaultMultiAgentConfig()
	broadcastConfig.Pattern = collaboration.PatternBroadcast

	broadcastSystem := collaboration.NewMultiAgentSystem(agents, broadcastConfig, logger)

	fmt.Println("广播流程:")
	fmt.Println("         ┌─────────┐")
	fmt.Println("  输入 ──┤ 广播器  ├── Agent 1 ──┐")
	fmt.Println("         │         ├── Agent 2 ──┼── 聚合 → 输出")
	fmt.Println("         └─────────┘── Agent 3 ──┘")
	fmt.Println("  所有 Agent 并行处理，结果合并")

	// 7. 消息通信
	fmt.Println("\n7. Agent 间消息通信")
	fmt.Println("  消息类型:")
	fmt.Println("    - Proposal: 提案")
	fmt.Println("    - Response: 响应")
	fmt.Println("    - Vote: 投票")
	fmt.Println("    - Consensus: 共识")
	fmt.Println("    - Broadcast: 广播")

	// 8. 优势
	fmt.Println("\n8. 多 Agent 协作优势")
	fmt.Println("  - 多样性: 不同视角和专长")
	fmt.Println("  - 鲁棒性: 单个 Agent 错误不影响整体")
	fmt.Println("  - 质量提升: 通过辩论和批评改进")
	fmt.Println("  - 灵活性: 多种协作模式适应不同场景")

	_ = debateSystem
	_ = pipelineSystem
	_ = broadcastSystem
	_ = ctx
}

func demoObservabilitySystem(logger *zap.Logger) {
	_ = context.Background()

	// 1. 创建可观测性系统
	fmt.Println("1. 创建可观测性系统")
	_ = observability.NewObservabilitySystem(logger)

	fmt.Println("可观测性系统已创建")
	fmt.Println("组件:")
	fmt.Println("  - MetricsCollector: 指标收集")
	fmt.Println("  - Tracer: 追踪记录")
	fmt.Println("  - Evaluator: 质量评估")

	// 2. 指标收集
	fmt.Println("\n2. 指标收集")
	fmt.Println("模拟记录 10 个任务...")

	// 显示示例指标
	fmt.Println("\n指标统计:")
	fmt.Println("  - 总任务数: 10")
	fmt.Println("  - 成功任务: 8")
	fmt.Println("  - 失败任务: 2")
	fmt.Println("  - 成功率: 80.00%")
	fmt.Println("  - 平均延迟: 145ms")
	fmt.Println("  - P95 延迟: 190ms")
	fmt.Println("  - 总 Token: 5450")
	fmt.Println("  - 总成本: $0.10")
	fmt.Println("  - 平均质量: 7.90")

	// 3. 追踪系统
	fmt.Println("\n3. 追踪系统")
	fmt.Println("开始追踪: trace-001")
	fmt.Println("  - Agent: agent-001")
	fmt.Println("  - 状态: running")
	fmt.Println("  - 开始时间: 2025-01-26 10:00:00")
	fmt.Println("\n追踪完成:")
	fmt.Println("  - 状态: completed")
	fmt.Println("  - 持续时间: 1.5s")
	fmt.Println("  - 步骤数: 3")

	// 4. 评估系统
	fmt.Println("\n4. 评估系统")
	fmt.Println("评估结果:")
	fmt.Println("  - 总分: 8.50")
	fmt.Println("  - 维度分数:")
	fmt.Println("    - 准确性: 9.0")
	fmt.Println("    - 完整性: 8.5")
	fmt.Println("    - 清晰度: 8.0")
	fmt.Println("    - 相关性: 8.5")

	// 5. 基准测试
	fmt.Println("\n5. 基准测试")
	fmt.Println("已注册基准测试: QA Benchmark")
	fmt.Println("测试用例数: 2")

	// 6. 可观测性仪表板
	fmt.Println("\n6. 可观测性仪表板")
	fmt.Println("  ┌─────────────────────────────────────┐")
	fmt.Println("  │        Agent 性能仪表板              │")
	fmt.Println("  ├─────────────────────────────────────┤")
	fmt.Println("  │ 成功率:  ████████░░ 80%             │")
	fmt.Println("  │ 延迟:    P50=150ms P95=280ms        │")
	fmt.Println("  │ 质量:    ████████░░ 8.2/10          │")
	fmt.Println("  │ 成本:    $0.055/task                │")
	fmt.Println("  │ Token:   550 tokens/task            │")
	fmt.Println("  └─────────────────────────────────────┘")

	// 7. 告警规则
	fmt.Println("\n7. 告警规则示例")
	fmt.Println("  - 成功率 < 70%: 发送告警")
	fmt.Println("  - P95 延迟 > 1s: 发送告警")
	fmt.Println("  - 成本 > $0.10/task: 发送告警")
	fmt.Println("  - 质量分数 < 6.0: 发送告警")

	// 8. 优势
	fmt.Println("\n8. 可观测性系统优势")
	fmt.Println("  - 实时监控: 实时了解 Agent 性能")
	fmt.Println("  - 问题定位: 快速定位性能瓶颈")
	fmt.Println("  - 质量保证: 持续评估输出质量")
	fmt.Println("  - 成本优化: 监控和优化成本")
	fmt.Println("  - 数据驱动: 基于数据做决策")
}
