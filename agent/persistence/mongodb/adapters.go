// Package mongodb provides adapter types that bridge the concrete MongoDB store
// implementations to the agent-layer interfaces defined in agent/interfaces.go.
//
// These adapters allow server.go to pass MongoDB stores to the agent builder
// without the agent package importing this package directly.
package mongodb

import (
	"context"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/agent/persistence"
)

// =============================================================================
// PromptStoreAdapter — adapts *MongoPromptStore to agent.PromptStoreProvider
// =============================================================================

// PromptStoreAdapter wraps MongoPromptStore to satisfy agent.PromptStoreProvider.
type PromptStoreAdapter struct {
	store *MongoPromptStore
}

// NewPromptStoreAdapter creates a PromptStoreAdapter.
func NewPromptStoreAdapter(store *MongoPromptStore) *PromptStoreAdapter {
	return &PromptStoreAdapter{store: store}
}

// GetActive returns the active prompt for the given agent type, name, and tenant.
func (a *PromptStoreAdapter) GetActive(ctx context.Context, agentType, name, tenantID string) (agent.PromptDocument, error) {
	doc, err := a.store.GetActive(ctx, agentType, name, tenantID)
	if err != nil {
		return agent.PromptDocument{}, err
	}
	return agent.PromptDocument{
		Version:     doc.Version,
		System:      doc.System,
		Constraints: doc.Constraints,
	}, nil
}

// Underlying returns the underlying MongoPromptStore for direct access.
func (a *PromptStoreAdapter) Underlying() *MongoPromptStore { return a.store }

// =============================================================================
// ConversationStoreAdapter — adapts *MongoConversationStore to agent.ConversationStoreProvider
// =============================================================================

// ConversationStoreAdapter wraps MongoConversationStore to satisfy agent.ConversationStoreProvider.
type ConversationStoreAdapter struct {
	store *MongoConversationStore
}

// NewConversationStoreAdapter creates a ConversationStoreAdapter.
func NewConversationStoreAdapter(store *MongoConversationStore) *ConversationStoreAdapter {
	return &ConversationStoreAdapter{store: store}
}

// Create inserts a new conversation document.
func (a *ConversationStoreAdapter) Create(ctx context.Context, doc *agent.ConversationDoc) error {
	mongoDoc := &ConversationDocument{
		ID:       doc.ID,
		AgentID:  doc.AgentID,
		TenantID: doc.TenantID,
		UserID:   doc.UserID,
		Messages: convertMessagesToMongo(doc.Messages),
	}
	return a.store.Create(ctx, mongoDoc)
}

// GetByID retrieves a conversation by ID.
func (a *ConversationStoreAdapter) GetByID(ctx context.Context, id string) (*agent.ConversationDoc, error) {
	doc, err := a.store.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return &agent.ConversationDoc{
		ID:       doc.ID,
		AgentID:  doc.AgentID,
		TenantID: doc.TenantID,
		UserID:   doc.UserID,
		Messages: convertMessagesFromMongo(doc.Messages),
	}, nil
}

// AppendMessages adds messages to an existing conversation.
func (a *ConversationStoreAdapter) AppendMessages(ctx context.Context, conversationID string, msgs []agent.ConversationMessage) error {
	for _, msg := range msgs {
		mongoMsg := MessageDocument{
			Role:      msg.Role,
			Content:   msg.Content,
			Timestamp: msg.Timestamp,
		}
		if err := a.store.AppendMessage(ctx, conversationID, mongoMsg); err != nil {
			return err
		}
	}
	return nil
}

// Underlying returns the underlying MongoConversationStore for direct access.
func (a *ConversationStoreAdapter) Underlying() *MongoConversationStore { return a.store }

// =============================================================================
// RunStoreAdapter — adapts *MongoRunStore to agent.RunStoreProvider
// =============================================================================

// RunStoreAdapter wraps MongoRunStore to satisfy agent.RunStoreProvider.
type RunStoreAdapter struct {
	store *MongoRunStore
}

// NewRunStoreAdapter creates a RunStoreAdapter.
func NewRunStoreAdapter(store *MongoRunStore) *RunStoreAdapter {
	return &RunStoreAdapter{store: store}
}

// RecordRun inserts a new run document.
func (a *RunStoreAdapter) RecordRun(ctx context.Context, doc *agent.RunDoc) error {
	mongoDoc := &RunDocument{
		ID:       doc.ID,
		AgentID:  doc.AgentID,
		TenantID: doc.TenantID,
		TraceID:  doc.TraceID,
		Status:   doc.Status,
		Input: RunInput{
			Content: doc.Input,
		},
		StartTime: doc.StartTime,
	}
	return a.store.RecordRun(ctx, mongoDoc)
}

// UpdateStatus updates the status and output of a run.
func (a *RunStoreAdapter) UpdateStatus(ctx context.Context, id, status string, output *agent.RunOutputDoc, errMsg string) error {
	var mongoOutput *RunOutput
	if output != nil {
		mongoOutput = &RunOutput{
			Content:      output.Content,
			TokensUsed:   output.TokensUsed,
			Cost:         output.Cost,
			FinishReason: output.FinishReason,
		}
	}
	return a.store.UpdateStatus(ctx, id, status, mongoOutput, errMsg)
}

// Underlying returns the underlying MongoRunStore for direct access.
func (a *RunStoreAdapter) Underlying() *MongoRunStore { return a.store }

// =============================================================================
// Helpers
// =============================================================================

func convertMessagesToMongo(msgs []agent.ConversationMessage) []MessageDocument {
	docs := make([]MessageDocument, len(msgs))
	for i, m := range msgs {
		docs[i] = MessageDocument{
			Role:      m.Role,
			Content:   m.Content,
			Timestamp: m.Timestamp,
		}
	}
	return docs
}

func convertMessagesFromMongo(docs []MessageDocument) []agent.ConversationMessage {
	msgs := make([]agent.ConversationMessage, len(docs))
	for i, d := range docs {
		msgs[i] = agent.ConversationMessage{
			Role:      d.Role,
			Content:   d.Content,
			Timestamp: d.Timestamp,
		}
	}
	return msgs
}

// Compile-time interface checks.
var (
	_ agent.PromptStoreProvider       = (*PromptStoreAdapter)(nil)
	_ agent.ConversationStoreProvider = (*ConversationStoreAdapter)(nil)
	_ agent.RunStoreProvider          = (*RunStoreAdapter)(nil)
)

// ErrNotFound re-exports persistence.ErrNotFound for callers that need to check it.
var ErrNotFound = persistence.ErrNotFound

// IsNotFound checks if an error is a not-found error.
func IsNotFound(err error) bool {
	return err == persistence.ErrNotFound
}
