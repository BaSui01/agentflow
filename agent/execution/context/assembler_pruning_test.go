package context

import (
	"context"
	"strings"
	"testing"

	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestAssemblerDropsLowerPrioritySegmentsBeforeStickyInput(t *testing.T) {
	cfg := DefaultAgentContextConfig("unknown")
	cfg.MaxContextTokens = 90
	cfg.ReserveForOutput = 0
	cfg.KeepLastN = 1
	cfg.EnableSummarize = false
	mgr := NewAgentContextManager(cfg, zap.NewNop())

	result, err := mgr.Assemble(context.Background(), &AssembleRequest{
		SystemPrompt:  "system stays",
		MemoryContext: []string{strings.Repeat("memory ", 80)},
		Retrieval:     []RetrievalItem{{Title: "doc", Content: strings.Repeat("retrieval ", 80), Source: "kb", Score: 0.9}},
		ToolState:     []ToolState{{ToolName: "shell", Summary: strings.Repeat("toolstate ", 80), ArtifactID: "artifact-1"}},
		Conversation: []types.Message{
			{Role: types.RoleUser, Content: strings.Repeat("old conversation ", 80)},
			{Role: types.RoleAssistant, Content: "latest answer stays"},
		},
		UserInput: "current question stays",
		Query:     "current question stays",
	})
	require.NoError(t, err)

	keptByID := segmentIDs(result.SegmentsKept)
	droppedByID := segmentIDs(result.SegmentsDropped)
	assert.Contains(t, keptByID, "system")
	assert.Contains(t, keptByID, "conversation-1")
	assert.Contains(t, keptByID, "input")
	assert.Contains(t, droppedByID, "retrieval-0")
	assert.Contains(t, droppedByID, "tool-0")
	assert.Contains(t, droppedByID, "memory-0")
	assert.Equal(t, "drop_conversation", result.Plan.CompressionReason)
}

func TestAssemblerAppliesPromptLayerDefaultsAndMetadataClone(t *testing.T) {
	mgr := NewAgentContextManager(DefaultAgentContextConfig("gpt-4o"), zap.NewNop())
	metadata := map[string]any{"source": "test"}

	result, err := mgr.Assemble(context.Background(), &AssembleRequest{
		EphemeralLayers: []PromptLayer{{ID: "hint", Content: "remember constraints", Metadata: metadata}},
		SkillContext:    []string{"skill instructions"},
		UserInput:       "question",
	})
	require.NoError(t, err)

	require.NotEmpty(t, result.Plan.AppliedLayers)
	layer := findLayer(result.Plan.AppliedLayers, "hint")
	require.NotNil(t, layer)
	assert.Equal(t, SegmentEphemeral, layer.Type)
	assert.Equal(t, 80, layer.Priority)
	assert.False(t, layer.Sticky)
	layer.Metadata["source"] = "mutated"
	assert.Equal(t, "test", metadata["source"])

	assert.NotNil(t, findLayer(result.Plan.AppliedLayers, "skill-0"))
}

func segmentIDs(segments []ContextSegment) map[string]bool {
	ids := make(map[string]bool, len(segments))
	for _, segment := range segments {
		ids[segment.ID] = true
	}
	return ids
}

func findLayer(layers []PromptLayerMeta, id string) *PromptLayerMeta {
	for i := range layers {
		if layers[i].ID == id {
			return &layers[i]
		}
	}
	return nil
}
