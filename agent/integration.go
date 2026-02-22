package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/agent/memory"
	"github.com/BaSui01/agentflow/llm"
	"go.uber.org/zap"
)

// EnhancedExecutionOptions 增强执行选项
type EnhancedExecutionOptions struct {
	// Reflection 选项
	UseReflection bool

	// 工具选择选项
	UseToolSelection bool

	// 提示词增强选项
	UsePromptEnhancer bool

	// Skills 选项
	UseSkills   bool
	SkillsQuery string

	// 记忆选项
	UseEnhancedMemory   bool
	LoadWorkingMemory   bool
	LoadShortTermMemory bool
	SaveToMemory        bool

	// 可观测性选项
	UseObservability bool
	RecordMetrics    bool
	RecordTrace      bool
}

// DefaultEnhancedExecutionOptions 默认增强执行选项
func DefaultEnhancedExecutionOptions() EnhancedExecutionOptions {
	return EnhancedExecutionOptions{
		UseReflection:       false,
		UseToolSelection:    false,
		UsePromptEnhancer:   false,
		UseSkills:           false,
		UseEnhancedMemory:   false,
		LoadWorkingMemory:   true,
		LoadShortTermMemory: true,
		SaveToMemory:        true,
		UseObservability:    true,
		RecordMetrics:       true,
		RecordTrace:         true,
	}
}

// EnableReflection 启用 Reflection 机制
func (b *BaseAgent) EnableReflection(executor ReflectionRunner) {
	b.reflectionExecutor = executor
	b.logger.Info("reflection enabled")
}

// EnableToolSelection 启用动态工具选择
func (b *BaseAgent) EnableToolSelection(selector DynamicToolSelectorRunner) {
	b.toolSelector = selector
	b.logger.Info("tool selection enabled")
}

// EnablePromptEnhancer 启用提示词增强
func (b *BaseAgent) EnablePromptEnhancer(enhancer PromptEnhancerRunner) {
	b.promptEnhancer = enhancer
	b.logger.Info("prompt enhancer enabled")
}

// EnableSkills 启用 Skills 系统
func (b *BaseAgent) EnableSkills(manager SkillDiscoverer) {
	b.skillManager = manager
	b.logger.Info("skills system enabled")
}

// EnableMCP 启用 MCP 集成
func (b *BaseAgent) EnableMCP(server MCPServerRunner) {
	b.mcpServer = server
	b.logger.Info("MCP integration enabled")
}

// EnableLSP 启用 LSP 集成。
func (b *BaseAgent) EnableLSP(client LSPClientRunner) {
	b.lspClient = client
	b.logger.Info("LSP integration enabled")
}

// EnableLSPWithLifecycle 启用 LSP，并注册可选生命周期对象（例如 *ManagedLSP）。
func (b *BaseAgent) EnableLSPWithLifecycle(client LSPClientRunner, lifecycle LSPLifecycleOwner) {
	b.lspClient = client
	b.lspLifecycle = lifecycle
	b.logger.Info("LSP integration enabled with lifecycle")
}

// EnableEnhancedMemory 启用增强记忆系统
func (b *BaseAgent) EnableEnhancedMemory(memorySystem EnhancedMemoryRunner) {
	b.enhancedMemory = memorySystem
	b.logger.Info("enhanced memory enabled")
}

// EnableObservability 启用可观测性系统
func (b *BaseAgent) EnableObservability(obsSystem ObservabilityRunner) {
	b.observabilitySystem = obsSystem
	b.logger.Info("observability enabled")
}

// ExecuteEnhanced 增强执行（集成所有功能）
func (b *BaseAgent) ExecuteEnhanced(ctx context.Context, input *Input, options EnhancedExecutionOptions) (*Output, error) {
	startTime := time.Now()

	b.logger.Info("starting enhanced execution",
		zap.String("trace_id", input.TraceID),
		zap.Bool("reflection", options.UseReflection),
		zap.Bool("tool_selection", options.UseToolSelection),
		zap.Bool("prompt_enhancer", options.UsePromptEnhancer),
		zap.Bool("skills", options.UseSkills),
		zap.Bool("enhanced_memory", options.UseEnhancedMemory),
		zap.Bool("observability", options.UseObservability),
	)

	// 1. 可观测性：开始追踪
	var traceID string
	if options.UseObservability && b.observabilitySystem != nil {
		traceID = input.TraceID
		b.logger.Debug("trace started", zap.String("trace_id", traceID))
		b.observabilitySystem.StartTrace(traceID, b.ID())
	}

	// 2. Skills：发现并加载技能
	var skillInstructions []string
	if options.UseSkills && b.skillManager != nil {
		query := options.SkillsQuery
		if query == "" {
			query = input.Content
		}
		b.logger.Debug("discovering skills", zap.String("query", query))

		found, err := b.skillManager.DiscoverSkills(ctx, query)
		if err != nil {
			b.logger.Warn("skill discovery failed", zap.Error(err))
		} else {
			for _, s := range found {
				if s == nil {
					continue
				}
				skillInstructions = append(skillInstructions, s.GetInstructions())
			}
			b.logger.Info("skills discovered", zap.Int("count", len(skillInstructions)))
		}
	}

	enhancedPrompt := input.Content
	if len(skillInstructions) > 0 {
		enhancedPrompt = prependSkillInstructions(input.Content, skillInstructions)
	}

	// 3. 增强记忆：加载上下文
	var memoryContext []string
	if options.UseEnhancedMemory && b.enhancedMemory != nil {
		if options.LoadWorkingMemory {
			b.logger.Debug("loading working memory")
			working, err := b.enhancedMemory.LoadWorking(ctx, b.ID())
			if err != nil {
				b.logger.Warn("failed to load working memory", zap.Error(err))
			} else {
				for _, w := range working {
					if wm, ok := w.(map[string]any); ok {
						if content, ok := wm["content"].(string); ok {
							memoryContext = append(memoryContext, content)
						}
					}
				}
				b.logger.Info("working memory loaded", zap.Int("count", len(working)))
			}
		}
		if options.LoadShortTermMemory {
			b.logger.Debug("loading short-term memory")
			shortTerm, err := b.enhancedMemory.LoadShortTerm(ctx, b.ID(), 5)
			if err != nil {
				b.logger.Warn("failed to load short-term memory", zap.Error(err))
			} else {
				for _, st := range shortTerm {
					if stm, ok := st.(map[string]any); ok {
						if content, ok := stm["content"].(string); ok {
							memoryContext = append(memoryContext, content)
						}
					}
				}
				b.logger.Info("short-term memory loaded", zap.Int("count", len(shortTerm)))
			}
		}
	}

	// 4. 提示词增强
	if options.UsePromptEnhancer && b.promptEnhancer != nil {
		b.logger.Debug("enhancing prompt")
		// 构建上下文
		contextStr := ""
		if len(skillInstructions) > 0 {
			contextStr += "Skills: " + fmt.Sprintf("%v", skillInstructions) + "\n"
		}
		if len(memoryContext) > 0 {
			contextStr += "Memory: " + fmt.Sprintf("%v", memoryContext) + "\n"
		}

		enhanced, err := b.promptEnhancer.EnhanceUserPrompt(input.Content, contextStr)
		if err != nil {
			b.logger.Warn("prompt enhancement failed", zap.Error(err))
		} else {
			enhancedPrompt = enhanced
			b.logger.Info("prompt enhanced")
		}
	}

	// 更新输入内容
	enhancedInput := &Input{
		TraceID:   input.TraceID,
		TenantID:  input.TenantID,
		UserID:    input.UserID,
		ChannelID: input.ChannelID,
		Content:   enhancedPrompt,
		Context:   input.Context,
		Variables: input.Variables,
	}

	// 5. 动态工具选择
	if options.UseToolSelection && b.toolSelector != nil && b.toolManager != nil {
		b.logger.Debug("selecting tools dynamically")
		// 获取可用工具
		availableTools := b.toolManager.GetAllowedTools(b.ID())
		selected, err := b.toolSelector.SelectTools(ctx, enhancedPrompt, availableTools)
		if err != nil {
			b.logger.Warn("tool selection failed", zap.Error(err))
		} else {
			b.logger.Info("tools selected dynamically", zap.Any("selected", selected))
			// 这里可以更新 Agent 的工具列表
		}
	}

	// 6. 执行任务
	var output *Output
	var err error

	if options.UseReflection && b.reflectionExecutor != nil {
		// 使用 Reflection 执行
		b.logger.Debug("executing with reflection")
		result, execErr := b.reflectionExecutor.ExecuteWithReflection(ctx, enhancedInput)
		if execErr != nil {
			return nil, fmt.Errorf("reflection execution failed: %w", execErr)
		}

		// 提取最终输出
		if reflectionResult, ok := result.(interface{ GetFinalOutput() *Output }); ok {
			output = reflectionResult.GetFinalOutput()
		} else {
			// 回退到普通执行
			output, err = b.Execute(ctx, enhancedInput)
		}
	} else {
		// 普通执行
		output, err = b.Execute(ctx, enhancedInput)
	}

	if err != nil {
		// 可观测性：记录错误
		if options.UseObservability && b.observabilitySystem != nil {
			b.logger.Error("execution failed", zap.Error(err))
			b.observabilitySystem.EndTrace(traceID, "failed", err)
		}
		return nil, err
	}

	// 7. 保存到增强记忆
	if options.UseEnhancedMemory && b.enhancedMemory != nil && options.SaveToMemory {
		b.logger.Debug("saving to enhanced memory")

		// 保存短期记忆
		metadata := map[string]any{
			"trace_id": input.TraceID,
			"tokens":   output.TokensUsed,
			"cost":     output.Cost,
		}
		if err := b.enhancedMemory.SaveShortTerm(ctx, b.ID(), output.Content, metadata); err != nil {
			b.logger.Warn("failed to save short-term memory", zap.Error(err))
		}

		// 记录情节
		event := &memory.EpisodicEvent{
			ID:        fmt.Sprintf("%s-%d", b.ID(), time.Now().UnixNano()),
			AgentID:   b.ID(),
			Type:      "task_execution",
			Content:   output.Content,
			Timestamp: time.Now(),
			Duration:  output.Duration,
			Context: map[string]any{
				"trace_id":   input.TraceID,
				"tokens":     output.TokensUsed,
				"cost":       output.Cost,
				"reflection": options.UseReflection,
			},
		}
		if err := b.enhancedMemory.RecordEpisode(ctx, event); err != nil {
			b.logger.Warn("failed to record episode", zap.Error(err))
		}
	}

	// 8. 可观测性：记录指标
	if options.UseObservability && b.observabilitySystem != nil {
		duration := time.Since(startTime)
		if options.RecordMetrics {
			b.logger.Debug("recording metrics")
			b.observabilitySystem.RecordTask(b.ID(), true, duration, output.TokensUsed, output.Cost, 0.8)
		}
		if options.RecordTrace {
			b.observabilitySystem.EndTrace(traceID, "completed", nil)
		}
	}

	b.logger.Info("enhanced execution completed",
		zap.String("trace_id", input.TraceID),
		zap.Duration("total_duration", time.Since(startTime)),
		zap.Int("tokens_used", output.TokensUsed),
	)

	return output, nil
}

// GetFeatureStatus 获取功能启用状态
func (b *BaseAgent) GetFeatureStatus() map[string]bool {
	return map[string]bool{
		"reflection":      b.reflectionExecutor != nil,
		"tool_selection":  b.toolSelector != nil,
		"prompt_enhancer": b.promptEnhancer != nil,
		"skills":          b.skillManager != nil,
		"mcp":             b.mcpServer != nil,
		"lsp":             b.lspClient != nil,
		"enhanced_memory": b.enhancedMemory != nil,
		"observability":   b.observabilitySystem != nil,
		"context_manager": b.contextManager != nil,
	}
}

// PrintFeatureStatus 打印功能状态
func (b *BaseAgent) PrintFeatureStatus() {
	status := b.GetFeatureStatus()

	b.logger.Info("Agent Feature Status",
		zap.String("agent_id", b.ID()),
		zap.Bool("reflection", status["reflection"]),
		zap.Bool("tool_selection", status["tool_selection"]),
		zap.Bool("prompt_enhancer", status["prompt_enhancer"]),
		zap.Bool("skills", status["skills"]),
		zap.Bool("mcp", status["mcp"]),
		zap.Bool("lsp", status["lsp"]),
		zap.Bool("enhanced_memory", status["enhanced_memory"]),
		zap.Bool("observability", status["observability"]),
		zap.Bool("context_manager", status["context_manager"]),
	)
}

// QuickSetupOptions 快速设置选项
type QuickSetupOptions struct {
	EnableAllFeatures bool

	// 功能开关
	EnableReflection     bool
	EnableToolSelection  bool
	EnablePromptEnhancer bool
	EnableSkills         bool
	EnableMCP            bool
	EnableLSP            bool
	EnableEnhancedMemory bool
	EnableObservability  bool

	// 配置
	ReflectionMaxIterations int
	ToolSelectionMaxTools   int
	SkillsDirectory         string
	MCPServerName           string
	LSPServerName           string
	LSPServerVersion        string
	MemoryTTL               time.Duration
}

// DefaultQuickSetupOptions 默认快速设置选项
func DefaultQuickSetupOptions() QuickSetupOptions {
	return QuickSetupOptions{
		EnableAllFeatures:       true,
		EnableReflection:        true,
		EnableToolSelection:     true,
		EnablePromptEnhancer:    true,
		EnableSkills:            true,
		EnableMCP:               false, // MCP 需要额外配置
		EnableLSP:               true,
		EnableEnhancedMemory:    true,
		EnableObservability:     true,
		ReflectionMaxIterations: 3,
		ToolSelectionMaxTools:   5,
		SkillsDirectory:         "./skills",
		MCPServerName:           "agent-mcp-server",
		LSPServerName:           defaultLSPServerName,
		LSPServerVersion:        defaultLSPServerVersion,
		MemoryTTL:               24 * time.Hour,
	}
}

// QuickSetup 快速设置（启用推荐功能）
// 注意：这个方法需要在实际项目中根据具体的类型进行实现
// 这里提供一个框架示例
func (b *BaseAgent) QuickSetup(ctx context.Context, options QuickSetupOptions) error {
	b.logger.Info("quick setup: enabling features",
		zap.Bool("all_features", options.EnableAllFeatures),
	)

	// 由于避免循环依赖，这里只能提供接口
	// 实际实现需要在调用方创建具体的实例并调用 Enable* 方法

	if options.EnableAllFeatures || options.EnableReflection {
		b.logger.Info("reflection should be enabled with max_iterations",
			zap.Int("max_iterations", options.ReflectionMaxIterations))
	}

	if options.EnableAllFeatures || options.EnableToolSelection {
		b.logger.Info("tool selection should be enabled with max_tools",
			zap.Int("max_tools", options.ToolSelectionMaxTools))
	}

	if options.EnableAllFeatures || options.EnablePromptEnhancer {
		b.logger.Info("prompt enhancer should be enabled")
	}

	if options.EnableAllFeatures || options.EnableSkills {
		b.logger.Info("skills should be enabled with directory",
			zap.String("directory", options.SkillsDirectory))
	}

	if options.EnableMCP {
		b.logger.Info("MCP should be enabled with server name",
			zap.String("server_name", options.MCPServerName))
	}

	if options.EnableAllFeatures || options.EnableLSP {
		b.logger.Info("LSP should be enabled with server info",
			zap.String("server_name", options.LSPServerName),
			zap.String("server_version", options.LSPServerVersion))
	}

	if options.EnableAllFeatures || options.EnableEnhancedMemory {
		b.logger.Info("enhanced memory should be enabled with TTL",
			zap.Duration("ttl", options.MemoryTTL))
	}

	if options.EnableAllFeatures || options.EnableObservability {
		b.logger.Info("observability should be enabled")
	}

	b.logger.Info("quick setup completed - features configured")
	return nil
}

// ValidateConfiguration 验证配置
func (b *BaseAgent) ValidateConfiguration() error {
	errors := []string{}

	// 检查必需组件
	if b.provider == nil {
		errors = append(errors, "provider not set")
	}

	// 检查功能依赖
	if b.config.EnableReflection && b.reflectionExecutor == nil {
		errors = append(errors, "reflection enabled but executor not set")
	}

	if b.config.EnableToolSelection && b.toolSelector == nil {
		errors = append(errors, "tool selection enabled but selector not set")
	}

	if b.config.EnablePromptEnhancer && b.promptEnhancer == nil {
		errors = append(errors, "prompt enhancer enabled but enhancer not set")
	}

	if b.config.EnableSkills && b.skillManager == nil {
		errors = append(errors, "skills enabled but manager not set")
	}

	if b.config.EnableMCP && b.mcpServer == nil {
		errors = append(errors, "MCP enabled but server not set")
	}

	if b.config.EnableLSP && b.lspClient == nil {
		errors = append(errors, "LSP enabled but client not set")
	}

	if b.config.EnableEnhancedMemory && b.enhancedMemory == nil {
		errors = append(errors, "enhanced memory enabled but system not set")
	}

	if b.config.EnableObservability && b.observabilitySystem == nil {
		errors = append(errors, "observability enabled but system not set")
	}

	if len(errors) > 0 {
		return fmt.Errorf("configuration validation failed: %v", errors)
	}

	b.logger.Info("configuration validated successfully")
	return nil
}

// GetFeatureMetrics 获取功能使用指标
func (b *BaseAgent) GetFeatureMetrics() map[string]any {
	status := b.GetFeatureStatus()

	metrics := map[string]any{
		"agent_id":   b.ID(),
		"agent_name": b.Name(),
		"agent_type": string(b.Type()),
		"features":   status,
		"config": map[string]any{
			"model":       b.config.Model,
			"provider":    b.config.Provider,
			"max_tokens":  b.config.MaxTokens,
			"temperature": b.config.Temperature,
		},
	}

	// 添加功能计数
	enabledCount := 0
	for _, enabled := range status {
		if enabled {
			enabledCount++
		}
	}
	metrics["enabled_features_count"] = enabledCount
	metrics["total_features_count"] = len(status)

	return metrics
}

func prependSkillInstructions(prompt string, instructions []string) string {
	if len(instructions) == 0 {
		return prompt
	}

	unique := make(map[string]struct{}, len(instructions))
	cleaned := make([]string, 0, len(instructions))
	for _, instruction := range instructions {
		instruction = strings.TrimSpace(instruction)
		if instruction == "" {
			continue
		}
		if _, exists := unique[instruction]; exists {
			continue
		}
		unique[instruction] = struct{}{}
		cleaned = append(cleaned, instruction)
	}

	if len(cleaned) == 0 {
		return prompt
	}

	var sb strings.Builder
	sb.WriteString("技能执行指令:\n")
	for idx, instruction := range cleaned {
		sb.WriteString(fmt.Sprintf("%d. %s\n", idx+1, instruction))
	}
	sb.WriteString("\n用户请求:\n")
	sb.WriteString(prompt)
	return sb.String()
}

// ExportConfiguration 导出配置（用于持久化或分享）
func (b *BaseAgent) ExportConfiguration() map[string]any {
	return map[string]any{
		"id":          b.config.ID,
		"name":        b.config.Name,
		"type":        string(b.config.Type),
		"description": b.config.Description,
		"model":       b.config.Model,
		"provider":    b.config.Provider,
		"features": map[string]bool{
			"reflection":      b.config.EnableReflection,
			"tool_selection":  b.config.EnableToolSelection,
			"prompt_enhancer": b.config.EnablePromptEnhancer,
			"skills":          b.config.EnableSkills,
			"mcp":             b.config.EnableMCP,
			"lsp":             b.config.EnableLSP,
			"enhanced_memory": b.config.EnableEnhancedMemory,
			"observability":   b.config.EnableObservability,
		},
		"tools":    b.config.Tools,
		"metadata": b.config.Metadata,
	}
}

// =============================================================================
// Adapters: wrap concrete types whose method signatures differ from the
// workflow-local interfaces (e.g. *ReflectionExecutor returns *ReflectionResult
// instead of any). Use these when passing concrete agent types to Enable*.
// =============================================================================

// reflectionRunnerAdapter wraps *ReflectionExecutor to satisfy ReflectionRunner.
type reflectionRunnerAdapter struct {
	executor *ReflectionExecutor
}

func (a *reflectionRunnerAdapter) ExecuteWithReflection(ctx context.Context, input *Input) (any, error) {
	return a.executor.ExecuteWithReflection(ctx, input)
}

// AsReflectionRunner wraps a *ReflectionExecutor as a ReflectionRunner.
func AsReflectionRunner(executor *ReflectionExecutor) ReflectionRunner {
	return &reflectionRunnerAdapter{executor: executor}
}

// toolSelectorRunnerAdapter wraps *DynamicToolSelector to satisfy DynamicToolSelectorRunner.
type toolSelectorRunnerAdapter struct {
	selector *DynamicToolSelector
}

func (a *toolSelectorRunnerAdapter) SelectTools(ctx context.Context, task string, availableTools any) (any, error) {
	tools, ok := availableTools.([]llm.ToolSchema)
	if !ok {
		return nil, fmt.Errorf("availableTools: expected []llm.ToolSchema, got %T", availableTools)
	}
	return a.selector.SelectTools(ctx, task, tools)
}

// AsToolSelectorRunner wraps a *DynamicToolSelector as a DynamicToolSelectorRunner.
func AsToolSelectorRunner(selector *DynamicToolSelector) DynamicToolSelectorRunner {
	return &toolSelectorRunnerAdapter{selector: selector}
}

// promptEnhancerRunnerAdapter wraps *PromptEnhancer to satisfy PromptEnhancerRunner.
type promptEnhancerRunnerAdapter struct {
	enhancer *PromptEnhancer
}

func (a *promptEnhancerRunnerAdapter) EnhanceUserPrompt(prompt, context string) (string, error) {
	return a.enhancer.EnhanceUserPrompt(prompt, context), nil
}

// AsPromptEnhancerRunner wraps a *PromptEnhancer as a PromptEnhancerRunner.
func AsPromptEnhancerRunner(enhancer *PromptEnhancer) PromptEnhancerRunner {
	return &promptEnhancerRunnerAdapter{enhancer: enhancer}
}
