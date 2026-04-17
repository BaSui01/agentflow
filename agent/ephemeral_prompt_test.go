package agent

import (
	"strings"
	"testing"

	agentcontext "github.com/BaSui01/agentflow/agent/context"
)

func TestEphemeralPromptLayerBuilder_Build(t *testing.T) {
	builder := NewEphemeralPromptLayerBuilder()
	layers := builder.Build(EphemeralPromptLayerInput{
		PublicContext: map[string]any{"workspace": "repo-a"},
		TraceID:       "trace-1",
		TenantID:      "tenant-1",
		UserID:        "user-1",
		ChannelID:     "session-1",
		TraceFeedbackPlan: &TraceFeedbackPlan{
			PlannerID:          "rule_based_trace_feedback_planner",
			PlannerVersion:     "v1",
			Confidence:         0.8,
			Metadata:           map[string]any{"planner_kind": "rule_based"},
			InjectMemoryRecall: true,
			Goal:               "resume_prior_execution",
			RecommendedAction:  TraceFeedbackSynopsisAndHistory,
			PrimaryLayer:       "trace_synopsis",
			SecondaryLayer:     "trace_history",
			Reasons:            []string{"resume", "verification_gate"},
			SelectedLayers:     []string{"trace_synopsis", "trace_history", "memory_recall"},
			Summary:            "goal=resume_prior_execution | action=synopsis_and_history | inject=trace_synopsis,trace_history",
		},
		TraceSynopsis:            "layers=session_overlay | ended=solved",
		TraceHistorySummary:      "12 entries;types=approval:4,validation_gate:3",
		TraceHistoryEventCount:   12,
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
	if len(layers) != 7 {
		t.Fatalf("expected 7 layers, got %d", len(layers))
	}
	if layers[0].ID != "session_overlay" {
		t.Fatalf("expected session_overlay layer first, got %q", layers[0].ID)
	}
	if layers[1].ID != "trace_feedback_plan" {
		t.Fatalf("expected trace_feedback_plan layer second, got %q", layers[1].ID)
	}
	if !strings.Contains(layers[1].Content, "Recommended action: synopsis_and_history") || !strings.Contains(layers[1].Content, "Planner: rule_based_trace_feedback_planner@v1") {
		t.Fatalf("expected plan content, got %q", layers[1].Content)
	}
	if layers[2].ID != "trace_synopsis" {
		t.Fatalf("expected trace_synopsis layer third, got %q", layers[2].ID)
	}
	if !strings.Contains(layers[2].Content, "ended=solved") {
		t.Fatalf("expected synopsis content, got %q", layers[2].Content)
	}
	if layers[3].ID != "trace_history" {
		t.Fatalf("expected trace_history layer fourth, got %q", layers[3].ID)
	}
	if !strings.Contains(layers[3].Content, "12 earlier timeline events compressed") {
		t.Fatalf("expected trace history content, got %q", layers[3].Content)
	}
	if meta, ok := layers[1].Metadata["inject_memory_recall"].(bool); !ok || !meta {
		t.Fatalf("expected trace_feedback_plan metadata to include inject_memory_recall=true, got %#v", layers[1].Metadata["inject_memory_recall"])
	}
	if layers[4].ID != "tool_guidance" {
		t.Fatalf("expected tool_guidance layer fifth, got %q", layers[4].ID)
	}
	if !strings.Contains(layers[4].Content, "Approval-required tools") {
		t.Fatalf("expected risk-layered tool guidance, got %q", layers[4].Content)
	}
	if layers[5].ID != "verification_gate" {
		t.Fatalf("expected verification_gate layer sixth, got %q", layers[5].ID)
	}
	if layers[6].ID != "context_pressure" {
		t.Fatalf("expected context_pressure layer seventh, got %q", layers[6].ID)
	}
}

func TestEphemeralPromptLayerBuilder_BuildSkipsEmptyInputs(t *testing.T) {
	builder := NewEphemeralPromptLayerBuilder()
	layers := builder.Build(EphemeralPromptLayerInput{})
	if layers != nil {
		t.Fatalf("expected nil layers, got %#v", layers)
	}
}
