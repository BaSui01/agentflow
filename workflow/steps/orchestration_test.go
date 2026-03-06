package steps

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/agent/multiagent"
	"github.com/BaSui01/agentflow/workflow/core"
	"go.uber.org/zap"
)

type mockOrchestrationAgent struct {
	id   string
	name string
	out  string
}

func (m *mockOrchestrationAgent) ID() string                                   { return m.id }
func (m *mockOrchestrationAgent) Name() string                                  { return m.name }
func (m *mockOrchestrationAgent) Type() agent.AgentType                         { return agent.TypeGeneric }
func (m *mockOrchestrationAgent) State() agent.State                            { return agent.StateReady }
func (m *mockOrchestrationAgent) Init(context.Context) error                    { return nil }
func (m *mockOrchestrationAgent) Teardown(context.Context) error                { return nil }
func (m *mockOrchestrationAgent) Plan(context.Context, *agent.Input) (*agent.PlanResult, error) {
	return nil, nil
}
func (m *mockOrchestrationAgent) Observe(context.Context, *agent.Feedback) error { return nil }
func (m *mockOrchestrationAgent) Execute(_ context.Context, input *agent.Input) (*agent.Output, error) {
	return &agent.Output{TraceID: input.TraceID, Content: m.out, Duration: 0}, nil
}

type mockAgentResolver struct {
	agents map[string]agent.Agent
	err    error
}

func (r *mockAgentResolver) ResolveAgent(ctx context.Context, agentID string) (agent.Agent, error) {
	if r.err != nil {
		return nil, r.err
	}
	if a, ok := r.agents[agentID]; ok {
		return a, nil
	}
	return nil, errors.New("agent not found")
}

func TestOrchestrationStep_Validate(t *testing.T) {
	resolver := &mockAgentResolver{agents: map[string]agent.Agent{"a1": &mockOrchestrationAgent{id: "a1"}}}

	tests := []struct {
		name    string
		step    *OrchestrationStep
		wantErr bool
	}{
		{
			name: "nil resolver",
			step: NewOrchestrationStep("s1", nil, nil, nil),
			wantErr: true,
		},
		{
			name: "empty mode",
			step: func() *OrchestrationStep {
				s := NewOrchestrationStep("s1", resolver, nil, nil)
				s.AgentIDs = []string{"a1"}
				return s
			}(),
			wantErr: true,
		},
		{
			name: "empty agent_ids",
			step: func() *OrchestrationStep {
				s := NewOrchestrationStep("s1", resolver, nil, nil)
				s.Mode = multiagent.ModeReasoning
				return s
			}(),
			wantErr: true,
		},
		{
			name: "valid",
			step: func() *OrchestrationStep {
				s := NewOrchestrationStep("s1", resolver, nil, nil)
				s.Mode = multiagent.ModeReasoning
				s.AgentIDs = []string{"a1"}
				return s
			}(),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.step.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestOrchestrationStep_Execute_ReasoningMode(t *testing.T) {
	reg := multiagent.NewModeRegistry()
	if err := multiagent.RegisterDefaultModes(reg, zap.NewNop()); err != nil {
		t.Fatalf("register modes: %v", err)
	}
	resolver := &mockAgentResolver{
		agents: map[string]agent.Agent{
			"a1": &mockOrchestrationAgent{id: "a1", name: "agent-1", out: "reasoning-result"},
		},
	}
	s := NewOrchestrationStep("orch-1", resolver, reg, zap.NewNop())
	s.Mode = multiagent.ModeReasoning
	s.AgentIDs = []string{"a1"}

	out, err := s.Execute(context.Background(), core.StepInput{
		Data: map[string]any{"content": "hello"},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if r, ok := out.Data["result"].(string); !ok || r != "reasoning-result" {
		t.Errorf("unexpected result: %#v", out.Data)
	}
}

func TestOrchestrationStep_Execute_CollaborationMode(t *testing.T) {
	reg := multiagent.NewModeRegistry()
	if err := multiagent.RegisterDefaultModes(reg, zap.NewNop()); err != nil {
		t.Fatalf("register modes: %v", err)
	}
	resolver := &mockAgentResolver{
		agents: map[string]agent.Agent{
			"a1": &mockOrchestrationAgent{id: "a1", name: "agent-1", out: "one"},
			"a2": &mockOrchestrationAgent{id: "a2", name: "agent-2", out: "two"},
		},
	}
	s := NewOrchestrationStep("orch-1", resolver, reg, zap.NewNop())
	s.Mode = multiagent.ModeCollaboration
	s.AgentIDs = []string{"a1", "a2"}

	out, err := s.Execute(context.Background(), core.StepInput{
		Data: map[string]any{"content": "collaborate"},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if _, ok := out.Data["result"]; !ok {
		t.Errorf("missing result: %#v", out.Data)
	}
}

func TestOrchestrationStep_Execute_ResolverError(t *testing.T) {
	resolver := &mockAgentResolver{err: errors.New("resolver failed")}
	s := NewOrchestrationStep("orch-1", resolver, nil, zap.NewNop())
	s.Mode = multiagent.ModeReasoning
	s.AgentIDs = []string{"a1"}

	_, err := s.Execute(context.Background(), core.StepInput{})
	if err == nil {
		t.Fatal("expected error")
	}
	var stepErr *core.StepError
	if !errors.As(err, &stepErr) {
		t.Errorf("expected StepError, got %T", err)
	}
}

func TestOrchestrationStep_Execute_AgentNotFound(t *testing.T) {
	reg := multiagent.NewModeRegistry()
	_ = multiagent.RegisterDefaultModes(reg, zap.NewNop())
	resolver := &mockAgentResolver{agents: map[string]agent.Agent{}}
	s := NewOrchestrationStep("orch-1", resolver, reg, zap.NewNop())
	s.Mode = multiagent.ModeReasoning
	s.AgentIDs = []string{"missing"}

	_, err := s.Execute(context.Background(), core.StepInput{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestOrchestrationStep_IDAndType(t *testing.T) {
	s := NewOrchestrationStep("step-id", &mockAgentResolver{}, nil, nil)
	if s.ID() != "step-id" {
		t.Errorf("ID() = %s, want step-id", s.ID())
	}
	if s.Type() != core.StepTypeOrchestration {
		t.Errorf("Type() = %s, want orchestration", s.Type())
	}
}

func TestOrchestrationStep_MaxRoundsInContext(t *testing.T) {
	reg := multiagent.NewModeRegistry()
	_ = multiagent.RegisterDefaultModes(reg, zap.NewNop())
	resolver := &mockAgentResolver{
		agents: map[string]agent.Agent{
			"a1": &mockOrchestrationAgent{id: "a1", out: "ok"},
			"a2": &mockOrchestrationAgent{id: "a2", out: "ok"},
		},
	}
	s := NewOrchestrationStep("orch-1", resolver, reg, zap.NewNop())
	s.Mode = multiagent.ModeDeliberation
	s.AgentIDs = []string{"a1", "a2"}
	s.MaxRounds = 2

	_, err := s.Execute(context.Background(), core.StepInput{Data: map[string]any{"content": "deliberate"}})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
}

func TestOrchestrationStep_Timeout(t *testing.T) {
	reg := multiagent.NewModeRegistry()
	_ = multiagent.RegisterDefaultModes(reg, zap.NewNop())
	resolver := &mockAgentResolver{
		agents: map[string]agent.Agent{"a1": &mockOrchestrationAgent{id: "a1", out: "ok"}},
	}
	s := NewOrchestrationStep("orch-1", resolver, reg, zap.NewNop())
	s.Mode = multiagent.ModeReasoning
	s.AgentIDs = []string{"a1"}
	s.Timeout = 5 * time.Second

	out, err := s.Execute(context.Background(), core.StepInput{Data: map[string]any{"content": "hi"}})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.Data["result"] != "ok" {
		t.Errorf("unexpected result: %#v", out.Data)
	}
}
