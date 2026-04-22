package checkpointcore

import "strings"

type LoopStateData struct {
	LoopStateID           string
	RunID                 string
	AgentID               string
	Goal                  string
	Plan                  []string
	AcceptanceCriteria    []string
	UnresolvedItems       []string
	RemainingRisks        []string
	CurrentPlanID         string
	PlanVersion           int
	CurrentStepID         string
	CurrentStage          string
	Iteration             int
	MaxIterations         int
	Decision              string
	StopReason            string
	SelectedReasoningMode string
	Confidence            float64
	NeedHuman             bool
	CheckpointID          string
	Resumable             bool
	ValidationStatus      string
	ValidationSummary     string
	ObservationsSummary   string
	LastOutputSummary     string
	LastError             string
	Observations          []Observation
	LastOutputContent     string
}

func (d *LoopStateData) Variables() map[string]any {
	if d == nil {
		return nil
	}
	return map[string]any{
		"loop_state_id":           d.LoopStateID,
		"run_id":                  d.RunID,
		"agent_id":                d.AgentID,
		"goal":                    d.Goal,
		"plan":                    CloneStringSlice(d.Plan),
		"acceptance_criteria":     CloneStringSlice(d.AcceptanceCriteria),
		"unresolved_items":        CloneStringSlice(d.UnresolvedItems),
		"remaining_risks":         CloneStringSlice(d.RemainingRisks),
		"current_plan_id":         d.CurrentPlanID,
		"plan_version":            d.PlanVersion,
		"current_step":            d.CurrentStepID,
		"current_step_id":         d.CurrentStepID,
		"current_stage":           d.CurrentStage,
		"iteration":               d.Iteration,
		"iteration_count":         d.Iteration,
		"max_iterations":          d.MaxIterations,
		"decision":                d.Decision,
		"stop_reason":             d.StopReason,
		"selected_reasoning_mode": d.SelectedReasoningMode,
		"confidence":              d.Confidence,
		"need_human":              d.NeedHuman,
		"checkpoint_id":           d.CheckpointID,
		"resumable":               d.Resumable,
		"validation_status":       d.ValidationStatus,
		"validation_summary":      d.ValidationSummary,
		"observations_summary":    d.ObservationsSummary,
		"last_output_summary":     d.LastOutputSummary,
		"last_error":              d.LastError,
	}
}

func (d *LoopStateData) RestoreFromContext(values map[string]any, syncCurrentStep func() string) {
	if d == nil || len(values) == 0 {
		return
	}
	if value, ok := ContextString(values, "loop_state_id"); ok {
		d.LoopStateID = value
	}
	if value, ok := ContextString(values, "run_id"); ok {
		d.RunID = value
	}
	if value, ok := ContextString(values, "agent_id"); ok {
		d.AgentID = value
	}
	if value, ok := ContextString(values, "goal"); ok {
		d.Goal = value
	}
	if plan, ok := ContextStrings(values, "loop_plan", "plan"); ok {
		d.Plan = plan
	}
	if criteria, ok := ContextStrings(values, "acceptance_criteria"); ok {
		d.AcceptanceCriteria = criteria
	}
	if items, ok := ContextStrings(values, "unresolved_items"); ok {
		d.UnresolvedItems = items
	}
	if risks, ok := ContextStrings(values, "remaining_risks"); ok {
		d.RemainingRisks = risks
	}
	if value, ok := ContextString(values, "current_plan_id"); ok {
		d.CurrentPlanID = value
	}
	if value, ok := ContextInt(values, "plan_version"); ok {
		d.PlanVersion = value
	}
	if value, ok := ContextString(values, "current_step", "current_step_id"); ok {
		d.CurrentStepID = value
	}
	if value, ok := ContextString(values, "current_stage"); ok {
		d.CurrentStage = value
	}
	if value, ok := ContextInt(values, "iteration", "iteration_count", "loop_iteration_count"); ok {
		d.Iteration = value
	}
	if value, ok := ContextInt(values, "max_iterations"); ok && value > 0 {
		d.MaxIterations = value
	}
	if value, ok := ContextString(values, "decision"); ok {
		d.Decision = value
	}
	if value, ok := ContextString(values, "stop_reason", "loop_stop_reason"); ok {
		d.StopReason = value
	}
	if value, ok := ContextString(values, "selected_reasoning_mode"); ok {
		d.SelectedReasoningMode = value
	}
	if value, ok := ContextBool(values, "resumable"); ok {
		d.Resumable = value
	}
	if value, ok := ContextString(values, "checkpoint_id"); ok {
		d.CheckpointID = value
	}
	if value, ok := ContextString(values, "validation_status"); ok {
		d.ValidationStatus = value
	}
	if value, ok := ContextString(values, "validation_summary"); ok {
		d.ValidationSummary = value
	}
	if value, ok := ContextFloat(values, "confidence", "loop_confidence"); ok {
		d.Confidence = value
	}
	if value, ok := ContextBool(values, "need_human", "loop_need_human"); ok {
		d.NeedHuman = value
	}
	if value, ok := ContextString(values, "observations_summary"); ok {
		d.ObservationsSummary = value
	}
	if value, ok := ContextString(values, "last_output_summary"); ok {
		d.LastOutputSummary = value
	}
	if value, ok := ContextString(values, "last_error"); ok {
		d.LastError = value
	}
	d.Normalize(syncCurrentStep)
}

func (d *LoopStateData) Normalize(syncCurrentStep func() string) {
	if d == nil {
		return
	}
	if d.PlanVersion <= 0 && len(d.Plan) > 0 {
		d.PlanVersion = DerivePlanVersion(d.Observations)
		if d.PlanVersion <= 0 {
			d.PlanVersion = 1
		}
	}
	if d.CurrentPlanID == "" && d.PlanVersion > 0 {
		d.CurrentPlanID = BuildLoopPlanID(d.LoopStateID, d.PlanVersion)
	}
	d.AcceptanceCriteria = NormalizeStringSlice(d.AcceptanceCriteria)
	d.UnresolvedItems = NormalizeStringSlice(d.UnresolvedItems)
	d.RemainingRisks = NormalizeStringSlice(d.RemainingRisks)
	if d.CurrentStepID == "" && syncCurrentStep != nil {
		d.CurrentStepID = strings.TrimSpace(syncCurrentStep())
	}
	if d.ValidationSummary == "" {
		d.ValidationSummary = SummarizeValidationState(d.ValidationStatus, d.UnresolvedItems, d.RemainingRisks)
	}
	if d.ObservationsSummary == "" {
		d.ObservationsSummary = SummarizeObservations(d.Observations)
	}
	if d.LastOutputSummary == "" {
		d.LastOutputSummary = SummarizeLastOutput(d.LastOutputContent, d.Observations)
	}
	if d.LastError == "" {
		d.LastError = SummarizeLastError(d.Observations)
	}
}

func (d *LoopStateData) ApplyValidationResult(result ValidationResult, syncCurrentStep func() string) {
	if d == nil {
		return
	}
	if len(result.AcceptanceCriteria) > 0 {
		d.AcceptanceCriteria = NormalizeStringSlice(result.AcceptanceCriteria)
	}
	d.UnresolvedItems = NormalizeStringSlice(result.UnresolvedItems)
	d.RemainingRisks = NormalizeStringSlice(result.RemainingRisks)
	d.ValidationStatus = result.Status
	d.ValidationSummary = strings.TrimSpace(result.Summary)
	if d.ValidationSummary == "" {
		d.ValidationSummary = strings.TrimSpace(result.Reason)
	}
	d.Normalize(syncCurrentStep)
}

type ExecutionContextData struct {
	CurrentNode         string
	Variables           map[string]any
	LoopStateID         string
	RunID               string
	AgentID             string
	Goal                string
	AcceptanceCriteria  []string
	UnresolvedItems     []string
	RemainingRisks      []string
	CurrentPlanID       string
	PlanVersion         int
	CurrentStepID       string
	ValidationStatus    string
	ValidationSummary   string
	ObservationsSummary string
	LastOutputSummary   string
	LastError           string
}

func (d *ExecutionContextData) LoopContextValues() map[string]any {
	if d == nil {
		return nil
	}
	values := map[string]any{
		"current_stage":        d.CurrentNode,
		"loop_state_id":        d.LoopStateID,
		"run_id":               d.RunID,
		"agent_id":             d.AgentID,
		"goal":                 d.Goal,
		"acceptance_criteria":  CloneStringSlice(d.AcceptanceCriteria),
		"unresolved_items":     CloneStringSlice(d.UnresolvedItems),
		"remaining_risks":      CloneStringSlice(d.RemainingRisks),
		"current_plan_id":      d.CurrentPlanID,
		"plan_version":         d.PlanVersion,
		"current_step":         d.CurrentStepID,
		"current_step_id":      d.CurrentStepID,
		"validation_status":    d.ValidationStatus,
		"validation_summary":   d.ValidationSummary,
		"observations_summary": d.ObservationsSummary,
		"last_output_summary":  d.LastOutputSummary,
		"last_error":           d.LastError,
	}
	for key, value := range d.Variables {
		values[key] = value
	}
	return values
}

type CheckpointData struct {
	LoopStateID         string
	RunID               string
	AgentID             string
	Goal                string
	AcceptanceCriteria  []string
	UnresolvedItems     []string
	RemainingRisks      []string
	CurrentPlanID       string
	PlanVersion         int
	CurrentStepID       string
	ValidationStatus    string
	ValidationSummary   string
	ObservationsSummary string
	LastOutputSummary   string
	LastError           string
	Metadata            map[string]any
	ExecutionContext    *ExecutionContextData
}

func (d *CheckpointData) LoopContextValues() map[string]any {
	if d == nil {
		return nil
	}
	return map[string]any{
		"agent_id":             d.AgentID,
		"loop_state_id":        d.LoopStateID,
		"run_id":               d.RunID,
		"goal":                 d.Goal,
		"acceptance_criteria":  CloneStringSlice(d.AcceptanceCriteria),
		"unresolved_items":     CloneStringSlice(d.UnresolvedItems),
		"remaining_risks":      CloneStringSlice(d.RemainingRisks),
		"current_plan_id":      d.CurrentPlanID,
		"plan_version":         d.PlanVersion,
		"current_step":         d.CurrentStepID,
		"current_step_id":      d.CurrentStepID,
		"validation_status":    d.ValidationStatus,
		"validation_summary":   d.ValidationSummary,
		"observations_summary": d.ObservationsSummary,
		"last_output_summary":  d.LastOutputSummary,
		"last_error":           d.LastError,
	}
}

func (d *CheckpointData) Normalize() {
	if d == nil {
		return
	}
	if d.Metadata == nil {
		d.Metadata = make(map[string]any)
	}
	if d.ExecutionContext == nil {
		d.ExecutionContext = &ExecutionContextData{}
	}
	if d.ExecutionContext.Variables == nil {
		d.ExecutionContext.Variables = make(map[string]any)
	}
	for key, value := range d.Metadata {
		d.ExecutionContext.Variables[key] = value
	}
	for key, value := range d.ExecutionContext.Variables {
		d.Metadata[key] = value
	}

	d.LoopStateID = FirstNonEmptyString(d.LoopStateID, ContextStringValue(d.ExecutionContext.Variables, "loop_state_id"), ContextStringValue(d.Metadata, "loop_state_id"), d.ExecutionContext.LoopStateID)
	d.RunID = FirstNonEmptyString(d.RunID, ContextStringValue(d.ExecutionContext.Variables, "run_id"), ContextStringValue(d.Metadata, "run_id"), d.ExecutionContext.RunID)
	d.AgentID = FirstNonEmptyString(d.AgentID, ContextStringValue(d.ExecutionContext.Variables, "agent_id"), ContextStringValue(d.Metadata, "agent_id"), d.ExecutionContext.AgentID)
	d.Goal = FirstNonEmptyString(d.Goal, ContextStringValue(d.ExecutionContext.Variables, "goal"), ContextStringValue(d.Metadata, "goal"), d.ExecutionContext.Goal)
	d.CurrentPlanID = FirstNonEmptyString(d.CurrentPlanID, ContextStringValue(d.ExecutionContext.Variables, "current_plan_id"), ContextStringValue(d.Metadata, "current_plan_id"), d.ExecutionContext.CurrentPlanID)
	d.CurrentStepID = FirstNonEmptyString(d.CurrentStepID, ContextStringValue(d.ExecutionContext.Variables, "current_step_id"), ContextStringValue(d.ExecutionContext.Variables, "current_step"), ContextStringValue(d.Metadata, "current_step_id"), ContextStringValue(d.Metadata, "current_step"), d.ExecutionContext.CurrentStepID)

	d.AcceptanceCriteria = checkpointLoopStrings(d.AcceptanceCriteria, d.ExecutionContext.Variables, d.Metadata, "acceptance_criteria", d.ExecutionContext.AcceptanceCriteria)
	d.UnresolvedItems = checkpointLoopStrings(d.UnresolvedItems, d.ExecutionContext.Variables, d.Metadata, "unresolved_items", d.ExecutionContext.UnresolvedItems)
	d.RemainingRisks = checkpointLoopStrings(d.RemainingRisks, d.ExecutionContext.Variables, d.Metadata, "remaining_risks", d.ExecutionContext.RemainingRisks)

	if d.ValidationStatus == "" {
		if value, ok := ContextString(d.ExecutionContext.Variables, "validation_status"); ok {
			d.ValidationStatus = value
		} else if value, ok := ContextString(d.Metadata, "validation_status"); ok {
			d.ValidationStatus = value
		} else if d.ExecutionContext.ValidationStatus != "" {
			d.ValidationStatus = d.ExecutionContext.ValidationStatus
		}
	}
	d.ValidationSummary = FirstNonEmptyString(d.ValidationSummary, ContextStringValue(d.ExecutionContext.Variables, "validation_summary"), ContextStringValue(d.Metadata, "validation_summary"), d.ExecutionContext.ValidationSummary)
	d.ObservationsSummary = FirstNonEmptyString(d.ObservationsSummary, ContextStringValue(d.ExecutionContext.Variables, "observations_summary"), ContextStringValue(d.Metadata, "observations_summary"), d.ExecutionContext.ObservationsSummary)
	d.LastOutputSummary = FirstNonEmptyString(d.LastOutputSummary, ContextStringValue(d.ExecutionContext.Variables, "last_output_summary"), ContextStringValue(d.Metadata, "last_output_summary"), d.ExecutionContext.LastOutputSummary)
	d.LastError = FirstNonEmptyString(d.LastError, ContextStringValue(d.ExecutionContext.Variables, "last_error"), ContextStringValue(d.Metadata, "last_error"), d.ExecutionContext.LastError)
	if d.PlanVersion <= 0 {
		if value, ok := ContextInt(d.ExecutionContext.Variables, "plan_version"); ok {
			d.PlanVersion = value
		} else if value, ok := ContextInt(d.Metadata, "plan_version"); ok {
			d.PlanVersion = value
		} else if d.ExecutionContext.PlanVersion > 0 {
			d.PlanVersion = d.ExecutionContext.PlanVersion
		}
	}

	d.ExecutionContext.LoopStateID = FirstNonEmptyString(d.ExecutionContext.LoopStateID, d.LoopStateID)
	d.ExecutionContext.RunID = FirstNonEmptyString(d.ExecutionContext.RunID, d.RunID)
	d.ExecutionContext.AgentID = FirstNonEmptyString(d.ExecutionContext.AgentID, d.AgentID)
	d.ExecutionContext.Goal = FirstNonEmptyString(d.ExecutionContext.Goal, d.Goal)
	d.ExecutionContext.AcceptanceCriteria = CloneStringSlice(d.AcceptanceCriteria)
	d.ExecutionContext.UnresolvedItems = CloneStringSlice(d.UnresolvedItems)
	d.ExecutionContext.RemainingRisks = CloneStringSlice(d.RemainingRisks)
	d.ExecutionContext.CurrentPlanID = FirstNonEmptyString(d.ExecutionContext.CurrentPlanID, d.CurrentPlanID)
	if d.ExecutionContext.PlanVersion <= 0 {
		d.ExecutionContext.PlanVersion = d.PlanVersion
	}
	d.ExecutionContext.CurrentStepID = FirstNonEmptyString(d.ExecutionContext.CurrentStepID, d.CurrentStepID)
	if d.ExecutionContext.ValidationStatus == "" {
		d.ExecutionContext.ValidationStatus = d.ValidationStatus
	}
	d.ExecutionContext.ValidationSummary = FirstNonEmptyString(d.ExecutionContext.ValidationSummary, d.ValidationSummary)
	d.ExecutionContext.ObservationsSummary = FirstNonEmptyString(d.ExecutionContext.ObservationsSummary, d.ObservationsSummary)
	d.ExecutionContext.LastOutputSummary = FirstNonEmptyString(d.ExecutionContext.LastOutputSummary, d.LastOutputSummary)
	d.ExecutionContext.LastError = FirstNonEmptyString(d.ExecutionContext.LastError, d.LastError)

	for key, value := range d.LoopContextValues() {
		d.Metadata[key] = value
		d.ExecutionContext.Variables[key] = value
	}
}

func checkpointLoopStrings(current []string, variables, metadata map[string]any, key string, fallback []string) []string {
	if len(current) > 0 {
		return current
	}
	if values, ok := ContextStrings(variables, key); ok {
		return values
	}
	if values, ok := ContextStrings(metadata, key); ok {
		return values
	}
	return CloneStringSlice(fallback)
}
