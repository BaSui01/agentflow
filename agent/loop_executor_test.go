package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/agent/reasoning"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

type loopRuntimeEventRecorder struct {
	events []RuntimeStreamEvent
}

func (r *loopRuntimeEventRecorder) emit(event RuntimeStreamEvent) {
	r.events = append(r.events, event)
}

type loopExecutorCheckpointStoreStub struct {
	checkpoint *Checkpoint
}

func (s *loopExecutorCheckpointStoreStub) Save(_ context.Context, checkpoint *Checkpoint) error {
	s.checkpoint = checkpoint
	return nil
}

func (s *loopExecutorCheckpointStoreStub) Load(_ context.Context, checkpointID string) (*Checkpoint, error) {
	if s.checkpoint != nil && s.checkpoint.ID == checkpointID {
		return s.checkpoint, nil
	}
	return nil, errors.New("checkpoint not found")
}

func (s *loopExecutorCheckpointStoreStub) LoadLatest(context.Context, string) (*Checkpoint, error) {
	return nil, errors.New("not implemented")
}

func (s *loopExecutorCheckpointStoreStub) List(context.Context, string, int) ([]*Checkpoint, error) {
	return nil, nil
}

func (s *loopExecutorCheckpointStoreStub) Delete(context.Context, string) error       { return nil }
func (s *loopExecutorCheckpointStoreStub) DeleteThread(context.Context, string) error { return nil }
func (s *loopExecutorCheckpointStoreStub) LoadVersion(context.Context, string, int) (*Checkpoint, error) {
	return nil, errors.New("not implemented")
}
func (s *loopExecutorCheckpointStoreStub) ListVersions(context.Context, string) ([]CheckpointVersion, error) {
	return nil, nil
}
func (s *loopExecutorCheckpointStoreStub) Rollback(context.Context, string, int) error { return nil }

type loopExecutorSelectorStub struct {
	mode string
}

func (s loopExecutorSelectorStub) Select(_ context.Context, _ *Input, _ *LoopState, _ *reasoning.PatternRegistry, _ bool) ReasoningSelection {
	return ReasoningSelection{Mode: s.mode}
}

type loopExecutorJudgeStub struct {
	decisions []*CompletionDecision
	calls     int
}

func (j *loopExecutorJudgeStub) Judge(_ context.Context, _ *LoopState, _ *Output, _ error) (*CompletionDecision, error) {
	if j.calls >= len(j.decisions) {
		return &CompletionDecision{Decision: LoopDecisionDone, StopReason: StopReasonSolved, Solved: true}, nil
	}
	decision := j.decisions[j.calls]
	j.calls++
	return decision, nil
}

type loopReasoningRuntimeStub struct {
	selection     ReasoningSelection
	output        *Output
	err           error
	reflectResult *LoopReflectionResult
	selectCalls   int
	executeCalls  int
	reflectCalls  int
}

func (s *loopReasoningRuntimeStub) Select(_ context.Context, _ *Input, _ *LoopState) ReasoningSelection {
	s.selectCalls++
	return s.selection
}

func (s *loopReasoningRuntimeStub) Execute(_ context.Context, _ *Input, _ *LoopState, _ ReasoningSelection) (*Output, error) {
	s.executeCalls++
	return s.output, s.err
}

func (s *loopReasoningRuntimeStub) Reflect(_ context.Context, _ *Input, _ *Output, _ *LoopState) (*LoopReflectionResult, error) {
	s.reflectCalls++
	return s.reflectResult, nil
}

func TestLoopExecutor_ExecuteSolvedOnFirstIteration(t *testing.T) {
	var planned bool
	var observed bool

	executor := &LoopExecutor{
		MaxIterations: 3,
		Planner: func(_ context.Context, _ *Input, state *LoopState) (*PlanResult, error) {
			planned = true
			if state.Iteration != 1 {
				t.Fatalf("expected planner to run during iteration 1, got %d", state.Iteration)
			}
			return &PlanResult{Steps: []string{"inspect", "answer"}}, nil
		},
		StepExecutor: func(_ context.Context, input *Input, state *LoopState, selection ReasoningSelection) (*Output, error) {
			if selection.Mode != ReasoningModeReact {
				t.Fatalf("expected react mode, got %q", selection.Mode)
			}
			if len(state.Plan) != 2 {
				t.Fatalf("expected 2 planned steps, got %d", len(state.Plan))
			}
			return &Output{
				TraceID:      input.TraceID,
				Content:      "resolved",
				CheckpointID: "cp-1",
				Resumable:    true,
				Metadata: map[string]any{
					"confidence": 0.9,
				},
			}, nil
		},
		Observer: func(_ context.Context, feedback *Feedback, state *LoopState) error {
			observed = true
			if feedback.Type != "loop_iteration" {
				t.Fatalf("expected loop_iteration feedback, got %q", feedback.Type)
			}
			if state.Iteration != 1 {
				t.Fatalf("expected observe on iteration 1, got %d", state.Iteration)
			}
			if got := feedback.Data["checkpoint_id"]; got != "cp-1" {
				t.Fatalf("expected checkpoint_id cp-1, got %#v", got)
			}
			if got := feedback.Data["resumable"]; got != true {
				t.Fatalf("expected resumable true, got %#v", got)
			}
			return nil
		},
		Selector: loopExecutorSelectorStub{mode: ReasoningModeReact},
		Judge: &loopExecutorJudgeStub{decisions: []*CompletionDecision{
			{Solved: true, Decision: LoopDecisionDone, StopReason: StopReasonSolved, Confidence: 0.9, Reason: "done"},
		}},
		Logger: zap.NewNop(),
	}

	output, err := executor.Execute(context.Background(), &Input{TraceID: "trace-1", Content: "solve"})
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}
	if !planned {
		t.Fatalf("expected planner to be called")
	}
	if !observed {
		t.Fatalf("expected observer to be called")
	}
	if output.Content != "resolved" {
		t.Fatalf("expected resolved output, got %q", output.Content)
	}
	if output.IterationCount != 1 {
		t.Fatalf("expected iteration count 1, got %d", output.IterationCount)
	}
	if output.SelectedReasoningMode != ReasoningModeReact {
		t.Fatalf("expected selected mode react, got %q", output.SelectedReasoningMode)
	}
	if output.StopReason != string(StopReasonSolved) {
		t.Fatalf("expected solved stop reason, got %q", output.StopReason)
	}
	if output.CheckpointID != "cp-1" {
		t.Fatalf("expected checkpoint id cp-1, got %q", output.CheckpointID)
	}
	if !output.Resumable {
		t.Fatalf("expected resumable output")
	}
	if got := output.Metadata["loop_iteration_count"]; got != 1 {
		t.Fatalf("expected metadata loop_iteration_count 1, got %#v", got)
	}
	if got := output.Metadata["loop_decision"]; got != LoopDecisionDone {
		t.Fatalf("expected loop_decision done, got %#v", got)
	}
}

func TestLoopExecutor_ExecuteToolTaskInDefaultClosedLoop(t *testing.T) {
	var observed bool

	executor := &LoopExecutor{
		MaxIterations: 2,
		Planner: func(_ context.Context, _ *Input, _ *LoopState) (*PlanResult, error) {
			return &PlanResult{Steps: []string{"inspect_tool_result", "summarize"}}, nil
		},
		StepExecutor: func(_ context.Context, input *Input, state *LoopState, selection ReasoningSelection) (*Output, error) {
			if selection.Mode != ReasoningModeReWOO {
				t.Fatalf("expected rewoo mode, got %q", selection.Mode)
			}
			if state.CurrentStage != LoopStageAct {
				t.Fatalf("expected act stage, got %q", state.CurrentStage)
			}
			return &Output{
				TraceID: input.TraceID,
				Content: "tool-backed answer",
				Metadata: map[string]any{
					"tool_name": "web_search",
					"tool_used": true,
				},
			}, nil
		},
		Observer: func(_ context.Context, feedback *Feedback, _ *LoopState) error {
			observed = true
			if got := feedback.Data["output_metadata"].(map[string]any)["tool_name"]; got != "web_search" {
				t.Fatalf("expected tool_name web_search, got %#v", got)
			}
			return nil
		},
		Selector: loopExecutorSelectorStub{mode: ReasoningModeReWOO},
		Judge: &loopExecutorJudgeStub{decisions: []*CompletionDecision{
			{Solved: true, Decision: LoopDecisionDone, StopReason: StopReasonSolved, Reason: "tool task done"},
		}},
		Logger: zap.NewNop(),
	}

	output, err := executor.Execute(context.Background(), &Input{TraceID: "trace-tool", Content: "use tool"})
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}
	if !observed {
		t.Fatalf("expected observer to consume tool task feedback")
	}
	if output.Content != "tool-backed answer" {
		t.Fatalf("expected tool-backed answer, got %q", output.Content)
	}
	if output.SelectedReasoningMode != ReasoningModeReWOO {
		t.Fatalf("expected rewoo mode, got %q", output.SelectedReasoningMode)
	}
	if got := output.Metadata["tool_name"]; got != "web_search" {
		t.Fatalf("expected tool_name metadata, got %#v", got)
	}
}

func TestLoopExecutor_ExecuteReplansThenSucceeds(t *testing.T) {
	var planCalls int
	var stepCalls int

	executor := &LoopExecutor{
		MaxIterations: 3,
		Planner: func(_ context.Context, _ *Input, state *LoopState) (*PlanResult, error) {
			planCalls++
			return &PlanResult{Steps: []string{"iteration", string(rune('0' + state.Iteration))}}, nil
		},
		StepExecutor: func(_ context.Context, _ *Input, state *LoopState, _ ReasoningSelection) (*Output, error) {
			stepCalls++
			if stepCalls == 1 {
				return &Output{Content: "   "}, nil
			}
			if state.Iteration != 2 {
				t.Fatalf("expected second act iteration, got %d", state.Iteration)
			}
			return &Output{Content: "final answer"}, nil
		},
		Selector: loopExecutorSelectorStub{mode: ReasoningModePlanAndExecute},
		Judge: &loopExecutorJudgeStub{decisions: []*CompletionDecision{
			{Decision: LoopDecisionReplan, StopReason: StopReasonBlocked, Reason: "retry with new plan"},
			{Solved: true, Decision: LoopDecisionDone, StopReason: StopReasonSolved, Reason: "done"},
		}},
		Logger: zap.NewNop(),
	}

	output, err := executor.Execute(context.Background(), &Input{Content: "complex task"})
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}
	if planCalls != 2 {
		t.Fatalf("expected planner to run twice, got %d", planCalls)
	}
	if stepCalls != 2 {
		t.Fatalf("expected step executor to run twice, got %d", stepCalls)
	}
	if output.IterationCount != 2 {
		t.Fatalf("expected iteration count 2, got %d", output.IterationCount)
	}
	if output.SelectedReasoningMode != ReasoningModePlanAndExecute {
		t.Fatalf("expected plan_and_execute mode, got %q", output.SelectedReasoningMode)
	}
	if output.StopReason != string(StopReasonSolved) {
		t.Fatalf("expected solved stop reason, got %q", output.StopReason)
	}
	if got := output.Metadata["loop_plan"]; got == nil {
		t.Fatalf("expected loop_plan metadata to be populated")
	}
}

func TestLoopExecutor_ExecuteReturnsTerminalErrorWithState(t *testing.T) {
	executor := &LoopExecutor{
		MaxIterations: 2,
		StepExecutor: func(_ context.Context, _ *Input, _ *LoopState, _ ReasoningSelection) (*Output, error) {
			return nil, errors.New("tool execution failed")
		},
		Selector: loopExecutorSelectorStub{mode: ReasoningModeReWOO},
		Judge: &loopExecutorJudgeStub{decisions: []*CompletionDecision{
			{Decision: LoopDecisionDone, StopReason: StopReasonToolFailureUnrecoverable, Reason: "fatal tool failure"},
		}},
		Logger: zap.NewNop(),
	}

	output, err := executor.Execute(context.Background(), &Input{Content: "use tools"})
	if err == nil {
		t.Fatalf("expected terminal error")
	}
	if output == nil {
		t.Fatalf("expected output envelope on error")
	}
	if output.IterationCount != 1 {
		t.Fatalf("expected iteration count 1, got %d", output.IterationCount)
	}
	if output.StopReason != string(StopReasonToolFailureUnrecoverable) {
		t.Fatalf("expected tool failure stop reason, got %q", output.StopReason)
	}
	if output.SelectedReasoningMode != ReasoningModeReWOO {
		t.Fatalf("expected rewoo mode, got %q", output.SelectedReasoningMode)
	}
}

func TestLoopExecutor_ExecuteStopsAtIterationBudget(t *testing.T) {
	executor := &LoopExecutor{
		MaxIterations: 1,
		StepExecutor: func(_ context.Context, _ *Input, _ *LoopState, _ ReasoningSelection) (*Output, error) {
			return &Output{Content: ""}, nil
		},
		Selector: loopExecutorSelectorStub{mode: ReasoningModeReact},
		Judge: &loopExecutorJudgeStub{decisions: []*CompletionDecision{
			{Decision: LoopDecisionContinue, StopReason: "", Reason: "need another pass"},
		}},
		Logger: zap.NewNop(),
	}

	output, err := executor.Execute(context.Background(), &Input{Content: "loop"})
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}
	if output.StopReason != string(StopReasonMaxIterations) {
		t.Fatalf("expected max_iterations stop reason, got %q", output.StopReason)
	}
	if output.IterationCount != 1 {
		t.Fatalf("expected iteration count 1, got %d", output.IterationCount)
	}
}

func TestLoopExecutor_UsesReasoningRuntimeWhenProvided(t *testing.T) {
	runtimeStub := &loopReasoningRuntimeStub{
		selection: ReasoningSelection{Mode: ReasoningModePlanAndExecute},
		output:    &Output{Content: "runtime-output"},
	}

	executor := &LoopExecutor{
		MaxIterations:    2,
		ReasoningRuntime: runtimeStub,
		Judge: &loopExecutorJudgeStub{decisions: []*CompletionDecision{
			{Solved: true, Decision: LoopDecisionDone, StopReason: StopReasonSolved, Reason: "done"},
		}},
		Logger: zap.NewNop(),
	}

	output, err := executor.Execute(context.Background(), &Input{Content: "solve"})
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}
	if runtimeStub.selectCalls != 1 {
		t.Fatalf("expected reasoning runtime Select to be called once, got %d", runtimeStub.selectCalls)
	}
	if runtimeStub.executeCalls != 1 {
		t.Fatalf("expected reasoning runtime Execute to be called once, got %d", runtimeStub.executeCalls)
	}
	if output.Content != "runtime-output" {
		t.Fatalf("expected runtime output, got %q", output.Content)
	}
	if output.SelectedReasoningMode != ReasoningModePlanAndExecute {
		t.Fatalf("expected runtime-selected mode, got %q", output.SelectedReasoningMode)
	}
}

func TestLoopExecutor_ExecuteReturnsObserverError(t *testing.T) {
	executor := &LoopExecutor{
		MaxIterations: 2,
		StepExecutor: func(_ context.Context, _ *Input, _ *LoopState, _ ReasoningSelection) (*Output, error) {
			return &Output{Content: "partial"}, nil
		},
		Observer: func(_ context.Context, _ *Feedback, _ *LoopState) error {
			return errors.New("observer failed")
		},
		Selector: loopExecutorSelectorStub{mode: ReasoningModeReact},
		Logger:   zap.NewNop(),
	}

	output, err := executor.Execute(context.Background(), &Input{Content: "loop"})
	if err == nil {
		t.Fatalf("expected observer error")
	}
	if output == nil {
		t.Fatalf("expected output envelope on observer error")
	}
	if output.StopReason != string(StopReasonBlocked) {
		t.Fatalf("expected blocked stop reason, got %q", output.StopReason)
	}
}

func TestLoopExecutor_ExecuteRequiresStepExecutor(t *testing.T) {
	executor := &LoopExecutor{}

	output, err := executor.Execute(context.Background(), &Input{Content: "loop"})
	if err == nil {
		t.Fatalf("expected missing step executor error")
	}
	if output != nil {
		t.Fatalf("expected nil output when step executor is missing")
	}
}

func TestLoopExecutor_ExecuteReflectsWithinLoop(t *testing.T) {
	var stepCalls int

	executor := &LoopExecutor{
		MaxIterations: 2,
		StepExecutor: func(_ context.Context, input *Input, _ *LoopState, _ ReasoningSelection) (*Output, error) {
			stepCalls++
			if stepCalls == 1 {
				return &Output{Content: "draft", Metadata: map[string]any{"confidence": 0.4}}, nil
			}
			if input.Content != "refined prompt" {
				t.Fatalf("expected refined prompt, got %q", input.Content)
			}
			return &Output{Content: "final", Metadata: map[string]any{"confidence": 0.9}}, nil
		},
		Selector: loopExecutorSelectorStub{mode: ReasoningModeReflection},
		Judge: &loopExecutorJudgeStub{decisions: []*CompletionDecision{
			{Decision: LoopDecisionReflect, NeedReflection: true, StopReason: StopReasonBlocked, Reason: "needs reflection"},
			{Decision: LoopDecisionDone, Solved: true, StopReason: StopReasonSolved, Reason: "resolved"},
		}},
		ReflectionStep: func(_ context.Context, _ *Input, _ *Output, _ *LoopState) (*LoopReflectionResult, error) {
			return &LoopReflectionResult{
				NextInput: &Input{Content: "refined prompt"},
				Observation: &LoopObservation{
					Metadata: map[string]any{
						"reflection_critique": Critique{Score: 0.4, IsGood: false},
					},
				},
			}, nil
		},
		Logger: zap.NewNop(),
	}

	output, err := executor.Execute(context.Background(), &Input{Content: "initial"})
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}
	if stepCalls != 2 {
		t.Fatalf("expected 2 step executions, got %d", stepCalls)
	}
	if got := output.Metadata["reflection_iterations"]; got != 1 {
		t.Fatalf("expected reflection_iterations 1, got %#v", got)
	}
}

func TestLoopExecutor_ExecuteEscalatesToHuman(t *testing.T) {
	executor := &LoopExecutor{
		MaxIterations: 2,
		StepExecutor: func(_ context.Context, _ *Input, _ *LoopState, _ ReasoningSelection) (*Output, error) {
			return &Output{Content: "needs approval"}, nil
		},
		Selector: loopExecutorSelectorStub{mode: ReasoningModeReact},
		Judge: &loopExecutorJudgeStub{decisions: []*CompletionDecision{
			{Decision: LoopDecisionEscalate, NeedHuman: true, StopReason: StopReasonNeedHuman, Reason: "requires human approval"},
		}},
		Logger: zap.NewNop(),
	}

	output, err := executor.Execute(context.Background(), &Input{Content: "approve payment"})
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}
	if output.StopReason != string(StopReasonNeedHuman) {
		t.Fatalf("expected need_human stop reason, got %q", output.StopReason)
	}
	if got := output.Metadata["loop_need_human"]; got != true {
		t.Fatalf("expected loop_need_human true, got %#v", got)
	}
	if got := output.Metadata["loop_decision"]; got != LoopDecisionEscalate {
		t.Fatalf("expected loop_decision escalate, got %#v", got)
	}
}

func TestLoopExecutor_ExecuteEmitsClosedLoopStatusEvents(t *testing.T) {
	recorder := &loopRuntimeEventRecorder{}
	ctx := WithRuntimeStreamEmitter(context.Background(), recorder.emit)
	executor := &LoopExecutor{
		MaxIterations: 2,
		Planner: func(_ context.Context, _ *Input, _ *LoopState) (*PlanResult, error) {
			return &PlanResult{Steps: []string{"inspect", "answer"}}, nil
		},
		StepExecutor: func(_ context.Context, _ *Input, _ *LoopState, _ ReasoningSelection) (*Output, error) {
			return &Output{Content: "done"}, nil
		},
		Observer: func(_ context.Context, _ *Feedback, _ *LoopState) error {
			return nil
		},
		Selector: loopExecutorSelectorStub{mode: ReasoningModePlanAndExecute},
		Judge: &loopExecutorJudgeStub{decisions: []*CompletionDecision{
			{Solved: true, Decision: LoopDecisionDone, StopReason: StopReasonSolved, Reason: "complete"},
		}},
		Logger: zap.NewNop(),
	}

	output, err := executor.Execute(ctx, &Input{Content: "closed loop"})
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}
	if output == nil {
		t.Fatalf("expected output")
	}

	var sawReasoningModeSelected bool
	var sawCompletionDecision bool
	var sawLoopStopped bool
	stageSeen := map[string]bool{}

	for _, event := range recorder.events {
		if event.Type != RuntimeStreamStatus {
			continue
		}
		stageSeen[event.CurrentStage] = true
		data, _ := event.Data.(map[string]any)
		status, _ := data["status"].(string)
		switch status {
		case "reasoning_mode_selected":
			sawReasoningModeSelected = true
			if event.SelectedMode != ReasoningModePlanAndExecute {
				t.Fatalf("expected selected mode plan_and_execute, got %q", event.SelectedMode)
			}
		case "completion_judge_decision":
			sawCompletionDecision = true
			if event.StopReason != string(StopReasonSolved) {
				t.Fatalf("expected solved stop reason, got %q", event.StopReason)
			}
		case "loop_stopped":
			sawLoopStopped = true
		}
	}

	if !sawReasoningModeSelected {
		t.Fatalf("expected reasoning_mode_selected status event")
	}
	if !sawCompletionDecision {
		t.Fatalf("expected completion_judge_decision status event")
	}
	if !sawLoopStopped {
		t.Fatalf("expected loop_stopped status event")
	}
	for _, stage := range []string{
		string(LoopStagePerceive),
		string(LoopStageAnalyze),
		string(LoopStagePlan),
		string(LoopStageAct),
		string(LoopStageObserve),
		"validate",
		string(LoopStageEvaluate),
	} {
		if !stageSeen[stage] {
			t.Fatalf("expected stage %q to appear in status stream", stage)
		}
	}
}

func TestLoopExecutor_ExecuteDisablePlannerSkipsPlannerAndPlanStage(t *testing.T) {
	recorder := &loopRuntimeEventRecorder{}
	ctx := WithRuntimeStreamEmitter(context.Background(), recorder.emit)
	var plannerCalls int

	executor := &LoopExecutor{
		MaxIterations:    2,
		ExecutionOptions: types.ExecutionOptions{Control: types.AgentControlOptions{DisablePlanner: true}},
		Planner: func(_ context.Context, _ *Input, _ *LoopState) (*PlanResult, error) {
			plannerCalls++
			return &PlanResult{Steps: []string{"should not run"}}, nil
		},
		StepExecutor: func(_ context.Context, _ *Input, state *LoopState, selection ReasoningSelection) (*Output, error) {
			if selection.Mode != ReasoningModeReact {
				t.Fatalf("expected react mode when planner is disabled, got %q", selection.Mode)
			}
			if len(state.Plan) != 0 {
				t.Fatalf("expected no loop plan when planner is disabled, got %#v", state.Plan)
			}
			if state.CurrentStage != LoopStageAct {
				t.Fatalf("expected act stage, got %q", state.CurrentStage)
			}
			return &Output{Content: "done"}, nil
		},
		Selector: loopExecutorSelectorStub{mode: ReasoningModePlanAndExecute},
		Judge: &loopExecutorJudgeStub{decisions: []*CompletionDecision{
			{Solved: true, Decision: LoopDecisionDone, StopReason: StopReasonSolved, Reason: "complete"},
		}},
		Logger: zap.NewNop(),
	}

	output, err := executor.Execute(ctx, &Input{
		Content: "specialist task",
		Context: map[string]any{
			"disable_planner": true,
		},
	})
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}
	if plannerCalls != 0 {
		t.Fatalf("expected planner to be skipped, got %d calls", plannerCalls)
	}
	if output.SelectedReasoningMode != ReasoningModeReact {
		t.Fatalf("expected react output mode, got %q", output.SelectedReasoningMode)
	}
	for _, event := range recorder.events {
		if event.CurrentStage == string(LoopStagePlan) {
			t.Fatalf("did not expect plan stage status event when planner is disabled")
		}
	}
}

func TestLoopExecutor_ExecuteHonorsTaskLevelTopLoopBudget(t *testing.T) {
	var stepCalls int

	executor := &LoopExecutor{
		MaxIterations:    5,
		ExecutionOptions: types.ExecutionOptions{Control: types.AgentControlOptions{MaxLoopIterations: 1}},
		StepExecutor: func(_ context.Context, input *Input, _ *LoopState, _ ReasoningSelection) (*Output, error) {
			stepCalls++
			if input.Context["max_loop_iterations"] != 1 {
				t.Fatalf("expected canonical task-level loop budget context to be preserved")
			}
			if input.Overrides == nil || input.Overrides.MaxLoopIterations == nil || *input.Overrides.MaxLoopIterations != 1 {
				t.Fatalf("expected MaxLoopIterations override to be preserved on input")
			}
			return &Output{Content: "still working"}, nil
		},
		Selector: loopExecutorSelectorStub{mode: ReasoningModeReact},
		Judge: &loopExecutorJudgeStub{decisions: []*CompletionDecision{
			{Decision: LoopDecisionContinue, Reason: "need another pass"},
			{Decision: LoopDecisionContinue, Reason: "need another pass"},
		}},
		Logger: zap.NewNop(),
	}

	output, err := executor.Execute(context.Background(), &Input{
		Content: "bounded task",
		Context: map[string]any{"max_loop_iterations": 1},
		Overrides: &RunConfig{
			MaxLoopIterations: IntPtr(1),
		},
	})
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}
	if output.IterationCount != 1 || stepCalls != 1 {
		t.Fatalf("expected task-level loop budget to cap execution at 1 iteration, got output=%d stepCalls=%d", output.IterationCount, stepCalls)
	}
	if output.StopReason != string(StopReasonMaxIterations) {
		t.Fatalf("expected task-level loop budget to stop with max_iterations, got %q", output.StopReason)
	}
}

func TestLoopExecutor_ExecuteDoesNotStopOnNonEmptyOutputWithoutValidation(t *testing.T) {
	executor := &LoopExecutor{
		MaxIterations: 2,
		StepExecutor: func(_ context.Context, _ *Input, _ *LoopState, _ ReasoningSelection) (*Output, error) {
			return &Output{
				Content: "candidate answer",
				Metadata: map[string]any{
					"acceptance_criteria_met":   false,
					"tool_verification_pending": true,
				},
			}, nil
		},
		Selector: loopExecutorSelectorStub{mode: ReasoningModeReact},
		Judge:    NewDefaultCompletionJudge(),
		Logger:   zap.NewNop(),
	}

	output, err := executor.Execute(context.Background(), &Input{Content: "only stop after validation"})
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}
	if output.IterationCount != 2 {
		t.Fatalf("expected loop to continue until validation passes or budget exhausts, got %d iterations", output.IterationCount)
	}
	if output.StopReason != string(StopReasonMaxIterations) {
		t.Fatalf("expected validation gap to keep loop open until max_iterations, got %q", output.StopReason)
	}
}

func TestLoopExecutor_ExecuteEmitsValidationStageStatusEvent(t *testing.T) {
	recorder := &loopRuntimeEventRecorder{}
	ctx := WithRuntimeStreamEmitter(context.Background(), recorder.emit)
	executor := &LoopExecutor{
		MaxIterations: 2,
		Planner: func(_ context.Context, _ *Input, _ *LoopState) (*PlanResult, error) {
			return &PlanResult{Steps: []string{"inspect", "validate", "answer"}}, nil
		},
		StepExecutor: func(_ context.Context, _ *Input, _ *LoopState, _ ReasoningSelection) (*Output, error) {
			return &Output{
				Content: "candidate",
				Metadata: map[string]any{
					"acceptance_criteria_met":  true,
					"tool_verification_passed": true,
				},
			}, nil
		},
		Observer: func(_ context.Context, _ *Feedback, _ *LoopState) error { return nil },
		Selector: loopExecutorSelectorStub{mode: ReasoningModePlanAndExecute},
		Judge: &loopExecutorJudgeStub{decisions: []*CompletionDecision{
			{Solved: true, Decision: LoopDecisionDone, StopReason: StopReasonSolved, Reason: "validated"},
		}},
		Logger: zap.NewNop(),
	}

	if _, err := executor.Execute(ctx, &Input{Content: "validate before accept"}); err != nil {
		t.Fatalf("execute returned error: %v", err)
	}

	validateStageIndex := -1
	completionDecisionIndex := -1
	for idx, event := range recorder.events {
		if event.Type != RuntimeStreamStatus {
			continue
		}
		data, _ := event.Data.(map[string]any)
		status, _ := data["status"].(string)
		if event.CurrentStage == "validate" && (status == "stage_changed" || status == "validation_checked") && validateStageIndex == -1 {
			validateStageIndex = idx
		}
		if status == "completion_judge_decision" && completionDecisionIndex == -1 {
			completionDecisionIndex = idx
		}
	}
	if validateStageIndex == -1 {
		t.Fatalf("expected validate stage status event to be emitted")
	}
	if completionDecisionIndex != -1 && validateStageIndex > completionDecisionIndex {
		t.Fatalf("expected validate stage status to precede completion_judge_decision")
	}
}

func TestLoopExecutor_ExecuteHonorsRunConfigMaxLoopIterationsOverride(t *testing.T) {
	var stepCalls int

	executor := &LoopExecutor{
		MaxIterations:    4,
		ExecutionOptions: types.ExecutionOptions{Control: types.AgentControlOptions{MaxLoopIterations: 1}},
		StepExecutor: func(_ context.Context, _ *Input, _ *LoopState, _ ReasoningSelection) (*Output, error) {
			stepCalls++
			return &Output{Content: "candidate"}, nil
		},
		Selector: loopExecutorSelectorStub{mode: ReasoningModeReact},
		Judge: &loopExecutorJudgeStub{decisions: []*CompletionDecision{
			{Decision: LoopDecisionContinue, Reason: "need validation"},
			{Decision: LoopDecisionContinue, Reason: "need validation"},
		}},
		Logger: zap.NewNop(),
	}

	output, err := executor.Execute(context.Background(), &Input{
		Content:   "budgeted task",
		Overrides: &RunConfig{MaxLoopIterations: IntPtr(1)},
	})
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}
	if output.IterationCount != 1 || stepCalls != 1 {
		t.Fatalf("expected RunConfig max_loop_iterations to cap execution at 1 iteration, got output=%d stepCalls=%d", output.IterationCount, stepCalls)
	}
	if output.StopReason != string(StopReasonMaxIterations) {
		t.Fatalf("expected RunConfig max_loop_iterations to stop with max_iterations, got %q", output.StopReason)
	}
}

func TestLoopExecutor_ExecuteEmitsValidationDecisionEventData(t *testing.T) {
	recorder := &loopRuntimeEventRecorder{}
	ctx := WithRuntimeStreamEmitter(context.Background(), recorder.emit)
	executor := &LoopExecutor{
		MaxIterations: 2,
		StepExecutor: func(_ context.Context, _ *Input, _ *LoopState, _ ReasoningSelection) (*Output, error) {
			return &Output{
				Content: "validated answer",
				Metadata: map[string]any{
					"acceptance_criteria_met":  true,
					"tool_verification_passed": true,
				},
			}, nil
		},
		Observer: func(_ context.Context, _ *Feedback, _ *LoopState) error { return nil },
		Selector: loopExecutorSelectorStub{mode: ReasoningModeReWOO},
		Judge: &loopExecutorJudgeStub{decisions: []*CompletionDecision{
			{Solved: true, Decision: LoopDecisionDone, StopReason: StopReasonSolved, Reason: "validated"},
		}},
		Logger: zap.NewNop(),
	}

	if _, err := executor.Execute(ctx, &Input{
		Content: "verify tool output before solving",
		Context: map[string]any{
			"acceptance_criteria":        []string{"must be checked"},
			"tool_verification_required": true,
			"validation_required":        true,
		},
	}); err != nil {
		t.Fatalf("execute returned error: %v", err)
	}

	for _, event := range recorder.events {
		if event.Type != RuntimeStreamStatus {
			continue
		}
		data, _ := event.Data.(map[string]any)
		status, _ := data["status"].(string)
		if status != "completion_judge_decision" {
			continue
		}
		if event.StopReason != string(StopReasonSolved) {
			t.Fatalf("expected solved stop reason after validation, got %q", event.StopReason)
		}
		if event.SelectedMode != ReasoningModeReWOO {
			t.Fatalf("expected verification-capable mode to be preserved, got %q", event.SelectedMode)
		}
		return
	}
	t.Fatalf("expected completion_judge_decision status event after validation")
}

func TestLoopExecutor_ExecuteResumesFromCheckpointContext(t *testing.T) {
	store := &loopExecutorCheckpointStoreStub{
		checkpoint: &Checkpoint{
			ID:        "cp-resume",
			ThreadID:  "trace-1",
			AgentID:   "agent-1",
			State:     StateRunning,
			CreatedAt: time.Now(),
			Metadata: map[string]any{
				"stop_reason": "blocked",
			},
			ExecutionContext: &ExecutionContext{
				CurrentNode: string(LoopStageAct),
				Variables: map[string]any{
					"goal":                    "resume task",
					"run_id":                  "trace-1",
					"iteration_count":         1,
					"current_step_id":         "step-2",
					"selected_reasoning_mode": ReasoningModePlanAndExecute,
					"plan":                    []string{"step-1", "step-2"},
					"acceptance_criteria":     []string{"tests pass"},
					"unresolved_items":        []string{"run integration tests"},
					"remaining_risks":         []string{"edge-case coverage"},
					"validation_status":       "pending",
					"validation_summary":      "unresolved: run integration tests; risks: edge-case coverage",
				},
			},
		},
	}
	manager := NewCheckpointManager(store, zap.NewNop())
	var seenState *LoopState
	executor := &LoopExecutor{
		MaxIterations:     3,
		CheckpointManager: manager,
		AgentID:           "agent-1",
		StepExecutor: func(_ context.Context, _ *Input, state *LoopState, _ ReasoningSelection) (*Output, error) {
			seenState = state
			return &Output{Content: "resumed answer"}, nil
		},
		Judge: &loopExecutorJudgeStub{decisions: []*CompletionDecision{
			{Solved: true, Decision: LoopDecisionDone, StopReason: StopReasonSolved},
		}},
		Logger: zap.NewNop(),
	}

	output, err := executor.Execute(context.Background(), &Input{
		TraceID: "trace-1",
		Content: "resume task",
		Context: map[string]any{"checkpoint_id": "cp-resume"},
	})
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}
	if seenState == nil {
		t.Fatalf("expected resumed loop state")
	}
	if seenState.CurrentStepID != "step-2" {
		t.Fatalf("expected resumed current step id, got %q", seenState.CurrentStepID)
	}
	if seenState.ValidationStatus != LoopValidationStatusPending {
		t.Fatalf("expected resumed validation_status pending, got %q", seenState.ValidationStatus)
	}
	if len(seenState.UnresolvedItems) == 0 || seenState.UnresolvedItems[0] != "run integration tests" {
		t.Fatalf("expected resumed unresolved_items to retain checkpoint item, got %#v", seenState.UnresolvedItems)
	}
	if seenState.SelectedReasoningMode != ReasoningModeReact {
		t.Fatalf("expected runtime-selected reasoning mode, got %q", seenState.SelectedReasoningMode)
	}
	if output.CheckpointID != "cp-resume" {
		t.Fatalf("expected checkpoint id propagated, got %q", output.CheckpointID)
	}
	if !output.Resumable {
		t.Fatalf("expected resumable output")
	}
}

func TestLoopExecutor_SaveCheckpointPersistsLoopResumeFields(t *testing.T) {
	store := &loopExecutorCheckpointStoreStub{}
	manager := NewCheckpointManager(store, zap.NewNop())
	executor := &LoopExecutor{
		MaxIterations:     3,
		CheckpointManager: manager,
		AgentID:           "agent-1",
		Planner: func(_ context.Context, _ *Input, _ *LoopState) (*PlanResult, error) {
			return &PlanResult{Steps: []string{"step-1", "step-2"}}, nil
		},
		StepExecutor: func(_ context.Context, _ *Input, _ *LoopState, _ ReasoningSelection) (*Output, error) {
			return &Output{Content: "partial"}, nil
		},
		Selector: loopExecutorSelectorStub{mode: ReasoningModePlanAndExecute},
		Judge: &loopExecutorJudgeStub{decisions: []*CompletionDecision{
			{Solved: true, Decision: LoopDecisionDone, StopReason: StopReasonSolved},
		}},
		Logger: zap.NewNop(),
	}

	ctx := types.WithRunID(context.Background(), "run-1")
	output, err := executor.Execute(ctx, &Input{TraceID: "trace-1", ChannelID: "thread-1", Content: "resume me"})
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}
	if output.CheckpointID == "" {
		t.Fatal("expected checkpoint id")
	}
	if store.checkpoint == nil {
		t.Fatal("expected checkpoint to be saved")
	}
	if store.checkpoint.AgentID != "agent-1" {
		t.Fatalf("expected agent_id persisted, got %q", store.checkpoint.AgentID)
	}
	if store.checkpoint.Metadata["run_id"] != "run-1" {
		t.Fatalf("expected run_id persisted, got %#v", store.checkpoint.Metadata["run_id"])
	}
	if store.checkpoint.Metadata["current_stage"] != "validate" {
		t.Fatalf("expected current_stage validate, got %#v", store.checkpoint.Metadata["current_stage"])
	}
	if store.checkpoint.Metadata["iteration_count"] != 1 {
		t.Fatalf("expected iteration_count 1, got %#v", store.checkpoint.Metadata["iteration_count"])
	}
	if store.checkpoint.Metadata["selected_reasoning_mode"] != ReasoningModePlanAndExecute {
		t.Fatalf("expected selected mode persisted, got %#v", store.checkpoint.Metadata["selected_reasoning_mode"])
	}
	if store.checkpoint.Metadata["validation_status"] != "passed" {
		t.Fatalf("expected validation_status passed, got %#v", store.checkpoint.Metadata["validation_status"])
	}
	if store.checkpoint.ValidationStatus != LoopValidationStatusPassed {
		t.Fatalf("expected checkpoint validation_status passed, got %q", store.checkpoint.ValidationStatus)
	}
	if store.checkpoint.ExecutionContext == nil {
		t.Fatal("expected execution context")
	}
	if store.checkpoint.ExecutionContext.Variables["current_step"] != "step-2" {
		t.Fatalf("expected current_step step-2, got %#v", store.checkpoint.ExecutionContext.Variables["current_step"])
	}
}

func TestLoopExecutor_ExecuteKeepsCodeTaskOpenWithoutVerificationEvidence(t *testing.T) {
	executor := &LoopExecutor{
		MaxIterations: 2,
		StepExecutor: func(_ context.Context, _ *Input, _ *LoopState, _ ReasoningSelection) (*Output, error) {
			return &Output{
				Content: "implemented code change",
				Metadata: map[string]any{
					"generated_code": "package main\nfunc main(){ println(\"hi\") }",
					"code_language":  "go",
				},
			}, nil
		},
		Selector: loopExecutorSelectorStub{mode: ReasoningModeReact},
		Judge:    NewDefaultCompletionJudge(),
		Logger:   zap.NewNop(),
	}

	output, err := executor.Execute(context.Background(), &Input{
		Content: "fix the Go bug and verify the result",
		Context: map[string]any{"task_type": "code"},
	})
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}
	if output.IterationCount != 2 {
		t.Fatalf("expected code task to keep looping until verification evidence exists, got %d iterations", output.IterationCount)
	}
	if output.StopReason != string(StopReasonMaxIterations) {
		t.Fatalf("expected missing code verification evidence to end by budget, got %q", output.StopReason)
	}
	if output.Metadata["validation_status"] != "pending" {
		t.Fatalf("expected pending validation_status, got %#v", output.Metadata["validation_status"])
	}
}
