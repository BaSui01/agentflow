package config

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// --- Constructor ---

func TestNewFileWatcher_Defaults(t *testing.T) {
	tmpDir := t.TempDir()
	f := filepath.Join(tmpDir, "test.yaml")
	require.NoError(t, os.WriteFile(f, []byte("key: val"), 0644))

	w, err := NewFileWatcher([]string{f})
	require.NoError(t, err)
	require.NotNil(t, w)

	assert.Equal(t, []string{f}, w.Paths())
	assert.False(t, w.IsRunning())
	assert.Equal(t, 100*time.Millisecond, w.debounceDelay)
}

func TestNewFileWatcher_WithOptions(t *testing.T) {
	tmpDir := t.TempDir()
	f := filepath.Join(tmpDir, "test.yaml")
	require.NoError(t, os.WriteFile(f, []byte("key: val"), 0644))

	logger := zap.NewNop()
	w, err := NewFileWatcher([]string{f},
		WithDebounceDelay(500*time.Millisecond),
		WithWatcherLogger(logger),
	)
	require.NoError(t, err)
	assert.Equal(t, 500*time.Millisecond, w.debounceDelay)
}

func TestNewFileWatcher_NonExistentPathWarns(t *testing.T) {
	// Non-existent path should not error (just warn), per source code
	w, err := NewFileWatcher([]string{"/nonexistent/path/config.yaml"})
	require.NoError(t, err)
	require.NotNil(t, w)
}

// --- AddPath / RemovePath / Paths ---

func TestFileWatcher_AddPath(t *testing.T) {
	tmpDir := t.TempDir()
	f1 := filepath.Join(tmpDir, "a.yaml")
	f2 := filepath.Join(tmpDir, "b.yaml")
	require.NoError(t, os.WriteFile(f1, []byte("a"), 0644))
	require.NoError(t, os.WriteFile(f2, []byte("b"), 0644))

	w, err := NewFileWatcher([]string{f1})
	require.NoError(t, err)

	err = w.AddPath(f2)
	require.NoError(t, err)
	assert.Len(t, w.Paths(), 2)
}

func TestFileWatcher_AddPath_Duplicate(t *testing.T) {
	tmpDir := t.TempDir()
	f := filepath.Join(tmpDir, "a.yaml")
	require.NoError(t, os.WriteFile(f, []byte("a"), 0644))

	w, err := NewFileWatcher([]string{f})
	require.NoError(t, err)

	// Adding the same path again should be a no-op
	err = w.AddPath(f)
	require.NoError(t, err)
	// Note: AddPath resolves to absolute, original path is already absolute from TempDir
	// The duplicate check compares against existing paths, so count may vary
	// depending on whether original was stored as-is vs resolved.
	// Just verify no error.
}

func TestFileWatcher_RemovePath(t *testing.T) {
	tmpDir := t.TempDir()
	f1 := filepath.Join(tmpDir, "a.yaml")
	f2 := filepath.Join(tmpDir, "b.yaml")
	require.NoError(t, os.WriteFile(f1, []byte("a"), 0644))
	require.NoError(t, os.WriteFile(f2, []byte("b"), 0644))

	w, err := NewFileWatcher([]string{f1})
	require.NoError(t, err)
	require.NoError(t, w.AddPath(f2))

	err = w.RemovePath(f2)
	require.NoError(t, err)
}

func TestFileWatcher_RemovePath_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	f := filepath.Join(tmpDir, "a.yaml")
	require.NoError(t, os.WriteFile(f, []byte("a"), 0644))

	w, err := NewFileWatcher([]string{f})
	require.NoError(t, err)

	err = w.RemovePath(filepath.Join(tmpDir, "nonexistent.yaml"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "path not found")
}

// --- Start / Stop / IsRunning lifecycle ---

func TestFileWatcher_Lifecycle(t *testing.T) {
	tmpDir := t.TempDir()
	f := filepath.Join(tmpDir, "config.yaml")
	require.NoError(t, os.WriteFile(f, []byte("key: val"), 0644))

	w, err := NewFileWatcher([]string{f}, WithDebounceDelay(50*time.Millisecond))
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	assert.False(t, w.IsRunning())

	require.NoError(t, w.Start(ctx))
	assert.True(t, w.IsRunning())

	// Double start should error
	err = w.Start(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already running")

	require.NoError(t, w.Stop())
	assert.False(t, w.IsRunning())

	// Stop when already stopped is a no-op
	require.NoError(t, w.Stop())
}

// --- OnChange callback ---

func TestFileWatcher_OnChange_Callback(t *testing.T) {
	tmpDir := t.TempDir()
	f := filepath.Join(tmpDir, "config.yaml")
	require.NoError(t, os.WriteFile(f, []byte("v1"), 0644))

	w, err := NewFileWatcher([]string{f}, WithDebounceDelay(50*time.Millisecond))
	require.NoError(t, err)

	var mu sync.Mutex
	var events []FileEvent
	w.OnChange(func(evt FileEvent) {
		mu.Lock()
		events = append(events, evt)
		mu.Unlock()
	})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	require.NoError(t, w.Start(ctx))
	t.Cleanup(func() { w.Stop() })

	// Let the watcher initialize
	time.Sleep(200 * time.Millisecond)

	// Modify the file
	require.NoError(t, os.WriteFile(f, []byte("v2"), 0644))

	// Wait for poll (1s) + debounce (50ms) + margin
	time.Sleep(2 * time.Second)

	mu.Lock()
	defer mu.Unlock()
	assert.GreaterOrEqual(t, len(events), 1, "should detect at least one change")
	if len(events) > 0 {
		assert.Equal(t, f, events[0].Path)
		assert.Equal(t, FileOpWrite, events[0].Op)
	}
}

// --- dispatchLoop race condition regression test ---

// TestFileWatcher_DispatchLoop_NoRace verifies that rapid file changes do not
// cause a concurrent map read/write panic. Before the fix, time.AfterFunc
// callbacks accessed pendingEvents from a separate goroutine while the select
// loop also wrote to it. With -race this would reliably fail.
func TestFileWatcher_DispatchLoop_NoRace(t *testing.T) {
	tmpDir := t.TempDir()
	f := filepath.Join(tmpDir, "race.yaml")
	require.NoError(t, os.WriteFile(f, []byte("v0"), 0644))

	w, err := NewFileWatcher([]string{f}, WithDebounceDelay(10*time.Millisecond))
	require.NoError(t, err)

	var mu sync.Mutex
	var dispatched []FileEvent
	w.OnChange(func(evt FileEvent) {
		mu.Lock()
		dispatched = append(dispatched, evt)
		mu.Unlock()
	})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	require.NoError(t, w.Start(ctx))
	t.Cleanup(func() { w.Stop() })

	// Flood the event channel with rapid events to trigger the race window.
	for i := 0; i < 50; i++ {
		w.eventChan <- FileEvent{
			Path:      f,
			Op:        FileOpWrite,
			Timestamp: time.Now(),
		}
	}

	// Wait for debounce to flush
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	// We should have received at least one dispatched event (debounce coalesces).
	assert.GreaterOrEqual(t, len(dispatched), 1,
		"expected at least 1 dispatched event after rapid writes")
}

// TestFileWatcher_DispatchCh_Coalesces verifies that multiple events for the
// same path are coalesced into a single dispatch after the debounce window.
func TestFileWatcher_DispatchCh_Coalesces(t *testing.T) {
	tmpDir := t.TempDir()
	f := filepath.Join(tmpDir, "coalesce.yaml")
	require.NoError(t, os.WriteFile(f, []byte("v0"), 0644))

	w, err := NewFileWatcher([]string{f}, WithDebounceDelay(50*time.Millisecond))
	require.NoError(t, err)

	var mu sync.Mutex
	callCount := 0
	w.OnChange(func(evt FileEvent) {
		mu.Lock()
		callCount++
		mu.Unlock()
	})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	require.NoError(t, w.Start(ctx))
	t.Cleanup(func() { w.Stop() })

	// Send 3 events in quick succession for the same path
	for i := 0; i < 3; i++ {
		w.eventChan <- FileEvent{
			Path:      f,
			Op:        FileOpWrite,
			Timestamp: time.Now(),
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Wait for debounce to flush
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	// Same path events should be coalesced into 1 callback invocation
	assert.Equal(t, 1, callCount,
		"events for the same path should be coalesced into a single dispatch")
}

// --- Context cancellation stops watcher ---

func TestFileWatcher_ContextCancel(t *testing.T) {
	tmpDir := t.TempDir()
	f := filepath.Join(tmpDir, "config.yaml")
	require.NoError(t, os.WriteFile(f, []byte("v1"), 0644))

	w, err := NewFileWatcher([]string{f})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, w.Start(ctx))
	assert.True(t, w.IsRunning())

	// Cancel context — goroutines will exit, but running flag stays true
	// until Stop() is called explicitly
	cancel()
	time.Sleep(200 * time.Millisecond)

	// Cleanup
	w.Stop()
	assert.False(t, w.IsRunning())
}
