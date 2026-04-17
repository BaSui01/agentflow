package agent

import (
	"testing"

	agentcontext "github.com/BaSui01/agentflow/agent/context"
)

func TestEphemeralPromptLayerBuilder_Build(t *testing.T) {
	builder := NewEphemeralPromptLayerBuilder()
	layers := builder.Build(EphemeralPromptLayerInput{
		PublicContext: map[string]any{"tenant_id": "tenant-1"},
		CheckpointID:  "cp-123",
		ContextStatus: &agentcontext.Status{
			UsageRatio:     0.9,
			Level:          agentcontext.LevelAggressive,
			Recommendation: "WARNING: compression recommended",
			CurrentTokens:  900,
			MaxTokens:      1000,
		},
	})
	if len(layers) != 3 {
		t.Fatalf("expected 3 layers, got %d", len(layers))
	}
	if layers[0].ID != "request_context" {
		t.Fatalf("expected request_context layer first, got %q", layers[0].ID)
	}
	if layers[1].ID != "resume_context" {
		t.Fatalf("expected resume_context layer second, got %q", layers[1].ID)
	}
	if layers[2].ID != "context_pressure" {
		t.Fatalf("expected context_pressure layer third, got %q", layers[2].ID)
	}
}

func TestEphemeralPromptLayerBuilder_BuildSkipsEmptyInputs(t *testing.T) {
	builder := NewEphemeralPromptLayerBuilder()
	layers := builder.Build(EphemeralPromptLayerInput{})
	if layers != nil {
		t.Fatalf("expected nil layers, got %#v", layers)
	}
}
