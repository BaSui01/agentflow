package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	llmpkg "github.com/BaSui01/agentflow/llm/core"
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

func (p *scriptedProvider) ListModels(_ context.Context) ([]llmpkg.Model, error) {
	return nil, nil
}

func (p *scriptedProvider) Endpoints() llmpkg.ProviderEndpoints {
	return llmpkg.ProviderEndpoints{}
}

func (e *countingToolExecutor) Execute(ctx context.Context, calls []llmpkg.ToolCall) []llmpkg.ToolResult {
	_ = ctx
	e.calls = append(e.calls, calls...)
	out := make([]llmpkg.ToolResult, 0, len(calls))
	for _, c := range calls {
		out = append(out, llmpkg.ToolResult{
			ToolCallID: c.ID,
			Name:       c.Name,
			Result:     json.RawMessage(`{"ok":true}`),
			Duration:   2 * time.Millisecond,
		})
	}
	return out
}

func (e *countingToolExecutor) ExecuteOne(ctx context.Context, call llmpkg.ToolCall) llmpkg.ToolResult {
	return e.Execute(ctx, []llmpkg.ToolCall{call})[0]
}

type asyncStreamableToolExecutor struct {
	mu            sync.Mutex
	calls         []llmpkg.ToolCall
	progressCount int
}

func (e *asyncStreamableToolExecutor) Execute(ctx context.Context, calls []llmpkg.ToolCall) []llmpkg.ToolResult {
	out := make([]llmpkg.ToolResult, 0, len(calls))
	for _, call := range calls {
		out = append(out, e.ExecuteOne(ctx, call))
	}
	return out
}

func (e *asyncStreamableToolExecutor) ExecuteOne(ctx context.Context, call llmpkg.ToolCall) llmpkg.ToolResult {
	_ = ctx
	e.mu.Lock()
	e.calls = append(e.calls, call)
	e.mu.Unlock()
	return llmpkg.ToolResult{
		ToolCallID: call.ID,
		Name:       call.Name,
		Result:     json.RawMessage(`{"echo":"hi"}`),
		Duration:   time.Millisecond,
	}
}

func (e *asyncStreamableToolExecutor) ExecuteOneStream(ctx context.Context, call llmpkg.ToolCall) <-chan ToolStreamEvent {
	ch := make(chan ToolStreamEvent, e.progressCount+2)
	go func() {
		defer close(ch)

		e.mu.Lock()
		e.calls = append(e.calls, call)
		e.mu.Unlock()

		send := func(event ToolStreamEvent) bool {
			select {
			case ch <- event:
				return true
			case <-ctx.Done():
				return false
			}
		}

		var wg sync.WaitGroup
		for i := 0; i < e.progressCount; i++ {
			wg.Add(1)
			go func(step int) {
				defer wg.Done()
				send(ToolStreamEvent{
					Type:     ToolStreamProgress,
					ToolName: call.Name,
					Data:     fmt.Sprintf("step-%d", step),
				})
			}(i)
		}
		wg.Wait()

		result := llmpkg.ToolResult{
			ToolCallID: call.ID,
			Name:       call.Name,
			Result:     json.RawMessage(`{"echo":"hi"}`),
			Duration:   time.Millisecond,
		}
		if !send(ToolStreamEvent{Type: ToolStreamOutput, ToolName: call.Name, Data: result.Result}) {
			return
		}
		send(ToolStreamEvent{Type: ToolStreamComplete, ToolName: call.Name, Data: result})
	}()
	return ch
}

func (e *asyncStreamableToolExecutor) snapshotCalls() []llmpkg.ToolCall {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]llmpkg.ToolCall(nil), e.calls...)
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

func TestReActExecutor_ExecuteStream_StreamableToolEventsAreRaceSafe(t *testing.T) {
	logger := zap.NewNop()

	stream1 := make(chan llmpkg.StreamChunk, 3)
	go func() {
		defer close(stream1)
		stream1 <- llmpkg.StreamChunk{
			ID:       "c1",
			Provider: "scripted",
			Model:    "dummy",
			Delta: llmpkg.Message{
				Role:    llmpkg.RoleAssistant,
				Content: "checking ",
			},
		}
		stream1 <- llmpkg.StreamChunk{
			ID:       "c1",
			Provider: "scripted",
			Model:    "dummy",
			Delta: llmpkg.Message{
				Role: llmpkg.RoleAssistant,
				ToolCalls: []llmpkg.ToolCall{{
					Index:     0,
					ID:        "call_stream_1",
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
					Index:     0,
					ID:        "call_stream_1",
					Arguments: json.RawMessage(`"i\"}"`),
				}},
			},
			FinishReason: "tool_calls",
		}
	}()

	stream2 := make(chan llmpkg.StreamChunk, 1)
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
		}
	}()

	provider := &scriptedProvider{
		supportsNative:  true,
		streamResponses: []<-chan llmpkg.StreamChunk{stream1, stream2},
	}
	toolExec := &asyncStreamableToolExecutor{progressCount: 8}
	executor := NewReActExecutor(provider, toolExec, ReActConfig{
		MaxIterations:     2,
		InactivityTimeout: time.Second,
	}, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	evCh, err := executor.ExecuteStream(ctx, &llmpkg.ChatRequest{
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

	typeCounts := map[string]int{}
	var (
		toolCalls   []llmpkg.ToolCall
		toolResults []llmpkg.ToolResult
		final       *llmpkg.ChatResponse
	)
	for ev := range evCh {
		typeCounts[ev.Type]++
		switch ev.Type {
		case ReActEventToolsStart:
			toolCalls = ev.ToolCalls
		case ReActEventToolsEnd:
			toolResults = ev.ToolResults
		case ReActEventCompleted:
			final = ev.FinalResponse
		case ReActEventError:
			t.Fatalf("unexpected error event: %s", ev.Error)
		}
	}

	if got := typeCounts[ReActEventLLMChunk]; got != 4 {
		t.Fatalf("expected 4 llm_chunk events, got %d", got)
	}
	if got := typeCounts[ReActEventToolsStart]; got != 1 {
		t.Fatalf("expected 1 tools_start event, got %d", got)
	}
	if got := typeCounts[ReActEventToolProgress]; got != toolExec.progressCount {
		t.Fatalf("expected %d tool_progress events, got %d", toolExec.progressCount, got)
	}
	if got := typeCounts[ReActEventToolsEnd]; got != 1 {
		t.Fatalf("expected 1 tools_end event, got %d", got)
	}
	if got := typeCounts[ReActEventCompleted]; got != 1 {
		t.Fatalf("expected 1 completed event, got %d", got)
	}
	if len(toolCalls) != 1 {
		t.Fatalf("expected one tool call, got %d", len(toolCalls))
	}
	if got, want := string(toolCalls[0].Arguments), `{"text":"hi"}`; got != want {
		t.Fatalf("arguments mismatch: got=%s want=%s", got, want)
	}
	if len(toolResults) != 1 {
		t.Fatalf("expected one tool result, got %d", len(toolResults))
	}
	if got, want := string(toolResults[0].Result), `{"echo":"hi"}`; got != want {
		t.Fatalf("tool result mismatch: got=%s want=%s", got, want)
	}
	if final == nil || len(final.Choices) == 0 || final.Choices[0].Message.Content != "done" {
		t.Fatalf("unexpected final response: %#v", final)
	}

	executedCalls := toolExec.snapshotCalls()
	if len(executedCalls) != 1 || executedCalls[0].ID != "call_stream_1" {
		t.Fatalf("unexpected executed calls: %#v", executedCalls)
	}
}

func TestReActExecutor_ExecuteStream_PreservesReasoningMetadata(t *testing.T) {
	logger := zap.NewNop()

	stream1 := make(chan llmpkg.StreamChunk, 2)
	go func() {
		defer close(stream1)
		stream1 <- llmpkg.StreamChunk{
			ID:       "c1",
			Provider: "scripted",
			Model:    "dummy",
			Delta: llmpkg.Message{
				Role:             llmpkg.RoleAssistant,
				ReasoningContent: strPtr("summary"),
				ReasoningSummaries: []llmpkg.ReasoningSummary{
					{Provider: "openai", ID: "rs_1", Kind: "summary_text", Text: "summary"},
				},
				OpaqueReasoning: []llmpkg.OpaqueReasoning{
					{Provider: "openai", ID: "rs_1", Kind: "encrypted_content", State: "enc_1"},
				},
				ThinkingBlocks: []llmpkg.ThinkingBlock{
					{Thinking: "step 1", Signature: "sig_1"},
				},
			},
			FinishReason: "stop",
		}
	}()

	provider := &scriptedProvider{
		supportsNative:  true,
		streamResponses: []<-chan llmpkg.StreamChunk{stream1},
	}
	toolExec := &countingToolExecutor{}
	executor := NewReActExecutor(provider, toolExec, ReActConfig{MaxIterations: 1}, logger)

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

	var final *llmpkg.ChatResponse
	for ev := range evCh {
		switch ev.Type {
		case "completed":
			final = ev.FinalResponse
		case "error":
			t.Fatalf("unexpected error event: %s", ev.Error)
		}
	}

	if final == nil || len(final.Choices) == 0 {
		t.Fatalf("unexpected final response: %#v", final)
	}
	msg := final.Choices[0].Message
	if msg.ReasoningContent == nil || *msg.ReasoningContent != "summary" {
		t.Fatalf("unexpected reasoning content: %#v", msg.ReasoningContent)
	}
	if len(msg.ReasoningSummaries) != 1 || msg.ReasoningSummaries[0].Text != "summary" {
		t.Fatalf("unexpected reasoning summaries: %#v", msg.ReasoningSummaries)
	}
	if len(msg.OpaqueReasoning) != 1 || msg.OpaqueReasoning[0].State != "enc_1" {
		t.Fatalf("unexpected opaque reasoning: %#v", msg.OpaqueReasoning)
	}
	if len(msg.ThinkingBlocks) != 1 || msg.ThinkingBlocks[0].Signature != "sig_1" {
		t.Fatalf("unexpected thinking blocks: %#v", msg.ThinkingBlocks)
	}
}

func strPtr(s string) *string { return &s }
