package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	llmtools "github.com/BaSui01/agentflow/llm/capabilities/tools"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

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
