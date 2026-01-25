package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	llmpkg "github.com/BaSui01/agentflow/llm"
	"go.uber.org/zap"
)

type scriptedCompletionProvider struct {
	responses []*llmpkg.ChatResponse
}

func (p *scriptedCompletionProvider) Completion(_ context.Context, _ *llmpkg.ChatRequest) (*llmpkg.ChatResponse, error) {
	if len(p.responses) == 0 {
		return nil, fmt.Errorf("no more responses")
	}
	resp := p.responses[0]
	p.responses = p.responses[1:]
	return resp, nil
}

func (p *scriptedCompletionProvider) Stream(_ context.Context, _ *llmpkg.ChatRequest) (<-chan llmpkg.StreamChunk, error) {
	ch := make(chan llmpkg.StreamChunk)
	close(ch)
	return ch, nil
}

func (p *scriptedCompletionProvider) HealthCheck(_ context.Context) (*llmpkg.HealthStatus, error) {
	return &llmpkg.HealthStatus{Healthy: true}, nil
}

func (p *scriptedCompletionProvider) Name() string { return "scripted" }

func (p *scriptedCompletionProvider) SupportsNativeFunctionCalling() bool { return true }

type scriptedToolExecutor struct {
	calls int
	failN int
}

func (e *scriptedToolExecutor) Execute(_ context.Context, calls []llmpkg.ToolCall) []ToolResult {
	out := make([]ToolResult, 0, len(calls))
	for _, c := range calls {
		e.calls++
		if e.failN > 0 {
			e.failN--
			out = append(out, ToolResult{
				ToolCallID: c.ID,
				Name:       c.Name,
				Error:      "invalid arguments",
				Duration:   time.Millisecond,
			})
			continue
		}
		out = append(out, ToolResult{
			ToolCallID: c.ID,
			Name:       c.Name,
			Result:     json.RawMessage(`{"ok":true}`),
			Duration:   time.Millisecond,
		})
	}
	return out
}

func (e *scriptedToolExecutor) ExecuteOne(ctx context.Context, call llmpkg.ToolCall) ToolResult {
	return e.Execute(ctx, []llmpkg.ToolCall{call})[0]
}

func TestReActExecutor_Execute_MultiTurnToolLoop_Success(t *testing.T) {
	logger := zap.NewNop()
	provider := &scriptedCompletionProvider{
		responses: []*llmpkg.ChatResponse{
			{
				Choices: []llmpkg.ChatChoice{{
					FinishReason: "tool_calls",
					Message: llmpkg.Message{
						Role: llmpkg.RoleAssistant,
						ToolCalls: []llmpkg.ToolCall{{
							ID:        "call_1",
							Name:      "echo",
							Arguments: json.RawMessage(`{"text":"hi"}`),
						}},
					},
				}},
			},
			{
				Choices: []llmpkg.ChatChoice{{
					FinishReason: "stop",
					Message: llmpkg.Message{
						Role:    llmpkg.RoleAssistant,
						Content: "done",
					},
				}},
			},
		},
	}
	toolExec := &scriptedToolExecutor{}
	executor := NewReActExecutor(provider, toolExec, ReActConfig{MaxIterations: 5}, logger)

	resp, steps, err := executor.Execute(context.Background(), &llmpkg.ChatRequest{
		Model:    "dummy",
		Messages: []llmpkg.Message{{Role: llmpkg.RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if toolExec.calls != 1 {
		t.Fatalf("expected 1 tool execution, got %d", toolExec.calls)
	}
	if len(steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(steps))
	}
	if resp == nil || len(resp.Choices) == 0 || resp.Choices[0].Message.Content != "done" {
		t.Fatalf("unexpected final response: %#v", resp)
	}
}

func TestReActExecutor_Execute_ToolFailureCanContinue_AndReachFinal(t *testing.T) {
	logger := zap.NewNop()
	provider := &scriptedCompletionProvider{
		responses: []*llmpkg.ChatResponse{
			{
				Choices: []llmpkg.ChatChoice{{
					FinishReason: "tool_calls",
					Message: llmpkg.Message{
						Role: llmpkg.RoleAssistant,
						ToolCalls: []llmpkg.ToolCall{{
							ID:        "call_1",
							Name:      "may_fail",
							Arguments: json.RawMessage(`{"x":1}`),
						}},
					},
				}},
			},
			{
				Choices: []llmpkg.ChatChoice{{
					FinishReason: "tool_calls",
					Message: llmpkg.Message{
						Role: llmpkg.RoleAssistant,
						ToolCalls: []llmpkg.ToolCall{{
							ID:        "call_2",
							Name:      "retry",
							Arguments: json.RawMessage(`{"x":2}`),
						}},
					},
				}},
			},
			{
				Choices: []llmpkg.ChatChoice{{
					FinishReason: "stop",
					Message: llmpkg.Message{
						Role:    llmpkg.RoleAssistant,
						Content: "done",
					},
				}},
			},
		},
	}
	toolExec := &scriptedToolExecutor{failN: 1}
	executor := NewReActExecutor(provider, toolExec, ReActConfig{MaxIterations: 5}, logger)

	resp, steps, err := executor.Execute(context.Background(), &llmpkg.ChatRequest{
		Model:    "dummy",
		Messages: []llmpkg.Message{{Role: llmpkg.RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if toolExec.calls != 2 {
		t.Fatalf("expected 2 tool executions, got %d", toolExec.calls)
	}
	if len(steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(steps))
	}
	if resp == nil || len(resp.Choices) == 0 || resp.Choices[0].Message.Content != "done" {
		t.Fatalf("unexpected final response: %#v", resp)
	}
}

func TestReActExecutor_Execute_MaxIterationsReached(t *testing.T) {
	logger := zap.NewNop()
	provider := &scriptedCompletionProvider{
		responses: []*llmpkg.ChatResponse{
			{
				Choices: []llmpkg.ChatChoice{{
					FinishReason: "tool_calls",
					Message: llmpkg.Message{
						Role: llmpkg.RoleAssistant,
						ToolCalls: []llmpkg.ToolCall{{
							ID:        "call_1",
							Name:      "loop",
							Arguments: json.RawMessage(`{"x":1}`),
						}},
					},
				}},
			},
			{
				Choices: []llmpkg.ChatChoice{{
					FinishReason: "tool_calls",
					Message: llmpkg.Message{
						Role: llmpkg.RoleAssistant,
						ToolCalls: []llmpkg.ToolCall{{
							ID:        "call_2",
							Name:      "loop",
							Arguments: json.RawMessage(`{"x":2}`),
						}},
					},
				}},
			},
		},
	}
	toolExec := &scriptedToolExecutor{}
	executor := NewReActExecutor(provider, toolExec, ReActConfig{MaxIterations: 2}, logger)

	resp, steps, err := executor.Execute(context.Background(), &llmpkg.ChatRequest{
		Model:    "dummy",
		Messages: []llmpkg.Message{{Role: llmpkg.RoleUser, Content: "hi"}},
	})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if resp != nil {
		t.Fatalf("expected nil response, got %#v", resp)
	}
	if len(steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(steps))
	}
}
