package loop

import (
	"context"
	"strings"

	"github.com/BaSui01/agentflow/agent/capabilities/reasoning"
)

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

type DefaultReasoningModeSelector struct{}

func (DefaultReasoningModeSelector) Select(_ context.Context, input *Input, state *State, registry *reasoning.PatternRegistry, reflectionEnabled bool) ReasoningSelection {
	if selection, ok := SelectResumedReasoningMode(state, registry, reflectionEnabled); ok {
		return selection
	}
	if ShouldUseReflection(input, state, registry, reflectionEnabled) {
		return BuildReasoningSelection(ReasoningModeReflection, registry)
	}
	if ShouldUseTreeOfThought(input, state, registry) {
		return BuildReasoningSelection(ReasoningModeTreeOfThought, registry)
	}
	if ShouldUseDynamicPlanner(input, state, registry) {
		return BuildReasoningSelection(ReasoningModeDynamicPlanner, registry)
	}
	if ShouldUsePlanAndExecute(input, state, registry) {
		return BuildReasoningSelection(ReasoningModePlanAndExecute, registry)
	}
	if ShouldUseReWOO(input, state, registry) {
		return BuildReasoningSelection(ReasoningModeReWOO, registry)
	}
	return BuildReasoningSelection(ReasoningModeReact, registry)
}

func SelectResumedReasoningMode(state *State, registry *reasoning.PatternRegistry, reflectionEnabled bool) (ReasoningSelection, bool) {
	if state == nil {
		return ReasoningSelection{}, false
	}
	if state.CurrentStage == "" || state.CurrentStage == "perceive" {
		return ReasoningSelection{}, false
	}
	if state.SelectedMode == "" {
		return ReasoningSelection{}, false
	}
	mode := NormalizeReasoningMode(state.SelectedMode)
	if mode == "" {
		return ReasoningSelection{}, false
	}
	return BuildReasoningSelectionWithFallback(mode, registry, reflectionEnabled), true
}

func BuildReasoningSelectionWithFallback(mode string, registry *reasoning.PatternRegistry, reflectionEnabled bool) ReasoningSelection {
	selection := BuildReasoningSelection(mode, registry)
	if selection.Mode == ReasoningModeReflection && !reflectionEnabled && selection.Pattern == nil {
		return BuildReasoningSelection(ReasoningModeReact, registry)
	}
	if selection.Mode != ReasoningModeReact && selection.Mode != ReasoningModeReflection && selection.Pattern == nil {
		return BuildReasoningSelection(ReasoningModeReact, registry)
	}
	return selection
}

func BuildReasoningSelection(mode string, registry *reasoning.PatternRegistry) ReasoningSelection {
	normalized := NormalizeReasoningMode(mode)
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

func NormalizeReasoningMode(value string) string {
	key := strings.ToLower(strings.TrimSpace(value))
	if key == "" {
		return ""
	}
	if normalized, ok := reasoningModeAliases[key]; ok {
		return normalized
	}
	return ""
}

func NormalizePlannerDisabledSelection(selection ReasoningSelection, registry *reasoning.PatternRegistry, input *Input, state *State, reflectionEnabled bool) ReasoningSelection {
	if NormalizeReasoningMode(selection.Mode) == ReasoningModeReflection && ShouldUseReflection(input, state, registry, reflectionEnabled) {
		return BuildReasoningSelection(ReasoningModeReflection, registry)
	}
	return BuildReasoningSelection(ReasoningModeReact, registry)
}

func ShouldUseReflection(input *Input, state *State, registry *reasoning.PatternRegistry, reflectionEnabled bool) bool {
	if !reflectionEnabled && !HasReasoningPattern(registry, ReasoningModeReflection) {
		return false
	}
	return contextBool(input, "requires_reflection") ||
		contextBool(input, "need_reflection") ||
		contextBool(input, "quality_critical") ||
		contextBool(input, "needs_critique") ||
		contextString(input, "current_stage") == "reflection" ||
		contextString(input, "loop_stage") == "reflection" ||
		(state != nil && state.Decision == "reflect") ||
		(state != nil && state.Iteration > 1 && strings.TrimSpace(state.Goal) != "" && strings.TrimSpace(state.ValidationSummary) == "")
}

func ShouldUseReWOO(input *Input, state *State, registry *reasoning.PatternRegistry) bool {
	if !HasReasoningPattern(registry, ReasoningModeReWOO) {
		return false
	}
	if state != nil && len(state.PlanSteps) >= 3 {
		return true
	}
	if state != nil && state.CurrentStage == "validate" {
		return true
	}
	return contextBool(input, "tool_intensive") ||
		contextBool(input, "tool_verification_required") ||
		contextBool(input, "requires_tools") ||
		contextBool(input, "requires_observationless_tool_plan") ||
		intContextAtLeast(input, "tool_count", 2) ||
		contentContainsAny(input, "tool", "tools", "search", "collect", "gather", "retrieve", "crawl", "inspect")
}

func ShouldUsePlanAndExecute(input *Input, state *State, registry *reasoning.PatternRegistry) bool {
	if !HasReasoningPattern(registry, ReasoningModePlanAndExecute) {
		return false
	}
	if state != nil && len(state.PlanSteps) > 0 {
		return true
	}
	return contextBool(input, "requires_plan") ||
		contextBool(input, "multi_step") ||
		contextBool(input, "needs_replanning") ||
		contextBool(input, "complex_task") ||
		intContextAtLeast(input, "plan_steps", 2) ||
		contentContainsAny(input, "plan", "steps", "implement", "execute", "roadmap", "break down")
}

func ShouldUseDynamicPlanner(input *Input, state *State, registry *reasoning.PatternRegistry) bool {
	if !HasReasoningPattern(registry, ReasoningModeDynamicPlanner) {
		return false
	}
	return contextBool(input, "requires_backtracking") ||
		contextBool(input, "blocked") ||
		contextBool(input, "requires_alternative_paths") ||
		contextBool(input, "dynamic_replanning") ||
		contextBool(input, "search_space_large") ||
		(state != nil && state.Decision == "replan") ||
		(state != nil && state.StopReason == "blocked") ||
		contentContainsAny(input, "backtrack", "alternative", "constraint", "optimize")
}

func ShouldUseTreeOfThought(input *Input, state *State, registry *reasoning.PatternRegistry) bool {
	if !HasReasoningPattern(registry, ReasoningModeTreeOfThought) {
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

func HasReasoningPattern(registry *reasoning.PatternRegistry, mode string) bool {
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

func intContextAtLeast(input *Input, key string, min int) bool {
	if input == nil || len(input.Context) == 0 {
		return false
	}
	raw, ok := input.Context[key]
	if !ok {
		return false
	}
	switch typed := raw.(type) {
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
	content := strings.ToLower(strings.TrimSpace(input.Content))
	if content == "" {
		return false
	}
	for _, term := range terms {
		if strings.Contains(content, strings.ToLower(strings.TrimSpace(term))) {
			return true
		}
	}
	return false
}
