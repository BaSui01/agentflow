package agent

import (
	"context"
	"strings"
	"time"

	agentcontext "github.com/BaSui01/agentflow/agent/context"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

type DefaultMemoryRuntime struct {
	facadeProvider func() *UnifiedMemoryFacade
	baseProvider   func() MemoryManager
	logger         *zap.Logger
}

func NewDefaultMemoryRuntime(facadeProvider func() *UnifiedMemoryFacade, baseProvider func() MemoryManager, logger *zap.Logger) *DefaultMemoryRuntime {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &DefaultMemoryRuntime{
		facadeProvider: facadeProvider,
		baseProvider:   baseProvider,
		logger:         logger,
	}
}

func (r *DefaultMemoryRuntime) RecallForPrompt(ctx context.Context, agentID string, opts MemoryRecallOptions) ([]agentcontext.PromptLayer, error) {
	base := r.base()
	if base == nil || strings.TrimSpace(opts.Query) == "" {
		return nil, nil
	}
	if opts.Status != nil && (opts.Status.Level == agentcontext.LevelAggressive || opts.Status.Level == agentcontext.LevelEmergency) {
		return nil, nil
	}
	topK := opts.TopK
	if topK <= 0 {
		topK = 3
	}
	records, err := base.Search(ctx, agentID, opts.Query, topK)
	if err != nil {
		return nil, err
	}
	lines := make([]string, 0, len(records))
	for _, rec := range records {
		if trimmed := strings.TrimSpace(rec.Content); trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	lines = normalizeStringSlice(lines)
	if len(lines) == 0 {
		return nil, nil
	}
	layer := agentcontext.PromptLayer{
		ID:       "memory_recall",
		Type:     agentcontext.SegmentEphemeral,
		Priority: 86,
		Sticky:   false,
		Content:  "<memory_recall>\nRelevant memories for this request:\n- " + strings.Join(lines, "\n- ") + "\n</memory_recall>",
		Metadata: map[string]any{
			"layer_kind":   "memory_recall",
			"query":        opts.Query,
			"recall_count": len(lines),
		},
	}
	return []agentcontext.PromptLayer{layer}, nil
}

func (r *DefaultMemoryRuntime) ObserveTurn(ctx context.Context, agentID string, turn MemoryObservationInput) error {
	facade := r.facade()
	if facade == nil {
		return nil
	}
	facade.SaveInteraction(ctx, agentID, turn.TraceID, turn.UserContent, turn.AssistantContent)
	if strings.TrimSpace(turn.UserContent) == "" && strings.TrimSpace(turn.AssistantContent) == "" {
		return nil
	}
	facade.RecordEpisode(ctx, &types.EpisodicEvent{
		ID:        agentID + "-" + turn.TraceID + "-turn",
		AgentID:   agentID,
		Type:      "turn_execution",
		Content:   strings.TrimSpace(turn.AssistantContent),
		Timestamp: time.Now(),
		Context: map[string]any{
			"trace_id":         turn.TraceID,
			"user_content":     turn.UserContent,
			"assistant_output": turn.AssistantContent,
			"metadata":         cloneAnyMap(turn.Metadata),
		},
	})
	return nil
}

func (r *DefaultMemoryRuntime) facade() *UnifiedMemoryFacade {
	if r.facadeProvider == nil {
		return nil
	}
	return r.facadeProvider()
}

func (r *DefaultMemoryRuntime) base() MemoryManager {
	if r.baseProvider == nil {
		return nil
	}
	return r.baseProvider()
}
