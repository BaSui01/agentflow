package agent_test

import (
	"context"

	agentpkg "github.com/BaSui01/agentflow/agent"
	agentcheckpoint "github.com/BaSui01/agentflow/agent/persistence/checkpoint"
	"go.uber.org/zap"
)

type fileCheckpointStoreAdapter struct {
	inner *agentcheckpoint.FileCheckpointStore
}

func newFileCheckpointStoreAdapter(dir string, logger *zap.Logger) (*fileCheckpointStoreAdapter, error) {
	inner, err := agentcheckpoint.NewFileCheckpointStore(dir, logger)
	if err != nil {
		return nil, err
	}
	return &fileCheckpointStoreAdapter{inner: inner}, nil
}

func (s *fileCheckpointStoreAdapter) Save(ctx context.Context, checkpoint *agentpkg.Checkpoint) error {
	return s.inner.Save(ctx, toPersistentCheckpoint(checkpoint))
}

func (s *fileCheckpointStoreAdapter) Load(ctx context.Context, checkpointID string) (*agentpkg.Checkpoint, error) {
	checkpoint, err := s.inner.Load(ctx, checkpointID)
	if err != nil {
		return nil, err
	}
	return fromPersistentCheckpoint(checkpoint), nil
}

func (s *fileCheckpointStoreAdapter) LoadLatest(ctx context.Context, threadID string) (*agentpkg.Checkpoint, error) {
	checkpoint, err := s.inner.LoadLatest(ctx, threadID)
	if err != nil {
		return nil, err
	}
	return fromPersistentCheckpoint(checkpoint), nil
}

func (s *fileCheckpointStoreAdapter) List(ctx context.Context, threadID string, limit int) ([]*agentpkg.Checkpoint, error) {
	checkpoints, err := s.inner.List(ctx, threadID, limit)
	if err != nil {
		return nil, err
	}
	converted := make([]*agentpkg.Checkpoint, 0, len(checkpoints))
	for _, checkpoint := range checkpoints {
		converted = append(converted, fromPersistentCheckpoint(checkpoint))
	}
	return converted, nil
}

func (s *fileCheckpointStoreAdapter) Delete(ctx context.Context, checkpointID string) error {
	return s.inner.Delete(ctx, checkpointID)
}

func (s *fileCheckpointStoreAdapter) DeleteThread(ctx context.Context, threadID string) error {
	return s.inner.DeleteThread(ctx, threadID)
}

func (s *fileCheckpointStoreAdapter) LoadVersion(ctx context.Context, threadID string, version int) (*agentpkg.Checkpoint, error) {
	checkpoint, err := s.inner.LoadVersion(ctx, threadID, version)
	if err != nil {
		return nil, err
	}
	return fromPersistentCheckpoint(checkpoint), nil
}

func (s *fileCheckpointStoreAdapter) ListVersions(ctx context.Context, threadID string) ([]agentpkg.CheckpointVersion, error) {
	versions, err := s.inner.ListVersions(ctx, threadID)
	if err != nil {
		return nil, err
	}
	converted := make([]agentpkg.CheckpointVersion, 0, len(versions))
	for _, version := range versions {
		converted = append(converted, agentpkg.CheckpointVersion{
			Version:   version.Version,
			ID:        version.ID,
			CreatedAt: version.CreatedAt,
			State:     agentpkg.State(version.State),
			Summary:   version.Summary,
		})
	}
	return converted, nil
}

func (s *fileCheckpointStoreAdapter) Rollback(ctx context.Context, threadID string, version int) error {
	return s.inner.Rollback(ctx, threadID, version)
}

func toPersistentCheckpoint(checkpoint *agentpkg.Checkpoint) *agentcheckpoint.Checkpoint {
	if checkpoint == nil {
		return nil
	}
	converted := &agentcheckpoint.Checkpoint{
		ID:                  checkpoint.ID,
		ThreadID:            checkpoint.ThreadID,
		AgentID:             checkpoint.AgentID,
		LoopStateID:         checkpoint.LoopStateID,
		RunID:               checkpoint.RunID,
		Goal:                checkpoint.Goal,
		AcceptanceCriteria:  append([]string(nil), checkpoint.AcceptanceCriteria...),
		UnresolvedItems:     append([]string(nil), checkpoint.UnresolvedItems...),
		RemainingRisks:      append([]string(nil), checkpoint.RemainingRisks...),
		CurrentPlanID:       checkpoint.CurrentPlanID,
		PlanVersion:         checkpoint.PlanVersion,
		CurrentStepID:       checkpoint.CurrentStepID,
		ValidationStatus:    string(checkpoint.ValidationStatus),
		ValidationSummary:   checkpoint.ValidationSummary,
		ObservationsSummary: checkpoint.ObservationsSummary,
		LastOutputSummary:   checkpoint.LastOutputSummary,
		LastError:           checkpoint.LastError,
		Version:             checkpoint.Version,
		State:               string(checkpoint.State),
		Messages:            toPersistentMessages(checkpoint.Messages),
		Metadata:            cloneAnyMap(checkpoint.Metadata),
		CreatedAt:           checkpoint.CreatedAt,
		ParentID:            checkpoint.ParentID,
		ExecutionContext:    toPersistentExecutionContext(checkpoint.ExecutionContext),
	}
	return converted
}

func fromPersistentCheckpoint(checkpoint *agentcheckpoint.Checkpoint) *agentpkg.Checkpoint {
	if checkpoint == nil {
		return nil
	}
	return &agentpkg.Checkpoint{
		ID:                  checkpoint.ID,
		ThreadID:            checkpoint.ThreadID,
		AgentID:             checkpoint.AgentID,
		LoopStateID:         checkpoint.LoopStateID,
		RunID:               checkpoint.RunID,
		Goal:                checkpoint.Goal,
		AcceptanceCriteria:  append([]string(nil), checkpoint.AcceptanceCriteria...),
		UnresolvedItems:     append([]string(nil), checkpoint.UnresolvedItems...),
		RemainingRisks:      append([]string(nil), checkpoint.RemainingRisks...),
		CurrentPlanID:       checkpoint.CurrentPlanID,
		PlanVersion:         checkpoint.PlanVersion,
		CurrentStepID:       checkpoint.CurrentStepID,
		ValidationStatus:    agentpkg.LoopValidationStatus(checkpoint.ValidationStatus),
		ValidationSummary:   checkpoint.ValidationSummary,
		ObservationsSummary: checkpoint.ObservationsSummary,
		LastOutputSummary:   checkpoint.LastOutputSummary,
		LastError:           checkpoint.LastError,
		Version:             checkpoint.Version,
		State:               agentpkg.State(checkpoint.State),
		Messages:            fromPersistentMessages(checkpoint.Messages),
		Metadata:            cloneAnyMap(checkpoint.Metadata),
		CreatedAt:           checkpoint.CreatedAt,
		ParentID:            checkpoint.ParentID,
		ExecutionContext:    fromPersistentExecutionContext(checkpoint.ExecutionContext),
	}
}

func toPersistentMessages(messages []agentpkg.CheckpointMessage) []agentcheckpoint.CheckpointMessage {
	if len(messages) == 0 {
		return nil
	}
	converted := make([]agentcheckpoint.CheckpointMessage, 0, len(messages))
	for _, message := range messages {
		converted = append(converted, agentcheckpoint.CheckpointMessage{
			Role:      message.Role,
			Content:   message.Content,
			ToolCalls: toPersistentToolCalls(message.ToolCalls),
			Metadata:  cloneAnyMap(message.Metadata),
		})
	}
	return converted
}

func fromPersistentMessages(messages []agentcheckpoint.CheckpointMessage) []agentpkg.CheckpointMessage {
	if len(messages) == 0 {
		return nil
	}
	converted := make([]agentpkg.CheckpointMessage, 0, len(messages))
	for _, message := range messages {
		converted = append(converted, agentpkg.CheckpointMessage{
			Role:      message.Role,
			Content:   message.Content,
			ToolCalls: fromPersistentToolCalls(message.ToolCalls),
			Metadata:  cloneAnyMap(message.Metadata),
		})
	}
	return converted
}

func toPersistentToolCalls(calls []agentpkg.CheckpointToolCall) []agentcheckpoint.CheckpointToolCall {
	if len(calls) == 0 {
		return nil
	}
	converted := make([]agentcheckpoint.CheckpointToolCall, 0, len(calls))
	for _, call := range calls {
		converted = append(converted, agentcheckpoint.CheckpointToolCall{
			ID:        call.ID,
			Name:      call.Name,
			Arguments: append([]byte(nil), call.Arguments...),
			Result:    append([]byte(nil), call.Result...),
			Error:     call.Error,
		})
	}
	return converted
}

func fromPersistentToolCalls(calls []agentcheckpoint.CheckpointToolCall) []agentpkg.CheckpointToolCall {
	if len(calls) == 0 {
		return nil
	}
	converted := make([]agentpkg.CheckpointToolCall, 0, len(calls))
	for _, call := range calls {
		converted = append(converted, agentpkg.CheckpointToolCall{
			ID:        call.ID,
			Name:      call.Name,
			Arguments: append([]byte(nil), call.Arguments...),
			Result:    append([]byte(nil), call.Result...),
			Error:     call.Error,
		})
	}
	return converted
}

func toPersistentExecutionContext(ctx *agentpkg.ExecutionContext) *agentcheckpoint.ExecutionContext {
	if ctx == nil {
		return nil
	}
	return &agentcheckpoint.ExecutionContext{
		WorkflowID:          ctx.WorkflowID,
		CurrentNode:         ctx.CurrentNode,
		NodeResults:         cloneAnyMap(ctx.NodeResults),
		Variables:           cloneAnyMap(ctx.Variables),
		LoopStateID:         ctx.LoopStateID,
		RunID:               ctx.RunID,
		AgentID:             ctx.AgentID,
		Goal:                ctx.Goal,
		AcceptanceCriteria:  append([]string(nil), ctx.AcceptanceCriteria...),
		UnresolvedItems:     append([]string(nil), ctx.UnresolvedItems...),
		RemainingRisks:      append([]string(nil), ctx.RemainingRisks...),
		CurrentPlanID:       ctx.CurrentPlanID,
		PlanVersion:         ctx.PlanVersion,
		CurrentStepID:       ctx.CurrentStepID,
		ValidationStatus:    string(ctx.ValidationStatus),
		ValidationSummary:   ctx.ValidationSummary,
		ObservationsSummary: ctx.ObservationsSummary,
		LastOutputSummary:   ctx.LastOutputSummary,
		LastError:           ctx.LastError,
	}
}

func fromPersistentExecutionContext(ctx *agentcheckpoint.ExecutionContext) *agentpkg.ExecutionContext {
	if ctx == nil {
		return nil
	}
	return &agentpkg.ExecutionContext{
		WorkflowID:          ctx.WorkflowID,
		CurrentNode:         ctx.CurrentNode,
		NodeResults:         cloneAnyMap(ctx.NodeResults),
		Variables:           cloneAnyMap(ctx.Variables),
		LoopStateID:         ctx.LoopStateID,
		RunID:               ctx.RunID,
		AgentID:             ctx.AgentID,
		Goal:                ctx.Goal,
		AcceptanceCriteria:  append([]string(nil), ctx.AcceptanceCriteria...),
		UnresolvedItems:     append([]string(nil), ctx.UnresolvedItems...),
		RemainingRisks:      append([]string(nil), ctx.RemainingRisks...),
		CurrentPlanID:       ctx.CurrentPlanID,
		PlanVersion:         ctx.PlanVersion,
		CurrentStepID:       ctx.CurrentStepID,
		ValidationStatus:    agentpkg.LoopValidationStatus(ctx.ValidationStatus),
		ValidationSummary:   ctx.ValidationSummary,
		ObservationsSummary: ctx.ObservationsSummary,
		LastOutputSummary:   ctx.LastOutputSummary,
		LastError:           ctx.LastError,
	}
}

func cloneAnyMap(values map[string]any) map[string]any {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}
