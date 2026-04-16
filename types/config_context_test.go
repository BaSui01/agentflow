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
