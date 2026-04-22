package agent

import (
	"context"
	"testing"

	agentcontext "github.com/BaSui01/agentflow/agent/execution/context"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

type memoryRuntimeBaseStub struct {
	saved    []MemoryRecord
	search   []MemoryRecord
	searchQ  string
	searchK  int
}

func (s *memoryRuntimeBaseStub) Save(_ context.Context, record MemoryRecord) error {
	s.saved = append(s.saved, record)
	return nil
}

func (s *memoryRuntimeBaseStub) Clear(context.Context, string, MemoryKind) error { return nil }

func (s *memoryRuntimeBaseStub) LoadRecent(context.Context, string, MemoryKind, int) ([]MemoryRecord, error) {
	return nil, nil
}

func (s *memoryRuntimeBaseStub) Search(_ context.Context, _ string, query string, topK int) ([]MemoryRecord, error) {
	s.searchQ = query
	s.searchK = topK
	return s.search, nil
}

func (s *memoryRuntimeBaseStub) Delete(context.Context, string) error { return nil }

func (s *memoryRuntimeBaseStub) Get(context.Context, string) (*MemoryRecord, error) { return nil, nil }

type memoryRuntimeEnhancedStub struct {
	saved       []struct {
		content  string
		metadata map[string]any
	}
	episodes []*types.EpisodicEvent
}

func (s *memoryRuntimeEnhancedStub) LoadWorking(context.Context, string) ([]types.MemoryEntry, error) {
	return nil, nil
}

func (s *memoryRuntimeEnhancedStub) LoadShortTerm(context.Context, string, int) ([]types.MemoryEntry, error) {
	return nil, nil
}

func (s *memoryRuntimeEnhancedStub) SaveShortTerm(_ context.Context, _ string, content string, metadata map[string]any) error {
	s.saved = append(s.saved, struct {
		content  string
		metadata map[string]any
	}{content: content, metadata: metadata})
	return nil
}

func (s *memoryRuntimeEnhancedStub) RecordEpisode(_ context.Context, event *types.EpisodicEvent) error {
	s.episodes = append(s.episodes, event)
	return nil
}

func TestUnifiedMemoryFacade_SaveInteraction_PrefersEnhancedMemory(t *testing.T) {
	base := &memoryRuntimeBaseStub{}
	enhanced := &memoryRuntimeEnhancedStub{}
	facade := NewUnifiedMemoryFacade(base, enhanced, zap.NewNop())

	facade.SaveInteraction(context.Background(), "agent-1", "trace-1", " user ", " assistant ")

	if len(base.saved) != 0 {
		t.Fatalf("expected base memory saves to be suppressed when enhanced memory exists, got %d", len(base.saved))
	}
	if len(enhanced.saved) != 2 {
		t.Fatalf("expected 2 enhanced saves, got %d", len(enhanced.saved))
	}
	if enhanced.saved[0].metadata["role"] != "user" || enhanced.saved[1].metadata["role"] != "assistant" {
		t.Fatalf("unexpected enhanced memory roles: %#v", enhanced.saved)
	}
}

func TestDefaultMemoryRuntime_ObserveTurn_SavesInteractionAndEpisode(t *testing.T) {
	base := &memoryRuntimeBaseStub{}
	enhanced := &memoryRuntimeEnhancedStub{}
	facade := NewUnifiedMemoryFacade(base, enhanced, zap.NewNop())
	rt := NewDefaultMemoryRuntime(func() *UnifiedMemoryFacade { return facade }, func() MemoryManager { return base }, zap.NewNop())

	err := rt.ObserveTurn(context.Background(), "agent-1", MemoryObservationInput{
		TraceID:          "trace-1",
		UserContent:      "hello",
		AssistantContent: "world",
		Metadata:         map[string]any{"k": "v"},
	})
	if err != nil {
		t.Fatalf("observe turn: %v", err)
	}

	if len(enhanced.saved) != 2 {
		t.Fatalf("expected 2 enhanced memory writes, got %d", len(enhanced.saved))
	}
	if len(enhanced.episodes) != 1 {
		t.Fatalf("expected 1 episode record, got %d", len(enhanced.episodes))
	}
	if enhanced.episodes[0].Context["trace_id"] != "trace-1" {
		t.Fatalf("expected trace_id to propagate into episode context")
	}
}

func TestDefaultMemoryRuntime_RecallForPrompt_SkipsAggressiveStatus(t *testing.T) {
	base := &memoryRuntimeBaseStub{
		search: []MemoryRecord{{Content: "important memory"}},
	}
	rt := NewDefaultMemoryRuntime(func() *UnifiedMemoryFacade { return nil }, func() MemoryManager { return base }, zap.NewNop())

	layers, err := rt.RecallForPrompt(context.Background(), "agent-1", MemoryRecallOptions{
		Query: "memory",
		Status: &agentcontext.Status{
			Level: agentcontext.LevelAggressive,
		},
	})
	if err != nil {
		t.Fatalf("recall for prompt: %v", err)
	}
	if len(layers) != 0 {
		t.Fatalf("expected no prompt layers under aggressive status, got %d", len(layers))
	}
	if base.searchQ != "" {
		t.Fatalf("expected search to be skipped under aggressive status")
	}
}
