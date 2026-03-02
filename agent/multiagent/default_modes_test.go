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

func TestRegisterDefaultModes_RegistersAllSixModes(t *testing.T) {
	reg := NewModeRegistry()
	if err := RegisterDefaultModes(reg, zap.NewNop()); err != nil {
		t.Fatalf("register default modes failed: %v", err)
	}
	got := reg.List()
	want := map[string]bool{
		ModeReasoning:     true,
		ModeCollaboration: true,
		ModeHierarchical:  true,
		ModeCrew:          true,
		ModeDeliberation:  true,
		ModeFederation:    true,
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
	agents := []agent.Agent{
		&modeTestAgent{id: "a1", name: "agent-1", out: "ok"},
	}
	in := &agent.Input{TraceID: "t1", Content: "hello"}

	for _, mode := range []string{ModeReasoning, ModeDeliberation, ModeFederation} {
		out, err := reg.Execute(context.Background(), mode, agents, in)
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
	if len(got) < 6 {
		t.Fatalf("expected at least 6 default modes, got %d (%v)", len(got), got)
	}
	required := map[string]bool{
		ModeReasoning:     false,
		ModeCollaboration: false,
		ModeHierarchical:  false,
		ModeCrew:          false,
		ModeDeliberation:  false,
		ModeFederation:    false,
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
