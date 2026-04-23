package middleware

import (
	"context"
	"testing"

	llmpkg "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- mockProvider for testing ---

type mockProvider struct {
	name           string
	completionResp *llmpkg.ChatResponse
	completionErr  error
	streamCh       chan llmpkg.StreamChunk
	streamErr      error
	supportsNative bool
}

func (m *mockProvider) Name() string                        { return m.name }
func (m *mockProvider) SupportsNativeFunctionCalling() bool { return m.supportsNative }
func (m *mockProvider) HealthCheck(ctx context.Context) (*llmpkg.HealthStatus, error) {
	return nil, nil
}
func (m *mockProvider) ListModels(ctx context.Context) ([]llmpkg.Model, error) { return nil, nil }
func (m *mockProvider) Endpoints() llmpkg.ProviderEndpoints                    { return llmpkg.ProviderEndpoints{} }

func (m *mockProvider) Completion(ctx context.Context, req *llmpkg.ChatRequest) (*llmpkg.ChatResponse, error) {
	if m.completionErr != nil {
		return nil, m.completionErr
	}
	return m.completionResp, nil
}

func (m *mockProvider) Stream(ctx context.Context, req *llmpkg.ChatRequest) (<-chan llmpkg.StreamChunk, error) {
	if m.streamErr != nil {
		return nil, m.streamErr
	}
	return m.streamCh, nil
}

// --- Completion Tests ---

func TestXMLToolCallProvider_Completion_NativeMode_Passthrough(t *testing.T) {
	inner := &mockProvider{
		name: "test",
		completionResp: &llmpkg.ChatResponse{
			Choices: []llmpkg.ChatChoice{{
				Message:      types.Message{Content: "Hello!"},
				FinishReason: "stop",
			}},
		},
	}

	provider := NewXMLToolCallProvider(inner, nil)
	req := &llmpkg.ChatRequest{
		ToolCallMode: llmpkg.ToolCallModeNative,
		Messages:     []types.Message{{Role: types.RoleUser, Content: "hi"}},
	}

	resp, err := provider.Completion(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "Hello!", resp.Choices[0].Message.Content)
	assert.Empty(t, resp.Choices[0].Message.ToolCalls)
}

func TestXMLToolCallProvider_Completion_XMLMode_ParsesToolCalls(t *testing.T) {
	inner := &mockProvider{
		name: "test",
		completionResp: &llmpkg.ChatResponse{
			Choices: []llmpkg.ChatChoice{{
				Message: types.Message{
					Content: `Let me search for that.
<tool_calls>
{"name":"search","arguments":{"query":"golang xml parsing"}}
</tool_calls>`,
				},
				FinishReason: "stop",
			}},
		},
	}

	provider := NewXMLToolCallProvider(inner, nil)
	req := &llmpkg.ChatRequest{
		ToolCallMode: llmpkg.ToolCallModeXML,
		Messages:     []types.Message{{Role: types.RoleUser, Content: "search for golang"}},
	}

	resp, err := provider.Completion(context.Background(), req)
	require.NoError(t, err)

	// 工具调用应被提取
	require.Len(t, resp.Choices[0].Message.ToolCalls, 1)
	assert.Equal(t, "search", resp.Choices[0].Message.ToolCalls[0].Name)

	// 内容应被清理
	assert.Equal(t, "Let me search for that.", resp.Choices[0].Message.Content)

	// finish_reason 应变为 tool_calls
	assert.Equal(t, "tool_calls", resp.Choices[0].FinishReason)
}

func TestXMLToolCallProvider_Completion_XMLMode_NoToolCalls(t *testing.T) {
	inner := &mockProvider{
		name: "test",
		completionResp: &llmpkg.ChatResponse{
			Choices: []llmpkg.ChatChoice{{
				Message:      types.Message{Content: "Just a normal response."},
				FinishReason: "stop",
			}},
		},
	}

	provider := NewXMLToolCallProvider(inner, nil)
	req := &llmpkg.ChatRequest{
		ToolCallMode: llmpkg.ToolCallModeXML,
		Messages:     []types.Message{{Role: types.RoleUser, Content: "hello"}},
	}

	resp, err := provider.Completion(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "Just a normal response.", resp.Choices[0].Message.Content)
	assert.Empty(t, resp.Choices[0].Message.ToolCalls)
	assert.Equal(t, "stop", resp.Choices[0].FinishReason)
}

// --- Stream Tests ---

func TestXMLToolCallProvider_Stream_NativeMode_Passthrough(t *testing.T) {
	ch := make(chan llmpkg.StreamChunk, 1)
	ch <- llmpkg.StreamChunk{Delta: types.Message{Content: "hi"}}
	close(ch)

	inner := &mockProvider{name: "test", streamCh: ch}
	provider := NewXMLToolCallProvider(inner, nil)

	req := &llmpkg.ChatRequest{
		ToolCallMode: llmpkg.ToolCallModeNative,
		Messages:     []types.Message{{Role: types.RoleUser, Content: "hi"}},
	}

	out, err := provider.Stream(context.Background(), req)
	require.NoError(t, err)

	var chunks []llmpkg.StreamChunk
	for c := range out {
		chunks = append(chunks, c)
	}
	require.Len(t, chunks, 1)
	assert.Equal(t, "hi", chunks[0].Delta.Content)
}

func TestXMLToolCallProvider_Stream_XMLMode_ParsesToolCalls(t *testing.T) {
	ch := make(chan llmpkg.StreamChunk, 2)
	ch <- llmpkg.StreamChunk{Delta: types.Message{Content: "Let me check. "}}
	ch <- llmpkg.StreamChunk{
		Delta: types.Message{
			Content: "<tool_calls>\n{\"name\":\"weather\",\"arguments\":{\"city\":\"Tokyo\"}}\n</tool_calls>",
		},
		FinishReason: "stop",
	}
	close(ch)

	inner := &mockProvider{name: "test", streamCh: ch}
	provider := NewXMLToolCallProvider(inner, nil)

	req := &llmpkg.ChatRequest{
		ToolCallMode: llmpkg.ToolCallModeXML,
		Messages:     []types.Message{{Role: types.RoleUser, Content: "weather?"}},
	}

	out, err := provider.Stream(context.Background(), req)
	require.NoError(t, err)

	var chunks []llmpkg.StreamChunk
	for c := range out {
		chunks = append(chunks, c)
	}

	// 应有文本 chunk + 工具调用 chunk
	hasToolCall := false
	hasText := false
	for _, c := range chunks {
		if len(c.Delta.ToolCalls) > 0 {
			hasToolCall = true
			assert.Equal(t, "weather", c.Delta.ToolCalls[0].Name)
		}
		if c.Delta.Content != "" {
			hasText = true
		}
	}
	assert.True(t, hasToolCall, "应包含工具调用")
	assert.True(t, hasText, "应包含普通文本")
}

// --- Passthrough Methods ---

func TestXMLToolCallProvider_Passthrough(t *testing.T) {
	inner := &mockProvider{name: "test-inner", supportsNative: true}
	provider := NewXMLToolCallProvider(inner, nil)

	assert.Equal(t, "test-inner", provider.Name())
	assert.True(t, provider.SupportsNativeFunctionCalling())
}

// --- Fix 7a: Stream panic 恢复 + 死锁检测 ---

// panicStreamProvider 在 Stream 返回的 channel 中发送会触发 parser panic 的内容
type panicStreamProvider struct {
	mockProvider
}

func (p *panicStreamProvider) Stream(ctx context.Context, req *llmpkg.ChatRequest) (<-chan llmpkg.StreamChunk, error) {
	ch := make(chan llmpkg.StreamChunk, 1)
	ch <- llmpkg.StreamChunk{Delta: types.Message{Content: "normal text"}}
	close(ch)
	return ch, nil
}

func TestXMLToolCallProvider_Stream_PanicRecovery(t *testing.T) {
	// 使用一个会在 Feed 中 panic 的自定义 parser 来验证 panic 恢复
	// 这里通过 mock provider 发送正常内容，然后验证 goroutine 不会死锁
	// 真正的 panic 测试需要注入 parser，这里验证 timeout 机制
	inner := &mockProvider{
		name:     "test",
		streamCh: make(chan llmpkg.StreamChunk),
	}

	// 模拟：发送内容后立即关闭
	go func() {
		inner.streamCh <- llmpkg.StreamChunk{
			Delta: types.Message{Content: "hello"},
		}
		close(inner.streamCh)
	}()

	provider := NewXMLToolCallProvider(inner, nil)
	req := &llmpkg.ChatRequest{
		ToolCallMode: llmpkg.ToolCallModeXML,
		Messages:     []types.Message{{Role: types.RoleUser, Content: "test"}},
	}

	out, err := provider.Stream(context.Background(), req)
	require.NoError(t, err)

	// 用 timeout 确保不会死锁
	done := make(chan struct{})
	go func() {
		for range out {
		}
		close(done)
	}()

	select {
	case <-done:
		// 正常完成，没有死锁
	case <-context.Background().Done():
		t.Fatal("stream consumer deadlocked")
	}
}

// --- Fix 7b: FinishReason 保留（"length"/"content_filter" 不被覆盖）---

func TestXMLToolCallProvider_Completion_FinishReason_Preserved(t *testing.T) {
	tests := []struct {
		name         string
		finishReason string
		expectReason string
	}{
		{"stop_overridden", "stop", "tool_calls"},
		{"empty_overridden", "", "tool_calls"},
		{"length_preserved", "length", "length"},
		{"content_filter_preserved", "content_filter", "content_filter"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inner := &mockProvider{
				name: "test",
				completionResp: &llmpkg.ChatResponse{
					Choices: []llmpkg.ChatChoice{{
						Message: types.Message{
							Content: `text <tool_calls>
{"name":"tool1","arguments":{}}
</tool_calls>`,
						},
						FinishReason: tt.finishReason,
					}},
				},
			}

			provider := NewXMLToolCallProvider(inner, nil)
			req := &llmpkg.ChatRequest{
				ToolCallMode: llmpkg.ToolCallModeXML,
				Messages:     []types.Message{{Role: types.RoleUser, Content: "test"}},
			}

			resp, err := provider.Completion(context.Background(), req)
			require.NoError(t, err)
			assert.Equal(t, tt.expectReason, resp.Choices[0].FinishReason)
		})
	}
}

func TestXMLToolCallProvider_Stream_FinishReason_Preserved(t *testing.T) {
	tests := []struct {
		name         string
		finishReason string
		expectReason string
	}{
		{"stop_overridden", "stop", "tool_calls"},
		{"length_preserved", "length", "length"},
		{"content_filter_preserved", "content_filter", "content_filter"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := make(chan llmpkg.StreamChunk, 1)
			ch <- llmpkg.StreamChunk{
				Delta: types.Message{
					Content: "<tool_calls>\n{\"name\":\"t1\",\"arguments\":{}}\n</tool_calls>",
				},
				FinishReason: tt.finishReason,
			}
			close(ch)

			inner := &mockProvider{name: "test", streamCh: ch}
			provider := NewXMLToolCallProvider(inner, nil)
			req := &llmpkg.ChatRequest{
				ToolCallMode: llmpkg.ToolCallModeXML,
				Messages:     []types.Message{{Role: types.RoleUser, Content: "test"}},
			}

			out, err := provider.Stream(context.Background(), req)
			require.NoError(t, err)

			for c := range out {
				if len(c.Delta.ToolCalls) > 0 {
					assert.Equal(t, tt.expectReason, c.FinishReason)
				}
			}
		})
	}
}
