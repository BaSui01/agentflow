package runtime

import (
	"context"
	"fmt"

	"github.com/BaSui01/agentflow/agent/capabilities/guardrails"
	planningcap "github.com/BaSui01/agentflow/agent/capabilities/planning"
	toolcap "github.com/BaSui01/agentflow/agent/capabilities/tools"
	agentcontext "github.com/BaSui01/agentflow/agent/execution/context"
	llm "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/llm/observability"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
	"strings"
	"sync/atomic"
	"time"
)

// preparedRequest holds the fully-built ChatRequest together with provider
// references needed by the execution paths (streaming, ReAct, plain completion).
type preparedRequest struct {
	req          *types.ChatRequest
	chatProvider llm.Provider
	toolProvider llm.Provider // for ReAct loop (may equal chatProvider)
	hasTools     bool
	handoffTools map[string]RuntimeHandoffTarget
	toolRisks    map[string]string
	maxReActIter int
	maxLoopIter  int
	options      types.ExecutionOptions
}

// prepareChatRequest builds a ChatRequest from messages, applying context
// engineering, model selection, RunConfig overrides, route hints, and tool
// filtering. Both ChatCompletion and StreamCompletion delegate here so that
// the logic is maintained in a single place.
func (b *BaseAgent) prepareChatRequest(ctx context.Context, messages []types.Message) (*preparedRequest, error) {
	if !b.hasMainExecutionSurface() {
		return nil, ErrProviderNotSet
	}
	if messages == nil || len(messages) == 0 {
		return nil, NewError(types.ErrInputValidation, "messages cannot be nil or empty")
	}

	chatProv := b.gatewayProvider()
	options := b.executionOptionsResolver().Resolve(ctx, b.config, nil)
	req, err := b.chatRequestAdapter().Build(options, messages)
	if err != nil {
		return nil, err
	}

	// 1. Tool whitelist filtering
	if b.toolManager != nil {
		allowedTools := b.toolManager.GetAllowedTools(b.config.Core.ID)
		switch {
		case options.Tools.DisableTools:
			req.Tools = nil
		case len(options.Tools.ToolWhitelist) > 0:
			req.Tools = filterToolSchemasByWhitelist(allowedTools, options.Tools.ToolWhitelist)
		case len(options.Tools.AllowedTools) > 0:
			req.Tools = filterToolSchemasByWhitelist(allowedTools, options.Tools.AllowedTools)
		}
	}
	handoffMap := map[string]RuntimeHandoffTarget(nil)
	handoffTargets := runtimeHandoffTargetsFromContext(ctx, b.config.Core.ID)
	if len(handoffTargets) > 0 {
		if len(req.Tools) == 0 {
			req.Tools = make([]types.ToolSchema, 0, len(handoffTargets))
		}
		handoffMap = make(map[string]RuntimeHandoffTarget, len(handoffTargets))
		seen := make(map[string]struct{}, len(req.Tools))
		for _, schema := range req.Tools {
			seen[schema.Name] = struct{}{}
		}
		for _, target := range handoffTargets {
			schema := runtimeHandoffToolSchema(target)
			handoffMap[schema.Name] = target
			if _, exists := seen[schema.Name]; exists {
				continue
			}
			seen[schema.Name] = struct{}{}
			req.Tools = append(req.Tools, schema)
		}
		if len(handoffMap) > 0 {
			if req.Metadata == nil {
				req.Metadata = make(map[string]string, 1)
			}
			req.Metadata["handoff_enabled"] = "true"
		}
	}

	// 2. 选择执行 provider。工具协议差异（如 XML fallback）统一在 llm/gateway 内处理。
	toolProv := chatProv
	if b.hasDedicatedToolExecutionSurface() {
		toolProv = b.gatewayToolProvider()
	}

	// 3. Effective loop budgets
	effectiveIter := options.Control.MaxReActIterations
	if effectiveIter <= 0 {
		effectiveIter = b.maxReActIterations()
	}
	toolRisks := make(map[string]string, len(req.Tools))
	for _, tool := range req.Tools {
		name := strings.TrimSpace(tool.Name)
		if name == "" {
			continue
		}
		toolRisks[name] = classifyToolRiskByName(name)
	}

	return &preparedRequest{
		req:          req,
		chatProvider: chatProv,
		toolProvider: toolProv,
		hasTools:     len(req.Tools) > 0 && (b.toolManager != nil || len(handoffTargets) > 0),
		handoffTools: handoffMap,
		toolRisks:    toolRisks,
		maxReActIter: effectiveIter,
		maxLoopIter:  options.Control.MaxLoopIterations,
		options:      options,
	}, nil
}

// lastUserQuery extracts the content of the last user message.
func lastUserQuery(messages []types.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == types.RoleUser {
			return messages[i].Content
		}
	}
	return ""
}

// effectiveToolModel returns the tool-specific model if configured, otherwise
// falls back to the main model.
func effectiveToolModel(mainModel string, configuredToolModel string) string {
	if v := strings.TrimSpace(configuredToolModel); v != "" {
		return v
	}
	return mainModel
}

// Merged from ephemeral_prompt.go.
type EphemeralPromptLayerBuilder = agentcontext.EphemeralPromptLayerBuilder
type EphemeralPromptLayerInput = agentcontext.EphemeralPromptLayerInput

func NewEphemeralPromptLayerBuilder() *EphemeralPromptLayerBuilder {
	return agentcontext.NewEphemeralPromptLayerBuilder()
}

type TraceFeedbackAction = agentcontext.TraceFeedbackAction
type TraceFeedbackSignals = agentcontext.TraceFeedbackSignals
type TraceFeedbackPlan = agentcontext.TraceFeedbackPlan
type TraceFeedbackConfig = agentcontext.TraceFeedbackConfig

const (
	TraceFeedbackSkip               = agentcontext.TraceFeedbackSkip
	TraceFeedbackSynopsisOnly       = agentcontext.TraceFeedbackSynopsisOnly
	TraceFeedbackHistoryOnly        = agentcontext.TraceFeedbackHistoryOnly
	TraceFeedbackMemoryRecallOnly   = agentcontext.TraceFeedbackMemoryRecallOnly
	TraceFeedbackSynopsisAndHistory = agentcontext.TraceFeedbackSynopsisAndHistory
)

type TraceFeedbackPlanner = agentcontext.TraceFeedbackPlanner
type TraceFeedbackPlanAdapter = agentcontext.TraceFeedbackPlanAdapter

func NewRuleBasedTraceFeedbackPlanner() TraceFeedbackPlanner {
	return agentcontext.NewRuleBasedTraceFeedbackPlanner()
}

func NewComposedTraceFeedbackPlanner(base TraceFeedbackPlanner, adapters ...TraceFeedbackPlanAdapter) TraceFeedbackPlanner {
	return agentcontext.NewComposedTraceFeedbackPlanner(base, adapters...)
}

func NewHintTraceFeedbackAdapter() TraceFeedbackPlanAdapter {
	return agentcontext.NewHintTraceFeedbackAdapter()
}

func DefaultTraceFeedbackConfig() TraceFeedbackConfig {
	return agentcontext.DefaultTraceFeedbackConfig()
}

func TraceFeedbackConfigFromAgentConfig(cfg types.AgentConfig) TraceFeedbackConfig {
	return agentcontext.TraceFeedbackConfigFromAgentConfig(cfg)
}

func collectTraceFeedbackSignals(input *Input, status *agentcontext.Status, snapshot ExplainabilitySynopsisSnapshot, hasMemoryRuntime bool) TraceFeedbackSignals {
	var acceptanceCriteriaCount int
	var handoff bool
	if input != nil {
		acceptanceCriteriaCount = len(normalizeStringSlice(acceptanceCriteriaForValidation(input, nil)))
		handoff = len(handoffMessagesFromInputContext(input.Context)) > 0
	}
	return agentcontext.CollectTraceFeedbackSignals(agentcontext.CollectTraceFeedbackSignalsInput{
		UserInputContext:        inputContext(input),
		Snapshot:                agentcontext.ExplainabilitySynopsisSnapshot(snapshot),
		HasMemoryRuntime:        hasMemoryRuntime,
		ContextStatus:           status,
		AcceptanceCriteriaCount: acceptanceCriteriaCount,
		Handoff:                 handoff,
	})
}

func cloneAnyMap(values map[string]any) map[string]any {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]any, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

// Merged from remote_tool_transport.go.
type RemoteToolTargetKind = toolcap.RemoteToolTargetKind

const (
	RemoteToolTargetHTTP  = toolcap.RemoteToolTargetHTTP
	RemoteToolTargetMCP   = toolcap.RemoteToolTargetMCP
	RemoteToolTargetA2A   = toolcap.RemoteToolTargetA2A
	RemoteToolTargetStdio = toolcap.RemoteToolTargetStdio
)

type RemoteToolTarget = toolcap.RemoteToolTarget
type ToolInvocationRequest = toolcap.ToolInvocationRequest
type ToolInvocationResult = toolcap.ToolInvocationResult
type RemoteToolTransport = toolcap.RemoteToolTransport

func NewDefaultRemoteToolTransport(logger *zap.Logger) RemoteToolTransport {
	return toolcap.NewDefaultRemoteToolTransport(logger)
}

// Merged from react.go.

// defaultCostCalc is a package-level cost calculator for estimating LLM call costs.
var defaultCostCalc = observability.NewCostCalculator()

const submitNumberedPlanTool = planningcap.SubmitNumberedPlanTool

// Plan 生成执行计划。
// 使用 LLM 分析任务并生成详细的执行步骤。
func (b *BaseAgent) Plan(ctx context.Context, input *Input) (*PlanResult, error) {
	if !b.hasMainExecutionSurface() {
		return nil, ErrProviderNotSet
	}

	planPrompt := fmt.Sprintf(`Plan the execution of this task for another agent.

Task:
%s

Use the %s tool to return the plan.

Requirements:
- Keep each step directly executable
- Prefer tool-first actions when tools are needed
- Mention dependencies or risks only when they affect execution
- Do not answer with prose outside the tool call`, input.Content, planningcap.SubmitNumberedPlanTool)

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
	nativeToolSupport := pr.chatProvider.SupportsNativeFunctionCalling()
	if !nativeToolSupport && b.mainProviderCompat != nil {
		nativeToolSupport = b.mainProviderCompat.SupportsNativeFunctionCalling()
	}
	pr.req.Tools = []types.ToolSchema{planningcap.NumberedPlanToolSchema()}
	pr.req.ToolChoice = &types.ToolChoice{Mode: types.ToolChoiceModeRequired}
	if nativeToolSupport {
		pr.req.ToolCallMode = types.ToolCallModeNative
	} else {
		pr.req.ToolCallMode = types.ToolCallModeXML
	}

	resp, err := pr.chatProvider.Completion(ctx, pr.req)
	if err != nil {
		return nil, NewErrorWithCause(types.ErrAgentExecution, "plan generation failed", err)
	}

	if resp == nil || len(resp.Choices) == 0 {
		return nil, NewError(types.ErrLLMResponseEmpty, "plan generation returned no choices")
	}
	choice := resp.FirstChoice()
	steps, parseErr := planningcap.ParseNumberedPlanToolCall(choice.Message)
	if parseErr != nil {
		return nil, NewErrorWithCause(types.ErrAgentExecution, "plan generation did not return tool call", parseErr)
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

// Execute 执行任务（完整的 ReAct 循环）。
// 这是 Agent 的核心执行方法，包含完整的推理-行动循环。
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

	if input.TraceID != "" {
		ctx = types.WithTraceID(ctx, input.TraceID)
	}
	if input.TenantID != "" {
		ctx = types.WithTenantID(ctx, input.TenantID)
	}
	if input.UserID != "" {
		ctx = types.WithUserID(ctx, input.UserID)
	}

	ctx = agentcontext.ApplyInputContext(ctx, input.Context)

	if runConfig := ResolveRunConfig(ctx, input); runConfig != nil {
		ctx = WithRunConfig(ctx, runConfig)
	}

	if !b.TryLockExec() {
		return nil, ErrAgentBusy
	}
	defer b.UnlockExec()

	if err := b.EnsureReady(); err != nil {
		return nil, err
	}

	if atomic.AddInt64(&b.execCount, 1) == 1 {
		if err := b.Transition(ctx, StateRunning); err != nil {
			atomic.AddInt64(&b.execCount, -1)
			return nil, err
		}
	}

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

	persistenceSession := b.beginRuntimePersistence(ctx, input, startTime)
	restoredMessages := persistenceSession.restoredMessages
	defer func() {
		if atomic.AddInt64(&b.execCount, -1) == 0 {
			if err := b.Transition(context.Background(), StateReady); err != nil {
				b.logger.Error("failed to transition to ready", zap.Error(err))
			}
		}
	}()
	defer b.finishRuntimePersistenceOnExit(ctx, persistenceSession, &execErr)

	b.logger.Info("executing task",
		zap.String("trace_id", input.TraceID),
		zap.String("agent_id", b.config.Core.ID),
		zap.String("agent_type", b.config.Core.Type),
	)

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

	promptContext := b.prepareRuntimePromptContext(ctx, input, activeBundle, restoredMessages)
	messages := promptContext.messages
	assembled := promptContext.assembled
	traceFeedbackPlan := promptContext.traceFeedbackPlan
	ctx = b.withApprovalExplainability(ctx, input)

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

			if lastValidationResult != nil {
				feedbackMsg := b.buildValidationFeedbackMessage(lastValidationResult)
				messages = append(messages, types.Message{
					Role:    types.RoleUser,
					Content: feedbackMsg,
				})
			}
		}

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

				failureAction := guardrails.FailureActionReject
				if runtimeGuardrailsCfgForOutput != nil {
					failureAction = runtimeGuardrailsCfgForOutput.OnOutputFailure
				}

				if failureAction == guardrails.FailureActionRetry && attempt < maxRetries {
					continue
				}

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
					return nil, &GuardrailsError{
						Type:    GuardrailsErrorTypeOutput,
						Message: fmt.Sprintf("output validation failed after %d retries", maxRetries),
						Errors:  lastValidationResult.Errors,
					}
				}
			} else {
				outputContent = filteredContent
			}
		}

		break
	}

	estimatedCost := defaultCostCalc.Calculate(resp.Provider, resp.Model, resp.Usage.PromptTokens, resp.Usage.CompletionTokens)

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

	b.completeRuntimePersistence(ctx, persistenceSession, input, runtimePersistenceCompletion{
		outputContent: outputContent,
		tokensUsed:    resp.Usage.TotalTokens,
		cost:          estimatedCost,
		finishReason:  choice.FinishReason,
	})

	b.logger.Info("execution completed",
		zap.String("trace_id", input.TraceID),
		zap.Duration("duration", duration),
		zap.Int("tokens_used", resp.Usage.TotalTokens),
	)

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
