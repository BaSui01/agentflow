package steps

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/workflow/core"
)

type testHumanHandler struct {
	result any
	err    error
}

func (h *testHumanHandler) RequestInput(ctx context.Context, prompt string, inputType string, options []string) (any, error) {
	return h.result, h.err
}

type testAgent struct {
	result any
	err    error
}

func (a *testAgent) Execute(ctx context.Context, input any) (any, error) {
	return a.result, a.err
}

func TestHumanStepExecute(t *testing.T) {
	step := NewHumanStep("human-1", &testHumanHandler{result: "approved"})
	step.Prompt = "approve?"
	step.InputType = "text"
	step.Timeout = 50 * time.Millisecond

	out, err := step.Execute(context.Background(), core.StepInput{})
	if err != nil {
		t.Fatalf("execute human step failed: %v", err)
	}
	if got := out.Data["input"]; got != "approved" {
		t.Fatalf("unexpected human output: %v", got)
	}
}

func TestCodeStepExecute(t *testing.T) {
	step := NewCodeStep("code-1", func(ctx context.Context, input core.StepInput) (map[string]any, error) {
		return map[string]any{"ok": true, "value": 42}, nil
	})

	out, err := step.Execute(context.Background(), core.StepInput{})
	if err != nil {
		t.Fatalf("execute code step failed: %v", err)
	}
	if got, ok := out.Data["ok"].(bool); !ok || !got {
		t.Fatalf("unexpected code output: %v", out.Data)
	}
}

func TestAgentStepExecute(t *testing.T) {
	step := NewAgentStep("agent-1", &testAgent{result: "done"})

	out, err := step.Execute(context.Background(), core.StepInput{Data: map[string]any{"task": "x"}})
	if err != nil {
		t.Fatalf("execute agent step failed: %v", err)
	}
	if got := out.Data["result"]; got != "done" {
		t.Fatalf("unexpected agent output: %v", got)
	}
}

func TestValidateNotConfigured(t *testing.T) {
	human := NewHumanStep("h", nil)
	if err := human.Validate(); err == nil {
		t.Fatal("expected human validate error")
	}

	code := NewCodeStep("c", nil)
	if err := code.Validate(); err == nil {
		t.Fatal("expected code validate error")
	}

	agent := NewAgentStep("a", nil)
	if err := agent.Validate(); err == nil {
		t.Fatal("expected agent validate error")
	}
}

func TestAgentStepExecutionError(t *testing.T) {
	step := NewAgentStep("agent-err", &testAgent{err: errors.New("boom")})
	if _, err := step.Execute(context.Background(), core.StepInput{}); err == nil {
		t.Fatal("expected execute error")
	}
}
