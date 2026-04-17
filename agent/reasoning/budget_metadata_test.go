package reasoning

import (
	"context"
	"testing"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

func TestReflexionExecutor_ExportsInternalBudgetMetadata(t *testing.T) {
	call := 0
	provider := &testProvider{
		completionFn: func(context.Context, *llm.ChatRequest) (*llm.ChatResponse, error) {
			call++
			switch call {
			case 1:
				return &llm.ChatResponse{
					Choices: []llm.ChatChoice{{Message: types.Message{Content: "candidate answer"}}},
				}, nil
			case 2:
				return &llm.ChatResponse{
					Choices: []llm.ChatChoice{{Message: types.Message{Content: `{"score": 0.9}`}}},
				}, nil
			default:
				return &llm.ChatResponse{}, nil
			}
		},
	}

	cfg := DefaultReflexionConfig()
	cfg.MaxTrials = 2
	cfg.SuccessThreshold = 0.85
	executor := NewReflexionExecutor(testGateway(provider), &testToolExecutor{}, nil, cfg, zap.NewNop())

	result, err := executor.Execute(context.Background(), "solve")
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}
	if got := result.Metadata["reflexion_trial_budget"]; got != 2 {
		t.Fatalf("expected reflexion trial budget 2, got %#v", got)
	}
	if got := result.Metadata["reflexion_success_threshold"]; got != 0.85 {
		t.Fatalf("expected reflexion success threshold 0.85, got %#v", got)
	}
	if got := result.Metadata["reflexion_budget_scope"]; got != "strategy_internal" {
		t.Fatalf("expected strategy_internal scope, got %#v", got)
	}
	if got := result.Metadata["internal_stop_cause"]; got != "completed" {
		t.Fatalf("expected completed internal stop cause, got %#v", got)
	}
}

func TestPlanAndExecute_ExportsInternalBudgetMetadata(t *testing.T) {
	call := 0
	provider := &testProvider{
		completionFn: func(context.Context, *llm.ChatRequest) (*llm.ChatResponse, error) {
			call++
			switch call {
			case 1:
				return &llm.ChatResponse{
					Choices: []llm.ChatChoice{{Message: types.Message{Content: `{"goal":"solve","steps":[{"id":"step_1","description":"do work"}]}`}}},
				}, nil
			case 2:
				return &llm.ChatResponse{
					Choices: []llm.ChatChoice{{Message: types.Message{Content: "step result"}}},
				}, nil
			case 3:
				return &llm.ChatResponse{
					Choices: []llm.ChatChoice{{Message: types.Message{Content: "final answer"}}},
				}, nil
			default:
				return &llm.ChatResponse{}, nil
			}
		},
	}

	cfg := DefaultPlanExecuteConfig()
	cfg.MaxReplanAttempts = 4
	cfg.MaxPlanSteps = 7
	executor := NewPlanAndExecute(testGateway(provider), &testToolExecutor{}, nil, cfg, zap.NewNop())

	result, err := executor.Execute(context.Background(), "solve")
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}
	if got := result.Metadata["plan_execute_replan_budget"]; got != 4 {
		t.Fatalf("expected plan execute replan budget 4, got %#v", got)
	}
	if got := result.Metadata["plan_execute_max_plan_steps"]; got != 7 {
		t.Fatalf("expected plan execute max plan steps 7, got %#v", got)
	}
	if got := result.Metadata["plan_execute_budget_scope"]; got != "strategy_internal" {
		t.Fatalf("expected strategy_internal scope, got %#v", got)
	}
	if got := result.Metadata["internal_stop_cause"]; got != "completed" {
		t.Fatalf("expected completed internal stop cause, got %#v", got)
	}
}

func TestDynamicPlanner_ExportsInternalBudgetMetadata(t *testing.T) {
	call := 0
	provider := &testProvider{
		completionFn: func(context.Context, *llm.ChatRequest) (*llm.ChatResponse, error) {
			call++
			switch call {
			case 1:
				return &llm.ChatResponse{
					Choices: []llm.ChatChoice{{Message: types.Message{Content: `{"steps":[{"action":"think","description":"analyze","confidence":0.8}]}`}}},
				}, nil
			case 2:
				return &llm.ChatResponse{
					Choices: []llm.ChatChoice{{Message: types.Message{Content: "final answer"}}},
				}, nil
			default:
				return &llm.ChatResponse{}, nil
			}
		},
	}

	cfg := DefaultDynamicPlannerConfig()
	cfg.MaxPlanDepth = 6
	cfg.ConfidenceThreshold = 0.55
	cfg.MaxBacktracks = 2
	executor := NewDynamicPlanner(testGateway(provider), &testToolExecutor{}, nil, cfg, zap.NewNop())

	result, err := executor.Execute(context.Background(), "solve")
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}
	if got := result.Metadata["dynamic_planner_max_plan_depth"]; got != 6 {
		t.Fatalf("expected max plan depth 6, got %#v", got)
	}
	if got := result.Metadata["dynamic_planner_confidence_threshold"]; got != 0.55 {
		t.Fatalf("expected confidence threshold 0.55, got %#v", got)
	}
	if got := result.Metadata["dynamic_planner_backtrack_budget"]; got != 2 {
		t.Fatalf("expected backtrack budget 2, got %#v", got)
	}
	if got := result.Metadata["dynamic_planner_budget_scope"]; got != "strategy_internal" {
		t.Fatalf("expected strategy_internal scope, got %#v", got)
	}
	if got := result.Metadata["internal_stop_cause"]; got != "completed" {
		t.Fatalf("expected completed internal stop cause, got %#v", got)
	}
}

func TestReflexionExecutor_UsesTrialBudgetAsInternalCauseOnly(t *testing.T) {
	call := 0
	provider := &testProvider{
		completionFn: func(context.Context, *llm.ChatRequest) (*llm.ChatResponse, error) {
			call++
			switch call % 3 {
			case 1:
				return &llm.ChatResponse{Choices: []llm.ChatChoice{{Message: types.Message{Content: "candidate answer"}}}}, nil
			case 2:
				return &llm.ChatResponse{Choices: []llm.ChatChoice{{Message: types.Message{Content: `{"score": 0.4}`}}}}, nil
			default:
				return &llm.ChatResponse{Choices: []llm.ChatChoice{{Message: types.Message{Content: `{"analysis":"retry","mistakes":[],"next_strategy":"retry"}`}}}}, nil
			}
		},
	}

	cfg := DefaultReflexionConfig()
	cfg.MaxTrials = 2
	cfg.SuccessThreshold = 0.85
	executor := NewReflexionExecutor(testGateway(provider), &testToolExecutor{}, nil, cfg, zap.NewNop())

	result, err := executor.Execute(context.Background(), "solve")
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}
	if got := result.Metadata["internal_stop_cause"]; got != "reflexion_trial_budget_exhausted" {
		t.Fatalf("expected trial budget exhausted internal cause, got %#v", got)
	}
}
