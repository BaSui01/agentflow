package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	agentcontext "github.com/BaSui01/agentflow/agent/context"
)

// EphemeralPromptLayerBuilder builds request-scoped prompt layers that should
// not mutate the stable system prompt bundle.
type EphemeralPromptLayerBuilder struct{}

type EphemeralPromptLayerInput struct {
	PublicContext map[string]any
	CheckpointID  string
	ContextStatus *agentcontext.Status
}

func NewEphemeralPromptLayerBuilder() *EphemeralPromptLayerBuilder {
	return &EphemeralPromptLayerBuilder{}
}

func (b *EphemeralPromptLayerBuilder) Build(input EphemeralPromptLayerInput) []agentcontext.PromptLayer {
	layers := make([]agentcontext.PromptLayer, 0, 3)
	if layer := buildRequestContextLayer(input.PublicContext); layer != nil {
		layers = append(layers, *layer)
	}
	if layer := buildResumeContextLayer(input.CheckpointID); layer != nil {
		layers = append(layers, *layer)
	}
	if layer := buildContextPressureLayer(input.ContextStatus); layer != nil {
		layers = append(layers, *layer)
	}
	if len(layers) == 0 {
		return nil
	}
	return layers
}

func buildRequestContextLayer(values map[string]any) *agentcontext.PromptLayer {
	if len(values) == 0 {
		return nil
	}
	payload, err := json.Marshal(values)
	if err != nil || len(payload) == 0 {
		return nil
	}
	return &agentcontext.PromptLayer{
		ID:       "request_context",
		Type:     agentcontext.SegmentEphemeral,
		Content:  "<request_context>\n" + string(payload) + "\n</request_context>",
		Priority: 85,
		Sticky:   true,
		Metadata: map[string]any{"source": "input_context"},
	}
}

func buildResumeContextLayer(checkpointID string) *agentcontext.PromptLayer {
	checkpointID = strings.TrimSpace(checkpointID)
	if checkpointID == "" {
		return nil
	}
	return &agentcontext.PromptLayer{
		ID:       "resume_context",
		Type:     agentcontext.SegmentEphemeral,
		Content:  "<resume_context>\nContinue from the checkpointed run state instead of restarting from scratch. checkpoint_id=" + checkpointID + "\n</resume_context>",
		Priority: 90,
		Sticky:   true,
		Metadata: map[string]any{"checkpoint_id": checkpointID},
	}
}

func buildContextPressureLayer(status *agentcontext.Status) *agentcontext.PromptLayer {
	if status == nil || status.Level < agentcontext.LevelNormal {
		return nil
	}
	level := strings.ToLower(status.Level.String())
	usagePercent := 0
	if status.UsageRatio > 0 {
		usagePercent = int(status.UsageRatio * 100)
	}
	return &agentcontext.PromptLayer{
		ID:   "context_pressure",
		Type: agentcontext.SegmentEphemeral,
		Content: fmt.Sprintf(
			"<context_pressure>\nContext usage is at %d%% of the available budget (%s). Be concise, avoid repeating prior context, and focus on unresolved items only.\n</context_pressure>",
			usagePercent,
			level,
		),
		Priority: 75,
		Sticky:   false,
		Metadata: map[string]any{
			"usage_ratio":     status.UsageRatio,
			"level":           status.Level.String(),
			"recommendation":  status.Recommendation,
			"current_tokens":  status.CurrentTokens,
			"max_tokens":      status.MaxTokens,
			"ephemeral_layer": "context_pressure",
		},
	}
}
