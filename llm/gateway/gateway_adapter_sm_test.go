package gateway

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/BaSui01/agentflow/llm"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
)

// ═══ StateMachine 测试 ═══

func TestStateMachine_NewStateMachine(t *testing.T) {
	sm := NewStateMachine()
	if sm.Current() != StatePlanned {
		t.Fatalf("expected initial state=planned, got %s", sm.Current())
	}
}

func TestStateMachine_ValidTransitions(t *testing.T) {
	tests := []struct {
		name  string
		steps []RequestState
	}{
		{"happy path", []RequestState{StateValidated, StateRouted, StateExecuting, StateCompleted}},
		{"streaming path", []RequestState{StateValidated, StateRouted, StateExecuting, StateStreaming, StateCompleted}},
		{"failure path", []RequestState{StateValidated, StateRouted, StateExecuting, StateFailed}},
		{"retry path", []RequestState{StateValidated, StateRouted, StateExecuting, StateRetried, StateExecuting, StateCompleted}},
		{"degraded path", []RequestState{StateValidated, StateRouted, StateExecuting, StateDegraded, StateCompleted}},
		{"streaming failure", []RequestState{StateValidated, StateRouted, StateExecuting, StateStreaming, StateFailed}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm := NewStateMachine()
			for _, step := range tt.steps {
				if err := sm.Transition(step); err != nil {
					t.Fatalf("transition to %s failed: %v", step, err)
				}
			}
			if sm.Current() != tt.steps[len(tt.steps)-1] {
				t.Fatalf("expected final state=%s, got %s", tt.steps[len(tt.steps)-1], sm.Current())
			}
		})
	}
}

func TestStateMachine_InvalidTransitions(t *testing.T) {
	tests := []struct {
		name string
		from RequestState
		to   RequestState
	}{
		{"planned->completed", StatePlanned, StateCompleted},
		{"planned->executing", StatePlanned, StateExecuting},
		{"validated->completed", StateValidated, StateCompleted},
		{"completed->planned", StateCompleted, StatePlanned},
		{"streaming->planned", StateStreaming, StatePlanned},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm := &StateMachine{state: tt.from}
			err := sm.Transition(tt.to)
			if err == nil {
				t.Fatalf("expected error for %s->%s, got nil", tt.from, tt.to)
			}
		})
	}
}

func TestStateMachine_ConcurrentAccess(t *testing.T) {
	sm := NewStateMachine()
	sm.Transition(StateValidated)
	sm.Transition(StateRouted)
	sm.Transition(StateExecuting)

	var wg sync.WaitGroup
	// 并发读
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = sm.Current()
		}()
	}
	wg.Wait()
}

// ═══ ChatProviderAdapter 测试 ═══

type mockGateway struct {
	invokeResp *llmcore.UnifiedResponse
	invokeErr  error
	streamCh   <-chan llmcore.UnifiedChunk
	streamErr  error
}

func (g *mockGateway) Invoke(_ context.Context, _ *llmcore.UnifiedRequest) (*llmcore.UnifiedResponse, error) {
	return g.invokeResp, g.invokeErr
}

func (g *mockGateway) Stream(_ context.Context, _ *llmcore.UnifiedRequest) (<-chan llmcore.UnifiedChunk, error) {
	return g.streamCh, g.streamErr
}

type mockFallbackProvider struct {
	name string
}

func (p *mockFallbackProvider) Name() string                                                          { return p.name }
func (p *mockFallbackProvider) Completion(_ context.Context, _ *llm.ChatRequest) (*llm.ChatResponse, error) { return nil, nil }
func (p *mockFallbackProvider) Stream(_ context.Context, _ *llm.ChatRequest) (<-chan llm.StreamChunk, error) { return nil, nil }
func (p *mockFallbackProvider) HealthCheck(_ context.Context) (*llm.HealthStatus, error) { return &llm.HealthStatus{Healthy: true}, nil }
func (p *mockFallbackProvider) SupportsNativeFunctionCalling() bool { return true }
func (p *mockFallbackProvider) ListModels(_ context.Context) ([]llm.Model, error) { return []llm.Model{{ID: "test"}}, nil }
func (p *mockFallbackProvider) Endpoints() llm.ProviderEndpoints { return llm.ProviderEndpoints{BaseURL: "http://test"} }

func TestChatProviderAdapter_NilGateway(t *testing.T) {
	adapter := NewChatProviderAdapter(nil, nil)

	_, err := adapter.Completion(context.Background(), &llm.ChatRequest{Model: "test"})
	if err == nil {
		t.Fatal("expected error with nil gateway")
	}

	_, err = adapter.Stream(context.Background(), &llm.ChatRequest{Model: "test"})
	if err == nil {
		t.Fatal("expected error with nil gateway for stream")
	}
}

func TestChatProviderAdapter_Completion_Success(t *testing.T) {
	expected := &llm.ChatResponse{
		ID: "resp1", Model: "test",
		Choices: []llm.ChatChoice{{Message: types.Message{Content: "hello"}}},
	}
	gw := &mockGateway{
		invokeResp: &llmcore.UnifiedResponse{Output: expected},
	}
	adapter := NewChatProviderAdapter(gw, nil)

	resp, err := adapter.Completion(context.Background(), &llm.ChatRequest{Model: "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ID != "resp1" || resp.Choices[0].Message.Content != "hello" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestChatProviderAdapter_Completion_GatewayError(t *testing.T) {
	gw := &mockGateway{invokeErr: fmt.Errorf("gateway down")}
	adapter := NewChatProviderAdapter(gw, nil)

	_, err := adapter.Completion(context.Background(), &llm.ChatRequest{Model: "test"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestChatProviderAdapter_Completion_InvalidResponse(t *testing.T) {
	gw := &mockGateway{
		invokeResp: &llmcore.UnifiedResponse{Output: "not a ChatResponse"},
	}
	adapter := NewChatProviderAdapter(gw, nil)

	_, err := adapter.Completion(context.Background(), &llm.ChatRequest{Model: "test"})
	if err == nil {
		t.Fatal("expected error for invalid response type")
	}
}

func TestChatProviderAdapter_Stream_Success(t *testing.T) {
	ch := make(chan llmcore.UnifiedChunk, 3)
	ch <- llmcore.UnifiedChunk{Output: &llm.StreamChunk{Delta: types.Message{Content: "hi"}}}
	ch <- llmcore.UnifiedChunk{Output: &llm.StreamChunk{Delta: types.Message{Content: " there"}}}
	close(ch)

	gw := &mockGateway{streamCh: ch}
	adapter := NewChatProviderAdapter(gw, nil)

	stream, err := adapter.Stream(context.Background(), &llm.ChatRequest{Model: "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var content string
	for chunk := range stream {
		if chunk.Err != nil {
			t.Fatalf("unexpected chunk error: %v", chunk.Err)
		}
		content += chunk.Delta.Content
	}
	if content != "hi there" {
		t.Fatalf("expected 'hi there', got '%s'", content)
	}
}

func TestChatProviderAdapter_Stream_Error(t *testing.T) {
	gw := &mockGateway{streamErr: fmt.Errorf("stream failed")}
	adapter := NewChatProviderAdapter(gw, nil)

	_, err := adapter.Stream(context.Background(), &llm.ChatRequest{Model: "test"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestChatProviderAdapter_Stream_ChunkError(t *testing.T) {
	ch := make(chan llmcore.UnifiedChunk, 1)
	ch <- llmcore.UnifiedChunk{Err: &types.Error{Code: types.ErrUpstreamError, Message: "upstream fail"}}
	close(ch)

	gw := &mockGateway{streamCh: ch}
	adapter := NewChatProviderAdapter(gw, nil)

	stream, err := adapter.Stream(context.Background(), &llm.ChatRequest{Model: "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	chunk := <-stream
	if chunk.Err == nil {
		t.Fatal("expected chunk error")
	}
}

func TestChatProviderAdapter_Stream_InvalidChunkType(t *testing.T) {
	ch := make(chan llmcore.UnifiedChunk, 1)
	ch <- llmcore.UnifiedChunk{Output: "not a StreamChunk"}
	close(ch)

	gw := &mockGateway{streamCh: ch}
	adapter := NewChatProviderAdapter(gw, nil)

	stream, _ := adapter.Stream(context.Background(), &llm.ChatRequest{Model: "test"})
	chunk := <-stream
	if chunk.Err == nil {
		t.Fatal("expected error for invalid chunk type")
	}
}

func TestChatProviderAdapter_WithFallback(t *testing.T) {
	fb := &mockFallbackProvider{name: "test-provider"}
	adapter := NewChatProviderAdapter(nil, fb)

	if adapter.Name() != "test-provider" {
		t.Fatalf("expected name=test-provider, got %s", adapter.Name())
	}
	if !adapter.SupportsNativeFunctionCalling() {
		t.Fatal("expected SupportsNativeFunctionCalling=true from fallback")
	}
	models, _ := adapter.ListModels(context.Background())
	if len(models) != 1 {
		t.Fatalf("expected 1 model from fallback, got %d", len(models))
	}
	ep := adapter.Endpoints()
	if ep.BaseURL != "http://test" {
		t.Fatalf("expected BaseURL from fallback, got %s", ep.BaseURL)
	}
	health, _ := adapter.HealthCheck(context.Background())
	if !health.Healthy {
		t.Fatal("expected healthy from fallback")
	}
}

func TestChatProviderAdapter_WithoutFallback(t *testing.T) {
	adapter := NewChatProviderAdapter(nil, nil)

	if adapter.Name() != "gateway" {
		t.Fatalf("expected name=gateway, got %s", adapter.Name())
	}
	if adapter.SupportsNativeFunctionCalling() {
		t.Fatal("expected false without fallback")
	}
	if adapter.SupportsStructuredOutput() {
		t.Fatal("expected false without fallback")
	}
	models, _ := adapter.ListModels(context.Background())
	if models != nil {
		t.Fatalf("expected nil models, got %v", models)
	}
	ep := adapter.Endpoints()
	if ep.BaseURL != "" {
		t.Fatalf("expected empty endpoints, got %+v", ep)
	}
	health, _ := adapter.HealthCheck(context.Background())
	if !health.Healthy {
		t.Fatal("expected healthy default")
	}
}

func TestChatProviderAdapter_Stream_ContextCancel(t *testing.T) {
	ch := make(chan llmcore.UnifiedChunk, 10)
	// 持续发送数据模拟无限流
	go func() {
		for i := 0; i < 100; i++ {
			ch <- llmcore.UnifiedChunk{Output: &llm.StreamChunk{Delta: types.Message{Content: "x"}}}
		}
		// 不关闭 ch，模拟长流
	}()

	gw := &mockGateway{streamCh: ch}
	adapter := NewChatProviderAdapter(gw, nil)

	ctx, cancel := context.WithCancel(context.Background())
	stream, err := adapter.Stream(ctx, &llm.ChatRequest{Model: "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 读几个 chunk 后取消
	<-stream
	<-stream
	cancel()

	// stream 应该最终关闭（不死锁）
	count := 0
	for range stream {
		count++
		if count > 200 {
			t.Fatal("stream not closing after cancel")
		}
	}
	// 到这里没死锁就是 PASS
}
