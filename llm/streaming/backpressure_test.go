package streaming

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDropPolicyOldest_ConcurrentWrite verifies that DropPolicyOldest does not
// block permanently when multiple goroutines write concurrently. Before the
// fix, the bare channel send `s.buffer <- token` after draining could block
// forever if another goroutine filled the buffer in between.
func TestDropPolicyOldest_ConcurrentWrite(t *testing.T) {
	config := BackpressureConfig{
		BufferSize:    4,
		HighWaterMark: 0.5, // triggers at 2/4 = 50%
		LowWaterMark:  0.1,
		DropPolicy:    DropPolicyOldest,
	}
	stream := NewBackpressureStream(config)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Pre-fill the buffer to the high water mark so DropPolicyOldest kicks in.
	for i := 0; i < config.BufferSize; i++ {
		err := stream.Write(ctx, Token{Content: "prefill", Index: i})
		require.NoError(t, err)
	}

	// Launch multiple concurrent writers. Before the fix, some of these could
	// deadlock on the bare `s.buffer <- token` send.
	const writers = 8
	var wg sync.WaitGroup
	errors := make([]error, writers)

	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			writeCtx, writeCancel := context.WithTimeout(ctx, 1*time.Second)
			defer writeCancel()
			errors[idx] = stream.Write(writeCtx, Token{Content: "concurrent", Index: 100 + idx})
		}(i)
	}

	// Drain some tokens so writers can make progress.
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case _, ok := <-stream.ReadChan():
				if !ok {
					return
				}
			}
		}
	}()

	wg.Wait()

	// All writes should have completed (no deadlock) and returned nil or a
	// context/stream error — never hung.
	for i, err := range errors {
		if err != nil {
			assert.ErrorIs(t, err, context.DeadlineExceeded,
				"writer %d returned unexpected error: %v", i, err)
		}
	}
}

// TestDropPolicyOldest_DropsOldToken verifies that the oldest token is
// discarded and the new token is written when the buffer is full.
func TestDropPolicyOldest_DropsOldToken(t *testing.T) {
	config := BackpressureConfig{
		BufferSize:    3,
		HighWaterMark: 0.9, // triggers only when buffer is nearly full (3/3 = 1.0 >= 0.9)
		LowWaterMark:  0.1,
		DropPolicy:    DropPolicyOldest,
	}
	stream := NewBackpressureStream(config)
	ctx := context.Background()

	// Fill the buffer completely (3 tokens). The first two writes go through
	// the normal path (level < 0.9). The third write fills the buffer.
	require.NoError(t, stream.Write(ctx, Token{Content: "a", Index: 0}))
	require.NoError(t, stream.Write(ctx, Token{Content: "b", Index: 1}))
	require.NoError(t, stream.Write(ctx, Token{Content: "c", Index: 2}))

	// Now the buffer is full (3/3 = 1.0 >= 0.9), so the next write triggers
	// DropPolicyOldest: it drains "a", then writes "d".
	require.NoError(t, stream.Write(ctx, Token{Content: "d", Index: 3}))

	stats := stream.Stats()
	assert.Equal(t, int64(1), stats.Dropped, "should have dropped 1 token")

	// Read remaining tokens — should be "b", "c", "d".
	tok1, err := stream.Read(ctx)
	require.NoError(t, err)
	assert.Equal(t, "b", tok1.Content)

	tok2, err := stream.Read(ctx)
	require.NoError(t, err)
	assert.Equal(t, "c", tok2.Content)

	tok3, err := stream.Read(ctx)
	require.NoError(t, err)
	assert.Equal(t, "d", tok3.Content)
}
