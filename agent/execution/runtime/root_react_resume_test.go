package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

func TestBaseAgentExecute_ResumesFromLatestCheckpointOnMainChain(t *testing.T) {
	provider := &testProvider{
		name: "mock",
		completionFn: func(_ context.Context, _ *llm.ChatRequest) (*llm.ChatResponse, error) {
			return &llm.ChatResponse{
				ID:       "resp-1",
				Provider: "mock",
				Model:    "gpt-4",
				Choices: []llm.ChatChoice{{
					Message: types.Message{
						Role:    llm.RoleAssistant,
						Content: "resumed answer",
					},
					FinishReason: "stop",
				}},
				Usage: llm.ChatUsage{TotalTokens: 10},
			}, nil
		},
	}
	agent := BuildBaseAgent(testAgentConfig("agent-1", "Agent", "gpt-4"), testGatewayFromProvider(provider), nil, nil, nil, zap.NewNop(), nil)
	requireReadyAgent(t, agent)

	store := newInMemoryCheckpointStore()
	manager := NewCheckpointManager(store, zap.NewNop())
	agent.SetCheckpointManager(manager)
	if err := store.Save(context.Background(), &Checkpoint{
		ID:                  "cp-latest",
		ThreadID:            "thread-1",
		AgentID:             "agent-1",
		LoopStateID:         "loop-run-1",
		RunID:               "run-1",
		Goal:                "resume goal",
		CurrentPlanID:       "loop-run-1-plan-2",
		PlanVersion:         2,
		CurrentStepID:       "step-3",
		ObservationsSummary: "plan:ready | act:partial",
		LastOutputSummary:   "partial answer",
		LastError:           "retryable tool error",
		State:               StateRunning,
		ExecutionContext: &ExecutionContext{
			CurrentNode:         string(LoopStageObserve),
			LoopStateID:         "loop-run-1",
			RunID:               "run-1",
			AgentID:             "agent-1",
			Goal:                "resume goal",
			CurrentPlanID:       "loop-run-1-plan-2",
			PlanVersion:         2,
			CurrentStepID:       "step-3",
			ObservationsSummary: "plan:ready | act:partial",
			LastOutputSummary:   "partial answer",
			LastError:           "retryable tool error",
			Variables: map[string]any{
				"goal":                    "resume goal",
				"run_id":                  "run-1",
				"loop_state_id":           "loop-run-1",
				"iteration_count":         2,
				"max_iterations":          3,
				"current_plan_id":         "loop-run-1-plan-2",
				"plan_version":            2,
				"current_step_id":         "step-3",
				"observations_summary":    "plan:ready | act:partial",
				"last_output_summary":     "partial answer",
				"last_error":              "retryable tool error",
				"selected_reasoning_mode": ReasoningModePlanAndExecute,
				"plan":                    []string{"step-1", "step-2", "step-3"},
			},
		},
		Metadata: map[string]any{
			"checkpoint_id":        "cp-latest",
			"resumable":            true,
			"loop_state_id":        "loop-run-1",
			"current_plan_id":      "loop-run-1-plan-2",
			"plan_version":         2,
			"observations_summary": "plan:ready | act:partial",
			"last_output_summary":  "partial answer",
			"last_error":           "retryable tool error",
		},
	}); err != nil {
		t.Fatalf("save checkpoint: %v", err)
	}

	output, err := agent.Execute(context.Background(), &Input{
		ChannelID: "thread-1",
		Context: map[string]any{
			"resume_latest": true,
		},
	})
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}
	if output.IterationCount != 3 {
		t.Fatalf("expected resumed iteration to continue from 2 to 3, got %d", output.IterationCount)
	}
	if output.CheckpointID == "" {
		t.Fatal("expected checkpoint id on output")
	}
	saved, err := store.Load(context.Background(), output.CheckpointID)
	if err != nil {
		t.Fatalf("load saved checkpoint: %v", err)
	}
	if saved.LoopStateID != "loop-run-1" {
		t.Fatalf("expected loop_state_id preserved, got %q", saved.LoopStateID)
	}
	if saved.CurrentPlanID != "loop-run-1-plan-2" {
		t.Fatalf("expected current_plan_id preserved, got %q", saved.CurrentPlanID)
	}
	if saved.PlanVersion != 2 {
		t.Fatalf("expected plan_version preserved, got %d", saved.PlanVersion)
	}
	if saved.ObservationsSummary == "" {
		t.Fatal("expected observations_summary on saved checkpoint")
	}
	if saved.LastOutputSummary != "resumed answer" {
		t.Fatalf("expected last_output_summary updated to resumed answer, got %q", saved.LastOutputSummary)
	}
}

func TestBaseAgentExecute_ResumesLoopFieldsFromExecutionContextMapping(t *testing.T) {
	provider := &testProvider{
		name: "mock",
		completionFn: func(_ context.Context, _ *llm.ChatRequest) (*llm.ChatResponse, error) {
			return &llm.ChatResponse{
				ID:       "resp-2",
				Provider: "mock",
				Model:    "gpt-4",
				Choices: []llm.ChatChoice{{
					Message:      types.Message{Role: llm.RoleAssistant, Content: "recovered answer"},
					FinishReason: "stop",
				}},
				Usage: llm.ChatUsage{TotalTokens: 12},
			}, nil
		},
	}
	agent := BuildBaseAgent(testAgentConfig("agent-2", "Agent", "gpt-4"), testGatewayFromProvider(provider), nil, nil, nil, zap.NewNop(), nil)
	requireReadyAgent(t, agent)

	store := newInMemoryCheckpointStore()
	manager := NewCheckpointManager(store, zap.NewNop())
	agent.SetCheckpointManager(manager)
	if err := store.Save(context.Background(), &Checkpoint{
		ID:        "cp-ctx-only",
		ThreadID:  "thread-2",
		AgentID:   "agent-2",
		State:     StateRunning,
		CreatedAt: time.Now(),
		ExecutionContext: &ExecutionContext{
			CurrentNode: string(LoopStageAct),
			Variables: map[string]any{
				"loop_state_id":        "loop-ctx-1",
				"run_id":               "run-ctx-1",
				"goal":                 "recovered goal",
				"current_plan_id":      "loop-ctx-1-plan-7",
				"plan_version":         7,
				"current_step":         "step-7",
				"observations_summary": "plan:ready | act:partial",
				"last_output_summary":  "partial before resume",
				"last_error":           "tool timeout",
				"iteration_count":      1,
				"max_iterations":       3,
			},
		},
	}); err != nil {
		t.Fatalf("save checkpoint: %v", err)
	}

	output, err := agent.Execute(context.Background(), &Input{
		ChannelID: "thread-2",
		Context: map[string]any{
			"resume_latest": true,
		},
	})
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}
	saved, err := store.Load(context.Background(), output.CheckpointID)
	if err != nil {
		t.Fatalf("load saved checkpoint: %v", err)
	}
	if saved.LoopStateID != "loop-ctx-1" {
		t.Fatalf("expected loop_state_id restored from execution context, got %q", saved.LoopStateID)
	}
	if saved.RunID != "run-ctx-1" {
		t.Fatalf("expected run_id restored from execution context, got %q", saved.RunID)
	}
	if saved.Goal != "recovered goal" {
		t.Fatalf("expected goal restored from execution context, got %q", saved.Goal)
	}
	if saved.CurrentPlanID != "loop-ctx-1-plan-7" {
		t.Fatalf("expected current_plan_id restored from execution context, got %q", saved.CurrentPlanID)
	}
	if saved.PlanVersion != 7 {
		t.Fatalf("expected plan_version restored from execution context, got %d", saved.PlanVersion)
	}
	if saved.CurrentStepID != "step-7" {
		t.Fatalf("expected current_step_id restored from execution context, got %q", saved.CurrentStepID)
	}
	if saved.ObservationsSummary == "" {
		t.Fatal("expected observations_summary persisted after resume")
	}
	if saved.Metadata["current_step"] != "step-7" {
		t.Fatalf("expected current_step alias in metadata, got %#v", saved.Metadata["current_step"])
	}
}

func TestPrepareResumeInput_RejectsCheckpointForDifferentAgent(t *testing.T) {
	agent := BuildBaseAgent(testAgentConfig("agent-expected", "Agent", "gpt-4"), testGatewayFromProvider(nil), nil, nil, nil, zap.NewNop(), nil)
	store := newInMemoryCheckpointStore()
	manager := NewCheckpointManager(store, zap.NewNop())
	agent.SetCheckpointManager(manager)

	if err := store.Save(context.Background(), &Checkpoint{
		ID:       "cp-wrong-agent",
		ThreadID: "thread-mismatch",
		AgentID:  "agent-other",
		State:    StateRunning,
	}); err != nil {
		t.Fatalf("save checkpoint: %v", err)
	}

	_, err := agent.prepareResumeInput(context.Background(), &Input{
		ChannelID: "thread-mismatch",
		Context: map[string]any{
			"resume_latest": true,
		},
	})
	if err == nil {
		t.Fatal("expected mismatch error")
	}

	typedErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got %T", err)
	}
	if typedErr.Base == nil {
		t.Fatal("expected base typed error")
	}
	if typedErr.Base.Code != types.ErrInputValidation {
		t.Fatalf("expected error code %q, got %q", types.ErrInputValidation, typedErr.Base.Code)
	}
}

func requireReadyAgent(t *testing.T, agent *BaseAgent) {
	t.Helper()
	if err := agent.Init(context.Background()); err != nil {
		t.Fatalf("init agent: %v", err)
	}
}


