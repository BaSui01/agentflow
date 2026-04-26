package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/agent/capabilities/reasoning"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// LoopExecutor executes loop iterations with planning, acting, observing, and evaluating.
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
		recordReflectionCritique(state, result)
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
	recordReflectionCritique(state, result)
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
		critiques := mergeReflectionCritiques(
			append([]Critique(nil), state.reflectionCritiques...),
			reflectionCritiquesFromObservations(state.Observations),
			outputReflectionCritiques(finalOutput),
		)
		if len(critiques) > 0 {
			finalOutput.Metadata["reflection_iterations"] = len(critiques)
			finalOutput.Metadata["reflection_critiques"] = critiques
			finalOutput.Metadata["reflection_critique"] = critiques[len(critiques)-1]
		}
	}
	if execErr != nil {
		return finalOutput, execErr
	}
	return finalOutput, nil
}

func recordReflectionCritique(state *LoopState, result *LoopReflectionResult) {
	if state == nil || result == nil {
		return
	}
	switch {
	case result.Critique != nil:
		state.reflectionCritiques = append(state.reflectionCritiques, *result.Critique)
	case result.Observation != nil && result.Observation.Metadata != nil:
		if raw, ok := result.Observation.Metadata["reflection_critique"]; ok {
			if critique, ok := coerceCritique(raw); ok {
				state.reflectionCritiques = append(state.reflectionCritiques, critique)
			}
		}
	}
}

func mergeReflectionCritiques(groups ...[]Critique) []Critique {
	if len(groups) == 0 {
		return nil
	}
	merged := make([]Critique, 0, 4)
	seen := make(map[string]struct{}, 4)
	for _, group := range groups {
		for _, critique := range group {
			key := critique.RawFeedback + "|" + fmt.Sprintf("%.4f|%t|%s|%s", critique.Score, critique.IsGood, strings.Join(critique.Issues, "\x00"), strings.Join(critique.Suggestions, "\x00"))
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			merged = append(merged, critique)
		}
	}
	if len(merged) == 0 {
		return nil
	}
	return merged
}
