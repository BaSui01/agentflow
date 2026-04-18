package agent

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	agentcontext "github.com/BaSui01/agentflow/agent/context"
	"github.com/BaSui01/agentflow/types"

	"github.com/BaSui01/agentflow/agent/guardrails"
	"github.com/BaSui01/agentflow/llm/observability"

	"go.uber.org/zap"
)

// defaultCostCalc is a package-level cost calculator for estimating LLM call costs.
var defaultCostCalc = observability.NewCostCalculator()

// Plan 生成执行计划
// 使用 LLM 分析任务并生成详细的执行步骤
func (b *BaseAgent) Plan(ctx context.Context, input *Input) (*PlanResult, error) {
	if !b.hasMainExecutionSurface() {
		return nil, ErrProviderNotSet
	}

	// 构建规划提示词
	planPrompt := fmt.Sprintf(`Plan the execution of this task for another agent.

Task:
%s

Use the %s tool to return the plan.

Requirements:
- Keep each step directly executable
- Prefer tool-first actions when tools are needed
- Mention dependencies or risks only when they affect execution
- Do not answer with prose outside the tool call`, input.Content, submitNumberedPlanTool)

	messages := []types.Message{
		{
			Role:    types.RoleSystem,
			Content: b.promptBundle.RenderSystemPromptWithVars(input.Variables),
		},
		{
			Role:    types.RoleUser,
			Content: planPrompt,
		},
	}

	pr, err := b.prepareChatRequest(ctx, messages)
	if err != nil {
		return nil, err
	}
	pr.req.Tools = []types.ToolSchema{numberedPlanToolSchema()}
	pr.req.ToolChoice = "required"
	pr.req.ToolCallMode = types.ToolCallModeNative

	resp, err := pr.chatProvider.Completion(ctx, pr.req)
	if err != nil {
		return nil, NewErrorWithCause(types.ErrAgentExecution, "plan generation failed", err)
	}

	// 解析计划
	if resp == nil || len(resp.Choices) == 0 {
		return nil, NewError(types.ErrLLMResponseEmpty, "plan generation returned no choices")
	}
	choice := resp.FirstChoice()
	steps, parseErr := parseNumberedPlanToolCall(choice.Message)
	if parseErr != nil {
		return nil, NewErrorWithCause(types.ErrAgentExecution, "plan generation did not return native tool call", parseErr)
	}
	if len(steps) == 0 {
		return nil, NewError(types.ErrLLMResponseEmpty, "plan generation returned no steps")
	}

	b.logger.Info("plan generated",
		zap.Int("steps", len(steps)),
		zap.String("trace_id", input.TraceID),
	)

	return &PlanResult{
		Steps: steps,
		Metadata: map[string]any{
			"tokens_used": resp.Usage.TotalTokens,
			"model":       resp.Model,
		},
	}, nil
}

// Execute 执行任务（完整的 ReAct 循环）
// 这是 Agent 的核心执行方法，包含完整的推理-行动循环
// Requirements 1.7: 集成输入验证
// Requirements 2.4: 输出验证失败时支持重试
func (b *BaseAgent) Execute(ctx context.Context, input *Input) (_ *Output, execErr error) {
	resumeInput, err := b.prepareResumeInput(ctx, input)
	if err != nil {
		return nil, err
	}
	return b.executeWithPipeline(ctx, resumeInput, b.configuredExecutionOptions())
}

func (b *BaseAgent) executeCore(ctx context.Context, input *Input) (_ *Output, execErr error) {
	if input == nil {
		return nil, NewError(types.ErrInputValidation, "input is nil")
	}
	if strings.TrimSpace(input.Content) == "" {
		return nil, NewError(types.ErrInputValidation, "input content is empty")
	}
	startTime := time.Now()

	// 0a. Inject top-level Input fields into Go context
	if input.TraceID != "" {
		ctx = types.WithTraceID(ctx, input.TraceID)
	}
	if input.TenantID != "" {
		ctx = types.WithTenantID(ctx, input.TenantID)
	}
	if input.UserID != "" {
		ctx = types.WithUserID(ctx, input.UserID)
	}

	// 0b. Inject Input.Context well-known keys into Go context (overrides top-level fields)
	ctx = applyInputContext(ctx, input.Context)

	// 0c. Apply runtime overrides from context and input in a single precedence chain.
	if runConfig := ResolveRunConfig(ctx, input); runConfig != nil {
		ctx = WithRunConfig(ctx, runConfig)
	}

	// 1. 尝试获取执行锁
	if !b.TryLockExec() {
		return nil, ErrAgentBusy
	}
	defer b.UnlockExec()

	// 2. 在锁保护下确保 Agent 就绪
	if err := b.EnsureReady(); err != nil {
		return nil, err
	}

	// 3. 转换状态到运行中（支持并发：首个请求负责转换，后续请求复用 Running 状态）
	if atomic.AddInt64(&b.execCount, 1) == 1 {
		if err := b.Transition(ctx, StateRunning); err != nil {
			atomic.AddInt64(&b.execCount, -1)
			return nil, err
		}
	}

	// 以下操作修改共享状态（b.promptBundle），必须在 execMu 保护下执行。

	// 3a. PromptStore: load active prompt from MongoDB if available (local copy, no shared state mutation)
	activeBundle := b.promptBundle
	if doc := b.persistence.LoadPrompt(ctx, b.config.Core.Type, b.config.Core.Name, ""); doc != nil {
		activeBundle.Version = doc.Version
		activeBundle.System = doc.System
		if len(doc.Constraints) > 0 {
			activeBundle.Constraints = doc.Constraints
		}
		b.logger.Info("loaded prompt from store",
			zap.String("version", doc.Version),
			zap.String("agent_type", b.config.Core.Type),
		)
	}

	// 3b. RunStore: record execution start
	runID := b.persistence.RecordRun(ctx, b.config.Core.ID, input.TenantID, input.TraceID, input.Content, startTime)

	// 3c. ConversationStore: restore conversation history
	conversationID := input.ChannelID
	restoredMessages := b.persistence.RestoreConversation(ctx, conversationID)
	defer func() {
		// Ensure run status is updated on any exit path (including panic).
		if runID != "" {
			if r := recover(); r != nil {
				if updateErr := b.persistence.UpdateRunStatus(ctx, runID, "failed", nil, fmt.Sprintf("panic: %v", r)); updateErr != nil {
					b.logger.Warn("failed to mark run as failed after panic", zap.Error(updateErr))
				}
				b.logger.Error("panic during execution, run marked as failed",
					zap.Any("panic", r),
					zap.Error(panicPayloadToError(r)),
					zap.String("run_id", runID),
				)
				if execErr == nil {
					execErr = NewErrorWithCause(types.ErrAgentExecution, "react execution panic", panicPayloadToError(r))
				}
			}
			if execErr != nil {
				if updateErr := b.persistence.UpdateRunStatus(ctx, runID, "failed", nil, execErr.Error()); updateErr != nil {
					b.logger.Warn("failed to mark run as failed", zap.Error(updateErr))
				}
			}
		}
		// 使用独立 context 确保状态恢复不受原始 ctx 取消影响
		if atomic.AddInt64(&b.execCount, -1) == 0 {
			if err := b.Transition(context.Background(), StateReady); err != nil {
				b.logger.Error("failed to transition to ready", zap.Error(err))
			}
		}
	}()

	b.logger.Info("executing task",
		zap.String("trace_id", input.TraceID),
		zap.String("agent_id", b.config.Core.ID),
		zap.String("agent_type", b.config.Core.Type),
	)

	// 4. 输入验证(监护)
	b.configMu.RLock()
	guardrailsEnabled := b.guardrailsEnabled
	inputValidatorChain := b.inputValidatorChain
	runtimeGuardrailsCfg := b.runtimeGuardrailsCfg
	b.configMu.RUnlock()

	if guardrailsEnabled && inputValidatorChain != nil {
		validationResult, err := inputValidatorChain.Validate(ctx, input.Content)
		if err != nil {
			b.logger.Error("input validation error", zap.Error(err))
			return nil, NewErrorWithCause(types.ErrInputValidation, "input validation error", err)
		}

		if !validationResult.Valid {
			b.logger.Warn("input validation failed",
				zap.String("trace_id", input.TraceID),
				zap.Any("errors", validationResult.Errors),
			)

			// 从配置中检查失败动作
			failureAction := guardrails.FailureActionReject
			if runtimeGuardrailsCfg != nil {
				failureAction = runtimeGuardrailsCfg.OnInputFailure
			}

			switch failureAction {
			case guardrails.FailureActionReject:
				return nil, &GuardrailsError{
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

	// 5. 收集上下文来源并通过 context runtime 组装消息
	memoryContext := b.collectContextMemory(input.Context)
	conversation := restoredMessages
	if handoffMessages := handoffMessagesFromInputContext(input.Context); len(handoffMessages) > 0 {
		conversation = handoffMessages
	}

	systemContent := activeBundle.RenderSystemPromptWithVars(input.Variables)
	if publicCtx := agentcontext.AdditionalContextText(publicInputContext(input.Context)); publicCtx != "" {
		systemContent += "\n\n<additional_context>\n" + publicCtx + "\n</additional_context>"
	}
	skillContext := skillInstructionsFromInputContext(input.Context)
	if len(skillContext) == 0 {
		skillContext = normalizeInstructionList(skillInstructionsFromCtx(ctx))
	}
	publicContext := publicInputContext(input.Context)
	retrievalItems := retrievalItemsFromInputContext(input.Context)
	if len(retrievalItems) == 0 && b.retriever != nil {
		if records, err := b.retriever.Retrieve(ctx, input.Content, 5); err != nil {
			b.logger.Warn("failed to load retrieval context", zap.Error(err))
		} else {
			retrievalItems = retrievalItemsFromRecords(records)
		}
	}
	toolStates := toolStatesFromInputContext(input.Context)
	if len(toolStates) == 0 && b.toolState != nil {
		if snapshots, err := b.toolState.LoadToolState(ctx, b.ID()); err != nil {
			b.logger.Warn("failed to load tool state context", zap.Error(err))
		} else {
			toolStates = toolStatesFromSnapshots(snapshots)
		}
	}

	ephemeralLayers, traceFeedbackPlan := b.buildEphemeralPromptLayers(ctx, publicContext, input, systemContent, skillContext, memoryContext, conversation, retrievalItems, toolStates)
	messages, assembled := b.assembleMessages(ctx, systemContent, ephemeralLayers, skillContext, memoryContext, conversation, retrievalItems, toolStates, input.Content)
	if assembled != nil {
		b.logger.Debug("context assembled",
			zap.Int("tokens_before", assembled.TokensBefore),
			zap.Int("tokens_after", assembled.TokensAfter),
			zap.String("strategy", assembled.Plan.Strategy),
			zap.String("compression_reason", assembled.Plan.CompressionReason),
			zap.Int("applied_layers", len(assembled.Plan.AppliedLayers)),
		)
		if emit, ok := runtimeStreamEmitterFromContext(ctx); ok {
			emitRuntimeStatus(emit, "prompt_layers_built", RuntimeStreamEvent{
				Timestamp:    time.Now(),
				CurrentStage: "context",
				Data: map[string]any{
					"context_plan":   assembled.Plan,
					"applied_layers": assembled.Plan.AppliedLayers,
					"layer_ids":      promptLayerIDs(assembled.Plan.AppliedLayers),
				},
			})
		}
		b.recordPromptLayerTimeline(input.TraceID, assembled.Plan)
	}
	ctx = b.withApprovalExplainability(ctx, input)

	// 7. 执行产出验证和重试支持
	// 要求2.4:对产出验证失败进行重试
	policy := b.loopControlPolicy()
	b.configMu.RLock()
	maxRetries := policy.RetryBudget
	outputValidator := b.outputValidator
	guardrailsEnabledForOutput := b.guardrailsEnabled
	runtimeGuardrailsCfgForOutput := runtimeGuardrailsFromPolicy(policy, b.runtimeGuardrailsCfg)
	b.configMu.RUnlock()

	var resp *types.ChatResponse
	var outputContent string
	var lastValidationResult *guardrails.ValidationResult
	var choice types.ChatChoice

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			b.logger.Info("retrying execution due to output validation failure",
				zap.Int("attempt", attempt),
				zap.String("trace_id", input.TraceID),
			)

			// 为重试添加验证失败的反馈
			if lastValidationResult != nil {
				feedbackMsg := b.buildValidationFeedbackMessage(lastValidationResult)
				messages = append(messages, types.Message{
					Role:    types.RoleUser,
					Content: feedbackMsg,
				})
			}
		}

		// 执行重新行动循环
		var err error
		resp, err = b.ChatCompletion(ctx, messages)
		if err != nil {
			b.logger.Error("execution failed",
				zap.Error(err),
				zap.String("trace_id", input.TraceID),
			)
			return nil, NewErrorWithCause(types.ErrAgentExecution, "execution failed", err)
		}

		if resp == nil || len(resp.Choices) == 0 {
			return nil, NewError(types.ErrLLMResponseEmpty, "execution returned no choices")
		}
		choice = resp.FirstChoice()
		outputContent = choice.Message.Content

		// 产出验证(护栏)
		if guardrailsEnabledForOutput && outputValidator != nil {
			var filteredContent string
			filteredContent, lastValidationResult, err = outputValidator.ValidateAndFilter(ctx, outputContent)
			if err != nil {
				b.logger.Error("output validation error", zap.Error(err))
				return nil, NewErrorWithCause(types.ErrOutputValidation, "output validation error", err)
			}

			if !lastValidationResult.Valid {
				b.logger.Warn("output validation failed",
					zap.String("trace_id", input.TraceID),
					zap.Int("attempt", attempt),
					zap.Any("errors", lastValidationResult.Errors),
				)

				// 检查失败动作
				failureAction := guardrails.FailureActionReject
				if runtimeGuardrailsCfgForOutput != nil {
					failureAction = runtimeGuardrailsCfgForOutput.OnOutputFailure
				}

				// 如果重试已经配置, 我们还没有用尽重试, 请继续
				if failureAction == guardrails.FailureActionRetry && attempt < maxRetries {
					continue
				}

				// 基于失败动作的处理
				switch failureAction {
				case guardrails.FailureActionReject:
					return nil, &GuardrailsError{
						Type:    GuardrailsErrorTypeOutput,
						Message: "output validation failed",
						Errors:  lastValidationResult.Errors,
					}
				case guardrails.FailureActionWarn:
					b.logger.Warn("output validation warning, using filtered content")
					outputContent = filteredContent
				case guardrails.FailureActionRetry:
					// 重复重复,拒绝
					return nil, &GuardrailsError{
						Type:    GuardrailsErrorTypeOutput,
						Message: fmt.Sprintf("output validation failed after %d retries", maxRetries),
						Errors:  lastValidationResult.Errors,
					}
				}
			} else {
				// 通过验证, 使用过滤内容
				outputContent = filteredContent
			}
		}

		// 通过验证或警告模式, 中断重试循环
		break
	}

	estimatedCost := defaultCostCalc.Calculate(resp.Provider, resp.Model, resp.Usage.PromptTokens, resp.Usage.CompletionTokens)

	// 8. 保存记忆/回合观察
	if b.memoryRuntime != nil {
		if err := b.memoryRuntime.ObserveTurn(ctx, b.ID(), MemoryObservationInput{
			TraceID:          input.TraceID,
			UserContent:      input.Content,
			AssistantContent: outputContent,
			Metadata: map[string]any{
				"tokens":        resp.Usage.TotalTokens,
				"cost":          estimatedCost,
				"finish_reason": choice.FinishReason,
			},
		}); err != nil {
			b.logger.Warn("memory runtime observe turn failed", zap.Error(err))
		}
	} else {
		skipBaseMemory := b.memoryFacade != nil && b.memoryFacade.SkipBaseMemory()
		if b.memory != nil && !skipBaseMemory {
			if err := b.SaveMemory(ctx, input.Content, MemoryShortTerm, map[string]any{
				"trace_id": input.TraceID,
				"role":     "user",
			}); err != nil {
				b.logger.Warn("failed to save user input to memory", zap.Error(err))
			}
			if err := b.SaveMemory(ctx, outputContent, MemoryShortTerm, map[string]any{
				"trace_id": input.TraceID,
				"role":     "assistant",
			}); err != nil {
				b.logger.Warn("failed to save response to memory", zap.Error(err))
			}
		}
	}

	duration := time.Since(startTime)

	// 9a. ConversationStore: persist conversation
	b.persistence.PersistConversation(ctx, conversationID, b.config.Core.ID, input.TenantID, input.UserID, input.Content, outputContent)

	// 9b. RunStore: update run status
	if runID != "" {
		outputDoc := &RunOutputDoc{
			Content:      outputContent,
			TokensUsed:   resp.Usage.TotalTokens,
			Cost:         estimatedCost,
			FinishReason: choice.FinishReason,
		}
		if err := b.persistence.UpdateRunStatus(ctx, runID, "completed", outputDoc, ""); err != nil {
			b.logger.Warn("failed to update run status", zap.Error(err))
		}
	}

	b.logger.Info("execution completed",
		zap.String("trace_id", input.TraceID),
		zap.Duration("duration", duration),
		zap.Int("tokens_used", resp.Usage.TotalTokens),
	)

	// 9. 返回结果
	outputMetadata := map[string]any{
		"model":    resp.Model,
		"provider": resp.Provider,
	}
	if assembled != nil {
		outputMetadata["context_plan"] = assembled.Plan
		outputMetadata["applied_prompt_layers"] = assembled.Plan.AppliedLayers
		outputMetadata["applied_prompt_layer_ids"] = promptLayerIDs(assembled.Plan.AppliedLayers)
	}
	outputMetadata["trace_feedback_plan"] = map[string]any{
		"planner_id":           traceFeedbackPlan.PlannerID,
		"planner_version":      traceFeedbackPlan.PlannerVersion,
		"confidence":           traceFeedbackPlan.Confidence,
		"goal":                 traceFeedbackPlan.Goal,
		"recommended_action":   string(traceFeedbackPlan.RecommendedAction),
		"inject_memory_recall": traceFeedbackPlan.InjectMemoryRecall,
		"primary_layer":        traceFeedbackPlan.PrimaryLayer,
		"secondary_layer":      traceFeedbackPlan.SecondaryLayer,
		"selected_layers":      cloneStringSlice(traceFeedbackPlan.SelectedLayers),
		"suppressed_layers":    cloneStringSlice(traceFeedbackPlan.SuppressedLayers),
		"reasons":              cloneStringSlice(traceFeedbackPlan.Reasons),
		"signals": map[string]any{
			"has_prior_synopsis":        traceFeedbackPlan.Signals.HasPriorSynopsis,
			"has_compressed_history":    traceFeedbackPlan.Signals.HasCompressedHistory,
			"resume":                    traceFeedbackPlan.Signals.Resume,
			"handoff":                   traceFeedbackPlan.Signals.Handoff,
			"multi_agent":               traceFeedbackPlan.Signals.MultiAgent,
			"verification":              traceFeedbackPlan.Signals.Verification,
			"complex_task":              traceFeedbackPlan.Signals.ComplexTask,
			"context_pressure":          traceFeedbackPlan.Signals.ContextPressure,
			"usage_ratio":               traceFeedbackPlan.Signals.UsageRatio,
			"acceptance_criteria_count": traceFeedbackPlan.Signals.AcceptanceCriteriaCount,
			"compressed_event_count":    traceFeedbackPlan.Signals.CompressedEventCount,
		},
		"metadata": cloneAnyMap(traceFeedbackPlan.Metadata),
		"summary":  traceFeedbackPlan.Summary,
	}
	if extraMetadata, ok := choice.Message.Metadata.(map[string]any); ok {
		for key, value := range extraMetadata {
			outputMetadata[key] = value
		}
	}
	return &Output{
		TraceID:          input.TraceID,
		Content:          outputContent,
		ReasoningContent: choice.Message.ReasoningContent,
		Metadata:         outputMetadata,
		TokensUsed:       resp.Usage.TotalTokens,
		Cost:             estimatedCost,
		Duration:         duration,
		FinishReason:     choice.FinishReason,
	}, nil
}

func (b *BaseAgent) withApprovalExplainability(ctx context.Context, input *Input) context.Context {
	recorder, ok := b.extensions.ObservabilitySystemExt().(ExplainabilityRecorder)
	if !ok || input == nil {
		return ctx
	}
	return withApprovalExplainabilityEmitter(ctx, recorder, strings.TrimSpace(input.TraceID))
}

func (b *BaseAgent) recordPromptLayerTimeline(traceID string, plan agentcontext.ContextPlan) {
	recorder, ok := b.extensions.ObservabilitySystemExt().(ExplainabilityTimelineRecorder)
	if !ok || strings.TrimSpace(traceID) == "" {
		return
	}
	recorder.AddExplainabilityTimeline(traceID, "prompt_layers", "Prompt layers assembled for this request", map[string]any{
		"context_plan":   plan,
		"applied_layers": plan.AppliedLayers,
		"layer_ids":      promptLayerIDs(plan.AppliedLayers),
	})
}

// 构建 ValidationFeedBackMessage 为重试创建回馈消息
func (b *BaseAgent) buildValidationFeedbackMessage(result *guardrails.ValidationResult) string {
	var sb strings.Builder
	sb.WriteString("Your previous response failed validation. Please regenerate your response addressing the following issues:\n")
	for _, err := range result.Errors {
		sb.WriteString(fmt.Sprintf("- %s: %s\n", err.Code, err.Message))
	}
	sb.WriteString("\nPlease provide a corrected response.")
	return sb.String()
}

func panicPayloadToError(v any) error {
	if err, ok := v.(error); ok {
		return err
	}
	return fmt.Errorf("panic: %v", v)
}

// Observe 处理反馈并更新 Agent 状态
// 这个方法允许 Agent 从外部反馈中学习和改进
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

	// 1. 保存反馈到记忆系统
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

	// 2. 发布反馈事件
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

func (b *BaseAgent) assembleMessages(
	ctx context.Context,
	systemPrompt string,
	ephemeralLayers []agentcontext.PromptLayer,
	skillContext []string,
	memoryContext []string,
	conversation []types.Message,
	retrieval []agentcontext.RetrievalItem,
	toolStates []agentcontext.ToolState,
	userInput string,
) ([]types.Message, *agentcontext.AssembleResult) {
	if manager, ok := b.contextManager.(interface {
		Assemble(context.Context, *agentcontext.AssembleRequest) (*agentcontext.AssembleResult, error)
	}); ok {
		result, err := manager.Assemble(ctx, &agentcontext.AssembleRequest{
			SystemPrompt:    systemPrompt,
			EphemeralLayers: ephemeralLayers,
			SkillContext:    skillContext,
			MemoryContext:   memoryContext,
			Conversation:    conversation,
			Retrieval:       retrieval,
			ToolState:       toolStates,
			UserInput:       userInput,
			Query:           userInput,
		})
		if err == nil && result != nil && len(result.Messages) > 0 {
			return result.Messages, result
		}
		if err != nil {
			b.logger.Warn("context assembly failed, falling back to legacy message construction", zap.Error(err))
		}
	}

	msgCap := 1 + len(ephemeralLayers) + len(skillContext) + len(memoryContext) + len(conversation) + 1
	messages := make([]types.Message, 0, msgCap)
	if strings.TrimSpace(systemPrompt) != "" {
		messages = append(messages, types.Message{Role: types.RoleSystem, Content: systemPrompt})
	}
	for _, layer := range ephemeralLayers {
		if strings.TrimSpace(layer.Content) == "" {
			continue
		}
		role := layer.Role
		if role == "" {
			role = types.RoleSystem
		}
		messages = append(messages, types.Message{Role: role, Content: layer.Content, Metadata: layer.Metadata})
	}
	for _, item := range skillContext {
		if strings.TrimSpace(item) == "" {
			continue
		}
		messages = append(messages, types.Message{Role: types.RoleSystem, Content: item})
	}
	for _, item := range memoryContext {
		messages = append(messages, types.Message{Role: types.RoleSystem, Content: item})
	}
	messages = append(messages, conversation...)
	messages = append(messages, types.Message{Role: types.RoleUser, Content: userInput})
	return messages, nil
}

func (b *BaseAgent) buildEphemeralPromptLayers(
	ctx context.Context,
	publicContext map[string]any,
	input *Input,
	systemPrompt string,
	skillContext []string,
	memoryContext []string,
	conversation []types.Message,
	retrieval []agentcontext.RetrievalItem,
	toolStates []agentcontext.ToolState,
) ([]agentcontext.PromptLayer, TraceFeedbackPlan) {
	if b.ephemeralPrompt == nil {
		return nil, TraceFeedbackPlan{}
	}
	status := b.estimateContextStatus(systemPrompt, skillContext, memoryContext, conversation, retrieval, toolStates, input)
	snapshot := b.latestTraceSynopsisSnapshot(input)
	plan := b.selectTraceFeedbackPlan(input, status, snapshot)
	checkpointID := ""
	if input != nil && input.Context != nil {
		if value, ok := input.Context["checkpoint_id"].(string); ok {
			checkpointID = strings.TrimSpace(value)
		}
	}
	b.recordTraceFeedbackDecision(input.TraceID, plan, status)
	layers := b.ephemeralPrompt.Build(EphemeralPromptLayerInput{
		PublicContext:            publicContext,
		TraceID:                  strings.TrimSpace(input.TraceID),
		TenantID:                 strings.TrimSpace(input.TenantID),
		UserID:                   strings.TrimSpace(input.UserID),
		ChannelID:                strings.TrimSpace(input.ChannelID),
		TraceFeedbackPlan:        &plan,
		TraceSynopsis:            conditionalTraceSynopsis(plan.InjectSynopsis, snapshot),
		TraceHistorySummary:      conditionalTraceHistory(plan.InjectHistory, snapshot),
		TraceHistoryEventCount:   conditionalTraceHistoryCount(plan.InjectHistory, snapshot),
		CheckpointID:             checkpointID,
		AllowedTools:             b.effectivePromptToolNames(ctx),
		ToolsDisabled:            promptToolsDisabled(ctx),
		AcceptanceCriteria:       acceptanceCriteriaForValidation(input, nil),
		ToolVerificationRequired: toolVerificationRequired(input, nil, nil),
		CodeVerificationRequired: codeTaskRequired(input, nil, nil),
		ContextStatus:            status,
	})
	if b.memoryRuntime != nil && plan.InjectMemoryRecall {
		recallLayers, err := b.memoryRuntime.RecallForPrompt(ctx, b.ID(), MemoryRecallOptions{
			Query:  input.Content,
			Status: status,
			TopK:   3,
		})
		if err != nil {
			b.logger.Warn("memory runtime recall failed", zap.Error(err))
		} else if len(recallLayers) > 0 {
			layers = append(layers, recallLayers...)
		}
	}
	return layers, plan
}

func (b *BaseAgent) estimateContextStatus(
	systemPrompt string,
	skillContext []string,
	memoryContext []string,
	conversation []types.Message,
	retrieval []agentcontext.RetrievalItem,
	toolStates []agentcontext.ToolState,
	input *Input,
) *agentcontext.Status {
	if b.contextManager == nil {
		return nil
	}
	messages := make([]types.Message, 0, 1+len(skillContext)+len(memoryContext)+len(conversation)+len(retrieval)+len(toolStates)+1)
	if strings.TrimSpace(systemPrompt) != "" {
		messages = append(messages, types.Message{Role: types.RoleSystem, Content: systemPrompt})
	}
	for _, item := range skillContext {
		if strings.TrimSpace(item) != "" {
			messages = append(messages, types.Message{Role: types.RoleSystem, Content: item})
		}
	}
	for _, item := range memoryContext {
		if strings.TrimSpace(item) != "" {
			messages = append(messages, types.Message{Role: types.RoleSystem, Content: item})
		}
	}
	messages = append(messages, conversation...)
	for _, item := range retrieval {
		if strings.TrimSpace(item.Content) != "" {
			messages = append(messages, types.Message{Role: types.RoleSystem, Content: item.Content})
		}
	}
	for _, item := range toolStates {
		if strings.TrimSpace(item.Summary) != "" {
			messages = append(messages, types.Message{Role: types.RoleSystem, Content: item.Summary})
		}
	}
	if input != nil && strings.TrimSpace(input.Content) != "" {
		messages = append(messages, types.Message{Role: types.RoleUser, Content: input.Content})
	}
	status := b.contextManager.GetStatus(messages)
	return &status
}

func (b *BaseAgent) selectTraceFeedbackPlan(input *Input, status *agentcontext.Status, snapshot ExplainabilitySynopsisSnapshot) TraceFeedbackPlan {
	planner := b.traceFeedbackPlanner
	if planner == nil {
		planner = NewComposedTraceFeedbackPlanner(NewRuleBasedTraceFeedbackPlanner(), NewHintTraceFeedbackAdapter())
	}
	sessionID := ""
	traceID := ""
	if input != nil {
		sessionID = strings.TrimSpace(input.ChannelID)
		traceID = strings.TrimSpace(input.TraceID)
	}
	if sessionID == "" {
		sessionID = traceID
	}
	return planner.Plan(&TraceFeedbackPlanningInput{
		AgentID:   b.ID(),
		TraceID:   traceID,
		SessionID: sessionID,
		UserInput: input,
		Signals:   collectTraceFeedbackSignals(input, status, snapshot, b.memoryRuntime != nil),
		Snapshot:  snapshot,
		Config:    TraceFeedbackConfigFromAgentConfig(b.config),
	})
}

func (b *BaseAgent) latestTraceSynopsis(input *Input) string {
	snapshot := b.latestTraceSynopsisSnapshot(input)
	if strings.TrimSpace(snapshot.Synopsis) != "" {
		return strings.TrimSpace(snapshot.Synopsis)
	}
	reader, ok := b.extensions.ObservabilitySystemExt().(ExplainabilitySynopsisReader)
	if !ok || input == nil {
		return ""
	}
	sessionID := strings.TrimSpace(input.ChannelID)
	if sessionID == "" {
		sessionID = strings.TrimSpace(input.TraceID)
	}
	return strings.TrimSpace(reader.GetLatestExplainabilitySynopsis(sessionID, b.ID(), strings.TrimSpace(input.TraceID)))
}

func (b *BaseAgent) latestTraceHistorySummary(input *Input) string {
	return strings.TrimSpace(b.latestTraceSynopsisSnapshot(input).CompressedHistory)
}

func (b *BaseAgent) latestTraceHistoryEventCount(input *Input) int {
	return b.latestTraceSynopsisSnapshot(input).CompressedEventCount
}

func (b *BaseAgent) latestTraceSynopsisSnapshot(input *Input) ExplainabilitySynopsisSnapshot {
	reader, ok := b.extensions.ObservabilitySystemExt().(ExplainabilitySynopsisSnapshotReader)
	if !ok || input == nil {
		return ExplainabilitySynopsisSnapshot{}
	}
	sessionID := strings.TrimSpace(input.ChannelID)
	if sessionID == "" {
		sessionID = strings.TrimSpace(input.TraceID)
	}
	return reader.GetLatestExplainabilitySynopsisSnapshot(sessionID, b.ID(), strings.TrimSpace(input.TraceID))
}

func conditionalTraceSynopsis(enabled bool, snapshot ExplainabilitySynopsisSnapshot) string {
	if !enabled {
		return ""
	}
	return strings.TrimSpace(snapshot.Synopsis)
}

func conditionalTraceHistory(enabled bool, snapshot ExplainabilitySynopsisSnapshot) string {
	if !enabled {
		return ""
	}
	return strings.TrimSpace(snapshot.CompressedHistory)
}

func conditionalTraceHistoryCount(enabled bool, snapshot ExplainabilitySynopsisSnapshot) int {
	if !enabled {
		return 0
	}
	return snapshot.CompressedEventCount
}

func (b *BaseAgent) recordTraceFeedbackDecision(traceID string, plan TraceFeedbackPlan, status *agentcontext.Status) {
	recorder, ok := b.extensions.ObservabilitySystemExt().(ExplainabilityTimelineRecorder)
	if !ok || strings.TrimSpace(traceID) == "" {
		return
	}
	metadata := map[string]any{
		"inject_synopsis":         plan.InjectSynopsis,
		"inject_history":          plan.InjectHistory,
		"inject_memory_recall":    plan.InjectMemoryRecall,
		"score":                   plan.Score,
		"synopsis_threshold":      plan.SynopsisThreshold,
		"history_threshold":       plan.HistoryThreshold,
		"memory_recall_threshold": plan.MemoryRecallThreshold,
		"reasons":                 cloneStringSlice(plan.Reasons),
		"selected_layers":         cloneStringSlice(plan.SelectedLayers),
		"suppressed_layers":       cloneStringSlice(plan.SuppressedLayers),
		"goal":                    plan.Goal,
		"recommended_action":      string(plan.RecommendedAction),
		"primary_layer":           plan.PrimaryLayer,
		"secondary_layer":         plan.SecondaryLayer,
		"planner_id":              plan.PlannerID,
		"planner_version":         plan.PlannerVersion,
		"confidence":              plan.Confidence,
		"planner_metadata":        cloneAnyMap(plan.Metadata),
		"signals": map[string]any{
			"has_prior_synopsis":        plan.Signals.HasPriorSynopsis,
			"has_compressed_history":    plan.Signals.HasCompressedHistory,
			"resume":                    plan.Signals.Resume,
			"handoff":                   plan.Signals.Handoff,
			"multi_agent":               plan.Signals.MultiAgent,
			"verification":              plan.Signals.Verification,
			"complex_task":              plan.Signals.ComplexTask,
			"context_pressure":          plan.Signals.ContextPressure,
			"usage_ratio":               plan.Signals.UsageRatio,
			"acceptance_criteria_count": plan.Signals.AcceptanceCriteriaCount,
			"compressed_event_count":    plan.Signals.CompressedEventCount,
		},
	}
	if status != nil {
		metadata["usage_ratio"] = status.UsageRatio
		metadata["pressure_level"] = status.Level.String()
	}
	recorder.AddExplainabilityTimeline(traceID, "trace_feedback_decision", plan.Summary, metadata)
}

func (b *BaseAgent) effectivePromptToolNames(ctx context.Context) []string {
	rc := GetRunConfig(ctx)
	if rc != nil && rc.DisableTools {
		return nil
	}
	var names []string
	if b.toolManager != nil {
		for _, schema := range b.toolManager.GetAllowedTools(b.config.Core.ID) {
			names = append(names, schema.Name)
		}
	}
	if rc != nil && len(rc.ToolWhitelist) > 0 {
		names = filterStringWhitelist(names, rc.ToolWhitelist)
	} else if len(b.config.Runtime.Tools) > 0 {
		names = filterStringWhitelist(names, b.config.Runtime.Tools)
	}
	for _, target := range runtimeHandoffTargetsFromContext(ctx, b.config.Core.ID) {
		names = append(names, runtimeHandoffToolSchema(target).Name)
	}
	return normalizeStringSlice(names)
}

func promptToolsDisabled(ctx context.Context) bool {
	rc := GetRunConfig(ctx)
	return rc != nil && rc.DisableTools
}

func filterStringWhitelist(values []string, whitelist []string) []string {
	if len(values) == 0 || len(whitelist) == 0 {
		return normalizeStringSlice(values)
	}
	allowed := make(map[string]struct{}, len(whitelist))
	for _, value := range whitelist {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		allowed[trimmed] = struct{}{}
	}
	if len(allowed) == 0 {
		return nil
	}
	filtered := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := allowed[strings.TrimSpace(value)]; ok {
			filtered = append(filtered, value)
		}
	}
	return normalizeStringSlice(filtered)
}

func promptLayerIDs(layers []agentcontext.PromptLayerMeta) []string {
	if len(layers) == 0 {
		return nil
	}
	ids := make([]string, 0, len(layers))
	for _, layer := range layers {
		if trimmed := strings.TrimSpace(layer.ID); trimmed != "" {
			ids = append(ids, trimmed)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	return ids
}

func retrievalItemsFromInputContext(values map[string]any) []agentcontext.RetrievalItem {
	if len(values) == 0 {
		return nil
	}
	raw, ok := values["retrieval_context"]
	if !ok {
		return nil
	}
	items, ok := raw.([]agentcontext.RetrievalItem)
	if !ok {
		return nil
	}
	return append([]agentcontext.RetrievalItem(nil), items...)
}

func skillInstructionsFromInputContext(values map[string]any) []string {
	if len(values) == 0 {
		return nil
	}
	raw, ok := values["skill_context"]
	if !ok {
		return nil
	}
	items, ok := raw.([]string)
	if !ok {
		return nil
	}
	return normalizeInstructionList(items)
}

func retrievalItemsFromRecords(records []types.RetrievalRecord) []agentcontext.RetrievalItem {
	if len(records) == 0 {
		return nil
	}
	items := make([]agentcontext.RetrievalItem, 0, len(records))
	for _, record := range records {
		if strings.TrimSpace(record.Content) == "" {
			continue
		}
		items = append(items, agentcontext.RetrievalItem{
			Title:   record.DocID,
			Content: record.Content,
			Source:  record.Source,
			Score:   record.Score,
		})
	}
	return items
}

func toolStatesFromInputContext(values map[string]any) []agentcontext.ToolState {
	if len(values) == 0 {
		return nil
	}
	raw, ok := values["tool_state"]
	if !ok {
		return nil
	}
	items, ok := raw.([]agentcontext.ToolState)
	if !ok {
		return nil
	}
	return append([]agentcontext.ToolState(nil), items...)
}

func toolStatesFromSnapshots(items []types.ToolStateSnapshot) []agentcontext.ToolState {
	if len(items) == 0 {
		return nil
	}
	out := make([]agentcontext.ToolState, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.Summary) == "" {
			continue
		}
		out = append(out, agentcontext.ToolState{
			ToolName:   item.ToolName,
			Summary:    item.Summary,
			ArtifactID: item.ArtifactID,
		})
	}
	return out
}

// applyInputContext injects well-known keys from Input.Context into the Go context
// using the corresponding types.WithXxx functions. Unknown keys are silently ignored.
func applyInputContext(ctx context.Context, inputCtx map[string]any) context.Context {
	if len(inputCtx) == 0 {
		return ctx
	}
	for k, v := range inputCtx {
		switch k {
		case "trace_id":
			if s, ok := v.(string); ok && s != "" {
				ctx = types.WithTraceID(ctx, s)
			}
		case "tenant_id":
			if s, ok := v.(string); ok && s != "" {
				ctx = types.WithTenantID(ctx, s)
			}
		case "user_id":
			if s, ok := v.(string); ok && s != "" {
				ctx = types.WithUserID(ctx, s)
			}
		case "run_id":
			if s, ok := v.(string); ok && s != "" {
				ctx = types.WithRunID(ctx, s)
			}
		case "parent_run_id":
			if s, ok := v.(string); ok && s != "" {
				ctx = types.WithParentRunID(ctx, s)
			}
		case "span_id":
			if s, ok := v.(string); ok && s != "" {
				ctx = types.WithSpanID(ctx, s)
			}
		case "agent_id":
			if s, ok := v.(string); ok && s != "" {
				ctx = types.WithAgentID(ctx, s)
			}
		case "llm_model":
			if s, ok := v.(string); ok && s != "" {
				ctx = types.WithLLMModel(ctx, s)
			}
		case "llm_provider":
			if s, ok := v.(string); ok && s != "" {
				ctx = types.WithLLMProvider(ctx, s)
			}
		case "llm_route_policy":
			if s, ok := v.(string); ok && s != "" {
				ctx = types.WithLLMRoutePolicy(ctx, s)
			}
		case "prompt_bundle_version":
			if s, ok := v.(string); ok && s != "" {
				ctx = types.WithPromptBundleVersion(ctx, s)
			}
		case "roles":
			if roles, ok := v.([]string); ok && len(roles) > 0 {
				ctx = types.WithRoles(ctx, roles)
			}
			// Also handle []any (common from JSON deserialization)
			if arr, ok := v.([]any); ok && len(arr) > 0 {
				roles := make([]string, 0, len(arr))
				for _, item := range arr {
					if s, ok := item.(string); ok {
						roles = append(roles, s)
					}
				}
				if len(roles) > 0 {
					ctx = types.WithRoles(ctx, roles)
				}
			}
		}
	}
	return ctx
}

func (b *BaseAgent) prepareResumeInput(ctx context.Context, input *Input) (*Input, error) {
	if input == nil || b.checkpointManager == nil {
		return input, nil
	}
	checkpointID, resumeLatest := resumeDirective(input)
	if checkpointID == "" && !resumeLatest {
		return input, nil
	}

	var (
		checkpoint *Checkpoint
		err        error
	)
	if checkpointID != "" {
		checkpoint, err = b.checkpointManager.LoadCheckpoint(ctx, checkpointID)
	} else {
		threadID := resumeThreadID(input, b.ID())
		checkpoint, err = b.checkpointManager.LoadLatestCheckpoint(ctx, threadID)
	}
	if err != nil {
		return nil, err
	}
	if checkpoint != nil && checkpoint.AgentID != "" && checkpoint.AgentID != b.ID() {
		return nil, NewError(types.ErrInputValidation,
			fmt.Sprintf("checkpoint agent ID mismatch: checkpoint belongs to %s, current agent is %s", checkpoint.AgentID, b.ID()))
	}
	return mergeInputWithCheckpoint(input, checkpoint), nil
}

func resumeDirective(input *Input) (string, bool) {
	if input == nil || len(input.Context) == 0 {
		return "", false
	}
	if checkpointID, ok := input.Context["checkpoint_id"].(string); ok && strings.TrimSpace(checkpointID) != "" {
		return strings.TrimSpace(checkpointID), true
	}
	if enabled, ok := input.Context["resume_latest"].(bool); ok && enabled {
		return "", true
	}
	if enabled, ok := input.Context["resume"].(bool); ok && enabled {
		return "", true
	}
	return "", false
}

func resumeThreadID(input *Input, fallbackAgentID string) string {
	if input == nil {
		return fallbackAgentID
	}
	if threadID := strings.TrimSpace(input.ChannelID); threadID != "" {
		return threadID
	}
	if traceID := strings.TrimSpace(input.TraceID); traceID != "" {
		return traceID
	}
	return fallbackAgentID
}

func mergeInputWithCheckpoint(input *Input, checkpoint *Checkpoint) *Input {
	merged := shallowCopyInput(input)
	if merged.Context == nil {
		merged.Context = make(map[string]any)
	}
	if checkpoint == nil {
		return merged
	}

	if strings.TrimSpace(merged.ChannelID) == "" {
		merged.ChannelID = checkpoint.ThreadID
	}
	merged.Context["checkpoint_id"] = checkpoint.ID
	merged.Context["resume_from_checkpoint"] = true
	merged.Context["resumable"] = true

	for key, value := range checkpoint.Metadata {
		merged.Context[key] = value
	}
	if checkpoint.ExecutionContext != nil {
		if strings.TrimSpace(checkpoint.ExecutionContext.CurrentNode) != "" {
			merged.Context["current_stage"] = checkpoint.ExecutionContext.CurrentNode
		}
		for key, value := range checkpoint.ExecutionContext.Variables {
			merged.Context[key] = value
		}
	}
	if strings.TrimSpace(merged.Content) == "" {
		if goal, ok := merged.Context["goal"].(string); ok && strings.TrimSpace(goal) != "" {
			merged.Content = goal
		}
	}
	return merged
}
