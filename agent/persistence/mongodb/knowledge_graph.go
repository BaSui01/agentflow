package mongodb

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/BaSui01/agentflow/agent/memory"
	mongoclient "github.com/BaSui01/agentflow/pkg/mongodb"
)

// Collection names for knowledge graph.
const (
	kgEntitiesCollection  = "kg_entities"
	kgRelationsCollection = "kg_relations"
)

// entityDocument is the MongoDB document for a knowledge graph entity.
type entityDocument struct {
	ID         string         `bson:"_id"        json:"id"`
	Type       string         `bson:"type"       json:"type"`
	Name       string         `bson:"name"       json:"name"`
	Properties map[string]any `bson:"properties" json:"properties,omitempty"`
	CreatedAt  time.Time      `bson:"created_at" json:"created_at"`
	UpdatedAt  time.Time      `bson:"updated_at" json:"updated_at"`
}

// relationDocument is the MongoDB document for a knowledge graph relation.
type relationDocument struct {
	ID         string         `bson:"_id"        json:"id"`
	FromID     string         `bson:"from_id"    json:"from_id"`
	ToID       string         `bson:"to_id"      json:"to_id"`
	Type       string         `bson:"type"       json:"type"`
	Properties map[string]any `bson:"properties" json:"properties,omitempty"`
	Weight     float64        `bson:"weight"     json:"weight"`
	CreatedAt  time.Time      `bson:"created_at" json:"created_at"`
}

// MongoKnowledgeGraph implements memory.KnowledgeGraph backed by MongoDB.
type MongoKnowledgeGraph struct {
	entities  *mongo.Collection
	relations *mongo.Collection
}

// NewKnowledgeGraph creates a MongoKnowledgeGraph and ensures indexes.
func NewKnowledgeGraph(ctx context.Context, client *mongoclient.Client) (*MongoKnowledgeGraph, error) {
	g := &MongoKnowledgeGraph{
		entities:  client.Collection(kgEntitiesCollection),
		relations: client.Collection(kgRelationsCollection),
	}

	entIndexes := []mongo.IndexModel{
		{Keys: bson.D{{Key: "type", Value: 1}}},
		{Keys: bson.D{{Key: "name", Value: 1}}},
	}
	if err := client.EnsureIndexes(ctx, kgEntitiesCollection, entIndexes); err != nil {
		return nil, err
	}

	relIndexes := []mongo.IndexModel{
		{Keys: bson.D{{Key: "from_id", Value: 1}, {Key: "type", Value: 1}}},
		{Keys: bson.D{{Key: "to_id", Value: 1}, {Key: "type", Value: 1}}},
	}
	if err := client.EnsureIndexes(ctx, kgRelationsCollection, relIndexes); err != nil {
		return nil, err
	}

	return g, nil
}

func (g *MongoKnowledgeGraph) AddEntity(ctx context.Context, entity *memory.Entity) error {
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

	doc := &entityDocument{
		ID:         entity.ID,
		Type:       entity.Type,
		Name:       entity.Name,
		Properties: entity.Properties,
		CreatedAt:  entity.CreatedAt,
		UpdatedAt:  entity.UpdatedAt,
	}

	opts := options.Replace().SetUpsert(true)
	_, err := g.entities.ReplaceOne(ctx, bson.D{{Key: "_id", Value: entity.ID}}, doc, opts)
	return err
}

func (g *MongoKnowledgeGraph) AddRelation(ctx context.Context, relation *memory.Relation) error {
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

	doc := &relationDocument{
		ID:         relation.ID,
		FromID:     relation.FromID,
		ToID:       relation.ToID,
		Type:       relation.Type,
		Properties: relation.Properties,
		Weight:     relation.Weight,
		CreatedAt:  relation.CreatedAt,
	}

	opts := options.Replace().SetUpsert(true)
	_, err := g.relations.ReplaceOne(ctx, bson.D{{Key: "_id", Value: relation.ID}}, doc, opts)
	return err
}

func (g *MongoKnowledgeGraph) QueryEntity(ctx context.Context, id string) (*memory.Entity, error) {
	if id == "" {
		return nil, fmt.Errorf("entity id is required")
	}

	var doc entityDocument
	err := g.entities.FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&doc)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("entity %q not found", id)
		}
		return nil, err
	}

	return &memory.Entity{
		ID:         doc.ID,
		Type:       doc.Type,
		Name:       doc.Name,
		Properties: doc.Properties,
		CreatedAt:  doc.CreatedAt,
		UpdatedAt:  doc.UpdatedAt,
	}, nil
}

func (g *MongoKnowledgeGraph) QueryRelations(ctx context.Context, entityID string, relationType string) ([]memory.Relation, error) {
	if entityID == "" {
		return nil, fmt.Errorf("entity id is required")
	}

	// Match relations where the entity is either the source or target.
	filter := bson.D{
		{Key: "$or", Value: bson.A{
			bson.D{{Key: "from_id", Value: entityID}},
			bson.D{{Key: "to_id", Value: entityID}},
		}},
	}
	if relationType != "" {
		filter = append(filter, bson.E{Key: "type", Value: relationType})
	}

	cursor, err := g.relations.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var docs []relationDocument
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, err
	}

	results := make([]memory.Relation, 0, len(docs))
	for _, doc := range docs {
		results = append(results, memory.Relation{
			ID:         doc.ID,
			FromID:     doc.FromID,
			ToID:       doc.ToID,
			Type:       doc.Type,
			Properties: doc.Properties,
			Weight:     doc.Weight,
			CreatedAt:  doc.CreatedAt,
		})
	}
	return results, nil
}

// FindPath finds paths between two entities using BFS up to maxDepth.
// It queries the relations collection iteratively, treating the graph as bidirectional
// (matching the in-memory implementation behavior).
func (g *MongoKnowledgeGraph) FindPath(ctx context.Context, fromID, toID string, maxDepth int) ([][]string, error) {
	if fromID == "" || toID == "" {
		return nil, fmt.Errorf("from_id and to_id are required")
	}
	if maxDepth <= 0 {
		return [][]string{}, nil
	}

	// Verify both entities exist.
	if err := g.entityExists(ctx, fromID); err != nil {
		return nil, err
	}
	if err := g.entityExists(ctx, toID); err != nil {
		return nil, err
	}

	// Load all relations into memory for path finding.
	// For large graphs, this could be optimized with $graphLookup on a
	// denormalized collection, but this matches the in-memory behavior.
	adjOut, adjIn, err := g.loadAdjacency(ctx)
	if err != nil {
		return nil, err
	}

	// BFS/DFS to find paths (matching in-memory DFS behavior).
	var paths [][]string
	visited := make(map[string]bool)
	g.dfs(ctx, fromID, toID, maxDepth, visited, []string{fromID}, &paths, adjOut, adjIn)

	return paths, nil
}

// entityExists checks if an entity exists in the entities collection.
func (g *MongoKnowledgeGraph) entityExists(ctx context.Context, id string) error {
	count, err := g.entities.CountDocuments(ctx, bson.D{{Key: "_id", Value: id}})
	if err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("entity %q not found", id)
	}
	return nil
}

// loadAdjacency loads all relations and builds adjacency lists.
func (g *MongoKnowledgeGraph) loadAdjacency(ctx context.Context) (outAdj, inAdj map[string][]string, err error) {
	cursor, err := g.relations.Find(ctx, bson.D{})
	if err != nil {
		return nil, nil, err
	}
	defer cursor.Close(ctx)

	outAdj = make(map[string][]string)
	inAdj = make(map[string][]string)

	var docs []relationDocument
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, nil, err
	}

	for _, doc := range docs {
		outAdj[doc.FromID] = append(outAdj[doc.FromID], doc.ToID)
		inAdj[doc.ToID] = append(inAdj[doc.ToID], doc.FromID)
	}
	return outAdj, inAdj, nil
}

// dfs performs depth-first search for paths (mirrors in-memory implementation).
func (g *MongoKnowledgeGraph) dfs(
	ctx context.Context,
	current, target string,
	depth int,
	visited map[string]bool,
	path []string,
	paths *[][]string,
	outAdj, inAdj map[string][]string,
) {
	if ctx.Err() != nil {
		return
	}
	if current == target && len(path) > 1 {
		found := make([]string, len(path))
		copy(found, path)
		*paths = append(*paths, found)
		return
	}
	if depth <= 0 {
		return
	}

	visited[current] = true
	defer func() { visited[current] = false }()

	// Traverse outgoing edges.
	for _, next := range outAdj[current] {
		if visited[next] {
			continue
		}
		g.dfs(ctx, next, target, depth-1, visited, append(path, next), paths, outAdj, inAdj)
	}

	// Traverse incoming edges (bidirectional search).
	for _, next := range inAdj[current] {
		if visited[next] {
			continue
		}
		g.dfs(ctx, next, target, depth-1, visited, append(path, next), paths, outAdj, inAdj)
	}
}

// Compile-time interface check.
var _ memory.KnowledgeGraph = (*MongoKnowledgeGraph)(nil)

