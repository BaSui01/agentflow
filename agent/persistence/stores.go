package persistence

import (
	"context"
	"fmt"
	"time"

	promptcap "github.com/BaSui01/agentflow/agent/capabilities/prompt"
	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

type PromptStoreProvider interface {
	GetActive(ctx context.Context, agentType, name, tenantID string) (PromptDocument, error)
}

type PromptDocument struct {
	Version     string                 `json:"version"`
	System      promptcap.SystemPrompt `json:"system"`
	Constraints []string               `json:"constraints,omitempty"`
}

type ConversationStoreProvider interface {
	Create(ctx context.Context, doc *ConversationDoc) error
	GetByID(ctx context.Context, id string) (*ConversationDoc, error)
	AppendMessages(ctx context.Context, conversationID string, msgs []ConversationMessage) error
	List(ctx context.Context, tenantID, parentID string, page, pageSize int) ([]*ConversationDoc, int64, error)
	Update(ctx context.Context, id string, updates ConversationUpdate) error
	Delete(ctx context.Context, id string) error
	DeleteByParentID(ctx context.Context, tenantID, parentID string) error
	GetMessages(ctx context.Context, conversationID string, offset, limit int) ([]ConversationMessage, int64, error)
	DeleteMessage(ctx context.Context, conversationID, messageID string) error
	ClearMessages(ctx context.Context, conversationID string) error
	Archive(ctx context.Context, id string) error
}

type ConversationDoc struct {
	ID       string                `json:"id"`
	ParentID string                `json:"parent_id,omitempty"`
	AgentID  string                `json:"agent_id"`
	TenantID string                `json:"tenant_id"`
	UserID   string                `json:"user_id"`
	Title    string                `json:"title,omitempty"`
	Messages []ConversationMessage `json:"messages"`
}

type ConversationMessage struct {
	ID        string    `json:"id,omitempty"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

type ConversationUpdate struct {
	Title    *string        `json:"title,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type RunStoreProvider interface {
	RecordRun(ctx context.Context, doc *RunDoc) error
	UpdateStatus(ctx context.Context, id, status string, output *RunOutputDoc, errMsg string) error
}

type RunDoc struct {
	ID        string    `json:"id"`
	AgentID   string    `json:"agent_id"`
	TenantID  string    `json:"tenant_id"`
	TraceID   string    `json:"trace_id"`
	Status    string    `json:"status"`
	Input     string    `json:"input"`
	StartTime time.Time `json:"start_time"`
}

type RunOutputDoc struct {
	Content      string  `json:"content"`
	TokensUsed   int     `json:"tokens_used"`
	Cost         float64 `json:"cost"`
	FinishReason string  `json:"finish_reason"`
}

const defaultMaxRestoreMessages = 200

type PersistenceStores struct {
	promptStore        PromptStoreProvider
	conversationStore  ConversationStoreProvider
	runStore           RunStoreProvider
	logger             *zap.Logger
	maxRestoreMessages int
}

func NewPersistenceStores(logger *zap.Logger) *PersistenceStores {
	return &PersistenceStores{logger: logger}
}

func (p *PersistenceStores) SetPromptStore(store PromptStoreProvider) {
	p.promptStore = store
}

func (p *PersistenceStores) SetConversationStore(store ConversationStoreProvider) {
	p.conversationStore = store
}

func (p *PersistenceStores) SetRunStore(store RunStoreProvider) {
	p.runStore = store
}

func (p *PersistenceStores) SetMaxRestoreMessages(n int) {
	p.maxRestoreMessages = n
}

func (p *PersistenceStores) PromptStore() PromptStoreProvider { return p.promptStore }

func (p *PersistenceStores) ConversationStore() ConversationStoreProvider { return p.conversationStore }

func (p *PersistenceStores) RunStore() RunStoreProvider { return p.runStore }

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

func (p *PersistenceStores) UpdateRunStatus(ctx context.Context, runID, status string, output *RunOutputDoc, errMsg string) error {
	if p.runStore == nil || runID == "" {
		return nil
	}
	return p.runStore.UpdateStatus(ctx, runID, status, output, errMsg)
}

func (p *PersistenceStores) RestoreConversation(ctx context.Context, conversationID string) []types.Message {
	if p.conversationStore == nil || conversationID == "" {
		return nil
	}

	limit := p.maxRestoreMessages
	if limit <= 0 {
		limit = defaultMaxRestoreMessages
	}

	_, total, err := p.conversationStore.GetMessages(ctx, conversationID, 0, 1)
	if err != nil || total == 0 {
		p.logger.Debug("conversation not found or empty, starting fresh",
			zap.String("conversation_id", conversationID),
			zap.Error(err),
		)
		return nil
	}

	offset := int(total) - limit
	if offset < 0 {
		offset = 0
	}

	raw, _, err := p.conversationStore.GetMessages(ctx, conversationID, offset, limit)
	if err != nil {
		p.logger.Debug("failed to restore conversation messages",
			zap.String("conversation_id", conversationID),
			zap.Error(err),
		)
		return nil
	}

	msgs := make([]types.Message, 0, len(raw))
	for _, msg := range raw {
		msgs = append(msgs, types.Message{
			Role:    types.Role(msg.Role),
			Content: msg.Content,
		})
	}
	p.logger.Debug("restored conversation history",
		zap.String("conversation_id", conversationID),
		zap.Int("messages", len(msgs)),
		zap.Int64("total", total),
	)
	return msgs
}

func (p *PersistenceStores) PersistConversation(ctx context.Context, conversationID, agentID, tenantID, userID, inputContent, outputContent string) {
	if p.conversationStore == nil || conversationID == "" {
		return
	}

	now := time.Now()
	newMsgs := []ConversationMessage{
		{Role: string(llm.RoleUser), Content: inputContent, Timestamp: now},
		{Role: string(llm.RoleAssistant), Content: outputContent, Timestamp: now},
	}

	appendErr := p.conversationStore.AppendMessages(ctx, conversationID, newMsgs)
	if appendErr == nil {
		return
	}

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

type ScopedPersistenceStores struct {
	inner *PersistenceStores
	scope string
}

func NewScopedPersistenceStores(inner *PersistenceStores, scope string) *ScopedPersistenceStores {
	return &ScopedPersistenceStores{inner: inner, scope: scope}
}

func (s *ScopedPersistenceStores) Scope() string { return s.scope }

func ScopedID(scope, id string) string {
	if id == "" {
		return ""
	}
	return scope + "/" + id
}

func (s *ScopedPersistenceStores) scopedID(id string) string {
	return ScopedID(s.scope, id)
}

func (s *ScopedPersistenceStores) RecordRun(ctx context.Context, agentID, tenantID, traceID, input string, startTime time.Time) string {
	return s.inner.RecordRun(ctx, s.scopedID(agentID), tenantID, traceID, input, startTime)
}

func (s *ScopedPersistenceStores) UpdateRunStatus(ctx context.Context, runID, status string, output *RunOutputDoc, errMsg string) error {
	return s.inner.UpdateRunStatus(ctx, runID, status, output, errMsg)
}

func (s *ScopedPersistenceStores) RestoreConversation(ctx context.Context, conversationID string) []types.Message {
	return s.inner.RestoreConversation(ctx, s.scopedID(conversationID))
}

func (s *ScopedPersistenceStores) PersistConversation(ctx context.Context, conversationID, agentID, tenantID, userID, inputContent, outputContent string) {
	s.inner.PersistConversation(ctx, s.scopedID(conversationID), s.scopedID(agentID), tenantID, userID, inputContent, outputContent)
}

func (s *ScopedPersistenceStores) LoadPrompt(ctx context.Context, agentType, name, tenantID string) *PromptDocument {
	return s.inner.LoadPrompt(ctx, agentType, name, tenantID)
}
