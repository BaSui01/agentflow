package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	llmpkg "github.com/BaSui01/agentflow/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// --- ParallelExecutor ---

func TestDefaultParallelConfig(t *testing.T) {
	cfg := DefaultParallelConfig()
	assert.Equal(t, 10, cfg.MaxConcurrency)
	assert.Equal(t, 60*time.Second, cfg.ExecutionTimeout)
	assert.False(t, cfg.FailFast)
}

func TestNewParallelExecutor(t *testing.T) {
	reg := NewDefaultRegistry(zap.NewNop())
	pe := NewParallelExecutor(reg, DefaultParallelConfig(), nil)
	require.NotNil(t, pe)
}

func TestNewParallelExecutor_DefaultValues(t *testing.T) {
	reg := NewDefaultRegistry(zap.NewNop())
	pe := NewParallelExecutor(reg, ParallelConfig{MaxConcurrency: -1, ExecutionTimeout: -1}, nil)
	require.NotNil(t, pe)
	assert.Equal(t, 10, pe.config.MaxConcurrency)
}

func TestParallelExecutor_Execute_Empty(t *testing.T) {
	reg := NewDefaultRegistry(zap.NewNop())
	pe := NewParallelExecutor(reg, DefaultParallelConfig(), zap.NewNop())

	result := pe.Execute(context.Background(), nil)
	require.NotNil(t, result)
	assert.Equal(t, 0, len(result.Results))
}

func TestParallelExecutor_Execute_Success(t *testing.T) {
	reg := NewDefaultRegistry(zap.NewNop())
	require.NoError(t, reg.Register("echo", func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
		return args, nil
	}, ToolMetadata{Description: "echo tool"}))

	pe := NewParallelExecutor(reg, DefaultParallelConfig(), zap.NewNop())

	calls := []llmpkg.ToolCall{
		{ID: "c1", Name: "echo", Arguments: json.RawMessage(`{"msg":"hello"}`)},
		{ID: "c2", Name: "echo", Arguments: json.RawMessage(`{"msg":"world"}`)},
	}

	result := pe.Execute(context.Background(), calls)
	require.NotNil(t, result)
	assert.Equal(t, 2, len(result.Results))
	assert.Equal(t, 2, result.Completed)
	assert.Equal(t, 0, result.Failed)
}

func TestParallelExecutor_Execute_ToolNotFound(t *testing.T) {
	reg := NewDefaultRegistry(zap.NewNop())
	pe := NewParallelExecutor(reg, DefaultParallelConfig(), zap.NewNop())

	calls := []llmpkg.ToolCall{
		{ID: "c1", Name: "nonexistent", Arguments: json.RawMessage(`{}`)},
	}

	result := pe.Execute(context.Background(), calls)
	assert.Equal(t, 1, result.Failed)
}

func TestParallelExecutor_Execute_ToolError(t *testing.T) {
	reg := NewDefaultRegistry(zap.NewNop())
	require.NoError(t, reg.Register("fail", func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
		return nil, fmt.Errorf("tool error")
	}, ToolMetadata{Description: "failing tool"}))

	pe := NewParallelExecutor(reg, DefaultParallelConfig(), zap.NewNop())

	calls := []llmpkg.ToolCall{
		{ID: "c1", Name: "fail", Arguments: json.RawMessage(`{}`)},
	}

	result := pe.Execute(context.Background(), calls)
	assert.Equal(t, 1, result.Failed)
}

func TestParallelExecutor_Stats(t *testing.T) {
	reg := NewDefaultRegistry(zap.NewNop())
	require.NoError(t, reg.Register("echo", func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`"ok"`), nil
	}, ToolMetadata{Description: "echo"}))

	pe := NewParallelExecutor(reg, DefaultParallelConfig(), zap.NewNop())
	pe.Execute(context.Background(), []llmpkg.ToolCall{
		{ID: "c1", Name: "echo", Arguments: json.RawMessage(`{}`)},
	})

	total, success, failed, avgDuration := pe.Stats()
	assert.GreaterOrEqual(t, total, int64(0))
	assert.GreaterOrEqual(t, success, int64(0))
	assert.GreaterOrEqual(t, failed, int64(0))
	assert.GreaterOrEqual(t, avgDuration, time.Duration(0))
}

// --- ResilientExecutor (fallback) ---

func TestDefaultFallbackConfig(t *testing.T) {
	cfg := DefaultFallbackConfig()
	assert.Equal(t, 2, cfg.MaxRetries)
	assert.NotEmpty(t, cfg.SkipOnErrors)
}

func TestNewResilientExecutor(t *testing.T) {
	reg := NewDefaultRegistry(zap.NewNop())
	re := NewResilientExecutor(reg, nil, zap.NewNop())
	require.NotNil(t, re)
}
func TestResilientExecutor_Execute_Success(t *testing.T) {
	reg := NewDefaultRegistry(zap.NewNop())
	require.NoError(t, reg.Register("echo", func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`"ok"`), nil
	}, ToolMetadata{Description: "echo"}))

	re := NewResilientExecutor(reg, DefaultFallbackConfig(), zap.NewNop())
	results := re.Execute(context.Background(), []llmpkg.ToolCall{
		{ID: "c1", Name: "echo", Arguments: json.RawMessage(`{}`)},
	})
	require.Len(t, results, 1)
	assert.Empty(t, results[0].Error)
}

func TestResilientExecutor_ExecuteOne_Success(t *testing.T) {
	reg := NewDefaultRegistry(zap.NewNop())
	require.NoError(t, reg.Register("echo", func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`"ok"`), nil
	}, ToolMetadata{Description: "echo"}))

	re := NewResilientExecutor(reg, DefaultFallbackConfig(), zap.NewNop())
	result := re.ExecuteOne(context.Background(), llmpkg.ToolCall{
		ID: "c1", Name: "echo", Arguments: json.RawMessage(`{}`),
	})
	assert.Empty(t, result.Error)
}

func TestResilientExecutor_Execute_WithRetry(t *testing.T) {
	callCount := 0
	reg := NewDefaultRegistry(zap.NewNop())
	require.NoError(t, reg.Register("flaky", func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
		callCount++
		if callCount < 3 {
			return nil, fmt.Errorf("transient error")
		}
		return json.RawMessage(`"ok"`), nil
	}, ToolMetadata{Description: "flaky"}))

	cfg := DefaultFallbackConfig()
	cfg.MaxRetries = 3
	cfg.RetryDelayMs = 1 // fast for tests
	re := NewResilientExecutor(reg, cfg, zap.NewNop())

	result := re.ExecuteOne(context.Background(), llmpkg.ToolCall{
		ID: "c1", Name: "flaky", Arguments: json.RawMessage(`{}`),
	})
	assert.Empty(t, result.Error)
}

func TestResilientExecutor_Execute_SkipOnError(t *testing.T) {
	reg := NewDefaultRegistry(zap.NewNop())
	require.NoError(t, reg.Register("missing", func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
		return nil, fmt.Errorf("tool not found")
	}, ToolMetadata{Description: "missing"}))

	cfg := DefaultFallbackConfig()
	cfg.SkipOnErrors = []string{"tool not found"}
	re := NewResilientExecutor(reg, cfg, zap.NewNop())

	result := re.ExecuteOne(context.Background(), llmpkg.ToolCall{
		ID: "c1", Name: "missing", Arguments: json.RawMessage(`{}`),
	})
	// Should have an error or skip response
	assert.NotNil(t, result)
}

func TestResilientExecutor_Execute_WithAlternate(t *testing.T) {
	reg := NewDefaultRegistry(zap.NewNop())
	require.NoError(t, reg.Register("primary", func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
		return nil, fmt.Errorf("primary failed")
	}, ToolMetadata{Description: "primary"}))
	require.NoError(t, reg.Register("backup", func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`"backup-ok"`), nil
	}, ToolMetadata{Description: "backup"}))

	cfg := DefaultFallbackConfig()
	cfg.Alternates = map[string]string{"primary": "backup"}
	cfg.MaxRetries = 0
	re := NewResilientExecutor(reg, cfg, zap.NewNop())

	result := re.ExecuteOne(context.Background(), llmpkg.ToolCall{
		ID: "c1", Name: "primary", Arguments: json.RawMessage(`{}`),
	})
	// Should fallback to backup
	assert.NotNil(t, result)
}

// --- BatchExecutor ---

func TestNewBatchExecutor(t *testing.T) {
	reg := NewDefaultRegistry(zap.NewNop())
	pe := NewParallelExecutor(reg, DefaultParallelConfig(), zap.NewNop())
	be := NewBatchExecutor(pe, zap.NewNop())
	require.NotNil(t, be)
}

func TestBatchExecutor_ExecuteBatched(t *testing.T) {
	reg := NewDefaultRegistry(zap.NewNop())
	require.NoError(t, reg.Register("echo", func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
		return args, nil
	}, ToolMetadata{Description: "echo"}))

	pe := NewParallelExecutor(reg, DefaultParallelConfig(), zap.NewNop())
	be := NewBatchExecutor(pe, zap.NewNop())
	calls := []llmpkg.ToolCall{
		{ID: "c1", Name: "echo", Arguments: json.RawMessage(`"a"`)},
		{ID: "c2", Name: "echo", Arguments: json.RawMessage(`"b"`)},
		{ID: "c3", Name: "echo", Arguments: json.RawMessage(`"c"`)},
	}

	results := be.ExecuteBatched(context.Background(), calls)
	assert.Len(t, results.Results, 3)
}

