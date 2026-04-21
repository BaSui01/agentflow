package multiagent

import (
	"context"
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/agent"
	"go.uber.org/zap"
)

// ScopedStores wraps agent.PersistenceStores and prefixes all operations with a sub-agent scope.
// This ensures sub-agent conversation/run/prompt data does not pollute the parent agent's stores.
type ScopedStores struct {
	inner   *agent.PersistenceStores
	agentID string
	logger  *zap.Logger
}

// NewScopedStores creates a ScopedStores for the given sub-agent.
func NewScopedStores(inner *agent.PersistenceStores, agentID string, logger *zap.Logger) *ScopedStores {
	return &ScopedStores{inner: inner, agentID: agentID, logger: logger}
}

func (s *ScopedStores) scopedKey(key string) string {
	return s.agentID + "/" + key
}

// RecordRun records a scoped run.
func (s *ScopedStores) RecordRun(ctx context.Context, tenantID, traceID, input string, startTime time.Time) string {
	return s.inner.RecordRun(ctx, s.agentID, tenantID, traceID, input, startTime)
}

// UpdateRunStatus updates a scoped run status.
func (s *ScopedStores) UpdateRunStatus(ctx context.Context, runID, status string, output *agent.RunOutputDoc, errMsg string) error {
	return s.inner.UpdateRunStatus(ctx, runID, status, output, errMsg)
}

// PersistConversation saves conversation with scoped conversation ID.
func (s *ScopedStores) PersistConversation(ctx context.Context, conversationID, tenantID, userID, inputContent, outputContent string) {
	scopedConvID := s.scopedKey(conversationID)
	s.inner.PersistConversation(ctx, scopedConvID, s.agentID, tenantID, userID, inputContent, outputContent)
}

// RestoreConversation restores conversation from scoped ID.
func (s *ScopedStores) RestoreConversation(ctx context.Context, conversationID string) []interface{} {
	scopedConvID := s.scopedKey(conversationID)
	msgs := s.inner.RestoreConversation(ctx, scopedConvID)
	result := make([]interface{}, len(msgs))
	for i, m := range msgs {
		result[i] = m
	}
	return result
}

// LoadPrompt loads prompt from the scoped agent type.
func (s *ScopedStores) LoadPrompt(ctx context.Context, agentType, name, tenantID string) *agent.PromptDocument {
	scopedType := fmt.Sprintf("%s/%s", s.agentID, agentType)
	return s.inner.LoadPrompt(ctx, scopedType, name, tenantID)
}
