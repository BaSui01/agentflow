package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// EnhancedExecutionOptions 增强执行选项
type EnhancedExecutionOptions struct {
	UseReflection bool

	UseToolSelection bool

	UsePromptEnhancer bool

	UseSkills   bool
	SkillsQuery string

	UseEnhancedMemory   bool
	LoadWorkingMemory   bool
	LoadShortTermMemory bool
	SaveToMemory        bool

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
	b.extensions.EnableReflection(executor)
}

// EnableToolSelection 启用动态工具选择
func (b *BaseAgent) EnableToolSelection(selector DynamicToolSelectorRunner) {
	b.extensions.EnableToolSelection(selector)
}

// EnablePromptEnhancer 启用提示词增强
func (b *BaseAgent) EnablePromptEnhancer(enhancer PromptEnhancerRunner) {
	b.extensions.EnablePromptEnhancer(enhancer)
}

// EnableSkills 启用 Skills 系统
func (b *BaseAgent) EnableSkills(manager SkillDiscoverer) {
	b.extensions.EnableSkills(manager)
}

// EnableMCP 启用 MCP 集成
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

// EnableEnhancedMemory 启用增强记忆系统
func (b *BaseAgent) EnableEnhancedMemory(memorySystem EnhancedMemoryRunner) {
	b.extensions.EnableEnhancedMemory(memorySystem)
	b.memoryFacade = NewUnifiedMemoryFacade(b.memory, memorySystem, b.logger)
}

// EnableObservability 启用可观测性系统
func (b *BaseAgent) EnableObservability(obsSystem ObservabilityRunner) {
	b.extensions.EnableObservability(obsSystem)
}

// ExecuteEnhanced 增强执行（集成所有功能）
// Uses a middleware pipeline so that each step is an independent, composable unit.
func (b *BaseAgent) ExecuteEnhanced(ctx context.Context, input *Input, options EnhancedExecutionOptions) (*Output, error) {
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

// coreExecutor returns the innermost execution function (Reflection or plain Execute).
func (b *BaseAgent) coreExecutor(options EnhancedExecutionOptions) ExecutionFunc {
	return func(ctx context.Context, input *Input) (*Output, error) {
		if options.UseReflection && b.extensions.ReflectionExecutor() != nil {
			b.logger.Debug("executing with reflection")
			output, err := b.extensions.ReflectionExecutor().ExecuteWithReflection(ctx, input)
			if err != nil {
				return nil, NewErrorWithCause(types.ErrAgentExecution, "reflection execution failed", err)
			}
			return output, nil
		}
		return b.Execute(ctx, input)
	}
}

// --- context keys for inter-middleware data passing ---

type enhancedCtxKey int

const (
	ctxKeySkillInstructions enhancedCtxKey = iota
	ctxKeyMemoryContext
)

func withSkillInstructions(ctx context.Context, instructions []string) context.Context {
	return context.WithValue(ctx, ctxKeySkillInstructions, instructions)
}

func skillInstructionsFromCtx(ctx context.Context) []string {
	v, _ := ctx.Value(ctxKeySkillInstructions).([]string)
	return v
}

func withMemoryContext(ctx context.Context, memCtx []string) context.Context {
	return context.WithValue(ctx, ctxKeyMemoryContext, memCtx)
}

func memoryContextFromCtx(ctx context.Context) []string {
	v, _ := ctx.Value(ctxKeyMemoryContext).([]string)
	return v
}

// --- Middleware implementations ---

func (b *BaseAgent) observabilityMiddleware(options EnhancedExecutionOptions) ExecutionMiddleware {
	return func(ctx context.Context, input *Input, next ExecutionFunc) (*Output, error) {
		startTime := time.Now()
		traceID := input.TraceID
		b.extensions.ObservabilitySystemExt().StartTrace(traceID, b.ID())

		output, err := next(ctx, input)

		if err != nil {
			b.extensions.ObservabilitySystemExt().EndTrace(traceID, "failed", err)
			return nil, err
		}
		duration := time.Since(startTime)
		if options.RecordMetrics {
			b.extensions.ObservabilitySystemExt().RecordTask(b.ID(), true, duration, output.TokensUsed, output.Cost, 0.8)
		}
		if options.RecordTrace {
			b.extensions.ObservabilitySystemExt().EndTrace(traceID, "completed", nil)
		}
		b.logger.Info("enhanced execution completed",
			zap.String("trace_id", input.TraceID),
			zap.Duration("total_duration", duration),
			zap.Int("tokens_used", output.TokensUsed),
		)
		return output, nil
	}
}

func (b *BaseAgent) skillsMiddleware(options EnhancedExecutionOptions) ExecutionMiddleware {
	return func(ctx context.Context, input *Input, next ExecutionFunc) (*Output, error) {
		query := options.SkillsQuery
		if query == "" {
			query = input.Content
		}
		b.logger.Debug("discovering skills", zap.String("trace_id", input.TraceID), zap.String("query", query))

		var skillInstructions []string
		found, err := b.extensions.SkillManagerExt().DiscoverSkills(ctx, query)
		if err != nil {
			b.logger.Warn("skill discovery failed", zap.String("trace_id", input.TraceID), zap.Error(err))
		} else {
			for _, s := range found {
				if s == nil {
					continue
				}
				skillInstructions = append(skillInstructions, s.GetInstructions())
			}
			b.logger.Info("skills discovered", zap.Int("count", len(skillInstructions)))
		}

		if len(skillInstructions) > 0 {
			input = shallowCopyInput(input)
			input.Content = prependSkillInstructions(input.Content, skillInstructions)
		}
		ctx = withSkillInstructions(ctx, skillInstructions)
		return next(ctx, input)
	}
}

func (b *BaseAgent) memoryLoadMiddleware(options EnhancedExecutionOptions) ExecutionMiddleware {
	return func(ctx context.Context, input *Input, next ExecutionFunc) (*Output, error) {
		var memoryContext []string
		if options.LoadWorkingMemory {
			b.logger.Debug("loading working memory", zap.String("trace_id", input.TraceID))
			working, err := b.extensions.EnhancedMemoryExt().LoadWorking(ctx, b.ID())
			if err != nil {
				b.logger.Warn("failed to load working memory", zap.String("trace_id", input.TraceID), zap.Error(err))
			} else {
				for _, entry := range working {
					if entry.Content != "" {
						memoryContext = append(memoryContext, entry.Content)
					}
				}
				b.logger.Info("working memory loaded", zap.String("trace_id", input.TraceID), zap.Int("count", len(working)))
			}
		}
		if options.LoadShortTermMemory {
			b.logger.Debug("loading short-term memory", zap.String("trace_id", input.TraceID))
			shortTerm, err := b.extensions.EnhancedMemoryExt().LoadShortTerm(ctx, b.ID(), 5)
			if err != nil {
				b.logger.Warn("failed to load short-term memory", zap.String("trace_id", input.TraceID), zap.Error(err))
			} else {
				for _, entry := range shortTerm {
					if entry.Content != "" {
						memoryContext = append(memoryContext, entry.Content)
					}
				}
				b.logger.Info("short-term memory loaded", zap.String("trace_id", input.TraceID), zap.Int("count", len(shortTerm)))
			}
		}

		ctx = withMemoryContext(ctx, memoryContext)
		return next(ctx, input)
	}
}

func (b *BaseAgent) promptEnhancerMiddleware() ExecutionMiddleware {
	return func(ctx context.Context, input *Input, next ExecutionFunc) (*Output, error) {
		b.logger.Debug("enhancing prompt", zap.String("trace_id", input.TraceID))
		contextStr := ""
		if si := skillInstructionsFromCtx(ctx); len(si) > 0 {
			contextStr += "Skills: " + fmt.Sprintf("%v", si) + "\n"
		}
		if mc := memoryContextFromCtx(ctx); len(mc) > 0 {
			contextStr += "Memory: " + fmt.Sprintf("%v", mc) + "\n"
		}

		enhanced, err := b.extensions.PromptEnhancerExt().EnhanceUserPrompt(input.Content, contextStr)
		if err != nil {
			b.logger.Warn("prompt enhancement failed", zap.String("trace_id", input.TraceID), zap.Error(err))
		} else {
			input = shallowCopyInput(input)
			input.Content = enhanced
			b.logger.Info("prompt enhanced", zap.String("trace_id", input.TraceID))
		}
		return next(ctx, input)
	}
}

func (b *BaseAgent) toolSelectionMiddleware() ExecutionMiddleware {
	return func(ctx context.Context, input *Input, next ExecutionFunc) (*Output, error) {
		b.logger.Debug("selecting tools dynamically", zap.String("trace_id", input.TraceID))
		availableTools := b.toolManager.GetAllowedTools(b.ID())
		selected, err := b.extensions.ToolSelector().SelectTools(ctx, input.Content, availableTools)
		if err != nil {
			b.logger.Warn("tool selection failed", zap.String("trace_id", input.TraceID), zap.Error(err))
		} else {
			b.logger.Info("tools selected dynamically", zap.String("trace_id", input.TraceID), zap.Any("selected", selected))
		}
		return next(ctx, input)
	}
}

func (b *BaseAgent) memorySaveMiddleware() ExecutionMiddleware {
	return func(ctx context.Context, input *Input, next ExecutionFunc) (*Output, error) {
		output, err := next(ctx, input)
		if err != nil {
			return nil, err
		}
		b.logger.Debug("saving to enhanced memory", zap.String("trace_id", input.TraceID))
		b.extensions.SaveToEnhancedMemory(ctx, b.ID(), input, output, false)
		return output, nil
	}
}

// shallowCopyInput creates a shallow copy of Input so that middlewares
// can safely mutate Content/Context without affecting the caller's value.
func shallowCopyInput(in *Input) *Input {
	cp := *in
	if in.Context != nil {
		cp.Context = make(map[string]any, len(in.Context))
		for k, v := range in.Context {
			cp.Context[k] = v
		}
	}
	return &cp
}

// --- Remaining helpers (unchanged) ---

// GetFeatureStatus 获取功能启用状态
func (b *BaseAgent) GetFeatureStatus() map[string]bool {
	status := b.extensions.GetFeatureStatus()
	status["context_manager"] = b.contextManager != nil
	return status
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

// ValidateConfiguration 验证配置
func (b *BaseAgent) ValidateConfiguration() error {
	validationErrors := b.extensions.ValidateConfiguration(b.config)

	if b.provider == nil {
		validationErrors = append(validationErrors, "provider not set")
	}

	if len(validationErrors) > 0 {
		return NewError(types.ErrInputValidation, "configuration validation failed: "+strings.Join(validationErrors, "; "))
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
			"model":       b.config.LLM.Model,
			"provider":    b.config.LLM.Provider,
			"max_tokens":  b.config.LLM.MaxTokens,
			"temperature": b.config.LLM.Temperature,
		},
	}

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
		"id":          b.config.Core.ID,
		"name":        b.config.Core.Name,
		"type":        b.config.Core.Type,
		"description": b.config.Core.Description,
		"model":       b.config.LLM.Model,
		"provider":    b.config.LLM.Provider,
		"features": map[string]bool{
			"reflection":      b.config.IsReflectionEnabled(),
			"tool_selection":  b.config.IsToolSelectionEnabled(),
			"prompt_enhancer": b.config.IsPromptEnhancerEnabled(),
			"skills":          b.config.IsSkillsEnabled(),
			"mcp":             b.config.IsMCPEnabled(),
			"lsp":             b.config.IsLSPEnabled(),
			"enhanced_memory": b.config.IsMemoryEnabled(),
			"observability":   b.config.IsObservabilityEnabled(),
		},
		"tools":    b.config.Runtime.Tools,
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

func (a *reflectionRunnerAdapter) ExecuteWithReflection(ctx context.Context, input *Input) (*Output, error) {
	result, err := a.executor.ExecuteWithReflection(ctx, input)
	if err != nil {
		return nil, err
	}
	return result.FinalOutput, nil
}

// AsReflectionRunner wraps a *ReflectionExecutor as a ReflectionRunner.
func AsReflectionRunner(executor *ReflectionExecutor) ReflectionRunner {
	return &reflectionRunnerAdapter{executor: executor}
}

// AsToolSelectorRunner wraps a *DynamicToolSelector as a DynamicToolSelectorRunner.
// Since the interface now uses concrete types, this is a direct cast.
func AsToolSelectorRunner(selector *DynamicToolSelector) DynamicToolSelectorRunner {
	return selector
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

// =============================================================================
// Execution Pipeline (middleware chain)
// =============================================================================

// ExecutionFunc is the core agent execution function signature.
type ExecutionFunc func(ctx context.Context, input *Input) (*Output, error)

// ExecutionMiddleware wraps an ExecutionFunc, adding pre/post processing.
// Call next to proceed to the next middleware (or the core executor).
type ExecutionMiddleware func(ctx context.Context, input *Input, next ExecutionFunc) (*Output, error)

// ExecutionPipeline chains middlewares around a core ExecutionFunc.
type ExecutionPipeline struct {
	middlewares []ExecutionMiddleware
	core        ExecutionFunc
}

// NewExecutionPipeline creates a pipeline that wraps the given core function.
func NewExecutionPipeline(core ExecutionFunc) *ExecutionPipeline {
	return &ExecutionPipeline{core: core}
}

// Use appends one or more middlewares. They execute in the order added
// (first added = outermost wrapper).
func (p *ExecutionPipeline) Use(mws ...ExecutionMiddleware) {
	p.middlewares = append(p.middlewares, mws...)
}

// Execute runs the full middleware chain followed by the core function.
func (p *ExecutionPipeline) Execute(ctx context.Context, input *Input) (*Output, error) {
	if input == nil {
		return nil, NewError(types.ErrInputValidation, "pipeline input is nil")
	}
	fn := p.core
	for i := len(p.middlewares) - 1; i >= 0; i-- {
		mw := p.middlewares[i]
		next := fn
		fn = func(ctx context.Context, input *Input) (*Output, error) {
			return mw(ctx, input, next)
		}
	}
	return fn(ctx, input)
}
