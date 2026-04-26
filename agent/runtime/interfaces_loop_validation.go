package runtime

import (
	"context"
	agentcore "github.com/BaSui01/agentflow/agent/core"
	executionloop "github.com/BaSui01/agentflow/agent/execution/loop"
)

// CompletionJudge decides whether the loop can stop or must continue.
type CompletionJudge interface {
	Judge(ctx context.Context, state *LoopState, output *Output, err error) (*CompletionDecision, error)
}

type LoopValidationStatus = agentcore.LoopValidationStatus

const (
	LoopValidationStatusPassed  LoopValidationStatus = agentcore.LoopValidationStatusPassed
	LoopValidationStatusPending LoopValidationStatus = agentcore.LoopValidationStatusPending
	LoopValidationStatusFailed  LoopValidationStatus = agentcore.LoopValidationStatusFailed
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
	decision, judgeErr := executionloop.JudgeDefault(ctx, loopExecutionStateFromRoot(state), loopExecutionOutputFromRoot(output), err)
	if judgeErr != nil {
		return nil, judgeErr
	}
	return rootCompletionDecisionFromLoop(decision), nil
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
	return rootLoopValidationResultFromLoop(executionloop.ValidateGeneric(loopExecutionInputFromRoot(input), loopExecutionStateFromRoot(state), loopExecutionOutputFromRoot(output), err)), nil
}

type ToolVerificationLoopValidator struct{}

func (ToolVerificationLoopValidator) Validate(_ context.Context, input *Input, state *LoopState, output *Output, err error) (*LoopValidationResult, error) {
	return rootLoopValidationResultFromLoop(executionloop.ValidateToolVerification(loopExecutionInputFromRoot(input), loopExecutionStateFromRoot(state), loopExecutionOutputFromRoot(output), err)), nil
}

type CodeTaskLoopValidator struct {
	codeValidator *CodeValidator
}

func NewCodeTaskLoopValidator() CodeTaskLoopValidator {
	return CodeTaskLoopValidator{codeValidator: NewCodeValidator()}
}

func (v CodeTaskLoopValidator) Validate(_ context.Context, input *Input, state *LoopState, output *Output, err error) (*LoopValidationResult, error) {
	return rootLoopValidationResultFromLoop(executionloop.ValidateCodeTask(loopCodeWarningProviderAdapter{validator: v.codeValidator}, loopExecutionInputFromRoot(input), loopExecutionStateFromRoot(state), loopExecutionOutputFromRoot(output), err)), nil
}

func newLoopValidationResult(status LoopValidationStatus, reason string) *LoopValidationResult {
	return rootLoopValidationResultFromLoop(executionloop.NewValidationResult(loopValidationStatusToSubpkg(status), reason))
}

func mergeLoopValidationResult(target *LoopValidationResult, incoming *LoopValidationResult) {
	if target == nil || incoming == nil {
		return
	}
	merged := rootLoopValidationResultToSubpkg(target)
	executionloop.MergeValidationResult(merged, rootLoopValidationResultToSubpkg(incoming))
	*target = *rootLoopValidationResultFromLoop(merged)
}

func finalizeLoopValidationResult(result *LoopValidationResult) {
	if result == nil {
		return
	}
	normalized := rootLoopValidationResultToSubpkg(result)
	executionloop.FinalizeValidationResult(normalized)
	*result = *rootLoopValidationResultFromLoop(normalized)
}

func acceptanceCriteriaForValidation(input *Input, state *LoopState) []string {
	return executionloop.AcceptanceCriteriaForValidation(loopExecutionInputFromRoot(input), loopExecutionStateFromRoot(state))
}

func toolVerificationRequired(input *Input, state *LoopState, output *Output) bool {
	return executionloop.ToolVerificationRequired(loopExecutionInputFromRoot(input), loopExecutionStateFromRoot(state), loopExecutionOutputFromRoot(output))
}

func codeTaskRequired(input *Input, state *LoopState, output *Output) bool {
	return executionloop.CodeTaskRequired(loopExecutionInputFromRoot(input), loopExecutionStateFromRoot(state), loopExecutionOutputFromRoot(output))
}

func classifyStopReason(msg string) StopReason {
	return StopReason(executionloop.ClassifyStopReason(msg))
}

func appendUniqueString(values []string, value string) []string {
	return agentcore.AppendUniqueString(values, value)
}

func fallbackString(values ...string) string {
	return agentcore.FallbackString(values...)
}

type loopCodeWarningProviderAdapter struct {
	validator *CodeValidator
}

func (a loopCodeWarningProviderAdapter) Validate(lang executionloop.CodeValidationLanguage, code string) []string {
	if a.validator == nil {
		return nil
	}
	return a.validator.Validate(CodeValidationLanguage(lang), code)
}

func loopExecutionInputFromRoot(input *Input) *executionloop.Input {
	if input == nil {
		return nil
	}
	var contextMap map[string]any
	if input.Context != nil {
		contextMap = mapsClone(input.Context)
	}
	return &executionloop.Input{
		Content: input.Content,
		Context: contextMap,
	}
}

func loopExecutionOutputFromRoot(output *Output) *executionloop.Output {
	if output == nil {
		return nil
	}
	return &executionloop.Output{
		Content:  output.Content,
		Metadata: cloneMetadata(output.Metadata),
	}
}

func loopExecutionStateFromRoot(state *LoopState) *executionloop.State {
	if state == nil {
		return nil
	}
	return &executionloop.State{
		Goal:               state.Goal,
		CurrentStage:       string(state.CurrentStage),
		Iteration:          state.Iteration,
		MaxIterations:      state.MaxIterations,
		Decision:           string(state.Decision),
		StopReason:         string(state.StopReason),
		PlanSteps:          cloneStringSlice(state.Plan),
		NeedHuman:          state.NeedHuman,
		Confidence:         state.Confidence,
		ValidationStatus:   loopValidationStatusToSubpkg(state.ValidationStatus),
		ValidationSummary:  state.ValidationSummary,
		AcceptanceCriteria: cloneStringSlice(state.AcceptanceCriteria),
		UnresolvedItems:    cloneStringSlice(state.UnresolvedItems),
		RemainingRisks:     cloneStringSlice(state.RemainingRisks),
		SelectedMode:       state.SelectedReasoningMode,
	}
}

func loopValidationStatusToSubpkg(status LoopValidationStatus) executionloop.ValidationStatus {
	return executionloop.ValidationStatus(status)
}

func rootLoopValidationStatusFromSubpkg(status executionloop.ValidationStatus) LoopValidationStatus {
	return LoopValidationStatus(status)
}

func rootCompletionDecisionFromLoop(decision *executionloop.CompletionDecision) *CompletionDecision {
	if decision == nil {
		return nil
	}
	return &CompletionDecision{
		Solved:         decision.Solved,
		NeedReplan:     decision.NeedReplan,
		NeedReflection: decision.NeedReflection,
		NeedHuman:      decision.NeedHuman,
		Decision:       LoopDecision(decision.Decision),
		StopReason:     StopReason(decision.StopReason),
		Confidence:     decision.Confidence,
		Reason:         decision.Reason,
	}
}

func rootLoopValidationResultFromLoop(result *executionloop.ValidationResult) *LoopValidationResult {
	if result == nil {
		return nil
	}
	issues := make([]LoopValidationIssue, 0, len(result.Issues))
	for _, issue := range result.Issues {
		issues = append(issues, LoopValidationIssue{
			Validator: issue.Validator,
			Code:      issue.Code,
			Category:  issue.Category,
			Status:    rootLoopValidationStatusFromSubpkg(issue.Status),
			Message:   issue.Message,
		})
	}
	metadata := map[string]any{}
	for key, value := range result.Metadata {
		switch typed := value.(type) {
		case []executionloop.ValidationIssue:
			mapped := make([]LoopValidationIssue, 0, len(typed))
			for _, issue := range typed {
				mapped = append(mapped, LoopValidationIssue{
					Validator: issue.Validator,
					Code:      issue.Code,
					Category:  issue.Category,
					Status:    rootLoopValidationStatusFromSubpkg(issue.Status),
					Message:   issue.Message,
				})
			}
			metadata[key] = mapped
		default:
			metadata[key] = value
		}
	}
	return &LoopValidationResult{
		Status:             rootLoopValidationStatusFromSubpkg(result.Status),
		Passed:             result.Passed,
		Pending:            result.Pending,
		Reason:             result.Reason,
		Summary:            result.Summary,
		Issues:             issues,
		AcceptanceCriteria: cloneStringSlice(result.AcceptanceCriteria),
		UnresolvedItems:    cloneStringSlice(result.UnresolvedItems),
		RemainingRisks:     cloneStringSlice(result.RemainingRisks),
		Metadata:           metadata,
	}
}

func rootLoopValidationResultToSubpkg(result *LoopValidationResult) *executionloop.ValidationResult {
	if result == nil {
		return nil
	}
	issues := make([]executionloop.ValidationIssue, 0, len(result.Issues))
	for _, issue := range result.Issues {
		issues = append(issues, executionloop.ValidationIssue{
			Validator: issue.Validator,
			Code:      issue.Code,
			Category:  issue.Category,
			Status:    loopValidationStatusToSubpkg(issue.Status),
			Message:   issue.Message,
		})
	}
	metadata := map[string]any{}
	for key, value := range result.Metadata {
		switch typed := value.(type) {
		case []LoopValidationIssue:
			mapped := make([]executionloop.ValidationIssue, 0, len(typed))
			for _, issue := range typed {
				mapped = append(mapped, executionloop.ValidationIssue{
					Validator: issue.Validator,
					Code:      issue.Code,
					Category:  issue.Category,
					Status:    loopValidationStatusToSubpkg(issue.Status),
					Message:   issue.Message,
				})
			}
			metadata[key] = mapped
		default:
			metadata[key] = value
		}
	}
	return &executionloop.ValidationResult{
		Status:             loopValidationStatusToSubpkg(result.Status),
		Passed:             result.Passed,
		Pending:            result.Pending,
		Reason:             result.Reason,
		Summary:            result.Summary,
		Issues:             issues,
		AcceptanceCriteria: cloneStringSlice(result.AcceptanceCriteria),
		UnresolvedItems:    cloneStringSlice(result.UnresolvedItems),
		RemainingRisks:     cloneStringSlice(result.RemainingRisks),
		Metadata:           metadata,
	}
}

func mapsClone(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]any, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

type LoopValidationFuncAdapter LoopValidationFunc

func (f LoopValidationFuncAdapter) Validate(ctx context.Context, input *Input, state *LoopState, output *Output, err error) (*LoopValidationResult, error) {
	return f(ctx, input, state, output, err)
}
