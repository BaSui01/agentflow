package tools

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	llmpkg "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// slowStreamingProvider 是一个模拟流式响应的 provider，用于测试空闲超时
type slowStreamingProvider struct {
	chunks         []llmpkg.StreamChunk
	chunkDelay     time.Duration // 每个 chunk 之间的延迟
	finalDelay     time.Duration // 发送完所有 chunks 后等待关闭的时间
	sendFinalChunk bool          // 是否发送最终 chunk
}

func (p *slowStreamingProvider) Completion(_ context.Context, _ *llmpkg.ChatRequest) (*llmpkg.ChatResponse, error) {
	return &llmpkg.ChatResponse{
		Choices: []llmpkg.ChatChoice{{
			FinishReason: "stop",
			Message:      llmpkg.Message{Role: llmpkg.RoleAssistant, Content: "done"},
		}},
	}, nil
}

func (p *slowStreamingProvider) Stream(ctx context.Context, _ *llmpkg.ChatRequest) (<-chan llmpkg.StreamChunk, error) {
	ch := make(chan llmpkg.StreamChunk, len(p.chunks)+1)
	go func() {
		defer close(ch)
		for _, chunk := range p.chunks {
			select {
			case <-ctx.Done():
				return
			case <-time.After(p.chunkDelay):
			}
			select {
			case ch <- chunk:
			case <-ctx.Done():
				return
			}
		}
		// 发送完 chunks 后等待
		if p.finalDelay > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(p.finalDelay):
			}
		}
		// 发送最终 chunk
		if p.sendFinalChunk {
			ch <- llmpkg.StreamChunk{
				ID:           "done",
				Provider:     "test",
				Model:        "test-model",
				FinishReason: "stop",
			}
		}
	}()
	return ch, nil
}

func (p *slowStreamingProvider) HealthCheck(_ context.Context) (*llmpkg.HealthStatus, error) {
	return &llmpkg.HealthStatus{Healthy: true}, nil
}

func (p *slowStreamingProvider) Name() string { return "slow-streaming" }

func (p *slowStreamingProvider) SupportsNativeFunctionCalling() bool { return true }

func (p *slowStreamingProvider) ListModels(_ context.Context) ([]llmpkg.Model, error) {
	return nil, nil
}

func (p *slowStreamingProvider) Endpoints() llmpkg.ProviderEndpoints {
	return llmpkg.ProviderEndpoints{}
}

// noOpToolExecutor 是一个无操作的工具执行器
type noOpToolExecutor struct{}

func (e *noOpToolExecutor) Execute(_ context.Context, calls []types.ToolCall) []types.ToolResult {
	results := make([]types.ToolResult, len(calls))
	for i, c := range calls {
		results[i] = types.ToolResult{
			ToolCallID: c.ID,
			Name:       c.Name,
			Result:     json.RawMessage(`{"ok":true}`),
			Duration:   time.Millisecond,
		}
	}
	return results
}

func (e *noOpToolExecutor) ExecuteOne(ctx context.Context, call types.ToolCall) types.ToolResult {
	return e.Execute(ctx, []types.ToolCall{call})[0]
}

// TestReActExecutor_ExecuteStream_InactivityTimeout 测试空闲超时功能
func TestReActExecutor_ExecuteStream_InactivityTimeout(t *testing.T) {
	logger := zap.NewNop()

	// 创建一个 provider，它发送一个 chunk 后故意暂停 200ms
	// 空闲超时设置为 100ms，所以应该会超时
	provider := &slowStreamingProvider{
		chunks: []llmpkg.StreamChunk{
			{ID: "1", Provider: "test", Model: "test-model", Delta: types.Message{Content: "hello"}},
		},
		chunkDelay:     10 * time.Millisecond,  // 快速发送第一个 chunk
		finalDelay:     200 * time.Millisecond, // 发送完后暂停 200ms（超过空闲超时）
		sendFinalChunk: true,
	}

	executor := NewReActExecutor(provider, &noOpToolExecutor{}, ReActConfig{
		MaxIterations:     5,
		InactivityTimeout: 100 * time.Millisecond, // 空闲超时 100ms
	}, logger)

	eventCh, err := executor.ExecuteStream(context.Background(), &llmpkg.ChatRequest{
		Model:    "test-model",
		Messages: []types.Message{{Role: llmpkg.RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("ExecuteStream failed: %v", err)
	}

	var lastEvent *ReActStreamEvent
	for ev := range eventCh {
		lastEvent = &ev
	}

	// 应该收到一个错误事件（空闲超时）
	if lastEvent == nil {
		t.Fatal("expected at least one event")
	}
	if lastEvent.Type != ReActEventError {
		t.Fatalf("expected error event, got: %s", lastEvent.Type)
	}
	if lastEvent.Error == "" {
		t.Fatal("expected error message")
	}
	t.Logf("Got expected inactivity timeout error: %s", lastEvent.Error)
}

// TestReActExecutor_ExecuteStream_NoTimeoutWhenDataFlowing 测试持续收到数据时不会超时
func TestReActExecutor_ExecuteStream_NoTimeoutWhenDataFlowing(t *testing.T) {
	logger := zap.NewNop()

	// 创建一个 provider，它持续快速发送数据，然后立即完成
	provider := &slowStreamingProvider{
		chunks: []llmpkg.StreamChunk{
			{ID: "1", Provider: "test", Model: "test-model", Delta: types.Message{Content: "a"}},
			{ID: "2", Provider: "test", Model: "test-model", Delta: types.Message{Content: "b"}},
			{ID: "3", Provider: "test", Model: "test-model", Delta: types.Message{Content: "c"}},
		},
		chunkDelay:     10 * time.Millisecond, // 快速发送
		finalDelay:     0,                     // 不暂停，立即完成
		sendFinalChunk: true,
	}

	executor := NewReActExecutor(provider, &noOpToolExecutor{}, ReActConfig{
		MaxIterations:     5,
		InactivityTimeout: 200 * time.Millisecond, // 空闲超时 200ms（大于总发送时间）
	}, logger)

	eventCh, err := executor.ExecuteStream(context.Background(), &llmpkg.ChatRequest{
		Model:    "test-model",
		Messages: []types.Message{{Role: llmpkg.RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("ExecuteStream failed: %v", err)
	}

	var events []ReActStreamEvent
	for ev := range eventCh {
		events = append(events, ev)
	}

	// 应该正常完成，没有超时错误
	var hasError bool
	var errorMsg string
	for _, ev := range events {
		if ev.Type == ReActEventError {
			hasError = true
			errorMsg = ev.Error
		}
	}

	if hasError {
		t.Fatalf("unexpected error: %s", errorMsg)
	}

	// 应该有完成事件
	var hasCompleted bool
	for _, ev := range events {
		if ev.Type == ReActEventCompleted {
			hasCompleted = true
		}
	}
	if !hasCompleted {
		t.Fatal("expected completed event")
	}
	t.Logf("Successfully completed with %d events", len(events))
}
