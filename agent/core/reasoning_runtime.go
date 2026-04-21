package core

import (
	"context"
	"strings"

	"github.com/BaSui01/agentflow/agent/reasoning"
	"github.com/BaSui01/agentflow/types"
)

// ReasoningExposureLevel controls which non-default reasoning patterns are
// registered into the runtime. The official execution path remains react,
// with reflection as an opt-in quality enhancement outside the registry.
type ReasoningExposureLevel string

const (
	ReasoningExposureOfficial ReasoningExposureLevel = "official"
	ReasoningExposureAdvanced ReasoningExposureLevel = "advanced"
	ReasoningExposureAll      ReasoningExposureLevel = "all"
)

func normalizeReasoningExposureLevel(level ReasoningExposureLevel) ReasoningExposureLevel {
	switch level {
	case ReasoningExposureAdvanced, ReasoningExposureAll:
		return level
	default:
		return ReasoningExposureOfficial
	}
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
	if selection, ok := runtimeSelectResumedReasoningMode(state, registry, reflectionEnabled); ok {
		return selection
	}
	if runtimeShouldUseReflection(input, state, registry, reflectionEnabled) {
		return runtimeBuildReasoningSelection(ReasoningModeReflection, registry)
	}
	if runtimeShouldUseTreeOfThought(input, state, registry) {
		return runtimeBuildReasoningSelection(ReasoningModeTreeOfThought, registry)
	}
	if runtimeShouldUseDynamicPlanner(input, state, registry) {
		return runtimeBuildReasoningSelection(ReasoningModeDynamicPlanner, registry)
	}
	if runtimeShouldUsePlanAndExecute(input, state, registry) {
		return runtimeBuildReasoningSelection(ReasoningModePlanAndExecute, registry)
	}
	if runtimeShouldUseReWOO(input, state, registry) {
		return runtimeBuildReasoningSelection(ReasoningModeReWOO, registry)
	}
	return runtimeBuildReasoningSelection(ReasoningModeReact, registry)
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

func runtimeSelectResumedReasoningMode(state *LoopState, registry *reasoning.PatternRegistry, reflectionEnabled bool) (ReasoningSelection, bool) {
	if state == nil {
		return ReasoningSelection{}, false
	}
	if state.CurrentStage == "" || state.CurrentStage == LoopStagePerceive {
		return ReasoningSelection{}, false
	}
	mode := runtimeNormalizeReasoningMode(state.SelectedReasoningMode)
	if mode == "" {
		return ReasoningSelection{}, false
	}
	return runtimeBuildReasoningSelectionWithFallback(mode, registry, reflectionEnabled), true
}

func runtimeBuildReasoningSelectionWithFallback(mode string, registry *reasoning.PatternRegistry, reflectionEnabled bool) ReasoningSelection {
	selection := runtimeBuildReasoningSelection(mode, registry)
	if selection.Mode == ReasoningModeReflection && !reflectionEnabled && selection.Pattern == nil {
		return runtimeBuildReasoningSelection(ReasoningModeReact, registry)
	}
	if selection.Mode != ReasoningModeReact && selection.Mode != ReasoningModeReflection && selection.Pattern == nil {
		return runtimeBuildReasoningSelection(ReasoningModeReact, registry)
	}
	return selection
}

func runtimeBuildReasoningSelection(mode string, registry *reasoning.PatternRegistry) ReasoningSelection {
	normalized := runtimeNormalizeReasoningMode(mode)
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

func runtimeNormalizeReasoningMode(value string) string {
	key := strings.ToLower(strings.TrimSpace(value))
	if key == "" {
		return ""
	}
	if normalized, ok := reasoningModeAliases[key]; ok {
		return normalized
	}
	return ""
}

func runtimeShouldUseReflection(input *Input, state *LoopState, registry *reasoning.PatternRegistry, reflectionEnabled bool) bool {
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

func runtimeShouldUseReWOO(input *Input, state *LoopState, registry *reasoning.PatternRegistry) bool {
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

func runtimeShouldUsePlanAndExecute(input *Input, state *LoopState, registry *reasoning.PatternRegistry) bool {
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

func runtimeShouldUseDynamicPlanner(input *Input, state *LoopState, registry *reasoning.PatternRegistry) bool {
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

func runtimeShouldUseTreeOfThought(input *Input, state *LoopState, registry *reasoning.PatternRegistry) bool {
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
