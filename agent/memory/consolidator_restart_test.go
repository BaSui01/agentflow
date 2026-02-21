package memory

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestMemoryConsolidator_StopThenRestart verifies that after Stop(),
// a subsequent Start()+Stop() cycle works correctly — i.e. closeOnce
// is reset so the new stopCh is actually closed.
func TestMemoryConsolidator_StopThenRestart(t *testing.T) {
	t.Parallel()

	cfg := DefaultEnhancedMemoryConfig()
	cfg.ConsolidationEnabled = true
	cfg.ConsolidationInterval = 50 * time.Millisecond

	system := NewEnhancedMemorySystem(nil, nil, nil, nil, nil, cfg, zap.NewNop())
	c := system.consolidator
	require.NotNil(t, c)

	ctx := context.Background()

	// First cycle: Start -> Stop
	require.NoError(t, c.Start(ctx))
	require.NoError(t, c.Stop())

	// Second cycle: Start -> Stop (this used to leak the goroutine)
	require.NoError(t, c.Start(ctx))
	require.NoError(t, c.Stop())

	// Third cycle to be thorough
	require.NoError(t, c.Start(ctx))
	require.NoError(t, c.Stop())
}

// TestMemoryConsolidator_StopClosesChannel verifies that Stop actually
// closes the stopCh so the run goroutine can exit.
func TestMemoryConsolidator_StopClosesChannel(t *testing.T) {
	t.Parallel()

	cfg := DefaultEnhancedMemoryConfig()
	cfg.ConsolidationEnabled = true
	cfg.ConsolidationInterval = 1 * time.Hour // long interval so ticker won't fire

	system := NewEnhancedMemorySystem(nil, nil, nil, nil, nil, cfg, zap.NewNop())
	c := system.consolidator
	require.NotNil(t, c)

	ctx := context.Background()
	require.NoError(t, c.Start(ctx))

	// Stop should close the channel
	require.NoError(t, c.Stop())

	// After stop, the channel should be closed (reading returns immediately)
	select {
	case <-c.stopCh:
		// expected — channel is closed
	default:
		t.Fatal("stopCh was not closed after Stop()")
	}
}

// TestMemoryConsolidator_RestartGoroutineExits ensures the goroutine from
// the first Start() exits before the second Start() launches a new one,
// preventing goroutine leaks.
func TestMemoryConsolidator_RestartGoroutineExits(t *testing.T) {
	t.Parallel()

	cfg := DefaultEnhancedMemoryConfig()
	cfg.ConsolidationEnabled = true
	cfg.ConsolidationInterval = 10 * time.Millisecond

	system := NewEnhancedMemorySystem(nil, nil, nil, nil, nil, cfg, zap.NewNop())
	c := system.consolidator
	require.NotNil(t, c)

	ctx := context.Background()

	for i := 0; i < 5; i++ {
		require.NoError(t, c.Start(ctx))
		// Give the goroutine a moment to start
		time.Sleep(5 * time.Millisecond)
		require.NoError(t, c.Stop())
		// Give the goroutine a moment to exit
		time.Sleep(15 * time.Millisecond)

		// After stop + drain, running should be false
		c.mu.Lock()
		running := c.running
		c.mu.Unlock()
		assert.False(t, running, "consolidator should not be running after Stop() on iteration %d", i)
	}
}

// TestMemoryConsolidator_ConcurrentStopStart exercises concurrent
// Start/Stop to check for races (run with -race).
func TestMemoryConsolidator_ConcurrentStopStart(t *testing.T) {
	t.Parallel()

	cfg := DefaultEnhancedMemoryConfig()
	cfg.ConsolidationEnabled = true
	cfg.ConsolidationInterval = 10 * time.Millisecond

	system := NewEnhancedMemorySystem(nil, nil, nil, nil, nil, cfg, zap.NewNop())
	c := system.consolidator
	require.NotNil(t, c)

	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = c.Start(ctx)
			time.Sleep(2 * time.Millisecond)
			_ = c.Stop()
		}()
	}
	wg.Wait()

	// Final cleanup: ensure it can still be started and stopped cleanly
	// (may already be stopped, so ignore errors)
	_ = c.Stop()
	time.Sleep(20 * time.Millisecond)
	require.NoError(t, c.Start(ctx))
	require.NoError(t, c.Stop())
}
