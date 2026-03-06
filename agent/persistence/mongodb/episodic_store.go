package mongodb

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/BaSui01/agentflow/agent/memory"
	"github.com/BaSui01/agentflow/types"
	mongoclient "github.com/BaSui01/agentflow/pkg/mongodb"
)

// Collection name for episodic events.
const episodicCollection = "episodic_events"

// episodicDocument is the MongoDB document for an episodic event.
type episodicDocument struct {
	ID        string         `bson:"_id"        json:"id"`
	AgentID   string         `bson:"agent_id"   json:"agent_id"`
	Type      string         `bson:"type"       json:"type"`
	Content   string         `bson:"content"    json:"content"`
	Context   map[string]any `bson:"context"    json:"context,omitempty"`
	Timestamp time.Time      `bson:"timestamp"  json:"timestamp"`
	Duration  time.Duration  `bson:"duration"   json:"duration"`
}

// MongoEpisodicStore implements memory.EpisodicStore backed by MongoDB.
type MongoEpisodicStore struct {
	coll *mongo.Collection
}

// NewEpisodicStore creates a MongoEpisodicStore and ensures indexes.
func NewEpisodicStore(ctx context.Context, client *mongoclient.Client) (*MongoEpisodicStore, error) {
	coll := client.Collection(episodicCollection)

	indexes := []mongo.IndexModel{
		{Keys: bson.D{{Key: "agent_id", Value: 1}, {Key: "timestamp", Value: -1}}},
		{Keys: bson.D{{Key: "type", Value: 1}, {Key: "timestamp", Value: -1}}},
	}
	if err := client.EnsureIndexes(ctx, episodicCollection, indexes); err != nil {
		return nil, err
	}

	return &MongoEpisodicStore{coll: coll}, nil
}

func (s *MongoEpisodicStore) RecordEvent(ctx context.Context, event *types.EpisodicEvent) error {
	if event == nil {
		return fmt.Errorf("event is nil")
	}
	if event.ID == "" {
		event.ID = fmt.Sprintf("ep_%d", time.Now().UnixNano())
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	doc := &episodicDocument{
		ID:        event.ID,
		AgentID:   event.AgentID,
		Type:      event.Type,
		Content:   event.Content,
		Context:   event.Context,
		Timestamp: event.Timestamp,
		Duration:  event.Duration,
	}

	_, err := s.coll.InsertOne(ctx, doc)
	if mongo.IsDuplicateKeyError(err) {
		// Idempotent: treat duplicate as success.
		return nil
	}
	return err
}

func (s *MongoEpisodicStore) QueryEvents(ctx context.Context, query memory.EpisodicQuery) ([]types.EpisodicEvent, error) {
	filter := bson.D{}
	if query.AgentID != "" {
		filter = append(filter, bson.E{Key: "agent_id", Value: query.AgentID})
	}
	if query.Type != "" {
		filter = append(filter, bson.E{Key: "type", Value: query.Type})
	}
	if !query.StartTime.IsZero() || !query.EndTime.IsZero() {
		timeFilter := bson.D{}
		if !query.StartTime.IsZero() {
			timeFilter = append(timeFilter, bson.E{Key: "$gte", Value: query.StartTime})
		}
		if !query.EndTime.IsZero() {
			timeFilter = append(timeFilter, bson.E{Key: "$lte", Value: query.EndTime})
		}
		filter = append(filter, bson.E{Key: "timestamp", Value: timeFilter})
	}

	// Results sorted by timestamp descending (newest first), matching in-memory behavior.
	opts := options.Find().SetSort(bson.D{{Key: "timestamp", Value: -1}})
	if query.Limit > 0 {
		opts.SetLimit(int64(query.Limit))
	}

	return s.findEvents(ctx, filter, opts)
}

func (s *MongoEpisodicStore) GetTimeline(ctx context.Context, agentID string, start, end time.Time) ([]types.EpisodicEvent, error) {
	filter := bson.D{}
	if agentID != "" {
		filter = append(filter, bson.E{Key: "agent_id", Value: agentID})
	}
	if !start.IsZero() || !end.IsZero() {
		timeFilter := bson.D{}
		if !start.IsZero() {
			timeFilter = append(timeFilter, bson.E{Key: "$gte", Value: start})
		}
		if !end.IsZero() {
			timeFilter = append(timeFilter, bson.E{Key: "$lte", Value: end})
		}
		filter = append(filter, bson.E{Key: "timestamp", Value: timeFilter})
	}

	// Timeline sorted by timestamp ascending (oldest first), matching in-memory behavior.
	opts := options.Find().SetSort(bson.D{{Key: "timestamp", Value: 1}})

	return s.findEvents(ctx, filter, opts)
}

// findEvents is a shared helper for querying episodic events.
func (s *MongoEpisodicStore) findEvents(ctx context.Context, filter bson.D, opts *options.FindOptionsBuilder) ([]types.EpisodicEvent, error) {
	cursor, err := s.coll.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var docs []episodicDocument
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, err
	}

	events := make([]types.EpisodicEvent, 0, len(docs))
	for _, doc := range docs {
		events = append(events, types.EpisodicEvent{
			ID:        doc.ID,
			AgentID:   doc.AgentID,
			Type:      doc.Type,
			Content:   doc.Content,
			Context:   doc.Context,
			Timestamp: doc.Timestamp,
			Duration:  doc.Duration,
		})
	}
	return events, nil
}

// Compile-time interface check.
var _ memory.EpisodicStore = (*MongoEpisodicStore)(nil)

