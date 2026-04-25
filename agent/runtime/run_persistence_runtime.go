package runtime

import (
	"context"
	"fmt"
	"time"

	agentcore "github.com/BaSui01/agentflow/agent/core"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

type runtimePersistenceSession struct {
	runID            string
	conversationID   string
	restoredMessages []types.Message
}

type runtimePersistenceCompletion struct {
	outputContent string
	tokensUsed    int
	cost          float64
	finishReason  string
}

func (b *BaseAgent) beginRuntimePersistence(ctx context.Context, input *Input, startTime time.Time) runtimePersistenceSession {
	if b == nil || b.persistence == nil || input == nil {
		return runtimePersistenceSession{}
	}
	conversationID := input.ChannelID
	return runtimePersistenceSession{
		runID: b.persistence.RecordRun(ctx,
			b.config.Core.ID,
			input.TenantID,
			input.TraceID,
			input.Content,
			startTime,
		),
		conversationID:   conversationID,
		restoredMessages: b.persistence.RestoreConversation(ctx, conversationID),
	}
}

func (b *BaseAgent) completeRuntimePersistence(ctx context.Context, session runtimePersistenceSession, input *Input, completion runtimePersistenceCompletion) {
	if b == nil || b.persistence == nil || input == nil {
		return
	}
	b.persistence.PersistConversation(ctx,
		session.conversationID,
		b.config.Core.ID,
		input.TenantID,
		input.UserID,
		input.Content,
		completion.outputContent,
	)

	if session.runID == "" {
		return
	}
	outputDoc := &RunOutputDoc{
		Content:      completion.outputContent,
		TokensUsed:   completion.tokensUsed,
		Cost:         completion.cost,
		FinishReason: completion.finishReason,
	}
	if err := b.persistence.UpdateRunStatus(ctx, session.runID, "completed", outputDoc, ""); err != nil {
		runtimePersistenceLogger(b).Warn("failed to update run status", zap.Error(err))
	}
}

func (b *BaseAgent) finishRuntimePersistenceOnExit(ctx context.Context, session runtimePersistenceSession, execErr *error) {
	if b == nil || b.persistence == nil || session.runID == "" {
		return
	}
	logger := runtimePersistenceLogger(b)
	if r := recover(); r != nil {
		panicErr := agentcore.PanicPayloadToError(r)
		if updateErr := b.persistence.UpdateRunStatus(ctx, session.runID, "failed", nil, fmt.Sprintf("panic: %v", r)); updateErr != nil {
			logger.Warn("failed to mark run as failed after panic", zap.Error(updateErr))
		}
		logger.Error("panic during execution, run marked as failed",
			zap.Any("panic", r),
			zap.Error(panicErr),
			zap.String("run_id", session.runID),
		)
		if execErr != nil && *execErr == nil {
			*execErr = NewErrorWithCause(types.ErrAgentExecution, "react execution panic", panicErr)
		}
	}
	if execErr != nil && *execErr != nil {
		if updateErr := b.persistence.UpdateRunStatus(ctx, session.runID, "failed", nil, (*execErr).Error()); updateErr != nil {
			logger.Warn("failed to mark run as failed", zap.Error(updateErr))
		}
	}
}

func runtimePersistenceLogger(b *BaseAgent) *zap.Logger {
	if b == nil || b.logger == nil {
		return zap.NewNop()
	}
	return b.logger
}
