package multiagent

import (
	"context"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/agent"
	"go.uber.org/zap"
)

type modeTestAgent struct {
	id     string
	name   string
	out    string
	latency time.Duration
}

func (m *modeTestAgent) ID() string                                   { return m.id }
func (m *modeTestAgent) Name() string                                 { return m.name }
func (m *modeTestAgent) Type() agent.AgentType                        { return agent.TypeGeneric }
func (m *modeTestAgent) State() agent.State                           { return agent.StateReady }
func (m *modeTestAgent) Init(context.Context) error                   { return nil }
func (m *modeTestAgent) Teardown(context.Context) error               { return nil }
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

type loopTestAgent struct {
	id        string
	name      string
	callCount int
	stopAt    int
}

func (a *loopTestAgent) ID() string                                   { return a.id }
func (a *loopTestAgent) Name() string                                 { return a.name }
func (a *loopTestAgent) Type() agent.AgentType                        { return agent.TypeGeneric }
func (a *loopTestAgent) State() agent.State                           { return agent.StateReady }
func (a *loopTestAgent) Init(context.Context) error                   { return nil }
func (a *loopTestAgent) Teardown(context.Context) error               { return nil }
func (a *loopTestAgent) Plan(context.Context, *agent.Input) (*agent.PlanResult, error) {
	return nil, nil
}
func (a *loopTestAgent) Observe(context.Context, *agent.Feedback) error { return nil }
func (a *loopTestAgent) Execute(_ context.Context, input *agent.Input) (*agent.Output, error) {
	a.callCount++
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
}
