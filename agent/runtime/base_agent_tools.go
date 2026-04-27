package runtime

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/agent/capabilities/guardrails"
	"github.com/BaSui01/agentflow/types"

	"go.uber.org/zap"
)

func (b *BaseAgent) buildValidationFeedbackMessage(result *guardrails.ValidationResult) string {
	var sb strings.Builder
	sb.WriteString("Your previous response failed validation. Please regenerate your response addressing the following issues:\n")
	for _, err := range result.Errors {
		sb.WriteString(fmt.Sprintf("- %s: %s\n", err.Code, err.Message))
	}
	sb.WriteString("\nPlease provide a corrected response.")
	return sb.String()
}

func (b *BaseAgent) Observe(ctx context.Context, feedback *Feedback) error {
	b.logger.Info("observing feedback",
		zap.String("agent_id", b.config.Core.ID),
		zap.String("feedback_type", feedback.Type),
	)

	metadata := map[string]any{
		"feedback_type": feedback.Type,
		"timestamp":     time.Now(),
	}
	for k, v := range feedback.Data {
		metadata[k] = v
	}

	switch {
	case b.memoryFacade != nil && b.memoryFacade.HasEnhanced():
		if err := b.memoryFacade.Enhanced().SaveShortTerm(ctx, b.ID(), feedback.Content, metadata); err != nil {
			b.logger.Error("failed to save feedback to enhanced memory", zap.Error(err))
			return NewErrorWithCause(types.ErrAgentExecution, "failed to save feedback", err)
		}
		b.memoryFacade.RecordEpisode(ctx, &types.EpisodicEvent{
			AgentID:   b.ID(),
			Type:      "feedback",
			Content:   feedback.Content,
			Context:   metadata,
			Timestamp: time.Now(),
		})
	case b.memory != nil:
		if err := b.SaveMemory(ctx, feedback.Content, MemoryLongTerm, metadata); err != nil {
			b.logger.Error("failed to save feedback to memory", zap.Error(err))
			return NewErrorWithCause(types.ErrAgentExecution, "failed to save feedback", err)
		}
	}

	if b.bus != nil {
		b.bus.Publish(&FeedbackEvent{
			AgentID_:     b.config.Core.ID,
			FeedbackType: feedback.Type,
			Content:      feedback.Content,
			Data:         feedback.Data,
			Timestamp_:   time.Now(),
		})
	}

	b.logger.Info("feedback observed successfully",
		zap.String("agent_id", b.config.Core.ID),
		zap.String("feedback_type", feedback.Type),
	)

	return nil
}

func (b *BaseAgent) collectContextMemory(values map[string]any) []string {
	var memoryContext []string
	appendValue := func(v string) {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			memoryContext = append(memoryContext, trimmed)
		}
	}

	b.recentMemoryMu.RLock()
	for _, mem := range b.recentMemory {
		if mem.Kind == MemoryShortTerm {
			appendValue(mem.Content)
		}
	}
	b.recentMemoryMu.RUnlock()

	if b.memoryFacade != nil {
		for _, item := range b.memoryFacade.LoadContext(context.Background(), b.ID()) {
			appendValue(item)
		}
	}

	if len(values) > 0 {
		if raw, ok := values["memory_context"].([]string); ok {
			for _, item := range raw {
				appendValue(item)
			}
		}
	}
	return memoryContext
}
