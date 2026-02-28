package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/agent/guardrails"
	"github.com/BaSui01/agentflow/llm"

	"go.uber.org/zap"
)

// =============================================================================
// Step 1: StateTransitionStep — transition to Running, defer back to Ready
// =============================================================================

// StateTransitionStep transitions the agent to Running and defers recovery to Ready.
type StateTransitionStep struct{}

func (s *StateTransitionStep) Name() string { return "state_transition" }

func (s *StateTransitionStep) Execute(ctx context.Context, pc *PipelineContext, next StepFunc) error {
	b := pc.agent

	if err := b.Transition(ctx, StateRunning); err != nil {
		return err
	}

	// Ensure we transition back to Ready when this step's scope exits,
	// regardless of whether downstream steps succeed or fail.
	defer func() {
		if err := b.Transition(context.Background(), StateReady); err != nil {
			b.logger.Error("failed to transition to ready", zap.Error(err))
		}
	}()

	return next(ctx, pc)
}

// =============================================================================
// Step 2: ResourceLoadStep — load prompt, record run, restore conversation
// =============================================================================

// ResourceLoadStep loads prompts from store, records the run, and restores conversation history.
type ResourceLoadStep struct{}

func (s *ResourceLoadStep) Name() string { return "resource_load" }

func (s *ResourceLoadStep) Execute(ctx context.Context, pc *PipelineContext, next StepFunc) error {
	b := pc.agent

	// 2a. PromptStore: load active prompt from MongoDB if available
	b.loadPromptFromStore(ctx)

	// 2b. RunStore: record execution start
	pc.RunID = b.persistence.RecordRun(ctx, b.config.ID, pc.Input.TenantID, pc.Input.TraceID, pc.Input.Content, pc.StartTime)

	// 2c. ConversationStore: restore conversation history
	pc.ConversationID = pc.Input.ChannelID
	pc.RestoredMessages = b.persistence.RestoreConversation(ctx, pc.ConversationID)

	b.logger.Info("executing task",
		zap.String("trace_id", pc.Input.TraceID),
		zap.String("agent_id", b.config.ID),
		zap.String("agent_type", string(b.config.Type)),
	)

	// Wrap downstream in panic recovery + run-status bookkeeping.
	var execErr error
	func() {
		defer func() {
			if pc.RunID != "" {
				if r := recover(); r != nil {
					_ = b.persistence.UpdateRunStatus(ctx, pc.RunID, "failed", nil, fmt.Sprintf("panic: %v", r))
					b.logger.Error("panic during execution, run marked as failed",
						zap.Any("panic", r), zap.String("run_id", pc.RunID))
					panic(r) // re-panic after recording
				}
			}
		}()
		execErr = next(ctx, pc)
	}()

	// If downstream returned an error, mark the run as failed.
	if execErr != nil && pc.RunID != "" {
		if updateErr := b.persistence.UpdateRunStatus(ctx, pc.RunID, "failed", nil, execErr.Error()); updateErr != nil {
			b.logger.Warn("failed to mark run as failed", zap.Error(updateErr))
		}
	}

	return execErr
}

// =============================================================================
// Step 3: InputGuardrailStep — validate input
// =============================================================================

// InputGuardrailStep validates the input content using guardrails.
type InputGuardrailStep struct{}

func (s *InputGuardrailStep) Name() string { return "input_guardrail" }

func (s *InputGuardrailStep) Execute(ctx context.Context, pc *PipelineContext, next StepFunc) error {
	b := pc.agent

	if b.guardrails.Enabled() {
		validationResult, err := b.guardrails.ValidateInput(ctx, pc.Input.Content)
		if err != nil {
			b.logger.Error("input validation error", zap.Error(err))
			return fmt.Errorf("input validation error: %w", err)
		}

		if !validationResult.Valid {
			b.logger.Warn("input validation failed",
				zap.String("trace_id", pc.Input.TraceID),
				zap.Any("errors", validationResult.Errors),
			)

			failureAction := guardrails.FailureActionReject
			if b.config.Guardrails != nil {
				failureAction = b.config.Guardrails.OnInputFailure
			}

			switch failureAction {
			case guardrails.FailureActionReject:
				return &GuardrailsError{
					Type:    GuardrailsErrorTypeInput,
					Message: "input validation failed",
					Errors:  validationResult.Errors,
				}
			case guardrails.FailureActionWarn:
				b.logger.Warn("input validation warning, continuing execution",
					zap.Any("warnings", validationResult.Errors),
				)
			}
		}
	}

	return next(ctx, pc)
}

// =============================================================================
// Step 4: MemoryLoadStep — load recent memory as context messages
// =============================================================================

// MemoryLoadStep loads recent memory records into context messages.
type MemoryLoadStep struct{}

func (s *MemoryLoadStep) Name() string { return "memory_load" }

func (s *MemoryLoadStep) Execute(ctx context.Context, pc *PipelineContext, next StepFunc) error {
	pc.ContextMessages = pc.agent.memoryCache.GetRecentMessages()
	return next(ctx, pc)
}

// =============================================================================
// Step 5: MessageBuildStep — assemble system prompt + context + restored + user
// =============================================================================

// MessageBuildStep assembles the full message list for the LLM call.
type MessageBuildStep struct{}

func (s *MessageBuildStep) Name() string { return "message_build" }

func (s *MessageBuildStep) Execute(ctx context.Context, pc *PipelineContext, next StepFunc) error {
	b := pc.agent

	pc.Messages = []llm.Message{
		{
			Role:    llm.RoleSystem,
			Content: b.config.PromptBundle.RenderSystemPromptWithVars(pc.Input.Variables),
		},
	}
	pc.Messages = append(pc.Messages, pc.ContextMessages...)
	if len(pc.RestoredMessages) > 0 {
		pc.Messages = append(pc.Messages, pc.RestoredMessages...)
	}
	pc.Messages = append(pc.Messages, llm.Message{
		Role:    llm.RoleUser,
		Content: pc.Input.Content,
	})

	return next(ctx, pc)
}

// =============================================================================
// Step 6: LLMExecutionStep — call ChatCompletion with output validation retry
// =============================================================================

// LLMExecutionStep calls the LLM and handles output guardrail validation with retries.
type LLMExecutionStep struct{}

func (s *LLMExecutionStep) Name() string { return "llm_execution" }

func (s *LLMExecutionStep) Execute(ctx context.Context, pc *PipelineContext, next StepFunc) error {
	b := pc.agent

	maxRetries := 0
	if b.config.Guardrails != nil {
		maxRetries = b.config.Guardrails.MaxRetries
	}

	var lastValidationResult *guardrails.ValidationResult
	var choice llm.ChatChoice

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			b.logger.Info("retrying execution due to output validation failure",
				zap.Int("attempt", attempt),
				zap.String("trace_id", pc.Input.TraceID),
			)
			if lastValidationResult != nil {
				feedbackMsg := b.buildValidationFeedbackMessage(lastValidationResult)
				pc.Messages = append(pc.Messages, llm.Message{
					Role:    llm.RoleUser,
					Content: feedbackMsg,
				})
			}
		}

		resp, err := b.ChatCompletion(ctx, pc.Messages)
		if err != nil {
			b.logger.Error("execution failed", zap.Error(err), zap.String("trace_id", pc.Input.TraceID))
			return fmt.Errorf("execution failed: %w", err)
		}
		pc.Response = resp

		var choiceErr error
		choice, choiceErr = llm.FirstChoice(resp)
		if choiceErr != nil {
			return fmt.Errorf("execution returned no choices: %w", choiceErr)
		}
		pc.OutputContent = choice.Message.Content

		// Output validation (guardrails)
		if b.guardrails.Enabled() {
			filteredContent, valResult, valErr := b.guardrails.ValidateAndFilterOutput(ctx, pc.OutputContent)
			if valErr != nil {
				b.logger.Error("output validation error", zap.Error(valErr))
				return fmt.Errorf("output validation error: %w", valErr)
			}
			lastValidationResult = valResult

			if !valResult.Valid {
				b.logger.Warn("output validation failed",
					zap.String("trace_id", pc.Input.TraceID),
					zap.Int("attempt", attempt),
					zap.Any("errors", valResult.Errors),
				)

				failureAction := guardrails.FailureActionReject
				if b.config.Guardrails != nil {
					failureAction = b.config.Guardrails.OnOutputFailure
				}

				if failureAction == guardrails.FailureActionRetry && attempt < maxRetries {
					continue
				}

				switch failureAction {
				case guardrails.FailureActionReject:
					return &GuardrailsError{
						Type:    GuardrailsErrorTypeOutput,
						Message: "output validation failed",
						Errors:  valResult.Errors,
					}
				case guardrails.FailureActionWarn:
					b.logger.Warn("output validation warning, using filtered content")
					pc.OutputContent = filteredContent
				case guardrails.FailureActionRetry:
					return &GuardrailsError{
						Type:    GuardrailsErrorTypeOutput,
						Message: fmt.Sprintf("output validation failed after %d retries", maxRetries),
						Errors:  valResult.Errors,
					}
				}
			} else {
				pc.OutputContent = filteredContent
			}
		}

		break
	}

	pc.FinishReason = choice.FinishReason
	pc.TokensUsed = pc.Response.Usage.TotalTokens
	pc.Metadata["model"] = pc.Response.Model
	pc.Metadata["provider"] = pc.Response.Provider

	return next(ctx, pc)
}

// =============================================================================
// Step 7: MemorySaveStep — save user input and agent response to memory
// =============================================================================

// MemorySaveStep saves the user input and agent response to the memory cache.
type MemorySaveStep struct{}

func (s *MemorySaveStep) Name() string { return "memory_save" }

func (s *MemorySaveStep) Execute(ctx context.Context, pc *PipelineContext, next StepFunc) error {
	b := pc.agent

	skipBaseMemory := false
	if pc.Input.Context != nil {
		if skip, ok := pc.Input.Context["_skip_base_memory"].(bool); ok && skip {
			skipBaseMemory = true
		}
	}
	if b.memoryCache.HasMemory() && !skipBaseMemory {
		if err := b.SaveMemory(ctx, pc.Input.Content, MemoryShortTerm, map[string]any{
			"trace_id": pc.Input.TraceID,
			"role":     "user",
		}); err != nil {
			b.logger.Warn("failed to save user input to memory", zap.Error(err))
		}
		if err := b.SaveMemory(ctx, pc.OutputContent, MemoryShortTerm, map[string]any{
			"trace_id": pc.Input.TraceID,
			"role":     "assistant",
		}); err != nil {
			b.logger.Warn("failed to save response to memory", zap.Error(err))
		}
	}

	return next(ctx, pc)
}

// =============================================================================
// Step 8: PersistStep — persist conversation + update run status
// =============================================================================

// PersistStep persists the conversation and updates the run status to completed.
type PersistStep struct{}

func (s *PersistStep) Name() string { return "persist" }

func (s *PersistStep) Execute(ctx context.Context, pc *PipelineContext, next StepFunc) error {
	b := pc.agent

	// 8a. ConversationStore: persist conversation
	b.persistence.PersistConversation(ctx, pc.ConversationID, b.config.ID, pc.Input.TenantID, pc.Input.UserID, pc.Input.Content, pc.OutputContent)

	// 8b. RunStore: update run status
	if pc.RunID != "" {
		outputDoc := &RunOutputDoc{
			Content:      pc.OutputContent,
			TokensUsed:   pc.TokensUsed,
			Cost:         0,
			FinishReason: pc.FinishReason,
		}
		if err := b.persistence.UpdateRunStatus(ctx, pc.RunID, "completed", outputDoc, ""); err != nil {
			b.logger.Warn("failed to update run status", zap.Error(err))
		}
	}

	b.logger.Info("execution completed",
		zap.String("trace_id", pc.Input.TraceID),
		zap.Duration("duration", time.Since(pc.StartTime)),
		zap.Int("tokens_used", pc.TokensUsed),
	)

	return next(ctx, pc)
}
