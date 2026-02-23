package llm

import (
	"errors"
	"testing"
	"time"
)

func TestObserveProviderHealthCheck(t *testing.T) {
	// These just verify no panics occur with various inputs
	t.Run("healthy with latency", func(t *testing.T) {
		observeProviderHealthCheck("provider-1", true, 100*time.Millisecond, nil)
	})

	t.Run("unhealthy with error", func(t *testing.T) {
		observeProviderHealthCheck("provider-2", false, 0, errors.New("timeout"))
	})

	t.Run("empty provider ID defaults to unknown", func(t *testing.T) {
		observeProviderHealthCheck("", true, 50*time.Millisecond, nil)
	})

	t.Run("zero latency skips histogram", func(t *testing.T) {
		observeProviderHealthCheck("provider-3", true, 0, nil)
	})
}
