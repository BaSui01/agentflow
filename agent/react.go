package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

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
	messages := []llm.Message{
		{
			Role:    llm.RoleSystem,
			Content: b.config.PromptBundle.RenderSystemPromptWithVars(input.Variables),
		},
		{
			Role:    llm.RoleUser,
			Content: planPrompt,
		},
	}

	// 调用 LLM
	resp, err := b.ChatCompletion(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("plan generation failed: %w", err)
	}

	// 解析计划
	choice, err := llm.FirstChoice(resp)
	if err != nil {
		return nil, fmt.Errorf("plan generation returned no choices: %w", err)
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
func (b *BaseAgent) Execute(ctx context.Context, input *Input) (*Output, error) {
	startTime := time.Now()

	// 1. 确保 Agent 就绪
	if err := b.EnsureReady(); err != nil {
		return nil, err
	}

	// 2. 尝试获取执行锁
	if !b.TryLockExec() {
		return nil, ErrAgentBusy
	}
	defer b.UnlockExec()

	// 3. 转换状态到运行中
	if err := b.Transition(ctx, StateRunning); err != nil {
		return nil, err
	}
	defer func() {
		if err := b.Transition(ctx, StateReady); err != nil {
			b.logger.Error("failed to transition to ready", zap.Error(err))
		}
	}()

	b.logger.Info("executing task",
		zap.String("trace_id", input.TraceID),
		zap.String("agent_id", b.config.ID),
		zap.String("agent_type", string(b.config.Type)),
	)

	// 4. 输入验证(监护)
	if b.guardrailsEnabled && b.inputValidatorChain != nil {
		validationResult, err := b.inputValidatorChain.Validate(ctx, input.Content)
		if err != nil {
			b.logger.Error("input validation error", zap.Error(err))
			return nil, fmt.Errorf("input validation error: %w", err)
		}

		if !validationResult.Valid {
			b.logger.Warn("input validation failed",
				zap.String("trace_id", input.TraceID),
				zap.Any("errors", validationResult.Errors),
			)

			// 从配置中检查失败动作
			failureAction := guardrails.FailureActionReject
			if b.config.Guardrails != nil {
				failureAction = b.config.Guardrails.OnInputFailure
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
	var contextMessages []llm.Message
	if b.memory != nil {
		b.recentMemoryMu.RLock()
		hasMemory := len(b.recentMemory) > 0
		if hasMemory {
			// 将最近的记忆转换为消息
			for _, mem := range b.recentMemory {
				if mem.Kind == MemoryShortTerm {
					contextMessages = append(contextMessages, llm.Message{
						Role:    llm.RoleAssistant,
						Content: mem.Content,
					})
				}
			}
		}
		b.recentMemoryMu.RUnlock()
	}

	// 6. 构建消息
	messages := []llm.Message{
		{
			Role:    llm.RoleSystem,
			Content: b.config.PromptBundle.RenderSystemPromptWithVars(input.Variables),
		},
	}

	// 添加上下文消息
	messages = append(messages, contextMessages...)

	// 添加用户输入
	messages = append(messages, llm.Message{
		Role:    llm.RoleUser,
		Content: input.Content,
	})

	// 7. 执行产出验证和重试支持
	// 要求2.4:对产出验证失败进行重试
	maxRetries := 0
	if b.config.Guardrails != nil {
		maxRetries = b.config.Guardrails.MaxRetries
	}

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
				messages = append(messages, llm.Message{
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
			return nil, fmt.Errorf("execution failed: %w", err)
		}

		var choiceErr error
		choice, choiceErr = llm.FirstChoice(resp)
		if choiceErr != nil {
			return nil, fmt.Errorf("execution returned no choices: %w", choiceErr)
		}
		outputContent = choice.Message.Content

		// 产出验证(护栏)
		if b.guardrailsEnabled && b.outputValidator != nil {
			var filteredContent string
			filteredContent, lastValidationResult, err = b.outputValidator.ValidateAndFilter(ctx, outputContent)
			if err != nil {
				b.logger.Error("output validation error", zap.Error(err))
				return nil, fmt.Errorf("output validation error: %w", err)
			}

			if !lastValidationResult.Valid {
				b.logger.Warn("output validation failed",
					zap.String("trace_id", input.TraceID),
					zap.Int("attempt", attempt),
					zap.Any("errors", lastValidationResult.Errors),
				)

				// 检查失败动作
				failureAction := guardrails.FailureActionReject
				if b.config.Guardrails != nil {
					failureAction = b.config.Guardrails.OnOutputFailure
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

	// 8. 保存记忆
	if b.memory != nil {
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

// Observe 处理反馈并更新 Agent 状态
// 这个方法允许 Agent 从外部反馈中学习和改进
func (b *BaseAgent) Observe(ctx context.Context, feedback *Feedback) error {
	b.logger.Info("observing feedback",
		zap.String("agent_id", b.config.ID),
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
			return fmt.Errorf("failed to save feedback: %w", err)
		}
	}

	// 2. 发布反馈事件
	if b.bus != nil {
		b.bus.Publish(&FeedbackEvent{
			AgentID_:     b.config.ID,
			FeedbackType: feedback.Type,
			Content:      feedback.Content,
			Data:         feedback.Data,
			Timestamp_:   time.Now(),
		})
	}

	b.logger.Info("feedback observed successfully",
		zap.String("agent_id", b.config.ID),
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



