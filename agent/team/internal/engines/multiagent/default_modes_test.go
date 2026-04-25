package multiagent

import (
	"context"
	"testing"
	"time"

	agent "github.com/BaSui01/agentflow/agent/runtime"
	"go.uber.org/zap"
)

type modeTestAgent struct {
	id      string
	name    string
	out     string
	latency time.Duration
}

func (m *modeTestAgent) ID() string                     { return m.id }
func (m *modeTestAgent) Name() string                   { return m.name }
func (m *modeTestAgent) Type() agent.AgentType          { return agent.TypeGeneric }
func (m *modeTestAgent) State() agent.State             { return agent.StateReady }
func (m *modeTestAgent) Init(context.Context) error     { return nil }
func (m *modeTestAgent) Teardown(context.Context) error { return nil }
func (m *modeTestAgent) Plan(context.Context, *agent.Input) (*agent.PlanResult, error) {
	return nil, nil
}
func (m *modeTestAgent) Observe(context.Context, *agent.Feedback) error { return nil }
func (m *modeTestAgent) Execute(_ context.Context, input *agent.Input) (*agent.Output, error) {
	return &agent.Output{
		TraceID:  input.TraceID,
		Content:  m.out,
		Duration: m.latency,
	}, nil
}

func TestRegisterDefaultModes_RegistersAllModes(t *testing.T) {
	reg := NewModeRegistry()
	if err := RegisterDefaultModes(reg, zap.NewNop()); err != nil {
		t.Fatalf("register default modes failed: %v", err)
	}
	got := reg.List()
	want := map[string]bool{
		ModeReasoning:      true,
		ModeCollaboration:  true,
		ModeHierarchical:   true,
		ModeCrew:           true,
		ModeDeliberation:   true,
		ModeFederation:     true,
		ModeParallel:       true,
		ModeLoop:           true,
		ModeTeamSupervisor: true,
		ModeTeamRoundRobin: true,
		ModeTeamSelector:   true,
		ModeTeamSwarm:      true,
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d modes, got %d (%v)", len(want), len(got), got)
	}
	for _, n := range got {
		if !want[n] {
			t.Fatalf("unexpected mode registered: %s", n)
		}
	}
}

func TestDefaultPrimaryModes_ExecuteViaUnifiedChain(t *testing.T) {
	reg := NewModeRegistry()
	if err := RegisterDefaultModes(reg, zap.NewNop()); err != nil {
		t.Fatalf("register default modes failed: %v", err)
	}
	singleAgent := []agent.Agent{&modeTestAgent{id: "a1", name: "agent-1", out: "ok"}}
	twoAgents := []agent.Agent{
		&modeTestAgent{id: "a1", name: "agent-1", out: "ok"},
		&modeTestAgent{id: "a2", name: "agent-2", out: "ok"},
	}
	in := &agent.Input{TraceID: "t1", Content: "hello"}

	for _, mode := range []string{ModeReasoning, ModeFederation} {
		out, err := reg.Execute(context.Background(), mode, singleAgent, in)
		if err != nil {
			t.Fatalf("%s execute failed: %v", mode, err)
		}
		if out.Content != "ok" {
			t.Fatalf("%s unexpected output: %q", mode, out.Content)
		}
		if out.Metadata == nil || out.Metadata["mode"] != mode {
			t.Fatalf("%s metadata missing mode tag: %#v", mode, out.Metadata)
		}
	}

	out, err := reg.Execute(context.Background(), ModeDeliberation, twoAgents, in)
	if err != nil {
		t.Fatalf("deliberation execute failed: %v", err)
	}
	if out.Content == "" {
		t.Fatalf("deliberation unexpected empty output")
	}
	if out.Metadata == nil || out.Metadata["mode"] != ModeDeliberation {
		t.Fatalf("deliberation metadata missing mode tag: %#v", out.Metadata)
	}
}

func TestCollaborationMode_RespectsCoordinationTypeFromInputContext(t *testing.T) {
	reg := NewModeRegistry()
	if err := RegisterDefaultModes(reg, zap.NewNop()); err != nil {
		t.Fatalf("register default modes failed: %v", err)
	}
	agents := []agent.Agent{
		&modeTestAgent{id: "a1", name: "agent-1", out: "one"},
		&modeTestAgent{id: "a2", name: "agent-2", out: "two"},
	}
	for _, ct := range []string{"debate", "consensus", "pipeline", "broadcast"} {
		in := &agent.Input{
			TraceID: "t1",
			Content: "hello",
			Context: map[string]any{"coordination_type": ct},
		}
		if _, err := reg.Execute(context.Background(), ModeCollaboration, agents, in); err != nil {
			t.Fatalf("mode collaboration (%s) execute failed: %v", ct, err)
		}
	}
}

func TestGlobalModeRegistry_AutoRegistersDefaults(t *testing.T) {
	reg := GlobalModeRegistry()
	got := reg.List()
	if len(got) < 7 {
		t.Fatalf("expected at least 7 default modes, got %d (%v)", len(got), got)
	}
	required := map[string]bool{
		ModeReasoning:     false,
		ModeCollaboration: false,
		ModeHierarchical:  false,
		ModeCrew:          false,
		ModeDeliberation:  false,
		ModeFederation:    false,
		ModeParallel:      false,
		ModeLoop:          false,
	}
	for _, name := range got {
		if _, ok := required[name]; ok {
			required[name] = true
		}
	}
	for name, ok := range required {
		if !ok {
			t.Fatalf("missing default mode in global registry: %s", name)
		}
	}
}

func TestParallelMode_ExecutesAllAgentsAndAggregates(t *testing.T) {
	reg := NewModeRegistry()
	if err := RegisterDefaultModes(reg, zap.NewNop()); err != nil {
		t.Fatalf("register default modes failed: %v", err)
	}
	agents := []agent.Agent{
		&modeTestAgent{id: "a1", name: "agent-1", out: "alpha"},
		&modeTestAgent{id: "a2", name: "agent-2", out: "beta"},
	}

	out, err := reg.Execute(context.Background(), ModeParallel, agents, &agent.Input{
		TraceID: "t1",
		Content: "hello",
		Context: map[string]any{"aggregation_strategy": string(StrategyMergeAll)},
	})
	if err != nil {
		t.Fatalf("parallel execute failed: %v", err)
	}
	if out.Metadata == nil {
		t.Fatalf("parallel mode should populate metadata")
	}
	if out.Metadata["mode"] != ModeParallel {
		t.Fatalf("parallel metadata missing mode tag: %#v", out.Metadata)
	}
	if out.Metadata["agent_count"] != len(agents) {
		t.Fatalf("parallel metadata missing agent_count: %#v", out.Metadata)
	}
	if out.Content != "alpha\n\nbeta" {
		t.Fatalf("unexpected parallel output: %q", out.Content)
	}
}

type loopTestAgent struct {
	id        string
	name      string
	callCount int
	stopAt    int
	outputFn  func(call int, input *agent.Input) *agent.Output
}

func (a *loopTestAgent) ID() string                     { return a.id }
func (a *loopTestAgent) Name() string                   { return a.name }
func (a *loopTestAgent) Type() agent.AgentType          { return agent.TypeGeneric }
func (a *loopTestAgent) State() agent.State             { return agent.StateReady }
func (a *loopTestAgent) Init(context.Context) error     { return nil }
func (a *loopTestAgent) Teardown(context.Context) error { return nil }
func (a *loopTestAgent) Plan(context.Context, *agent.Input) (*agent.PlanResult, error) {
	return nil, nil
}
func (a *loopTestAgent) Observe(context.Context, *agent.Feedback) error { return nil }
func (a *loopTestAgent) Execute(_ context.Context, input *agent.Input) (*agent.Output, error) {
	a.callCount++
	if a.outputFn != nil {
		return a.outputFn(a.callCount, input), nil
	}
	content := "working..."
	if a.callCount >= a.stopAt {
		content = "done LOOP_COMPLETE"
	}
	return &agent.Output{TraceID: input.TraceID, Content: content}, nil
}

func TestLoopMode_StopsOnKeyword(t *testing.T) {
	reg := NewModeRegistry()
	if err := RegisterDefaultModes(reg, zap.NewNop()); err != nil {
		t.Fatalf("register default modes failed: %v", err)
	}
	ag := &loopTestAgent{id: "loop-1", name: "looper", stopAt: 3}
	in := &agent.Input{TraceID: "t1", Content: "iterate", Context: map[string]any{"max_iterations": 10}}

	out, err := reg.Execute(context.Background(), ModeLoop, []agent.Agent{ag}, in)
	if err != nil {
		t.Fatalf("loop mode failed: %v", err)
	}
	if ag.callCount != 3 {
		t.Fatalf("expected 3 iterations, got %d", ag.callCount)
	}
	if out.Metadata == nil || out.Metadata["mode"] != ModeLoop {
		t.Fatalf("missing loop mode tag in metadata: %#v", out.Metadata)
	}
	if out.Metadata["iteration_count"] != 3 {
		t.Fatalf("expected iteration_count 3, got %#v", out.Metadata["iteration_count"])
	}
}

func TestLoopMode_StopsAtMaxIterations(t *testing.T) {
	reg := NewModeRegistry()
	if err := RegisterDefaultModes(reg, zap.NewNop()); err != nil {
		t.Fatalf("register default modes failed: %v", err)
	}
	ag := &loopTestAgent{id: "loop-1", name: "looper", stopAt: 999}
	in := &agent.Input{TraceID: "t1", Content: "iterate", Context: map[string]any{"max_iterations": 3}}

	out, err := reg.Execute(context.Background(), ModeLoop, []agent.Agent{ag}, in)
	if err != nil {
		t.Fatalf("loop mode failed: %v", err)
	}
	if ag.callCount != 3 {
		t.Fatalf("expected 3 iterations (max), got %d", ag.callCount)
	}
	if out.Content != "working..." {
		t.Fatalf("unexpected output: %q", out.Content)
	}
	if out.Metadata["iteration_count"] != 3 {
		t.Fatalf("expected iteration_count 3, got %#v", out.Metadata["iteration_count"])
	}
}

func TestLoopMode_PrefersStandardStopReasonOverKeyword(t *testing.T) {
	reg := NewModeRegistry()
	if err := RegisterDefaultModes(reg, zap.NewNop()); err != nil {
		t.Fatalf("register default modes failed: %v", err)
	}
	ag := &loopTestAgent{
		id:   "loop-1",
		name: "looper",
		outputFn: func(call int, input *agent.Input) *agent.Output {
			return &agent.Output{
				TraceID:               input.TraceID,
				Content:               "done without keyword",
				CurrentStage:          "evaluate",
				IterationCount:        call,
				SelectedReasoningMode: "plan_and_execute",
				StopReason:            "solved",
				CheckpointID:          "cp-top-level",
				Resumable:             true,
			}
		},
	}

	out, err := reg.Execute(context.Background(), ModeLoop, []agent.Agent{ag}, &agent.Input{
		TraceID: "t1",
		Content: "iterate",
		Context: map[string]any{"max_iterations": 10, "stop_keyword": "NEVER_MATCH"},
	})
	if err != nil {
		t.Fatalf("loop mode failed: %v", err)
	}
	if ag.callCount != 1 {
		t.Fatalf("expected stop after first iteration by stop_reason, got %d", ag.callCount)
	}
	if out.StopReason != "solved" {
		t.Fatalf("expected solved stop_reason, got %q", out.StopReason)
	}
	if out.CurrentStage != "evaluate" {
		t.Fatalf("expected evaluate stage, got %q", out.CurrentStage)
	}
	if out.IterationCount != 1 {
		t.Fatalf("expected iteration_count 1, got %d", out.IterationCount)
	}
	if out.SelectedReasoningMode != "plan_and_execute" {
		t.Fatalf("expected plan_and_execute mode, got %q", out.SelectedReasoningMode)
	}
	if out.Metadata["selected_reasoning_mode"] != "plan_and_execute" {
		t.Fatalf("expected metadata selected_reasoning_mode, got %#v", out.Metadata["selected_reasoning_mode"])
	}
	if out.Metadata["stop_reason"] != "solved" {
		t.Fatalf("expected metadata stop_reason, got %#v", out.Metadata["stop_reason"])
	}
	if out.Metadata["current_stage"] != "evaluate" {
		t.Fatalf("expected metadata current_stage, got %#v", out.Metadata["current_stage"])
	}
	if out.Metadata["checkpoint_id"] != "cp-top-level" {
		t.Fatalf("expected metadata checkpoint_id, got %#v", out.Metadata["checkpoint_id"])
	}
	if out.Metadata["resumable"] != true {
		t.Fatalf("expected metadata resumable=true, got %#v", out.Metadata["resumable"])
	}
}

func TestLoopMode_UsesMetadataFieldsWhenTopLevelFieldsMissing(t *testing.T) {
	reg := NewModeRegistry()
	if err := RegisterDefaultModes(reg, zap.NewNop()); err != nil {
		t.Fatalf("register default modes failed: %v", err)
	}
	ag := &loopTestAgent{
		id:   "loop-1",
		name: "looper",
		outputFn: func(call int, input *agent.Input) *agent.Output {
			return &agent.Output{
				TraceID: input.TraceID,
				Content: "metadata stop",
				Metadata: map[string]any{
					"stop_reason":             "blocked",
					"current_stage":           "observe",
					"iteration_count":         call,
					"selected_reasoning_mode": "rewoo",
					"checkpoint_id":           "cp-1",
					"resumable":               true,
				},
			}
		},
	}

	out, err := reg.Execute(context.Background(), ModeLoop, []agent.Agent{ag}, &agent.Input{
		TraceID: "t1",
		Content: "iterate",
		Context: map[string]any{"max_iterations": 10},
	})
	if err != nil {
		t.Fatalf("loop mode failed: %v", err)
	}
	if ag.callCount != 1 {
		t.Fatalf("expected stop after first iteration by metadata stop_reason, got %d", ag.callCount)
	}
	if out.StopReason != "blocked" {
		t.Fatalf("expected blocked stop_reason, got %q", out.StopReason)
	}
	if out.CurrentStage != "observe" {
		t.Fatalf("expected observe stage, got %q", out.CurrentStage)
	}
	if out.IterationCount != 1 {
		t.Fatalf("expected iteration_count 1, got %d", out.IterationCount)
	}
	if out.SelectedReasoningMode != "rewoo" {
		t.Fatalf("expected rewoo mode, got %q", out.SelectedReasoningMode)
	}
	if out.CheckpointID != "cp-1" {
		t.Fatalf("expected checkpoint cp-1, got %q", out.CheckpointID)
	}
	if !out.Resumable {
		t.Fatalf("expected resumable output")
	}
	if out.Metadata["checkpoint_id"] != "cp-1" {
		t.Fatalf("expected metadata checkpoint_id cp-1, got %#v", out.Metadata["checkpoint_id"])
	}
	if out.Metadata["resumable"] != true {
		t.Fatalf("expected metadata resumable=true, got %#v", out.Metadata["resumable"])
	}
}

func TestLoopMode_FallsBackToStopKeywordOnlyWhenStopReasonMissing(t *testing.T) {
	reg := NewModeRegistry()
	if err := RegisterDefaultModes(reg, zap.NewNop()); err != nil {
		t.Fatalf("register default modes failed: %v", err)
	}
	ag := &loopTestAgent{
		id:   "loop-1",
		name: "looper",
		outputFn: func(call int, input *agent.Input) *agent.Output {
			content := "still working"
			if call == 2 {
				content = "done CUSTOM_STOP"
			}
			return &agent.Output{
				TraceID:               input.TraceID,
				Content:               content,
				CurrentStage:          "act",
				IterationCount:        call,
				SelectedReasoningMode: "react",
			}
		},
	}

	out, err := reg.Execute(context.Background(), ModeLoop, []agent.Agent{ag}, &agent.Input{
		TraceID: "t1",
		Content: "iterate",
		Context: map[string]any{"max_iterations": 10, "stop_keyword": "CUSTOM_STOP"},
	})
	if err != nil {
		t.Fatalf("loop mode failed: %v", err)
	}
	if ag.callCount != 2 {
		t.Fatalf("expected keyword fallback to stop on second iteration, got %d", ag.callCount)
	}
	if out.StopReason != "" {
		t.Fatalf("expected empty stop_reason when only keyword matched, got %q", out.StopReason)
	}
	if out.Metadata["iteration_count"] != 2 {
		t.Fatalf("expected metadata iteration_count 2, got %#v", out.Metadata["iteration_count"])
	}
}

func TestReasoningMode_RemainsSingleAgentDefaultInsteadOfLoop(t *testing.T) {
	reg := NewModeRegistry()
	if err := RegisterDefaultModes(reg, zap.NewNop()); err != nil {
		t.Fatalf("register default modes failed: %v", err)
	}
	ag := &loopTestAgent{
		id:   "single-1",
		name: "single",
		outputFn: func(call int, input *agent.Input) *agent.Output {
			return &agent.Output{
				TraceID:               input.TraceID,
				Content:               "single agent",
				CurrentStage:          "evaluate",
				IterationCount:        call,
				SelectedReasoningMode: "react",
				StopReason:            "solved",
			}
		},
	}

	out, err := reg.Execute(context.Background(), ModeReasoning, []agent.Agent{ag}, &agent.Input{
		TraceID: "t-single",
		Content: "hello",
	})
	if err != nil {
		t.Fatalf("reasoning mode failed: %v", err)
	}
	if ag.callCount != 1 {
		t.Fatalf("expected single execution, got %d", ag.callCount)
	}
	if out.Metadata["mode"] != ModeReasoning {
		t.Fatalf("expected reasoning mode tag, got %#v", out.Metadata["mode"])
	}
	if out.Metadata["mode"] == ModeLoop {
		t.Fatalf("single agent default must not be loop mode")
	}
	if out.Metadata["agent_count"] != 1 {
		t.Fatalf("expected agent_count 1, got %#v", out.Metadata["agent_count"])
	}
}
