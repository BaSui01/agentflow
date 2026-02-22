package main

import (
	"context"
	"fmt"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/agent/collaboration"
	"github.com/BaSui01/agentflow/agent/hierarchical"
	"github.com/BaSui01/agentflow/agent/protocol/mcp"
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

	// 1. 性能优化配置 — 使用真实的 Config 结构体
	fmt.Println("1. 性能优化配置")
	prodConfig := agent.Config{
		ID:          "prod-agent",
		Name:        "Production Agent",
		Type:        agent.TypeGeneric,
		Model:       "gpt-4",
		MaxTokens:   2000,
		Temperature: 0.7,

		EnableReflection:     true,
		EnableToolSelection:  true,
		EnablePromptEnhancer: true,
		EnableSkills:         true,
		EnableEnhancedMemory: true,
		EnableObservability:  true,
	}
	fmt.Printf("  Model: %s, MaxTokens: %d, Temperature: %.1f\n",
		prodConfig.Model, prodConfig.MaxTokens, prodConfig.Temperature)

	// 2. Reflection 配置
	fmt.Println("\n2. Reflection 配置")
	reflectionConfig := agent.ReflectionExecutorConfig{
		Enabled:       true,
		MaxIterations: 2,
		MinQuality:    0.75,
	}
	fmt.Printf("  MaxIterations: %d, MinQuality: %.2f\n",
		reflectionConfig.MaxIterations, reflectionConfig.MinQuality)

	// 3. 工具选择配置
	fmt.Println("\n3. 工具选择配置")
	toolConfig := agent.DefaultToolSelectionConfig()
	fmt.Printf("  MaxTools: %d, UseLLMRanking: %v\n",
		toolConfig.MaxTools, toolConfig.UseLLMRanking)

	// 4. 记忆配置
	fmt.Println("\n4. 记忆配置")
	memoryConfig := memory.DefaultEnhancedMemoryConfig()
	fmt.Printf("  ShortTermTTL: %v, WorkingMemorySize: %d\n",
		memoryConfig.ShortTermTTL, memoryConfig.WorkingMemorySize)

	// 5. 可观测性 — 使用真实的 MetricsCollector 演示告警检查
	fmt.Println("\n5. 可观测性告警检查")
	collector := observability.NewMetricsCollector(logger)
	// 模拟一些任务数据
	collector.RecordTask("prod-agent", true, 100*1e6, 500, 0.01, 8.0)
	collector.RecordTask("prod-agent", true, 200*1e6, 600, 0.012, 7.5)
	collector.RecordTask("prod-agent", false, 500*1e6, 400, 0.008, 3.0)

	m := collector.GetMetrics("prod-agent")
	if m != nil {
		alerts := map[string]struct {
			value     float64
			threshold float64
			op        string
		}{
			"成功率":    {m.TaskSuccessRate * 100, 70, "<"},
			"每任务成本": {m.CostPerTask, 0.10, ">"},
			"平均质量":  {m.AvgOutputQuality, 6.0, "<"},
		}
		for name, a := range alerts {
			status := "OK"
			if (a.op == "<" && a.value < a.threshold) || (a.op == ">" && a.value > a.threshold) {
				status = "ALERT"
			}
			fmt.Printf("  [%s] %s: %.2f (阈值 %s %.2f)\n", status, name, a.value, a.op, a.threshold)
		}
	}

	_ = prodConfig
	_ = reflectionConfig
}
