package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/agent/skills"
	"go.uber.org/zap"
)

// EnhancedExecutionOptions 增强执行选项
type EnhancedExecutionOptions struct {
	// Reflection 选项
	UseReflection    bool
	ReflectionConfig interface{} // ReflectionConfig

	// 工具选择选项
	UseToolSelection    bool
	ToolSelectionConfig interface{} // ToolSelectionConfig

	// 提示词增强选项
	UsePromptEnhancer       bool
	PromptEngineeringConfig interface{} // PromptEngineeringConfig

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
func (b *BaseAgent) EnableReflection(executor interface{}) {
	b.reflectionExecutor = executor
	b.logger.Info("reflection enabled")
}

// EnableToolSelection 启用动态工具选择
func (b *BaseAgent) EnableToolSelection(selector interface{}) {
	b.toolSelector = selector
	b.logger.Info("tool selection enabled")
}

// EnablePromptEnhancer 启用提示词增强
func (b *BaseAgent) EnablePromptEnhancer(enhancer interface{}) {
	b.promptEnhancer = enhancer
	b.logger.Info("prompt enhancer enabled")
}

// EnableSkills 启用 Skills 系统
func (b *BaseAgent) EnableSkills(manager interface{}) {
	b.skillManager = manager
	b.logger.Info("skills system enabled")
}

// EnableMCP 启用 MCP 集成
func (b *BaseAgent) EnableMCP(server interface{}) {
	b.mcpServer = server
	b.logger.Info("MCP integration enabled")
}

// EnableEnhancedMemory 启用增强记忆系统
func (b *BaseAgent) EnableEnhancedMemory(memorySystem interface{}) {
	b.enhancedMemory = memorySystem
	b.logger.Info("enhanced memory enabled")
}

// EnableObservability 启用可观测性系统
func (b *BaseAgent) EnableObservability(obsSystem interface{}) {
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
		// 类型断言并调用
		if obs, ok := b.observabilitySystem.(interface {
			StartTrace(traceID, agentID string)
		}); ok {
			obs.StartTrace(traceID, b.ID())
		}
	}

	// 2. Skills：发现并加载技能
	var skillInstructions []string
	if options.UseSkills && b.skillManager != nil {
		query := options.SkillsQuery
		if query == "" {
			query = input.Content
		}
		b.logger.Debug("discovering skills", zap.String("query", query))

		// Prefer the built-in skills manager signature.
		if sm, ok := b.skillManager.(interface {
			DiscoverSkills(ctx context.Context, task string) ([]*skills.Skill, error)
		}); ok {
			found, err := sm.DiscoverSkills(ctx, query)
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
		} else if sm, ok := b.skillManager.(interface {
			DiscoverSkills(ctx context.Context, task string) (interface{}, error)
		}); ok {
			// Backwards-compatible fallback for custom implementations.
			raw, err := sm.DiscoverSkills(ctx, query)
			if err != nil {
				b.logger.Warn("skill discovery failed", zap.Error(err))
			} else if list, ok := raw.([]interface{}); ok {
				for _, s := range list {
					if skill, ok := s.(interface{ GetInstructions() string }); ok {
						skillInstructions = append(skillInstructions, skill.GetInstructions())
					}
				}
				b.logger.Info("skills discovered", zap.Int("count", len(skillInstructions)))
			}
		}
	}

	// 3. 增强记忆：加载上下文
	var memoryContext []string
	if options.UseEnhancedMemory && b.enhancedMemory != nil {
		if options.LoadWorkingMemory {
			b.logger.Debug("loading working memory")
			if mem, ok := b.enhancedMemory.(interface {
				LoadWorking(ctx context.Context, agentID string) ([]interface{}, error)
			}); ok {
				working, err := mem.LoadWorking(ctx, b.ID())
				if err != nil {
					b.logger.Warn("failed to load working memory", zap.Error(err))
				} else {
					for _, w := range working {
						if wm, ok := w.(map[string]interface{}); ok {
							if content, ok := wm["content"].(string); ok {
								memoryContext = append(memoryContext, content)
							}
						}
					}
					b.logger.Info("working memory loaded", zap.Int("count", len(working)))
				}
			}
		}
		if options.LoadShortTermMemory {
			b.logger.Debug("loading short-term memory")
			if mem, ok := b.enhancedMemory.(interface {
				LoadShortTerm(ctx context.Context, agentID string, limit int) ([]interface{}, error)
			}); ok {
				shortTerm, err := mem.LoadShortTerm(ctx, b.ID(), 5)
				if err != nil {
					b.logger.Warn("failed to load short-term memory", zap.Error(err))
				} else {
					for _, st := range shortTerm {
						if stm, ok := st.(map[string]interface{}); ok {
							if content, ok := stm["content"].(string); ok {
								memoryContext = append(memoryContext, content)
							}
						}
					}
					b.logger.Info("short-term memory loaded", zap.Int("count", len(shortTerm)))
				}
			}
		}
	}

	// 4. 提示词增强
	enhancedPrompt := input.Content
	if options.UsePromptEnhancer && b.promptEnhancer != nil {
		b.logger.Debug("enhancing prompt")
		if pe, ok := b.promptEnhancer.(interface {
			EnhanceUserPrompt(prompt, context string) (string, error)
		}); ok {
			// 构建上下文
			contextStr := ""
			if len(skillInstructions) > 0 {
				contextStr += "Skills: " + fmt.Sprintf("%v", skillInstructions) + "\n"
			}
			if len(memoryContext) > 0 {
				contextStr += "Memory: " + fmt.Sprintf("%v", memoryContext) + "\n"
			}

			enhanced, err := pe.EnhanceUserPrompt(input.Content, contextStr)
			if err != nil {
				b.logger.Warn("prompt enhancement failed", zap.Error(err))
			} else {
				enhancedPrompt = enhanced
				b.logger.Info("prompt enhanced")
			}
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
		if ts, ok := b.toolSelector.(interface {
			SelectTools(ctx context.Context, task string, availableTools interface{}) (interface{}, error)
		}); ok {
			// 获取可用工具
			availableTools := b.toolManager.GetAllowedTools(b.ID())
			selected, err := ts.SelectTools(ctx, enhancedPrompt, availableTools)
			if err != nil {
				b.logger.Warn("tool selection failed", zap.Error(err))
			} else {
				b.logger.Info("tools selected dynamically", zap.Any("selected", selected))
				// 这里可以更新 Agent 的工具列表
			}
		}
	}

	// 6. 执行任务
	var output *Output
	var err error

	if options.UseReflection && b.reflectionExecutor != nil {
		// 使用 Reflection 执行
		b.logger.Debug("executing with reflection")
		if re, ok := b.reflectionExecutor.(interface {
			ExecuteWithReflection(ctx context.Context, input *Input) (interface{}, error)
		}); ok {
			result, execErr := re.ExecuteWithReflection(ctx, enhancedInput)
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
			if obs, ok := b.observabilitySystem.(interface {
				EndTrace(traceID, status string, err error)
			}); ok {
				obs.EndTrace(traceID, "failed", err)
			}
		}
		return nil, err
	}

	// 7. 保存到增强记忆
	if options.UseEnhancedMemory && b.enhancedMemory != nil && options.SaveToMemory {
		b.logger.Debug("saving to enhanced memory")

		// 保存短期记忆
		if mem, ok := b.enhancedMemory.(interface {
			SaveShortTerm(ctx context.Context, agentID, content string, metadata map[string]interface{}) error
		}); ok {
			metadata := map[string]interface{}{
				"trace_id": input.TraceID,
				"tokens":   output.TokensUsed,
				"cost":     output.Cost,
			}
			if err := mem.SaveShortTerm(ctx, b.ID(), output.Content, metadata); err != nil {
				b.logger.Warn("failed to save short-term memory", zap.Error(err))
			}
		}

		// 记录情节
		if mem, ok := b.enhancedMemory.(interface {
			RecordEpisode(ctx context.Context, event interface{}) error
		}); ok {
			// 创建情节事件（需要导入 memory 包的类型）
			event := map[string]interface{}{
				"id":        fmt.Sprintf("%s-%d", b.ID(), time.Now().UnixNano()),
				"agent_id":  b.ID(),
				"type":      "task_execution",
				"content":   output.Content,
				"timestamp": time.Now(),
				"duration":  output.Duration,
				"context": map[string]interface{}{
					"trace_id":   input.TraceID,
					"tokens":     output.TokensUsed,
					"cost":       output.Cost,
					"reflection": options.UseReflection,
				},
			}
			if err := mem.RecordEpisode(ctx, event); err != nil {
				b.logger.Warn("failed to record episode", zap.Error(err))
			}
		}
	}

	// 8. 可观测性：记录指标
	if options.UseObservability && b.observabilitySystem != nil {
		duration := time.Since(startTime)
		if options.RecordMetrics {
			b.logger.Debug("recording metrics")
			if obs, ok := b.observabilitySystem.(interface {
				RecordTask(agentID string, success bool, duration time.Duration, tokens int, cost, quality float64)
			}); ok {
				obs.RecordTask(b.ID(), true, duration, output.TokensUsed, output.Cost, 0.8)
			}
		}
		if options.RecordTrace {
			if obs, ok := b.observabilitySystem.(interface {
				EndTrace(traceID, status string, err error)
			}); ok {
				obs.EndTrace(traceID, "completed", nil)
			}
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
	EnableEnhancedMemory bool
	EnableObservability  bool

	// 配置
	ReflectionMaxIterations int
	ToolSelectionMaxTools   int
	SkillsDirectory         string
	MCPServerName           string
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
		EnableEnhancedMemory:    true,
		EnableObservability:     true,
		ReflectionMaxIterations: 3,
		ToolSelectionMaxTools:   5,
		SkillsDirectory:         "./skills",
		MCPServerName:           "agent-mcp-server",
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
func (b *BaseAgent) GetFeatureMetrics() map[string]interface{} {
	status := b.GetFeatureStatus()

	metrics := map[string]interface{}{
		"agent_id":   b.ID(),
		"agent_name": b.Name(),
		"agent_type": string(b.Type()),
		"features":   status,
		"config": map[string]interface{}{
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

// ExportConfiguration 导出配置（用于持久化或分享）
func (b *BaseAgent) ExportConfiguration() map[string]interface{} {
	return map[string]interface{}{
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
			"enhanced_memory": b.config.EnableEnhancedMemory,
			"observability":   b.config.EnableObservability,
		},
		"tools":    b.config.Tools,
		"metadata": b.config.Metadata,
	}
}
