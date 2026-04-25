package runtime

import (
	"context"
	"strings"
	"time"

	agentcontext "github.com/BaSui01/agentflow/agent/execution/context"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

type runtimePromptContext struct {
	messages          []types.Message
	assembled         *agentcontext.AssembleResult
	traceFeedbackPlan TraceFeedbackPlan
}

func (b *BaseAgent) prepareRuntimePromptContext(ctx context.Context, input *Input, activeBundle PromptBundle, restoredMessages []types.Message) runtimePromptContext {
	if input == nil {
		return runtimePromptContext{}
	}
	logger := runtimePromptLogger(b)
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
		skillContext = normalizeInstructionList(agentcontext.SkillInstructionsFromContext(ctx))
	}
	publicContext := publicInputContext(input.Context)
	retrievalItems := retrievalItemsFromInputContext(input.Context)
	if len(retrievalItems) == 0 && b.retriever != nil {
		if records, err := b.retriever.Retrieve(ctx, input.Content, 5); err != nil {
			logger.Warn("failed to load retrieval context", zap.Error(err))
		} else {
			retrievalItems = retrievalItemsFromRecords(records)
		}
	}
	toolStates := toolStatesFromInputContext(input.Context)
	if len(toolStates) == 0 && b.toolState != nil {
		if snapshots, err := b.toolState.LoadToolState(ctx, b.ID()); err != nil {
			logger.Warn("failed to load tool state context", zap.Error(err))
		} else {
			toolStates = toolStatesFromSnapshots(snapshots)
		}
	}

	ephemeralLayers, traceFeedbackPlan := b.buildEphemeralPromptLayers(ctx, publicContext, input, systemContent, skillContext, memoryContext, conversation, retrievalItems, toolStates)
	messages, assembled := b.assembleMessages(ctx, systemContent, ephemeralLayers, skillContext, memoryContext, conversation, retrievalItems, toolStates, input.Content)
	if assembled != nil {
		logger.Debug("context assembled",
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
	return runtimePromptContext{
		messages:          messages,
		assembled:         assembled,
		traceFeedbackPlan: traceFeedbackPlan,
	}
}

func (b *BaseAgent) recordPromptLayerTimeline(traceID string, plan agentcontext.ContextPlan) {
	recorder, ok := runtimePromptObservability(b).(ExplainabilityTimelineRecorder)
	if !ok || strings.TrimSpace(traceID) == "" {
		return
	}
	recorder.AddExplainabilityTimeline(traceID, "prompt_layers", "Prompt layers assembled for this request", map[string]any{
		"context_plan":   plan,
		"applied_layers": plan.AppliedLayers,
		"layer_ids":      promptLayerIDs(plan.AppliedLayers),
	})
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
			runtimePromptLogger(b).Warn("context assembly failed, falling back to message construction", zap.Error(err))
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
			runtimePromptLogger(b).Warn("memory runtime recall failed", zap.Error(err))
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
	return planner.Plan(&agentcontext.TraceFeedbackPlanningInput{
		AgentID:          b.ID(),
		TraceID:          traceID,
		SessionID:        sessionID,
		UserInputContext: cloneAnyMap(inputContext(input)),
		Signals:          collectTraceFeedbackSignals(input, status, snapshot, b.memoryRuntime != nil),
		Snapshot:         agentcontext.ExplainabilitySynopsisSnapshot(snapshot),
		Config:           TraceFeedbackConfigFromAgentConfig(b.config),
	})
}

func (b *BaseAgent) latestTraceSynopsis(input *Input) string {
	snapshot := b.latestTraceSynopsisSnapshot(input)
	if strings.TrimSpace(snapshot.Synopsis) != "" {
		return strings.TrimSpace(snapshot.Synopsis)
	}
	reader, ok := runtimePromptObservability(b).(ExplainabilitySynopsisReader)
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
	reader, ok := runtimePromptObservability(b).(ExplainabilitySynopsisSnapshotReader)
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
	recorder, ok := runtimePromptObservability(b).(ExplainabilityTimelineRecorder)
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
	} else if allowed := b.config.ExecutionOptions().Tools.AllowedTools; len(allowed) > 0 {
		names = filterStringWhitelist(names, allowed)
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

func runtimePromptLogger(b *BaseAgent) *zap.Logger {
	if b == nil || b.logger == nil {
		return zap.NewNop()
	}
	return b.logger
}

func runtimePromptObservability(b *BaseAgent) ObservabilityRunner {
	if b == nil || b.extensions == nil {
		return nil
	}
	return b.extensions.ObservabilitySystemExt()
}
