package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/BaSui01/agentflow/agent/mcp"
	"github.com/BaSui01/agentflow/agent/memory"
	"github.com/BaSui01/agentflow/agent/skills"
	"go.uber.org/zap"
)

// 演示中优先级功能：Skills 系统、MCP 集成、记忆系统升级

func main() {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	fmt.Println("=== 中优先级功能演示 ===")

	// 示例 1: Skills 系统
	fmt.Println("=== 示例 1: Skills 系统 ===")
	demoSkillsSystem(logger)

	fmt.Println("\n=== 示例 2: MCP 集成 ===")
	demoMCPIntegration(logger)

	fmt.Println("\n=== 示例 3: 增强记忆系统 ===")
	demoEnhancedMemory(logger)
}

func demoSkillsSystem(logger *zap.Logger) {
	ctx := context.Background()

	// 1. 创建技能管理器
	config := skills.DefaultSkillManagerConfig()
	manager := skills.NewSkillManager(config, logger)

	// 2. 使用构建器创建技能
	fmt.Println("1. 创建技能")

	codeReviewSkill, _ := skills.NewSkillBuilder("code-review", "代码审查").
		WithDescription("专业的代码审查技能，检查代码质量、安全性和最佳实践").
		WithCategory("development").
		WithTags("code", "review", "quality").
		WithInstructions(`你是一个专业的代码审查专家。请从以下角度审查代码：
1. 代码质量：可读性、可维护性
2. 安全性：潜在的安全漏洞
3. 性能：性能优化建议
4. 最佳实践：是否遵循语言最佳实践`).
		WithTools("static_analyzer", "security_scanner").
		WithExample(
			"审查这段 Go 代码",
			"发现 3 个问题：1. 缺少错误处理 2. 变量命名不规范 3. 可以优化性能",
			"使用静态分析工具检查代码",
		).
		WithPriority(10).
		WithLazyLoad(false).
		Build()

	dataAnalysisSkill, _ := skills.NewSkillBuilder("data-analysis", "数据分析").
		WithDescription("数据分析和可视化技能").
		WithCategory("analytics").
		WithTags("data", "analysis", "visualization").
		WithInstructions("分析数据并生成可视化报告").
		WithTools("pandas", "matplotlib", "numpy").
		WithPriority(8).
		Build()

	// 3. 注册技能
	fmt.Println("2. 注册技能")
	manager.RegisterSkill(codeReviewSkill)
	manager.RegisterSkill(dataAnalysisSkill)

	// 4. 列出所有技能
	fmt.Println("\n3. 列出所有技能")
	allSkills := manager.ListSkills()
	for i, skill := range allSkills {
		fmt.Printf("  %d. %s (%s) - %s\n", i+1, skill.Name, skill.Category, skill.Description)
	}

	// 5. 搜索技能
	fmt.Println("\n4. 搜索技能")
	searchResults := manager.SearchSkills("code")
	fmt.Printf("搜索 'code' 找到 %d 个技能:\n", len(searchResults))
	for _, skill := range searchResults {
		fmt.Printf("  - %s\n", skill.Name)
	}

	// 6. 发现适合任务的技能
	fmt.Println("\n5. 发现适合任务的技能")
	task := "我需要审查一段 Python 代码的质量"
	discoveredSkills, _ := manager.DiscoverSkills(ctx, task)
	fmt.Printf("任务: %s\n", task)
	fmt.Printf("发现 %d 个匹配的技能:\n", len(discoveredSkills))
	for i, skill := range discoveredSkills {
		score := skill.MatchesTask(task)
		fmt.Printf("  %d. %s (匹配度: %.2f)\n", i+1, skill.Name, score)
	}

	// 7. 加载技能
	fmt.Println("\n6. 加载和使用技能")
	loadedSkill, _ := manager.LoadSkill(ctx, "code-review")
	fmt.Printf("已加载技能: %s\n", loadedSkill.Name)
	fmt.Printf("需要的工具: %v\n", loadedSkill.Tools)
	fmt.Printf("指令:\n%s\n", loadedSkill.Instructions)

	// 8. 保存技能到文件
	fmt.Println("\n7. 保存技能到文件")
	skillDir := "./skills/code-review"
	if err := skills.SaveSkillToDirectory(codeReviewSkill, skillDir); err != nil {
		log.Printf("保存技能失败: %v", err)
	} else {
		fmt.Printf("技能已保存到: %s\n", skillDir)
	}

	// 9. 从文件加载技能
	fmt.Println("\n8. 从文件加载技能")
	loadedFromFile, err := skills.LoadSkillFromDirectory(skillDir)
	if err != nil {
		log.Printf("加载技能失败: %v", err)
	} else {
		fmt.Printf("从文件加载技能: %s (版本: %s)\n", loadedFromFile.Name, loadedFromFile.Version)
	}

	// 10. 统计信息
	fmt.Println("\n9. 统计信息")
	fmt.Printf("已加载技能数: %d\n", manager.GetLoadedSkillsCount())
	fmt.Printf("索引中技能数: %d\n", manager.GetIndexedSkillsCount())
}

func demoMCPIntegration(logger *zap.Logger) {
	ctx := context.Background()

	// 1. 创建 MCP 服务器
	fmt.Println("1. 创建 MCP 服务器")
	server := mcp.NewMCPServer("demo-server", "1.0.0", logger)

	// 2. 注册资源
	fmt.Println("\n2. 注册资源")
	resource := &mcp.Resource{
		URI:         "file:///data/users.json",
		Name:        "用户数据",
		Description: "系统用户数据",
		Type:        mcp.ResourceTypeFile,
		MimeType:    "application/json",
		Content:     `{"users": [{"id": 1, "name": "Alice"}]}`,
		Metadata: map[string]interface{}{
			"version": "1.0",
		},
		Size:      100,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := server.RegisterResource(resource); err != nil {
		log.Printf("注册资源失败: %v", err)
	} else {
		fmt.Printf("已注册资源: %s\n", resource.Name)
	}

	// 3. 注册工具
	fmt.Println("\n3. 注册工具")
	toolDef := &mcp.ToolDefinition{
		Name:        "calculate",
		Description: "执行数学计算",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"expression": map[string]interface{}{
					"type":        "string",
					"description": "数学表达式",
				},
			},
			"required": []string{"expression"},
		},
	}

	toolHandler := func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		expr := args["expression"].(string)
		// 简化实现：实际应使用表达式解析器
		return fmt.Sprintf("计算结果: %s = 42", expr), nil
	}

	if err := server.RegisterTool(toolDef, toolHandler); err != nil {
		log.Printf("注册工具失败: %v", err)
	} else {
		fmt.Printf("已注册工具: %s\n", toolDef.Name)
	}

	// 4. 注册提示词模板
	fmt.Println("\n4. 注册提示词模板")
	promptTemplate := &mcp.PromptTemplate{
		Name:        "code-review",
		Description: "代码审查提示词",
		Template:    "请审查以下 {{language}} 代码：\n\n{{code}}\n\n重点关注：{{focus}}",
		Variables:   []string{"language", "code", "focus"},
		Examples: []mcp.PromptExample{
			{
				Variables: map[string]string{
					"language": "Go",
					"code":     "func main() { ... }",
					"focus":    "错误处理",
				},
				Output: "审查结果：...",
			},
		},
	}

	if err := server.RegisterPrompt(promptTemplate); err != nil {
		log.Printf("注册提示词失败: %v", err)
	} else {
		fmt.Printf("已注册提示词模板: %s\n", promptTemplate.Name)
	}

	// 5. 列出所有资源
	fmt.Println("\n5. 列出所有资源")
	resources, _ := server.ListResources(ctx)
	fmt.Printf("资源数量: %d\n", len(resources))
	for _, r := range resources {
		fmt.Printf("  - %s (%s)\n", r.Name, r.Type)
	}

	// 6. 列出所有工具
	fmt.Println("\n6. 列出所有工具")
	tools, _ := server.ListTools(ctx)
	fmt.Printf("工具数量: %d\n", len(tools))
	for _, t := range tools {
		fmt.Printf("  - %s: %s\n", t.Name, t.Description)
	}

	// 7. 调用工具
	fmt.Println("\n7. 调用工具")
	result, err := server.CallTool(ctx, "calculate", map[string]interface{}{
		"expression": "2 + 2",
	})
	if err != nil {
		log.Printf("调用工具失败: %v", err)
	} else {
		fmt.Printf("工具调用结果: %v\n", result)
	}

	// 8. 获取提示词
	fmt.Println("\n8. 获取渲染后的提示词")
	prompt, err := server.GetPrompt(ctx, "code-review", map[string]string{
		"language": "Python",
		"code":     "def hello(): print('Hello')",
		"focus":    "代码风格",
	})
	if err != nil {
		log.Printf("获取提示词失败: %v", err)
	} else {
		fmt.Printf("渲染后的提示词:\n%s\n", prompt)
	}

	// 9. 获取服务器信息
	fmt.Println("\n9. 服务器信息")
	info := server.GetServerInfo()
	fmt.Printf("服务器名称: %s\n", info.Name)
	fmt.Printf("版本: %s\n", info.Version)
	fmt.Printf("协议版本: %s\n", info.ProtocolVersion)
	fmt.Printf("能力: 资源=%v, 工具=%v, 提示词=%v\n",
		info.Capabilities.Resources,
		info.Capabilities.Tools,
		info.Capabilities.Prompts,
	)

	// 10. 订阅资源更新
	fmt.Println("\n10. 订阅资源更新")
	updateCh, _ := server.SubscribeResource(ctx, resource.URI)
	fmt.Printf("已订阅资源: %s\n", resource.URI)

	// 模拟资源更新
	go func() {
		time.Sleep(1 * time.Second)
		resource.Content = `{"users": [{"id": 1, "name": "Alice"}, {"id": 2, "name": "Bob"}]}`
		resource.UpdatedAt = time.Now()
		server.UpdateResource(resource)
	}()

	// 接收更新
	select {
	case updated := <-updateCh:
		fmt.Printf("收到资源更新: %s (更新时间: %v)\n", updated.Name, updated.UpdatedAt)
	case <-time.After(2 * time.Second):
		fmt.Println("未收到更新")
	}
}

func demoEnhancedMemory(logger *zap.Logger) {
	ctx := context.Background()

	// 1. 创建增强记忆系统
	fmt.Println("1. 创建增强记忆系统")
	config := memory.DefaultEnhancedMemoryConfig()

	// 注意：实际使用时需要提供真实的存储实现
	// 这里使用 nil 仅作演示
	var shortTerm memory.MemoryStore = nil
	var working memory.MemoryStore = nil
	var longTerm memory.VectorStore = nil
	var episodic memory.EpisodicStore = nil
	var semantic memory.KnowledgeGraph = nil

	memSystem := memory.NewEnhancedMemorySystem(
		shortTerm,
		working,
		longTerm,
		episodic,
		semantic,
		config,
		logger,
	)

	fmt.Println("增强记忆系统已创建")
	fmt.Printf("配置:\n")
	fmt.Printf("  - 短期记忆 TTL: %v\n", config.ShortTermTTL)
	fmt.Printf("  - 工作记忆容量: %d\n", config.WorkingMemorySize)
	fmt.Printf("  - 长期记忆: %v\n", config.LongTermEnabled)
	fmt.Printf("  - 情节记忆: %v\n", config.EpisodicEnabled)
	fmt.Printf("  - 语义记忆: %v\n", config.SemanticEnabled)
	fmt.Printf("  - 记忆整合: %v\n", config.ConsolidationEnabled)

	// 2. 演示记忆层次
	fmt.Println("\n2. 记忆层次结构")
	fmt.Println("  ┌─────────────────┐")
	fmt.Println("  │  短期记忆 (STM)  │ ← 最近的交互（24小时）")
	fmt.Println("  └────────┬────────┘")
	fmt.Println("           │")
	fmt.Println("  ┌────────▼────────┐")
	fmt.Println("  │  工作记忆 (WM)   │ ← 当前任务上下文（20条）")
	fmt.Println("  └────────┬────────┘")
	fmt.Println("           │ 整合")
	fmt.Println("  ┌────────▼────────┐")
	fmt.Println("  │  长期记忆 (LTM)  │ ← 重要信息（向量化）")
	fmt.Println("  └─────────────────┘")
	fmt.Println("           │")
	fmt.Println("  ┌────────▼────────┐")
	fmt.Println("  │  情节记忆 (EM)   │ ← 时间序列事件")
	fmt.Println("  └─────────────────┘")
	fmt.Println("           │")
	fmt.Println("  ┌────────▼────────┐")
	fmt.Println("  │  语义记忆 (SM)   │ ← 知识图谱")
	fmt.Println("  └─────────────────┘")

	// 3. 演示记忆操作（模拟）
	fmt.Println("\n3. 记忆操作示例")

	// 短期记忆
	fmt.Println("\n  a) 短期记忆操作")
	fmt.Println("     - 保存: 用户询问 'AI 是什么？'")
	fmt.Println("     - 保存: Agent 回答 'AI 是人工智能...'")
	fmt.Println("     - TTL: 24 小时后自动过期")

	// 工作记忆
	fmt.Println("\n  b) 工作记忆操作")
	fmt.Println("     - 保存: 当前对话上下文")
	fmt.Println("     - 容量: 最多 20 条记录")
	fmt.Println("     - 清除: 任务完成后清空")

	// 长期记忆
	fmt.Println("\n  c) 长期记忆操作")
	fmt.Println("     - 向量化: 将重要信息转换为向量")
	fmt.Println("     - 存储: 保存到向量数据库")
	fmt.Println("     - 搜索: 语义相似度搜索")

	// 情节记忆
	fmt.Println("\n  d) 情节记忆操作")
	event := &memory.EpisodicEvent{
		ID:        "event-001",
		AgentID:   "agent-001",
		Type:      "task_completed",
		Content:   "完成代码审查任务",
		Timestamp: time.Now(),
		Duration:  5 * time.Minute,
	}
	fmt.Printf("     - 记录事件: %s\n", event.Content)
	fmt.Printf("     - 时间: %v\n", event.Timestamp)
	fmt.Printf("     - 持续时间: %v\n", event.Duration)

	// 语义记忆
	fmt.Println("\n  e) 语义记忆操作")
	entity := &memory.Entity{
		ID:   "entity-001",
		Type: "concept",
		Name: "人工智能",
		Properties: map[string]interface{}{
			"definition": "模拟人类智能的计算机系统",
			"category":   "technology",
		},
	}
	fmt.Printf("     - 添加实体: %s (%s)\n", entity.Name, entity.Type)
	fmt.Printf("     - 属性: %v\n", entity.Properties)

	relation := &memory.Relation{
		ID:     "rel-001",
		FromID: "entity-001",
		ToID:   "entity-002",
		Type:   "related_to",
		Weight: 0.8,
	}
	fmt.Printf("     - 添加关系: %s -> %s (权重: %.2f)\n",
		relation.FromID, relation.ToID, relation.Weight)

	// 4. 记忆整合
	fmt.Println("\n4. 记忆整合机制")
	fmt.Println("  整合流程:")
	fmt.Println("  1. 定期扫描短期记忆")
	fmt.Println("  2. 识别重要信息（基于访问频率、重要性评分）")
	fmt.Println("  3. 转移到长期记忆（向量化存储）")
	fmt.Println("  4. 提取知识并更新语义记忆")
	fmt.Println("  5. 清理过期的短期记忆")
	fmt.Printf("  整合间隔: %v\n", config.ConsolidationInterval)

	// 5. 记忆检索策略
	fmt.Println("\n5. 记忆检索策略")
	fmt.Println("  a) 短期记忆: 时间顺序检索（最近的优先）")
	fmt.Println("  b) 工作记忆: 全部加载到上下文")
	fmt.Println("  c) 长期记忆: 语义相似度搜索（Top-K）")
	fmt.Println("  d) 情节记忆: 时间范围查询")
	fmt.Println("  e) 语义记忆: 图遍历查询")

	// 6. 性能优化
	fmt.Println("\n6. 性能优化")
	fmt.Println("  - 短期记忆: Redis 缓存（毫秒级）")
	fmt.Println("  - 工作记忆: 内存存储（微秒级）")
	fmt.Println("  - 长期记忆: 向量索引（HNSW/IVF）")
	fmt.Println("  - 情节记忆: 时序数据库（InfluxDB/TimescaleDB）")
	fmt.Println("  - 语义记忆: 图数据库（Neo4j/ArangoDB）")

	// 7. 使用场景
	fmt.Println("\n7. 使用场景")
	fmt.Println("  - 对话系统: 维护多轮对话上下文")
	fmt.Println("  - 个性化: 基于历史交互提供个性化服务")
	fmt.Println("  - 知识管理: 构建和查询知识图谱")
	fmt.Println("  - 任务规划: 基于历史经验优化任务执行")
	fmt.Println("  - 学习改进: 从过去的错误中学习")

	_ = memSystem // 避免未使用变量警告
	_ = ctx
}
