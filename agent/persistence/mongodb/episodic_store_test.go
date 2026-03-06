//go:build integration

package mongodb

import (
	"context"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/agent/memory"
	"github.com/BaSui01/agentflow/types"
)

func TestMongoEpisodicStore_RecordAndQuery(t *testing.T) {
	client := testMongoClient(t)
	ctx := context.Background()

	store, err := NewEpisodicStore(ctx, client)
	if err != nil {
		t.Fatalf("NewEpisodicStore: %v", err)
	}

	now := time.Now()
	event := &types.EpisodicEvent{
		ID:        "ep-test-1",
		AgentID:   "agent-1",
		Type:      "task_execution",
		Content:   "executed task A",
		Timestamp: now,
		Duration:  2 * time.Second,
		Context:   map[string]any{"trace_id": "t1"},
	}

	if err := store.RecordEvent(ctx, event); err != nil {
		t.Fatalf("RecordEvent: %v", err)
	}

	// Query by agent ID.
	events, err := store.QueryEvents(ctx, memory.EpisodicQuery{
		AgentID: "agent-1",
		Limit:   10,
	})
	if err != nil {
		t.Fatalf("QueryEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].ID != "ep-test-1" {
		t.Errorf("event ID = %q, want %q", events[0].ID, "ep-test-1")
	}
}

func TestMongoEpisodicStore_IdempotentInsert(t *testing.T) {
	client := testMongoClient(t)
	ctx := context.Background()

	store, err := NewEpisodicStore(ctx, client)
	if err != nil {
		t.Fatalf("NewEpisodicStore: %v", err)
	}

	event := &types.EpisodicEvent{
		ID:      "ep-dup-1",
		AgentID: "agent-1",
		Type:    "task_execution",
		Content: "duplicate test",
	}

	if err := store.RecordEvent(ctx, event); err != nil {
		t.Fatalf("first RecordEvent: %v", err)
	}
	// Second insert with same ID should succeed (idempotent).
	if err := store.RecordEvent(ctx, event); err != nil {
		t.Fatalf("duplicate RecordEvent should be idempotent: %v", err)
	}
}

func TestMongoEpisodicStore_GetTimeline(t *testing.T) {
	client := testMongoClient(t)
	ctx := context.Background()

	store, err := NewEpisodicStore(ctx, client)
	if err != nil {
		t.Fatalf("NewEpisodicStore: %v", err)
	}

	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		_ = store.RecordEvent(ctx, &types.EpisodicEvent{
			ID:        "tl-" + string(rune('a'+i)),
			AgentID:   "agent-tl",
			Type:      "task_execution",
			Content:   "event",
			Timestamp: base.Add(time.Duration(i) * time.Hour),
		})
	}

	events, err := store.GetTimeline(ctx, "agent-tl", base, base.Add(3*time.Hour))
	if err != nil {
		t.Fatalf("GetTimeline: %v", err)
	}
	if len(events) != 3 {
		t.Errorf("expected 3 timeline events, got %d", len(events))
	}
	// Timeline should be sorted ascending.
	if len(events) >= 2 && events[0].Timestamp.After(events[1].Timestamp) {
		t.Error("timeline events not sorted ascending")
	}
}

