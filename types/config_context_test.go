package types

import "testing"

func TestDefaultContextConfig(t *testing.T) {
	cfg := DefaultContextConfig()
	if cfg == nil {
		t.Fatal("expected default context config")
	}
	if !cfg.Enabled {
		t.Fatal("expected context to be enabled by default")
	}
	if cfg.MaxContextTokens == 0 {
		t.Fatal("expected max context tokens to be set")
	}
	if !cfg.TraceFeedbackEnabled {
		t.Fatal("expected trace feedback to be enabled by default")
	}
	if cfg.TraceFeedbackComplexityThreshold != 2 {
		t.Fatalf("expected complexity threshold 2, got %d", cfg.TraceFeedbackComplexityThreshold)
	}
	if cfg.TraceHistoryMaxUsageRatio != 0.85 {
		t.Fatalf("expected trace history max usage ratio 0.85, got %v", cfg.TraceHistoryMaxUsageRatio)
	}
}

func TestAgentConfig_IsContextEnabled(t *testing.T) {
	var cfg AgentConfig
	if cfg.IsContextEnabled() {
		t.Fatal("nil context config should be disabled")
	}
	cfg.Context = &ContextConfig{Enabled: true}
	if !cfg.IsContextEnabled() {
		t.Fatal("expected enabled context config")
	}
}
