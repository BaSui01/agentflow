package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Neo4jClient abstracts the small Cypher execution surface needed by semantic memory.
type Neo4jClient interface {
	Execute(ctx context.Context, query string, params map[string]any) ([]map[string]any, error)
}

const neo4jQueryMergeEntity = `
MERGE (e:MemoryEntity {id: $id})
SET e.type = $type,
    e.name = $name,
    e.properties_json = $properties_json,
    e.created_at = $created_at,
    e.updated_at = $updated_at
RETURN e`

const neo4jQueryMergeRelation = `
MATCH (from:MemoryEntity {id: $from_id})
MATCH (to:MemoryEntity {id: $to_id})
MERGE (from)-[r:MEMORY_RELATION {id: $id}]->(to)
SET r.type = $type,
    r.properties_json = $properties_json,
    r.weight = $weight,
    r.created_at = $created_at
RETURN r`

const neo4jQueryEntityByID = `
MATCH (e:MemoryEntity {id: $id})
RETURN e`

const neo4jQueryRelationsByEntity = `
MATCH (:MemoryEntity {id: $entity_id})-[r:MEMORY_RELATION]-(:MemoryEntity)
WHERE $type = '' OR r.type = $type
RETURN r`

const neo4jQueryFindPath = `
MATCH path = shortestPath((from:MemoryEntity {id: $from_id})-[*..$max_depth]-(to:MemoryEntity {id: $to_id}))
RETURN path`

// Neo4jKnowledgeGraph persists semantic memory in a Neo4j-compatible graph backend.
type Neo4jKnowledgeGraph struct {
	client Neo4jClient
}

// NewNeo4jKnowledgeGraph creates a Neo4j-backed KnowledgeGraph.
func NewNeo4jKnowledgeGraph(client Neo4jClient) (*Neo4jKnowledgeGraph, error) {
	if client == nil {
		return nil, fmt.Errorf("neo4j client is required")
	}
	return &Neo4jKnowledgeGraph{client: client}, nil
}

func (g *Neo4jKnowledgeGraph) AddEntity(ctx context.Context, entity *Entity) error {
	if entity == nil {
		return fmt.Errorf("entity is nil")
	}
	if entity.ID == "" {
		return fmt.Errorf("entity id is required")
	}
	now := time.Now()
	if entity.CreatedAt.IsZero() {
		entity.CreatedAt = now
	}
	entity.UpdatedAt = now

	props, err := json.Marshal(entity.Properties)
	if err != nil {
		return fmt.Errorf("marshal entity properties: %w", err)
	}
	_, err = g.client.Execute(ctx, neo4jQueryMergeEntity, map[string]any{
		"id":              entity.ID,
		"type":            entity.Type,
		"name":            entity.Name,
		"properties_json": string(props),
		"created_at":      entity.CreatedAt.UTC(),
		"updated_at":      entity.UpdatedAt.UTC(),
	})
	return err
}

func (g *Neo4jKnowledgeGraph) AddRelation(ctx context.Context, relation *Relation) error {
	if relation == nil {
		return fmt.Errorf("relation is nil")
	}
	if relation.ID == "" {
		relation.ID = fmt.Sprintf("rel_%d", time.Now().UnixNano())
	}
	if relation.FromID == "" || relation.ToID == "" {
		return fmt.Errorf("relation from_id and to_id are required")
	}
	if relation.CreatedAt.IsZero() {
		relation.CreatedAt = time.Now()
	}

	props, err := json.Marshal(relation.Properties)
	if err != nil {
		return fmt.Errorf("marshal relation properties: %w", err)
	}
	_, err = g.client.Execute(ctx, neo4jQueryMergeRelation, map[string]any{
		"id":              relation.ID,
		"from_id":         relation.FromID,
		"to_id":           relation.ToID,
		"type":            relation.Type,
		"properties_json": string(props),
		"weight":          relation.Weight,
		"created_at":      relation.CreatedAt.UTC(),
	})
	return err
}

func (g *Neo4jKnowledgeGraph) QueryEntity(ctx context.Context, id string) (*Entity, error) {
	if id == "" {
		return nil, fmt.Errorf("entity id is required")
	}
	rows, err := g.client.Execute(ctx, neo4jQueryEntityByID, map[string]any{"id": id})
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("entity %q not found", id)
	}
	entity, ok := rows[0]["entity"].(Entity)
	if ok {
		return &entity, nil
	}
	entityPtr, ok := rows[0]["entity"].(*Entity)
	if ok {
		copied := *entityPtr
		return &copied, nil
	}
	return nil, fmt.Errorf("unexpected entity row type")
}

func (g *Neo4jKnowledgeGraph) QueryRelations(ctx context.Context, entityID, relationType string) ([]Relation, error) {
	if entityID == "" {
		return nil, fmt.Errorf("entity id is required")
	}
	rows, err := g.client.Execute(ctx, neo4jQueryRelationsByEntity, map[string]any{
		"entity_id": entityID,
		"type":      relationType,
	})
	if err != nil {
		return nil, err
	}
	relations := make([]Relation, 0, len(rows))
	for _, row := range rows {
		switch rel := row["relation"].(type) {
		case Relation:
			relations = append(relations, rel)
		case *Relation:
			relations = append(relations, *rel)
		default:
			return nil, fmt.Errorf("unexpected relation row type")
		}
	}
	return relations, nil
}

func (g *Neo4jKnowledgeGraph) FindPath(ctx context.Context, fromID, toID string, maxDepth int) ([][]string, error) {
	if fromID == "" || toID == "" {
		return nil, fmt.Errorf("from_id and to_id are required")
	}
	if maxDepth <= 0 {
		return [][]string{}, nil
	}
	rows, err := g.client.Execute(ctx, neo4jQueryFindPath, map[string]any{
		"from_id":   fromID,
		"to_id":     toID,
		"max_depth": maxDepth,
	})
	if err != nil {
		return nil, err
	}
	paths := make([][]string, 0, len(rows))
	for _, row := range rows {
		path, ok := row["path"].([]string)
		if !ok {
			return nil, fmt.Errorf("unexpected path row type")
		}
		paths = append(paths, append([]string(nil), path...))
	}
	return paths, nil
}
