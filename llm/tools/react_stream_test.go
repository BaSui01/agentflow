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

type countingToolExecutor struct {
	calls []llmpkg.ToolCall
}

type scriptedProvider struct {
	supportsNative  bool
	streamResponses []<-chan llmpkg.StreamChunk
}

func (p *scriptedProvider) Completion(_ context.Context, _ *llmpkg.ChatRequest) (*llmpkg.ChatResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (p *scriptedProvider) Stream(_ context.Context, _ *llmpkg.ChatRequest) (<-chan llmpkg.StreamChunk, error) {
	if len(p.streamResponses) == 0 {
		ch := make(chan llmpkg.StreamChunk)
		close(ch)
		return ch, nil
	}
	out := p.streamResponses[0]
	p.streamResponses = p.streamResponses[1:]
	return out, nil
}

func (p *scriptedProvider) HealthCheck(_ context.Context) (*llmpkg.HealthStatus, error) {
	return &llmpkg.HealthStatus{Healthy: true}, nil
}

func (p *scriptedProvider) Name() string { return "scripted" }

func (p *scriptedProvider) SupportsNativeFunctionCalling() bool { return p.supportsNative }

func (e *countingToolExecutor) Execute(ctx context.Context, calls []llmpkg.ToolCall) []ToolResult {
	_ = ctx
	e.calls = append(e.calls, calls...)
	out := make([]ToolResult, 0, len(calls))
	for _, c := range calls {
		out = append(out, ToolResult{
			ToolCallID: c.ID,
			Name:       c.Name,
			Result:     json.RawMessage(`{"ok":true}`),
			Duration:   2 * time.Millisecond,
		})
	}
	return out
}

func (e *countingToolExecutor) ExecuteOne(ctx context.Context, call llmpkg.ToolCall) ToolResult {
	return e.Execute(ctx, []llmpkg.ToolCall{call})[0]
}

func TestReActExecutor_ExecuteStream_AssemblesToolCallArgumentsAcrossChunks(t *testing.T) {
	logger := zap.NewNop()

	stream1 := make(chan llmpkg.StreamChunk, 4)
	go func() {
		defer close(stream1)
		stream1 <- llmpkg.StreamChunk{
			ID:       "c1",
			Provider: "scripted",
			Model:    "dummy",
			Delta: llmpkg.Message{
				Role: llmpkg.RoleAssistant,
				ToolCalls: []llmpkg.ToolCall{{
					ID:        "call_1",
					Name:      "echo",
					Arguments: json.RawMessage(`"{\"text\":\"h"`),
				}},
			},
		}
		stream1 <- llmpkg.StreamChunk{
			ID:       "c1",
			Provider: "scripted",
			Model:    "dummy",
			Delta: llmpkg.Message{
				Role: llmpkg.RoleAssistant,
				ToolCalls: []llmpkg.ToolCall{{
					ID:        "call_1",
					Arguments: json.RawMessage(`"i\"}"`),
				}},
			},
			FinishReason: "tool_calls",
		}
	}()

	stream2 := make(chan llmpkg.StreamChunk, 2)
	go func() {
		defer close(stream2)
		stream2 <- llmpkg.StreamChunk{
			ID:       "c2",
			Provider: "scripted",
			Model:    "dummy",
			Delta: llmpkg.Message{
				Role:    llmpkg.RoleAssistant,
				Content: "done",
			},
			FinishReason: "stop",
			Usage:        &llmpkg.ChatUsage{TotalTokens: 7},
		}
	}()

	provider := &scriptedProvider{
		supportsNative:  true,
		streamResponses: []<-chan llmpkg.StreamChunk{stream1, stream2},
	}
	toolExec := &countingToolExecutor{}
	executor := NewReActExecutor(provider, toolExec, ReActConfig{
		MaxIterations: 3,
	}, logger)

	evCh, err := executor.ExecuteStream(context.Background(), &llmpkg.ChatRequest{
		Model:    "dummy",
		Messages: []llmpkg.Message{{Role: llmpkg.RoleUser, Content: "hi"}},
		Tools: []llmpkg.ToolSchema{{
			Name:       "echo",
			Parameters: json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}},"required":["text"]}`),
		}},
	})
	if err != nil {
		t.Fatalf("ExecuteStream failed: %v", err)
	}

	var (
		toolCalls []llmpkg.ToolCall
		final     *llmpkg.ChatResponse
	)
	for ev := range evCh {
		switch ev.Type {
		case "tools_start":
			toolCalls = ev.ToolCalls
		case "completed":
			final = ev.FinalResponse
		case "error":
			t.Fatalf("unexpected error event: %s", ev.Error)
		}
	}

	if len(toolExec.calls) != 1 {
		t.Fatalf("expected 1 tool call execution, got %d", len(toolExec.calls))
	}
	if len(toolCalls) != 1 {
		t.Fatalf("expected 1 tools_start call, got %d", len(toolCalls))
	}
	if got, want := string(toolCalls[0].Arguments), `{"text":"hi"}`; got != want {
		t.Fatalf("arguments mismatch: got=%s want=%s", got, want)
	}
	if final == nil || len(final.Choices) == 0 || final.Choices[0].Message.Content != "done" {
		t.Fatalf("unexpected final response: %#v", final)
	}
}
