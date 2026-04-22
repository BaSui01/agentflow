package core

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/BaSui01/agentflow/agent/capabilities/reasoning"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

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
