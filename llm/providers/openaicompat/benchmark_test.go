package openaicompat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"

	providerbase "github.com/BaSui01/agentflow/llm/providers/base"
	"github.com/BaSui01/agentflow/types"
)

// BenchmarkStreamSSE_Parse measures SSE stream parsing throughput.
func BenchmarkStreamSSE_Parse(b *testing.B) {
	// Build a realistic SSE payload with 100 lines.
	var sb strings.Builder
	for i := 0; i < 100; i++ {
		chunk := fmt.Sprintf(
			`{"id":"chatcmpl-%d","model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant","content":"token%d"},"finish_reason":null}]}`,
			i, i,
		)
		sb.WriteString("data: ")
		sb.WriteString(chunk)
		sb.WriteString("\n\n")
	}
	sb.WriteString("data: [DONE]\n\n")
	payload := sb.String()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		reader := io.NopCloser(strings.NewReader(payload))
		ctx := context.Background()
		ch := StreamSSE(ctx, reader, "bench")
		for range ch {
			// drain
		}
	}
}

// BenchmarkUnwrapStringifiedJSON measures double-serialization unwrap performance.
func BenchmarkUnwrapStringifiedJSON(b *testing.B) {
	// Normal JSON object (fast path)
	normalJSON := json.RawMessage(`{"city":"Beijing","temp":25}`)
	// Double-serialized string (slow path)
	doubleJSON := json.RawMessage(`"{\"city\":\"Beijing\",\"temp\":25}"`)

	b.Run("NormalJSON", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = providerbase.UnwrapStringifiedJSON(normalJSON)
		}
	})

	b.Run("DoubleSerializedJSON", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = providerbase.UnwrapStringifiedJSON(doubleJSON)
		}
	})
}

// BenchmarkConvertMessagesToOpenAI measures message conversion performance with 10 messages.
func BenchmarkConvertMessagesToOpenAI(b *testing.B) {
	msgs := make([]types.Message, 10)
	for i := 0; i < 10; i++ {
		role := types.Role("user")
		if i%2 == 1 {
			role = types.Role("assistant")
		}
		msgs[i] = types.Message{
			Role:    role,
			Content: fmt.Sprintf("This is message number %d with some realistic content for benchmarking.", i),
		}
		if i%3 == 0 {
			msgs[i].ToolCalls = []types.ToolCall{
				{
					ID:        fmt.Sprintf("call_%d", i),
					Name:      "get_weather",
					Arguments: json.RawMessage(`{"city":"Beijing"}`),
				},
			}
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = providerbase.ConvertMessagesToOpenAI(msgs)
	}
}
