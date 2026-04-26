package runtime

import (
	"testing"

	"github.com/BaSui01/agentflow/agent/capabilities/guardrails"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

func TestRuntimeGuardrailsFromTypes(t *testing.T) {
	t.Run("nil config returns nil", func(t *testing.T) {
		got := runtimeGuardrailsFromTypes(nil)
		if got != nil {
			t.Errorf("expected nil, got %+v", got)
		}
	})

	t.Run("disabled config returns nil", func(t *testing.T) {
		cfg := &types.GuardrailsConfig{Enabled: false}
		got := runtimeGuardrailsFromTypes(cfg)
		if got != nil {
			t.Errorf("expected nil, got %+v", got)
		}
	})

	t.Run("converts basic config", func(t *testing.T) {
		cfg := &types.GuardrailsConfig{
			Enabled:            true,
			MaxInputLength:     5000,
			BlockedKeywords:    []string{"bad", "evil"},
			PIIDetection:       true,
			InjectionDetection: true,
			MaxRetries:         3,
			OnInputFailure:     "reject",
			OnOutputFailure:    "filter",
		}

		got := runtimeGuardrailsFromTypes(cfg)
		if got == nil {
			t.Fatal("expected non-nil result")
		}
		if got.MaxInputLength != 5000 {
			t.Errorf("MaxInputLength: expected 5000, got %d", got.MaxInputLength)
		}
		if len(got.BlockedKeywords) != 2 {
			t.Errorf("BlockedKeywords: expected 2, got %d", len(got.BlockedKeywords))
		}
		if !got.PIIDetectionEnabled {
			t.Error("PIIDetectionEnabled: expected true")
		}
		if !got.InjectionDetection {
			t.Error("InjectionDetection: expected true")
		}
		if got.MaxRetries != 3 {
			t.Errorf("MaxRetries: expected 3, got %d", got.MaxRetries)
		}
		if got.OnInputFailure != "reject" {
			t.Errorf("OnInputFailure: expected reject, got %s", got.OnInputFailure)
		}
		if got.OnOutputFailure != "filter" {
			t.Errorf("OnOutputFailure: expected filter, got %s", got.OnOutputFailure)
		}
	})

	t.Run("trims whitespace from failure actions", func(t *testing.T) {
		cfg := &types.GuardrailsConfig{
			Enabled:        true,
			OnInputFailure: "  reject  ",
		}

		got := runtimeGuardrailsFromTypes(cfg)
		if got == nil {
			t.Fatal("expected non-nil result")
		}
		if got.OnInputFailure != "reject" {
			t.Errorf("OnInputFailure: expected 'reject', got '%s'", got.OnInputFailure)
		}
	})
}

func TestTypesGuardrailsFromRuntime(t *testing.T) {
	t.Run("nil config returns nil", func(t *testing.T) {
		got := typesGuardrailsFromRuntime(nil)
		if got != nil {
			t.Errorf("expected nil, got %+v", got)
		}
	})

	t.Run("converts runtime config to types", func(t *testing.T) {
		cfg := &guardrails.GuardrailsConfig{
			MaxInputLength:      3000,
			BlockedKeywords:     []string{"keyword1"},
			PIIDetectionEnabled: true,
			InjectionDetection:  true,
			MaxRetries:          2,
			OnInputFailure:      "reject",
			OnOutputFailure:     "warn",
		}

		got := typesGuardrailsFromRuntime(cfg)
		if got == nil {
			t.Fatal("expected non-nil result")
		}
		if !got.Enabled {
			t.Error("Enabled: expected true")
		}
		if got.MaxInputLength != 3000 {
			t.Errorf("MaxInputLength: expected 3000, got %d", got.MaxInputLength)
		}
		if len(got.BlockedKeywords) != 1 {
			t.Errorf("BlockedKeywords: expected 1, got %d", len(got.BlockedKeywords))
		}
		if got.BlockedKeywords[0] != "keyword1" {
			t.Errorf("BlockedKeywords[0]: expected keyword1, got %s", got.BlockedKeywords[0])
		}
		if got.OnInputFailure != "reject" {
			t.Errorf("OnInputFailure: expected reject, got %s", got.OnInputFailure)
		}
		if got.OnOutputFailure != "warn" {
			t.Errorf("OnOutputFailure: expected warn, got %s", got.OnOutputFailure)
		}
	})

	t.Run("does not mutate original slice", func(t *testing.T) {
		cfg := &guardrails.GuardrailsConfig{
			BlockedKeywords: []string{"a", "b"},
		}

		got := typesGuardrailsFromRuntime(cfg)
		got.BlockedKeywords[0] = "modified"

		if cfg.BlockedKeywords[0] != "a" {
			t.Error("original slice was mutated")
		}
	})
}

func TestNewGuardrailsManager(t *testing.T) {
	logger := zap.NewNop()
	mgr := NewGuardrailsManager(logger)

	if mgr == nil {
		t.Fatal("expected non-nil manager")
	}
}

func TestNewGuardrailsCoordinator(t *testing.T) {
	logger := zap.NewNop()
	cfg := guardrails.DefaultConfig()
	coord := NewGuardrailsCoordinator(cfg, logger)

	if coord == nil {
		t.Fatal("expected non-nil coordinator")
	}
}
