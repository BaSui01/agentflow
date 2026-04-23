package steps

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/workflow/core"
)

type testHumanHandler struct {
	result string
	err    error
}

func (h *testHumanHandler) RequestInput(ctx context.Context, prompt string, inputType string, options []string) (*core.HumanInputResult, error) {
	if h.err != nil {
		return nil, h.err
	}
	return &core.HumanInputResult{Value: h.result}, nil
}

type testAgent struct {
	result string
	err    error
	input  map[string]any
}

func (a *testAgent) Execute(ctx context.Context, input map[string]any) (*core.AgentExecutionOutput, error) {
	a.input = input
	if a.err != nil {
		return nil, a.err
	}
	return &core.AgentExecutionOutput{Content: a.result}, nil
}

type testLLMGateway struct {
	invokeResponse *core.LLMResponse
	invokeErr      error
	streamChunks   []core.LLMStreamChunk
	streamErr      error
}

func (g *testLLMGateway) Invoke(ctx context.Context, req *core.LLMRequest) (*core.LLMResponse, error) {
	if g.invokeErr != nil {
		return nil, g.invokeErr
	}
	if g.invokeResponse != nil {
		return g.invokeResponse, nil
	}
	return &core.LLMResponse{Content: "invoke:" + req.Prompt, Model: req.Model}, nil
}

func (g *testLLMGateway) Stream(ctx context.Context, req *core.LLMRequest) (<-chan core.LLMStreamChunk, error) {
	if g.streamErr != nil {
		return nil, g.streamErr
	}
	ch := make(chan core.LLMStreamChunk, len(g.streamChunks))
	for _, chunk := range g.streamChunks {
		ch <- chunk
	}
	close(ch)
	return ch, nil
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

func TestLLMStepExecute_StreamsTokensWhenEmitterPresent(t *testing.T) {
	reasoning := "think"
	step := NewLLMStep("llm-1", &testLLMGateway{
		streamChunks: []core.LLMStreamChunk{
			{Delta: "hel", Model: "gpt-test"},
			{Delta: "lo", ReasoningContent: &reasoning, Model: "gpt-test"},
			{
				Model: "gpt-test",
				Usage: &core.LLMUsage{PromptTokens: 1, CompletionTokens: 2, TotalTokens: 3},
				Done:  true,
			},
		},
	})
	step.Prompt = "say hi"

	var events []core.WorkflowStreamEvent
	ctx := core.WithWorkflowStreamEmitter(context.Background(), func(event core.WorkflowStreamEvent) {
		events = append(events, event)
	})

	out, err := step.Execute(ctx, core.StepInput{})
	if err != nil {
		t.Fatalf("execute llm step failed: %v", err)
	}
	if got := out.Data["content"]; got != "hello" {
		t.Fatalf("unexpected streamed content: %v", got)
	}
	if got := out.Data["reasoning_content"]; got != "think" {
		t.Fatalf("unexpected reasoning content: %v", got)
	}
	if out.Usage == nil || out.Usage.TotalTokens != 3 {
		t.Fatalf("expected streamed usage to propagate, got %#v", out.Usage)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 token events, got %d", len(events))
	}
}

func TestLLMStepExecute_FallsBackToInvokeWhenStreamUnavailable(t *testing.T) {
	step := NewLLMStep("llm-1", &testLLMGateway{
		streamErr:      errors.New("stream unavailable"),
		invokeResponse: &core.LLMResponse{Content: "fallback", Model: "gpt-test"},
	})

	ctx := core.WithWorkflowStreamEmitter(context.Background(), func(event core.WorkflowStreamEvent) {})
	out, err := step.Execute(ctx, core.StepInput{})
	if err != nil {
		t.Fatalf("execute llm step failed: %v", err)
	}
	if got := out.Data["content"]; got != "fallback" {
		t.Fatalf("expected invoke fallback content, got %v", got)
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
	executor := &testAgent{result: "done"}
	step := NewAgentStep("agent-1", executor)
	step.AgentID = "helper"

	out, err := step.Execute(context.Background(), core.StepInput{Data: map[string]any{"task": "x"}})
	if err != nil {
		t.Fatalf("execute agent step failed: %v", err)
	}
	if got := out.Data["result"]; got != "done" {
		t.Fatalf("unexpected agent output: %v", got)
	}
	if got := executor.input["agent_id"]; got != "helper" {
		t.Fatalf("expected injected agent_id, got %v", got)
	}
	if _, exists := executor.input["agent_model"]; exists {
		t.Fatalf("agent_model should not be injected: %v", executor.input)
	}
	if _, exists := executor.input["agent_prompt"]; exists {
		t.Fatalf("agent_prompt should not be injected: %v", executor.input)
	}
	if _, exists := executor.input["agent_tools"]; exists {
		t.Fatalf("agent_tools should not be injected: %v", executor.input)
	}
}

func TestAgentStepExecutePreservesExistingAgentID(t *testing.T) {
	executor := &testAgent{result: "done"}
	step := NewAgentStep("agent-1", executor)
	step.AgentID = "helper"

	_, err := step.Execute(context.Background(), core.StepInput{
		Data: map[string]any{
			"agent_id": "caller",
			"task":     "x",
		},
	})
	if err != nil {
		t.Fatalf("execute agent step failed: %v", err)
	}
	if got := executor.input["agent_id"]; got != "caller" {
		t.Fatalf("expected existing agent_id to win, got %v", got)
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
