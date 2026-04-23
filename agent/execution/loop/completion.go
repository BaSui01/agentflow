package loop

import (
	"context"
	"strings"

	checkpointcore "github.com/BaSui01/agentflow/agent/persistence/checkpoint/core"
)

type ValidationStatus string

const (
	ValidationStatusPassed  ValidationStatus = "passed"
	ValidationStatusPending ValidationStatus = "pending"
	ValidationStatusFailed  ValidationStatus = "failed"
)

type StopReason string

const (
	StopReasonSolved                   StopReason = "solved"
	StopReasonMaxIterations            StopReason = "max_iterations"
	StopReasonTimeout                  StopReason = "timeout"
	StopReasonNeedHuman                StopReason = "need_human"
	StopReasonValidationFailed         StopReason = "validation_failed"
	StopReasonToolFailureUnrecoverable StopReason = "tool_failure_unrecoverable"
	StopReasonBlocked                  StopReason = "blocked"
)

type Decision string

const (
	DecisionDone     Decision = "done"
	DecisionContinue Decision = "continue"
	DecisionReplan   Decision = "replan"
	DecisionReflect  Decision = "reflect"
	DecisionEscalate Decision = "escalate"
)

type CompletionDecision struct {
	Solved         bool
	Decision       Decision
	StopReason     StopReason
	Confidence     float64
	NeedReplan     bool
	NeedReflection bool
	NeedHuman      bool
	Reason         string
}

type ValidationIssue struct {
	Validator string           `json:"validator,omitempty"`
	Code      string           `json:"code,omitempty"`
	Category  string           `json:"category,omitempty"`
	Status    ValidationStatus `json:"status,omitempty"`
	Message   string           `json:"message,omitempty"`
}

type ValidationResult struct {
	Status             ValidationStatus  `json:"status,omitempty"`
	Passed             bool              `json:"passed"`
	Pending            bool              `json:"pending,omitempty"`
	Reason             string            `json:"reason,omitempty"`
	Summary            string            `json:"summary,omitempty"`
	Issues             []ValidationIssue `json:"issues,omitempty"`
	AcceptanceCriteria []string          `json:"acceptance_criteria,omitempty"`
	UnresolvedItems    []string          `json:"unresolved_items,omitempty"`
	RemainingRisks     []string          `json:"remaining_risks,omitempty"`
	Metadata           map[string]any    `json:"metadata,omitempty"`
}

type ValidationStateView struct {
	Status          ValidationStatus
	Reason          string
	UnresolvedItems []string
	RemainingRisks  []string
}

type Input struct {
	Content string
	Context map[string]any
}

type Output struct {
	Content  string
	Metadata map[string]any
}

type State struct {
	Goal               string
	CurrentStage       string
	Iteration          int
	MaxIterations      int
	Decision           string
	StopReason         string
	PlanSteps          []string
	NeedHuman          bool
	Confidence         float64
	ValidationStatus   ValidationStatus
	ValidationSummary  string
	AcceptanceCriteria []string
	UnresolvedItems    []string
	RemainingRisks     []string
	SelectedMode       string
}

type CodeValidationLanguage string

const (
	CodeLangPython     CodeValidationLanguage = "python"
	CodeLangJavaScript CodeValidationLanguage = "javascript"
	CodeLangTypeScript CodeValidationLanguage = "typescript"
	CodeLangGo         CodeValidationLanguage = "go"
	CodeLangRust       CodeValidationLanguage = "rust"
	CodeLangBash       CodeValidationLanguage = "bash"
)

type CodeWarningProvider interface {
	Validate(lang CodeValidationLanguage, code string) []string
}

func JudgeDefault(ctx context.Context, state *State, output *Output, err error) (*CompletionDecision, error) {
	if ctx != nil && ctx.Err() != nil {
		return &CompletionDecision{Decision: DecisionDone, StopReason: StopReasonTimeout, Reason: ctx.Err().Error()}, nil
	}
	if state != nil && state.NeedHuman {
		return &CompletionDecision{
			NeedHuman:  true,
			Decision:   DecisionEscalate,
			StopReason: StopReasonNeedHuman,
			Confidence: NormalizedConfidence(output, state),
			Reason:     "loop state requires human intervention",
		}, nil
	}
	if err != nil {
		return &CompletionDecision{Decision: DecisionDone, StopReason: ClassifyStopReason(err.Error()), Reason: err.Error()}, nil
	}
	if output == nil {
		if ReachedMaxIterations(state) {
			return &CompletionDecision{
				Decision:   DecisionDone,
				StopReason: StopReasonMaxIterations,
				Confidence: NormalizedConfidence(output, state),
				Reason:     "loop iteration budget exhausted",
			}, nil
		}
		return &CompletionDecision{Decision: DecisionReplan, NeedReplan: true, StopReason: StopReasonBlocked, Reason: "output is nil"}, nil
	}
	validation := CompletionValidationState(state, output)
	if strings.TrimSpace(output.Content) != "" && validation.Status == ValidationStatusPassed {
		return &CompletionDecision{
			Solved:     true,
			Decision:   DecisionDone,
			StopReason: StopReasonSolved,
			Confidence: NormalizedConfidence(output, state),
			Reason:     "validated output produced",
		}, nil
	}
	if strings.TrimSpace(output.Content) != "" && validation.Status == ValidationStatusPending {
		if ReachedMaxIterations(state) {
			return &CompletionDecision{
				Decision:   DecisionDone,
				StopReason: StopReasonMaxIterations,
				Confidence: NormalizedConfidence(output, state),
				Reason:     validation.Reason,
			}, nil
		}
		return &CompletionDecision{
			Decision:   DecisionContinue,
			Confidence: NormalizedConfidence(output, state),
			Reason:     validation.Reason,
		}, nil
	}
	if strings.TrimSpace(output.Content) != "" && validation.Status == ValidationStatusFailed {
		if ReachedMaxIterations(state) {
			return &CompletionDecision{
				Decision:   DecisionDone,
				StopReason: StopReasonValidationFailed,
				Confidence: NormalizedConfidence(output, state),
				Reason:     validation.Reason,
			}, nil
		}
		return &CompletionDecision{
			Decision:   DecisionReplan,
			NeedReplan: true,
			StopReason: StopReasonValidationFailed,
			Confidence: NormalizedConfidence(output, state),
			Reason:     validation.Reason,
		}, nil
	}
	if ReachedMaxIterations(state) {
		return &CompletionDecision{
			Decision:   DecisionDone,
			StopReason: StopReasonMaxIterations,
			Confidence: NormalizedConfidence(output, state),
			Reason:     "loop iteration budget exhausted",
		}, nil
	}
	return &CompletionDecision{
		Decision:   DecisionReplan,
		NeedReplan: true,
		StopReason: StopReasonBlocked,
		Confidence: NormalizedConfidence(output, state),
		Reason:     "output content is empty",
	}, nil
}

func ReachedMaxIterations(state *State) bool {
	return state != nil && state.MaxIterations > 0 && state.Iteration >= state.MaxIterations
}

func ClassifyStopReason(msg string) StopReason {
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

func NormalizedConfidence(output *Output, state *State) float64 {
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

func CompletionValidationState(state *State, output *Output) ValidationStateView {
	status := ValidationStatusPassed
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
		if metaStatus := ValidationStatus(MetadataString(output.Metadata, "validation_status")); metaStatus != "" {
			status = WorseValidationStatus(status, metaStatus)
		}
		validationPending := false
		if pending, ok := MetadataBool(output.Metadata, "validation_pending"); ok && pending {
			validationPending = true
			status = WorseValidationStatus(status, ValidationStatusPending)
			reason = fallbackString(reason, fallbackMetadataReason(output.Metadata, "validation pending"))
			unresolvedItems = appendUniqueString(unresolvedItems, "complete validation")
		}
		if passed, ok := MetadataBool(output.Metadata, "validation_passed"); ok && !passed && !validationPending {
			status = WorseValidationStatus(status, ValidationStatusFailed)
			reason = fallbackString(reason, fallbackMetadataReason(output.Metadata, "validation failed"))
		}
		if acceptanceMet, ok := MetadataBool(output.Metadata, "acceptance_criteria_met", "acceptance_passed"); ok && !acceptanceMet {
			status = WorseValidationStatus(status, ValidationStatusPending)
			reason = fallbackString(reason, "acceptance criteria not met")
			unresolvedItems = appendUniqueString(unresolvedItems, "validate acceptance criteria")
		}
		if pending, ok := MetadataBool(output.Metadata, "tool_verification_pending", "verification_pending"); ok && pending {
			status = WorseValidationStatus(status, ValidationStatusPending)
			reason = fallbackString(reason, "tool verification pending")
			unresolvedItems = appendUniqueString(unresolvedItems, "tool verification pending")
		}
		if passed, ok := MetadataBool(output.Metadata, "tool_verification_passed", "verification_passed", "verified"); ok && !passed {
			status = WorseValidationStatus(status, ValidationStatusPending)
			reason = fallbackString(reason, "tool verification pending")
			unresolvedItems = appendUniqueString(unresolvedItems, "tool verification pending")
		}
		if values, ok := ContextStrings(output.Metadata, "unresolved_items", "remaining_work"); ok {
			unresolvedItems = normalizeStringSlice(append(unresolvedItems, values...))
		}
		if values, ok := ContextStrings(output.Metadata, "remaining_risks"); ok {
			remainingRisks = normalizeStringSlice(append(remainingRisks, values...))
		}
		reason = fallbackString(reason, MetadataString(output.Metadata, "validation_summary", "validation_reason", "validation_message"))
	}
	if len(unresolvedItems) > 0 || len(remainingRisks) > 0 {
		if status == "" || status == ValidationStatusPassed {
			status = ValidationStatusPending
		}
	}
	if status == "" {
		switch {
		case state != nil && len(state.AcceptanceCriteria) > 0:
			status = ValidationStatusPending
		case GoalRequiresValidation(state):
			status = ValidationStatusPending
		default:
			status = ValidationStatusPassed
		}
	}
	if reason == "" {
		reason = SummarizeValidationState(status, unresolvedItems, remainingRisks)
	}
	return ValidationStateView{
		Status:          status,
		Reason:          reason,
		UnresolvedItems: unresolvedItems,
		RemainingRisks:  remainingRisks,
	}
}

func GoalRequiresValidation(state *State) bool {
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

func NewValidationResult(status ValidationStatus, reason string) *ValidationResult {
	result := &ValidationResult{
		Status:   status,
		Reason:   strings.TrimSpace(reason),
		Metadata: map[string]any{},
	}
	FinalizeValidationResult(result)
	return result
}

func MergeValidationResult(target *ValidationResult, incoming *ValidationResult) {
	if target == nil || incoming == nil {
		return
	}
	FinalizeValidationResult(incoming)
	target.Status = WorseValidationStatus(target.Status, incoming.Status)
	target.AcceptanceCriteria = normalizeStringSlice(append(target.AcceptanceCriteria, incoming.AcceptanceCriteria...))
	target.UnresolvedItems = normalizeStringSlice(append(target.UnresolvedItems, incoming.UnresolvedItems...))
	target.RemainingRisks = normalizeStringSlice(append(target.RemainingRisks, incoming.RemainingRisks...))
	target.Issues = append(target.Issues, incoming.Issues...)
	if strings.TrimSpace(target.Reason) == "" || incoming.Status == ValidationStatusFailed || (incoming.Status == ValidationStatusPending && target.Status != ValidationStatusFailed) {
		target.Reason = incoming.Reason
	}
	if target.Metadata == nil {
		target.Metadata = map[string]any{}
	}
	for key, value := range incoming.Metadata {
		target.Metadata[key] = value
	}
}

func FinalizeValidationResult(result *ValidationResult) {
	if result == nil {
		return
	}
	result.AcceptanceCriteria = normalizeStringSlice(result.AcceptanceCriteria)
	result.UnresolvedItems = normalizeStringSlice(result.UnresolvedItems)
	result.RemainingRisks = normalizeStringSlice(result.RemainingRisks)
	switch result.Status {
	case ValidationStatusFailed, ValidationStatusPending, ValidationStatusPassed:
	default:
		result.Status = ValidationStatusPassed
	}
	result.Passed = result.Status == ValidationStatusPassed
	result.Pending = result.Status == ValidationStatusPending
	if result.Summary == "" {
		result.Summary = SummarizeValidationState(result.Status, result.UnresolvedItems, result.RemainingRisks)
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
		result.Metadata["validation_issues"] = append([]ValidationIssue(nil), result.Issues...)
	}
}

func WorseValidationStatus(left, right ValidationStatus) ValidationStatus {
	if validationStatusRank(right) > validationStatusRank(left) {
		return right
	}
	if left == "" {
		return ValidationStatusPassed
	}
	return left
}

func ValidateGeneric(input *Input, state *State, output *Output, err error) *ValidationResult {
	result := NewValidationResult(ValidationStatusPassed, "validation passed")
	result.AcceptanceCriteria = AcceptanceCriteriaForValidation(input, state)
	result.UnresolvedItems = UnresolvedItemsForValidation(state, output)
	result.RemainingRisks = RemainingRisksForValidation(state, output)
	if err != nil {
		result.Status = ValidationStatusFailed
		result.Reason = "validation skipped due to execution error"
		result.Issues = append(result.Issues, newValidationIssue("generic", "execution_error", "validation", result.Status, result.Reason))
		FinalizeValidationResult(result)
		return result
	}
	if output == nil {
		result.Status = ValidationStatusPending
		result.Reason = "output missing for validation"
		result.UnresolvedItems = append(result.UnresolvedItems, "produce validated output")
		result.Issues = append(result.Issues, newValidationIssue("generic", "missing_output", "validation", result.Status, result.Reason))
		FinalizeValidationResult(result)
		return result
	}

	acceptanceRequired := len(result.AcceptanceCriteria) > 0
	result.Metadata["acceptance_criteria_required"] = acceptanceRequired
	if acceptanceRequired {
		if value, ok := MetadataBool(output.Metadata, "acceptance_criteria_met", "acceptance_passed"); ok {
			result.Metadata["acceptance_criteria_met"] = value
			if !value {
				result.Status = WorseValidationStatus(result.Status, ValidationStatusPending)
				result.Reason = "acceptance criteria not met"
				result.UnresolvedItems = append(result.UnresolvedItems, "validate acceptance criteria")
				result.Issues = append(result.Issues, newValidationIssue("generic", "acceptance_not_met", "acceptance", result.Status, result.Reason))
			}
		} else {
			result.Status = ValidationStatusPending
			result.Reason = "acceptance criteria not yet validated"
			result.UnresolvedItems = append(result.UnresolvedItems, "validate acceptance criteria")
			result.Issues = append(result.Issues, newValidationIssue("generic", "acceptance_pending", "acceptance", result.Status, result.Reason))
			result.Metadata["acceptance_criteria_met"] = false
		}
	}

	explicitValidationPassed, hasExplicitValidationPassed := MetadataBool(output.Metadata, "validation_passed")
	explicitValidationPending, _ := MetadataBool(output.Metadata, "validation_pending")
	if hasExplicitValidationPassed && !explicitValidationPassed {
		result.Status = ValidationStatusFailed
		result.Reason = fallbackMetadataReason(output.Metadata, "validation failed")
		result.Issues = append(result.Issues, newValidationIssue("generic", "validation_failed", "validation", result.Status, result.Reason))
	}
	if explicitValidationPending {
		result.Status = WorseValidationStatus(result.Status, ValidationStatusPending)
		result.Reason = fallbackMetadataReason(output.Metadata, "validation pending")
		result.UnresolvedItems = append(result.UnresolvedItems, "complete validation")
		result.Issues = append(result.Issues, newValidationIssue("generic", "validation_pending", "validation", ValidationStatusPending, result.Reason))
	}
	if GoalRequiresValidation(state) && !hasExplicitValidationPassed && !explicitValidationPending && !acceptanceRequired {
		result.Status = WorseValidationStatus(result.Status, ValidationStatusPending)
		result.Reason = "validation required before completion"
		result.UnresolvedItems = append(result.UnresolvedItems, "add validation evidence")
		result.Issues = append(result.Issues, newValidationIssue("generic", "validation_required", "validation", ValidationStatusPending, result.Reason))
	}
	if len(result.UnresolvedItems) > 0 && result.Status == ValidationStatusPassed {
		result.Status = ValidationStatusPending
		if strings.TrimSpace(result.Reason) == "" {
			result.Reason = "unresolved items remain"
		}
	}
	if len(result.RemainingRisks) > 0 && result.Status == ValidationStatusPassed {
		result.Status = ValidationStatusPending
		if strings.TrimSpace(result.Reason) == "" {
			result.Reason = "remaining risks require validation"
		}
	}
	FinalizeValidationResult(result)
	return result
}

func ValidateToolVerification(input *Input, state *State, output *Output, err error) *ValidationResult {
	result := NewValidationResult(ValidationStatusPassed, "tool verification passed")
	if err != nil || output == nil {
		FinalizeValidationResult(result)
		return result
	}
	required := ToolVerificationRequired(input, state, output)
	result.Metadata["tool_verification_required"] = required
	if !required {
		FinalizeValidationResult(result)
		return result
	}
	if pending, ok := MetadataBool(output.Metadata, "tool_verification_pending", "verification_pending"); ok && pending {
		result.Status = ValidationStatusPending
		result.Reason = "tool verification pending"
		result.UnresolvedItems = append(result.UnresolvedItems, "verify tool-backed output")
		result.Issues = append(result.Issues, newValidationIssue("tool", "tool_verification_pending", "tool", result.Status, result.Reason))
		result.Metadata["tool_verification_pending"] = true
		FinalizeValidationResult(result)
		return result
	}
	if passed, ok := MetadataBool(output.Metadata, "tool_verification_passed", "verification_passed", "verified"); ok {
		result.Metadata["tool_verification_passed"] = passed
		if !passed {
			result.Status = ValidationStatusFailed
			result.Reason = "tool verification failed"
			result.Issues = append(result.Issues, newValidationIssue("tool", "tool_verification_failed", "tool", result.Status, result.Reason))
			FinalizeValidationResult(result)
			return result
		}
		FinalizeValidationResult(result)
		return result
	}
	result.Status = ValidationStatusPending
	result.Reason = "tool verification pending"
	result.UnresolvedItems = append(result.UnresolvedItems, "verify tool-backed output")
	result.Issues = append(result.Issues, newValidationIssue("tool", "tool_verification_missing", "tool", result.Status, result.Reason))
	result.Metadata["tool_verification_passed"] = false
	result.Metadata["tool_verification_pending"] = true
	FinalizeValidationResult(result)
	return result
}

func ValidateCodeTask(validator CodeWarningProvider, input *Input, state *State, output *Output, err error) *ValidationResult {
	result := NewValidationResult(ValidationStatusPassed, "code verification passed")
	if err != nil || output == nil {
		FinalizeValidationResult(result)
		return result
	}
	required := CodeTaskRequired(input, state, output)
	result.Metadata["code_verification_required"] = required
	if !required {
		FinalizeValidationResult(result)
		return result
	}
	if pending, ok := MetadataBool(output.Metadata, "code_verification_pending", "tests_pending"); ok && pending {
		result.Status = ValidationStatusPending
		result.Reason = "code verification pending"
		result.UnresolvedItems = append(result.UnresolvedItems, "run tests or verification for code changes")
		result.Issues = append(result.Issues, newValidationIssue("code", "code_verification_pending", "code", result.Status, result.Reason))
		FinalizeValidationResult(result)
		return result
	}
	if passed, ok := MetadataBool(output.Metadata, "code_verification_passed", "tests_passed", "tests_green"); ok {
		result.Metadata["code_verification_passed"] = passed
		if !passed {
			result.Status = ValidationStatusFailed
			result.Reason = "code verification failed"
			result.Issues = append(result.Issues, newValidationIssue("code", "code_verification_failed", "code", result.Status, result.Reason))
			FinalizeValidationResult(result)
			return result
		}
	} else {
		result.Status = ValidationStatusPending
		result.Reason = "code task requires tests or verification evidence"
		result.UnresolvedItems = append(result.UnresolvedItems, "run tests or verification for code changes")
		result.Issues = append(result.Issues, newValidationIssue("code", "code_verification_missing", "code", result.Status, result.Reason))
	}
	if lang, code, ok := CodeSnippetForValidation(output); ok && validator != nil {
		warnings := validator.Validate(lang, code)
		if len(warnings) > 0 {
			result.Status = WorseValidationStatus(result.Status, ValidationStatusPending)
			result.RemainingRisks = append(result.RemainingRisks, warnings...)
			result.Issues = append(result.Issues, newValidationIssue("code", "code_risk_detected", "code", ValidationStatusPending, strings.Join(warnings, "; ")))
		}
	}
	FinalizeValidationResult(result)
	return result
}

func AcceptanceCriteriaForValidation(input *Input, state *State) []string {
	if state != nil && len(state.AcceptanceCriteria) > 0 {
		return cloneStringSlice(state.AcceptanceCriteria)
	}
	if input == nil || len(input.Context) == 0 {
		return nil
	}
	if values, ok := ContextStrings(input.Context, "acceptance_criteria"); ok {
		return values
	}
	return nil
}

func UnresolvedItemsForValidation(state *State, output *Output) []string {
	var items []string
	if state != nil {
		items = append(items, state.UnresolvedItems...)
	}
	if output != nil {
		if values, ok := ContextStrings(output.Metadata, "unresolved_items", "remaining_work"); ok {
			items = append(items, values...)
		}
	}
	return normalizeStringSlice(items)
}

func RemainingRisksForValidation(state *State, output *Output) []string {
	var risks []string
	if state != nil {
		risks = append(risks, state.RemainingRisks...)
	}
	if output != nil {
		if values, ok := ContextStrings(output.Metadata, "remaining_risks"); ok {
			risks = append(risks, values...)
		}
	}
	return normalizeStringSlice(risks)
}

func ToolVerificationRequired(input *Input, state *State, output *Output) bool {
	if contextBool(input, "tool_verification_required") {
		return true
	}
	if output == nil {
		return false
	}
	if MetadataBoolTrue(output.Metadata, "tool_verification_required") {
		return true
	}
	if metadataHasAny(output.Metadata, "tool_used", "tool_name", "tool_calls", "tool_results", "search_results", "tool_verification_passed", "tool_verification_pending", "verification_pending", "verification_passed", "verified") {
		return true
	}
	return state != nil && strings.EqualFold(state.SelectedMode, "rewoo")
}

func CodeTaskRequired(input *Input, state *State, output *Output) bool {
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

func CodeSnippetForValidation(output *Output) (CodeValidationLanguage, string, bool) {
	if output == nil || len(output.Metadata) == 0 {
		return "", "", false
	}
	rawCode := MetadataString(output.Metadata, "generated_code", "code")
	rawLang := strings.ToLower(MetadataString(output.Metadata, "code_language", "language"))
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

func MetadataBool(values map[string]any, keys ...string) (bool, bool) {
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

func MetadataBoolTrue(values map[string]any, keys ...string) bool {
	flag, ok := MetadataBool(values, keys...)
	return ok && flag
}

func MetadataString(values map[string]any, keys ...string) string {
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

func ContextStrings(values map[string]any, keys ...string) ([]string, bool) {
	return checkpointcore.ContextStrings(values, keys...)
}

func SummarizeValidationState(status ValidationStatus, unresolvedItems, remainingRisks []string) string {
	return checkpointcore.SummarizeValidationState(string(status), unresolvedItems, remainingRisks)
}

func contextBool(input *Input, key string) bool {
	if input == nil || len(input.Context) == 0 {
		return false
	}
	raw, ok := input.Context[key]
	if !ok {
		return false
	}
	flag, ok := raw.(bool)
	return ok && flag
}

func contextString(input *Input, key string) string {
	if input == nil || len(input.Context) == 0 {
		return ""
	}
	raw, ok := input.Context[key]
	if !ok {
		return ""
	}
	text, ok := raw.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
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

func newValidationIssue(validator string, code string, category string, status ValidationStatus, message string) ValidationIssue {
	return ValidationIssue{
		Validator: strings.TrimSpace(validator),
		Code:      strings.TrimSpace(code),
		Category:  strings.TrimSpace(category),
		Status:    status,
		Message:   strings.TrimSpace(message),
	}
}

func fallbackMetadataReason(metadata map[string]any, fallback string) string {
	reason := MetadataString(metadata, "validation_reason", "validation_message")
	if reason == "" {
		return fallback
	}
	return reason
}

func validationStatusRank(status ValidationStatus) int {
	switch status {
	case ValidationStatusFailed:
		return 3
	case ValidationStatusPending:
		return 2
	case ValidationStatusPassed:
		return 1
	default:
		return 0
	}
}

func cloneStringSlice(values []string) []string {
	return checkpointcore.CloneStringSlice(values)
}

func normalizeStringSlice(values []string) []string {
	return checkpointcore.NormalizeStringSlice(values)
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
