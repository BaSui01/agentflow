package mongodb

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/BaSui01/agentflow/agent/persistence"
	mongoclient "github.com/BaSui01/agentflow/pkg/mongodb"
	"github.com/BaSui01/agentflow/types"
)

// Collection name for conversations.
const conversationsCollection = "conversations"

// ConversationDocument is the MongoDB document for a single conversation.
type ConversationDocument struct {
	ID         string            `bson:"_id"         json:"id"`
	ParentID   string            `bson:"parent_id"   json:"parent_id,omitempty"`
	AgentID    string            `bson:"agent_id"    json:"agent_id"`
	TenantID   string            `bson:"tenant_id"   json:"tenant_id"`
	UserID     string            `bson:"user_id"     json:"user_id"`
	Title      string            `bson:"title"       json:"title,omitempty"`
	Messages   []MessageDocument `bson:"messages"    json:"messages"`
	Branches   []BranchDocument  `bson:"branches"    json:"branches,omitempty"`
	Metadata   map[string]any    `bson:"metadata"    json:"metadata,omitempty"`
	Archived   bool              `bson:"archived"    json:"archived"`
	ArchivedAt *time.Time        `bson:"archived_at" json:"archived_at,omitempty"`
	CreatedAt  time.Time         `bson:"created_at"  json:"created_at"`
	UpdatedAt  time.Time         `bson:"updated_at"  json:"updated_at"`
}

// MessageDocument maps to types.Message for BSON storage.
type MessageDocument struct {
	ID         string               `bson:"id"           json:"id,omitempty"`
	Role       string               `bson:"role"         json:"role"`
	Content    string               `bson:"content"      json:"content,omitempty"`
	Name       string               `bson:"name"         json:"name,omitempty"`
	ToolCalls  []types.ToolCall     `bson:"tool_calls"   json:"tool_calls,omitempty"`
	ToolCallID string               `bson:"tool_call_id" json:"tool_call_id,omitempty"`
	Images     []types.ImageContent `bson:"images"       json:"images,omitempty"`
	Metadata   any                  `bson:"metadata"     json:"metadata,omitempty"`
	Timestamp  time.Time            `bson:"timestamp"    json:"timestamp,omitempty"`
}

// BranchDocument stores conversation branch metadata.
type BranchDocument struct {
	ID          string    `bson:"id"          json:"id"`
	Name        string    `bson:"name"        json:"name"`
	Description string    `bson:"description" json:"description,omitempty"`
	ParentIndex int       `bson:"parent_index" json:"parent_index"`
	IsActive    bool      `bson:"is_active"   json:"is_active"`
	CreatedAt   time.Time `bson:"created_at"  json:"created_at"`
}

// ConversationFilter defines query parameters for listing conversations.
type ConversationFilter struct {
	AgentID   string     `json:"agent_id,omitempty"`
	TenantID  string     `json:"tenant_id,omitempty"`
	ParentID  string     `json:"parent_id,omitempty"`
	UserID    string     `json:"user_id,omitempty"`
	StartTime *time.Time `json:"start_time,omitempty"`
	EndTime   *time.Time `json:"end_time,omitempty"`
	Limit     int        `json:"limit,omitempty"`
	Offset    int        `json:"offset,omitempty"`
}

// ConversationStore defines operations for conversation persistence.
type ConversationStore interface {
	// Create inserts a new conversation.
	Create(ctx context.Context, doc *ConversationDocument) error
	// GetByID retrieves a conversation by ID.
	GetByID(ctx context.Context, id string) (*ConversationDocument, error)
	// AppendMessage adds a message to an existing conversation.
	AppendMessage(ctx context.Context, conversationID string, msg MessageDocument) error
	// GetMessages returns a paginated slice of messages and total count.
	GetMessages(ctx context.Context, conversationID string, offset, limit int) ([]MessageDocument, int64, error)
	// List returns conversations matching the filter along with total count.
	List(ctx context.Context, filter ConversationFilter) ([]*ConversationDocument, int64, error)
	// Update applies field-level updates to a conversation.
	Update(ctx context.Context, id string, updates bson.D) error
	// Delete removes a conversation by ID.
	Delete(ctx context.Context, id string) error
	// DeleteByParentID removes all conversations under a given parent within a tenant.
	DeleteByParentID(ctx context.Context, tenantID, parentID string) error
	// DeleteMessage removes a single embedded message by its ID.
	DeleteMessage(ctx context.Context, conversationID, messageID string) error
	// ClearMessages removes all messages from a conversation.
	ClearMessages(ctx context.Context, conversationID string) error
	// Archive marks a conversation as archived.
	Archive(ctx context.Context, id string) error
	// UpdateBranches replaces the branches array for a conversation.
	UpdateBranches(ctx context.Context, conversationID string, branches []BranchDocument) error
}

// MongoConversationStore implements ConversationStore backed by MongoDB.
type MongoConversationStore struct {
	coll *mongo.Collection
}

// NewConversationStore creates a MongoConversationStore and ensures indexes.
func NewConversationStore(ctx context.Context, client *mongoclient.Client) (*MongoConversationStore, error) {
	coll := client.Collection(conversationsCollection)

	indexes := []mongo.IndexModel{
		{Keys: bson.D{{Key: "agent_id", Value: 1}, {Key: "tenant_id", Value: 1}, {Key: "created_at", Value: -1}}},
		{Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "tenant_id", Value: 1}}},
		{Keys: bson.D{{Key: "tenant_id", Value: 1}, {Key: "parent_id", Value: 1}, {Key: "created_at", Value: -1}}},
	}
	if err := client.EnsureIndexes(ctx, conversationsCollection, indexes); err != nil {
		return nil, err
	}

	return &MongoConversationStore{coll: coll}, nil
}

func (s *MongoConversationStore) Create(ctx context.Context, doc *ConversationDocument) error {
	if doc.ID == "" {
		return persistence.ErrInvalidInput
	}
	now := time.Now()
	doc.CreatedAt = now
	doc.UpdatedAt = now
	if doc.Messages == nil {
		doc.Messages = []MessageDocument{}
	}

	_, err := s.coll.InsertOne(ctx, doc)
	if mongo.IsDuplicateKeyError(err) {
		return persistence.ErrAlreadyExists
	}
	return err
}

func (s *MongoConversationStore) GetByID(ctx context.Context, id string) (*ConversationDocument, error) {
	var doc ConversationDocument
	err := s.coll.FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&doc)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, persistence.ErrNotFound
		}
		return nil, err
	}
	return &doc, nil
}

func (s *MongoConversationStore) AppendMessage(ctx context.Context, conversationID string, msg MessageDocument) error {
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}
	result, err := s.coll.UpdateOne(ctx,
		bson.D{{Key: "_id", Value: conversationID}},
		bson.D{
			{Key: "$push", Value: bson.D{{Key: "messages", Value: msg}}},
			{Key: "$set", Value: bson.D{{Key: "updated_at", Value: time.Now()}}},
		},
	)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return persistence.ErrNotFound
	}
	return nil
}

// GetMessages paginates the messages array and returns the total message count.
func (s *MongoConversationStore) GetMessages(ctx context.Context, conversationID string, offset, limit int) ([]MessageDocument, int64, error) {
	if limit <= 0 {
		limit = 50
	}

	// Fetch slice + total count via aggregation.
	pipeline := bson.A{
		bson.D{{Key: "$match", Value: bson.D{{Key: "_id", Value: conversationID}}}},
		bson.D{{Key: "$project", Value: bson.D{
			{Key: "total", Value: bson.D{{Key: "$size", Value: "$messages"}}},
			{Key: "messages", Value: bson.D{{Key: "$slice", Value: bson.A{"$messages", offset, limit}}}},
		}}},
	}

	type result struct {
		Total    int64             `bson:"total"`
		Messages []MessageDocument `bson:"messages"`
	}

	cursor, err := s.coll.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var res result
	if cursor.Next(ctx) {
		if err := cursor.Decode(&res); err != nil {
			return nil, 0, err
		}
	} else {
		return nil, 0, persistence.ErrNotFound
	}
	return res.Messages, res.Total, nil
}

func (s *MongoConversationStore) List(ctx context.Context, filter ConversationFilter) ([]*ConversationDocument, int64, error) {
	f := bson.D{}
	if filter.AgentID != "" {
		f = append(f, bson.E{Key: "agent_id", Value: filter.AgentID})
	}
	if filter.TenantID != "" {
		f = append(f, bson.E{Key: "tenant_id", Value: filter.TenantID})
	}
	if filter.UserID != "" {
		f = append(f, bson.E{Key: "user_id", Value: filter.UserID})
	}
	if filter.ParentID != "" {
		f = append(f, bson.E{Key: "parent_id", Value: filter.ParentID})
	}
	if filter.StartTime != nil || filter.EndTime != nil {
		timeFilter := bson.D{}
		if filter.StartTime != nil {
			timeFilter = append(timeFilter, bson.E{Key: "$gte", Value: *filter.StartTime})
		}
		if filter.EndTime != nil {
			timeFilter = append(timeFilter, bson.E{Key: "$lte", Value: *filter.EndTime})
		}
		f = append(f, bson.E{Key: "created_at", Value: timeFilter})
	}

	total, err := s.coll.CountDocuments(ctx, f)
	if err != nil {
		return nil, 0, err
	}

	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}})
	// Exclude messages array from list queries for performance.
	opts.SetProjection(bson.D{{Key: "messages", Value: 0}})
	if filter.Limit > 0 {
		opts.SetLimit(int64(filter.Limit))
	}
	if filter.Offset > 0 {
		opts.SetSkip(int64(filter.Offset))
	}

	cursor, err := s.coll.Find(ctx, f, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var docs []*ConversationDocument
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, 0, err
	}
	return docs, total, nil
}

func (s *MongoConversationStore) Update(ctx context.Context, id string, updates bson.D) error {
	updates = append(updates, bson.E{Key: "updated_at", Value: time.Now()})
	result, err := s.coll.UpdateOne(ctx,
		bson.D{{Key: "_id", Value: id}},
		bson.D{{Key: "$set", Value: updates}},
	)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return persistence.ErrNotFound
	}
	return nil
}

// DeleteByParentID implements ConversationStore.DeleteByParentID.
// 安全要求: 调用方必须确保 tenantID 来自可信来源（如 JWT 或已验证的租户上下文），
// 不得使用用户可控的 tenantID，否则可能导致跨租户数据泄露。
func (s *MongoConversationStore) DeleteByParentID(ctx context.Context, tenantID, parentID string) error {
	_, err := s.coll.DeleteMany(ctx, bson.D{
		{Key: "tenant_id", Value: tenantID},
		{Key: "parent_id", Value: parentID},
	})
	return err
}

func (s *MongoConversationStore) DeleteMessage(ctx context.Context, conversationID, messageID string) error {
	result, err := s.coll.UpdateOne(ctx,
		bson.D{{Key: "_id", Value: conversationID}},
		bson.D{
			{Key: "$pull", Value: bson.D{{Key: "messages", Value: bson.D{{Key: "id", Value: messageID}}}}},
			{Key: "$set", Value: bson.D{{Key: "updated_at", Value: time.Now()}}},
		},
	)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return persistence.ErrNotFound
	}
	return nil
}

func (s *MongoConversationStore) ClearMessages(ctx context.Context, conversationID string) error {
	result, err := s.coll.UpdateOne(ctx,
		bson.D{{Key: "_id", Value: conversationID}},
		bson.D{{Key: "$set", Value: bson.D{
			{Key: "messages", Value: []MessageDocument{}},
			{Key: "updated_at", Value: time.Now()},
		}}},
	)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return persistence.ErrNotFound
	}
	return nil
}

func (s *MongoConversationStore) Archive(ctx context.Context, id string) error {
	now := time.Now()
	result, err := s.coll.UpdateOne(ctx,
		bson.D{{Key: "_id", Value: id}},
		bson.D{{Key: "$set", Value: bson.D{
			{Key: "archived", Value: true},
			{Key: "archived_at", Value: now},
			{Key: "updated_at", Value: now},
		}}},
	)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return persistence.ErrNotFound
	}
	return nil
}

func (s *MongoConversationStore) Delete(ctx context.Context, id string) error {
	result, err := s.coll.DeleteOne(ctx, bson.D{{Key: "_id", Value: id}})
	if err != nil {
		return err
	}
	if result.DeletedCount == 0 {
		return persistence.ErrNotFound
	}
	return nil
}

func (s *MongoConversationStore) UpdateBranches(ctx context.Context, conversationID string, branches []BranchDocument) error {
	result, err := s.coll.UpdateOne(ctx,
		bson.D{{Key: "_id", Value: conversationID}},
		bson.D{
			{Key: "$set", Value: bson.D{
				{Key: "branches", Value: branches},
				{Key: "updated_at", Value: time.Now()},
			}},
		},
	)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return persistence.ErrNotFound
	}
	return nil
}
