package agent

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// =============================================================================
// ObservabilityStep — StartTrace / EndTrace wrapper
// =============================================================================

// ObservabilityStep wraps the pipeline with observability tracing.
type ObservabilityStep struct {
	options EnhancedExecutionOptions
}

func (s *ObservabilityStep) Name() string { return "observability" }

func (s *ObservabilityStep) Execute(ctx context.Context, pc *PipelineContext, next StepFunc) error {
	b := pc.agent
	obs := b.extensions.ObservabilitySystemExt()
	if obs == nil {
		return next(ctx, pc)
	}

	traceID := pc.Input.TraceID
	b.logger.Debug("trace started", zap.String("trace_id", traceID))
	obs.StartTrace(traceID, b.ID())

	err := next(ctx, pc)

	if err != nil {
		b.logger.Error("execution failed", zap.Error(err))
		obs.EndTrace(traceID, "failed", err)
		return err
	}

	if s.options.RecordMetrics {
		b.logger.Debug("recording metrics")
		obs.RecordTask(b.ID(), true, time.Since(pc.StartTime), pc.TokensUsed, 0, 0.8)
	}
	if s.options.RecordTrace {
		obs.EndTrace(traceID, "completed", nil)
	}

	return nil
}

// =============================================================================
// SkillsDiscoveryStep — discover skills and modify prompt
// =============================================================================

// SkillsDiscoveryStep discovers relevant skills and prepends instructions to the input.
type SkillsDiscoveryStep struct {
	options EnhancedExecutionOptions
}

func (s *SkillsDiscoveryStep) Name() string { return "skills_discovery" }

func (s *SkillsDiscoveryStep) Execute(ctx context.Context, pc *PipelineContext, next StepFunc) error {
	b := pc.agent
	sm := b.extensions.SkillManagerExt()
	if sm == nil {
		return next(ctx, pc)
	}

	query := s.options.SkillsQuery
	if query == "" {
		query = pc.Input.Content
	}
	b.logger.Debug("discovering skills", zap.String("query", query))

	found, err := sm.DiscoverSkills(ctx, query)
	if err != nil {
		b.logger.Warn("skill discovery failed", zap.Error(err))
		return next(ctx, pc)
	}

	var instructions []string
	for _, skill := range found {
		if skill == nil {
			continue
		}
		instructions = append(instructions, skill.GetInstructions())
	}
	b.logger.Info("skills discovered", zap.Int("count", len(instructions)))

	if len(instructions) > 0 {
		pc.Input.Content = prependSkillInstructions(pc.Input.Content, instructions)
		pc.Metadata["skill_instructions"] = instructions
	}

	return next(ctx, pc)
}

// =============================================================================
// EnhancedMemoryLoadStep — load working + short-term memory
// =============================================================================

// EnhancedMemoryLoadStep loads enhanced memory context.
type EnhancedMemoryLoadStep struct {
	options EnhancedExecutionOptions
}

func (s *EnhancedMemoryLoadStep) Name() string { return "enhanced_memory_load" }

func (s *EnhancedMemoryLoadStep) Execute(ctx context.Context, pc *PipelineContext, next StepFunc) error {
	b := pc.agent
	em := b.extensions.EnhancedMemoryExt()
	if em == nil {
		return next(ctx, pc)
	}

	var memoryContext []string

	if s.options.LoadWorkingMemory {
		b.logger.Debug("loading working memory")
		working, err := em.LoadWorking(ctx, b.ID())
		if err != nil {
			b.logger.Warn("failed to load working memory", zap.Error(err))
		} else {
			for _, w := range working {
				if wm, ok := w.(map[string]any); ok {
					if content, ok := wm["content"].(string); ok {
						memoryContext = append(memoryContext, content)
					}
				}
			}
			b.logger.Info("working memory loaded", zap.Int("count", len(working)))
		}
	}

	if s.options.LoadShortTermMemory {
		b.logger.Debug("loading short-term memory")
		shortTerm, err := em.LoadShortTerm(ctx, b.ID(), 5)
		if err != nil {
			b.logger.Warn("failed to load short-term memory", zap.Error(err))
		} else {
			for _, st := range shortTerm {
				if stm, ok := st.(map[string]any); ok {
					if content, ok := stm["content"].(string); ok {
						memoryContext = append(memoryContext, content)
					}
				}
			}
			b.logger.Info("short-term memory loaded", zap.Int("count", len(shortTerm)))
		}
	}

	if len(memoryContext) > 0 {
		pc.Metadata["enhanced_memory_context"] = memoryContext
	}

	// Mark skip base memory to avoid duplication
	if s.options.SaveToMemory {
		if pc.Input.Context == nil {
			pc.Input.Context = make(map[string]any)
		}
		pc.Input.Context["_skip_base_memory"] = true
	}

	return next(ctx, pc)
}

// =============================================================================
// PromptEnhancerStep — enhance user prompt
// =============================================================================

// PromptEnhancerStep enhances the user prompt using the prompt enhancer extension.
type PromptEnhancerStep struct{}

func (s *PromptEnhancerStep) Name() string { return "prompt_enhancer" }

func (s *PromptEnhancerStep) Execute(ctx context.Context, pc *PipelineContext, next StepFunc) error {
	b := pc.agent
	pe := b.extensions.PromptEnhancerExt()
	if pe == nil {
		return next(ctx, pc)
	}

	b.logger.Debug("enhancing prompt")

	contextStr := ""
	if instructions, ok := pc.Metadata["skill_instructions"].([]string); ok && len(instructions) > 0 {
		contextStr += "Skills: " + fmt.Sprintf("%v", instructions) + "\n"
	}
	if memCtx, ok := pc.Metadata["enhanced_memory_context"].([]string); ok && len(memCtx) > 0 {
		contextStr += "Memory: " + fmt.Sprintf("%v", memCtx) + "\n"
	}

	enhanced, err := pe.EnhanceUserPrompt(pc.Input.Content, contextStr)
	if err != nil {
		b.logger.Warn("prompt enhancement failed", zap.Error(err))
		return next(ctx, pc)
	}

	pc.Input.Content = enhanced
	b.logger.Info("prompt enhanced")

	return next(ctx, pc)
}

// =============================================================================
// ToolSelectionStep — dynamic tool selection
// =============================================================================

// ToolSelectionStep dynamically selects tools based on the task.
type ToolSelectionStep struct{}

func (s *ToolSelectionStep) Name() string { return "tool_selection" }

func (s *ToolSelectionStep) Execute(ctx context.Context, pc *PipelineContext, next StepFunc) error {
	b := pc.agent
	ts := b.extensions.ToolSelector()
	if ts == nil || b.llmEngine.toolManager == nil {
		return next(ctx, pc)
	}

	b.logger.Debug("selecting tools dynamically")
	availableTools := b.llmEngine.toolManager.GetAllowedTools(b.ID())
	selected, err := ts.SelectTools(ctx, pc.Input.Content, availableTools)
	if err != nil {
		b.logger.Warn("tool selection failed", zap.Error(err))
	} else {
		b.logger.Info("tools selected dynamically", zap.Any("selected", selected))
	}

	return next(ctx, pc)
}

// =============================================================================
// ReflectionLLMStep — execute with reflection
// =============================================================================

// ReflectionLLMStep uses the reflection executor instead of direct LLM call.
type ReflectionLLMStep struct{}

func (s *ReflectionLLMStep) Name() string { return "reflection_llm" }

func (s *ReflectionLLMStep) Execute(ctx context.Context, pc *PipelineContext, next StepFunc) error {
	b := pc.agent
	re := b.extensions.ReflectionExecutor()
	if re == nil {
		// Fallback to standard LLM execution
		step := &LLMExecutionStep{}
		return step.Execute(ctx, pc, next)
	}

	b.logger.Debug("executing with reflection")
	result, err := re.ExecuteWithReflection(ctx, pc.Input)
	if err != nil {
		return fmt.Errorf("reflection execution failed: %w", err)
	}

	if reflectionResult, ok := result.(interface{ GetFinalOutput() *Output }); ok {
		output := reflectionResult.GetFinalOutput()
		pc.OutputContent = output.Content
		pc.TokensUsed = output.TokensUsed
		pc.FinishReason = output.FinishReason
		pc.Metadata["model"] = "reflection"
		pc.Metadata["provider"] = "reflection"
	} else {
		// Fallback to standard LLM execution
		step := &LLMExecutionStep{}
		return step.Execute(ctx, pc, next)
	}

	return next(ctx, pc)
}

// =============================================================================
// EnhancedMemorySaveStep — save to enhanced memory
// =============================================================================

// EnhancedMemorySaveStep saves output to the enhanced memory system.
type EnhancedMemorySaveStep struct {
	options EnhancedExecutionOptions
}

func (s *EnhancedMemorySaveStep) Name() string { return "enhanced_memory_save" }

func (s *EnhancedMemorySaveStep) Execute(ctx context.Context, pc *PipelineContext, next StepFunc) error {
	b := pc.agent

	output := &Output{
		TraceID:      pc.Input.TraceID,
		Content:      pc.OutputContent,
		TokensUsed:   pc.TokensUsed,
		FinishReason: pc.FinishReason,
		Duration:     time.Since(pc.StartTime),
	}

	b.extensions.SaveToEnhancedMemory(ctx, b.ID(), pc.Input, output, s.options.UseReflection)

	return next(ctx, pc)
}
