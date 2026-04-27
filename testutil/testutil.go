package testutil

import (
	"context"
	"testing"
	"time"
)

func TestContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}

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
