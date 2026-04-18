package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/agent/execution"
	"github.com/BaSui01/agentflow/agent/reasoning"
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
			Planner:           b.loopPlanner(),
			StepExecutor:      b.loopStepExecutor(options),
			Observer:          b.loopObserver(),
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
	codeValidator *execution.CodeValidator
}

func NewCodeTaskLoopValidator() CodeTaskLoopValidator {
	return CodeTaskLoopValidator{codeValidator: execution.NewCodeValidator()}
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

func codeSnippetForValidation(output *Output) (execution.Language, string, bool) {
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
		return execution.LangPython, rawCode, true
	case "javascript", "js":
		return execution.LangJavaScript, rawCode, true
	case "typescript", "ts":
		return execution.LangTypeScript, rawCode, true
	case "go", "golang":
		return execution.LangGo, rawCode, true
	case "rust":
		return execution.LangRust, rawCode, true
	case "bash", "shell", "sh":
		return execution.LangBash, rawCode, true
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

const (
	ReasoningModeReact          = "react"
	ReasoningModeReflection     = "reflection"
	ReasoningModeReWOO          = "rewoo"
	ReasoningModePlanAndExecute = "plan_and_execute"
	ReasoningModeDynamicPlanner = "dynamic_planner"
	ReasoningModeTreeOfThought  = "tree_of_thought"
)

var reasoningModeAliases = map[string]string{
	"react":            ReasoningModeReact,
	"reflection":       ReasoningModeReflection,
	"reflexion":        ReasoningModeReflection,
	"rewoo":            ReasoningModeReWOO,
	"plan_and_execute": ReasoningModePlanAndExecute,
	"plan_execute":     ReasoningModePlanAndExecute,
	"dynamic_planner":  ReasoningModeDynamicPlanner,
	"tree_of_thought":  ReasoningModeTreeOfThought,
	"tree-of-thought":  ReasoningModeTreeOfThought,
	"tree of thought":  ReasoningModeTreeOfThought,
	"tot":              ReasoningModeTreeOfThought,
}

var reasoningPatternCandidates = map[string][]string{
	ReasoningModeReflection:     {"reflexion", ReasoningModeReflection},
	ReasoningModeReWOO:          {ReasoningModeReWOO},
	ReasoningModePlanAndExecute: {ReasoningModePlanAndExecute, "plan_execute"},
	ReasoningModeDynamicPlanner: {ReasoningModeDynamicPlanner},
	ReasoningModeTreeOfThought:  {ReasoningModeTreeOfThought},
}

type ReasoningSelection struct {
	Mode    string
	Pattern reasoning.ReasoningPattern
}

type ReasoningModeSelector interface {
	Select(ctx context.Context, input *Input, state *LoopState, registry *reasoning.PatternRegistry, reflectionEnabled bool) ReasoningSelection
}

type DefaultReasoningModeSelector struct{}

func NewDefaultReasoningModeSelector() ReasoningModeSelector { return DefaultReasoningModeSelector{} }

func (DefaultReasoningModeSelector) Select(ctx context.Context, input *Input, state *LoopState, registry *reasoning.PatternRegistry, reflectionEnabled bool) ReasoningSelection {
	runConfig := ResolveRunConfig(ctx, input)
	if DisablePlannerEnabled(input, runConfig) {
		if selection, ok := selectResumedReasoningMode(state, registry, reflectionEnabled); ok && selection.Mode == ReasoningModeReflection {
			return selection
		}
		if shouldUseReflection(input, state, registry, reflectionEnabled) {
			return buildReasoningSelection(ReasoningModeReflection, registry)
		}
		return buildReasoningSelection(ReasoningModeReact, registry)
	}
	if selection, ok := selectResumedReasoningMode(state, registry, reflectionEnabled); ok {
		return selection
	}
	if shouldUseReflection(input, state, registry, reflectionEnabled) {
		return buildReasoningSelection(ReasoningModeReflection, registry)
	}
	if shouldUseTreeOfThought(input, state, registry) {
		return buildReasoningSelection(ReasoningModeTreeOfThought, registry)
	}
	if shouldUseDynamicPlanner(input, state, registry) {
		return buildReasoningSelection(ReasoningModeDynamicPlanner, registry)
	}
	if shouldUsePlanAndExecute(input, state, registry) {
		return buildReasoningSelection(ReasoningModePlanAndExecute, registry)
	}
	if shouldUseReWOO(input, state, registry) {
		return buildReasoningSelection(ReasoningModeReWOO, registry)
	}
	return buildReasoningSelection(ReasoningModeReact, registry)
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
		SelectedReasoningMode: normalizeReasoningMode(result.Pattern),
	}
}

func (b *BaseAgent) loopSelector(options EnhancedExecutionOptions) ReasoningModeSelector {
	base := b.reasoningSelector
	if base == nil {
		base = NewDefaultReasoningModeSelector()
	}
	if !(options.UseReflection && b.extensions.ReflectionExecutor() != nil) {
		return base
	}
	return reasoningModeSelectorFunc(func(ctx context.Context, input *Input, state *LoopState, registry *reasoning.PatternRegistry, reflectionEnabled bool) ReasoningSelection {
		selection := base.Select(ctx, input, state, registry, reflectionEnabled)
		if DisablePlannerEnabled(input, ResolveRunConfig(ctx, input)) {
			return selection
		}
		if strings.TrimSpace(selection.Mode) == "" || selection.Mode == ReasoningModeReact {
			selection.Mode = ReasoningModeReflection
		}
		return selection
	})
}

func (b *BaseAgent) loopMaxIterations() int {
	policy := b.loopControlPolicy()
	if policy.LoopIterationBudget > 0 {
		return policy.LoopIterationBudget
	}
	return 1
}

func (b *BaseAgent) loopPlanner() LoopPlannerFunc {
	return func(ctx context.Context, input *Input, _ *LoopState) (*PlanResult, error) {
		// Specialist execution tasks may already be decomposed by an upstream
		// orchestrator, so do not wrap them in another planner prompt.
		if DisablePlannerEnabled(input, ResolveRunConfig(ctx, input)) {
			return nil, nil
		}
		return b.Plan(ctx, input)
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

func selectResumedReasoningMode(state *LoopState, registry *reasoning.PatternRegistry, reflectionEnabled bool) (ReasoningSelection, bool) {
	if state == nil {
		return ReasoningSelection{}, false
	}
	if state.CurrentStage == "" || state.CurrentStage == LoopStagePerceive {
		return ReasoningSelection{}, false
	}
	mode := normalizeReasoningMode(state.SelectedReasoningMode)
	if mode == "" {
		return ReasoningSelection{}, false
	}
	return buildReasoningSelectionWithFallback(mode, registry, reflectionEnabled), true
}

func buildReasoningSelectionWithFallback(mode string, registry *reasoning.PatternRegistry, reflectionEnabled bool) ReasoningSelection {
	selection := buildReasoningSelection(mode, registry)
	if selection.Mode == ReasoningModeReflection && !reflectionEnabled && selection.Pattern == nil {
		return buildReasoningSelection(ReasoningModeReact, registry)
	}
	if selection.Mode != ReasoningModeReact && selection.Mode != ReasoningModeReflection && selection.Pattern == nil {
		return buildReasoningSelection(ReasoningModeReact, registry)
	}
	return selection
}

func buildReasoningSelection(mode string, registry *reasoning.PatternRegistry) ReasoningSelection {
	normalized := normalizeReasoningMode(mode)
	if normalized == "" {
		normalized = ReasoningModeReact
	}
	selection := ReasoningSelection{Mode: normalized}
	if registry == nil {
		return selection
	}
	for _, candidate := range reasoningPatternCandidates[normalized] {
		pattern, ok := registry.Get(candidate)
		if ok {
			selection.Pattern = pattern
			return selection
		}
	}
	return selection
}

func normalizeReasoningMode(value string) string {
	key := strings.ToLower(strings.TrimSpace(value))
	if key == "" {
		return ""
	}
	if normalized, ok := reasoningModeAliases[key]; ok {
		return normalized
	}
	return ""
}

func shouldUseReflection(input *Input, state *LoopState, registry *reasoning.PatternRegistry, reflectionEnabled bool) bool {
	if !reflectionEnabled && !hasReasoningPattern(registry, ReasoningModeReflection) {
		return false
	}
	return contextBool(input, "requires_reflection") ||
		contextBool(input, "need_reflection") ||
		contextBool(input, "quality_critical") ||
		contextBool(input, "needs_critique") ||
		contextString(input, "current_stage") == "reflection" ||
		contextString(input, "loop_stage") == "reflection" ||
		(state != nil && state.Decision == LoopDecisionReflect) ||
		(state != nil && state.Iteration > 1 && state.LastOutput != nil && strings.TrimSpace(state.LastOutput.Content) == "")
}

func shouldUseReWOO(input *Input, state *LoopState, registry *reasoning.PatternRegistry) bool {
	if !hasReasoningPattern(registry, ReasoningModeReWOO) {
		return false
	}
	if state != nil && len(state.Plan) > 0 && len(state.Plan) >= 3 {
		return true
	}
	if state != nil && state.CurrentStage == LoopStageValidate {
		return true
	}
	return contextBool(input, "tool_intensive") ||
		contextBool(input, "tool_verification_required") ||
		contextBool(input, "requires_tools") ||
		contextBool(input, "requires_observationless_tool_plan") ||
		intContextAtLeast(input, "tool_count", 2) ||
		contentContainsAny(input, "tool", "tools", "search", "collect", "gather", "retrieve", "crawl", "inspect")
}

func shouldUsePlanAndExecute(input *Input, state *LoopState, registry *reasoning.PatternRegistry) bool {
	if !hasReasoningPattern(registry, ReasoningModePlanAndExecute) {
		return false
	}
	if state != nil && len(state.Plan) > 0 {
		return true
	}
	return contextBool(input, "requires_plan") ||
		contextBool(input, "multi_step") ||
		contextBool(input, "needs_replanning") ||
		contextBool(input, "complex_task") ||
		intContextAtLeast(input, "plan_steps", 2) ||
		contentContainsAny(input, "plan", "steps", "implement", "execute", "roadmap", "break down")
}

func shouldUseDynamicPlanner(input *Input, state *LoopState, registry *reasoning.PatternRegistry) bool {
	if !hasReasoningPattern(registry, ReasoningModeDynamicPlanner) {
		return false
	}
	return contextBool(input, "requires_backtracking") ||
		contextBool(input, "blocked") ||
		contextBool(input, "requires_alternative_paths") ||
		contextBool(input, "dynamic_replanning") ||
		contextBool(input, "search_space_large") ||
		(state != nil && state.Decision == LoopDecisionReplan) ||
		(state != nil && state.StopReason == StopReasonBlocked) ||
		contentContainsAny(input, "backtrack", "alternative", "constraint", "optimize")
}

func shouldUseTreeOfThought(input *Input, state *LoopState, registry *reasoning.PatternRegistry) bool {
	if !hasReasoningPattern(registry, ReasoningModeTreeOfThought) {
		return false
	}
	if state != nil && state.Iteration > 1 && state.Confidence < 0.5 {
		return true
	}
	return contextBool(input, "high_uncertainty") ||
		contextBool(input, "explore_multiple_paths") ||
		contextBool(input, "compare_branches") ||
		intContextAtLeast(input, "candidate_count", 3) ||
		contentContainsAny(input, "compare options", "multiple approaches", "explore", "brainstorm", "tradeoff", "uncertain")
}

func hasReasoningPattern(registry *reasoning.PatternRegistry, mode string) bool {
	if registry == nil {
		return false
	}
	for _, candidate := range reasoningPatternCandidates[mode] {
		if _, ok := registry.Get(candidate); ok {
			return true
		}
	}
	return false
}

func contextBool(input *Input, key string) bool {
	if input == nil || len(input.Context) == 0 {
		return false
	}
	value, ok := input.Context[key]
	if !ok {
		return false
	}
	flag, ok := value.(bool)
	return ok && flag
}

func contextString(input *Input, key string) string {
	if input == nil || len(input.Context) == 0 {
		return ""
	}
	value, ok := input.Context[key]
	if !ok {
		return ""
	}
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func intContextAtLeast(input *Input, key string, min int) bool {
	if input == nil || len(input.Context) == 0 {
		return false
	}
	value, ok := input.Context[key]
	if !ok {
		return false
	}
	switch typed := value.(type) {
	case int:
		return typed >= min
	case int32:
		return int(typed) >= min
	case int64:
		return int(typed) >= min
	case float64:
		return int(typed) >= min
	default:
		return false
	}
}

func contentContainsAny(input *Input, terms ...string) bool {
	if input == nil {
		return false
	}
	content := strings.ToLower(input.Content)
	for _, term := range terms {
		if strings.Contains(content, strings.ToLower(term)) {
			return true
		}
	}
	return false
}

func normalizePlannerDisabledSelection(selection ReasoningSelection, registry *reasoning.PatternRegistry, input *Input, state *LoopState, reflectionEnabled bool) ReasoningSelection {
	if normalizeReasoningMode(selection.Mode) == ReasoningModeReflection && shouldUseReflection(input, state, registry, reflectionEnabled) {
		return buildReasoningSelection(ReasoningModeReflection, registry)
	}
	return buildReasoningSelection(ReasoningModeReact, registry)
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

func (e *LoopExecutor) initialState(ctx context.Context, input *Input) *LoopState {
	maxIterations := e.ExecutionOptions.Control.MaxLoopIterations
	if maxIterations <= 0 {
		maxIterations = e.maxIterations()
	}
	if value, ok := topLevelLoopBudget(input); ok && value > 0 {
		maxIterations = value
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

func topLevelLoopBudget(input *Input) (int, bool) {
	if input == nil || len(input.Context) == 0 {
		return 0, false
	}
	return intOverrideFromContext(input.Context, "top_level_loop_budget")
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
	if e.ReasoningRuntime != nil {
		selection := e.ReasoningRuntime.Select(ctx, input, state)
		if strings.TrimSpace(selection.Mode) == "" {
			selection.Mode = ReasoningModeReact
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
	if e.ExecutionOptions.Control.DisablePlanner {
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

func cloneMetadata(metadata map[string]any) map[string]any {
	if len(metadata) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(metadata))
	for k, v := range metadata {
		cloned[k] = v
	}
	return cloned
}

type reasoningModeSelectorFunc func(ctx context.Context, input *Input, state *LoopState, registry *reasoning.PatternRegistry, reflectionEnabled bool) ReasoningSelection

func (f reasoningModeSelectorFunc) Select(ctx context.Context, input *Input, state *LoopState, registry *reasoning.PatternRegistry, reflectionEnabled bool) ReasoningSelection {
	return f(ctx, input, state, registry, reflectionEnabled)
}

type LoopValidationFuncAdapter LoopValidationFunc

func (f LoopValidationFuncAdapter) Validate(ctx context.Context, input *Input, state *LoopState, output *Output, err error) (*LoopValidationResult, error) {
	return f(ctx, input, state, output, err)
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
		sessionID := traceID
		if input != nil && strings.TrimSpace(input.ChannelID) != "" {
			sessionID = strings.TrimSpace(input.ChannelID)
		}
		b.extensions.ObservabilitySystemExt().StartTrace(traceID, b.ID())
		if recorder, ok := b.extensions.ObservabilitySystemExt().(ExplainabilityRecorder); ok {
			recorder.StartExplainabilityTrace(traceID, sessionID, b.ID())
		}

		output, err := next(ctx, input)

		if err != nil {
			b.extensions.ObservabilitySystemExt().EndTrace(traceID, "failed", err)
			if recorder, ok := b.extensions.ObservabilitySystemExt().(ExplainabilityRecorder); ok {
				recorder.EndExplainabilityTrace(traceID, false, "", err.Error())
			}
			return nil, err
		}
		duration := time.Since(startTime)
		if options.RecordMetrics {
			b.extensions.ObservabilitySystemExt().RecordTask(b.ID(), true, duration, output.TokensUsed, output.Cost, 0.8)
		}
		if options.RecordTrace {
			b.extensions.ObservabilitySystemExt().EndTrace(traceID, "completed", nil)
		}
		if recorder, ok := b.extensions.ObservabilitySystemExt().(ExplainabilityRecorder); ok {
			recorder.EndExplainabilityTrace(traceID, true, output.Content, "")
		}
		b.logger.Info("enhanced execution completed",
			zap.String("trace_id", input.TraceID),
			zap.Duration("total_duration", duration),
			zap.Int("tokens_used", output.TokensUsed),
			zap.Any("prompt_layer_ids", output.Metadata["applied_prompt_layer_ids"]),
			zap.Any("context_plan", output.Metadata["context_plan"]),
		)
		return output, nil
	}
}

func explainabilityTimelineRecorder(obs ObservabilityRunner) ExplainabilityTimelineRecorder {
	recorder, _ := obs.(ExplainabilityTimelineRecorder)
	return recorder
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

		skillInstructions = normalizeInstructionList(skillInstructions)
		if len(skillInstructions) > 0 {
			input = shallowCopyInput(input)
			if input.Context == nil {
				input.Context = make(map[string]any, 1)
			}
			input.Context["skill_context"] = append([]string(nil), skillInstructions...)
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

		if len(memoryContext) > 0 {
			input = shallowCopyInput(input)
			if input.Context == nil {
				input.Context = make(map[string]any, 1)
			}
			input.Context["memory_context"] = append([]string(nil), memoryContext...)
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
			toolNames := make([]string, 0, len(selected))
			for _, tool := range selected {
				name := strings.TrimSpace(tool.Name)
				if name == "" {
					continue
				}
				toolNames = append(toolNames, name)
			}

			override := &RunConfig{}
			if len(toolNames) == 0 {
				override.DisableTools = true
			} else {
				override.ToolWhitelist = toolNames
			}
			ctx = WithRunConfig(ctx, MergeRunConfig(GetRunConfig(ctx), override))

			b.logger.Info("tools selected dynamically",
				zap.String("trace_id", input.TraceID),
				zap.Strings("selected_tools", toolNames),
				zap.Bool("tools_disabled", len(toolNames) == 0),
			)
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
		if b.memoryRuntime != nil {
			return output, nil
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

func normalizeInstructionList(instructions []string) []string {
	if len(instructions) == 0 {
		return nil
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
		return nil
	}
	return cleaned
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

func (a *reflectionRunnerAdapter) ReflectStep(ctx context.Context, input *Input, output *Output, state *LoopState) (*LoopReflectionResult, error) {
	return a.executor.ReflectStep(ctx, input, output, state)
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
