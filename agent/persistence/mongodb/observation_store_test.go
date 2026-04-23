//go:build integration

package mongodb

import (
	"context"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/agent/capabilities/memory"
)

func TestMongoObservationStore_SaveAndLoadRecent(t *testing.T) {
	client := testMongoClient(t)
	ctx := context.Background()

	store, err := NewObservationStore(ctx, client)
	if err != nil {
		t.Fatalf("NewObservationStore: %v", err)
	}

	now := time.Now().UTC()
	obs1 := memory.Observation{ID: "obs-1", AgentID: "a1", Date: "2025-06-01", Content: "first", CreatedAt: now.Add(-2 * time.Hour)}
	obs2 := memory.Observation{ID: "obs-2", AgentID: "a1", Date: "2025-06-01", Content: "second", CreatedAt: now.Add(-1 * time.Hour)}
	obs3 := memory.Observation{ID: "obs-3", AgentID: "a2", Date: "2025-06-01", Content: "other", CreatedAt: now}

	if err := store.Save(ctx, obs1); err != nil {
		t.Fatalf("Save obs1: %v", err)
	}
	if err := store.Save(ctx, obs2); err != nil {
		t.Fatalf("Save obs2: %v", err)
	}
	if err := store.Save(ctx, obs3); err != nil {
		t.Fatalf("Save obs3: %v", err)
	}

	results, err := store.LoadRecent(ctx, "a1", 10)
	if err != nil {
		t.Fatalf("LoadRecent: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2, got %d", len(results))
	}
	if results[0].Content != "second" {
		t.Errorf("expected newest first, got %q", results[0].Content)
	}

	results, err = store.LoadRecent(ctx, "a1", 1)
	if err != nil {
		t.Fatalf("LoadRecent limit=1: %v", err)
	}
	if len(results) != 1 || results[0].Content != "second" {
		t.Errorf("unexpected: %+v", results)
	}
}

func TestMongoObservationStore_LoadByDateRange(t *testing.T) {
	client := testMongoClient(t)
	ctx := context.Background()

	store, err := NewObservationStore(ctx, client)
	if err != nil {
		t.Fatalf("NewObservationStore: %v", err)
	}

	base := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		_ = store.Save(ctx, memory.Observation{
			ID:        "dr-" + string(rune('a'+i)),
			AgentID:   "a-range",
			Date:      "2025-06-01",
			Content:   "event",
			CreatedAt: base.Add(time.Duration(i) * 24 * time.Hour),
		})
	}

	results, err := store.LoadByDateRange(ctx, "a-range", base, base.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("LoadByDateRange: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2, got %d", len(results))
	}
	if len(results) >= 2 && results[0].CreatedAt.After(results[1].CreatedAt) {
		t.Error("date range results not sorted ascending")
	}
}

func TestMongoObservationStore_Upsert(t *testing.T) {
	client := testMongoClient(t)
	ctx := context.Background()

	store, err := NewObservationStore(ctx, client)
	if err != nil {
		t.Fatalf("NewObservationStore: %v", err)
	}

	obs := memory.Observation{ID: "obs-upsert", AgentID: "a1", Date: "2025-06-01", Content: "original", CreatedAt: time.Now().UTC()}
	if err := store.Save(ctx, obs); err != nil {
		t.Fatalf("Save: %v", err)
	}

	obs.Content = "updated"
	if err := store.Save(ctx, obs); err != nil {
		t.Fatalf("Save upsert: %v", err)
	}

	results, err := store.LoadRecent(ctx, "a1", 10)
	if err != nil {
		t.Fatalf("LoadRecent: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 after upsert, got %d", len(results))
	}
	if results[0].Content != "updated" {
		t.Errorf("expected updated content, got %q", results[0].Content)
	}
}

func TestMongoObservationStore_Metadata(t *testing.T) {
	client := testMongoClient(t)
	ctx := context.Background()

	store, err := NewObservationStore(ctx, client)
	if err != nil {
		t.Fatalf("NewObservationStore: %v", err)
	}

	obs := memory.Observation{
		ID:        "obs-meta",
		AgentID:   "a1",
		Date:      "2025-06-01",
		Content:   "with metadata",
		CreatedAt: time.Now().UTC(),
		Metadata:  map[string]any{"source": "conversation", "turn_id": float64(42)},
	}
	if err := store.Save(ctx, obs); err != nil {
		t.Fatalf("Save: %v", err)
	}

	results, err := store.LoadRecent(ctx, "a1", 1)
	if err != nil {
		t.Fatalf("LoadRecent: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1, got %d", len(results))
	}
	if results[0].Metadata["source"] != "conversation" {
		t.Errorf("metadata source = %v", results[0].Metadata["source"])
	}
}
