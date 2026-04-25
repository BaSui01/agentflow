package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	llmpkg "github.com/BaSui01/agentflow/llm/core"
	"go.uber.org/zap"
)

type scriptedCompletionProvider struct {
	responses []*llmpkg.ChatResponse
	err       error
	calls     int
}

func (p *scriptedCompletionProvider) Completion(_ context.Context, _ *llmpkg.ChatRequest) (*llmpkg.ChatResponse, error) {
	p.calls++
	if p.err != nil {
		return nil, p.err
	}
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

func (p *scriptedCompletionProvider) ListModels(_ context.Context) ([]llmpkg.Model, error) {
	return nil, nil
}

func (p *scriptedCompletionProvider) Endpoints() llmpkg.ProviderEndpoints {
	return llmpkg.ProviderEndpoints{}
}

type scriptedToolExecutor struct {
	calls int
	failN int
}

func (e *scriptedToolExecutor) Execute(_ context.Context, calls []llmpkg.ToolCall) []llmpkg.ToolResult {
	out := make([]llmpkg.ToolResult, 0, len(calls))
	for _, c := range calls {
		e.calls++
		if e.failN > 0 {
			e.failN--
			out = append(out, llmpkg.ToolResult{
				ToolCallID: c.ID,
				Name:       c.Name,
				Error:      "invalid arguments",
				Duration:   time.Millisecond,
			})
			continue
		}
		out = append(out, llmpkg.ToolResult{
			ToolCallID: c.ID,
			Name:       c.Name,
			Result:     json.RawMessage(`{"ok":true}`),
			Duration:   time.Millisecond,
		})
	}
	return out
}

func (e *scriptedToolExecutor) ExecuteOne(ctx context.Context, call llmpkg.ToolCall) llmpkg.ToolResult {
	return e.Execute(ctx, []llmpkg.ToolCall{call})[0]
}

type cancelAfterToolExecutor struct {
	cancel context.CancelFunc
}

func (e cancelAfterToolExecutor) Execute(_ context.Context, calls []llmpkg.ToolCall) []llmpkg.ToolResult {
	out := make([]llmpkg.ToolResult, 0, len(calls))
	for _, call := range calls {
		out = append(out, llmpkg.ToolResult{
			ToolCallID: call.ID,
			Name:       call.Name,
			Result:     json.RawMessage(`{"ok":true}`),
			Duration:   time.Millisecond,
		})
	}
	e.cancel()
	return out
}

func (e cancelAfterToolExecutor) ExecuteOne(ctx context.Context, call llmpkg.ToolCall) llmpkg.ToolResult {
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

func TestReActExecutor_Execute_ContextCancelledBeforeFirstLLMCall(t *testing.T) {
	provider := &scriptedCompletionProvider{
		responses: []*llmpkg.ChatResponse{{
			Choices: []llmpkg.ChatChoice{{
				FinishReason: "stop",
				Message: llmpkg.Message{
					Role:    llmpkg.RoleAssistant,
					Content: "should not be called",
				},
			}},
		}},
	}
	executor := NewReActExecutor(provider, &scriptedToolExecutor{}, ReActConfig{MaxIterations: 1}, zap.NewNop())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	resp, steps, err := executor.Execute(ctx, &llmpkg.ChatRequest{
		Model:    "dummy",
		Messages: []llmpkg.Message{{Role: llmpkg.RoleUser, Content: "hi"}},
	})
	if err == nil {
		t.Fatalf("expected context cancellation error, got nil")
	}
	if !strings.Contains(err.Error(), "context cancelled") {
		t.Fatalf("expected context cancellation error, got %v", err)
	}
	if resp != nil {
		t.Fatalf("expected no response, got %#v", resp)
	}
	if len(steps) != 0 {
		t.Fatalf("expected no steps, got %d", len(steps))
	}
	if provider.calls != 0 {
		t.Fatalf("expected provider not to be called, got %d calls", provider.calls)
	}
}

func TestReActExecutor_Execute_ContextCancelledAfterToolResultStopsNextLLMCall(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	provider := &scriptedCompletionProvider{
		responses: []*llmpkg.ChatResponse{
			{
				Choices: []llmpkg.ChatChoice{{
					FinishReason: "tool_calls",
					Message: llmpkg.Message{
						Role: llmpkg.RoleAssistant,
						ToolCalls: []llmpkg.ToolCall{{
							ID:        "call_cancel",
							Name:      "cancel_after_tool",
							Arguments: json.RawMessage(`{"ok":true}`),
						}},
					},
				}},
			},
			{
				Choices: []llmpkg.ChatChoice{{
					FinishReason: "stop",
					Message: llmpkg.Message{
						Role:    llmpkg.RoleAssistant,
						Content: "should not be called",
					},
				}},
			},
		},
	}
	executor := NewReActExecutor(provider, cancelAfterToolExecutor{cancel: cancel}, ReActConfig{MaxIterations: 2}, zap.NewNop())

	resp, steps, err := executor.Execute(ctx, &llmpkg.ChatRequest{
		Model:    "dummy",
		Messages: []llmpkg.Message{{Role: llmpkg.RoleUser, Content: "hi"}},
	})
	if err == nil {
		t.Fatalf("expected context cancellation error, got nil")
	}
	if !strings.Contains(err.Error(), "context cancelled") {
		t.Fatalf("expected context cancellation error, got %v", err)
	}
	if provider.calls != 1 {
		t.Fatalf("expected provider to stop after first call, got %d calls", provider.calls)
	}
	if resp == nil {
		t.Fatalf("expected last response to be returned")
	}
	if len(steps) != 1 {
		t.Fatalf("expected one completed tool step, got %d", len(steps))
	}
	if len(steps[0].Observations) != 1 || steps[0].Observations[0].Error != "" {
		t.Fatalf("expected successful tool observation before cancellation, got %#v", steps[0].Observations)
	}
}

func TestReActExecutor_Execute_ProviderErrorIsControlled(t *testing.T) {
	logger := zap.NewNop()
	provider := &scriptedCompletionProvider{err: fmt.Errorf("provider unavailable")}
	executor := NewReActExecutor(provider, &scriptedToolExecutor{}, ReActConfig{MaxIterations: 1}, logger)

	resp, steps, err := executor.Execute(context.Background(), &llmpkg.ChatRequest{
		Model:    "dummy",
		Messages: []llmpkg.Message{{Role: llmpkg.RoleUser, Content: "hi"}},
	})
	if err == nil {
		t.Fatalf("expected provider error, got nil")
	}
	if !strings.Contains(err.Error(), "LLM call failed at iteration 1") {
		t.Fatalf("expected controlled provider error, got %v", err)
	}
	if resp != nil {
		t.Fatalf("expected no response, got %#v", resp)
	}
	if len(steps) != 0 {
		t.Fatalf("expected no steps, got %d", len(steps))
	}
}

func TestReActExecutor_Execute_ToolErrorsStopOnMalformedOrUnknownTool(t *testing.T) {
	tests := []struct {
		name        string
		call        llmpkg.ToolCall
		register    bool
		wantErrText string
	}{
		{
			name: "malformed arguments",
			call: llmpkg.ToolCall{
				ID:        "call_bad_args",
				Name:      "echo",
				Arguments: json.RawMessage(`{bad`),
			},
			register:    true,
			wantErrText: "invalid arguments",
		},
		{
			name: "unknown tool",
			call: llmpkg.ToolCall{
				ID:        "call_unknown",
				Name:      "missing_tool",
				Arguments: json.RawMessage(`{"x":1}`),
			},
			wantErrText: "tool not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &scriptedCompletionProvider{
				responses: []*llmpkg.ChatResponse{{
					Choices: []llmpkg.ChatChoice{{
						FinishReason: "tool_calls",
						Message: llmpkg.Message{
							Role:      llmpkg.RoleAssistant,
							ToolCalls: []llmpkg.ToolCall{tt.call},
						},
					}},
				}},
			}
			registry := NewDefaultRegistry(zap.NewNop())
			if tt.register {
				err := registry.Register("echo", func(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
					return args, nil
				}, ToolMetadata{
					Schema: llmpkg.ToolSchema{
						Name:       "echo",
						Parameters: json.RawMessage(`{"type":"object"}`),
					},
				})
				if err != nil {
					t.Fatalf("register tool: %v", err)
				}
			}
			executor := NewReActExecutor(provider, NewDefaultExecutor(registry, zap.NewNop()), ReActConfig{MaxIterations: 1, StopOnError: true}, zap.NewNop())

			resp, steps, err := executor.Execute(context.Background(), &llmpkg.ChatRequest{
				Model:    "dummy",
				Messages: []llmpkg.Message{{Role: llmpkg.RoleUser, Content: "hi"}},
			})
			if err == nil {
				t.Fatalf("expected tool error, got nil")
			}
			if !strings.Contains(err.Error(), "tool execution failed") {
				t.Fatalf("expected controlled tool execution error, got %v", err)
			}
			if resp == nil {
				t.Fatalf("expected last response to be returned")
			}
			if len(steps) != 1 {
				t.Fatalf("expected one step, got %d", len(steps))
			}
			if len(steps[0].Observations) != 1 {
				t.Fatalf("expected one observation, got %d", len(steps[0].Observations))
			}
			if !strings.Contains(steps[0].Observations[0].Error, tt.wantErrText) {
				t.Fatalf("expected observation error containing %q, got %q", tt.wantErrText, steps[0].Observations[0].Error)
			}
		})
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
	if resp == nil {
		t.Fatalf("expected last response to be returned, got nil")
	}
	if len(steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(steps))
	}
}
