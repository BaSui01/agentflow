package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// BenchmarkToolExecutor_ExecuteOne measures single tool execution performance.
func BenchmarkToolExecutor_ExecuteOne(b *testing.B) {
	logger := zap.NewNop()
	registry := NewDefaultRegistry(logger)

	// Register a lightweight tool
	_ = registry.Register("echo", func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
		return args, nil
	}, ToolMetadata{
		Schema: types.ToolSchema{
			Name:        "echo",
			Description: "echoes input",
		},
	})

	executor := NewDefaultExecutor(registry, logger)

	call := types.ToolCall{
		ID:        "call_bench_1",
		Name:      "echo",
		Arguments: json.RawMessage(`{"msg":"hello"}`),
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = executor.ExecuteOne(context.Background(), call)
	}
}

// BenchmarkToolRegistry_List measures tool listing performance.
func BenchmarkToolRegistry_List(b *testing.B) {
	logger := zap.NewNop()
	registry := NewDefaultRegistry(logger)

	// Register 20 tools to simulate a realistic registry
	for i := 0; i < 20; i++ {
		name := "tool_" + string(rune('a'+i))
		_ = registry.Register(name, func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
			return nil, nil
		}, ToolMetadata{
			Schema: types.ToolSchema{
				Name:        name,
				Description: "benchmark tool",
				Parameters:  json.RawMessage(`{"type":"object"}`),
			},
		})
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = registry.List()
	}
}
