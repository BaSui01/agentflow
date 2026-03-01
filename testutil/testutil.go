package testutil

import (
	"context"
	"testing"
	"time"
)

// TestContext returns a cancellable context bound to test lifecycle.
func TestContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// WaitFor polls cond until it returns true or timeout is reached.
func WaitFor(cond func() bool, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return cond()
}

// WaitForChannel waits for a value from ch until timeout.
func WaitForChannel[T any](ch <-chan T, timeout time.Duration) (T, bool) {
	var zero T
	select {
	case v, ok := <-ch:
		if !ok {
			return zero, false
		}
		return v, true
	case <-time.After(timeout):
		return zero, false
	}
}
