package runtime

import (
	"context"
	"strings"
	reasoning "github.com/BaSui01/agentflow/agent/capabilities/reasoning"
	agentfeatures "github.com/BaSui01/agentflow/agent/integration"
	llmtools "github.com/BaSui01/agentflow/llm/capabilities/tools"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	types "github.com/BaSui01/agentflow/types"
	zap "go.uber.org/zap"
)

// EnableReflection 启用 Reflection 机制。
func (b *BaseAgent) EnableReflection(executor ReflectionRunner) {
	b.extensions.EnableReflection(executor)
}

// EnableToolSelection 启用动态工具选择。
func (b *BaseAgent) EnableToolSelection(selector DynamicToolSelectorRunner) {
	b.extensions.EnableToolSelection(selector)
}

// EnablePromptEnhancer 启用提示词增强。
func (b *BaseAgent) EnablePromptEnhancer(enhancer PromptEnhancerRunner) {
	b.extensions.EnablePromptEnhancer(enhancer)
}

// EnableSkills 启用 Skills 系统。
func (b *BaseAgent) EnableSkills(manager SkillDiscoverer) {
	b.extensions.EnableSkills(manager)
}

// EnableMCP 启用 MCP 集成。
func (b *BaseAgent) EnableMCP(server MCPServerRunner) {
	b.extensions.EnableMCP(server)
}

// EnableLSP 启用 LSP 集成。
func (b *BaseAgent) EnableLSP(client LSPClientRunner) {
	b.extensions.EnableLSP(client)
}

// EnableLSPWithLifecycle 启用 LSP，并注册可选生命周期对象（例如 *ManagedLSP）。
func (b *BaseAgent) EnableLSPWithLifecycle(client LSPClientRunner, lifecycle LSPLifecycleOwner) {
	b.extensions.EnableLSPWithLifecycle(client, lifecycle)
}

// EnableEnhancedMemory 启用增强记忆系统。
func (b *BaseAgent) EnableEnhancedMemory(memorySystem EnhancedMemoryRunner) {
	b.extensions.EnableEnhancedMemory(memorySystem)
	b.memoryFacade = NewUnifiedMemoryFacade(b.memory, memorySystem, b.logger)
}

// EnableObservability 启用可观测性系统。
func (b *BaseAgent) EnableObservability(obsSystem ObservabilityRunner) {
	b.extensions.EnableObservability(obsSystem)
}
// ExecuteEnhanced 增强执行（集成所有功能）。
// Uses a middleware pipeline so that each step is an independent, composable unit.
func (b *BaseAgent) ExecuteEnhanced(ctx context.Context, input *Input, options EnhancedExecutionOptions) (*Output, error) {
	return b.executeWithPipeline(ctx, input, options)
}

func (b *BaseAgent) executeWithPipeline(ctx context.Context, input *Input, options EnhancedExecutionOptions) (*Output, error) {
	if input == nil {
		return nil, NewError(types.ErrInputValidation, "input is nil")
	}
	if input.TraceID != "" {
		ctx = types.WithTraceID(ctx, input.TraceID)
	}
	pipeline := NewExecutionPipeline(b.coreExecutor(options))

	if options.UseObservability && b.extensions.ObservabilitySystemExt() != nil {
		pipeline.Use(b.observabilityMiddleware(options))
	}
	if options.UseSkills && b.extensions.SkillManagerExt() != nil {
		pipeline.Use(b.skillsMiddleware(options))
	}
	if options.UseEnhancedMemory && b.extensions.EnhancedMemoryExt() != nil {
		pipeline.Use(b.memoryLoadMiddleware(options))
	}
	if options.UsePromptEnhancer && b.extensions.PromptEnhancerExt() != nil {
		pipeline.Use(b.promptEnhancerMiddleware())
	}
	if options.UseToolSelection && b.extensions.ToolSelector() != nil && b.toolManager != nil {
		pipeline.Use(b.toolSelectionMiddleware())
	}
	if options.UseEnhancedMemory && b.extensions.EnhancedMemoryExt() != nil && options.SaveToMemory {
		pipeline.Use(b.memorySaveMiddleware())
	}

	b.logger.Info("starting enhanced execution",
		zap.String("trace_id", input.TraceID),
		zap.Bool("reflection", options.UseReflection),
		zap.Bool("tool_selection", options.UseToolSelection),
		zap.Bool("prompt_enhancer", options.UsePromptEnhancer),
		zap.Bool("skills", options.UseSkills),
		zap.Bool("enhanced_memory", options.UseEnhancedMemory),
		zap.Bool("observability", options.UseObservability),
	)

	return pipeline.Execute(ctx, input)
}

func (b *BaseAgent) configuredExecutionOptions() EnhancedExecutionOptions {
	options := DefaultEnhancedExecutionOptions()
	options.UseReflection = b.config.IsReflectionEnabled() && b.extensions.ReflectionExecutor() != nil
	options.UseToolSelection = b.config.IsToolSelectionEnabled() && b.extensions.ToolSelector() != nil && b.toolManager != nil
	options.UsePromptEnhancer = b.config.IsPromptEnhancerEnabled() && b.extensions.PromptEnhancerExt() != nil
	options.UseSkills = b.config.IsSkillsEnabled() && b.extensions.SkillManagerExt() != nil
	options.UseEnhancedMemory = b.config.IsMemoryEnabled() && b.extensions.EnhancedMemoryExt() != nil
	if !options.UseEnhancedMemory {
		options.LoadWorkingMemory = false
		options.LoadShortTermMemory = false
		options.SaveToMemory = false
	}

	options.UseObservability = b.config.IsObservabilityEnabled() && b.extensions.ObservabilitySystemExt() != nil
	if obsCfg := b.config.Extensions.Observability; obsCfg != nil {
		options.RecordMetrics = obsCfg.MetricsEnabled
		options.RecordTrace = obsCfg.TracingEnabled
	} else if !options.UseObservability {
		options.RecordMetrics = false
		options.RecordTrace = false
	}

	return options
}

// coreExecutor returns the innermost execution function (Reflection or core execution).
// Merged from loop_executor.go.
func (b *BaseAgent) coreExecutor(options EnhancedExecutionOptions) ExecutionFunc {
	return func(ctx context.Context, input *Input) (*Output, error) {
		if err := b.EnsureReady(); err != nil {
			return nil, err
		}
		executionOptions := b.executionOptionsResolver().Resolve(ctx, b.config, input)
		maxIterations := executionOptions.Control.MaxLoopIterations
		if maxIterations <= 0 {
			maxIterations = b.loopMaxIterations()
		}
		executor := &LoopExecutor{
			MaxIterations:     maxIterations,
			ExecutionOptions:  executionOptions,
			Planner:           b.loopPlanner(executionOptions),
			StepExecutor:      b.loopStepExecutor(options),
			Observer:          b.loopObserver(),
			Selector:          b.loopSelector(executionOptions, options),
			Judge:             b.completionJudge,
			ReflectionStep:    b.loopReflectionStep(options),
			ReasoningRuntime:  b.effectiveReasoningRuntime(executionOptions, options),
			ReasoningRegistry: b.reasoningRegistry,
			ReflectionEnabled: options.UseReflection && b.extensions.ReflectionExecutor() != nil,
			CheckpointManager: b.checkpointManager,
			Explainability:    explainabilityTimelineRecorder(b.extensions.ObservabilitySystemExt()),
			TraceID:           strings.TrimSpace(input.TraceID),
			AgentID:           b.ID(),
			Logger:            b.logger,
		}
		return executor.Execute(ctx, input)
	}
}
// Merged from loop_executor_runtime.go.
func (b *BaseAgent) loopMaxIterations() int {
	policy := b.loopControlPolicy()
	if policy.LoopIterationBudget > 0 {
		return policy.LoopIterationBudget
	}
	return 1
}
// GetFeatureStatus 获取功能启用状态。
func (b *BaseAgent) GetFeatureStatus() map[string]bool {
	return agentfeatures.FeatureStatus(b.extensions.GetFeatureStatus(), b.contextManager != nil)
}

// PrintFeatureStatus 打印功能状态。
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

// ValidateConfiguration 验证配置。
func (b *BaseAgent) ValidateConfiguration() error {
	validationErrors := agentfeatures.ConfigurationValidationErrors(b.extensions.ValidateConfiguration(b.config), b.hasMainExecutionSurface())
	if len(validationErrors) > 0 {
		return NewError(types.ErrInputValidation, "configuration validation failed: "+strings.Join(validationErrors, "; "))
	}

	b.logger.Info("configuration validated successfully")
	return nil
}

// GetFeatureMetrics 获取功能使用指标。
func (b *BaseAgent) GetFeatureMetrics() map[string]any {
	return agentfeatures.FeatureMetrics(b.ID(), b.Name(), string(b.Type()), b.GetFeatureStatus(), b.config.ExecutionOptions())
}

// ExportConfiguration 导出配置（用于持久化或分享）。
func (b *BaseAgent) ExportConfiguration() map[string]any {
	return agentfeatures.ExportConfiguration(b.config)
}

func (b *BaseAgent) loopPlanner(options types.ExecutionOptions) LoopPlannerFunc {
	return func(ctx context.Context, input *Input, _ *LoopState) (*PlanResult, error) {
		if options.Control.DisablePlanner {
			return nil, nil
		}
		plan, err := b.Plan(ctx, input)
		if err != nil && isIgnorableLoopPlanError(err) {
			b.logger.Warn("loop planner skipped after ignorable plan error",
				zap.Error(err),
				zap.String("trace_id", input.TraceID),
			)
			return nil, nil
		}
		return plan, err
	}
}

func (b *BaseAgent) loopObserver() LoopObserveFunc {
	return func(ctx context.Context, feedback *Feedback, _ *LoopState) error {
		return b.Observe(ctx, feedback)
	}
}

func (b *BaseAgent) loopStepExecutor(options EnhancedExecutionOptions) LoopStepExecutorFunc {
	return func(ctx context.Context, input *Input, _ *LoopState, selection ReasoningSelection) (*Output, error) {
		switch {
		case selection.Pattern != nil:
			result, err := selection.Pattern.Execute(ctx, input.Content)
			if err != nil {
				return nil, NewErrorWithCause(types.ErrAgentExecution, "reasoning execution failed", err)
			}
			return OutputFromReasoningResult(input.TraceID, result), nil
		default:
			return b.executeCore(ctx, input)
		}
	}
}

func (b *BaseAgent) loopReflectionStep(options EnhancedExecutionOptions) LoopReflectionFunc {
	if !(options.UseReflection && b.extensions.ReflectionExecutor() != nil) {
		return nil
	}
	reflector, ok := b.extensions.ReflectionExecutor().(interface {
		ReflectStep(ctx context.Context, input *Input, output *Output, state *LoopState) (*LoopReflectionResult, error)
	})
	if !ok {
		return nil
	}
	return func(ctx context.Context, input *Input, output *Output, state *LoopState) (*LoopReflectionResult, error) {
		result, err := reflector.ReflectStep(ctx, input, output, state)
		if err != nil {
			return nil, NewErrorWithCause(types.ErrAgentExecution, "reflection step failed", err)
		}
		return result, nil
	}
}
// =============================================================================
// Config helpers (merged from config_helpers.go)
// =============================================================================

func ensureAgentType(cfg *types.AgentConfig) {
	if cfg == nil {
		return
	}
	if strings.TrimSpace(cfg.Core.Type) == "" {
		cfg.Core.Type = string(TypeGeneric)
	}
}

func ensureReflectionEnabled(cfg *types.AgentConfig) {
	if cfg.Features.Reflection == nil {
		cfg.Features.Reflection = &types.ReflectionConfig{}
	}
	cfg.Features.Reflection.Enabled = true
	if cfg.Control.Reflection == nil {
		cfg.Control.Reflection = &types.ReflectionConfig{}
	}
	cfg.Control.Reflection.Enabled = true
}

func ensureToolSelectionEnabled(cfg *types.AgentConfig) {
	if cfg.Features.ToolSelection == nil {
		cfg.Features.ToolSelection = &types.ToolSelectionConfig{}
	}
	cfg.Features.ToolSelection.Enabled = true
	if cfg.Control.ToolSelection == nil {
		cfg.Control.ToolSelection = &types.ToolSelectionConfig{}
	}
	cfg.Control.ToolSelection.Enabled = true
}

func ensurePromptEnhancerEnabled(cfg *types.AgentConfig) {
	if cfg.Features.PromptEnhancer == nil {
		cfg.Features.PromptEnhancer = &types.PromptEnhancerConfig{}
	}
	cfg.Features.PromptEnhancer.Enabled = true
	if cfg.Control.PromptEnhancer == nil {
		cfg.Control.PromptEnhancer = &types.PromptEnhancerConfig{}
	}
	cfg.Control.PromptEnhancer.Enabled = true
}

func ensureSkillsEnabled(cfg *types.AgentConfig) {
	if cfg.Extensions.Skills == nil {
		cfg.Extensions.Skills = &types.SkillsConfig{}
	}
	cfg.Extensions.Skills.Enabled = true
}

func ensureMCPEnabled(cfg *types.AgentConfig) {
	if cfg.Extensions.MCP == nil {
		cfg.Extensions.MCP = &types.MCPConfig{}
	}
	cfg.Extensions.MCP.Enabled = true
}

func ensureLSPEnabled(cfg *types.AgentConfig) {
	if cfg.Extensions.LSP == nil {
		cfg.Extensions.LSP = &types.LSPConfig{}
	}
	cfg.Extensions.LSP.Enabled = true
}

func ensureEnhancedMemoryEnabled(cfg *types.AgentConfig) {
	if cfg.Features.Memory == nil {
		cfg.Features.Memory = &types.MemoryConfig{}
	}
	cfg.Features.Memory.Enabled = true
	if cfg.Control.Memory == nil {
		cfg.Control.Memory = &types.MemoryConfig{}
	}
	cfg.Control.Memory.Enabled = true
}

func ensureObservabilityEnabled(cfg *types.AgentConfig) {
	if cfg.Extensions.Observability == nil {
		cfg.Extensions.Observability = &types.ObservabilityConfig{}
	}
	cfg.Extensions.Observability.Enabled = true
}
func promptBundleFromConfig(cfg types.AgentConfig) PromptBundle {
	system := strings.TrimSpace(cfg.ExecutionOptions().Control.SystemPrompt)
	if system == "" {
		return PromptBundle{}
	}
	return PromptBundle{
		System: SystemPrompt{
			Identity: system,
		},
	}
}
// NewDefaultReasoningRegistry constructs the default reasoning registry used by
// runtime.Builder and registry-backed creation paths when the caller does not
// inject one explicitly. The default product surface keeps advanced and
// experimental strategies out of the runtime unless they are explicitly
// enabled.
func NewDefaultReasoningRegistry(
	gateway llmcore.Gateway,
	model string,
	toolManager ToolManager,
	agentID string,
	bus EventBus,
	logger *zap.Logger,
) *reasoning.PatternRegistry {
	return NewReasoningRegistryForExposure(
		gateway,
		model,
		toolManager,
		agentID,
		bus,
		ReasoningExposureOfficial,
		logger,
	)
}

// NewReasoningRegistryForExposure constructs a reasoning registry for the given
// public runtime exposure level.
func NewReasoningRegistryForExposure(
	gateway llmcore.Gateway,
	model string,
	toolManager ToolManager,
	agentID string,
	bus EventBus,
	level ReasoningExposureLevel,
	logger *zap.Logger,
) *reasoning.PatternRegistry {
	if logger == nil {
		logger = zap.NewNop()
	}
	level = normalizeReasoningExposureLevel(level)
	registry := reasoning.NewPatternRegistry()
	toolExecutor := newToolManagerExecutor(toolManager, agentID, nil, bus)
	toolSchemas := reasoningToolSchemas(toolManager, agentID)
	registerReasoningPatternsForExposure(registry, gateway, model, toolExecutor, toolSchemas, level, logger)
	return registry
}

func registerDefaultReasoningPattern(registry *reasoning.PatternRegistry, pattern reasoning.ReasoningPattern, logger *zap.Logger) {
	if err := registry.Register(pattern); err != nil {
		logger.Warn("skip duplicate default reasoning pattern", zap.String("pattern", pattern.Name()), zap.Error(err))
	}
}

func reasoningToolSchemas(toolManager ToolManager, agentID string) []types.ToolSchema {
	if toolManager == nil {
		return nil
	}
	return toolManager.GetAllowedTools(agentID)
}

func registerReasoningPatternsForExposure(
	registry *reasoning.PatternRegistry,
	gateway llmcore.Gateway,
	model string,
	toolExecutor llmtools.ToolExecutor,
	toolSchemas []types.ToolSchema,
	level ReasoningExposureLevel,
	logger *zap.Logger,
) {
	level = normalizeReasoningExposureLevel(level)
	if level == ReasoningExposureOfficial {
		return
	}

	refCfg := reasoning.DefaultReflexionConfig()
	refCfg.Model = model
	registerDefaultReasoningPattern(registry, reasoning.NewReflexionExecutor(gateway, toolExecutor, toolSchemas, refCfg, logger), logger)

	rewooCfg := reasoning.DefaultReWOOConfig()
	rewooCfg.Model = model
	registerDefaultReasoningPattern(registry, reasoning.NewReWOO(gateway, toolExecutor, toolSchemas, rewooCfg, logger), logger)

	peCfg := reasoning.DefaultPlanExecuteConfig()
	peCfg.Model = model
	registerDefaultReasoningPattern(registry, reasoning.NewPlanAndExecute(gateway, toolExecutor, toolSchemas, peCfg, logger), logger)

	if level != ReasoningExposureAll {
		return
	}

	dpCfg := reasoning.DefaultDynamicPlannerConfig()
	dpCfg.Model = model
	registerDefaultReasoningPattern(registry, reasoning.NewDynamicPlanner(gateway, toolExecutor, toolSchemas, dpCfg, logger), logger)

	totCfg := reasoning.DefaultTreeOfThoughtConfig()
	totCfg.Model = model
	registerDefaultReasoningPattern(registry, reasoning.NewTreeOfThought(gateway, toolExecutor, totCfg, logger), logger)

	idCfg := reasoning.DefaultIterativeDeepeningConfig()
	registerDefaultReasoningPattern(registry, reasoning.NewIterativeDeepening(gateway, toolExecutor, idCfg, logger), logger)
}
func (b *BaseAgent) loopSelector(executionOptions types.ExecutionOptions, options EnhancedExecutionOptions) ReasoningModeSelector {
	base := b.reasoningSelector
	if base == nil {
		base = NewDefaultReasoningModeSelector()
	}
	if !(options.UseReflection && b.extensions.ReflectionExecutor() != nil) {
		return base
	}
	return reasoningModeSelectorFunc(func(ctx context.Context, input *Input, state *LoopState, registry *reasoning.PatternRegistry, reflectionEnabled bool) ReasoningSelection {
		selection := base.Select(ctx, input, state, registry, reflectionEnabled)
		if executionOptions.Control.DisablePlanner {
			return selection
		}
		if strings.TrimSpace(selection.Mode) == "" || selection.Mode == ReasoningModeReact {
			selection.Mode = ReasoningModeReflection
		}
		return selection
	})
}

func (b *BaseAgent) effectiveReasoningRuntime(options types.ExecutionOptions, enhanced EnhancedExecutionOptions) ReasoningRuntime {
	if b.reasoningRuntime != nil {
		return b.reasoningRuntime
	}
	return NewDefaultReasoningRuntime(
		options,
		b.reasoningRegistry,
		enhanced.UseReflection && b.extensions.ReflectionExecutor() != nil,
		b.loopSelector(options, enhanced),
		b.loopStepExecutor(enhanced),
		b.loopReflectionStep(enhanced),
	)
}
