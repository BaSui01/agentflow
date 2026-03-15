package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/types"

	"github.com/BaSui01/agentflow/agent/guardrails"
	"github.com/BaSui01/agentflow/llm"

	"go.uber.org/zap"
)

// Plan 生成执行计划
// 使用 LLM 分析任务并生成详细的执行步骤
func (b *BaseAgent) Plan(ctx context.Context, input *Input) (*PlanResult, error) {
	if b.provider == nil {
		return nil, ErrProviderNotSet
	}

	// 构建规划提示词
	planPrompt := fmt.Sprintf(`你是一个任务规划专家。请为以下任务制定详细的执行计划。

任务描述：
%s

请按照以下格式输出执行计划：
1. 第一步：[步骤描述]
2. 第二步：[步骤描述]
3. ...

要求：
- 步骤要具体、可执行
- 考虑可能的风险和依赖关系
- 估算每个步骤的复杂度`, input.Content)

	// 构建消息
	messages := []types.Message{
		{
			Role:    llm.RoleSystem,
			Content: b.promptBundle.RenderSystemPromptWithVars(input.Variables),
		},
		{
			Role:    llm.RoleUser,
			Content: planPrompt,
		},
	}

	// 调用 LLM
	resp, err := b.ChatCompletion(ctx, messages)
	if err != nil {
		return nil, NewErrorWithCause(types.ErrAgentExecution, "plan generation failed", err)
	}

	// 解析计划
	choice, err := llm.FirstChoice(resp)
	if err != nil {
		return nil, NewErrorWithCause(types.ErrLLMResponseEmpty, "plan generation returned no choices", err)
	}
	planContent := choice.Message.Content
	steps := parsePlanSteps(planContent)

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
	if input == nil {
		return nil, NewError(types.ErrInputValidation, "input is nil")
	}
	if strings.TrimSpace(input.Content) == "" {
		return nil, NewError(types.ErrInputValidation, "input content is empty")
	}
	startTime := time.Now()

	// 0. Apply Input.Overrides to context (takes precedence over existing RunConfig)
	if input.Overrides != nil {
		ctx = WithRunConfig(ctx, input.Overrides)
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

	// 3. 转换状态到运行中
	if err := b.Transition(ctx, StateRunning); err != nil {
		return nil, err
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
		if err := b.Transition(context.Background(), StateReady); err != nil {
			b.logger.Error("failed to transition to ready", zap.Error(err))
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

	// 5. 加载最近的记忆（如果有）
	var contextMessages []types.Message
	if b.memory != nil {
		b.recentMemoryMu.RLock()
		hasMemory := len(b.recentMemory) > 0
		if hasMemory {
			// 将最近的记忆转换为消息
			for _, mem := range b.recentMemory {
				if mem.Kind == MemoryShortTerm {
					role := llm.RoleAssistant
					if r, ok := mem.Metadata["role"].(string); ok && r != "" {
						role = types.Role(r)
					}
					contextMessages = append(contextMessages, types.Message{
						Role:    role,
						Content: mem.Content,
					})
				}
			}
		}
		b.recentMemoryMu.RUnlock()
	}

	// 6. 构建消息
	msgCap := 1 + len(contextMessages) + len(restoredMessages) + 1
	messages := make([]types.Message, 0, msgCap)
	messages = append(messages, types.Message{
		Role:    llm.RoleSystem,
		Content: activeBundle.RenderSystemPromptWithVars(input.Variables),
	})

	// 添加上下文消息
	messages = append(messages, contextMessages...)

	// 添加从 ConversationStore 恢复的历史消息
	if len(restoredMessages) > 0 {
		messages = append(messages, restoredMessages...)
	}

	// 添加用户输入
	messages = append(messages, types.Message{
		Role:    llm.RoleUser,
		Content: input.Content,
	})

	// 7. 执行产出验证和重试支持
	// 要求2.4:对产出验证失败进行重试
	b.configMu.RLock()
	maxRetries := 0
	if b.runtimeGuardrailsCfg != nil {
		maxRetries = b.runtimeGuardrailsCfg.MaxRetries
	}
	outputValidator := b.outputValidator
	guardrailsEnabledForOutput := b.guardrailsEnabled
	runtimeGuardrailsCfgForOutput := b.runtimeGuardrailsCfg
	b.configMu.RUnlock()

	var resp *llm.ChatResponse
	var outputContent string
	var lastValidationResult *guardrails.ValidationResult
	var choice llm.ChatChoice

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
					Role:    llm.RoleUser,
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

		var choiceErr error
		choice, choiceErr = llm.FirstChoice(resp)
		if choiceErr != nil {
			return nil, NewErrorWithCause(types.ErrLLMResponseEmpty, "execution returned no choices", choiceErr)
		}
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

	// 8. 保存记忆（如果增强记忆已接管，则跳过基础记忆保存）
	skipBaseMemory := b.memoryFacade != nil && b.memoryFacade.SkipBaseMemory()
	if b.memory != nil && !skipBaseMemory {
		// 保存用户输入
		if err := b.SaveMemory(ctx, input.Content, MemoryShortTerm, map[string]any{
			"trace_id": input.TraceID,
			"role":     "user",
		}); err != nil {
			b.logger.Warn("failed to save user input to memory", zap.Error(err))
		}

		// 保存 Agent 响应
		if err := b.SaveMemory(ctx, outputContent, MemoryShortTerm, map[string]any{
			"trace_id": input.TraceID,
			"role":     "assistant",
		}); err != nil {
			b.logger.Warn("failed to save response to memory", zap.Error(err))
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
			Cost:         0, // cost is computed elsewhere
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
	return &Output{
		TraceID: input.TraceID,
		Content: outputContent,
		Metadata: map[string]any{
			"model":    resp.Model,
			"provider": resp.Provider,
		},
		TokensUsed:   resp.Usage.TotalTokens,
		Duration:     duration,
		FinishReason: choice.FinishReason,
	}, nil
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

	// 1. 保存反馈到长期记忆
	if b.memory != nil {
		metadata := map[string]any{
			"feedback_type": feedback.Type,
			"timestamp":     time.Now(),
		}

		// 合并额外的数据
		for k, v := range feedback.Data {
			metadata[k] = v
		}

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

// parsePlanSteps 从 LLM 响应中解析执行步骤
func parsePlanSteps(content string) []string {
	var steps []string
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// 匹配 "1. xxx" 或 "- xxx" 格式
		if len(line) > 2 && (line[0] >= '0' && line[0] <= '9' || line[0] == '-') {
			// 移除序号
			if idx := strings.Index(line, "."); idx > 0 && idx < 5 {
				line = strings.TrimSpace(line[idx+1:])
			} else if line[0] == '-' {
				line = strings.TrimSpace(line[1:])
			}

			if line != "" {
				steps = append(steps, line)
			}
		}
	}

	// 如果没有解析到步骤，将整个内容作为一个步骤
	if len(steps) == 0 {
		steps = append(steps, content)
	}

	return steps
}
