package orchestration

import (
	"testing"

	"go.uber.org/zap"
)

func TestNewDefaultOrchestrator_UsesModeRegistryExecutors(t *testing.T) {
	o := NewDefaultOrchestrator(DefaultOrchestratorConfig(), zap.NewNop())

	tests := []struct {
		pattern Pattern
	}{
		{pattern: PatternCollaboration},
		{pattern: PatternCrew},
		{pattern: PatternHierarchical},
	}

	for _, tt := range tests {
		exec, ok := o.patterns[tt.pattern]
		if !ok {
			t.Fatalf("expected pattern %q to be registered", tt.pattern)
		}
		if _, ok := exec.(*ModeRegistryExecutor); !ok {
			t.Fatalf("pattern %q should use ModeRegistryExecutor, got %T", tt.pattern, exec)
		}
	}

	if _, ok := o.patterns[PatternHandoff]; !ok {
		t.Fatalf("expected handoff pattern to remain registered")
	}
}

