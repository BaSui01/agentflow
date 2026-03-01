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

// Collection name for memory store.
const memoryStoreCollection = "memory_store"

// memoryDocument is the MongoDB document for a memory entry.
type memoryDocument struct {
	Key       string    `bson:"_id"        json:"key"`
	Value     any       `bson:"value"      json:"value"`
	CreatedAt time.Time `bson:"created_at" json:"created_at"`
	ExpiresAt time.Time `bson:"expires_at" json:"expires_at,omitempty"`
}

// MongoMemoryStore implements memory.MemoryStore backed by MongoDB.
type MongoMemoryStore struct {
	coll *mongo.Collection
}

// NewMemoryStore creates a MongoMemoryStore and ensures indexes.
func NewMemoryStore(ctx context.Context, client *mongoclient.Client) (*MongoMemoryStore, error) {
	coll := client.Collection(memoryStoreCollection)

	indexes := []mongo.IndexModel{
		// TTL index: MongoDB automatically deletes documents when expires_at is reached.
		{
			Keys:    bson.D{{Key: "expires_at", Value: 1}},
			Options: options.Index().SetExpireAfterSeconds(0),
		},
	}
	if err := client.EnsureIndexes(ctx, memoryStoreCollection, indexes); err != nil {
		return nil, err
	}

	return &MongoMemoryStore{coll: coll}, nil
}

func (s *MongoMemoryStore) Save(ctx context.Context, key string, value any, ttl time.Duration) error {
	if key == "" {
		return fmt.Errorf("key is required")
	}

	now := time.Now()
	doc := &memoryDocument{
		Key:       key,
		Value:     value,
		CreatedAt: now,
	}
	if ttl > 0 {
		doc.ExpiresAt = now.Add(ttl)
	}

	opts := options.Replace().SetUpsert(true)
	_, err := s.coll.ReplaceOne(ctx, bson.D{{Key: "_id", Value: key}}, doc, opts)
	return err
}

func (s *MongoMemoryStore) Load(ctx context.Context, key string) (any, error) {
	if key == "" {
		return nil, fmt.Errorf("key is required")
	}

	var doc memoryDocument
	err := s.coll.FindOne(ctx, bson.D{{Key: "_id", Value: key}}).Decode(&doc)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("key %q not found", key)
		}
		return nil, err
	}

	// Check expiry (MongoDB TTL index may have a delay of up to 60s).
	if !doc.ExpiresAt.IsZero() && time.Now().After(doc.ExpiresAt) {
		return nil, fmt.Errorf("key %q not found", key)
	}

	return doc.Value, nil
}

func (s *MongoMemoryStore) Delete(ctx context.Context, key string) error {
	if key == "" {
		return fmt.Errorf("key is required")
	}
	_, err := s.coll.DeleteOne(ctx, bson.D{{Key: "_id", Value: key}})
	return err
}

func (s *MongoMemoryStore) List(ctx context.Context, pattern string, limit int) ([]any, error) {
	filter := bson.D{}
	if pattern != "" && pattern != "*" {
		// Convert wildcard pattern to regex.
		regex := wildcardToRegex(pattern)
		filter = bson.D{{Key: "_id", Value: bson.D{{Key: "$regex", Value: regex}}}}
	}

	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}})
	if limit > 0 {
		opts.SetLimit(int64(limit))
	}

	cursor, err := s.coll.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var docs []memoryDocument
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, err
	}

	now := time.Now()
	result := make([]any, 0, len(docs))
	for _, doc := range docs {
		if !doc.ExpiresAt.IsZero() && now.After(doc.ExpiresAt) {
			continue
		}
		result = append(result, doc.Value)
	}
	return result, nil
}

func (s *MongoMemoryStore) Clear(ctx context.Context) error {
	_, err := s.coll.DeleteMany(ctx, bson.D{})
	return err
}

// wildcardToRegex converts a simple wildcard pattern (with *) to a MongoDB regex.
func wildcardToRegex(pattern string) string {
	regex := "^"
	for _, ch := range pattern {
		switch ch {
		case '*':
			regex += ".*"
		case '.', '(', ')', '[', ']', '{', '}', '+', '?', '^', '$', '|', '\\':
			regex += "\\" + string(ch)
		default:
			regex += string(ch)
		}
	}
	regex += "$"
	return regex
}

// Compile-time interface check.
var _ memory.MemoryStore = (*MongoMemoryStore)(nil)

