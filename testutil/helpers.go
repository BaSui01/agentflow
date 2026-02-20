// testutil 通用测试辅助与断言工具。
//
// 提供上下文构造、消息断言及异步等待断言等能力。
package testutil

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/types"
)

// --- 上下文辅助 ---

// TestContext 返回带超时的测试上下文
func TestContext(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// TestContextWithTimeout 返回带自定义超时的测试上下文
func TestContextWithTimeout(t *testing.T, timeout time.Duration) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(cancel)
	return ctx
}

// CancelledContext 返回已取消的上下文
func CancelledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

// --- 断言辅助 ---

// AssertMessagesEqual 断言两个消息切片相等
func AssertMessagesEqual(t *testing.T, expected, actual []types.Message) {
	t.Helper()

	if len(expected) != len(actual) {
		t.Errorf("message count mismatch: expected %d, got %d", len(expected), len(actual))
		return
	}

	for i := range expected {
		if expected[i].Role != actual[i].Role {
			t.Errorf("message[%d] role mismatch: expected %q, got %q", i, expected[i].Role, actual[i].Role)
		}
		if expected[i].Content != actual[i].Content {
			t.Errorf("message[%d] content mismatch: expected %q, got %q", i, expected[i].Content, actual[i].Content)
		}
	}
}

// AssertToolCallsEqual 断言两个工具调用切片相等
func AssertToolCallsEqual(t *testing.T, expected, actual []types.ToolCall) {
	t.Helper()

	if len(expected) != len(actual) {
		t.Errorf("tool call count mismatch: expected %d, got %d", len(expected), len(actual))
		return
	}

	for i := range expected {
		if expected[i].Name != actual[i].Name {
			t.Errorf("tool call[%d] name mismatch: expected %q, got %q", i, expected[i].Name, actual[i].Name)
		}
		if string(expected[i].Arguments) != string(actual[i].Arguments) {
			t.Errorf("tool call[%d] arguments mismatch: expected %s, got %s", i, expected[i].Arguments, actual[i].Arguments)
		}
	}
}

// AssertJSONEqual 断言两个值的 JSON 表示相等
func AssertJSONEqual(t *testing.T, expected, actual any) {
	t.Helper()

	expectedJSON, err := json.Marshal(expected)
	if err != nil {
		t.Fatalf("failed to marshal expected: %v", err)
	}

	actualJSON, err := json.Marshal(actual)
	if err != nil {
		t.Fatalf("failed to marshal actual: %v", err)
	}

	if string(expectedJSON) != string(actualJSON) {
		t.Errorf("JSON mismatch:\nexpected: %s\nactual: %s", expectedJSON, actualJSON)
	}
}

// AssertEventuallyTrue 断言条件最终为真
func AssertEventuallyTrue(t *testing.T, condition func() bool, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Errorf("condition did not become true within %v", timeout)
}

// AssertEventuallyEqual 断言值最终相等
func AssertEventuallyEqual(t *testing.T, expected any, getter func() any, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	var lastValue any

	for time.Now().Before(deadline) {
		lastValue = getter()
		if reflect.DeepEqual(expected, lastValue) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Errorf("value did not become %v within %v, last value: %v", expected, timeout, lastValue)
}

// AssertNoError 断言没有错误
func AssertNoError(t *testing.T, err error, msgAndArgs ...any) {
	t.Helper()
	if err != nil {
		if len(msgAndArgs) > 0 {
			t.Errorf("%v: unexpected error: %v", msgAndArgs[0], err)
		} else {
			t.Errorf("unexpected error: %v", err)
		}
	}
}

// AssertError 断言有错误
func AssertError(t *testing.T, err error, msgAndArgs ...any) {
	t.Helper()
	if err == nil {
		if len(msgAndArgs) > 0 {
			t.Errorf("%v: expected error but got nil", msgAndArgs[0])
		} else {
			t.Error("expected error but got nil")
		}
	}
}

// AssertContains 断言字符串包含子串
func AssertContains(t *testing.T, s, substr string) {
	t.Helper()
	if !contains(s, substr) {
		t.Errorf("expected %q to contain %q", s, substr)
	}
}

// AssertNotContains 断言字符串不包含子串
func AssertNotContains(t *testing.T, s, substr string) {
	t.Helper()
	if contains(s, substr) {
		t.Errorf("expected %q to not contain %q", s, substr)
	}
}

func contains(s, substr string) bool {
	return len(substr) == 0 || (len(s) >= len(substr) && searchSubstring(s, substr))
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// --- 时间辅助 ---

// WaitFor 等待条件满足或超时
func WaitFor(condition func() bool, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

// WaitForChannel 等待通道接收或超时
func WaitForChannel[T any](ch <-chan T, timeout time.Duration) (T, bool) {
	select {
	case v := <-ch:
		return v, true
	case <-time.After(timeout):
		var zero T
		return zero, false
	}
}

// --- 测试数据辅助 ---

// MustJSON 将值转换为 JSON 字符串，失败时 panic
func MustJSON(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(data)
}

// MustParseJSON 解析 JSON 字符串，失败时 panic
func MustParseJSON[T any](s string) T {
	var v T
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		panic(err)
	}
	return v
}

// CopyMessages 深拷贝消息切片
func CopyMessages(messages []types.Message) []types.Message {
	if messages == nil {
		return nil
	}
	copied := make([]types.Message, len(messages))
	copy(copied, messages)
	return copied
}

// CopyToolCalls 深拷贝工具调用切片
func CopyToolCalls(toolCalls []types.ToolCall) []types.ToolCall {
	if toolCalls == nil {
		return nil
	}
	copied := make([]types.ToolCall, len(toolCalls))
	for i, tc := range toolCalls {
		argsCopy := make(json.RawMessage, len(tc.Arguments))
		copy(argsCopy, tc.Arguments)
		copied[i] = types.ToolCall{
			ID:        tc.ID,
			Name:      tc.Name,
			Arguments: argsCopy,
		}
	}
	return copied
}

// --- Mock 辅助 ---

// CollectStreamChunks 收集流式块到切片
func CollectStreamChunks(ch <-chan llm.StreamChunk) []llm.StreamChunk {
	var chunks []llm.StreamChunk
	for chunk := range ch {
		chunks = append(chunks, chunk)
	}
	return chunks
}

// CollectStreamContent 收集流式内容到字符串
func CollectStreamContent(ch <-chan llm.StreamChunk) string {
	var content string
	for chunk := range ch {
		content += chunk.Delta.Content
	}
	return content
}

// SendChunksToChannel 发送块到通道
func SendChunksToChannel(chunks []llm.StreamChunk) <-chan llm.StreamChunk {
	ch := make(chan llm.StreamChunk, len(chunks))
	go func() {
		defer close(ch)
		for _, chunk := range chunks {
			ch <- chunk
		}
	}()
	return ch
}

// --- 基准测试辅助 ---

// BenchmarkHelper 基准测试辅助结构
type BenchmarkHelper struct {
	b *testing.B
}

// NewBenchmarkHelper 创建基准测试辅助
func NewBenchmarkHelper(b *testing.B) *BenchmarkHelper {
	return &BenchmarkHelper{b: b}
}

// ResetTimer 重置计时器
func (h *BenchmarkHelper) ResetTimer() {
	h.b.ResetTimer()
}

// StopTimer 停止计时器
func (h *BenchmarkHelper) StopTimer() {
	h.b.StopTimer()
}

// StartTimer 启动计时器
func (h *BenchmarkHelper) StartTimer() {
	h.b.StartTimer()
}

// ReportAllocs 报告内存分配
func (h *BenchmarkHelper) ReportAllocs() {
	h.b.ReportAllocs()
}

// RunParallel 并行运行基准测试
func (h *BenchmarkHelper) RunParallel(body func()) {
	h.b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			body()
		}
	})
}
