package mongodb

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/BaSui01/agentflow/agent/persistence"
	mongoclient "github.com/BaSui01/agentflow/pkg/mongodb"
)

// Collection name for agent runs.
const runsCollection = "agent_runs"

// RunDocument is the MongoDB document for an agent execution run.
type RunDocument struct {
	ID        string         `bson:"_id"         json:"id"`
	AgentID   string         `bson:"agent_id"    json:"agent_id"`
	TenantID  string         `bson:"tenant_id"   json:"tenant_id"`
	TraceID   string         `bson:"trace_id"    json:"trace_id,omitempty"`
	Status    string         `bson:"status"      json:"status"`
	Input     RunInput       `bson:"input"       json:"input"`
	Output    *RunOutput     `bson:"output"      json:"output,omitempty"`
	Error     string         `bson:"error"       json:"error,omitempty"`
	StartTime time.Time      `bson:"start_time"  json:"start_time"`
	EndTime   *time.Time     `bson:"end_time"    json:"end_time,omitempty"`
	Duration  time.Duration  `bson:"duration"    json:"duration,omitempty"`
	Metadata  map[string]any `bson:"metadata"    json:"metadata,omitempty"`
}

// RunInput mirrors agent.Input for BSON storage.
type RunInput struct {
	Content   string            `bson:"content"    json:"content"`
	Variables map[string]string `bson:"variables"  json:"variables,omitempty"`
	Context   map[string]any    `bson:"context"    json:"context,omitempty"`
}

// RunOutput mirrors agent.Output for BSON storage.
type RunOutput struct {
	Content      string         `bson:"content"       json:"content"`
	TokensUsed   int            `bson:"tokens_used"   json:"tokens_used,omitempty"`
	Cost         float64        `bson:"cost"          json:"cost,omitempty"`
	FinishReason string         `bson:"finish_reason" json:"finish_reason,omitempty"`
	Metadata     map[string]any `bson:"metadata"      json:"metadata,omitempty"`
}

// RunFilter defines query parameters for listing runs.
type RunFilter struct {
	AgentID   string     `json:"agent_id,omitempty"`
	TenantID  string     `json:"tenant_id,omitempty"`
	Status    string     `json:"status,omitempty"`
	StartTime *time.Time `json:"start_time,omitempty"`
	EndTime   *time.Time `json:"end_time,omitempty"`
	Limit     int        `json:"limit,omitempty"`
	Offset    int        `json:"offset,omitempty"`
}

// RunStats holds aggregated statistics for agent runs.
type RunStats struct {
	TotalRuns   int64   `bson:"total_runs"   json:"total_runs"`
	Completed   int64   `bson:"completed"    json:"completed"`
	Failed      int64   `bson:"failed"       json:"failed"`
	TotalTokens int64   `bson:"total_tokens" json:"total_tokens"`
	TotalCost   float64 `bson:"total_cost"   json:"total_cost"`
}

// RunStore defines operations for agent run persistence.
type RunStore interface {
	// RecordRun inserts a new run document.
	RecordRun(ctx context.Context, doc *RunDocument) error
	// GetByID retrieves a run by ID.
	GetByID(ctx context.Context, id string) (*RunDocument, error)
	// UpdateStatus updates the status, output, error, and timing of a run.
	UpdateStatus(ctx context.Context, id, status string, output *RunOutput, errMsg string) error
	// List returns runs matching the filter.
	List(ctx context.Context, filter RunFilter) ([]*RunDocument, error)
	// Stats returns aggregated statistics for runs matching the filter.
	Stats(ctx context.Context, agentID, tenantID string) (*RunStats, error)
}

// MongoRunStore implements RunStore backed by MongoDB.
type MongoRunStore struct {
	coll *mongo.Collection
}

// NewRunStore creates a MongoRunStore and ensures indexes.
func NewRunStore(ctx context.Context, client *mongoclient.Client) (*MongoRunStore, error) {
	coll := client.Collection(runsCollection)

	indexes := []mongo.IndexModel{
		{Keys: bson.D{{Key: "agent_id", Value: 1}, {Key: "tenant_id", Value: 1}, {Key: "start_time", Value: -1}}},
		{Keys: bson.D{{Key: "status", Value: 1}, {Key: "start_time", Value: -1}}},
		{Keys: bson.D{{Key: "trace_id", Value: 1}}},
	}
	if err := client.EnsureIndexes(ctx, runsCollection, indexes); err != nil {
		return nil, err
	}

	return &MongoRunStore{coll: coll}, nil
}

func (s *MongoRunStore) RecordRun(ctx context.Context, doc *RunDocument) error {
	if doc.ID == "" {
		return persistence.ErrInvalidInput
	}
	if doc.StartTime.IsZero() {
		doc.StartTime = time.Now()
	}

	_, err := s.coll.InsertOne(ctx, doc)
	if mongo.IsDuplicateKeyError(err) {
		return persistence.ErrAlreadyExists
	}
	return err
}

func (s *MongoRunStore) GetByID(ctx context.Context, id string) (*RunDocument, error) {
	var doc RunDocument
	err := s.coll.FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&doc)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, persistence.ErrNotFound
		}
		return nil, err
	}
	return &doc, nil
}

func (s *MongoRunStore) UpdateStatus(ctx context.Context, id, status string, output *RunOutput, errMsg string) error {
	now := time.Now()
	update := bson.D{
		{Key: "$set", Value: bson.D{
			{Key: "status", Value: status},
			{Key: "end_time", Value: now},
		}},
	}

	setFields := update[0].Value.(bson.D)
	if output != nil {
		setFields = append(setFields, bson.E{Key: "output", Value: output})
	}
	if errMsg != "" {
		setFields = append(setFields, bson.E{Key: "error", Value: errMsg})
	}
	update[0].Value = setFields

	result, err := s.coll.UpdateOne(ctx, bson.D{{Key: "_id", Value: id}}, update)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return persistence.ErrNotFound
	}
	return nil
}

func (s *MongoRunStore) List(ctx context.Context, filter RunFilter) ([]*RunDocument, error) {
	f := bson.D{}
	if filter.AgentID != "" {
		f = append(f, bson.E{Key: "agent_id", Value: filter.AgentID})
	}
	if filter.TenantID != "" {
		f = append(f, bson.E{Key: "tenant_id", Value: filter.TenantID})
	}
	if filter.Status != "" {
		f = append(f, bson.E{Key: "status", Value: filter.Status})
	}
	if filter.StartTime != nil || filter.EndTime != nil {
		timeFilter := bson.D{}
		if filter.StartTime != nil {
			timeFilter = append(timeFilter, bson.E{Key: "$gte", Value: *filter.StartTime})
		}
		if filter.EndTime != nil {
			timeFilter = append(timeFilter, bson.E{Key: "$lte", Value: *filter.EndTime})
		}
		f = append(f, bson.E{Key: "start_time", Value: timeFilter})
	}

	opts := options.Find().SetSort(bson.D{{Key: "start_time", Value: -1}})
	if filter.Limit > 0 {
		opts.SetLimit(int64(filter.Limit))
	}
	if filter.Offset > 0 {
		opts.SetSkip(int64(filter.Offset))
	}

	cursor, err := s.coll.Find(ctx, f, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var docs []*RunDocument
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, err
	}
	return docs, nil
}

func (s *MongoRunStore) Stats(ctx context.Context, agentID, tenantID string) (*RunStats, error) {
	matchStage := bson.D{}
	if agentID != "" {
		matchStage = append(matchStage, bson.E{Key: "agent_id", Value: agentID})
	}
	if tenantID != "" {
		matchStage = append(matchStage, bson.E{Key: "tenant_id", Value: tenantID})
	}

	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: matchStage}},
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: nil},
			{Key: "total_runs", Value: bson.D{{Key: "$sum", Value: 1}}},
			{Key: "completed", Value: bson.D{{Key: "$sum", Value: bson.D{
				{Key: "$cond", Value: bson.A{bson.D{{Key: "$eq", Value: bson.A{"$status", "completed"}}}, 1, 0}},
			}}}},
			{Key: "failed", Value: bson.D{{Key: "$sum", Value: bson.D{
				{Key: "$cond", Value: bson.A{bson.D{{Key: "$eq", Value: bson.A{"$status", "failed"}}}, 1, 0}},
			}}}},
			{Key: "total_tokens", Value: bson.D{{Key: "$sum", Value: "$output.tokens_used"}}},
			{Key: "total_cost", Value: bson.D{{Key: "$sum", Value: "$output.cost"}}},
		}}},
	}

	cursor, err := s.coll.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []RunStats
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return &RunStats{}, nil
	}
	return &results[0], nil
}

