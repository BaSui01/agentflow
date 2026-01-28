package llm

import "testing"

func TestHealthMonitor_DefaultScore_QPSLimit_AndProbe(t *testing.T) {
	t.Parallel()

	m := NewHealthMonitor(nil)
	t.Cleanup(m.Stop)

	if got := m.GetHealthScore("p1"); got != 1.0 {
		t.Fatalf("expected default health score 1.0, got %v", got)
	}

	m.SetMaxQPS("p1", 2)
	m.IncrementQPS("p1")
	if got := m.GetHealthScore("p1"); got != 1.0 {
		t.Fatalf("expected healthy score under QPS limit, got %v", got)
	}

	m.SetMaxQPS("p2", 1)
	m.IncrementQPS("p2")
	if got := m.GetHealthScore("p2"); got != 0.0 {
		t.Fatalf("expected score 0.0 when QPS limit reached, got %v", got)
	}

	m.UpdateProbe("p3", &HealthStatus{Healthy: false}, nil)
	if got := m.GetHealthScore("p3"); got != 0.0 {
		t.Fatalf("expected score 0.0 when probe is unhealthy, got %v", got)
	}
}
