package agent

import (
	"strings"
	"testing"

	agentcontext "github.com/BaSui01/agentflow/agent/context"
)

func TestEphemeralPromptLayerBuilder_Build(t *testing.T) {
	builder := NewEphemeralPromptLayerBuilder()
	layers := builder.Build(EphemeralPromptLayerInput{
		PublicContext:            map[string]any{"workspace": "repo-a"},
		TraceID:                  "trace-1",
		TenantID:                 "tenant-1",
		UserID:                   "user-1",
		ChannelID:                "session-1",
		CheckpointID:             "cp-123",
		AllowedTools:             []string{"read_file", "write_file"},
		AcceptanceCriteria:       []string{"cite sources"},
		ToolVerificationRequired: true,
		ContextStatus: &agentcontext.Status{
			UsageRatio:     0.9,
			Level:          agentcontext.LevelAggressive,
			Recommendation: "WARNING: compression recommended",
			CurrentTokens:  900,
			MaxTokens:      1000,
		},
	})
	if len(layers) != 4 {
		t.Fatalf("expected 4 layers, got %d", len(layers))
	}
	if layers[0].ID != "session_overlay" {
		t.Fatalf("expected session_overlay layer first, got %q", layers[0].ID)
	}
	if layers[1].ID != "tool_guidance" {
		t.Fatalf("expected tool_guidance layer second, got %q", layers[1].ID)
	}
	if !strings.Contains(layers[1].Content, "Approval-required tools") {
		t.Fatalf("expected risk-layered tool guidance, got %q", layers[1].Content)
	}
	if layers[2].ID != "verification_gate" {
		t.Fatalf("expected verification_gate layer third, got %q", layers[2].ID)
	}
	if layers[3].ID != "context_pressure" {
		t.Fatalf("expected context_pressure layer fourth, got %q", layers[3].ID)
	}
}

func TestEphemeralPromptLayerBuilder_BuildSkipsEmptyInputs(t *testing.T) {
	builder := NewEphemeralPromptLayerBuilder()
	layers := builder.Build(EphemeralPromptLayerInput{})
	if layers != nil {
		t.Fatalf("expected nil layers, got %#v", layers)
	}
}
