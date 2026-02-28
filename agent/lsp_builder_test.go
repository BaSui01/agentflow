package agent

import (
	"context"
	"testing"

	"go.uber.org/zap"
)

func TestAgentBuilder_WithDefaultLSPServer(t *testing.T) {
	logger := zap.NewNop()
	provider := &testProvider{name: "test-model"}

	cfg := Config{
		ID:    "lsp-builder-test",
		Name:  "LSP Builder Test",
		Type:  TypeGeneric,
		Model: "test-model",
	}

	ag, err := NewAgentBuilder(cfg).
		WithProvider(provider).
		WithLogger(logger).
		WithDefaultLSPServer("", "").
		Build()
	if err != nil {
		t.Fatalf("build agent with default lsp failed: %v", err)
	}

	status := ag.GetFeatureStatus()
	if !status["lsp"] {
		t.Fatalf("expected lsp feature enabled, status=%v", status)
	}

	if err := ag.ValidateConfiguration(); err != nil {
		t.Fatalf("validate configuration failed: %v", err)
	}

	if err := ag.Teardown(context.Background()); err != nil {
		t.Fatalf("teardown failed: %v", err)
	}
}
