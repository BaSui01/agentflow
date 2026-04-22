package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/BaSui01/agentflow/agent/capabilities/guardrails"
	"github.com/BaSui01/agentflow/agent/capabilities/reasoning"
	agentcontext "github.com/BaSui01/agentflow/agent/execution/context"
	llmtools "github.com/BaSui01/agentflow/llm/capabilities/tools"
	"github.com/BaSui01/agentflow/llm/observability"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
	"strings"
	"sync/atomic"
	"time"
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

// CompletionDecision is the normalized evaluation result for loop execution.
type CompletionDecision struct {
	Solved         bool         `json:"solved"`
	NeedReplan     bool         `json:"need_replan,omitempty"`
	NeedReflection bool         `json:"need_reflection,omitempty"`
	NeedHuman      bool         `json:"need_human,omitempty"`
	Decision       LoopDecision `json:"decision"`
	StopReason     StopReason   `json:"stop_reason,omitempty"`
	Confidence     float64      `json:"confidence,omitempty"`
	Reason         string       `json:"reason,omitempty"`
}

func (b *BaseAgent) loopMaxIterations() int {
	policy := b.loopControlPolicy()
	if policy.LoopIterationBudget > 0 {
		return policy.LoopIterationBudget
	}
	return 1
}

type LoopPlannerFunc func(ctx context.Context, input *Input, state *LoopState) (*PlanResult, error)
type LoopStepExecutorFunc func(ctx context.Context, input *Input, state *LoopState, selection ReasoningSelection) (*Output, error)
type LoopObserveFunc func(ctx context.Context, feedback *Feedback, state *LoopState) error
type LoopReflectionFunc func(ctx context.Context, input *Input, output *Output, state *LoopState) (*LoopReflectionResult, error)
type LoopValidationFunc func(ctx context.Context, input *Input, state *LoopState, output *Output, err error) (*LoopValidationResult, error)

type LoopReflectionResult struct {
	NextInput   *Input
	Critique    *Critique
	Observation *LoopObservation
}

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

	if !b.hasMainExecutionSurface() {
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
	executionOptions := b.config.ExecutionOptions()

	metrics := map[string]any{
		"agent_id":   b.ID(),
		"agent_name": b.Name(),
		"agent_type": string(b.Type()),
		"features":   status,
		"config": map[string]any{
			"model":       executionOptions.Model.Model,
			"provider":    executionOptions.Model.Provider,
			"max_tokens":  executionOptions.Model.MaxTokens,
			"temperature": executionOptions.Model.Temperature,
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

// ExportConfiguration 导出配置（用于持久化或分享）
func (b *BaseAgent) ExportConfiguration() map[string]any {
	executionOptions := b.config.ExecutionOptions()
	return map[string]any{
		"id":              b.config.Core.ID,
		"name":            b.config.Core.Name,
		"type":            b.config.Core.Type,
		"description":     b.config.Core.Description,
		"model":           executionOptions.Model.Model,
		"provider":        executionOptions.Model.Provider,
		"runtime_model":   executionOptions.Model,
		"runtime_control": executionOptions.Control,
		"runtime_tools":   executionOptions.Tools,
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
		"tools":    executionOptions.Tools.AllowedTools,
		"metadata": b.config.Metadata,
	}
}

// Merged from completion.go.

// DefaultStreamInactivityTimeout 是流式响应的默认空闲超时时间.
// 只要还在收到数据，就不会超时；只有在超过此时间没有新数据时才触发超时.
const DefaultStreamInactivityTimeout = 5 * time.Minute

// CompletionJudge decides whether the loop can stop or must continue.
type CompletionJudge interface {
	Judge(ctx context.Context, state *LoopState, output *Output, err error) (*CompletionDecision, error)
}

type LoopValidationStatus string

const (
	LoopValidationStatusPassed  LoopValidationStatus = "passed"
	LoopValidationStatusPending LoopValidationStatus = "pending"
	LoopValidationStatusFailed  LoopValidationStatus = "failed"
)

type LoopValidationIssue struct {
	Validator string               `json:"validator,omitempty"`
	Code      string               `json:"code,omitempty"`
	Category  string               `json:"category,omitempty"`
	Status    LoopValidationStatus `json:"status,omitempty"`
	Message   string               `json:"message,omitempty"`
}

type LoopValidationResult struct {
	Status             LoopValidationStatus  `json:"status,omitempty"`
	Passed             bool                  `json:"passed"`
	Pending            bool                  `json:"pending,omitempty"`
	Reason             string                `json:"reason,omitempty"`
	Summary            string                `json:"summary,omitempty"`
	Issues             []LoopValidationIssue `json:"issues,omitempty"`
	AcceptanceCriteria []string              `json:"acceptance_criteria,omitempty"`
	UnresolvedItems    []string              `json:"unresolved_items,omitempty"`
	RemainingRisks     []string              `json:"remaining_risks,omitempty"`
	Metadata           map[string]any        `json:"metadata,omitempty"`
}

type LoopValidator interface {
	Validate(ctx context.Context, input *Input, state *LoopState, output *Output, err error) (*LoopValidationResult, error)
}

type LoopValidatorChain struct {
	validators []LoopValidator
}

func NewLoopValidatorChain(validators ...LoopValidator) *LoopValidatorChain {
	filtered := make([]LoopValidator, 0, len(validators))
	for _, validator := range validators {
		if validator != nil {
			filtered = append(filtered, validator)
		}
	}
	return &LoopValidatorChain{validators: filtered}
}

func (c *LoopValidatorChain) Validate(ctx context.Context, input *Input, state *LoopState, output *Output, err error) (*LoopValidationResult, error) {
	aggregate := newLoopValidationResult(LoopValidationStatusPassed, "validation passed")
	aggregate.AcceptanceCriteria = acceptanceCriteriaForValidation(input, state)
	for _, validator := range c.validators {
		result, validateErr := validator.Validate(ctx, input, state, output, err)
		if validateErr != nil {
			return nil, validateErr
		}
		mergeLoopValidationResult(aggregate, result)
	}
	finalizeLoopValidationResult(aggregate)
	return aggregate, nil
}

// DefaultCompletionJudge implements the unified stop semantics.
type DefaultCompletionJudge struct{}

func NewDefaultCompletionJudge() *DefaultCompletionJudge { return &DefaultCompletionJudge{} }

func (j *DefaultCompletionJudge) Judge(ctx context.Context, state *LoopState, output *Output, err error) (*CompletionDecision, error) {
	if ctx != nil && ctx.Err() != nil {
		return &CompletionDecision{Decision: LoopDecisionDone, StopReason: StopReasonTimeout, Reason: ctx.Err().Error()}, nil
	}
	if state != nil && state.NeedHuman {
		return &CompletionDecision{
			NeedHuman:  true,
			Decision:   LoopDecisionEscalate,
			StopReason: StopReasonNeedHuman,
			Confidence: normalizedConfidence(output, state),
			Reason:     "loop state requires human intervention",
		}, nil
	}
	if err != nil {
		return &CompletionDecision{Decision: LoopDecisionDone, StopReason: classifyStopReason(err.Error()), Reason: err.Error()}, nil
	}
	if output == nil {
		if reachedMaxIterations(state) {
			return &CompletionDecision{
				Decision:   LoopDecisionDone,
				StopReason: StopReasonMaxIterations,
				Confidence: normalizedConfidence(output, state),
				Reason:     "loop iteration budget exhausted",
			}, nil
		}
		return &CompletionDecision{Decision: LoopDecisionReplan, NeedReplan: true, StopReason: StopReasonBlocked, Reason: "output is nil"}, nil
	}
	validation := completionValidationState(state, output)
	if strings.TrimSpace(output.Content) != "" && validation.Status == LoopValidationStatusPassed {
		return &CompletionDecision{
			Solved:     true,
			Decision:   LoopDecisionDone,
			StopReason: StopReasonSolved,
			Confidence: normalizedConfidence(output, state),
			Reason:     "validated output produced",
		}, nil
	}
	if strings.TrimSpace(output.Content) != "" && validation.Status == LoopValidationStatusPending {
		if reachedMaxIterations(state) {
			return &CompletionDecision{
				Decision:   LoopDecisionDone,
				StopReason: StopReasonMaxIterations,
				Confidence: normalizedConfidence(output, state),
				Reason:     validation.Reason,
			}, nil
		}
		return &CompletionDecision{
			Decision:   LoopDecisionContinue,
			StopReason: "",
			Confidence: normalizedConfidence(output, state),
			Reason:     validation.Reason,
		}, nil
	}
	if strings.TrimSpace(output.Content) != "" && validation.Status == LoopValidationStatusFailed {
		if reachedMaxIterations(state) {
			return &CompletionDecision{
				Decision:   LoopDecisionDone,
				StopReason: StopReasonValidationFailed,
				Confidence: normalizedConfidence(output, state),
				Reason:     validation.Reason,
			}, nil
		}
		return &CompletionDecision{
			Decision:   LoopDecisionReplan,
			NeedReplan: true,
			StopReason: StopReasonValidationFailed,
			Confidence: normalizedConfidence(output, state),
			Reason:     validation.Reason,
		}, nil
	}
	if reachedMaxIterations(state) {
		return &CompletionDecision{
			Decision:   LoopDecisionDone,
			StopReason: StopReasonMaxIterations,
			Confidence: normalizedConfidence(output, state),
			Reason:     "loop iteration budget exhausted",
		}, nil
	}
	return &CompletionDecision{
		Decision:   LoopDecisionReplan,
		NeedReplan: true,
		StopReason: StopReasonBlocked,
		Confidence: normalizedConfidence(output, state),
		Reason:     "output content is empty",
	}, nil
}

func reachedMaxIterations(state *LoopState) bool {
	return state != nil && state.MaxIterations > 0 && state.Iteration >= state.MaxIterations
}

func classifyStopReason(msg string) StopReason {
	normalized := strings.ToLower(strings.TrimSpace(msg))
	switch {
	case strings.Contains(normalized, "timeout"),
		strings.Contains(normalized, "deadline exceeded"),
		strings.Contains(normalized, "context deadline exceeded"),
		strings.Contains(normalized, "context canceled"),
		strings.Contains(normalized, "context cancelled"):
		return StopReasonTimeout
	case strings.Contains(normalized, "validation"):
		return StopReasonValidationFailed
	case strings.Contains(normalized, "tool"):
		return StopReasonToolFailureUnrecoverable
	default:
		return StopReasonBlocked
	}
}

func normalizedConfidence(output *Output, state *LoopState) float64 {
	if output != nil && output.Metadata != nil {
		raw, ok := output.Metadata["confidence"]
		if ok {
			value, ok := raw.(float64)
			if ok {
				return clampConfidence(value)
			}
		}
	}
	if state != nil && state.Confidence > 0 {
		return clampConfidence(state.Confidence)
	}
	return 1
}

func clampConfidence(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func goalRequiresValidation(state *LoopState) bool {
	if state == nil {
		return false
	}
	goal := strings.ToLower(strings.TrimSpace(state.Goal))
	if goal == "" {
		return false
	}
	return strings.Contains(goal, "validate") ||
		strings.Contains(goal, "validation") ||
		strings.Contains(goal, "verify") ||
		strings.Contains(goal, "verified") ||
		strings.Contains(goal, "acceptance")
}

type completionValidationStateView struct {
	Status          LoopValidationStatus
	Reason          string
	UnresolvedItems []string
	RemainingRisks  []string
}

func completionValidationState(state *LoopState, output *Output) completionValidationStateView {
	status := LoopValidationStatusPassed
	unresolvedItems := cloneStringSlice(nil)
	remainingRisks := cloneStringSlice(nil)
	reason := ""
	if state != nil {
		status = state.ValidationStatus
		unresolvedItems = normalizeStringSlice(cloneStringSlice(state.UnresolvedItems))
		remainingRisks = normalizeStringSlice(cloneStringSlice(state.RemainingRisks))
		reason = strings.TrimSpace(state.ValidationSummary)
	}
	if output != nil {
		if metaStatus := LoopValidationStatus(metadataString(output.Metadata, "validation_status")); metaStatus != "" {
			status = worseValidationStatus(status, metaStatus)
		}
		validationPending := false
		if pending, ok := metadataBool(output.Metadata, "validation_pending"); ok && pending {
			validationPending = true
			status = worseValidationStatus(status, LoopValidationStatusPending)
			reason = fallbackString(reason, fallbackMetadataReason(output.Metadata, "validation pending"))
			unresolvedItems = appendUniqueString(unresolvedItems, "complete validation")
		}
		if passed, ok := metadataBool(output.Metadata, "validation_passed"); ok && !passed && !validationPending {
			status = worseValidationStatus(status, LoopValidationStatusFailed)
			reason = fallbackString(reason, fallbackMetadataReason(output.Metadata, "validation failed"))
		}
		if acceptanceMet, ok := metadataBool(output.Metadata, "acceptance_criteria_met", "acceptance_passed"); ok && !acceptanceMet {
			status = worseValidationStatus(status, LoopValidationStatusPending)
			reason = fallbackString(reason, "acceptance criteria not met")
			unresolvedItems = appendUniqueString(unresolvedItems, "validate acceptance criteria")
		}
		if pending, ok := metadataBool(output.Metadata, "tool_verification_pending", "verification_pending"); ok && pending {
			status = worseValidationStatus(status, LoopValidationStatusPending)
			reason = fallbackString(reason, "tool verification pending")
			unresolvedItems = appendUniqueString(unresolvedItems, "tool verification pending")
		}
		if passed, ok := metadataBool(output.Metadata, "tool_verification_passed", "verification_passed", "verified"); ok && !passed {
			status = worseValidationStatus(status, LoopValidationStatusPending)
			reason = fallbackString(reason, "tool verification pending")
			unresolvedItems = appendUniqueString(unresolvedItems, "tool verification pending")
		}
		if values, ok := loopContextStrings(output.Metadata, "unresolved_items", "remaining_work"); ok {
			unresolvedItems = normalizeStringSlice(append(unresolvedItems, values...))
		}
		if values, ok := loopContextStrings(output.Metadata, "remaining_risks"); ok {
			remainingRisks = normalizeStringSlice(append(remainingRisks, values...))
		}
		reason = fallbackString(reason, metadataString(output.Metadata, "validation_summary", "validation_reason", "validation_message"))
	}
	if len(unresolvedItems) > 0 || len(remainingRisks) > 0 {
		if status == "" || status == LoopValidationStatusPassed {
			status = LoopValidationStatusPending
		}
	}
	if status == "" {
		switch {
		case len(state.AcceptanceCriteria) > 0:
			status = LoopValidationStatusPending
		case goalRequiresValidation(state):
			status = LoopValidationStatusPending
		default:
			status = LoopValidationStatusPassed
		}
	}
	if reason == "" {
		reason = summarizeValidationState(status, unresolvedItems, remainingRisks)
	}
	return completionValidationStateView{
		Status:          status,
		Reason:          reason,
		UnresolvedItems: unresolvedItems,
		RemainingRisks:  remainingRisks,
	}
}

type DefaultLoopValidator struct {
	chain *LoopValidatorChain
}

func NewDefaultLoopValidator() *DefaultLoopValidator {
	return &DefaultLoopValidator{
		chain: NewLoopValidatorChain(
			GenericLoopValidator{},
			ToolVerificationLoopValidator{},
			NewCodeTaskLoopValidator(),
		),
	}
}

func (v *DefaultLoopValidator) Validate(ctx context.Context, input *Input, state *LoopState, output *Output, err error) (*LoopValidationResult, error) {
	if v == nil || v.chain == nil {
		return newLoopValidationResult(LoopValidationStatusPassed, "validation passed"), nil
	}
	return v.chain.Validate(ctx, input, state, output, err)
}

type GenericLoopValidator struct{}

func (GenericLoopValidator) Validate(_ context.Context, input *Input, state *LoopState, output *Output, err error) (*LoopValidationResult, error) {
	result := newLoopValidationResult(LoopValidationStatusPassed, "validation passed")
	result.AcceptanceCriteria = acceptanceCriteriaForValidation(input, state)
	result.UnresolvedItems = unresolvedItemsForValidation(state, output)
	result.RemainingRisks = remainingRisksForValidation(state, output)
	if err != nil {
		result.Status = LoopValidationStatusFailed
		result.Reason = "validation skipped due to execution error"
		result.Issues = append(result.Issues, newValidationIssue("generic", "execution_error", "validation", result.Status, result.Reason))
		finalizeLoopValidationResult(result)
		return result, nil
	}
	if output == nil {
		result.Status = LoopValidationStatusPending
		result.Reason = "output missing for validation"
		result.UnresolvedItems = append(result.UnresolvedItems, "produce validated output")
		result.Issues = append(result.Issues, newValidationIssue("generic", "missing_output", "validation", result.Status, result.Reason))
		finalizeLoopValidationResult(result)
		return result, nil
	}

	acceptanceRequired := len(result.AcceptanceCriteria) > 0
	result.Metadata["acceptance_criteria_required"] = acceptanceRequired
	if acceptanceRequired {
		if value, ok := metadataBool(output.Metadata, "acceptance_criteria_met", "acceptance_passed"); ok {
			result.Metadata["acceptance_criteria_met"] = value
			if !value {
				result.Status = worseValidationStatus(result.Status, LoopValidationStatusPending)
				result.Reason = "acceptance criteria not met"
				result.UnresolvedItems = append(result.UnresolvedItems, "validate acceptance criteria")
				result.Issues = append(result.Issues, newValidationIssue("generic", "acceptance_not_met", "acceptance", result.Status, result.Reason))
			}
		} else {
			result.Status = LoopValidationStatusPending
			result.Reason = "acceptance criteria not yet validated"
			result.UnresolvedItems = append(result.UnresolvedItems, "validate acceptance criteria")
			result.Issues = append(result.Issues, newValidationIssue("generic", "acceptance_pending", "acceptance", result.Status, result.Reason))
			result.Metadata["acceptance_criteria_met"] = false
		}
	}

	explicitValidationPassed, hasExplicitValidationPassed := metadataBool(output.Metadata, "validation_passed")
	explicitValidationPending, _ := metadataBool(output.Metadata, "validation_pending")
	if hasExplicitValidationPassed && !explicitValidationPassed {
		result.Status = LoopValidationStatusFailed
		result.Reason = fallbackMetadataReason(output.Metadata, "validation failed")
		result.Issues = append(result.Issues, newValidationIssue("generic", "validation_failed", "validation", result.Status, result.Reason))
	}
	if explicitValidationPending {
		result.Status = worseValidationStatus(result.Status, LoopValidationStatusPending)
		result.Reason = fallbackMetadataReason(output.Metadata, "validation pending")
		result.UnresolvedItems = append(result.UnresolvedItems, "complete validation")
		result.Issues = append(result.Issues, newValidationIssue("generic", "validation_pending", "validation", LoopValidationStatusPending, result.Reason))
	}
	if goalRequiresValidation(state) && !hasExplicitValidationPassed && !explicitValidationPending && !acceptanceRequired {
		result.Status = worseValidationStatus(result.Status, LoopValidationStatusPending)
		result.Reason = "validation required before completion"
		result.UnresolvedItems = append(result.UnresolvedItems, "add validation evidence")
		result.Issues = append(result.Issues, newValidationIssue("generic", "validation_required", "validation", LoopValidationStatusPending, result.Reason))
	}
	if len(result.UnresolvedItems) > 0 && result.Status == LoopValidationStatusPassed {
		result.Status = LoopValidationStatusPending
		if strings.TrimSpace(result.Reason) == "" {
			result.Reason = "unresolved items remain"
		}
	}
	if len(result.RemainingRisks) > 0 && result.Status == LoopValidationStatusPassed {
		result.Status = LoopValidationStatusPending
		if strings.TrimSpace(result.Reason) == "" {
			result.Reason = "remaining risks require validation"
		}
	}
	finalizeLoopValidationResult(result)
	return result, nil
}

type ToolVerificationLoopValidator struct{}

func (ToolVerificationLoopValidator) Validate(_ context.Context, input *Input, state *LoopState, output *Output, err error) (*LoopValidationResult, error) {
	result := newLoopValidationResult(LoopValidationStatusPassed, "tool verification passed")
	if err != nil || output == nil {
		finalizeLoopValidationResult(result)
		return result, nil
	}
	required := toolVerificationRequired(input, state, output)
	result.Metadata["tool_verification_required"] = required
	if !required {
		finalizeLoopValidationResult(result)
		return result, nil
	}
	if pending, ok := metadataBool(output.Metadata, "tool_verification_pending", "verification_pending"); ok && pending {
		result.Status = LoopValidationStatusPending
		result.Reason = "tool verification pending"
		result.UnresolvedItems = append(result.UnresolvedItems, "verify tool-backed output")
		result.Issues = append(result.Issues, newValidationIssue("tool", "tool_verification_pending", "tool", result.Status, result.Reason))
		result.Metadata["tool_verification_pending"] = true
		finalizeLoopValidationResult(result)
		return result, nil
	}
	if passed, ok := metadataBool(output.Metadata, "tool_verification_passed", "verification_passed", "verified"); ok {
		result.Metadata["tool_verification_passed"] = passed
		if !passed {
			result.Status = LoopValidationStatusFailed
			result.Reason = "tool verification failed"
			result.Issues = append(result.Issues, newValidationIssue("tool", "tool_verification_failed", "tool", result.Status, result.Reason))
			finalizeLoopValidationResult(result)
			return result, nil
		}
		finalizeLoopValidationResult(result)
		return result, nil
	}
	result.Status = LoopValidationStatusPending
	result.Reason = "tool verification pending"
	result.UnresolvedItems = append(result.UnresolvedItems, "verify tool-backed output")
	result.Issues = append(result.Issues, newValidationIssue("tool", "tool_verification_missing", "tool", result.Status, result.Reason))
	result.Metadata["tool_verification_passed"] = false
	result.Metadata["tool_verification_pending"] = true
	finalizeLoopValidationResult(result)
	return result, nil
}

type CodeTaskLoopValidator struct {
	codeValidator *CodeValidator
}

func NewCodeTaskLoopValidator() CodeTaskLoopValidator {
	return CodeTaskLoopValidator{codeValidator: NewCodeValidator()}
}

func (v CodeTaskLoopValidator) Validate(_ context.Context, input *Input, state *LoopState, output *Output, err error) (*LoopValidationResult, error) {
	result := newLoopValidationResult(LoopValidationStatusPassed, "code verification passed")
	if err != nil || output == nil {
		finalizeLoopValidationResult(result)
		return result, nil
	}
	required := codeTaskRequired(input, state, output)
	result.Metadata["code_verification_required"] = required
	if !required {
		finalizeLoopValidationResult(result)
		return result, nil
	}
	if pending, ok := metadataBool(output.Metadata, "code_verification_pending", "tests_pending"); ok && pending {
		result.Status = LoopValidationStatusPending
		result.Reason = "code verification pending"
		result.UnresolvedItems = append(result.UnresolvedItems, "run tests or verification for code changes")
		result.Issues = append(result.Issues, newValidationIssue("code", "code_verification_pending", "code", result.Status, result.Reason))
		finalizeLoopValidationResult(result)
		return result, nil
	}
	if passed, ok := metadataBool(output.Metadata, "code_verification_passed", "tests_passed", "tests_green"); ok {
		result.Metadata["code_verification_passed"] = passed
		if !passed {
			result.Status = LoopValidationStatusFailed
			result.Reason = "code verification failed"
			result.Issues = append(result.Issues, newValidationIssue("code", "code_verification_failed", "code", result.Status, result.Reason))
			finalizeLoopValidationResult(result)
			return result, nil
		}
	} else {
		result.Status = LoopValidationStatusPending
		result.Reason = "code task requires tests or verification evidence"
		result.UnresolvedItems = append(result.UnresolvedItems, "run tests or verification for code changes")
		result.Issues = append(result.Issues, newValidationIssue("code", "code_verification_missing", "code", result.Status, result.Reason))
	}
	if lang, code, ok := codeSnippetForValidation(output); ok && v.codeValidator != nil {
		warnings := v.codeValidator.Validate(lang, code)
		if len(warnings) > 0 {
			result.Status = worseValidationStatus(result.Status, LoopValidationStatusPending)
			result.RemainingRisks = append(result.RemainingRisks, warnings...)
			result.Issues = append(result.Issues, newValidationIssue("code", "code_risk_detected", "code", LoopValidationStatusPending, strings.Join(warnings, "; ")))
		}
	}
	finalizeLoopValidationResult(result)
	return result, nil
}

func hasAcceptanceCriteria(input *Input) bool {
	if input == nil || len(input.Context) == 0 {
		return false
	}
	raw, ok := input.Context["acceptance_criteria"]
	if !ok || raw == nil {
		return false
	}
	switch typed := raw.(type) {
	case string:
		return strings.TrimSpace(typed) != ""
	case []string:
		return len(typed) > 0
	case []any:
		return len(typed) > 0
	default:
		return true
	}
}

func newLoopValidationResult(status LoopValidationStatus, reason string) *LoopValidationResult {
	result := &LoopValidationResult{
		Status:   status,
		Reason:   strings.TrimSpace(reason),
		Metadata: map[string]any{},
	}
	finalizeLoopValidationResult(result)
	return result
}

func mergeLoopValidationResult(target *LoopValidationResult, incoming *LoopValidationResult) {
	if target == nil || incoming == nil {
		return
	}
	finalizeLoopValidationResult(incoming)
	target.Status = worseValidationStatus(target.Status, incoming.Status)
	target.AcceptanceCriteria = normalizeStringSlice(append(target.AcceptanceCriteria, incoming.AcceptanceCriteria...))
	target.UnresolvedItems = normalizeStringSlice(append(target.UnresolvedItems, incoming.UnresolvedItems...))
	target.RemainingRisks = normalizeStringSlice(append(target.RemainingRisks, incoming.RemainingRisks...))
	target.Issues = append(target.Issues, incoming.Issues...)
	if strings.TrimSpace(target.Reason) == "" || incoming.Status == LoopValidationStatusFailed || (incoming.Status == LoopValidationStatusPending && target.Status != LoopValidationStatusFailed) {
		target.Reason = incoming.Reason
	}
	if target.Metadata == nil {
		target.Metadata = map[string]any{}
	}
	for key, value := range incoming.Metadata {
		target.Metadata[key] = value
	}
}

func finalizeLoopValidationResult(result *LoopValidationResult) {
	if result == nil {
		return
	}
	result.AcceptanceCriteria = normalizeStringSlice(result.AcceptanceCriteria)
	result.UnresolvedItems = normalizeStringSlice(result.UnresolvedItems)
	result.RemainingRisks = normalizeStringSlice(result.RemainingRisks)
	switch result.Status {
	case LoopValidationStatusFailed, LoopValidationStatusPending, LoopValidationStatusPassed:
	default:
		result.Status = LoopValidationStatusPassed
	}
	result.Passed = result.Status == LoopValidationStatusPassed
	result.Pending = result.Status == LoopValidationStatusPending
	if result.Summary == "" {
		result.Summary = summarizeValidationState(result.Status, result.UnresolvedItems, result.RemainingRisks)
	}
	if strings.TrimSpace(result.Reason) == "" {
		result.Reason = result.Summary
	}
	if result.Metadata == nil {
		result.Metadata = map[string]any{}
	}
	result.Metadata["validation_status"] = string(result.Status)
	result.Metadata["validation_passed"] = result.Passed
	result.Metadata["validation_pending"] = result.Pending
	result.Metadata["validation_reason"] = result.Reason
	result.Metadata["validation_summary"] = result.Summary
	result.Metadata["acceptance_criteria"] = cloneStringSlice(result.AcceptanceCriteria)
	result.Metadata["unresolved_items"] = cloneStringSlice(result.UnresolvedItems)
	result.Metadata["remaining_risks"] = cloneStringSlice(result.RemainingRisks)
	if len(result.Issues) > 0 {
		result.Metadata["validation_issues"] = append([]LoopValidationIssue(nil), result.Issues...)
	}
}

func worseValidationStatus(left, right LoopValidationStatus) LoopValidationStatus {
	if validationStatusRank(right) > validationStatusRank(left) {
		return right
	}
	if left == "" {
		return LoopValidationStatusPassed
	}
	return left
}

func validationStatusRank(status LoopValidationStatus) int {
	switch status {
	case LoopValidationStatusFailed:
		return 3
	case LoopValidationStatusPending:
		return 2
	case LoopValidationStatusPassed:
		return 1
	default:
		return 0
	}
}

func acceptanceCriteriaForValidation(input *Input, state *LoopState) []string {
	if state != nil && len(state.AcceptanceCriteria) > 0 {
		return cloneStringSlice(state.AcceptanceCriteria)
	}
	if input == nil || len(input.Context) == 0 {
		return nil
	}
	if values, ok := loopContextStrings(input.Context, "acceptance_criteria"); ok {
		return values
	}
	return nil
}

func unresolvedItemsForValidation(state *LoopState, output *Output) []string {
	var items []string
	if state != nil {
		items = append(items, state.UnresolvedItems...)
	}
	if output != nil {
		if values, ok := loopContextStrings(output.Metadata, "unresolved_items", "remaining_work"); ok {
			items = append(items, values...)
		}
	}
	return normalizeStringSlice(items)
}

func remainingRisksForValidation(state *LoopState, output *Output) []string {
	var risks []string
	if state != nil {
		risks = append(risks, state.RemainingRisks...)
	}
	if output != nil {
		if values, ok := loopContextStrings(output.Metadata, "remaining_risks"); ok {
			risks = append(risks, values...)
		}
	}
	return normalizeStringSlice(risks)
}

func appendUniqueString(values []string, value string) []string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return values
	}
	for _, existing := range values {
		if strings.EqualFold(strings.TrimSpace(existing), trimmed) {
			return values
		}
	}
	return append(values, trimmed)
}

func fallbackString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func newValidationIssue(validator string, code string, category string, status LoopValidationStatus, message string) LoopValidationIssue {
	return LoopValidationIssue{
		Validator: strings.TrimSpace(validator),
		Code:      strings.TrimSpace(code),
		Category:  strings.TrimSpace(category),
		Status:    status,
		Message:   strings.TrimSpace(message),
	}
}

func fallbackMetadataReason(metadata map[string]any, fallback string) string {
	reason := metadataString(metadata, "validation_reason", "validation_message")
	if reason == "" {
		return fallback
	}
	return reason
}

func toolVerificationRequired(input *Input, state *LoopState, output *Output) bool {
	if contextBool(input, "tool_verification_required") {
		return true
	}
	if output == nil {
		return false
	}
	if metadataBoolTrue(output.Metadata, "tool_verification_required") {
		return true
	}
	if metadataHasAny(output.Metadata, "tool_used", "tool_name", "tool_calls", "tool_results", "search_results", "tool_verification_passed", "tool_verification_pending", "verification_pending", "verification_passed", "verified") {
		return true
	}
	return state != nil && state.SelectedReasoningMode == ReasoningModeReWOO
}

func codeTaskRequired(input *Input, state *LoopState, output *Output) bool {
	if contextBool(input, "code_task") || contextBool(input, "requires_code") {
		return true
	}
	if taskType := strings.ToLower(strings.TrimSpace(contextString(input, "task_type"))); taskType != "" {
		switch taskType {
		case "code", "coding", "implementation", "fix", "bugfix", "refactor":
			return true
		}
	}
	if output != nil && metadataHasAny(output.Metadata, "generated_code", "code_language", "code_verification_passed", "tests_passed", "tests_pending") {
		return true
	}
	goal := ""
	if state != nil {
		goal = state.Goal
	}
	if strings.TrimSpace(goal) == "" && input != nil {
		goal = input.Content
	}
	normalized := strings.ToLower(goal)
	return strings.Contains(normalized, "fix") ||
		strings.Contains(normalized, "bug") ||
		strings.Contains(normalized, "code") ||
		strings.Contains(normalized, "implement") ||
		strings.Contains(normalized, "refactor") ||
		strings.Contains(normalized, "test")
}

func codeSnippetForValidation(output *Output) (CodeValidationLanguage, string, bool) {
	if output == nil || len(output.Metadata) == 0 {
		return "", "", false
	}
	rawCode := metadataString(output.Metadata, "generated_code", "code")
	rawLang := strings.ToLower(metadataString(output.Metadata, "code_language", "language"))
	if rawCode == "" || rawLang == "" {
		return "", "", false
	}
	switch rawLang {
	case "python":
		return CodeLangPython, rawCode, true
	case "javascript", "js":
		return CodeLangJavaScript, rawCode, true
	case "typescript", "ts":
		return CodeLangTypeScript, rawCode, true
	case "go", "golang":
		return CodeLangGo, rawCode, true
	case "rust":
		return CodeLangRust, rawCode, true
	case "bash", "shell", "sh":
		return CodeLangBash, rawCode, true
	default:
		return "", "", false
	}
}

func metadataBool(values map[string]any, keys ...string) (bool, bool) {
	if len(values) == 0 {
		return false, false
	}
	for _, key := range keys {
		raw, ok := values[key]
		if !ok {
			continue
		}
		flag, ok := raw.(bool)
		if ok {
			return flag, true
		}
	}
	return false, false
}

func metadataHasAny(values map[string]any, keys ...string) bool {
	if len(values) == 0 {
		return false
	}
	for _, key := range keys {
		if _, ok := values[key]; ok {
			return true
		}
	}
	return false
}

func metadataBoolTrue(values map[string]any, keys ...string) bool {
	flag, ok := metadataBool(values, keys...)
	return ok && flag
}

func metadataString(values map[string]any, keys ...string) string {
	if len(values) == 0 {
		return ""
	}
	for _, key := range keys {
		raw, ok := values[key]
		if !ok {
			continue
		}
		text, ok := raw.(string)
		if ok && strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text)
		}
	}
	return ""
}

type LoopValidationFuncAdapter LoopValidationFunc

func (f LoopValidationFuncAdapter) Validate(ctx context.Context, input *Input, state *LoopState, output *Output, err error) (*LoopValidationResult, error) {
	return f(ctx, input, state, output, err)
}

// ChatCompletion 调用 LLM 完成对话
func (b *BaseAgent) ChatCompletion(ctx context.Context, messages []types.Message) (*types.ChatResponse, error) {
	pr, err := b.prepareChatRequest(ctx, messages)
	if err != nil {
		return nil, err
	}

	emit, streaming := runtimeStreamEmitterFromContext(ctx)
	if streaming {
		return b.chatCompletionStreaming(ctx, pr, emit)
	}

	if pr.hasTools {
		return b.chatCompletionWithTools(ctx, pr)
	}

	return pr.chatProvider.Completion(ctx, pr.req)
}

// chatCompletionStreaming handles the streaming execution path of ChatCompletion.
// 支持 Steering：通过 context 中的 SteeringChannel 接收实时引导/停止后发送指令。
func (b *BaseAgent) chatCompletionStreaming(ctx context.Context, pr *preparedRequest, emit RuntimeStreamEmitter) (*types.ChatResponse, error) {
	steerCh, _ := SteeringChannelFromContext(ctx)
	reactIterationBudget := reactToolLoopBudget(pr)
	ctx = WithRuntimeConversationMessages(ctx, pr.req.Messages)

	if pr.hasTools {
		return b.chatCompletionStreamingWithTools(ctx, pr, emit, steerCh, reactIterationBudget)
	}
	return b.chatCompletionStreamingDirect(ctx, pr, emit, steerCh)
}

type reactStreamingState struct {
	final            *types.ChatResponse
	currentIteration int
	selectedMode     string
}

func (b *BaseAgent) chatCompletionStreamingWithTools(ctx context.Context, pr *preparedRequest, emit RuntimeStreamEmitter, steerCh *SteeringChannel, reactIterationBudget int) (*types.ChatResponse, error) {
	state, eventCh, err := b.startReactStreaming(ctx, pr, steerCh, reactIterationBudget, emit)
	if err != nil {
		return nil, err
	}
	for ev := range eventCh {
		if err := b.handleReactStreamEvent(emit, pr, state, ev); err != nil {
			return nil, err
		}
	}
	if state.final == nil {
		return nil, ErrNoResponse
	}
	return state.final, nil
}

func (b *BaseAgent) startReactStreaming(ctx context.Context, pr *preparedRequest, steerCh *SteeringChannel, reactIterationBudget int, emit RuntimeStreamEmitter) (*reactStreamingState, <-chan llmtools.ReActStreamEvent, error) {
	const selectedMode = ReasoningModeReact
	reactReq := *pr.req
	reactReq.Model = effectiveToolModel(pr.req.Model, pr.options.Tools.ToolModel)
	ctx = withRuntimeApprovalEmitter(ctx, emit, pr)
	toolProtocol := b.toolProtocolRuntime().Prepare(b, pr)
	executor := llmtools.NewReActExecutor(
		pr.toolProvider,
		toolProtocol.Executor,
		llmtools.ReActConfig{MaxIterations: reactIterationBudget, StopOnError: false},
		b.logger,
	)
	if steerCh != nil {
		executor.SetSteeringChannel(steerCh.Receive())
	}
	eventCh, err := executor.ExecuteStream(ctx, &reactReq)
	if err != nil {
		return nil, nil, err
	}
	emitRuntimeStatus(emit, "reasoning_mode_selected", RuntimeStreamEvent{
		Timestamp:      time.Now(),
		CurrentStage:   "reasoning",
		IterationCount: 0,
		SelectedMode:   selectedMode,
		Data: map[string]any{
			"mode":                   selectedMode,
			"react_iteration_budget": reactIterationBudget,
		},
	})
	return &reactStreamingState{selectedMode: selectedMode}, eventCh, nil
}

func (b *BaseAgent) handleReactStreamEvent(emit RuntimeStreamEmitter, pr *preparedRequest, state *reactStreamingState, ev llmtools.ReActStreamEvent) error {
	switch ev.Type {
	case llmtools.ReActEventIterationStart:
		state.currentIteration = ev.Iteration
	case llmtools.ReActEventLLMChunk:
		emitReactLLMChunk(emit, state, ev)
	case llmtools.ReActEventToolsStart:
		emitReactToolCalls(emit, pr, state, ev.ToolCalls)
	case llmtools.ReActEventToolsEnd:
		emitReactToolResults(emit, pr, state, ev.ToolResults)
	case llmtools.ReActEventToolProgress:
		emit(RuntimeStreamEvent{
			Type:           RuntimeStreamToolProgress,
			Timestamp:      time.Now(),
			ToolCallID:     ev.ToolCallID,
			ToolName:       ev.ToolName,
			Data:           ev.ProgressData,
			CurrentStage:   "acting",
			IterationCount: state.currentIteration,
			SelectedMode:   state.selectedMode,
		})
	case llmtools.ReActEventSteering:
		emit(RuntimeStreamEvent{
			Type:            RuntimeStreamSteering,
			Timestamp:       time.Now(),
			SteeringContent: ev.SteeringContent,
			CurrentStage:    "reasoning",
			IterationCount:  state.currentIteration,
			SelectedMode:    state.selectedMode,
		})
	case llmtools.ReActEventStopAndSend:
		emit(RuntimeStreamEvent{
			Type:           RuntimeStreamStopAndSend,
			Timestamp:      time.Now(),
			CurrentStage:   "reasoning",
			IterationCount: state.currentIteration,
			SelectedMode:   state.selectedMode,
		})
	case llmtools.ReActEventCompleted:
		state.final = ev.FinalResponse
		emitReactCompletion(emit, state)
	case llmtools.ReActEventError:
		stopReason := string(classifyStopReason(ev.Error))
		emitCompletionLoopStatus(emit, state.currentIteration, state.selectedMode, stopReason)
		return NewErrorWithCause(types.ErrAgentExecution, "streaming execution error", errors.New(ev.Error))
	}
	return nil
}

func emitReactLLMChunk(emit RuntimeStreamEmitter, state *reactStreamingState, ev llmtools.ReActStreamEvent) {
	if ev.Chunk == nil {
		return
	}
	if ev.Chunk.Delta.Content != "" {
		emit(RuntimeStreamEvent{
			Type:           RuntimeStreamToken,
			Timestamp:      time.Now(),
			Token:          ev.Chunk.Delta.Content,
			Delta:          ev.Chunk.Delta.Content,
			SDKEventType:   SDKRawResponseEvent,
			CurrentStage:   "reasoning",
			IterationCount: state.currentIteration,
			SelectedMode:   state.selectedMode,
		})
	}
	if ev.Chunk.Delta.ReasoningContent != nil && *ev.Chunk.Delta.ReasoningContent != "" {
		emit(RuntimeStreamEvent{
			Type:           RuntimeStreamReasoning,
			Timestamp:      time.Now(),
			Reasoning:      *ev.Chunk.Delta.ReasoningContent,
			SDKEventType:   SDKRawResponseEvent,
			CurrentStage:   "reasoning",
			IterationCount: state.currentIteration,
			SelectedMode:   state.selectedMode,
		})
	}
}

func emitReactToolCalls(emit RuntimeStreamEmitter, pr *preparedRequest, state *reactStreamingState, calls []types.ToolCall) {
	for _, call := range calls {
		sdkEventName := SDKToolCalled
		if runtimeHandoffToolRequested(pr, call.Name) {
			sdkEventName = SDKHandoffRequested
		}
		emit(RuntimeStreamEvent{
			Type:           RuntimeStreamToolCall,
			Timestamp:      time.Now(),
			CurrentStage:   "acting",
			IterationCount: state.currentIteration,
			SelectedMode:   state.selectedMode,
			ToolCall: &RuntimeToolCall{
				ID:        call.ID,
				Name:      call.Name,
				Arguments: append(json.RawMessage(nil), call.Arguments...),
			},
			SDKEventType: SDKRunItemEvent,
			SDKEventName: sdkEventName,
		})
	}
}

func emitReactToolResults(emit RuntimeStreamEmitter, pr *preparedRequest, state *reactStreamingState, results []types.ToolResult) {
	for _, tr := range results {
		sdkEventName, resultPayload := reactToolResultPayload(pr, tr)
		emitApprovalRuntimeEventFromToolResult(emit, pr, state, tr)
		emit(RuntimeStreamEvent{
			Type:           RuntimeStreamToolResult,
			Timestamp:      time.Now(),
			CurrentStage:   "acting",
			IterationCount: state.currentIteration,
			SelectedMode:   state.selectedMode,
			ToolResult: &RuntimeToolResult{
				ToolCallID: tr.ToolCallID,
				Name:       tr.Name,
				Result:     resultPayload,
				Error:      tr.Error,
				Duration:   tr.Duration,
			},
			SDKEventType: SDKRunItemEvent,
			SDKEventName: sdkEventName,
		})
	}
}

func withRuntimeApprovalEmitter(ctx context.Context, emit RuntimeStreamEmitter, pr *preparedRequest) context.Context {
	if emit == nil {
		return ctx
	}
	return llmtools.WithPermissionEventEmitter(ctx, func(event llmtools.PermissionEvent) {
		emitRuntimeApprovalEvent(emit, pr, event)
	})
}

func withApprovalExplainabilityEmitter(ctx context.Context, recorder ExplainabilityRecorder, traceID string) context.Context {
	if recorder == nil || strings.TrimSpace(traceID) == "" {
		return ctx
	}
	return llmtools.WithPermissionEventEmitter(ctx, func(event llmtools.PermissionEvent) {
		content := strings.TrimSpace(event.Reason)
		if content == "" {
			content = string(event.Type)
		}
		metadata := map[string]any{
			"approval_type": event.Type,
			"approval_id":   event.ApprovalID,
			"decision":      string(event.Decision),
			"tool_name":     event.ToolName,
			"rule_id":       event.RuleID,
		}
		if len(event.Metadata) > 0 {
			for key, value := range event.Metadata {
				metadata[key] = value
			}
		}
		recorder.AddExplainabilityStep(traceID, "approval", content, metadata)
		if timelineRecorder, ok := recorder.(ExplainabilityTimelineRecorder); ok {
			timelineRecorder.AddExplainabilityTimeline(traceID, "approval", content, metadata)
		}
	})
}

func emitRuntimeApprovalEvent(emit RuntimeStreamEmitter, pr *preparedRequest, event llmtools.PermissionEvent) {
	if emit == nil {
		return
	}
	sdkEventName := SDKApprovalResponse
	if event.Type == llmtools.PermissionEventRequested {
		sdkEventName = SDKApprovalRequested
	}
	data := map[string]any{
		"approval_type": event.Type,
		"decision":      string(event.Decision),
		"reason":        event.Reason,
		"approval_id":   event.ApprovalID,
	}
	if len(event.Metadata) > 0 {
		for key, value := range event.Metadata {
			data[key] = value
		}
	}
	if risk := toolRiskForPreparedRequest(pr, event.ToolName, event.Metadata); risk != "" {
		data["hosted_tool_risk"] = risk
	}
	emit(RuntimeStreamEvent{
		Type:         RuntimeStreamApproval,
		SDKEventType: SDKRunItemEvent,
		SDKEventName: sdkEventName,
		Timestamp:    time.Now(),
		ToolName:     event.ToolName,
		Data:         data,
	})
}

func emitApprovalRuntimeEventFromToolResult(emit RuntimeStreamEmitter, pr *preparedRequest, state *reactStreamingState, tr types.ToolResult) {
	if emit == nil {
		return
	}
	risk := toolRiskForPreparedRequest(pr, tr.Name, nil)
	if risk != toolRiskRequiresApproval {
		return
	}
	approvalID, required := parseApprovalRequiredError(tr.Error)
	if required {
		emit(RuntimeStreamEvent{
			Type:           RuntimeStreamApproval,
			SDKEventType:   SDKRunItemEvent,
			SDKEventName:   SDKApprovalRequested,
			Timestamp:      time.Now(),
			ToolName:       tr.Name,
			CurrentStage:   "acting",
			IterationCount: state.currentIteration,
			SelectedMode:   state.selectedMode,
			Data: map[string]any{
				"approval_type":    "approval_requested",
				"approval_id":      approvalID,
				"hosted_tool_risk": risk,
				"reason":           tr.Error,
			},
		})
		return
	}
	if tr.Error == "" {
		emit(RuntimeStreamEvent{
			Type:           RuntimeStreamApproval,
			SDKEventType:   SDKRunItemEvent,
			SDKEventName:   SDKApprovalResponse,
			Timestamp:      time.Now(),
			ToolName:       tr.Name,
			CurrentStage:   "acting",
			IterationCount: state.currentIteration,
			SelectedMode:   state.selectedMode,
			Data: map[string]any{
				"approval_type":    "approval_granted",
				"approved":         true,
				"hosted_tool_risk": risk,
			},
		})
	}
}

func parseApprovalRequiredError(errText string) (string, bool) {
	trimmed := strings.TrimSpace(errText)
	if !strings.HasPrefix(trimmed, "approval required") {
		return "", false
	}
	const prefix = "approval required (ID: "
	if strings.HasPrefix(trimmed, prefix) {
		rest := strings.TrimPrefix(trimmed, prefix)
		if idx := strings.Index(rest, ")"); idx >= 0 {
			return strings.TrimSpace(rest[:idx]), true
		}
	}
	return "", true
}

func toolRiskForPreparedRequest(pr *preparedRequest, toolName string, metadata map[string]string) string {
	if metadata != nil {
		if risk := strings.TrimSpace(metadata["hosted_tool_risk"]); risk != "" {
			return risk
		}
	}
	if pr != nil && len(pr.toolRisks) > 0 {
		if risk, ok := pr.toolRisks[strings.TrimSpace(toolName)]; ok {
			return risk
		}
	}
	return classifyToolRiskByName(toolName)
}

func reactToolResultPayload(pr *preparedRequest, tr types.ToolResult) (SDKRunItemEventName, json.RawMessage) {
	sdkEventName := SDKToolOutput
	resultPayload := append(json.RawMessage(nil), tr.Result...)
	if runtimeHandoffToolRequested(pr, tr.Name) {
		sdkEventName = SDKHandoffOccured
		if control := tr.Control(); control != nil && control.Handoff != nil {
			if raw, err := json.Marshal(control.Handoff); err == nil {
				resultPayload = raw
			}
		}
	}
	return sdkEventName, resultPayload
}

func emitReactCompletion(emit RuntimeStreamEmitter, state *reactStreamingState) {
	final := state.final
	if emit != nil && final != nil && len(final.Choices) > 0 && final.Choices[0].Message.Content != "" {
		emit(RuntimeStreamEvent{
			Type:           RuntimeStreamStatus,
			SDKEventType:   SDKRunItemEvent,
			SDKEventName:   SDKMessageOutputCreated,
			Timestamp:      time.Now(),
			CurrentStage:   "responding",
			IterationCount: state.currentIteration,
			SelectedMode:   state.selectedMode,
			Data: map[string]any{
				"content": final.Choices[0].Message.Content,
			},
		})
	}
	stopReason := normalizeRuntimeStopReasonFromResponse(final)
	emitCompletionLoopStatus(emit, state.currentIteration, state.selectedMode, stopReason)
}

type directStreamingAttemptResult struct {
	assembled        types.Message
	lastID           string
	lastProvider     string
	lastModel        string
	lastUsage        *types.ChatUsage
	lastFinishReason string
	reasoning        string
	steering         *SteeringMessage
}

func (b *BaseAgent) chatCompletionStreamingDirect(ctx context.Context, pr *preparedRequest, emit RuntimeStreamEmitter, steerCh *SteeringChannel) (*types.ChatResponse, error) {
	messages := append([]types.Message(nil), pr.req.Messages...)
	var cumulativeUsage types.ChatUsage
	emitRuntimeStatus(emit, "reasoning_mode_selected", RuntimeStreamEvent{
		Timestamp:      time.Now(),
		CurrentStage:   "responding",
		IterationCount: 1,
	})

	for {
		attempt, err := b.runDirectStreamingAttempt(ctx, pr, messages, emit, steerCh)
		if err != nil {
			return nil, err
		}
		accumulateChatUsage(&cumulativeUsage, attempt.lastUsage)
		if attempt.steering == nil || attempt.steering.IsZero() {
			return finalizeDirectStreamingResponse(emit, attempt, cumulativeUsage), nil
		}
		emitDirectSteeringEvent(emit, attempt.steering)
		messages = types.ApplySteeringToMessages(*attempt.steering, messages, attempt.assembled.Content, attempt.reasoning, types.RoleAssistant)
	}
}

func (b *BaseAgent) runDirectStreamingAttempt(ctx context.Context, pr *preparedRequest, messages []types.Message, emit RuntimeStreamEmitter, steerCh *SteeringChannel) (*directStreamingAttemptResult, error) {
	streamCtx, cancelStream := context.WithCancel(ctx)
	defer cancelStream()
	pr.req.Messages = messages
	streamCh, err := pr.chatProvider.Stream(streamCtx, pr.req)
	if err != nil {
		return nil, err
	}
	result := &directStreamingAttemptResult{}
	var reasoningBuf strings.Builder

	// 空闲超时机制：只要还在收到数据，就重置计时器
	inactivityTimer := time.NewTimer(DefaultStreamInactivityTimeout)
	defer inactivityTimer.Stop()

chunkLoop:
	for {
		select {
		case chunk, ok := <-streamCh:
			if !ok {
				break chunkLoop
			}
			// 收到数据，重置空闲超时计时器
			if !inactivityTimer.Stop() {
				select {
				case <-inactivityTimer.C:
				default:
				}
			}
			inactivityTimer.Reset(DefaultStreamInactivityTimeout)

			if chunk.Err != nil {
				return nil, chunk.Err
			}
			consumeDirectStreamChunk(emit, result, &reasoningBuf, chunk)
		case msg := <-steerChOrNil(steerCh):
			result.steering = &msg
			cancelStream()
			for range streamCh {
			}
			break chunkLoop
		case <-inactivityTimer.C:
			// 空闲超时：超过 DefaultStreamInactivityTimeout 没有收到新数据
			cancelStream()
			b.logger.Warn("stream inactivity timeout",
				zap.Duration("timeout", DefaultStreamInactivityTimeout),
			)
			return nil, NewError(types.ErrAgentExecution, "stream inactivity timeout after "+DefaultStreamInactivityTimeout.String()+" (no data received)")
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	result.reasoning = reasoningBuf.String()
	return result, nil
}

func consumeDirectStreamChunk(emit RuntimeStreamEmitter, result *directStreamingAttemptResult, reasoningBuf *strings.Builder, chunk types.StreamChunk) {
	if chunk.ID != "" {
		result.lastID = chunk.ID
	}
	if chunk.Provider != "" {
		result.lastProvider = chunk.Provider
	}
	if chunk.Model != "" {
		result.lastModel = chunk.Model
	}
	if chunk.Usage != nil {
		result.lastUsage = chunk.Usage
	}
	if chunk.FinishReason != "" {
		result.lastFinishReason = chunk.FinishReason
	}
	if chunk.Delta.Content != "" {
		emit(RuntimeStreamEvent{
			Type:           RuntimeStreamToken,
			Timestamp:      time.Now(),
			Token:          chunk.Delta.Content,
			Delta:          chunk.Delta.Content,
			SDKEventType:   SDKRawResponseEvent,
			CurrentStage:   "responding",
			IterationCount: 1,
		})
		result.assembled.Content += chunk.Delta.Content
	}
	if chunk.Delta.ReasoningContent != nil && *chunk.Delta.ReasoningContent != "" {
		emit(RuntimeStreamEvent{
			Type:           RuntimeStreamReasoning,
			Timestamp:      time.Now(),
			Reasoning:      *chunk.Delta.ReasoningContent,
			SDKEventType:   SDKRawResponseEvent,
			CurrentStage:   "responding",
			IterationCount: 1,
		})
		reasoningBuf.WriteString(*chunk.Delta.ReasoningContent)
	}
	if len(chunk.Delta.ReasoningSummaries) > 0 {
		result.assembled.ReasoningSummaries = append(result.assembled.ReasoningSummaries, chunk.Delta.ReasoningSummaries...)
	}
	if len(chunk.Delta.OpaqueReasoning) > 0 {
		result.assembled.OpaqueReasoning = append(result.assembled.OpaqueReasoning, chunk.Delta.OpaqueReasoning...)
	}
	if len(chunk.Delta.ThinkingBlocks) > 0 {
		result.assembled.ThinkingBlocks = append(result.assembled.ThinkingBlocks, chunk.Delta.ThinkingBlocks...)
	}
}

func accumulateChatUsage(total, usage *types.ChatUsage) {
	if usage == nil || total == nil {
		return
	}
	total.PromptTokens += usage.PromptTokens
	total.CompletionTokens += usage.CompletionTokens
	total.TotalTokens += usage.TotalTokens
}

func finalizeDirectStreamingResponse(emit RuntimeStreamEmitter, attempt *directStreamingAttemptResult, cumulativeUsage types.ChatUsage) *types.ChatResponse {
	if attempt.reasoning != "" {
		rc := attempt.reasoning
		attempt.assembled.ReasoningContent = &rc
	}
	attempt.assembled.Role = types.RoleAssistant
	resp := &types.ChatResponse{
		ID:       attempt.lastID,
		Provider: attempt.lastProvider,
		Model:    attempt.lastModel,
		Choices: []types.ChatChoice{{
			Index:        0,
			FinishReason: attempt.lastFinishReason,
			Message:      attempt.assembled,
		}},
		Usage: cumulativeUsage,
	}
	if emit != nil && attempt.assembled.Content != "" {
		emit(RuntimeStreamEvent{
			Type:           RuntimeStreamStatus,
			SDKEventType:   SDKRunItemEvent,
			SDKEventName:   SDKMessageOutputCreated,
			Timestamp:      time.Now(),
			CurrentStage:   "responding",
			IterationCount: 1,
			Data: map[string]any{
				"content": attempt.assembled.Content,
			},
		})
	}
	emitCompletionLoopStatus(emit, 1, "", normalizeRuntimeStopReason(attempt.lastFinishReason))
	return resp
}

func emitDirectSteeringEvent(emit RuntimeStreamEmitter, steering *SteeringMessage) {
	switch steering.Type {
	case SteeringTypeGuide:
		emit(RuntimeStreamEvent{
			Type:            RuntimeStreamSteering,
			Timestamp:       time.Now(),
			SteeringContent: steering.Content,
			CurrentStage:    "responding",
			IterationCount:  1,
		})
	case SteeringTypeStopAndSend:
		emit(RuntimeStreamEvent{
			Type:           RuntimeStreamStopAndSend,
			Timestamp:      time.Now(),
			CurrentStage:   "responding",
			IterationCount: 1,
		})
	}
}

// chatCompletionWithTools executes a non-streaming ReAct loop with tools.
func (b *BaseAgent) chatCompletionWithTools(ctx context.Context, pr *preparedRequest) (*types.ChatResponse, error) {
	ctx = WithRuntimeConversationMessages(ctx, pr.req.Messages)
	reactReq := *pr.req
	reactReq.Model = effectiveToolModel(pr.req.Model, pr.options.Tools.ToolModel)
	reactIterationBudget := reactToolLoopBudget(pr)
	toolProtocol := b.toolProtocolRuntime().Prepare(b, pr)
	executor := llmtools.NewReActExecutor(
		pr.toolProvider,
		toolProtocol.Executor,
		llmtools.ReActConfig{MaxIterations: reactIterationBudget, StopOnError: false},
		b.logger,
	)
	resp, _, err := executor.Execute(ctx, &reactReq)
	if err != nil {
		return resp, NewErrorWithCause(types.ErrAgentExecution, "ReAct execution failed", err)
	}
	return resp, nil
}

func reactToolLoopBudget(pr *preparedRequest) int {
	if pr != nil && pr.maxReActIter > 0 {
		return pr.maxReActIter
	}
	return 1
}

// StreamCompletion 流式调用 LLM
func (b *BaseAgent) StreamCompletion(ctx context.Context, messages []types.Message) (<-chan types.StreamChunk, error) {
	pr, err := b.prepareChatRequest(ctx, messages)
	if err != nil {
		return nil, err
	}
	return pr.chatProvider.Stream(ctx, pr.req)
}

// =============================================================================
// Runtime Stream Events
// =============================================================================

type runtimeStreamEmitterKey struct{}

// RuntimeStreamEventType identifies the kind of runtime stream event.
type RuntimeStreamEventType string
type SDKStreamEventType string
type SDKRunItemEventName string

const (
	RuntimeStreamToken        RuntimeStreamEventType = "token"
	RuntimeStreamReasoning    RuntimeStreamEventType = "reasoning"
	RuntimeStreamToolCall     RuntimeStreamEventType = "tool_call"
	RuntimeStreamToolResult   RuntimeStreamEventType = "tool_result"
	RuntimeStreamToolProgress RuntimeStreamEventType = "tool_progress"
	RuntimeStreamApproval     RuntimeStreamEventType = "approval"
	RuntimeStreamSession      RuntimeStreamEventType = "session"
	RuntimeStreamStatus       RuntimeStreamEventType = "status"
	RuntimeStreamSteering     RuntimeStreamEventType = "steering"
	RuntimeStreamStopAndSend  RuntimeStreamEventType = "stop_and_send"
)

const (
	SDKRawResponseEvent  SDKStreamEventType = "raw_response_event"
	SDKRunItemEvent      SDKStreamEventType = "run_item_stream_event"
	SDKAgentUpdatedEvent SDKStreamEventType = "agent_updated_stream_event"
)

const (
	SDKMessageOutputCreated SDKRunItemEventName = "message_output_created"
	SDKHandoffRequested     SDKRunItemEventName = "handoff_requested"
	SDKToolCalled           SDKRunItemEventName = "tool_called"
	SDKToolSearchCalled     SDKRunItemEventName = "tool_search_called"
	SDKToolSearchOutput     SDKRunItemEventName = "tool_search_output_created"
	SDKToolOutput           SDKRunItemEventName = "tool_output"
	SDKReasoningCreated     SDKRunItemEventName = "reasoning_item_created"
	SDKApprovalRequested    SDKRunItemEventName = "approval_requested"
	SDKApprovalResponse     SDKRunItemEventName = "approval_response"
	SDKMCPApprovalRequested SDKRunItemEventName = "mcp_approval_requested"
	SDKMCPApprovalResponse  SDKRunItemEventName = "mcp_approval_response"
	SDKMCPListTools         SDKRunItemEventName = "mcp_list_tools"
)

var SDKHandoffOccured = SDKRunItemEventName(handoffOccuredEventName())

func handoffOccuredEventName() string {
	return string([]byte{'h', 'a', 'n', 'd', 'o', 'f', 'f', '_', 'o', 'c', 'c', 'u', 'r', 'e', 'd'})
}

// RuntimeToolCall carries tool invocation metadata in a stream event.
type RuntimeToolCall struct {
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// RuntimeToolResult carries tool execution results in a stream event.
type RuntimeToolResult struct {
	ToolCallID string          `json:"tool_call_id,omitempty"`
	Name       string          `json:"name"`
	Result     json.RawMessage `json:"result,omitempty"`
	Error      string          `json:"error,omitempty"`
	Duration   time.Duration   `json:"duration,omitempty"`
}

// RuntimeStreamEvent is a single event emitted during streamed Agent execution.
type RuntimeStreamEvent struct {
	Type            RuntimeStreamEventType `json:"type"`
	SDKEventType    SDKStreamEventType     `json:"sdk_event_type,omitempty"`
	SDKEventName    SDKRunItemEventName    `json:"sdk_event_name,omitempty"`
	Timestamp       time.Time              `json:"timestamp"`
	Token           string                 `json:"token,omitempty"`
	Delta           string                 `json:"delta,omitempty"`
	Reasoning       string                 `json:"reasoning,omitempty"`
	ToolCall        *RuntimeToolCall       `json:"tool_call,omitempty"`
	ToolResult      *RuntimeToolResult     `json:"tool_result,omitempty"`
	ToolCallID      string                 `json:"tool_call_id,omitempty"`
	ToolName        string                 `json:"tool_name,omitempty"`
	Data            any                    `json:"data,omitempty"`
	SteeringContent string                 `json:"steering_content,omitempty"` // steering 确认内容
	CurrentStage    string                 `json:"current_stage,omitempty"`
	IterationCount  int                    `json:"iteration_count,omitempty"`
	SelectedMode    string                 `json:"selected_reasoning_mode,omitempty"`
	StopReason      string                 `json:"stop_reason,omitempty"`
	CheckpointID    string                 `json:"checkpoint_id,omitempty"`
	Resumable       bool                   `json:"resumable,omitempty"`
}

// RuntimeStreamEmitter is a callback that receives runtime stream events.
type RuntimeStreamEmitter func(RuntimeStreamEvent)

// WithRuntimeStreamEmitter stores an emitter in the context.
func WithRuntimeStreamEmitter(ctx context.Context, emit RuntimeStreamEmitter) context.Context {
	if emit == nil {
		return ctx
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, runtimeStreamEmitterKey{}, emit)
}

func runtimeStreamEmitterFromContext(ctx context.Context) (RuntimeStreamEmitter, bool) {
	if ctx == nil {
		return nil, false
	}
	v := ctx.Value(runtimeStreamEmitterKey{})
	if v == nil {
		return nil, false
	}
	emit, ok := v.(RuntimeStreamEmitter)
	return emit, ok && emit != nil
}

func emitRuntimeStatus(emit RuntimeStreamEmitter, status string, event RuntimeStreamEvent) {
	if emit == nil {
		return
	}
	event.Type = RuntimeStreamStatus
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	if event.Data == nil {
		event.Data = map[string]any{"status": status}
	} else if payload, ok := event.Data.(map[string]any); ok {
		if _, exists := payload["status"]; !exists {
			payload["status"] = status
		}
		event.Data = payload
	}
	emit(event)
}

func emitCompletionLoopStatus(emit RuntimeStreamEmitter, iteration int, selectedMode, stopReason string) {
	normalizedStopReason := normalizeTopLevelStopReason(stopReason, stopReason)
	emitRuntimeStatus(emit, "completion_judge_decision", RuntimeStreamEvent{
		Timestamp:      time.Now(),
		CurrentStage:   "evaluate",
		IterationCount: iteration,
		SelectedMode:   selectedMode,
		StopReason:     normalizedStopReason,
		Data: map[string]any{
			"decision":            "done",
			"solved":              normalizedStopReason == string(StopReasonSolved),
			"internal_stop_cause": stopReason,
		},
	})
	emitRuntimeStatus(emit, "loop_stopped", RuntimeStreamEvent{
		Timestamp:      time.Now(),
		CurrentStage:   "completed",
		IterationCount: iteration,
		SelectedMode:   selectedMode,
		StopReason:     normalizedStopReason,
		Data: map[string]any{
			"state":               "stopped",
			"internal_stop_cause": stopReason,
		},
	})
}

func normalizeRuntimeStopReasonFromResponse(resp *types.ChatResponse) string {
	if resp == nil || len(resp.Choices) == 0 {
		return normalizeRuntimeStopReason("")
	}
	return normalizeRuntimeStopReason(resp.Choices[0].FinishReason)
}

func normalizeRuntimeStopReason(finishReason string) string {
	normalized := strings.TrimSpace(finishReason)
	if normalized == "" {
		return string(StopReasonSolved)
	}
	return normalizeTopLevelStopReason(normalized, normalized)
}

func runtimeHandoffTargetsFromPreparedRequest(pr *preparedRequest) []RuntimeHandoffTarget {
	if pr == nil || len(pr.handoffTools) == 0 {
		return nil
	}
	out := make([]RuntimeHandoffTarget, 0, len(pr.handoffTools))
	for _, target := range pr.handoffTools {
		out = append(out, target)
	}
	return out
}

func runtimeHandoffToolRequested(pr *preparedRequest, toolName string) bool {
	if pr == nil || len(pr.handoffTools) == 0 {
		return false
	}
	_, ok := pr.handoffTools[toolName]
	return ok
}

// Merged from react.go.

// defaultCostCalc is a package-level cost calculator for estimating LLM call costs.
var defaultCostCalc = observability.NewCostCalculator()

const submitNumberedPlanTool = "submit_numbered_plan"

type numberedPlanSubmission struct {
	Steps []string `json:"steps"`
}

func numberedPlanToolSchema() types.ToolSchema {
	strict := true
	return types.ToolSchema{
		Type:        types.ToolTypeFunction,
		Name:        submitNumberedPlanTool,
		Description: "Submit the ordered execution steps for the task.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"steps": {
					"type": "array",
					"items": {"type": "string"},
					"minItems": 1,
					"description": "Ordered execution steps."
				}
			},
			"required": ["steps"],
			"additionalProperties": false
		}`),
		Strict: &strict,
	}
}

func parseNumberedPlanToolCall(message types.Message) ([]string, error) {
	for _, call := range message.ToolCalls {
		if call.Name != submitNumberedPlanTool {
			continue
		}
		var submission numberedPlanSubmission
		if err := json.Unmarshal(call.Arguments, &submission); err != nil {
			return nil, fmt.Errorf("decode numbered plan tool call: %w", err)
		}
		if len(submission.Steps) == 0 {
			return nil, fmt.Errorf("numbered plan tool call did not include steps")
		}
		return submission.Steps, nil
	}
	return nil, fmt.Errorf("numbered plan tool call not found")
}

// Plan 生成执行计划
// 使用 LLM 分析任务并生成详细的执行步骤
func (b *BaseAgent) Plan(ctx context.Context, input *Input) (*PlanResult, error) {
	if !b.hasMainExecutionSurface() {
		return nil, ErrProviderNotSet
	}

	// 构建规划提示词
	planPrompt := fmt.Sprintf(`Plan the execution of this task for another agent.

Task:
%s

Use the %s tool to return the plan.

Requirements:
- Keep each step directly executable
- Prefer tool-first actions when tools are needed
- Mention dependencies or risks only when they affect execution
- Do not answer with prose outside the tool call`, input.Content, submitNumberedPlanTool)

	messages := []types.Message{
		{
			Role:    types.RoleSystem,
			Content: b.promptBundle.RenderSystemPromptWithVars(input.Variables),
		},
		{
			Role:    types.RoleUser,
			Content: planPrompt,
		},
	}

	pr, err := b.prepareChatRequest(ctx, messages)
	if err != nil {
		return nil, err
	}
	nativeToolSupport := pr.chatProvider.SupportsNativeFunctionCalling()
	if !nativeToolSupport && b.mainProviderCompat != nil {
		nativeToolSupport = b.mainProviderCompat.SupportsNativeFunctionCalling()
	}
	pr.req.Tools = []types.ToolSchema{numberedPlanToolSchema()}
	pr.req.ToolChoice = &types.ToolChoice{Mode: types.ToolChoiceModeRequired}
	if nativeToolSupport {
		pr.req.ToolCallMode = types.ToolCallModeNative
	} else {
		pr.req.ToolCallMode = types.ToolCallModeXML
	}

	resp, err := pr.chatProvider.Completion(ctx, pr.req)
	if err != nil {
		return nil, NewErrorWithCause(types.ErrAgentExecution, "plan generation failed", err)
	}

	// 解析计划
	if resp == nil || len(resp.Choices) == 0 {
		return nil, NewError(types.ErrLLMResponseEmpty, "plan generation returned no choices")
	}
	choice := resp.FirstChoice()
	steps, parseErr := parseNumberedPlanToolCall(choice.Message)
	if parseErr != nil {
		return nil, NewErrorWithCause(types.ErrAgentExecution, "plan generation did not return tool call", parseErr)
	}
	if len(steps) == 0 {
		return nil, NewError(types.ErrLLMResponseEmpty, "plan generation returned no steps")
	}

	b.logger.Info("plan generated",
		zap.Int("steps", len(steps)),
		zap.String("trace_id", input.TraceID),
	)

	return &PlanResult{
		Steps: steps,
		Metadata: map[string]any{
			"tokens_used": resp.Usage.TotalTokens,
			"model":       resp.Model,
		},
	}, nil
}

// Execute 执行任务（完整的 ReAct 循环）
// 这是 Agent 的核心执行方法，包含完整的推理-行动循环
// Requirements 1.7: 集成输入验证
// Requirements 2.4: 输出验证失败时支持重试
func (b *BaseAgent) Execute(ctx context.Context, input *Input) (_ *Output, execErr error) {
	resumeInput, err := b.prepareResumeInput(ctx, input)
	if err != nil {
		return nil, err
	}
	return b.executeWithPipeline(ctx, resumeInput, b.configuredExecutionOptions())
}

func (b *BaseAgent) executeCore(ctx context.Context, input *Input) (_ *Output, execErr error) {
	if input == nil {
		return nil, NewError(types.ErrInputValidation, "input is nil")
	}
	if strings.TrimSpace(input.Content) == "" {
		return nil, NewError(types.ErrInputValidation, "input content is empty")
	}
	startTime := time.Now()

	// 0a. Inject top-level Input fields into Go context
	if input.TraceID != "" {
		ctx = types.WithTraceID(ctx, input.TraceID)
	}
	if input.TenantID != "" {
		ctx = types.WithTenantID(ctx, input.TenantID)
	}
	if input.UserID != "" {
		ctx = types.WithUserID(ctx, input.UserID)
	}

	// 0b. Inject Input.Context well-known keys into Go context (overrides top-level fields)
	ctx = applyInputContext(ctx, input.Context)

	// 0c. Apply runtime overrides from context and input in a single precedence chain.
	if runConfig := ResolveRunConfig(ctx, input); runConfig != nil {
		ctx = WithRunConfig(ctx, runConfig)
	}

	// 1. 尝试获取执行锁
	if !b.TryLockExec() {
		return nil, ErrAgentBusy
	}
	defer b.UnlockExec()

	// 2. 在锁保护下确保 Agent 就绪
	if err := b.EnsureReady(); err != nil {
		return nil, err
	}

	// 3. 转换状态到运行中（支持并发：首个请求负责转换，后续请求复用 Running 状态）
	if atomic.AddInt64(&b.execCount, 1) == 1 {
		if err := b.Transition(ctx, StateRunning); err != nil {
			atomic.AddInt64(&b.execCount, -1)
			return nil, err
		}
	}

	// 以下操作修改共享状态（b.promptBundle），必须在 execMu 保护下执行。

	// 3a. PromptStore: load active prompt from MongoDB if available (local copy, no shared state mutation)
	activeBundle := b.promptBundle
	if doc := b.persistence.LoadPrompt(ctx, b.config.Core.Type, b.config.Core.Name, ""); doc != nil {
		activeBundle.Version = doc.Version
		activeBundle.System = doc.System
		if len(doc.Constraints) > 0 {
			activeBundle.Constraints = doc.Constraints
		}
		b.logger.Info("loaded prompt from store",
			zap.String("version", doc.Version),
			zap.String("agent_type", b.config.Core.Type),
		)
	}

	// 3b. RunStore: record execution start
	runID := b.persistence.RecordRun(ctx, b.config.Core.ID, input.TenantID, input.TraceID, input.Content, startTime)

	// 3c. ConversationStore: restore conversation history
	conversationID := input.ChannelID
	restoredMessages := b.persistence.RestoreConversation(ctx, conversationID)
	defer func() {
		// Ensure run status is updated on any exit path (including panic).
		if runID != "" {
			if r := recover(); r != nil {
				if updateErr := b.persistence.UpdateRunStatus(ctx, runID, "failed", nil, fmt.Sprintf("panic: %v", r)); updateErr != nil {
					b.logger.Warn("failed to mark run as failed after panic", zap.Error(updateErr))
				}
				b.logger.Error("panic during execution, run marked as failed",
					zap.Any("panic", r),
					zap.Error(panicPayloadToError(r)),
					zap.String("run_id", runID),
				)
				if execErr == nil {
					execErr = NewErrorWithCause(types.ErrAgentExecution, "react execution panic", panicPayloadToError(r))
				}
			}
			if execErr != nil {
				if updateErr := b.persistence.UpdateRunStatus(ctx, runID, "failed", nil, execErr.Error()); updateErr != nil {
					b.logger.Warn("failed to mark run as failed", zap.Error(updateErr))
				}
			}
		}
		// 使用独立 context 确保状态恢复不受原始 ctx 取消影响
		if atomic.AddInt64(&b.execCount, -1) == 0 {
			if err := b.Transition(context.Background(), StateReady); err != nil {
				b.logger.Error("failed to transition to ready", zap.Error(err))
			}
		}
	}()

	b.logger.Info("executing task",
		zap.String("trace_id", input.TraceID),
		zap.String("agent_id", b.config.Core.ID),
		zap.String("agent_type", b.config.Core.Type),
	)

	// 4. 输入验证(监护)
	b.configMu.RLock()
	guardrailsEnabled := b.guardrailsEnabled
	inputValidatorChain := b.inputValidatorChain
	runtimeGuardrailsCfg := b.runtimeGuardrailsCfg
	b.configMu.RUnlock()

	if guardrailsEnabled && inputValidatorChain != nil {
		validationResult, err := inputValidatorChain.Validate(ctx, input.Content)
		if err != nil {
			b.logger.Error("input validation error", zap.Error(err))
			return nil, NewErrorWithCause(types.ErrInputValidation, "input validation error", err)
		}

		if !validationResult.Valid {
			b.logger.Warn("input validation failed",
				zap.String("trace_id", input.TraceID),
				zap.Any("errors", validationResult.Errors),
			)

			// 从配置中检查失败动作
			failureAction := guardrails.FailureActionReject
			if runtimeGuardrailsCfg != nil {
				failureAction = runtimeGuardrailsCfg.OnInputFailure
			}

			switch failureAction {
			case guardrails.FailureActionReject:
				return nil, &GuardrailsError{
					Type:    GuardrailsErrorTypeInput,
					Message: "input validation failed",
					Errors:  validationResult.Errors,
				}
			case guardrails.FailureActionWarn:
				b.logger.Warn("input validation warning, continuing execution",
					zap.Any("warnings", validationResult.Errors),
				)
			}
		}
	}

	// 5. 收集上下文来源并通过 context runtime 组装消息
	memoryContext := b.collectContextMemory(input.Context)
	conversation := restoredMessages
	if handoffMessages := handoffMessagesFromInputContext(input.Context); len(handoffMessages) > 0 {
		conversation = handoffMessages
	}

	systemContent := activeBundle.RenderSystemPromptWithVars(input.Variables)
	if publicCtx := agentcontext.AdditionalContextText(publicInputContext(input.Context)); publicCtx != "" {
		systemContent += "\n\n<additional_context>\n" + publicCtx + "\n</additional_context>"
	}
	skillContext := skillInstructionsFromInputContext(input.Context)
	if len(skillContext) == 0 {
		skillContext = normalizeInstructionList(skillInstructionsFromCtx(ctx))
	}
	publicContext := publicInputContext(input.Context)
	retrievalItems := retrievalItemsFromInputContext(input.Context)
	if len(retrievalItems) == 0 && b.retriever != nil {
		if records, err := b.retriever.Retrieve(ctx, input.Content, 5); err != nil {
			b.logger.Warn("failed to load retrieval context", zap.Error(err))
		} else {
			retrievalItems = retrievalItemsFromRecords(records)
		}
	}
	toolStates := toolStatesFromInputContext(input.Context)
	if len(toolStates) == 0 && b.toolState != nil {
		if snapshots, err := b.toolState.LoadToolState(ctx, b.ID()); err != nil {
			b.logger.Warn("failed to load tool state context", zap.Error(err))
		} else {
			toolStates = toolStatesFromSnapshots(snapshots)
		}
	}

	ephemeralLayers, traceFeedbackPlan := b.buildEphemeralPromptLayers(ctx, publicContext, input, systemContent, skillContext, memoryContext, conversation, retrievalItems, toolStates)
	messages, assembled := b.assembleMessages(ctx, systemContent, ephemeralLayers, skillContext, memoryContext, conversation, retrievalItems, toolStates, input.Content)
	if assembled != nil {
		b.logger.Debug("context assembled",
			zap.Int("tokens_before", assembled.TokensBefore),
			zap.Int("tokens_after", assembled.TokensAfter),
			zap.String("strategy", assembled.Plan.Strategy),
			zap.String("compression_reason", assembled.Plan.CompressionReason),
			zap.Int("applied_layers", len(assembled.Plan.AppliedLayers)),
		)
		if emit, ok := runtimeStreamEmitterFromContext(ctx); ok {
			emitRuntimeStatus(emit, "prompt_layers_built", RuntimeStreamEvent{
				Timestamp:    time.Now(),
				CurrentStage: "context",
				Data: map[string]any{
					"context_plan":   assembled.Plan,
					"applied_layers": assembled.Plan.AppliedLayers,
					"layer_ids":      promptLayerIDs(assembled.Plan.AppliedLayers),
				},
			})
		}
		b.recordPromptLayerTimeline(input.TraceID, assembled.Plan)
	}
	ctx = b.withApprovalExplainability(ctx, input)

	// 7. 执行产出验证和重试支持
	// 要求2.4:对产出验证失败进行重试
	policy := b.loopControlPolicy()
	b.configMu.RLock()
	maxRetries := policy.RetryBudget
	outputValidator := b.outputValidator
	guardrailsEnabledForOutput := b.guardrailsEnabled
	runtimeGuardrailsCfgForOutput := runtimeGuardrailsFromPolicy(policy, b.runtimeGuardrailsCfg)
	b.configMu.RUnlock()

	var resp *types.ChatResponse
	var outputContent string
	var lastValidationResult *guardrails.ValidationResult
	var choice types.ChatChoice

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			b.logger.Info("retrying execution due to output validation failure",
				zap.Int("attempt", attempt),
				zap.String("trace_id", input.TraceID),
			)

			// 为重试添加验证失败的反馈
			if lastValidationResult != nil {
				feedbackMsg := b.buildValidationFeedbackMessage(lastValidationResult)
				messages = append(messages, types.Message{
					Role:    types.RoleUser,
					Content: feedbackMsg,
				})
			}
		}

		// 执行重新行动循环
		var err error
		resp, err = b.ChatCompletion(ctx, messages)
		if err != nil {
			b.logger.Error("execution failed",
				zap.Error(err),
				zap.String("trace_id", input.TraceID),
			)
			return nil, NewErrorWithCause(types.ErrAgentExecution, "execution failed", err)
		}

		if resp == nil || len(resp.Choices) == 0 {
			return nil, NewError(types.ErrLLMResponseEmpty, "execution returned no choices")
		}
		choice = resp.FirstChoice()
		outputContent = choice.Message.Content

		// 产出验证(护栏)
		if guardrailsEnabledForOutput && outputValidator != nil {
			var filteredContent string
			filteredContent, lastValidationResult, err = outputValidator.ValidateAndFilter(ctx, outputContent)
			if err != nil {
				b.logger.Error("output validation error", zap.Error(err))
				return nil, NewErrorWithCause(types.ErrOutputValidation, "output validation error", err)
			}

			if !lastValidationResult.Valid {
				b.logger.Warn("output validation failed",
					zap.String("trace_id", input.TraceID),
					zap.Int("attempt", attempt),
					zap.Any("errors", lastValidationResult.Errors),
				)

				// 检查失败动作
				failureAction := guardrails.FailureActionReject
				if runtimeGuardrailsCfgForOutput != nil {
					failureAction = runtimeGuardrailsCfgForOutput.OnOutputFailure
				}

				// 如果重试已经配置, 我们还没有用尽重试, 请继续
				if failureAction == guardrails.FailureActionRetry && attempt < maxRetries {
					continue
				}

				// 基于失败动作的处理
				switch failureAction {
				case guardrails.FailureActionReject:
					return nil, &GuardrailsError{
						Type:    GuardrailsErrorTypeOutput,
						Message: "output validation failed",
						Errors:  lastValidationResult.Errors,
					}
				case guardrails.FailureActionWarn:
					b.logger.Warn("output validation warning, using filtered content")
					outputContent = filteredContent
				case guardrails.FailureActionRetry:
					// 重复重复,拒绝
					return nil, &GuardrailsError{
						Type:    GuardrailsErrorTypeOutput,
						Message: fmt.Sprintf("output validation failed after %d retries", maxRetries),
						Errors:  lastValidationResult.Errors,
					}
				}
			} else {
				// 通过验证, 使用过滤内容
				outputContent = filteredContent
			}
		}

		// 通过验证或警告模式, 中断重试循环
		break
	}

	estimatedCost := defaultCostCalc.Calculate(resp.Provider, resp.Model, resp.Usage.PromptTokens, resp.Usage.CompletionTokens)

	// 8. 保存记忆/回合观察
	if b.memoryRuntime != nil {
		if err := b.memoryRuntime.ObserveTurn(ctx, b.ID(), MemoryObservationInput{
			TraceID:          input.TraceID,
			UserContent:      input.Content,
			AssistantContent: outputContent,
			Metadata: map[string]any{
				"tokens":        resp.Usage.TotalTokens,
				"cost":          estimatedCost,
				"finish_reason": choice.FinishReason,
			},
		}); err != nil {
			b.logger.Warn("memory runtime observe turn failed", zap.Error(err))
		}
	} else {
		skipBaseMemory := b.memoryFacade != nil && b.memoryFacade.SkipBaseMemory()
		if b.memory != nil && !skipBaseMemory {
			if err := b.SaveMemory(ctx, input.Content, MemoryShortTerm, map[string]any{
				"trace_id": input.TraceID,
				"role":     "user",
			}); err != nil {
				b.logger.Warn("failed to save user input to memory", zap.Error(err))
			}
			if err := b.SaveMemory(ctx, outputContent, MemoryShortTerm, map[string]any{
				"trace_id": input.TraceID,
				"role":     "assistant",
			}); err != nil {
				b.logger.Warn("failed to save response to memory", zap.Error(err))
			}
		}
	}

	duration := time.Since(startTime)

	// 9a. ConversationStore: persist conversation
	b.persistence.PersistConversation(ctx, conversationID, b.config.Core.ID, input.TenantID, input.UserID, input.Content, outputContent)

	// 9b. RunStore: update run status
	if runID != "" {
		outputDoc := &RunOutputDoc{
			Content:      outputContent,
			TokensUsed:   resp.Usage.TotalTokens,
			Cost:         estimatedCost,
			FinishReason: choice.FinishReason,
		}
		if err := b.persistence.UpdateRunStatus(ctx, runID, "completed", outputDoc, ""); err != nil {
			b.logger.Warn("failed to update run status", zap.Error(err))
		}
	}

	b.logger.Info("execution completed",
		zap.String("trace_id", input.TraceID),
		zap.Duration("duration", duration),
		zap.Int("tokens_used", resp.Usage.TotalTokens),
	)

	// 9. 返回结果
	outputMetadata := map[string]any{
		"model":    resp.Model,
		"provider": resp.Provider,
	}
	if assembled != nil {
		outputMetadata["context_plan"] = assembled.Plan
		outputMetadata["applied_prompt_layers"] = assembled.Plan.AppliedLayers
		outputMetadata["applied_prompt_layer_ids"] = promptLayerIDs(assembled.Plan.AppliedLayers)
	}
	outputMetadata["trace_feedback_plan"] = map[string]any{
		"planner_id":           traceFeedbackPlan.PlannerID,
		"planner_version":      traceFeedbackPlan.PlannerVersion,
		"confidence":           traceFeedbackPlan.Confidence,
		"goal":                 traceFeedbackPlan.Goal,
		"recommended_action":   string(traceFeedbackPlan.RecommendedAction),
		"inject_memory_recall": traceFeedbackPlan.InjectMemoryRecall,
		"primary_layer":        traceFeedbackPlan.PrimaryLayer,
		"secondary_layer":      traceFeedbackPlan.SecondaryLayer,
		"selected_layers":      cloneStringSlice(traceFeedbackPlan.SelectedLayers),
		"suppressed_layers":    cloneStringSlice(traceFeedbackPlan.SuppressedLayers),
		"reasons":              cloneStringSlice(traceFeedbackPlan.Reasons),
		"signals": map[string]any{
			"has_prior_synopsis":        traceFeedbackPlan.Signals.HasPriorSynopsis,
			"has_compressed_history":    traceFeedbackPlan.Signals.HasCompressedHistory,
			"resume":                    traceFeedbackPlan.Signals.Resume,
			"handoff":                   traceFeedbackPlan.Signals.Handoff,
			"multi_agent":               traceFeedbackPlan.Signals.MultiAgent,
			"verification":              traceFeedbackPlan.Signals.Verification,
			"complex_task":              traceFeedbackPlan.Signals.ComplexTask,
			"context_pressure":          traceFeedbackPlan.Signals.ContextPressure,
			"usage_ratio":               traceFeedbackPlan.Signals.UsageRatio,
			"acceptance_criteria_count": traceFeedbackPlan.Signals.AcceptanceCriteriaCount,
			"compressed_event_count":    traceFeedbackPlan.Signals.CompressedEventCount,
		},
		"metadata": cloneAnyMap(traceFeedbackPlan.Metadata),
		"summary":  traceFeedbackPlan.Summary,
	}
	if extraMetadata, ok := choice.Message.Metadata.(map[string]any); ok {
		for key, value := range extraMetadata {
			outputMetadata[key] = value
		}
	}
	return &Output{
		TraceID:          input.TraceID,
		Content:          outputContent,
		ReasoningContent: choice.Message.ReasoningContent,
		Metadata:         outputMetadata,
		TokensUsed:       resp.Usage.TotalTokens,
		Cost:             estimatedCost,
		Duration:         duration,
		FinishReason:     choice.FinishReason,
	}, nil
}

func (b *BaseAgent) withApprovalExplainability(ctx context.Context, input *Input) context.Context {
	recorder, ok := b.extensions.ObservabilitySystemExt().(ExplainabilityRecorder)
	if !ok || input == nil {
		return ctx
	}
	return withApprovalExplainabilityEmitter(ctx, recorder, strings.TrimSpace(input.TraceID))
}

func (b *BaseAgent) recordPromptLayerTimeline(traceID string, plan agentcontext.ContextPlan) {
	recorder, ok := b.extensions.ObservabilitySystemExt().(ExplainabilityTimelineRecorder)
	if !ok || strings.TrimSpace(traceID) == "" {
		return
	}
	recorder.AddExplainabilityTimeline(traceID, "prompt_layers", "Prompt layers assembled for this request", map[string]any{
		"context_plan":   plan,
		"applied_layers": plan.AppliedLayers,
		"layer_ids":      promptLayerIDs(plan.AppliedLayers),
	})
}

// 构建 ValidationFeedBackMessage 为重试创建回馈消息
func (b *BaseAgent) buildValidationFeedbackMessage(result *guardrails.ValidationResult) string {
	var sb strings.Builder
	sb.WriteString("Your previous response failed validation. Please regenerate your response addressing the following issues:\n")
	for _, err := range result.Errors {
		sb.WriteString(fmt.Sprintf("- %s: %s\n", err.Code, err.Message))
	}
	sb.WriteString("\nPlease provide a corrected response.")
	return sb.String()
}

func panicPayloadToError(v any) error {
	if err, ok := v.(error); ok {
		return err
	}
	return fmt.Errorf("panic: %v", v)
}

// Observe 处理反馈并更新 Agent 状态
// 这个方法允许 Agent 从外部反馈中学习和改进
func (b *BaseAgent) Observe(ctx context.Context, feedback *Feedback) error {
	b.logger.Info("observing feedback",
		zap.String("agent_id", b.config.Core.ID),
		zap.String("feedback_type", feedback.Type),
	)

	metadata := map[string]any{
		"feedback_type": feedback.Type,
		"timestamp":     time.Now(),
	}
	for k, v := range feedback.Data {
		metadata[k] = v
	}

	// 1. 保存反馈到记忆系统
	switch {
	case b.memoryFacade != nil && b.memoryFacade.HasEnhanced():
		if err := b.memoryFacade.Enhanced().SaveShortTerm(ctx, b.ID(), feedback.Content, metadata); err != nil {
			b.logger.Error("failed to save feedback to enhanced memory", zap.Error(err))
			return NewErrorWithCause(types.ErrAgentExecution, "failed to save feedback", err)
		}
		b.memoryFacade.RecordEpisode(ctx, &types.EpisodicEvent{
			AgentID:   b.ID(),
			Type:      "feedback",
			Content:   feedback.Content,
			Context:   metadata,
			Timestamp: time.Now(),
		})
	case b.memory != nil:
		if err := b.SaveMemory(ctx, feedback.Content, MemoryLongTerm, metadata); err != nil {
			b.logger.Error("failed to save feedback to memory", zap.Error(err))
			return NewErrorWithCause(types.ErrAgentExecution, "failed to save feedback", err)
		}
	}

	// 2. 发布反馈事件
	if b.bus != nil {
		b.bus.Publish(&FeedbackEvent{
			AgentID_:     b.config.Core.ID,
			FeedbackType: feedback.Type,
			Content:      feedback.Content,
			Data:         feedback.Data,
			Timestamp_:   time.Now(),
		})
	}

	b.logger.Info("feedback observed successfully",
		zap.String("agent_id", b.config.Core.ID),
		zap.String("feedback_type", feedback.Type),
	)

	return nil
}

func (b *BaseAgent) collectContextMemory(values map[string]any) []string {
	var memoryContext []string
	appendValue := func(v string) {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			memoryContext = append(memoryContext, trimmed)
		}
	}

	b.recentMemoryMu.RLock()
	for _, mem := range b.recentMemory {
		if mem.Kind == MemoryShortTerm {
			appendValue(mem.Content)
		}
	}
	b.recentMemoryMu.RUnlock()

	if b.memoryFacade != nil {
		for _, item := range b.memoryFacade.LoadContext(context.Background(), b.ID()) {
			appendValue(item)
		}
	}

	if len(values) > 0 {
		if raw, ok := values["memory_context"].([]string); ok {
			for _, item := range raw {
				appendValue(item)
			}
		}
	}
	return memoryContext
}

func (b *BaseAgent) assembleMessages(
	ctx context.Context,
	systemPrompt string,
	ephemeralLayers []agentcontext.PromptLayer,
	skillContext []string,
	memoryContext []string,
	conversation []types.Message,
	retrieval []agentcontext.RetrievalItem,
	toolStates []agentcontext.ToolState,
	userInput string,
) ([]types.Message, *agentcontext.AssembleResult) {
	if manager, ok := b.contextManager.(interface {
		Assemble(context.Context, *agentcontext.AssembleRequest) (*agentcontext.AssembleResult, error)
	}); ok {
		result, err := manager.Assemble(ctx, &agentcontext.AssembleRequest{
			SystemPrompt:    systemPrompt,
			EphemeralLayers: ephemeralLayers,
			SkillContext:    skillContext,
			MemoryContext:   memoryContext,
			Conversation:    conversation,
			Retrieval:       retrieval,
			ToolState:       toolStates,
			UserInput:       userInput,
			Query:           userInput,
		})
		if err == nil && result != nil && len(result.Messages) > 0 {
			return result.Messages, result
		}
		if err != nil {
			b.logger.Warn("context assembly failed, falling back to legacy message construction", zap.Error(err))
		}
	}

	msgCap := 1 + len(ephemeralLayers) + len(skillContext) + len(memoryContext) + len(conversation) + 1
	messages := make([]types.Message, 0, msgCap)
	if strings.TrimSpace(systemPrompt) != "" {
		messages = append(messages, types.Message{Role: types.RoleSystem, Content: systemPrompt})
	}
	for _, layer := range ephemeralLayers {
		if strings.TrimSpace(layer.Content) == "" {
			continue
		}
		role := layer.Role
		if role == "" {
			role = types.RoleSystem
		}
		messages = append(messages, types.Message{Role: role, Content: layer.Content, Metadata: layer.Metadata})
	}
	for _, item := range skillContext {
		if strings.TrimSpace(item) == "" {
			continue
		}
		messages = append(messages, types.Message{Role: types.RoleSystem, Content: item})
	}
	for _, item := range memoryContext {
		messages = append(messages, types.Message{Role: types.RoleSystem, Content: item})
	}
	messages = append(messages, conversation...)
	messages = append(messages, types.Message{Role: types.RoleUser, Content: userInput})
	return messages, nil
}

func (b *BaseAgent) buildEphemeralPromptLayers(
	ctx context.Context,
	publicContext map[string]any,
	input *Input,
	systemPrompt string,
	skillContext []string,
	memoryContext []string,
	conversation []types.Message,
	retrieval []agentcontext.RetrievalItem,
	toolStates []agentcontext.ToolState,
) ([]agentcontext.PromptLayer, TraceFeedbackPlan) {
	if b.ephemeralPrompt == nil {
		return nil, TraceFeedbackPlan{}
	}
	status := b.estimateContextStatus(systemPrompt, skillContext, memoryContext, conversation, retrieval, toolStates, input)
	snapshot := b.latestTraceSynopsisSnapshot(input)
	plan := b.selectTraceFeedbackPlan(input, status, snapshot)
	checkpointID := ""
	if input != nil && input.Context != nil {
		if value, ok := input.Context["checkpoint_id"].(string); ok {
			checkpointID = strings.TrimSpace(value)
		}
	}
	b.recordTraceFeedbackDecision(input.TraceID, plan, status)
	layers := b.ephemeralPrompt.Build(EphemeralPromptLayerInput{
		PublicContext:            publicContext,
		TraceID:                  strings.TrimSpace(input.TraceID),
		TenantID:                 strings.TrimSpace(input.TenantID),
		UserID:                   strings.TrimSpace(input.UserID),
		ChannelID:                strings.TrimSpace(input.ChannelID),
		TraceFeedbackPlan:        &plan,
		TraceSynopsis:            conditionalTraceSynopsis(plan.InjectSynopsis, snapshot),
		TraceHistorySummary:      conditionalTraceHistory(plan.InjectHistory, snapshot),
		TraceHistoryEventCount:   conditionalTraceHistoryCount(plan.InjectHistory, snapshot),
		CheckpointID:             checkpointID,
		AllowedTools:             b.effectivePromptToolNames(ctx),
		ToolsDisabled:            promptToolsDisabled(ctx),
		AcceptanceCriteria:       acceptanceCriteriaForValidation(input, nil),
		ToolVerificationRequired: toolVerificationRequired(input, nil, nil),
		CodeVerificationRequired: codeTaskRequired(input, nil, nil),
		ContextStatus:            status,
	})
	if b.memoryRuntime != nil && plan.InjectMemoryRecall {
		recallLayers, err := b.memoryRuntime.RecallForPrompt(ctx, b.ID(), MemoryRecallOptions{
			Query:  input.Content,
			Status: status,
			TopK:   3,
		})
		if err != nil {
			b.logger.Warn("memory runtime recall failed", zap.Error(err))
		} else if len(recallLayers) > 0 {
			layers = append(layers, recallLayers...)
		}
	}
	return layers, plan
}

func (b *BaseAgent) estimateContextStatus(
	systemPrompt string,
	skillContext []string,
	memoryContext []string,
	conversation []types.Message,
	retrieval []agentcontext.RetrievalItem,
	toolStates []agentcontext.ToolState,
	input *Input,
) *agentcontext.Status {
	if b.contextManager == nil {
		return nil
	}
	messages := make([]types.Message, 0, 1+len(skillContext)+len(memoryContext)+len(conversation)+len(retrieval)+len(toolStates)+1)
	if strings.TrimSpace(systemPrompt) != "" {
		messages = append(messages, types.Message{Role: types.RoleSystem, Content: systemPrompt})
	}
	for _, item := range skillContext {
		if strings.TrimSpace(item) != "" {
			messages = append(messages, types.Message{Role: types.RoleSystem, Content: item})
		}
	}
	for _, item := range memoryContext {
		if strings.TrimSpace(item) != "" {
			messages = append(messages, types.Message{Role: types.RoleSystem, Content: item})
		}
	}
	messages = append(messages, conversation...)
	for _, item := range retrieval {
		if strings.TrimSpace(item.Content) != "" {
			messages = append(messages, types.Message{Role: types.RoleSystem, Content: item.Content})
		}
	}
	for _, item := range toolStates {
		if strings.TrimSpace(item.Summary) != "" {
			messages = append(messages, types.Message{Role: types.RoleSystem, Content: item.Summary})
		}
	}
	if input != nil && strings.TrimSpace(input.Content) != "" {
		messages = append(messages, types.Message{Role: types.RoleUser, Content: input.Content})
	}
	status := b.contextManager.GetStatus(messages)
	return &status
}

func (b *BaseAgent) selectTraceFeedbackPlan(input *Input, status *agentcontext.Status, snapshot ExplainabilitySynopsisSnapshot) TraceFeedbackPlan {
	planner := b.traceFeedbackPlanner
	if planner == nil {
		planner = NewComposedTraceFeedbackPlanner(NewRuleBasedTraceFeedbackPlanner(), NewHintTraceFeedbackAdapter())
	}
	sessionID := ""
	traceID := ""
	if input != nil {
		sessionID = strings.TrimSpace(input.ChannelID)
		traceID = strings.TrimSpace(input.TraceID)
	}
	if sessionID == "" {
		sessionID = traceID
	}
	return planner.Plan(&TraceFeedbackPlanningInput{
		AgentID:   b.ID(),
		TraceID:   traceID,
		SessionID: sessionID,
		UserInput: input,
		Signals:   collectTraceFeedbackSignals(input, status, snapshot, b.memoryRuntime != nil),
		Snapshot:  snapshot,
		Config:    TraceFeedbackConfigFromAgentConfig(b.config),
	})
}

func (b *BaseAgent) latestTraceSynopsis(input *Input) string {
	snapshot := b.latestTraceSynopsisSnapshot(input)
	if strings.TrimSpace(snapshot.Synopsis) != "" {
		return strings.TrimSpace(snapshot.Synopsis)
	}
	reader, ok := b.extensions.ObservabilitySystemExt().(ExplainabilitySynopsisReader)
	if !ok || input == nil {
		return ""
	}
	sessionID := strings.TrimSpace(input.ChannelID)
	if sessionID == "" {
		sessionID = strings.TrimSpace(input.TraceID)
	}
	return strings.TrimSpace(reader.GetLatestExplainabilitySynopsis(sessionID, b.ID(), strings.TrimSpace(input.TraceID)))
}

func (b *BaseAgent) latestTraceHistorySummary(input *Input) string {
	return strings.TrimSpace(b.latestTraceSynopsisSnapshot(input).CompressedHistory)
}

func (b *BaseAgent) latestTraceHistoryEventCount(input *Input) int {
	return b.latestTraceSynopsisSnapshot(input).CompressedEventCount
}

func (b *BaseAgent) latestTraceSynopsisSnapshot(input *Input) ExplainabilitySynopsisSnapshot {
	reader, ok := b.extensions.ObservabilitySystemExt().(ExplainabilitySynopsisSnapshotReader)
	if !ok || input == nil {
		return ExplainabilitySynopsisSnapshot{}
	}
	sessionID := strings.TrimSpace(input.ChannelID)
	if sessionID == "" {
		sessionID = strings.TrimSpace(input.TraceID)
	}
	return reader.GetLatestExplainabilitySynopsisSnapshot(sessionID, b.ID(), strings.TrimSpace(input.TraceID))
}

func conditionalTraceSynopsis(enabled bool, snapshot ExplainabilitySynopsisSnapshot) string {
	if !enabled {
		return ""
	}
	return strings.TrimSpace(snapshot.Synopsis)
}

func conditionalTraceHistory(enabled bool, snapshot ExplainabilitySynopsisSnapshot) string {
	if !enabled {
		return ""
	}
	return strings.TrimSpace(snapshot.CompressedHistory)
}

func conditionalTraceHistoryCount(enabled bool, snapshot ExplainabilitySynopsisSnapshot) int {
	if !enabled {
		return 0
	}
	return snapshot.CompressedEventCount
}

func (b *BaseAgent) recordTraceFeedbackDecision(traceID string, plan TraceFeedbackPlan, status *agentcontext.Status) {
	recorder, ok := b.extensions.ObservabilitySystemExt().(ExplainabilityTimelineRecorder)
	if !ok || strings.TrimSpace(traceID) == "" {
		return
	}
	metadata := map[string]any{
		"inject_synopsis":         plan.InjectSynopsis,
		"inject_history":          plan.InjectHistory,
		"inject_memory_recall":    plan.InjectMemoryRecall,
		"score":                   plan.Score,
		"synopsis_threshold":      plan.SynopsisThreshold,
		"history_threshold":       plan.HistoryThreshold,
		"memory_recall_threshold": plan.MemoryRecallThreshold,
		"reasons":                 cloneStringSlice(plan.Reasons),
		"selected_layers":         cloneStringSlice(plan.SelectedLayers),
		"suppressed_layers":       cloneStringSlice(plan.SuppressedLayers),
		"goal":                    plan.Goal,
		"recommended_action":      string(plan.RecommendedAction),
		"primary_layer":           plan.PrimaryLayer,
		"secondary_layer":         plan.SecondaryLayer,
		"planner_id":              plan.PlannerID,
		"planner_version":         plan.PlannerVersion,
		"confidence":              plan.Confidence,
		"planner_metadata":        cloneAnyMap(plan.Metadata),
		"signals": map[string]any{
			"has_prior_synopsis":        plan.Signals.HasPriorSynopsis,
			"has_compressed_history":    plan.Signals.HasCompressedHistory,
			"resume":                    plan.Signals.Resume,
			"handoff":                   plan.Signals.Handoff,
			"multi_agent":               plan.Signals.MultiAgent,
			"verification":              plan.Signals.Verification,
			"complex_task":              plan.Signals.ComplexTask,
			"context_pressure":          plan.Signals.ContextPressure,
			"usage_ratio":               plan.Signals.UsageRatio,
			"acceptance_criteria_count": plan.Signals.AcceptanceCriteriaCount,
			"compressed_event_count":    plan.Signals.CompressedEventCount,
		},
	}
	if status != nil {
		metadata["usage_ratio"] = status.UsageRatio
		metadata["pressure_level"] = status.Level.String()
	}
	recorder.AddExplainabilityTimeline(traceID, "trace_feedback_decision", plan.Summary, metadata)
}

func (b *BaseAgent) effectivePromptToolNames(ctx context.Context) []string {
	rc := GetRunConfig(ctx)
	if rc != nil && rc.DisableTools {
		return nil
	}
	var names []string
	if b.toolManager != nil {
		for _, schema := range b.toolManager.GetAllowedTools(b.config.Core.ID) {
			names = append(names, schema.Name)
		}
	}
	if rc != nil && len(rc.ToolWhitelist) > 0 {
		names = filterStringWhitelist(names, rc.ToolWhitelist)
	} else if allowed := b.config.ExecutionOptions().Tools.AllowedTools; len(allowed) > 0 {
		names = filterStringWhitelist(names, allowed)
	}
	for _, target := range runtimeHandoffTargetsFromContext(ctx, b.config.Core.ID) {
		names = append(names, runtimeHandoffToolSchema(target).Name)
	}
	return normalizeStringSlice(names)
}

func promptToolsDisabled(ctx context.Context) bool {
	rc := GetRunConfig(ctx)
	return rc != nil && rc.DisableTools
}

func filterStringWhitelist(values []string, whitelist []string) []string {
	if len(values) == 0 || len(whitelist) == 0 {
		return normalizeStringSlice(values)
	}
	allowed := make(map[string]struct{}, len(whitelist))
	for _, value := range whitelist {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		allowed[trimmed] = struct{}{}
	}
	if len(allowed) == 0 {
		return nil
	}
	filtered := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := allowed[strings.TrimSpace(value)]; ok {
			filtered = append(filtered, value)
		}
	}
	return normalizeStringSlice(filtered)
}

func promptLayerIDs(layers []agentcontext.PromptLayerMeta) []string {
	if len(layers) == 0 {
		return nil
	}
	ids := make([]string, 0, len(layers))
	for _, layer := range layers {
		if trimmed := strings.TrimSpace(layer.ID); trimmed != "" {
			ids = append(ids, trimmed)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	return ids
}

func retrievalItemsFromInputContext(values map[string]any) []agentcontext.RetrievalItem {
	if len(values) == 0 {
		return nil
	}
	raw, ok := values["retrieval_context"]
	if !ok {
		return nil
	}
	items, ok := raw.([]agentcontext.RetrievalItem)
	if !ok {
		return nil
	}
	return append([]agentcontext.RetrievalItem(nil), items...)
}

func skillInstructionsFromInputContext(values map[string]any) []string {
	if len(values) == 0 {
		return nil
	}
	raw, ok := values["skill_context"]
	if !ok {
		return nil
	}
	items, ok := raw.([]string)
	if !ok {
		return nil
	}
	return normalizeInstructionList(items)
}

func retrievalItemsFromRecords(records []types.RetrievalRecord) []agentcontext.RetrievalItem {
	if len(records) == 0 {
		return nil
	}
	items := make([]agentcontext.RetrievalItem, 0, len(records))
	for _, record := range records {
		if strings.TrimSpace(record.Content) == "" {
			continue
		}
		items = append(items, agentcontext.RetrievalItem{
			Title:   record.DocID,
			Content: record.Content,
			Source:  record.Source,
			Score:   record.Score,
		})
	}
	return items
}

func toolStatesFromInputContext(values map[string]any) []agentcontext.ToolState {
	if len(values) == 0 {
		return nil
	}
	raw, ok := values["tool_state"]
	if !ok {
		return nil
	}
	items, ok := raw.([]agentcontext.ToolState)
	if !ok {
		return nil
	}
	return append([]agentcontext.ToolState(nil), items...)
}

func toolStatesFromSnapshots(items []types.ToolStateSnapshot) []agentcontext.ToolState {
	if len(items) == 0 {
		return nil
	}
	out := make([]agentcontext.ToolState, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.Summary) == "" {
			continue
		}
		out = append(out, agentcontext.ToolState{
			ToolName:   item.ToolName,
			Summary:    item.Summary,
			ArtifactID: item.ArtifactID,
		})
	}
	return out
}

// applyInputContext injects well-known keys from Input.Context into the Go context
// using the corresponding types.WithXxx functions. Unknown keys are silently ignored.
func applyInputContext(ctx context.Context, inputCtx map[string]any) context.Context {
	if len(inputCtx) == 0 {
		return ctx
	}
	for k, v := range inputCtx {
		switch k {
		case "trace_id":
			if s, ok := v.(string); ok && s != "" {
				ctx = types.WithTraceID(ctx, s)
			}
		case "tenant_id":
			if s, ok := v.(string); ok && s != "" {
				ctx = types.WithTenantID(ctx, s)
			}
		case "user_id":
			if s, ok := v.(string); ok && s != "" {
				ctx = types.WithUserID(ctx, s)
			}
		case "run_id":
			if s, ok := v.(string); ok && s != "" {
				ctx = types.WithRunID(ctx, s)
			}
		case "parent_run_id":
			if s, ok := v.(string); ok && s != "" {
				ctx = types.WithParentRunID(ctx, s)
			}
		case "span_id":
			if s, ok := v.(string); ok && s != "" {
				ctx = types.WithSpanID(ctx, s)
			}
		case "agent_id":
			if s, ok := v.(string); ok && s != "" {
				ctx = types.WithAgentID(ctx, s)
			}
		case "llm_model":
			if s, ok := v.(string); ok && s != "" {
				ctx = types.WithLLMModel(ctx, s)
			}
		case "llm_provider":
			if s, ok := v.(string); ok && s != "" {
				ctx = types.WithLLMProvider(ctx, s)
			}
		case "llm_route_policy":
			if s, ok := v.(string); ok && s != "" {
				ctx = types.WithLLMRoutePolicy(ctx, s)
			}
		case "prompt_bundle_version":
			if s, ok := v.(string); ok && s != "" {
				ctx = types.WithPromptBundleVersion(ctx, s)
			}
		case "roles":
			if roles, ok := v.([]string); ok && len(roles) > 0 {
				ctx = types.WithRoles(ctx, roles)
			}
			// Also handle []any (common from JSON deserialization)
			if arr, ok := v.([]any); ok && len(arr) > 0 {
				roles := make([]string, 0, len(arr))
				for _, item := range arr {
					if s, ok := item.(string); ok {
						roles = append(roles, s)
					}
				}
				if len(roles) > 0 {
					ctx = types.WithRoles(ctx, roles)
				}
			}
		}
	}
	return ctx
}

func (b *BaseAgent) prepareResumeInput(ctx context.Context, input *Input) (*Input, error) {
	if input == nil || b.checkpointManager == nil {
		return input, nil
	}
	checkpointID, resumeLatest := resumeDirective(input)
	if checkpointID == "" && !resumeLatest {
		return input, nil
	}

	var (
		checkpoint *Checkpoint
		err        error
	)
	if checkpointID != "" {
		checkpoint, err = b.checkpointManager.LoadCheckpoint(ctx, checkpointID)
	} else {
		threadID := resumeThreadID(input, b.ID())
		checkpoint, err = b.checkpointManager.LoadLatestCheckpoint(ctx, threadID)
	}
	if err != nil {
		return nil, err
	}
	if checkpoint != nil && checkpoint.AgentID != "" && checkpoint.AgentID != b.ID() {
		return nil, NewError(types.ErrInputValidation,
			fmt.Sprintf("checkpoint agent ID mismatch: checkpoint belongs to %s, current agent is %s", checkpoint.AgentID, b.ID()))
	}
	return mergeInputWithCheckpoint(input, checkpoint), nil
}

func resumeDirective(input *Input) (string, bool) {
	if input == nil || len(input.Context) == 0 {
		return "", false
	}
	if checkpointID, ok := input.Context["checkpoint_id"].(string); ok && strings.TrimSpace(checkpointID) != "" {
		return strings.TrimSpace(checkpointID), true
	}
	if enabled, ok := input.Context["resume_latest"].(bool); ok && enabled {
		return "", true
	}
	if enabled, ok := input.Context["resume"].(bool); ok && enabled {
		return "", true
	}
	return "", false
}

func resumeThreadID(input *Input, fallbackAgentID string) string {
	if input == nil {
		return fallbackAgentID
	}
	if threadID := strings.TrimSpace(input.ChannelID); threadID != "" {
		return threadID
	}
	if traceID := strings.TrimSpace(input.TraceID); traceID != "" {
		return traceID
	}
	return fallbackAgentID
}

func mergeInputWithCheckpoint(input *Input, checkpoint *Checkpoint) *Input {
	merged := shallowCopyInput(input)
	if merged.Context == nil {
		merged.Context = make(map[string]any)
	}
	if checkpoint == nil {
		return merged
	}

	if strings.TrimSpace(merged.ChannelID) == "" {
		merged.ChannelID = checkpoint.ThreadID
	}
	merged.Context["checkpoint_id"] = checkpoint.ID
	merged.Context["resume_from_checkpoint"] = true
	merged.Context["resumable"] = true

	for key, value := range checkpoint.Metadata {
		merged.Context[key] = value
	}
	if checkpoint.ExecutionContext != nil {
		if strings.TrimSpace(checkpoint.ExecutionContext.CurrentNode) != "" {
			merged.Context["current_stage"] = checkpoint.ExecutionContext.CurrentNode
		}
		for key, value := range checkpoint.ExecutionContext.Variables {
			merged.Context[key] = value
		}
	}
	if strings.TrimSpace(merged.Content) == "" {
		if goal, ok := merged.Context["goal"].(string); ok && strings.TrimSpace(goal) != "" {
			merged.Content = goal
		}
	}
	return merged
}

// Merged from loop_executor.go.

type LoopExecutor struct {
	MaxIterations     int
	ExecutionOptions  types.ExecutionOptions
	Planner           LoopPlannerFunc
	StepExecutor      LoopStepExecutorFunc
	Observer          LoopObserveFunc
	Validator         LoopValidationFunc
	Selector          ReasoningModeSelector
	ReasoningRuntime  ReasoningRuntime
	Judge             CompletionJudge
	ReflectionStep    LoopReflectionFunc
	ReasoningRegistry *reasoning.PatternRegistry
	ReflectionEnabled bool
	CheckpointManager *CheckpointManager
	Explainability    ExplainabilityTimelineRecorder
	TraceID           string
	AgentID           string
	Logger            *zap.Logger
}

func (e *LoopExecutor) Execute(ctx context.Context, input *Input) (*Output, error) {
	if input == nil {
		return nil, NewError("LOOP_INPUT_NIL", "loop input is nil")
	}
	if e.StepExecutor == nil && e.ReasoningRuntime == nil {
		return nil, NewError("LOOP_STEP_EXECUTOR_MISSING", "loop step executor is required")
	}
	state := e.initialState(ctx, input)
	logger := e.logger()
	judge := e.judge()
	options := e.executionOptions()
	needPlan := e.Planner != nil && !options.Control.DisablePlanner
	e.emitStatus(ctx, state, RuntimeStreamStatus, nil)
	for {
		if err := ctx.Err(); err != nil {
			state.AdvanceStage(LoopStageEvaluate)
			state.MarkStopped(StopReasonTimeout, LoopDecisionDone)
			return e.finalize(state, state.LastOutput, err)
		}
		if state.Iteration >= state.MaxIterations {
			state.AdvanceStage(LoopStageEvaluate)
			state.MarkStopped(StopReasonMaxIterations, LoopDecisionDone)
			return e.finalize(state, state.LastOutput, nil)
		}
		state.Iteration++
		state.AdvanceStage(LoopStagePerceive)
		e.emitStatus(ctx, state, RuntimeStreamStatus, map[string]any{"status": "stage_changed"})
		state.AddObservation(LoopObservation{Stage: LoopStagePerceive, Content: strings.TrimSpace(input.Content), Iteration: state.Iteration})
		state.AdvanceStage(LoopStageAnalyze)
		e.emitStatus(ctx, state, RuntimeStreamStatus, map[string]any{"status": "stage_changed"})
		selection := e.selectReasoning(ctx, input, state)
		state.SelectedReasoningMode = selection.Mode
		state.AddObservation(LoopObservation{Stage: LoopStageAnalyze, Content: selection.Mode, Iteration: state.Iteration, Metadata: map[string]any{"reasoning_mode": selection.Mode}})
		e.emitStatus(ctx, state, RuntimeStreamStatus, map[string]any{"status": "reasoning_mode_selected", "selected_reasoning_mode": selection.Mode})
		if needPlan {
			state.AdvanceStage(LoopStagePlan)
			e.emitStatus(ctx, state, RuntimeStreamStatus, map[string]any{"status": "stage_changed"})
			planResult, err := e.Planner(ctx, input, state)
			if err != nil {
				state.AddObservation(LoopObservation{Stage: LoopStagePlan, Iteration: state.Iteration, Error: err.Error()})
				state.MarkStopped(classifyStopReason(err.Error()), LoopDecisionDone)
				return e.finalize(state, state.LastOutput, err)
			}
			if planResult == nil || len(planResult.Steps) == 0 {
				state.Plan = nil
				state.AddObservation(LoopObservation{Stage: LoopStagePlan, Content: "plan_skipped", Iteration: state.Iteration})
			} else {
				state.Plan = append([]string(nil), planResult.Steps...)
				state.SyncCurrentStep()
				state.AddObservation(LoopObservation{Stage: LoopStagePlan, Content: "plan_ready", Iteration: state.Iteration, Metadata: map[string]any{"steps": len(planResult.Steps)}})
			}
			needPlan = false
		}
		state.AdvanceStage(LoopStageAct)
		state.SyncCurrentStep()
		e.emitStatus(ctx, state, RuntimeStreamStatus, map[string]any{"status": "stage_changed"})
		output, execErr := e.executeReasoning(ctx, input, state, selection)
		state.LastOutput = output
		if output != nil {
			if strings.TrimSpace(output.CheckpointID) != "" {
				state.CheckpointID = output.CheckpointID
			}
			state.Resumable = state.Resumable || output.Resumable
			state.AddObservation(LoopObservation{Stage: LoopStageAct, Content: output.Content, Iteration: state.Iteration, Metadata: cloneMetadata(output.Metadata)})
		} else if execErr == nil {
			state.AddObservation(LoopObservation{Stage: LoopStageAct, Iteration: state.Iteration, Content: "empty_output"})
		}
		if execErr != nil {
			state.AddObservation(LoopObservation{Stage: LoopStageAct, Iteration: state.Iteration, Error: execErr.Error()})
		}
		state.AdvanceStage(LoopStageObserve)
		e.emitStatus(ctx, state, RuntimeStreamStatus, map[string]any{"status": "stage_changed"})
		if observeErr := e.observe(ctx, state, output, execErr); observeErr != nil {
			state.AddObservation(LoopObservation{Stage: LoopStageObserve, Iteration: state.Iteration, Error: observeErr.Error()})
			state.MarkStopped(classifyStopReason(observeErr.Error()), LoopDecisionDone)
			return e.finalize(state, output, observeErr)
		}
		state.AdvanceStage(LoopStageValidate)
		e.emitStatus(ctx, state, RuntimeStreamStatus, map[string]any{"status": "stage_changed"})
		validation, validateErr := e.validator().Validate(ctx, input, state, output, execErr)
		if validateErr != nil {
			state.AddObservation(LoopObservation{Stage: LoopStageValidate, Iteration: state.Iteration, Error: validateErr.Error()})
			state.ValidationStatus = LoopValidationStatusFailed
			state.ValidationSummary = validateErr.Error()
			e.saveCheckpoint(ctx, input, state, output)
			state.MarkStopped(StopReasonValidationFailed, LoopDecisionDone)
			return e.finalize(state, output, validateErr)
		}
		if validation != nil {
			state.ApplyValidationResult(validation)
			state.AddObservation(LoopObservation{
				Stage:     LoopStageValidate,
				Content:   validation.Summary,
				Iteration: state.Iteration,
				Metadata:  cloneMetadata(validation.Metadata),
			})
			if output != nil && len(validation.Metadata) > 0 {
				if output.Metadata == nil {
					output.Metadata = map[string]any{}
				}
				for key, value := range validation.Metadata {
					output.Metadata[key] = value
				}
				state.LastOutput = output
			}
			e.emitStatus(ctx, state, RuntimeStreamStatus, map[string]any{
				"status":             "validation_checked",
				"validation_status":  string(validation.Status),
				"validation_passed":  validation.Passed,
				"validation_pending": validation.Pending,
				"validation_summary": validation.Summary,
				"unresolved_items":   cloneStringSlice(validation.UnresolvedItems),
				"remaining_risks":    cloneStringSlice(validation.RemainingRisks),
			})
			e.recordTimeline("validation_gate", validation.Summary, map[string]any{
				"validation_status":   string(validation.Status),
				"validation_passed":   validation.Passed,
				"validation_pending":  validation.Pending,
				"acceptance_criteria": cloneStringSlice(validation.AcceptanceCriteria),
				"unresolved_items":    cloneStringSlice(validation.UnresolvedItems),
				"remaining_risks":     cloneStringSlice(validation.RemainingRisks),
			})
		}
		e.saveCheckpoint(ctx, input, state, output)
		state.AdvanceStage(LoopStageEvaluate)
		e.emitStatus(ctx, state, RuntimeStreamStatus, map[string]any{"status": "stage_changed"})
		decision, judgeErr := judge.Judge(ctx, state, output, execErr)
		if judgeErr != nil {
			state.MarkStopped(classifyStopReason(judgeErr.Error()), LoopDecisionDone)
			return e.finalize(state, output, judgeErr)
		}
		if decision == nil {
			nilDecisionErr := errors.New("completion judge returned nil decision")
			state.MarkStopped(StopReasonBlocked, LoopDecisionDone)
			return e.finalize(state, output, nilDecisionErr)
		}
		state.Decision = decision.Decision
		state.StopReason = decision.StopReason
		state.Confidence = decision.Confidence
		state.NeedHuman = decision.NeedHuman
		if state.NeedHuman && state.StopReason == "" {
			state.StopReason = StopReasonNeedHuman
		}
		state.AddObservation(LoopObservation{
			Stage:     LoopStageEvaluate,
			Content:   decision.Reason,
			Iteration: state.Iteration,
			Metadata: map[string]any{
				"decision":        decision.Decision,
				"confidence":      decision.Confidence,
				"solved":          decision.Solved,
				"need_replan":     decision.NeedReplan,
				"need_reflection": decision.NeedReflection,
				"need_human":      decision.NeedHuman,
				"stop_reason":     decision.StopReason,
			},
		})
		e.emitStatus(ctx, state, RuntimeStreamStatus, map[string]any{"status": "completion_judge_decision", "decision": string(decision.Decision), "confidence": decision.Confidence, "stop_reason": string(decision.StopReason)})
		e.recordTimeline("completion_decision", decision.Reason, map[string]any{
			"decision":        string(decision.Decision),
			"confidence":      decision.Confidence,
			"solved":          decision.Solved,
			"need_replan":     decision.NeedReplan,
			"need_reflection": decision.NeedReflection,
			"need_human":      decision.NeedHuman,
			"stop_reason":     string(decision.StopReason),
		})
		logger.Debug("loop iteration evaluated", zap.Int("iteration", state.Iteration), zap.String("reasoning_mode", state.SelectedReasoningMode), zap.String("decision", string(decision.Decision)), zap.String("stop_reason", string(state.StopReason)))
		switch decision.Decision {
		case LoopDecisionDone, LoopDecisionEscalate:
			e.emitStatus(ctx, state, RuntimeStreamStatus, map[string]any{"status": "loop_stopped"})
			return e.finalize(state, output, execErr)
		case LoopDecisionReplan:
			state.AdvanceStage(LoopStageDecideNext)
			e.emitStatus(ctx, state, RuntimeStreamStatus, map[string]any{"status": "stage_changed"})
			state.Plan = nil
			state.CurrentStepID = ""
			needPlan = e.Planner != nil
		case LoopDecisionContinue:
			state.AdvanceStage(LoopStageDecideNext)
			e.emitStatus(ctx, state, RuntimeStreamStatus, map[string]any{"status": "stage_changed"})
		case LoopDecisionReflect:
			state.AdvanceStage(LoopStageDecideNext)
			e.emitStatus(ctx, state, RuntimeStreamStatus, map[string]any{"status": "stage_changed"})
			nextInput, reflectErr := e.reflect(ctx, input, output, state)
			if reflectErr != nil {
				state.MarkStopped(classifyStopReason(reflectErr.Error()), LoopDecisionDone)
				return e.finalize(state, output, reflectErr)
			}
			if nextInput != nil {
				input = nextInput
			}
			needPlan = e.Planner != nil
		default:
			unsupportedErr := NewError(types.ErrAgentExecution, fmt.Sprintf("unsupported loop decision %q", decision.Decision))
			state.MarkStopped(StopReasonBlocked, LoopDecisionDone)
			return e.finalize(state, output, unsupportedErr)
		}
	}
}

// Merged from loop_executor_runtime.go.

// ReasoningRuntime bridges mode selection, reasoning execution, and reflection
// into a single loop-facing runtime contract.
type ReasoningRuntime interface {
	Select(ctx context.Context, input *Input, state *LoopState) ReasoningSelection
	Execute(ctx context.Context, input *Input, state *LoopState, selection ReasoningSelection) (*Output, error)
	Reflect(ctx context.Context, input *Input, output *Output, state *LoopState) (*LoopReflectionResult, error)
}

type defaultReasoningRuntime struct {
	registry          *reasoning.PatternRegistry
	reflectionEnabled bool
	options           types.ExecutionOptions
	selector          ReasoningModeSelector
	stepExecutor      LoopStepExecutorFunc
	reflectionStep    LoopReflectionFunc
}

// NewDefaultReasoningRuntime wraps the existing selector / executor / reflection
// callbacks behind the unified ReasoningRuntime interface.
func NewDefaultReasoningRuntime(
	options types.ExecutionOptions,
	registry *reasoning.PatternRegistry,
	reflectionEnabled bool,
	selector ReasoningModeSelector,
	stepExecutor LoopStepExecutorFunc,
	reflectionStep LoopReflectionFunc,
) ReasoningRuntime {
	return &defaultReasoningRuntime{
		registry:          registry,
		reflectionEnabled: reflectionEnabled,
		options:           options,
		selector:          selector,
		stepExecutor:      stepExecutor,
		reflectionStep:    reflectionStep,
	}
}

func (r *defaultReasoningRuntime) Select(ctx context.Context, input *Input, state *LoopState) ReasoningSelection {
	selection := ReasoningSelection{Mode: ReasoningModeReact}
	if r.selector != nil {
		selection = r.selector.Select(ctx, input, state, r.registry, r.reflectionEnabled)
		if strings.TrimSpace(selection.Mode) == "" {
			selection.Mode = ReasoningModeReact
		}
	}
	if r.options.Control.DisablePlanner {
		return normalizePlannerDisabledSelection(selection, r.registry, input, state, r.reflectionEnabled)
	}
	return selection
}

func (r *defaultReasoningRuntime) Execute(ctx context.Context, input *Input, state *LoopState, selection ReasoningSelection) (*Output, error) {
	if r.stepExecutor == nil {
		return nil, NewError("LOOP_STEP_EXECUTOR_MISSING", "loop step executor is required")
	}
	return r.stepExecutor(ctx, input, state, selection)
}

func (r *defaultReasoningRuntime) Reflect(ctx context.Context, input *Input, output *Output, state *LoopState) (*LoopReflectionResult, error) {
	if r.reflectionStep == nil {
		return nil, nil
	}
	return r.reflectionStep(ctx, input, output, state)
}

func OutputFromReasoningResult(traceID string, result *reasoning.ReasoningResult) *Output {
	if result == nil {
		return &Output{TraceID: traceID}
	}
	metadata := make(map[string]any, len(result.Metadata)+4)
	for key, value := range result.Metadata {
		metadata[key] = value
	}
	metadata["reasoning_pattern"] = result.Pattern
	metadata["reasoning_task"] = result.Task
	metadata["reasoning_confidence"] = result.Confidence
	metadata["reasoning_steps"] = result.Steps
	return &Output{
		TraceID:               traceID,
		Content:               result.FinalAnswer,
		Metadata:              metadata,
		TokensUsed:            result.TotalTokens,
		Duration:              result.TotalLatency,
		CurrentStage:          "reasoning_completed",
		IterationCount:        len(result.Steps),
		SelectedReasoningMode: runtimeNormalizeReasoningMode(result.Pattern),
	}
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

func normalizePlannerDisabledSelection(selection ReasoningSelection, registry *reasoning.PatternRegistry, input *Input, state *LoopState, reflectionEnabled bool) ReasoningSelection {
	if runtimeNormalizeReasoningMode(selection.Mode) == ReasoningModeReflection && runtimeShouldUseReflection(input, state, registry, reflectionEnabled) {
		return runtimeBuildReasoningSelection(ReasoningModeReflection, registry)
	}
	return runtimeBuildReasoningSelection(ReasoningModeReact, registry)
}

func isIgnorableLoopPlanError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(text, "tool call") ||
		strings.Contains(text, "returned no steps") ||
		strings.Contains(text, "returned no choices")
}

func (e *LoopExecutor) initialState(ctx context.Context, input *Input) *LoopState {
	maxIterations := e.ExecutionOptions.Control.MaxLoopIterations
	if maxIterations <= 0 {
		maxIterations = e.maxIterations()
	}
	state := NewLoopState(input, maxIterations)
	if state.AgentID == "" {
		state.AgentID = e.AgentID
	}
	if runID, ok := types.RunID(ctx); ok && strings.TrimSpace(runID) != "" {
		state.RunID = runID
	} else if input != nil && state.RunID == "" {
		state.RunID = strings.TrimSpace(input.TraceID)
	}
	if state.LoopStateID == "" {
		state.LoopStateID = buildLoopStateID(input, state, e.AgentID)
	}
	if e.CheckpointManager != nil && input != nil && input.Context != nil {
		if checkpointID, ok := input.Context["checkpoint_id"].(string); ok && strings.TrimSpace(checkpointID) != "" {
			checkpoint, err := e.CheckpointManager.LoadCheckpoint(ctx, checkpointID)
			if err != nil {
				e.logger().Warn("resume checkpoint load failed", zap.String("checkpoint_id", checkpointID), zap.Error(err))
			} else if checkpoint != nil {
				state.CheckpointID = checkpoint.ID
				state.Resumable = true
				if checkpoint.AgentID != "" {
					state.AgentID = checkpoint.AgentID
				}
				state.restoreFromContext(checkpoint.LoopContextValues())
				state.restoreFromContext(checkpoint.Metadata)
				if checkpoint.ExecutionContext != nil {
					state.restoreFromContext(checkpoint.ExecutionContext.LoopContextValues())
				}
			}
		}
	}
	state.SyncCurrentStep()
	return state
}

func (e *LoopExecutor) maxIterations() int {
	if e.MaxIterations > 0 {
		return e.MaxIterations
	}
	return 1
}

func (e *LoopExecutor) logger() *zap.Logger {
	if e.Logger != nil {
		return e.Logger
	}
	return zap.NewNop()
}

func (e *LoopExecutor) executionOptions() types.ExecutionOptions {
	return e.ExecutionOptions.Clone()
}

func (e *LoopExecutor) selector() ReasoningModeSelector {
	if e.Selector != nil {
		return e.Selector
	}
	return NewDefaultReasoningModeSelector()
}

func (e *LoopExecutor) selectReasoning(ctx context.Context, input *Input, state *LoopState) ReasoningSelection {
	disablePlanner := e.ExecutionOptions.Control.DisablePlanner
	if e.ReasoningRuntime != nil {
		selection := e.ReasoningRuntime.Select(ctx, input, state)
		if strings.TrimSpace(selection.Mode) == "" {
			selection.Mode = ReasoningModeReact
		}
		if disablePlanner {
			return normalizePlannerDisabledSelection(selection, e.ReasoningRegistry, input, state, e.ReflectionEnabled)
		}
		return selection
	}
	selection := ReasoningSelection{Mode: ReasoningModeReact}
	if selector := e.selector(); selector != nil {
		selection = selector.Select(ctx, input, state, e.ReasoningRegistry, e.ReflectionEnabled)
		if strings.TrimSpace(selection.Mode) == "" {
			selection.Mode = ReasoningModeReact
		}
	}
	if disablePlanner {
		return normalizePlannerDisabledSelection(selection, e.ReasoningRegistry, input, state, e.ReflectionEnabled)
	}
	return selection
}

func (e *LoopExecutor) executeReasoning(ctx context.Context, input *Input, state *LoopState, selection ReasoningSelection) (*Output, error) {
	if e.ReasoningRuntime != nil {
		return e.ReasoningRuntime.Execute(ctx, input, state, selection)
	}
	if e.StepExecutor == nil {
		return nil, NewError("LOOP_STEP_EXECUTOR_MISSING", "loop step executor is required")
	}
	return e.StepExecutor(ctx, input, state, selection)
}

func (e *LoopExecutor) judge() CompletionJudge {
	if e.Judge != nil {
		return e.Judge
	}
	return NewDefaultCompletionJudge()
}

func (e *LoopExecutor) validator() LoopValidator {
	if e.Validator != nil {
		return LoopValidationFuncAdapter(e.Validator)
	}
	return NewDefaultLoopValidator()
}

func (e *LoopExecutor) observe(ctx context.Context, state *LoopState, output *Output, execErr error) error {
	if e.Observer == nil {
		return nil
	}
	feedbackType := "loop_iteration"
	content := ""
	data := map[string]any{
		"iteration":               state.Iteration,
		"current_stage":           state.CurrentStage,
		"selected_reasoning_mode": state.SelectedReasoningMode,
		"checkpoint_id":           state.CheckpointID,
		"resumable":               state.Resumable,
		"validation_status":       string(state.ValidationStatus),
		"validation_summary":      state.ValidationSummary,
		"unresolved_items":        cloneStringSlice(state.UnresolvedItems),
		"remaining_risks":         cloneStringSlice(state.RemainingRisks),
	}
	if len(state.Plan) > 0 {
		data["plan"] = append([]string(nil), state.Plan...)
	}
	if output != nil {
		content = output.Content
		if output.Metadata != nil {
			data["output_metadata"] = cloneMetadata(output.Metadata)
		}
	}
	if execErr != nil {
		feedbackType = "loop_error"
		content = execErr.Error()
	}
	return e.Observer(ctx, &Feedback{Type: feedbackType, Content: content, Data: data}, state)
}

func (e *LoopExecutor) saveCheckpoint(ctx context.Context, input *Input, state *LoopState, output *Output) {
	if e.CheckpointManager == nil || state == nil || input == nil {
		return
	}
	threadID := strings.TrimSpace(input.ChannelID)
	if threadID == "" {
		threadID = strings.TrimSpace(input.TraceID)
	}
	if threadID == "" {
		threadID = e.AgentID
	}
	checkpoint := &Checkpoint{
		ID:       state.CheckpointID,
		ThreadID: threadID,
		AgentID:  e.AgentID,
		State:    StateRunning,
	}
	state.PopulateCheckpoint(checkpoint)
	if output != nil && strings.TrimSpace(output.Content) != "" {
		checkpoint.Messages = []CheckpointMessage{{
			Role:    "assistant",
			Content: output.Content,
			Metadata: map[string]any{
				"iteration_count": state.Iteration,
			},
		}}
	}
	if err := e.CheckpointManager.SaveCheckpoint(ctx, checkpoint); err != nil {
		e.logger().Warn("save loop checkpoint failed", zap.Error(err))
		return
	}
	state.CheckpointID = checkpoint.ID
	state.Resumable = true
}

func buildLoopStateID(input *Input, state *LoopState, agentID string) string {
	if state != nil && strings.TrimSpace(state.LoopStateID) != "" {
		return strings.TrimSpace(state.LoopStateID)
	}
	if state != nil && strings.TrimSpace(state.RunID) != "" {
		return "loop_" + strings.TrimSpace(state.RunID)
	}
	if input != nil && strings.TrimSpace(input.TraceID) != "" {
		return "loop_" + strings.TrimSpace(input.TraceID)
	}
	if strings.TrimSpace(agentID) != "" {
		return "loop_" + strings.TrimSpace(agentID)
	}
	return "loop_default"
}

func (e *LoopExecutor) reflect(ctx context.Context, input *Input, output *Output, state *LoopState) (*Input, error) {
	if e.ReasoningRuntime != nil {
		result, err := e.ReasoningRuntime.Reflect(ctx, input, output, state)
		if err != nil {
			return nil, err
		}
		if result == nil {
			return input, nil
		}
		if result.Observation != nil {
			observation := *result.Observation
			if observation.Stage == "" {
				observation.Stage = LoopStageDecideNext
			}
			if observation.Iteration == 0 {
				observation.Iteration = state.Iteration
			}
			state.AddObservation(observation)
		}
		if result.NextInput != nil {
			return result.NextInput, nil
		}
		return input, nil
	}
	if e.ReflectionStep == nil {
		return input, nil
	}
	result, err := e.ReflectionStep(ctx, input, output, state)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return input, nil
	}
	if result.Observation != nil {
		observation := *result.Observation
		if observation.Stage == "" {
			observation.Stage = LoopStageDecideNext
		}
		if observation.Iteration == 0 {
			observation.Iteration = state.Iteration
		}
		state.AddObservation(observation)
	}
	if result.NextInput != nil {
		return result.NextInput, nil
	}
	return input, nil
}

func (e *LoopExecutor) emitStatus(ctx context.Context, state *LoopState, eventType RuntimeStreamEventType, data map[string]any) {
	emit, ok := runtimeStreamEmitterFromContext(ctx)
	if !ok || state == nil {
		return
	}
	emit(RuntimeStreamEvent{
		Type:           eventType,
		Timestamp:      time.Now(),
		Data:           data,
		CurrentStage:   string(state.CurrentStage),
		IterationCount: state.Iteration,
		SelectedMode:   state.SelectedReasoningMode,
		StopReason:     string(state.StopReason),
		CheckpointID:   state.CheckpointID,
		Resumable:      state.Resumable,
	})
}

func (e *LoopExecutor) recordTimeline(entryType, summary string, metadata map[string]any) {
	if e == nil || e.Explainability == nil || strings.TrimSpace(e.TraceID) == "" {
		return
	}
	e.Explainability.AddExplainabilityTimeline(e.TraceID, entryType, summary, metadata)
}

func (e *LoopExecutor) finalize(state *LoopState, output *Output, execErr error) (*Output, error) {
	if state != nil && state.StopReason == "" {
		switch {
		case execErr == nil && output != nil && strings.TrimSpace(output.Content) != "":
			state.StopReason = StopReasonSolved
		case execErr == nil && state.Iteration >= state.MaxIterations:
			state.StopReason = StopReasonMaxIterations
		case execErr != nil:
			state.StopReason = classifyStopReason(execErr.Error())
		default:
			state.StopReason = StopReasonBlocked
		}
	}
	finalOutput := output
	if finalOutput == nil {
		finalOutput = &Output{}
	}
	if state != nil {
		finalOutput.IterationCount = state.Iteration
		finalOutput.CurrentStage = string(state.CurrentStage)
		finalOutput.SelectedReasoningMode = state.SelectedReasoningMode
		finalOutput.StopReason = string(state.StopReason)
		finalOutput.Resumable = state.Resumable
		finalOutput.CheckpointID = state.CheckpointID
		if finalOutput.Metadata == nil {
			finalOutput.Metadata = map[string]any{}
		}
		if len(state.Plan) > 0 {
			finalOutput.Metadata["loop_plan"] = append([]string(nil), state.Plan...)
		}
		finalOutput.Metadata["loop_iteration_count"] = state.Iteration
		finalOutput.Metadata["iteration_count"] = state.Iteration
		finalOutput.Metadata["loop_stop_reason"] = state.StopReason
		finalOutput.Metadata["stop_reason"] = string(state.StopReason)
		finalOutput.Metadata["loop_decision"] = state.Decision
		finalOutput.Metadata["loop_confidence"] = state.Confidence
		finalOutput.Metadata["loop_need_human"] = state.NeedHuman
		finalOutput.Metadata["current_stage"] = string(state.CurrentStage)
		finalOutput.Metadata["selected_reasoning_mode"] = state.SelectedReasoningMode
		finalOutput.Metadata["checkpoint_id"] = state.CheckpointID
		finalOutput.Metadata["resumable"] = state.Resumable
		finalOutput.Metadata["validation_status"] = string(state.ValidationStatus)
		finalOutput.Metadata["validation_summary"] = state.ValidationSummary
		finalOutput.Metadata["acceptance_criteria"] = cloneStringSlice(state.AcceptanceCriteria)
		finalOutput.Metadata["unresolved_items"] = cloneStringSlice(state.UnresolvedItems)
		finalOutput.Metadata["remaining_risks"] = cloneStringSlice(state.RemainingRisks)
		if critiques := reflectionCritiquesFromObservations(state.Observations); len(critiques) > 0 {
			finalOutput.Metadata["reflection_iterations"] = len(critiques)
			finalOutput.Metadata["reflection_critiques"] = critiques
		}
	}
	if execErr != nil {
		return finalOutput, execErr
	}
	return finalOutput, nil
}
