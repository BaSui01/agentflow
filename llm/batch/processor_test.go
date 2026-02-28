package batch

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/BaSui01/agentflow/testutil"
)

// echoHandler returns a BatchHandler that echoes request IDs as content.
func echoHandler() BatchHandler {
	return func(ctx context.Context, requests []*Request) []*Response {
		responses := make([]*Response, len(requests))
		for i, req := range requests {
			responses[i] = &Response{ID: req.ID, Content: "echo:" + req.ID, Tokens: 1}
		}
		return responses
	}
}

func makeRequest(id string) *Request {
	return &Request{
		ID:       id,
		Model:    "test-model",
		Messages: []Message{{Role: "user", Content: "hello"}},
	}
}

func TestBatchProcessor_NewAndClose(t *testing.T) {
	ctx := testutil.TestContext(t)
	_ = ctx

	cfg := DefaultBatchConfig()
	cfg.Workers = 2

	bp := NewBatchProcessor(cfg, echoHandler())
	require.NotNil(t, bp)
	assert.False(t, bp.closed.Load(), "processor should not be closed after creation")

	bp.Close()
	assert.True(t, bp.closed.Load(), "processor should be closed after Close()")

	// Double close should not panic
	bp.Close()
	assert.True(t, bp.closed.Load())
}

func TestBatchProcessor_Submit(t *testing.T) {
	ctx := testutil.TestContext(t)

	var called atomic.Int32
	handler := func(ctx context.Context, requests []*Request) []*Response {
		called.Add(1)
		responses := make([]*Response, len(requests))
		for i, req := range requests {
			responses[i] = &Response{ID: req.ID, Content: "ok", Tokens: 5}
		}
		return responses
	}

	cfg := DefaultBatchConfig()
	cfg.MaxBatchSize = 1 // process immediately
	cfg.MaxWaitTime = 50 * time.Millisecond
	bp := NewBatchProcessor(cfg, handler)
	t.Cleanup(bp.Close)

	respCh := bp.Submit(ctx, makeRequest("req-1"))
	resp, ok := testutil.WaitForChannel(respCh, 5*time.Second)
	require.True(t, ok, "should receive response")
	assert.Equal(t, "req-1", resp.ID)
	assert.Equal(t, "ok", resp.Content)
	assert.NoError(t, resp.Error)
	assert.Equal(t, 5, resp.Tokens)

	ok = testutil.WaitFor(func() bool { return called.Load() >= 1 }, 5*time.Second)
	assert.True(t, ok, "handler should have been called")
}

func TestBatchProcessor_Submit_AfterClose(t *testing.T) {
	ctx := testutil.TestContext(t)

	bp := NewBatchProcessor(DefaultBatchConfig(), echoHandler())
	bp.Close()

	respCh := bp.Submit(ctx, makeRequest("req-closed"))
	resp, ok := testutil.WaitForChannel(respCh, 2*time.Second)
	require.True(t, ok, "should receive error response")
	assert.Equal(t, ErrBatchClosed, resp.Error)
}

func TestBatchProcessor_SubmitSync(t *testing.T) {
	ctx := testutil.TestContext(t)

	cfg := DefaultBatchConfig()
	cfg.MaxBatchSize = 1
	cfg.MaxWaitTime = 50 * time.Millisecond
	bp := NewBatchProcessor(cfg, echoHandler())
	t.Cleanup(bp.Close)

	resp, err := bp.SubmitSync(ctx, makeRequest("sync-1"))
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "sync-1", resp.ID)
	assert.Equal(t, "echo:sync-1", resp.Content)
}

func TestBatchProcessor_SubmitSync_Timeout(t *testing.T) {
	// Handler that blocks longer than context deadline
	handler := func(ctx context.Context, requests []*Request) []*Response {
		select {
		case <-ctx.Done():
		case <-time.After(10 * time.Second):
		}
		return nil
	}

	cfg := DefaultBatchConfig()
	cfg.MaxBatchSize = 1
	cfg.MaxWaitTime = 10 * time.Millisecond
	bp := NewBatchProcessor(cfg, handler)
	t.Cleanup(bp.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	t.Cleanup(cancel)

	_, err := bp.SubmitSync(ctx, makeRequest("timeout-1"))
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestBatchStats_BatchEfficiency(t *testing.T) {
	tests := []struct {
		name     string
		stats    BatchStats
		expected float64
	}{
		{
			name:     "zero batches returns zero",
			stats:    BatchStats{Batched: 0, Completed: 0, Failed: 0},
			expected: 0,
		},
		{
			name:     "all completed",
			stats:    BatchStats{Batched: 5, Completed: 25, Failed: 0},
			expected: 5.0, // 25/5
		},
		{
			name:     "mixed completed and failed",
			stats:    BatchStats{Batched: 4, Completed: 6, Failed: 2},
			expected: 2.0, // 8/4
		},
		{
			name:     "all failed",
			stats:    BatchStats{Batched: 3, Completed: 0, Failed: 9},
			expected: 3.0, // 9/3
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.stats.BatchEfficiency()
			assert.InDelta(t, tt.expected, got, 0.001)
		})
	}
}

func TestBatchProcessor_Stats(t *testing.T) {
	ctx := testutil.TestContext(t)

	cfg := DefaultBatchConfig()
	cfg.MaxBatchSize = 1
	cfg.MaxWaitTime = 50 * time.Millisecond
	bp := NewBatchProcessor(cfg, echoHandler())
	t.Cleanup(bp.Close)

	// Submit and wait for completion
	resp, err := bp.SubmitSync(ctx, makeRequest("stats-1"))
	require.NoError(t, err)
	require.NotNil(t, resp)

	ok := testutil.WaitFor(func() bool {
		s := bp.Stats()
		return s.Submitted >= 1 && s.Completed >= 1
	}, 5*time.Second)
	require.True(t, ok, "stats should reflect submitted and completed")

	stats := bp.Stats()
	assert.GreaterOrEqual(t, stats.Submitted, int64(1))
	assert.GreaterOrEqual(t, stats.Completed, int64(1))
}

func TestBatchProcessor_Submit_ContextCancel(t *testing.T) {
	// Use a handler that blocks until signalled so the queue stays full
	blocker := make(chan struct{})
	handler := func(ctx context.Context, requests []*Request) []*Response {
		<-blocker
		return nil
	}

	cfg := DefaultBatchConfig()
	cfg.MaxBatchSize = 100 // large batch so timer triggers, not size
	cfg.MaxWaitTime = 10 * time.Second
	cfg.QueueSize = 1 // tiny queue
	cfg.Workers = 1
	bp := NewBatchProcessor(cfg, handler)
	t.Cleanup(func() {
		close(blocker)
		bp.Close()
	})

	// Fill the queue so the next Submit blocks on the channel send
	bp.Submit(context.Background(), makeRequest("fill-1"))

	// Now submit with an already-cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	respCh := bp.Submit(ctx, makeRequest("cancel-1"))
	resp, ok := testutil.WaitForChannel(respCh, 2*time.Second)
	require.True(t, ok, "should receive response on cancelled context")
	require.NotNil(t, resp)
	assert.ErrorIs(t, resp.Error, context.Canceled)
}

func TestBatchProcessor_Submit_QueueFull(t *testing.T) {
	// Handler that blocks forever so queue stays full
	blocker := make(chan struct{})
	handler := func(ctx context.Context, requests []*Request) []*Response {
		<-blocker
		return nil
	}

	cfg := DefaultBatchConfig()
	cfg.MaxBatchSize = 100
	cfg.MaxWaitTime = 10 * time.Second
	cfg.QueueSize = 1
	cfg.Workers = 1
	bp := NewBatchProcessor(cfg, handler)
	t.Cleanup(func() {
		close(blocker)
		bp.Close()
	})

	// Fill the single queue slot
	bp.Submit(context.Background(), makeRequest("fill-1"))

	// Next submit should get ErrBatchFull (queue is full, context not cancelled)
	ctx := testutil.TestContext(t)
	respCh := bp.Submit(ctx, makeRequest("full-1"))
	resp, ok := testutil.WaitForChannel(respCh, 2*time.Second)
	require.True(t, ok, "should receive response")
	assert.Equal(t, ErrBatchFull, resp.Error)
}

func TestBatchProcessor_ProcessBatch_HandlerError(t *testing.T) {
	handler := func(ctx context.Context, requests []*Request) []*Response {
		responses := make([]*Response, len(requests))
		for i, req := range requests {
			responses[i] = &Response{
				ID:    req.ID,
				Error: fmt.Errorf("provider error for %s", req.ID),
			}
		}
		return responses
	}

	cfg := DefaultBatchConfig()
	cfg.MaxBatchSize = 1
	cfg.MaxWaitTime = 50 * time.Millisecond
	bp := NewBatchProcessor(cfg, handler)
	t.Cleanup(bp.Close)

	ctx := testutil.TestContext(t)
	_, err := bp.SubmitSync(ctx, makeRequest("err-1"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider error for err-1")

	ok := testutil.WaitFor(func() bool {
		return bp.Stats().Failed >= 1
	}, 5*time.Second)
	assert.True(t, ok, "failed counter should increment for handler errors")
}

func TestBatchProcessor_ProcessBatch_MissingResponse(t *testing.T) {
	// Handler that returns an empty slice — no response for any request
	handler := func(ctx context.Context, requests []*Request) []*Response {
		return []*Response{}
	}

	cfg := DefaultBatchConfig()
	cfg.MaxBatchSize = 1
	cfg.MaxWaitTime = 50 * time.Millisecond
	bp := NewBatchProcessor(cfg, handler)
	t.Cleanup(bp.Close)

	ctx := testutil.TestContext(t)
	_, err := bp.SubmitSync(ctx, makeRequest("missing-1"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no response for request")

	ok := testutil.WaitFor(func() bool {
		return bp.Stats().Failed >= 1
	}, 5*time.Second)
	assert.True(t, ok, "failed counter should increment for missing responses")
}

func TestBatchProcessor_TimerFlush(t *testing.T) {
	var handlerCalled atomic.Int32
	handler := func(ctx context.Context, requests []*Request) []*Response {
		handlerCalled.Add(1)
		responses := make([]*Response, len(requests))
		for i, req := range requests {
			responses[i] = &Response{ID: req.ID, Content: "timer:" + req.ID, Tokens: 1}
		}
		return responses
	}

	cfg := DefaultBatchConfig()
	cfg.MaxBatchSize = 100 // very large — timer should fire before batch fills
	cfg.MaxWaitTime = 50 * time.Millisecond
	cfg.Workers = 1
	bp := NewBatchProcessor(cfg, handler)
	t.Cleanup(bp.Close)

	ctx := testutil.TestContext(t)
	resp, err := bp.SubmitSync(ctx, makeRequest("timer-1"))
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "timer:timer-1", resp.Content)
	assert.GreaterOrEqual(t, handlerCalled.Load(), int32(1))
}

func TestDefaultBatchConfig(t *testing.T) {
	cfg := DefaultBatchConfig()
	assert.Equal(t, 10, cfg.MaxBatchSize)
	assert.Equal(t, 100*time.Millisecond, cfg.MaxWaitTime)
	assert.Equal(t, 1000, cfg.QueueSize)
	assert.Equal(t, 4, cfg.Workers)
	assert.True(t, cfg.RetryOnFailure)
}

func TestBatchProcessor_Concurrent(t *testing.T) {
	ctx := testutil.TestContext(t)

	cfg := DefaultBatchConfig()
	cfg.MaxBatchSize = 5
	cfg.MaxWaitTime = 50 * time.Millisecond
	cfg.Workers = 4
	cfg.QueueSize = 200
	bp := NewBatchProcessor(cfg, echoHandler())
	t.Cleanup(bp.Close)

	const numGoroutines = 10
	const requestsPerGoroutine = 10

	var wg sync.WaitGroup
	var successCount atomic.Int32

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(gID int) {
			defer wg.Done()
			for r := 0; r < requestsPerGoroutine; r++ {
				req := makeRequest(fmt.Sprintf("g%d-r%d", gID, r))
				resp, err := bp.SubmitSync(ctx, req)
				if err == nil && resp != nil {
					successCount.Add(1)
				}
			}
		}(g)
	}

	wg.Wait()
	assert.Greater(t, successCount.Load(), int32(0), "at least some requests should succeed")

	stats := bp.Stats()
	assert.GreaterOrEqual(t, stats.Submitted, int64(1))
}
