package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type mockNeo4jClient struct {
	mu        sync.Mutex
	entities  map[string]*Entity
	relations map[string]*Relation
}

func newMockNeo4jClient() *mockNeo4jClient {
	return &mockNeo4jClient{
		entities:  make(map[string]*Entity),
		relations: make(map[string]*Relation),
	}
}

func (c *mockNeo4jClient) Execute(_ context.Context, query string, params map[string]any) ([]map[string]any, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch query {
	case neo4jQueryMergeEntity:
		props := map[string]any{}
		if raw, ok := params["properties_json"].(string); ok && raw != "" {
			if err := json.Unmarshal([]byte(raw), &props); err != nil {
				return nil, err
			}
		}
		ent := &Entity{
			ID:         params["id"].(string),
			Type:       params["type"].(string),
			Name:       params["name"].(string),
			Properties: props,
			CreatedAt:  params["created_at"].(time.Time),
			UpdatedAt:  params["updated_at"].(time.Time),
		}
		c.entities[ent.ID] = ent
		return nil, nil
	case neo4jQueryMergeRelation:
		props := map[string]any{}
		if raw, ok := params["properties_json"].(string); ok && raw != "" {
			if err := json.Unmarshal([]byte(raw), &props); err != nil {
				return nil, err
			}
		}
		rel := &Relation{
			ID:         params["id"].(string),
			FromID:     params["from_id"].(string),
			ToID:       params["to_id"].(string),
			Type:       params["type"].(string),
			Properties: props,
			Weight:     params["weight"].(float64),
			CreatedAt:  params["created_at"].(time.Time),
		}
		c.relations[rel.ID] = rel
		return nil, nil
	case neo4jQueryEntityByID:
		ent := c.entities[params["id"].(string)]
		if ent == nil {
			return []map[string]any{}, nil
		}
		return []map[string]any{{"entity": *ent}}, nil
	case neo4jQueryRelationsByEntity:
		entityID := params["entity_id"].(string)
		relationType, _ := params["type"].(string)
		var rows []map[string]any
		for _, rel := range c.relations {
			if rel.FromID != entityID && rel.ToID != entityID {
				continue
			}
			if relationType != "" && rel.Type != relationType {
				continue
			}
			rows = append(rows, map[string]any{"relation": *rel})
		}
		return rows, nil
	case neo4jQueryFindPath:
		fromID := params["from_id"].(string)
		toID := params["to_id"].(string)
		if _, ok := c.entities[fromID]; !ok {
			return nil, fmt.Errorf("entity %q not found", fromID)
		}
		if _, ok := c.entities[toID]; !ok {
			return nil, fmt.Errorf("entity %q not found", toID)
		}
		for _, rel := range c.relations {
			if rel.FromID == fromID && rel.ToID == toID {
				return []map[string]any{{"path": []string{fromID, toID}}}, nil
			}
		}
		return []map[string]any{}, nil
	default:
		return nil, fmt.Errorf("unexpected query: %s", query)
	}
}

func TestNeo4jKnowledgeGraph_PersistsAcrossGraphInstances(t *testing.T) {
	ctx := context.Background()
	client := newMockNeo4jClient()

	graph1, err := NewNeo4jKnowledgeGraph(client)
	require.NoError(t, err)
	now := time.Date(2026, 5, 12, 11, 0, 0, 0, time.UTC)
	require.NoError(t, graph1.AddEntity(ctx, &Entity{
		ID:         "user-1",
		Type:       "user",
		Name:       "Alice",
		Properties: map[string]any{"tier": "gold"},
		CreatedAt:  now,
	}))
	require.NoError(t, graph1.AddEntity(ctx, &Entity{ID: "topic-1", Type: "topic", Name: "Go", CreatedAt: now}))
	require.NoError(t, graph1.AddRelation(ctx, &Relation{
		ID:         "rel-1",
		FromID:     "user-1",
		ToID:       "topic-1",
		Type:       "likes",
		Properties: map[string]any{"source": "chat"},
		Weight:     0.9,
		CreatedAt:  now,
	}))

	graph2, err := NewNeo4jKnowledgeGraph(client)
	require.NoError(t, err)
	ent, err := graph2.QueryEntity(ctx, "user-1")
	require.NoError(t, err)
	require.Equal(t, "Alice", ent.Name)
	require.Equal(t, "gold", ent.Properties["tier"])

	relations, err := graph2.QueryRelations(ctx, "user-1", "likes")
	require.NoError(t, err)
	require.Len(t, relations, 1)
	require.Equal(t, "topic-1", relations[0].ToID)

	paths, err := graph2.FindPath(ctx, "user-1", "topic-1", 2)
	require.NoError(t, err)
	require.Equal(t, [][]string{{"user-1", "topic-1"}}, paths)
}
