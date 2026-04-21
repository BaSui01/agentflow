package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
)

// execution_pipeline.go — (*BaseAgent) 维度的中间件实现。
// Pipeline / Middleware 核心类型、上下文键与 helper 仍定义在 base_agent.go。
// 本文件只承载 observability/skills/memoryLoad/promptEnhancer/memorySave 五个
// middleware 的 BaseAgent 绑定实现，避免 base_agent.go 继续承担所有中间件代码。

func (b *BaseAgent) observabilityMiddleware(options EnhancedExecutionOptions) ExecutionMiddleware {
	return func(ctx context.Context, input *Input, next ExecutionFunc) (*Output, error) {
		startTime := time.Now()
		traceID := input.TraceID
		sessionID := traceID
		if input != nil && strings.TrimSpace(input.ChannelID) != "" {
			sessionID = strings.TrimSpace(input.ChannelID)
		}
		b.extensions.ObservabilitySystemExt().StartTrace(traceID, b.ID())
		if recorder, ok := b.extensions.ObservabilitySystemExt().(ExplainabilityRecorder); ok {
			recorder.StartExplainabilityTrace(traceID, sessionID, b.ID())
		}

		output, err := next(ctx, input)

		if err != nil {
			b.extensions.ObservabilitySystemExt().EndTrace(traceID, "failed", err)
			if recorder, ok := b.extensions.ObservabilitySystemExt().(ExplainabilityRecorder); ok {
				recorder.EndExplainabilityTrace(traceID, false, "", err.Error())
			}
			return nil, err
		}
		duration := time.Since(startTime)
		if options.RecordMetrics {
			b.extensions.ObservabilitySystemExt().RecordTask(b.ID(), true, duration, output.TokensUsed, output.Cost, 0.8)
		}
		if options.RecordTrace {
			b.extensions.ObservabilitySystemExt().EndTrace(traceID, "completed", nil)
		}
		if recorder, ok := b.extensions.ObservabilitySystemExt().(ExplainabilityRecorder); ok {
			recorder.EndExplainabilityTrace(traceID, true, output.Content, "")
		}
		b.logger.Info("enhanced execution completed",
			zap.String("trace_id", input.TraceID),
			zap.Duration("total_duration", duration),
			zap.Int("tokens_used", output.TokensUsed),
			zap.Any("prompt_layer_ids", output.Metadata["applied_prompt_layer_ids"]),
			zap.Any("context_plan", output.Metadata["context_plan"]),
		)
		return output, nil
	}
}

func (b *BaseAgent) skillsMiddleware(options EnhancedExecutionOptions) ExecutionMiddleware {
	return func(ctx context.Context, input *Input, next ExecutionFunc) (*Output, error) {
		query := options.SkillsQuery
		if query == "" {
			query = input.Content
		}
		b.logger.Debug("discovering skills", zap.String("trace_id", input.TraceID), zap.String("query", query))

		var skillInstructions []string
		found, err := b.extensions.SkillManagerExt().DiscoverSkills(ctx, query)
		if err != nil {
			b.logger.Warn("skill discovery failed", zap.String("trace_id", input.TraceID), zap.Error(err))
		} else {
			for _, s := range found {
				if s == nil {
					continue
				}
				skillInstructions = append(skillInstructions, s.GetInstructions())
			}
			b.logger.Info("skills discovered", zap.Int("count", len(skillInstructions)))
		}

		skillInstructions = normalizeInstructionList(skillInstructions)
		if len(skillInstructions) > 0 {
			input = shallowCopyInput(input)
			if input.Context == nil {
				input.Context = make(map[string]any, 1)
			}
			input.Context["skill_context"] = append([]string(nil), skillInstructions...)
		}
		ctx = withSkillInstructions(ctx, skillInstructions)
		return next(ctx, input)
	}
}

func (b *BaseAgent) memoryLoadMiddleware(options EnhancedExecutionOptions) ExecutionMiddleware {
	return func(ctx context.Context, input *Input, next ExecutionFunc) (*Output, error) {
		var memoryContext []string
		if options.LoadWorkingMemory {
			b.logger.Debug("loading working memory", zap.String("trace_id", input.TraceID))
			working, err := b.extensions.EnhancedMemoryExt().LoadWorking(ctx, b.ID())
			if err != nil {
				b.logger.Warn("failed to load working memory", zap.String("trace_id", input.TraceID), zap.Error(err))
			} else {
				for _, entry := range working {
					if entry.Content != "" {
						memoryContext = append(memoryContext, entry.Content)
					}
				}
				b.logger.Info("working memory loaded", zap.String("trace_id", input.TraceID), zap.Int("count", len(working)))
			}
		}
		if options.LoadShortTermMemory {
			b.logger.Debug("loading short-term memory", zap.String("trace_id", input.TraceID))
			shortTerm, err := b.extensions.EnhancedMemoryExt().LoadShortTerm(ctx, b.ID(), 5)
			if err != nil {
				b.logger.Warn("failed to load short-term memory", zap.String("trace_id", input.TraceID), zap.Error(err))
			} else {
				for _, entry := range shortTerm {
					if entry.Content != "" {
						memoryContext = append(memoryContext, entry.Content)
					}
				}
				b.logger.Info("short-term memory loaded", zap.String("trace_id", input.TraceID), zap.Int("count", len(shortTerm)))
			}
		}

		if len(memoryContext) > 0 {
			input = shallowCopyInput(input)
			if input.Context == nil {
				input.Context = make(map[string]any, 1)
			}
			input.Context["memory_context"] = append([]string(nil), memoryContext...)
		}
		ctx = withMemoryContext(ctx, memoryContext)
		return next(ctx, input)
	}
}

func (b *BaseAgent) promptEnhancerMiddleware() ExecutionMiddleware {
	return func(ctx context.Context, input *Input, next ExecutionFunc) (*Output, error) {
		b.logger.Debug("enhancing prompt", zap.String("trace_id", input.TraceID))
		contextStr := ""
		if si := skillInstructionsFromCtx(ctx); len(si) > 0 {
			contextStr += "Skills: " + fmt.Sprintf("%v", si) + "\n"
		}
		if mc := memoryContextFromCtx(ctx); len(mc) > 0 {
			contextStr += "Memory: " + fmt.Sprintf("%v", mc) + "\n"
		}

		enhanced, err := b.extensions.PromptEnhancerExt().EnhanceUserPrompt(input.Content, contextStr)
		if err != nil {
			b.logger.Warn("prompt enhancement failed", zap.String("trace_id", input.TraceID), zap.Error(err))
		} else {
			input = shallowCopyInput(input)
			input.Content = enhanced
			b.logger.Info("prompt enhanced", zap.String("trace_id", input.TraceID))
		}
		return next(ctx, input)
	}
}

func (b *BaseAgent) memorySaveMiddleware() ExecutionMiddleware {
	return func(ctx context.Context, input *Input, next ExecutionFunc) (*Output, error) {
		output, err := next(ctx, input)
		if err != nil {
			return nil, err
		}
		if b.memoryRuntime != nil {
			return output, nil
		}
		b.logger.Debug("saving to enhanced memory", zap.String("trace_id", input.TraceID))
		b.extensions.SaveToEnhancedMemory(ctx, b.ID(), input, output, false)
		return output, nil
	}
}
