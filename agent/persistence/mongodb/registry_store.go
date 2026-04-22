package mongodb

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/BaSui01/agentflow/agent/capabilities/tools"
	"github.com/BaSui01/agentflow/agent/persistence"
	mongoclient "github.com/BaSui01/agentflow/pkg/mongodb"
)

// Collection name for agent registry.
const registryCollection = "agent_registry"

// registryDocument wraps tools.AgentInfo with a MongoDB _id field.
type registryDocument struct {
	ID        string           `bson:"_id"`
	AgentInfo *tools.AgentInfo `bson:"agent_info"`
	UpdatedAt time.Time        `bson:"updated_at"`
}

// MongoRegistryStore implements tools.RegistryStore backed by MongoDB.
type MongoRegistryStore struct {
	coll *mongo.Collection
}

// NewRegistryStore creates a MongoRegistryStore and ensures indexes.
// The _id index is created automatically by MongoDB; no additional indexes needed.
func NewRegistryStore(ctx context.Context, client *mongoclient.Client) (*MongoRegistryStore, error) {
	_ = ctx // used for consistency with other store constructors
	coll := client.Collection(registryCollection)
	return &MongoRegistryStore{coll: coll}, nil
}

func (s *MongoRegistryStore) Save(ctx context.Context, agent *tools.AgentInfo) error {
	if agent == nil || agent.Card == nil {
		return persistence.ErrInvalidInput
	}

	id := agent.Card.Name
	if id == "" {
		return persistence.ErrInvalidInput
	}

	doc := &registryDocument{
		ID:        id,
		AgentInfo: agent,
		UpdatedAt: time.Now(),
	}

	opts := options.Replace().SetUpsert(true)
	_, err := s.coll.ReplaceOne(ctx, bson.D{{Key: "_id", Value: id}}, doc, opts)
	return err
}

func (s *MongoRegistryStore) Load(ctx context.Context, id string) (*tools.AgentInfo, error) {
	var doc registryDocument
	err := s.coll.FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&doc)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("agent %s not found", id)
		}
		return nil, err
	}
	return doc.AgentInfo, nil
}

func (s *MongoRegistryStore) LoadAll(ctx context.Context) ([]*tools.AgentInfo, error) {
	opts := options.Find().SetLimit(1000)
	cursor, err := s.coll.Find(ctx, bson.D{}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var docs []registryDocument
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, err
	}

	result := make([]*tools.AgentInfo, 0, len(docs))
	for _, doc := range docs {
		result = append(result, doc.AgentInfo)
	}
	return result, nil
}

func (s *MongoRegistryStore) Delete(ctx context.Context, id string) error {
	result, err := s.coll.DeleteOne(ctx, bson.D{{Key: "_id", Value: id}})
	if err != nil {
		return err
	}
	if result.DeletedCount == 0 {
		return fmt.Errorf("agent %s not found", id)
	}
	return nil
}

// Compile-time interface check.
var _ tools.RegistryStore = (*MongoRegistryStore)(nil)
