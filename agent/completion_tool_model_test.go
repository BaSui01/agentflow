package agent

import "testing"

func TestEffectiveToolModel_UsesConfiguredToolModel(t *testing.T) {
	got := effectiveToolModel("gpt-4o", "gpt-4o-mini")
	if got != "gpt-4o-mini" {
		t.Fatalf("expected configured tool model, got %q", got)
	}
}

func TestEffectiveToolModel_FallbackToMainModel(t *testing.T) {
	got := effectiveToolModel("gpt-4o", " ")
	if got != "gpt-4o" {
		t.Fatalf("expected main model fallback, got %q", got)
	}
}
