package core

import (
	"context"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/agent/capabilities/reasoning"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

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
