// Package mongodb implements MongoDB-backed persistence stores for agent data.
package mongodb

import (
	"context"
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/types"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	agent "github.com/BaSui01/agentflow/agent/execution/runtime"
	"github.com/BaSui01/agentflow/agent/persistence"
	mongoclient "github.com/BaSui01/agentflow/pkg/mongodb"
)

// Collection name for agent prompts.
const promptsCollection = "agent_prompts"

// PromptDocument is the MongoDB document schema for a PromptBundle.
type PromptDocument struct {
	ID        string    `bson:"_id"        json:"id"`
	AgentType string    `bson:"agent_type" json:"agent_type"`
	Name      string    `bson:"name"       json:"name"`
	Version   string    `bson:"version"    json:"version"`
	TenantID  string    `bson:"tenant_id"  json:"tenant_id"`
	IsActive  bool      `bson:"is_active"  json:"is_active"`
	CreatedAt time.Time `bson:"created_at" json:"created_at"`
	UpdatedAt time.Time `bson:"updated_at" json:"updated_at"`

	// Embedded PromptBundle fields.
	System      agent.SystemPrompt `bson:"system"      json:"system"`
	Tools       []types.ToolSchema `bson:"tools"       json:"tools,omitempty"`
	Examples    []agent.Example    `bson:"examples"    json:"examples,omitempty"`
	Constraints []string           `bson:"constraints" json:"constraints,omitempty"`
}

// PromptStore defines CRUD operations for agent prompt bundles.
type PromptStore interface {
	// Create inserts a new prompt document.
	Create(ctx context.Context, doc *PromptDocument) error
	// GetByID retrieves a prompt by its ID.
	GetByID(ctx context.Context, id string) (*PromptDocument, error)
	// Update replaces an existing prompt document.
	Update(ctx context.Context, doc *PromptDocument) error
	// Delete removes a prompt by ID.
	Delete(ctx context.Context, id string) error
	// ListByAgentType returns prompts matching the given agent type and tenant.
	ListByAgentType(ctx context.Context, agentType, tenantID string, limit, offset int) ([]*PromptDocument, error)
	// GetActive returns the active prompt for a given agent type, name, and tenant.
	GetActive(ctx context.Context, agentType, name, tenantID string) (*PromptDocument, error)
	// SetActive marks a specific version as active and deactivates others.
	SetActive(ctx context.Context, id string) error
}

// MongoPromptStore implements PromptStore backed by MongoDB.
type MongoPromptStore struct {
	coll *mongo.Collection
}

// NewPromptStore creates a MongoPromptStore and ensures indexes.
func NewPromptStore(ctx context.Context, client *mongoclient.Client) (*MongoPromptStore, error) {
	coll := client.Collection(promptsCollection)

	indexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "agent_type", Value: 1}, {Key: "name", Value: 1}, {Key: "version", Value: 1}, {Key: "tenant_id", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.D{{Key: "agent_type", Value: 1}, {Key: "tenant_id", Value: 1}, {Key: "is_active", Value: 1}},
		},
	}
	if err := client.EnsureIndexes(ctx, promptsCollection, indexes); err != nil {
		return nil, err
	}

	return &MongoPromptStore{coll: coll}, nil
}

func (s *MongoPromptStore) Create(ctx context.Context, doc *PromptDocument) error {
	if doc.ID == "" {
		return persistence.ErrInvalidInput
	}
	now := time.Now()
	doc.CreatedAt = now
	doc.UpdatedAt = now

	_, err := s.coll.InsertOne(ctx, doc)
	if mongo.IsDuplicateKeyError(err) {
		return persistence.ErrAlreadyExists
	}
	return err
}

func (s *MongoPromptStore) GetByID(ctx context.Context, id string) (*PromptDocument, error) {
	var doc PromptDocument
	err := s.coll.FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&doc)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, persistence.ErrNotFound
		}
		return nil, err
	}
	return &doc, nil
}

func (s *MongoPromptStore) Update(ctx context.Context, doc *PromptDocument) error {
	doc.UpdatedAt = time.Now()
	result, err := s.coll.ReplaceOne(ctx, bson.D{{Key: "_id", Value: doc.ID}}, doc)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return persistence.ErrNotFound
	}
	return nil
}

func (s *MongoPromptStore) Delete(ctx context.Context, id string) error {
	result, err := s.coll.DeleteOne(ctx, bson.D{{Key: "_id", Value: id}})
	if err != nil {
		return err
	}
	if result.DeletedCount == 0 {
		return persistence.ErrNotFound
	}
	return nil
}

func (s *MongoPromptStore) ListByAgentType(ctx context.Context, agentType, tenantID string, limit, offset int) ([]*PromptDocument, error) {
	filter := bson.D{{Key: "agent_type", Value: agentType}}
	if tenantID != "" {
		filter = append(filter, bson.E{Key: "tenant_id", Value: tenantID})
	}

	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}})
	if limit > 0 {
		opts.SetLimit(int64(limit))
	}
	if offset > 0 {
		opts.SetSkip(int64(offset))
	}

	cursor, err := s.coll.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var docs []*PromptDocument
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, err
	}
	return docs, nil
}

func (s *MongoPromptStore) GetActive(ctx context.Context, agentType, name, tenantID string) (*PromptDocument, error) {
	filter := bson.D{
		{Key: "agent_type", Value: agentType},
		{Key: "name", Value: name},
		{Key: "is_active", Value: true},
	}
	if tenantID != "" {
		filter = append(filter, bson.E{Key: "tenant_id", Value: tenantID})
	}

	var doc PromptDocument
	err := s.coll.FindOne(ctx, filter).Decode(&doc)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, persistence.ErrNotFound
		}
		return nil, err
	}
	return &doc, nil
}

func (s *MongoPromptStore) SetActive(ctx context.Context, id string) error {
	// Fetch the target document to know its agent_type + name + tenant_id.
	doc, err := s.GetByID(ctx, id)
	if err != nil {
		return err
	}

	// Deactivate all versions with the same agent_type + name + tenant_id.
	_, err = s.coll.UpdateMany(ctx,
		bson.D{
			{Key: "agent_type", Value: doc.AgentType},
			{Key: "name", Value: doc.Name},
			{Key: "tenant_id", Value: doc.TenantID},
		},
		bson.D{{Key: "$set", Value: bson.D{{Key: "is_active", Value: false}, {Key: "updated_at", Value: time.Now()}}}},
	)
	if err != nil {
		return fmt.Errorf("deactivate versions: %w", err)
	}

	// Activate the target version.
	_, err = s.coll.UpdateOne(ctx,
		bson.D{{Key: "_id", Value: id}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "is_active", Value: true}, {Key: "updated_at", Value: time.Now()}}}},
	)
	return err
}
