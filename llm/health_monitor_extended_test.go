package llm

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealthMonitor_IncrementAndGetQPS(t *testing.T) {
	m := NewHealthMonitor(nil)
	t.Cleanup(m.Stop)

	assert.Equal(t, 0, m.GetCurrentQPS("p1"))

	m.IncrementQPS("p1")
	m.IncrementQPS("p1")
	m.IncrementQPS("p1")

	qps := m.GetCurrentQPS("p1")
	assert.Equal(t, 3, qps)
}

func TestHealthMonitor_SetMaxQPS(t *testing.T) {
	m := NewHealthMonitor(nil)
	t.Cleanup(m.Stop)

	m.SetMaxQPS("p1", 10)
	// Should create counter even if it didn't exist
	assert.Equal(t, 0, m.GetCurrentQPS("p1"))

	// Set on existing counter
	m.IncrementQPS("p1")
	m.SetMaxQPS("p1", 5)
	assert.Equal(t, 1, m.GetCurrentQPS("p1"))
}

func TestHealthMonitor_GetHealthScore_WithStoredScore(t *testing.T) {
	m := NewHealthMonitor(nil)
	t.Cleanup(m.Stop)

	// Manually set a health score
	m.mu.Lock()
	m.healthScore["p1"] = 0.7
	m.mu.Unlock()

	score := m.GetHealthScore("p1")
	assert.Equal(t, 0.7, score)
}

func TestHealthMonitor_UpdateProbe(t *testing.T) {
	m := NewHealthMonitor(nil)
	t.Cleanup(m.Stop)

	t.Run("empty provider code is no-op", func(t *testing.T) {
		m.UpdateProbe("", &HealthStatus{Healthy: true}, nil)
		// Should not panic or store anything
	})

	t.Run("healthy probe", func(t *testing.T) {
		m.UpdateProbe("p1", &HealthStatus{
			Healthy:   true,
			Latency:   100 * time.Millisecond,
			ErrorRate: 0.01,
		}, nil)

		score := m.GetHealthScore("p1")
		assert.Equal(t, 1.0, score) // healthy probe, default score
	})

	t.Run("unhealthy probe", func(t *testing.T) {
		m.UpdateProbe("p2", &HealthStatus{Healthy: false}, nil)
		score := m.GetHealthScore("p2")
		assert.Equal(t, 0.0, score) // unhealthy probe -> 0
	})

	t.Run("error overrides healthy status", func(t *testing.T) {
		m.UpdateProbe("p3", &HealthStatus{Healthy: true}, errors.New("connection refused"))
		score := m.GetHealthScore("p3")
		assert.Equal(t, 0.0, score) // error makes it unhealthy
	})

	t.Run("nil status with error", func(t *testing.T) {
		m.UpdateProbe("p4", nil, errors.New("timeout"))
		score := m.GetHealthScore("p4")
		assert.Equal(t, 0.0, score)
	})
}

func TestHealthMonitor_GetAllProviderStats(t *testing.T) {
	m := NewHealthMonitor(nil)
	t.Cleanup(m.Stop)

	// Empty initially
	stats := m.GetAllProviderStats()
	assert.Empty(t, stats)

	// Add some health scores
	m.mu.Lock()
	m.healthScore["p1"] = 0.9
	m.healthScore["p2"] = 0.5
	m.mu.Unlock()

	stats = m.GetAllProviderStats()
	assert.Len(t, stats, 2)
}

func TestHealthMonitor_GetAllProviderStats_WithProbeTime(t *testing.T) {
	m := NewHealthMonitor(nil)
	t.Cleanup(m.Stop)

	probeTime := time.Now().Add(-5 * time.Minute)
	m.mu.Lock()
	m.healthScore["p1"] = 0.8
	m.probe["p1"] = ProviderProbeResult{
		Healthy:     true,
		LastCheckAt: probeTime,
	}
	m.mu.Unlock()

	stats := m.GetAllProviderStats()
	require.Len(t, stats, 1)
	assert.Equal(t, probeTime, stats[0].LastCheckAt)
}

func TestQPSCounter_BumpWindow_LargeGap(t *testing.T) {
	c := newQPSCounter(time.Now())
	// Simulate a large time gap (>60 seconds)
	now := time.Now().Unix()
	c.buckets[now%60].Store(100)

	// Bump with a gap > 60 seconds should clear all buckets
	c.bumpWindow(now + 70)

	var total int64
	for i := range c.buckets {
		total += c.buckets[i].Load()
	}
	assert.Equal(t, int64(0), total)
}

