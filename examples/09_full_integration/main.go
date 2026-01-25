package main

import (
	"context"
	"fmt"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/agent/collaboration"
	"github.com/BaSui01/agentflow/agent/hierarchical"
	"github.com/BaSui01/agentflow/agent/mcp"
	"github.com/BaSui01/agentflow/agent/memory"
	"github.com/BaSui01/agentflow/agent/observability"
	"github.com/BaSui01/agentflow/agent/skills"
	"go.uber.org/zap"
)

// 完整集成示例：展示如何将所有功能集成到实际项目中

func main() {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	fmt.Println("=== AgentFlow 2025 完整集成示例 ===")

	// 场景 1: 单 Agent 增强版
	fmt.Println("场景 1: 单 Agent 增强版（启用所有功能）")
	demoEnhancedSingleAgent(logger)

	// 场景 2: 层次化多 Agent 系统
	fmt.Println("\n场景 2: 层次化多 Agent 系统")
	demoHierarchicalSystem(logger)

	// 场景 3: 协作式多 Agent 系统
	fmt.Println("\n场景 3: 协作式多 Agent 系统")
	demoCollaborativeSystem(logger)

	// 场景 4: 生产环境配置
	fmt.Println("\n场景 4: 生产环境配置建议")
	demoProductionConfig(logger)
}

func demoEnhancedSingleAgent(logger *zap.Logger) {
	ctx := context.Background()

	// 1. 创建基础 Agent
	fmt.Println("\n1. 创建基础 Agent")
	config := agent.Config{
		ID:          "enhanced-agent-001",
		Name:        "Enhanced Agent",
		Type:        agent.TypeGeneric,
		Model:       "gpt-4",
		MaxTokens:   2000,
		Temperature: 0.7,

		// 启用新功能
		EnableReflection:     true,
		EnableToolSelection:  true,
		EnablePromptEnhancer: true,
		EnableSkills:         true,
		EnableEnhancedMemory: true,
		EnableObservability:  true,
	}

	// 注意：实际使用时需要提供真实的 provider
	baseAgent := agent.NewBaseAgent(config, nil, nil, nil, nil, logger)

	// 2. 启用 Reflection
	fmt.Println("\n2. 启用 Reflection 机制")
	reflectionConfig := agent.ReflectionExecutorConfig{
		Enabled:       true,
		MaxIterations: 3,
		MinQuality:    0.7,
	}
	reflectionExecutor := agent.NewReflectionExecutor(baseAgent, reflectionConfig)
	baseAgent.EnableReflection(reflectionExecutor)

	// 3. 启用动态工具选择
	fmt.Println("3. 启用动态工具选择")
	toolSelectionConfig := agent.DefaultToolSelectionConfig()
	toolSelector := agent.NewDynamicToolSelector(baseAgent, *toolSelectionConfig)
	baseAgent.EnableToolSelection(toolSelector)

	// 4. 启用提示词增强
	fmt.Println("4. 启用提示词增强")
	promptConfig := agent.DefaultPromptEngineeringConfig()
	promptEnhancer := agent.NewPromptEnhancer(promptConfig)
	baseAgent.EnablePromptEnhancer(promptEnhancer)

	// 5. 启用 Skills 系统
	fmt.Println("5. 启用 Skills 系统")
	skillsConfig := skills.DefaultSkillManagerConfig()
	skillManager := skills.NewSkillManager(skillsConfig, logger)

	// 注册一些技能
	codeReviewSkill, _ := skills.NewSkillBuilder("code-review", "代码审查").
		WithDescription("专业的代码审查技能").
		WithInstructions("审查代码质量、安全性和最佳实践").
		Build()
	skillManager.RegisterSkill(codeReviewSkill)

	baseAgent.EnableSkills(skillManager)

	// 6. 启用 MCP
	fmt.Println("6. 启用 MCP 集成")
	mcpServer := mcp.NewMCPServer("agent-mcp-server", "1.0.0", logger)
	baseAgent.EnableMCP(mcpServer)

	// 7. 启用增强记忆
	fmt.Println("7. 启用增强记忆系统")
	memoryConfig := memory.DefaultEnhancedMemoryConfig()
	enhancedMemory := memory.NewEnhancedMemorySystem(
		nil, nil, nil, nil, nil, // 实际使用时需要提供真实的存储
		memoryConfig,
		logger,
	)
	baseAgent.EnableEnhancedMemory(enhancedMemory)

	// 8. 启用可观测性
	fmt.Println("8. 启用可观测性系统")
	obsSystem := observability.NewObservabilitySystem(logger)
	baseAgent.EnableObservability(obsSystem)

	// 9. 检查功能状态
	fmt.Println("\n9. 功能状态检查")
	baseAgent.PrintFeatureStatus()
	status := baseAgent.GetFeatureStatus()

	enabledCount := 0
	for _, enabled := range status {
		if enabled {
			enabledCount++
		}
	}
	fmt.Printf("已启用功能: %d/%d\n", enabledCount, len(status))

	// 10. 使用增强执行
	fmt.Println("\n10. 执行增强任务")
	options := agent.EnhancedExecutionOptions{
		UseReflection:       true,
		UseToolSelection:    true,
		UsePromptEnhancer:   true,
		UseSkills:           true,
		UseEnhancedMemory:   true,
		LoadWorkingMemory:   true,
		LoadShortTermMemory: true,
		SaveToMemory:        true,
		UseObservability:    true,
		RecordMetrics:       true,
		RecordTrace:         true,
	}

	input := &agent.Input{
		TraceID: "trace-001",
		Content: "请审查这段代码的质量",
	}

	// 注意：实际执行需要真实的 provider
	fmt.Println("执行选项:")
	fmt.Printf("  - Reflection: %v\n", options.UseReflection)
	fmt.Printf("  - 工具选择: %v\n", options.UseToolSelection)
	fmt.Printf("  - 提示词增强: %v\n", options.UsePromptEnhancer)
	fmt.Printf("  - Skills: %v\n", options.UseSkills)
	fmt.Printf("  - 增强记忆: %v\n", options.UseEnhancedMemory)
	fmt.Printf("  - 可观测性: %v\n", options.UseObservability)

	_ = ctx
	_ = input
	_ = options
}

func demoHierarchicalSystem(logger *zap.Logger) {
	ctx := context.Background()

	fmt.Println("\n适用场景: 复杂任务需要分解和并行执行")
	fmt.Println("示例: 大型代码库审查、多文档分析、批量数据处理")

	// 1. 创建 Supervisor
	supervisorConfig := agent.Config{
		ID:          "supervisor",
		Name:        "Supervisor Agent",
		Type:        agent.TypeGeneric,
		Model:       "gpt-4",
		Description: "负责任务分解和结果聚合",
	}
	supervisor := agent.NewBaseAgent(supervisorConfig, nil, nil, nil, nil, logger)

	// 2. 创建 Workers
	workers := []agent.Agent{}
	workerTypes := []string{"analyzer", "reviewer", "optimizer"}

	for i, wType := range workerTypes {
		workerConfig := agent.Config{
			ID:          fmt.Sprintf("worker-%d", i+1),
			Name:        fmt.Sprintf("Worker %s", wType),
			Type:        agent.AgentType(wType),
			Model:       "gpt-3.5-turbo",
			Description: fmt.Sprintf("专门负责 %s", wType),
		}
		worker := agent.NewBaseAgent(workerConfig, nil, nil, nil, nil, logger)
		workers = append(workers, worker)
	}

	// 3. 创建层次化系统
	hierarchicalConfig := hierarchical.DefaultHierarchicalConfig()
	hierarchicalConfig.MaxWorkers = 3
	hierarchicalConfig.WorkerSelection = "least_loaded"
	hierarchicalConfig.EnableLoadBalance = true

	hierarchicalAgent := hierarchical.NewHierarchicalAgent(
		supervisor,
		supervisor, // supervisor implements agent.Agent interface
		workers,
		hierarchicalConfig,
		logger,
	)

	fmt.Println("\n层次化系统配置:")
	fmt.Printf("  - Supervisor: %s\n", supervisor.Name())
	fmt.Printf("  - Workers: %d 个\n", len(workers))
	fmt.Printf("  - 分配策略: %s\n", hierarchicalConfig.WorkerSelection)
	fmt.Printf("  - 负载均衡: %v\n", hierarchicalConfig.EnableLoadBalance)
	fmt.Printf("  - 任务超时: %v\n", hierarchicalConfig.TaskTimeout)

	fmt.Println("\n执行流程:")
	fmt.Println("  1. Supervisor 接收任务")
	fmt.Println("  2. 分解为 3 个子任务")
	fmt.Println("  3. 分配给 3 个 Workers 并行执行")
	fmt.Println("  4. 收集结果")
	fmt.Println("  5. Supervisor 聚合最终结果")

	_ = hierarchicalAgent
	_ = ctx
}

func demoCollaborativeSystem(logger *zap.Logger) {
	ctx := context.Background()

	fmt.Println("\n适用场景: 需要多个视角和专业意见")
	fmt.Println("示例: 决策支持、创意生成、问题诊断")

	// 1. 创建专家 Agents
	experts := []agent.Agent{}
	expertRoles := []struct {
		id   string
		name string
		desc string
	}{
		{"expert-analyst", "数据分析专家", "擅长数据分析和统计"},
		{"expert-critic", "批判性思考专家", "擅长发现问题和漏洞"},
		{"expert-creative", "创意专家", "擅长创新和头脑风暴"},
	}

	for _, role := range expertRoles {
		config := agent.Config{
			ID:          role.id,
			Name:        role.name,
			Type:        agent.TypeGeneric,
			Model:       "gpt-4",
			Description: role.desc,
		}
		expert := agent.NewBaseAgent(config, nil, nil, nil, nil, logger)
		experts = append(experts, expert)
	}

	// 2. 创建协作系统（辩论模式）
	debateConfig := collaboration.DefaultMultiAgentConfig()
	debateConfig.Pattern = collaboration.PatternDebate
	debateConfig.MaxRounds = 3
	debateConfig.ConsensusThreshold = 0.7

	debateSystem := collaboration.NewMultiAgentSystem(experts, debateConfig, logger)

	fmt.Println("\n协作系统配置:")
	fmt.Printf("  - 模式: %s\n", debateConfig.Pattern)
	fmt.Printf("  - 专家数: %d\n", len(experts))
	fmt.Printf("  - 最大轮次: %d\n", debateConfig.MaxRounds)
	fmt.Printf("  - 共识阈值: %.2f\n", debateConfig.ConsensusThreshold)

	fmt.Println("\n辩论流程:")
	fmt.Println("  第 1 轮: 每个专家提出初始观点")
	fmt.Println("  第 2 轮: 专家互相评论和改进")
	fmt.Println("  第 3 轮: 达成共识或选择最佳答案")

	// 3. 其他协作模式
	fmt.Println("\n其他可用模式:")

	patterns := []struct {
		pattern collaboration.CollaborationPattern
		desc    string
		useCase string
	}{
		{collaboration.PatternConsensus, "共识模式", "投票决策"},
		{collaboration.PatternPipeline, "流水线模式", "顺序处理"},
		{collaboration.PatternBroadcast, "广播模式", "并行处理"},
		{collaboration.PatternNetwork, "网络模式", "自由通信"},
	}

	for i, p := range patterns {
		fmt.Printf("  %d. %s - %s (适用: %s)\n", i+1, p.pattern, p.desc, p.useCase)
	}

	_ = debateSystem
	_ = ctx
}

func demoProductionConfig(logger *zap.Logger) {
	fmt.Println("\n生产环境配置建议")

	// 1. 性能优化配置
	fmt.Println("1. 性能优化配置")
	fmt.Println("```go")
	fmt.Println("config := agent.Config{")
	fmt.Println("    // 基础配置")
	fmt.Println("    Model:       \"gpt-4\",")
	fmt.Println("    MaxTokens:   2000,")
	fmt.Println("    Temperature: 0.7,")
	fmt.Println("    ")
	fmt.Println("    // 功能开关（根据需求选择）")
	fmt.Println("    EnableReflection:     true,  // 质量要求高")
	fmt.Println("    EnableToolSelection:  true,  // 工具多时启用")
	fmt.Println("    EnablePromptEnhancer: true,  // 提升成功率")
	fmt.Println("    EnableSkills:         true,  // 需要专业能力")
	fmt.Println("    EnableEnhancedMemory: true,  // 需要上下文")
	fmt.Println("    EnableObservability:  true,  // 生产必备")
	fmt.Println("}")
	fmt.Println("```")

	// 2. Reflection 配置
	fmt.Println("\n2. Reflection 配置")
	fmt.Println("```go")
	fmt.Println("reflectionConfig := agent.ReflectionExecutorConfig{")
	fmt.Println("    Enabled:       true,")
	fmt.Println("    MaxIterations: 2,      // 生产环境建议 2-3 次")
	fmt.Println("    MinQuality:    0.75,   // 根据业务要求调整")
	fmt.Println("}")
	fmt.Println("```")

	// 3. 工具选择配置
	fmt.Println("\n3. 工具选择配置")
	fmt.Println("```go")
	fmt.Println("toolConfig := agent.ToolSelectionConfig{")
	fmt.Println("    Enabled:           true,")
	fmt.Println("    SemanticWeight:    0.5,   // 语义权重")
	fmt.Println("    CostWeight:        0.3,   // 成本敏感时提高")
	fmt.Println("    LatencyWeight:     0.1,   // 延迟敏感时提高")
	fmt.Println("    ReliabilityWeight: 0.1,")
	fmt.Println("    MaxTools:          5,     // 限制工具数量")
	fmt.Println("    UseLLMRanking:     false, // 生产环境可关闭")
	fmt.Println("}")
	fmt.Println("```")

	// 4. 记忆配置
	fmt.Println("\n4. 记忆配置")
	fmt.Println("```go")
	fmt.Println("memoryConfig := memory.EnhancedMemoryConfig{")
	fmt.Println("    ShortTermTTL:          24 * time.Hour,")
	fmt.Println("    ShortTermMaxSize:      100,")
	fmt.Println("    WorkingMemorySize:     20,")
	fmt.Println("    LongTermEnabled:       true,")
	fmt.Println("    EpisodicEnabled:       true,")
	fmt.Println("    SemanticEnabled:       false, // 可选")
	fmt.Println("    ConsolidationEnabled:  true,")
	fmt.Println("    ConsolidationInterval: 1 * time.Hour,")
	fmt.Println("}")
	fmt.Println("```")

	// 5. 可观测性配置
	fmt.Println("\n5. 可观测性配置")
	fmt.Println("```go")
	fmt.Println("// 设置告警阈值")
	fmt.Println("alerts := map[string]float64{")
	fmt.Println("    \"success_rate_min\":  0.70,  // 成功率 < 70% 告警")
	fmt.Println("    \"p95_latency_max\":   1000,  // P95 延迟 > 1s 告警")
	fmt.Println("    \"cost_per_task_max\": 0.10,  // 成本 > $0.10 告警")
	fmt.Println("    \"quality_min\":       6.0,   // 质量 < 6.0 告警")
	fmt.Println("}")
	fmt.Println("```")

	// 6. 部署建议
	fmt.Println("\n6. 部署建议")
	fmt.Println("  a) 基础设施:")
	fmt.Println("     - Redis: 短期记忆和缓存")
	fmt.Println("     - PostgreSQL: 元数据和配置")
	fmt.Println("     - Qdrant/Pinecone: 向量存储")
	fmt.Println("     - InfluxDB: 时序数据（可选）")
	fmt.Println("     - Prometheus: 指标监控")
	fmt.Println("     - Grafana: 可视化仪表板")

	fmt.Println("\n  b) 性能调优:")
	fmt.Println("     - 启用 Redis 缓存（幂等性）")
	fmt.Println("     - 使用连接池")
	fmt.Println("     - 设置合理的超时时间")
	fmt.Println("     - 启用熔断器")
	fmt.Println("     - 限流保护")

	fmt.Println("\n  c) 成本优化:")
	fmt.Println("     - 使用动态工具选择减少 Token")
	fmt.Println("     - 启用缓存减少重复调用")
	fmt.Println("     - 根据任务复杂度选择模型")
	fmt.Println("     - 监控成本指标")

	fmt.Println("\n  d) 质量保证:")
	fmt.Println("     - 启用 Reflection 提升质量")
	fmt.Println("     - 使用基准测试持续评估")
	fmt.Println("     - 收集用户反馈")
	fmt.Println("     - A/B 测试不同配置")

	// 7. 监控指标
	fmt.Println("\n7. 关键监控指标")
	fmt.Println("  - 任务成功率 (目标: > 85%)")
	fmt.Println("  - P50/P95/P99 延迟")
	fmt.Println("  - Token 消耗趋势")
	fmt.Println("  - 每任务成本")
	fmt.Println("  - 输出质量分数")
	fmt.Println("  - 错误率和类型")
	fmt.Println("  - 缓存命中率")

	// 8. 渐进式启用
	fmt.Println("\n8. 渐进式启用策略")
	fmt.Println("  阶段 1 (第 1 周):")
	fmt.Println("    - 启用可观测性")
	fmt.Println("    - 启用提示词增强")
	fmt.Println("    - 收集基线数据")
	fmt.Println("  ")
	fmt.Println("  阶段 2 (第 2-3 周):")
	fmt.Println("    - 启用动态工具选择")
	fmt.Println("    - 启用增强记忆")
	fmt.Println("    - 对比性能提升")
	fmt.Println("  ")
	fmt.Println("  阶段 3 (第 4 周):")
	fmt.Println("    - 启用 Reflection")
	fmt.Println("    - 启用 Skills 系统")
	fmt.Println("    - 全面评估效果")
	fmt.Println("  ")
	fmt.Println("  阶段 4 (第 5+ 周):")
	fmt.Println("    - 根据需求启用 MCP")
	fmt.Println("    - 考虑多 Agent 协作")
	fmt.Println("    - 持续优化调参")
}
