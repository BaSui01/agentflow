//go:build integration

package mongodb

import (
	"context"
	"testing"

	"github.com/BaSui01/agentflow/agent/memory"
)

func TestMongoKnowledgeGraph_AddAndQueryEntity(t *testing.T) {
	client := testMongoClient(t)
	ctx := context.Background()

	kg, err := NewKnowledgeGraph(ctx, client)
	if err != nil {
		t.Fatalf("NewKnowledgeGraph: %v", err)
	}

	entity := &memory.Entity{
		ID:   "ent-1",
		Type: "person",
		Name: "Alice",
		Properties: map[string]any{
			"role": "engineer",
		},
	}

	if err := kg.AddEntity(ctx, entity); err != nil {
		t.Fatalf("AddEntity: %v", err)
	}

	got, err := kg.QueryEntity(ctx, "ent-1")
	if err != nil {
		t.Fatalf("QueryEntity: %v", err)
	}
	if got.Name != "Alice" {
		t.Errorf("Name = %q, want %q", got.Name, "Alice")
	}
	if got.Type != "person" {
		t.Errorf("Type = %q, want %q", got.Type, "person")
	}
}

func TestMongoKnowledgeGraph_AddAndQueryRelation(t *testing.T) {
	client := testMongoClient(t)
	ctx := context.Background()

	kg, err := NewKnowledgeGraph(ctx, client)
	if err != nil {
		t.Fatalf("NewKnowledgeGraph: %v", err)
	}

	_ = kg.AddEntity(ctx, &memory.Entity{ID: "rel-a", Type: "person", Name: "A"})
	_ = kg.AddEntity(ctx, &memory.Entity{ID: "rel-b", Type: "person", Name: "B"})

	rel := &memory.Relation{
		ID:     "r1",
		FromID: "rel-a",
		ToID:   "rel-b",
		Type:   "knows",
		Weight: 1.0,
	}
	if err := kg.AddRelation(ctx, rel); err != nil {
		t.Fatalf("AddRelation: %v", err)
	}

	rels, err := kg.QueryRelations(ctx, "rel-a", "knows")
	if err != nil {
		t.Fatalf("QueryRelations: %v", err)
	}
	if len(rels) != 1 {
		t.Fatalf("expected 1 relation, got %d", len(rels))
	}
	if rels[0].ToID != "rel-b" {
		t.Errorf("ToID = %q, want %q", rels[0].ToID, "rel-b")
	}
}

func TestMongoKnowledgeGraph_FindPath(t *testing.T) {
	client := testMongoClient(t)
	ctx := context.Background()

	kg, err := NewKnowledgeGraph(ctx, client)
	if err != nil {
		t.Fatalf("NewKnowledgeGraph: %v", err)
	}

	// Create a chain: A -> B -> C
	_ = kg.AddEntity(ctx, &memory.Entity{ID: "fp-a", Type: "node", Name: "A"})
	_ = kg.AddEntity(ctx, &memory.Entity{ID: "fp-b", Type: "node", Name: "B"})
	_ = kg.AddEntity(ctx, &memory.Entity{ID: "fp-c", Type: "node", Name: "C"})

	_ = kg.AddRelation(ctx, &memory.Relation{ID: "fp-r1", FromID: "fp-a", ToID: "fp-b", Type: "link"})
	_ = kg.AddRelation(ctx, &memory.Relation{ID: "fp-r2", FromID: "fp-b", ToID: "fp-c", Type: "link"})

	paths, err := kg.FindPath(ctx, "fp-a", "fp-c", 3)
	if err != nil {
		t.Fatalf("FindPath: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("expected at least one path, got none")
	}

	// The path should be [fp-a, fp-b, fp-c].
	found := false
	for _, p := range paths {
		if len(p) == 3 && p[0] == "fp-a" && p[1] == "fp-b" && p[2] == "fp-c" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected path [fp-a fp-b fp-c], got %v", paths)
	}
}

func TestMongoKnowledgeGraph_EntityNotFound(t *testing.T) {
	client := testMongoClient(t)
	ctx := context.Background()

	kg, err := NewKnowledgeGraph(ctx, client)
	if err != nil {
		t.Fatalf("NewKnowledgeGraph: %v", err)
	}

	_, err = kg.QueryEntity(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent entity, got nil")
	}
}
