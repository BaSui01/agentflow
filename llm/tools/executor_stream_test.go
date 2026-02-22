package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	llmpkg "github.com/BaSui01/agentflow/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ====== F1: ExecuteOneStream tests ======

func TestDefaultExecutor_ExecuteOneStream_Success(t *testing.T) {
	logger := zap.NewNop()
	registry := NewDefaultRegistry(logger)

	echoFunc := func(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
		return args, nil
	}
	require.NoError(t, registry.Register("echo", echoFunc, ToolMetadata{
		Schema:  llmpkg.ToolSchema{Name: "echo"},
		Timeout: 5 * time.Second,
	}))

	executor := NewDefaultExecutor(registry, logger)

	call := llmpkg.ToolCall{
		ID:        "call_1",
		Name:      "echo",
		Arguments: json.RawMessage(`{"msg":"hello"}`),
	}

	ch := executor.ExecuteOneStream(context.Background(), call)

	var events []ToolStreamEvent
	for ev := range ch {
		events = append(events, ev)
	}

	require.Len(t, events, 3, "expected progress + output + complete")
	assert.Equal(t, ToolStreamProgress, events[0].Type)
	assert.Equal(t, "echo", events[0].ToolName)
	assert.Equal(t, ToolStreamOutput, events[1].Type)
	assert.Equal(t, ToolStreamComplete, events[2].Type)

	// complete event carries the full ToolResult
	result, ok := events[2].Data.(ToolResult)
	require.True(t, ok)
	assert.Equal(t, "echo", result.Name)
	assert.Empty(t, result.Error)
}

func TestDefaultExecutor_ExecuteOneStream_ToolNotFound(t *testing.T) {
	logger := zap.NewNop()
	registry := NewDefaultRegistry(logger)
	executor := NewDefaultExecutor(registry, logger)

	call := llmpkg.ToolCall{ID: "call_1", Name: "nonexistent"}
	ch := executor.ExecuteOneStream(context.Background(), call)

	var events []ToolStreamEvent
	for ev := range ch {
		events = append(events, ev)
	}

	// progress + error
	require.Len(t, events, 2)
	assert.Equal(t, ToolStreamProgress, events[0].Type)
	assert.Equal(t, ToolStreamError, events[1].Type)
	assert.Error(t, events[1].Error)
}

func TestDefaultExecutor_ExecuteOneStream_ContextCancelled(t *testing.T) {
	logger := zap.NewNop()
	registry := NewDefaultRegistry(logger)

	slowFunc := func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(5 * time.Second):
			return json.RawMessage(`{"done":true}`), nil
		}
	}
	require.NoError(t, registry.Register("slow", slowFunc, ToolMetadata{
		Schema:  llmpkg.ToolSchema{Name: "slow"},
		Timeout: 10 * time.Second,
	}))

	executor := NewDefaultExecutor(registry, logger)

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel immediately so the tool execution sees a cancelled context
	cancel()

	call := llmpkg.ToolCall{ID: "call_1", Name: "slow"}
	ch := executor.ExecuteOneStream(ctx, call)

	var hasError bool
	for ev := range ch {
		if ev.Type == ToolStreamError {
			hasError = true
		}
	}
	assert.True(t, hasError, "expected an error event from cancelled context")
}

func TestDefaultExecutor_ImplementsStreamableToolExecutor(t *testing.T) {
	// Compile-time check is in executor.go, but verify at test level too
	var _ StreamableToolExecutor = (*DefaultExecutor)(nil)
}

// ====== F2: Retry tests ======

func TestDefaultExecutor_ExecuteWithRetry_SucceedsAfterRetries(t *testing.T) {
	logger := zap.NewNop()
	registry := NewDefaultRegistry(logger)

	var callCount atomic.Int32
	flakyFunc := func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
		n := callCount.Add(1)
		if n < 3 {
			return nil, fmt.Errorf("transient error")
		}
		return json.RawMessage(`{"ok":true}`), nil
	}
	require.NoError(t, registry.Register("flaky", flakyFunc, ToolMetadata{
		Schema:  llmpkg.ToolSchema{Name: "flaky"},
		Timeout: 5 * time.Second,
	}))

	executor := NewDefaultExecutorWithConfig(registry, logger, ExecutorConfig{
		MaxRetries:   3,
		RetryDelay:   10 * time.Millisecond,
		RetryBackoff: 1.5,
	})

	call := llmpkg.ToolCall{ID: "call_1", Name: "flaky", Arguments: json.RawMessage(`{}`)}
	result := executor.Execute(context.Background(), []llmpkg.ToolCall{call})

	require.Len(t, result, 1)
	assert.Empty(t, result[0].Error, "should succeed after retries")
	assert.Equal(t, int32(3), callCount.Load(), "expected 3 total calls (1 initial + 2 retries)")
}

func TestDefaultExecutor_ExecuteWithRetry_ExhaustsRetries(t *testing.T) {
	logger := zap.NewNop()
	registry := NewDefaultRegistry(logger)

	alwaysFailFunc := func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
		return nil, fmt.Errorf("permanent error")
	}
	require.NoError(t, registry.Register("fail", alwaysFailFunc, ToolMetadata{
		Schema:  llmpkg.ToolSchema{Name: "fail"},
		Timeout: 5 * time.Second,
	}))

	executor := NewDefaultExecutorWithConfig(registry, logger, ExecutorConfig{
		MaxRetries:   2,
		RetryDelay:   5 * time.Millisecond,
		RetryBackoff: 1.0,
	})

	call := llmpkg.ToolCall{ID: "call_1", Name: "fail", Arguments: json.RawMessage(`{}`)}
	result := executor.Execute(context.Background(), []llmpkg.ToolCall{call})

	require.Len(t, result, 1)
	assert.Contains(t, result[0].Error, "permanent error")
}

func TestDefaultExecutor_ExecuteWithRetry_NoRetryOnZeroConfig(t *testing.T) {
	logger := zap.NewNop()
	registry := NewDefaultRegistry(logger)

	var callCount atomic.Int32
	failFunc := func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
		callCount.Add(1)
		return nil, fmt.Errorf("fail")
	}
	require.NoError(t, registry.Register("once", failFunc, ToolMetadata{
		Schema:  llmpkg.ToolSchema{Name: "once"},
		Timeout: 5 * time.Second,
	}))

	// Default executor has MaxRetries=0
	executor := NewDefaultExecutor(registry, logger)

	call := llmpkg.ToolCall{ID: "call_1", Name: "once", Arguments: json.RawMessage(`{}`)}
	result := executor.Execute(context.Background(), []llmpkg.ToolCall{call})

	require.Len(t, result, 1)
	assert.Contains(t, result[0].Error, "fail")
	assert.Equal(t, int32(1), callCount.Load(), "should not retry with default config")
}

func TestDefaultExecutor_ParallelExecutionWithRetry_IndependentTools(t *testing.T) {
	logger := zap.NewNop()
	registry := NewDefaultRegistry(logger)

	// Tool A: always succeeds
	require.NoError(t, registry.Register("fast", func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{"fast":true}`), nil
	}, ToolMetadata{
		Schema:  llmpkg.ToolSchema{Name: "fast"},
		Timeout: 5 * time.Second,
	}))

	// Tool B: fails then succeeds
	var slowCount atomic.Int32
	require.NoError(t, registry.Register("flaky2", func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
		n := slowCount.Add(1)
		if n < 2 {
			return nil, fmt.Errorf("transient")
		}
		return json.RawMessage(`{"flaky2":true}`), nil
	}, ToolMetadata{
		Schema:  llmpkg.ToolSchema{Name: "flaky2"},
		Timeout: 5 * time.Second,
	}))

	executor := NewDefaultExecutorWithConfig(registry, logger, ExecutorConfig{
		MaxRetries:   2,
		RetryDelay:   5 * time.Millisecond,
		RetryBackoff: 1.0,
	})

	calls := []llmpkg.ToolCall{
		{ID: "c1", Name: "fast", Arguments: json.RawMessage(`{}`)},
		{ID: "c2", Name: "flaky2", Arguments: json.RawMessage(`{}`)},
	}
	results := executor.Execute(context.Background(), calls)

	require.Len(t, results, 2)
	assert.Empty(t, results[0].Error, "fast tool should succeed")
	assert.Empty(t, results[1].Error, "flaky2 tool should succeed after retry")
}

func TestDefaultExecutor_ExecuteOneStream_WithRetry(t *testing.T) {
	logger := zap.NewNop()
	registry := NewDefaultRegistry(logger)

	var callCount atomic.Int32
	flakyFunc := func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
		n := callCount.Add(1)
		if n < 2 {
			return nil, fmt.Errorf("transient")
		}
		return json.RawMessage(`{"ok":true}`), nil
	}
	require.NoError(t, registry.Register("flaky_stream", flakyFunc, ToolMetadata{
		Schema:  llmpkg.ToolSchema{Name: "flaky_stream"},
		Timeout: 5 * time.Second,
	}))

	executor := NewDefaultExecutorWithConfig(registry, logger, ExecutorConfig{
		MaxRetries:   2,
		RetryDelay:   5 * time.Millisecond,
		RetryBackoff: 1.0,
	})

	call := llmpkg.ToolCall{ID: "call_1", Name: "flaky_stream", Arguments: json.RawMessage(`{}`)}
	ch := executor.ExecuteOneStream(context.Background(), call)

	var events []ToolStreamEvent
	for ev := range ch {
		events = append(events, ev)
	}

	// Should succeed: progress + output + complete
	require.Len(t, events, 3)
	assert.Equal(t, ToolStreamProgress, events[0].Type)
	assert.Equal(t, ToolStreamOutput, events[1].Type)
	assert.Equal(t, ToolStreamComplete, events[2].Type)
}

func TestDefaultExecutorConfig_Defaults(t *testing.T) {
	cfg := DefaultExecutorConfig()
	assert.Equal(t, 0, cfg.MaxRetries)
	assert.Equal(t, 100*time.Millisecond, cfg.RetryDelay)
	assert.Equal(t, 2.0, cfg.RetryBackoff)
}

func TestNewDefaultExecutorWithConfig_SanitizesInvalidValues(t *testing.T) {
	logger := zap.NewNop()
	registry := NewDefaultRegistry(logger)

	executor := NewDefaultExecutorWithConfig(registry, logger, ExecutorConfig{
		MaxRetries:   1,
		RetryDelay:   -1, // invalid
		RetryBackoff: 0,  // invalid
	})

	// Should have been corrected to defaults
	assert.Equal(t, 100*time.Millisecond, executor.config.RetryDelay)
	assert.Equal(t, 2.0, executor.config.RetryBackoff)
}
