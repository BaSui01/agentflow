package mongodb

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/BaSui01/agentflow/llm/tools"
	mongoclient "github.com/BaSui01/agentflow/pkg/mongodb"
)

// Collection name for audit logs.
const auditCollection = "audit_logs"

// MongoAuditBackend implements tools.AuditBackend backed by MongoDB.
type MongoAuditBackend struct {
	coll *mongo.Collection
}

// NewAuditBackend creates a MongoAuditBackend and ensures indexes.
func NewAuditBackend(ctx context.Context, client *mongoclient.Client) (*MongoAuditBackend, error) {
	coll := client.Collection(auditCollection)

	indexes := []mongo.IndexModel{
		{Keys: bson.D{{Key: "agent_id", Value: 1}, {Key: "timestamp", Value: -1}}},
		{Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "timestamp", Value: -1}}},
		{Keys: bson.D{{Key: "event_type", Value: 1}, {Key: "timestamp", Value: -1}}},
		{Keys: bson.D{{Key: "tool_name", Value: 1}, {Key: "timestamp", Value: -1}}},
		{Keys: bson.D{{Key: "session_id", Value: 1}}},
		{Keys: bson.D{{Key: "trace_id", Value: 1}}},
	}
	if err := client.EnsureIndexes(ctx, auditCollection, indexes); err != nil {
		return nil, err
	}

	return &MongoAuditBackend{coll: coll}, nil
}

// Write inserts an audit entry into MongoDB.
func (b *MongoAuditBackend) Write(ctx context.Context, entry *tools.AuditEntry) error {
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}
	_, err := b.coll.InsertOne(ctx, entry)
	return err
}

// Query retrieves audit entries matching the filter.
func (b *MongoAuditBackend) Query(ctx context.Context, filter *tools.AuditFilter) ([]*tools.AuditEntry, error) {
	f := bson.D{}
	if filter.AgentID != "" {
		f = append(f, bson.E{Key: "agent_id", Value: filter.AgentID})
	}
	if filter.UserID != "" {
		f = append(f, bson.E{Key: "user_id", Value: filter.UserID})
	}
	if filter.ToolName != "" {
		f = append(f, bson.E{Key: "tool_name", Value: filter.ToolName})
	}
	if filter.EventType != "" {
		f = append(f, bson.E{Key: "event_type", Value: filter.EventType})
	}
	if filter.SessionID != "" {
		f = append(f, bson.E{Key: "session_id", Value: filter.SessionID})
	}
	if filter.TraceID != "" {
		f = append(f, bson.E{Key: "trace_id", Value: filter.TraceID})
	}
	if filter.StartTime != nil || filter.EndTime != nil {
		timeFilter := bson.D{}
		if filter.StartTime != nil {
			timeFilter = append(timeFilter, bson.E{Key: "$gte", Value: *filter.StartTime})
		}
		if filter.EndTime != nil {
			timeFilter = append(timeFilter, bson.E{Key: "$lte", Value: *filter.EndTime})
		}
		f = append(f, bson.E{Key: "timestamp", Value: timeFilter})
	}

	opts := options.Find().SetSort(bson.D{{Key: "timestamp", Value: -1}})
	if filter.Limit > 0 {
		opts.SetLimit(int64(filter.Limit))
	}
	if filter.Offset > 0 {
		opts.SetSkip(int64(filter.Offset))
	}

	cursor, err := b.coll.Find(ctx, f, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var entries []*tools.AuditEntry
	if err := cursor.All(ctx, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

// Close is a no-op; the MongoDB client lifecycle is managed by pkg/mongodb.Client.
func (b *MongoAuditBackend) Close() error {
	return nil
}

// Compile-time interface check.
var _ tools.AuditBackend = (*MongoAuditBackend)(nil)

