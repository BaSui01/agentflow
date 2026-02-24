package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"go.uber.org/zap"
)

// PersistenceStores encapsulates MongoDB persistence store fields extracted from BaseAgent.
type PersistenceStores struct {
	promptStore       PromptStoreProvider
	conversationStore ConversationStoreProvider
	runStore          RunStoreProvider
	logger            *zap.Logger
}

// NewPersistenceStores creates a new PersistenceStores.
func NewPersistenceStores(logger *zap.Logger) *PersistenceStores {
	return &PersistenceStores{logger: logger}
}

// SetPromptStore sets the prompt store provider.
func (p *PersistenceStores) SetPromptStore(store PromptStoreProvider) {
	p.promptStore = store
}

// SetConversationStore sets the conversation store provider.
func (p *PersistenceStores) SetConversationStore(store ConversationStoreProvider) {
	p.conversationStore = store
}

// SetRunStore sets the run store provider.
func (p *PersistenceStores) SetRunStore(store RunStoreProvider) {
	p.runStore = store
}

// PromptStore returns the prompt store provider.
func (p *PersistenceStores) PromptStore() PromptStoreProvider { return p.promptStore }

// ConversationStore returns the conversation store provider.
func (p *PersistenceStores) ConversationStore() ConversationStoreProvider {
	return p.conversationStore
}

// RunStore returns the run store provider.
func (p *PersistenceStores) RunStore() RunStoreProvider { return p.runStore }

// LoadPrompt attempts to load the active prompt from PromptStore.
// Returns nil if unavailable.
func (p *PersistenceStores) LoadPrompt(ctx context.Context, agentType, name, tenantID string) *PromptDocument {
	if p.promptStore == nil {
		return nil
	}
	doc, err := p.promptStore.GetActive(ctx, agentType, name, tenantID)
	if err != nil {
		p.logger.Debug("no active prompt in store, using config default",
			zap.String("agent_type", agentType),
			zap.String("name", name),
		)
		return nil
	}
	return &doc
}

// RecordRun records an execution run start. Returns the run ID (empty on failure).
func (p *PersistenceStores) RecordRun(ctx context.Context, agentID, tenantID, traceID, input string, startTime time.Time) string {
	if p.runStore == nil {
		return ""
	}
	runID := fmt.Sprintf("run_%s_%d", agentID, startTime.UnixNano())
	doc := &RunDoc{
		ID:        runID,
		AgentID:   agentID,
		TenantID:  tenantID,
		TraceID:   traceID,
		Status:    "running",
		Input:     input,
		StartTime: startTime,
	}
	if err := p.runStore.RecordRun(ctx, doc); err != nil {
		p.logger.Warn("failed to record run start", zap.Error(err))
		return ""
	}
	return runID
}

// UpdateRunStatus updates the status of a run.
func (p *PersistenceStores) UpdateRunStatus(ctx context.Context, runID, status string, output *RunOutputDoc, errMsg string) error {
	if p.runStore == nil || runID == "" {
		return nil
	}
	return p.runStore.UpdateStatus(ctx, runID, status, output, errMsg)
}

// RestoreConversation restores conversation history from the store.
func (p *PersistenceStores) RestoreConversation(ctx context.Context, conversationID string) []llm.Message {
	if p.conversationStore == nil || conversationID == "" {
		return nil
	}
	conv, err := p.conversationStore.GetByID(ctx, conversationID)
	if err != nil {
		p.logger.Debug("conversation not found or error, starting fresh",
			zap.String("conversation_id", conversationID),
			zap.Error(err),
		)
		return nil
	}
	if conv == nil {
		return nil
	}
	var msgs []llm.Message
	for _, msg := range conv.Messages {
		msgs = append(msgs, llm.Message{
			Role:    llm.Role(msg.Role),
			Content: msg.Content,
		})
	}
	p.logger.Debug("restored conversation history",
		zap.String("conversation_id", conversationID),
		zap.Int("messages", len(msgs)),
	)
	return msgs
}

// PersistConversation saves user input and agent output to ConversationStore.
func (p *PersistenceStores) PersistConversation(ctx context.Context, conversationID, agentID, tenantID, userID, inputContent, outputContent string) {
	if p.conversationStore == nil || conversationID == "" {
		return
	}

	now := time.Now()
	newMsgs := []ConversationMessage{
		{Role: string(llm.RoleUser), Content: inputContent, Timestamp: now},
		{Role: string(llm.RoleAssistant), Content: outputContent, Timestamp: now},
	}

	// Try to append to existing conversation first.
	appendErr := p.conversationStore.AppendMessages(ctx, conversationID, newMsgs)
	if appendErr == nil {
		return
	}

	// AppendMessages failed — attempt to create a new conversation.
	doc := &ConversationDoc{
		ID:       conversationID,
		AgentID:  agentID,
		TenantID: tenantID,
		UserID:   userID,
		Messages: newMsgs,
	}
	if createErr := p.conversationStore.Create(ctx, doc); createErr != nil {
		p.logger.Warn("failed to persist conversation",
			zap.String("conversation_id", conversationID),
			zap.NamedError("append_err", appendErr),
			zap.NamedError("create_err", createErr),
		)
	}
}
