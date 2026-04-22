package agent

import (
	"context"
	"strings"
	"time"

	agentcontext "github.com/BaSui01/agentflow/agent/execution/context"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// UnifiedMemoryFacade wraps a base MemoryManager and an optional
// EnhancedMemoryRunner, providing a single save/load surface.
// When enhanced memory is present, saves go through the enhanced system
// and base memory writes are suppressed, eliminating the _skip_base_memory hack.
type UnifiedMemoryFacade struct {
	base     MemoryManager
	enhanced EnhancedMemoryRunner
	logger   *zap.Logger
}

// NewUnifiedMemoryFacade creates a facade. Both base and enhanced are optional.
func NewUnifiedMemoryFacade(base MemoryManager, enhanced EnhancedMemoryRunner, logger *zap.Logger) *UnifiedMemoryFacade {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &UnifiedMemoryFacade{base: base, enhanced: enhanced, logger: logger}
}

// HasBase returns true if a base MemoryManager is configured.
func (f *UnifiedMemoryFacade) HasBase() bool { return f.base != nil }

// HasEnhanced returns true if enhanced memory is configured.
func (f *UnifiedMemoryFacade) HasEnhanced() bool { return f.enhanced != nil }

// Base returns the underlying MemoryManager (may be nil).
func (f *UnifiedMemoryFacade) Base() MemoryManager { return f.base }

// Enhanced returns the underlying EnhancedMemoryRunner (may be nil).
func (f *UnifiedMemoryFacade) Enhanced() EnhancedMemoryRunner { return f.enhanced }

// SaveInteraction saves both user input and agent output to the appropriate
// memory system. If enhanced memory is active it takes ownership;
// otherwise the base MemoryManager is used.
func (f *UnifiedMemoryFacade) SaveInteraction(ctx context.Context, agentID, traceID, userContent, agentContent string) {
	if f.enhanced != nil {
		if strings.TrimSpace(userContent) != "" {
			if err := f.enhanced.SaveShortTerm(ctx, agentID, userContent, map[string]any{"trace_id": traceID, "role": "user"}); err != nil {
				f.logger.Warn("enhanced memory save (user) failed", zap.Error(err))
			}
		}
		if strings.TrimSpace(agentContent) != "" {
			if err := f.enhanced.SaveShortTerm(ctx, agentID, agentContent, map[string]any{"trace_id": traceID, "role": "assistant"}); err != nil {
				f.logger.Warn("enhanced memory save (assistant) failed", zap.Error(err))
			}
		}
		return
	}

	if f.base != nil {
		userRec := MemoryRecord{
			AgentID:  agentID,
			Kind:     MemoryShortTerm,
			Content:  userContent,
			Metadata: map[string]any{"trace_id": traceID, "role": "user"},
		}
		if err := f.base.Save(ctx, userRec); err != nil {
			f.logger.Warn("base memory save (user) failed", zap.Error(err))
		}
		agentRec := MemoryRecord{
			AgentID:  agentID,
			Kind:     MemoryShortTerm,
			Content:  agentContent,
			Metadata: map[string]any{"trace_id": traceID, "role": "assistant"},
		}
		if err := f.base.Save(ctx, agentRec); err != nil {
			f.logger.Warn("base memory save (agent) failed", zap.Error(err))
		}
	}
}

// LoadContext loads contextual memory for enrichment. Enhanced memory
// is preferred if available.
func (f *UnifiedMemoryFacade) LoadContext(ctx context.Context, agentID string) []string {
	if f.enhanced != nil {
		return f.loadEnhancedContext(ctx, agentID)
	}
	return nil
}

func (f *UnifiedMemoryFacade) loadEnhancedContext(ctx context.Context, agentID string) []string {
	var context []string

	working, err := f.enhanced.LoadWorking(ctx, agentID)
	if err != nil {
		f.logger.Warn("failed to load working memory", zap.Error(err))
	} else {
		for _, entry := range working {
			if entry.Content != "" {
				context = append(context, entry.Content)
			}
		}
	}

	shortTerm, err := f.enhanced.LoadShortTerm(ctx, agentID, 5)
	if err != nil {
		f.logger.Warn("failed to load short-term memory", zap.Error(err))
	} else {
		for _, entry := range shortTerm {
			if entry.Content != "" {
				context = append(context, entry.Content)
			}
		}
	}

	return context
}

// RecordEpisode records an episodic event via enhanced memory (if available).
func (f *UnifiedMemoryFacade) RecordEpisode(ctx context.Context, event *types.EpisodicEvent) {
	if f.enhanced == nil {
		return
	}
	if err := f.enhanced.RecordEpisode(ctx, event); err != nil {
		f.logger.Warn("failed to record episode", zap.Error(err))
	}
}

// SkipBaseMemory returns true if enhanced memory is active and
// base memory saves should be suppressed.
func (f *UnifiedMemoryFacade) SkipBaseMemory() bool {
	return f.enhanced != nil
}

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
