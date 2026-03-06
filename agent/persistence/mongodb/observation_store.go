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

const observationCollection = "observations"

type observationDocument struct {
	ID        string         `bson:"_id"         json:"id"`
	AgentID   string         `bson:"agent_id"    json:"agent_id"`
	Date      string         `bson:"date"        json:"date"`
	Content   string         `bson:"content"     json:"content"`
	CreatedAt time.Time      `bson:"created_at"  json:"created_at"`
	Metadata  map[string]any `bson:"metadata"    json:"metadata,omitempty"`
}

// MongoObservationStore implements memory.ObservationStore backed by MongoDB.
type MongoObservationStore struct {
	coll *mongo.Collection
}

// NewObservationStore creates a MongoObservationStore and ensures indexes.
func NewObservationStore(ctx context.Context, client *mongoclient.Client) (*MongoObservationStore, error) {
	coll := client.Collection(observationCollection)

	indexes := []mongo.IndexModel{
		{Keys: bson.D{{Key: "agent_id", Value: 1}, {Key: "created_at", Value: -1}}},
		{Keys: bson.D{{Key: "agent_id", Value: 1}}},
	}
	if err := client.EnsureIndexes(ctx, observationCollection, indexes); err != nil {
		return nil, err
	}

	return &MongoObservationStore{coll: coll}, nil
}

func (s *MongoObservationStore) Save(ctx context.Context, obs memory.Observation) error {
	if obs.ID == "" {
		obs.ID = fmt.Sprintf("obs_%d", time.Now().UnixNano())
	}
	if obs.CreatedAt.IsZero() {
		obs.CreatedAt = time.Now()
	}

	doc := observationDocument{
		ID:        obs.ID,
		AgentID:   obs.AgentID,
		Date:      obs.Date,
		Content:   obs.Content,
		CreatedAt: obs.CreatedAt,
		Metadata:  obs.Metadata,
	}

	filter := bson.D{{Key: "_id", Value: doc.ID}}
	update := bson.D{{Key: "$set", Value: doc}}
	opts := options.UpdateOne().SetUpsert(true)

	_, err := s.coll.UpdateOne(ctx, filter, update, opts)
	return err
}

func (s *MongoObservationStore) LoadRecent(ctx context.Context, agentID string, limit int) ([]memory.Observation, error) {
	filter := bson.D{{Key: "agent_id", Value: agentID}}
	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}}).
		SetLimit(int64(limit))

	return s.findObservations(ctx, filter, opts)
}

func (s *MongoObservationStore) LoadByDateRange(ctx context.Context, agentID string, start, end time.Time) ([]memory.Observation, error) {
	filter := bson.D{
		{Key: "agent_id", Value: agentID},
		{Key: "created_at", Value: bson.D{
			{Key: "$gte", Value: start},
			{Key: "$lte", Value: end},
		}},
	}
	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}})

	return s.findObservations(ctx, filter, opts)
}

func (s *MongoObservationStore) findObservations(ctx context.Context, filter bson.D, opts *options.FindOptionsBuilder) ([]memory.Observation, error) {
	cursor, err := s.coll.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var docs []observationDocument
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, err
	}

	results := make([]memory.Observation, 0, len(docs))
	for _, doc := range docs {
		results = append(results, memory.Observation{
			ID:        doc.ID,
			AgentID:   doc.AgentID,
			Date:      doc.Date,
			Content:   doc.Content,
			CreatedAt: doc.CreatedAt,
			Metadata:  doc.Metadata,
		})
	}
	return results, nil
}

var _ memory.ObservationStore = (*MongoObservationStore)(nil)
