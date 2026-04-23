package dsl_test

import (
	"context"
	"testing"

	"github.com/BaSui01/agentflow/agent/collaboration/multiagent"
	agent "github.com/BaSui01/agentflow/agent/execution/runtime"
	workflow "github.com/BaSui01/agentflow/workflow/core"
	"github.com/BaSui01/agentflow/workflow/dsl"
	"github.com/BaSui01/agentflow/workflow/engine"
	"go.uber.org/zap"
)

type mockOrchestrationAgent struct {
	id  string
	out string
}

func (m *mockOrchestrationAgent) ID() string                     { return m.id }
func (m *mockOrchestrationAgent) Name() string                   { return m.id }
func (m *mockOrchestrationAgent) Type() agent.AgentType          { return agent.TypeGeneric }
func (m *mockOrchestrationAgent) State() agent.State             { return agent.StateReady }
func (m *mockOrchestrationAgent) Init(context.Context) error     { return nil }
func (m *mockOrchestrationAgent) Teardown(context.Context) error { return nil }
func (m *mockOrchestrationAgent) Plan(context.Context, *agent.Input) (*agent.PlanResult, error) {
	return nil, nil
}
func (m *mockOrchestrationAgent) Observe(context.Context, *agent.Feedback) error { return nil }
func (m *mockOrchestrationAgent) Execute(_ context.Context, in *agent.Input) (*agent.Output, error) {
	return &agent.Output{TraceID: in.TraceID, Content: m.out, Duration: 0}, nil
}

type mockAgentResolver struct {
	agents map[string]agent.Agent
}

func (r *mockAgentResolver) ResolveAgent(ctx context.Context, agentID string) (agent.Agent, error) {
	if a, ok := r.agents[agentID]; ok {
		return a, nil
	}
	return nil, nil
}

func TestWorkflowDSL_OrchestrationStep_MultiAgent_SharedState(t *testing.T) {
	reg := multiagent.NewModeRegistry()
	if err := multiagent.RegisterDefaultModes(reg, zap.NewNop()); err != nil {
		t.Fatalf("register modes: %v", err)
	}
	resolver := &mockAgentResolver{
		agents: map[string]agent.Agent{
			"a1": &mockOrchestrationAgent{id: "a1", out: "agent-one-output"},
			"a2": &mockOrchestrationAgent{id: "a2", out: "agent-two-output"},
		},
	}
	deps := engine.StepDependencies{
		AgentResolver: resolver,
	}

	dslYAML := `
version: "1"
name: orchestration-test
description: test workflow with orchestration step
workflow:
  entry: orch
  nodes:
    - id: orch
      type: action
      step_def:
        type: orchestration
        orchestration:
          mode: deliberation
          agent_ids: [a1, a2]
          max_rounds: 2
`

	parser := dsl.NewParser().WithStepDependencies(deps)
	wf, err := parser.Parse([]byte(dslYAML))
	if err != nil {
		t.Fatalf("parse DSL: %v", err)
	}
	if wf == nil {
		t.Fatal("workflow is nil")
	}

	sharedState := multiagent.NewInMemorySharedState()
	exec := workflow.NewDAGExecutor(nil, zap.NewNop())
	facade := workflow.NewFacade(exec)

	input := map[string]any{
		"content":      "deliberate on this",
		"shared_state": sharedState,
	}
	ctx := context.Background()
	result, err := facade.ExecuteDAG(ctx, wf, input)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}

	snapshot := sharedState.Snapshot(ctx)
	if _, ok := snapshot["result:deliberation"]; !ok {
		t.Errorf("expected result:deliberation in SharedState, got keys: %v", keysOf(snapshot))
	}
}

func keysOf(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
