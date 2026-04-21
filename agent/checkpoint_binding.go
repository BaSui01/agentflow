package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

func (s *LoopState) CheckpointVariables() map[string]any {
	if s == nil {
		return nil
	}
	s.normalizeCheckpointFields()
	variables := map[string]any{
		"loop_state_id":           s.LoopStateID,
		"run_id":                  s.RunID,
		"agent_id":                s.AgentID,
		"goal":                    s.Goal,
		"plan":                    append([]string(nil), s.Plan...),
		"acceptance_criteria":     cloneStringSlice(s.AcceptanceCriteria),
		"unresolved_items":        cloneStringSlice(s.UnresolvedItems),
		"remaining_risks":         cloneStringSlice(s.RemainingRisks),
		"current_plan_id":         s.CurrentPlanID,
		"plan_version":            s.PlanVersion,
		"current_step":            s.CurrentStepID,
		"current_step_id":         s.CurrentStepID,
		"current_stage":           string(s.CurrentStage),
		"iteration":               s.Iteration,
		"iteration_count":         s.Iteration,
		"max_iterations":          s.MaxIterations,
		"decision":                string(s.Decision),
		"stop_reason":             string(s.StopReason),
		"selected_reasoning_mode": s.SelectedReasoningMode,
		"confidence":              s.Confidence,
		"need_human":              s.NeedHuman,
		"checkpoint_id":           s.CheckpointID,
		"resumable":               s.Resumable,
		"validation_status":       string(s.ValidationStatus),
		"validation_summary":      s.ValidationSummary,
		"observations_summary":    s.ObservationsSummary,
		"last_output_summary":     s.LastOutputSummary,
		"last_error":              s.LastError,
	}
	if len(s.Observations) > 0 {
		variables["loop_observations"] = append([]LoopObservation(nil), s.Observations...)
	}
	return variables
}

func (s *LoopState) PopulateCheckpoint(checkpoint *Checkpoint) {
	if s == nil || checkpoint == nil {
		return
	}
	variables := s.CheckpointVariables()
	metadata := cloneMetadata(checkpoint.Metadata)
	if metadata == nil {
		metadata = make(map[string]any, len(variables))
	}
	for key, value := range variables {
		metadata[key] = value
	}
	var executionContext *ExecutionContext
	if checkpoint.ExecutionContext != nil {
		copied := *checkpoint.ExecutionContext
		executionContext = &copied
	} else {
		executionContext = &ExecutionContext{}
	}
	if executionContext.Variables == nil {
		executionContext.Variables = make(map[string]any, len(variables))
	}
	for key, value := range variables {
		executionContext.Variables[key] = value
	}
	checkpoint.AgentID = s.AgentID
	checkpoint.LoopStateID = s.LoopStateID
	checkpoint.RunID = s.RunID
	checkpoint.Goal = s.Goal
	checkpoint.AcceptanceCriteria = cloneStringSlice(s.AcceptanceCriteria)
	checkpoint.UnresolvedItems = cloneStringSlice(s.UnresolvedItems)
	checkpoint.RemainingRisks = cloneStringSlice(s.RemainingRisks)
	checkpoint.CurrentPlanID = s.CurrentPlanID
	checkpoint.PlanVersion = s.PlanVersion
	checkpoint.CurrentStepID = s.CurrentStepID
	checkpoint.ValidationStatus = s.ValidationStatus
	checkpoint.ValidationSummary = s.ValidationSummary
	checkpoint.ObservationsSummary = s.ObservationsSummary
	checkpoint.LastOutputSummary = s.LastOutputSummary
	checkpoint.LastError = s.LastError
	checkpoint.Metadata = metadata
	executionContext.CurrentNode = string(s.CurrentStage)
	executionContext.LoopStateID = s.LoopStateID
	executionContext.RunID = s.RunID
	executionContext.AgentID = s.AgentID
	executionContext.Goal = s.Goal
	executionContext.AcceptanceCriteria = cloneStringSlice(s.AcceptanceCriteria)
	executionContext.UnresolvedItems = cloneStringSlice(s.UnresolvedItems)
	executionContext.RemainingRisks = cloneStringSlice(s.RemainingRisks)
	executionContext.CurrentPlanID = s.CurrentPlanID
	executionContext.PlanVersion = s.PlanVersion
	executionContext.CurrentStepID = s.CurrentStepID
	executionContext.ValidationStatus = s.ValidationStatus
	executionContext.ValidationSummary = s.ValidationSummary
	executionContext.ObservationsSummary = s.ObservationsSummary
	executionContext.LastOutputSummary = s.LastOutputSummary
	executionContext.LastError = s.LastError
	checkpoint.ExecutionContext = executionContext
}

func (s *LoopState) restoreFromContext(values map[string]any) {
	if s == nil || len(values) == 0 {
		return
	}
	s.restoreCoreContext(values)
	s.restorePlanContext(values)
	s.restoreExecutionContext(values)
	s.restoreValidationContext(values)
	s.normalizeCheckpointFields()
}

func (s *LoopState) restoreCoreContext(values map[string]any) {
	if value, ok := loopContextString(values, "loop_state_id"); ok {
		s.LoopStateID = value
	}
	if value, ok := loopContextString(values, "run_id"); ok {
		s.RunID = value
	}
	if value, ok := loopContextString(values, "agent_id"); ok {
		s.AgentID = value
	}
	if value, ok := loopContextString(values, "goal"); ok {
		s.Goal = value
	}
}

func (s *LoopState) restorePlanContext(values map[string]any) {
	if plan, ok := loopContextStrings(values, "loop_plan", "plan"); ok {
		s.Plan = plan
	}
	if criteria, ok := loopContextStrings(values, "acceptance_criteria"); ok {
		s.AcceptanceCriteria = criteria
	}
	if items, ok := loopContextStrings(values, "unresolved_items"); ok {
		s.UnresolvedItems = items
	}
	if risks, ok := loopContextStrings(values, "remaining_risks"); ok {
		s.RemainingRisks = risks
	}
	if value, ok := loopContextString(values, "current_plan_id"); ok {
		s.CurrentPlanID = value
	}
	if value, ok := loopContextInt(values, "plan_version"); ok {
		s.PlanVersion = value
	}
	if value, ok := loopContextString(values, "current_step", "current_step_id"); ok {
		s.CurrentStepID = value
	}
}

func (s *LoopState) restoreExecutionContext(values map[string]any) {
	if value, ok := loopContextString(values, "current_stage"); ok {
		s.CurrentStage = LoopStage(value)
	}
	if value, ok := loopContextInt(values, "iteration", "iteration_count", "loop_iteration_count"); ok {
		s.Iteration = value
	}
	if value, ok := loopContextInt(values, "max_iterations"); ok && value > 0 {
		s.MaxIterations = value
	}
	if value, ok := loopContextString(values, "decision"); ok {
		s.Decision = LoopDecision(value)
	}
	if value, ok := loopContextString(values, "stop_reason", "loop_stop_reason"); ok {
		s.StopReason = StopReason(value)
	}
	if value, ok := loopContextString(values, "selected_reasoning_mode"); ok {
		s.SelectedReasoningMode = value
	}
	if value, ok := loopContextBool(values, "resumable"); ok {
		s.Resumable = value
	}
	if value, ok := loopContextString(values, "checkpoint_id"); ok {
		s.CheckpointID = value
	}
}

func (s *LoopState) restoreValidationContext(values map[string]any) {
	if value, ok := loopContextString(values, "validation_status"); ok {
		s.ValidationStatus = LoopValidationStatus(value)
	}
	if value, ok := loopContextString(values, "validation_summary"); ok {
		s.ValidationSummary = value
	}
	if value, ok := loopContextFloat(values, "confidence", "loop_confidence"); ok {
		s.Confidence = value
	}
	if value, ok := loopContextBool(values, "need_human", "loop_need_human"); ok {
		s.NeedHuman = value
	}
	if value, ok := loopContextString(values, "observations_summary"); ok {
		s.ObservationsSummary = value
	}
	if value, ok := loopContextString(values, "last_output_summary"); ok {
		s.LastOutputSummary = value
	}
	if value, ok := loopContextString(values, "last_error"); ok {
		s.LastError = value
	}
	if observations, ok := loopContextObservations(values, "loop_observations", "observations"); ok {
		s.Observations = observations
	}
}

func (s *LoopState) normalizeCheckpointFields() {
	if s == nil {
		return
	}
	if s.PlanVersion <= 0 && len(s.Plan) > 0 {
		s.PlanVersion = derivePlanVersion(s.Observations)
		if s.PlanVersion <= 0 {
			s.PlanVersion = 1
		}
	}
	if s.CurrentPlanID == "" && s.PlanVersion > 0 {
		s.CurrentPlanID = buildLoopPlanID(s.LoopStateID, s.PlanVersion)
	}
	s.AcceptanceCriteria = normalizeStringSlice(s.AcceptanceCriteria)
	s.UnresolvedItems = normalizeStringSlice(s.UnresolvedItems)
	s.RemainingRisks = normalizeStringSlice(s.RemainingRisks)
	if s.CurrentStepID == "" {
		s.SyncCurrentStep()
	}
	if s.ValidationSummary == "" {
		s.ValidationSummary = summarizeValidationState(s.ValidationStatus, s.UnresolvedItems, s.RemainingRisks)
	}
	if s.ObservationsSummary == "" {
		s.ObservationsSummary = summarizeObservations(s.Observations)
	}
	if s.LastOutputSummary == "" {
		s.LastOutputSummary = summarizeLastOutput(s.LastOutput, s.Observations)
	}
	if s.LastError == "" {
		s.LastError = summarizeLastError(s.Observations)
	}
}

func buildLoopPlanID(loopStateID string, planVersion int) string {
	base := strings.TrimSpace(loopStateID)
	if base == "" {
		base = "loop"
	}
	return fmt.Sprintf("%s-plan-%d", base, planVersion)
}

func derivePlanVersion(observations []LoopObservation) int {
	count := 0
	for _, observation := range observations {
		if observation.Stage == LoopStagePlan {
			count++
		}
	}
	return count
}

func summarizeObservations(observations []LoopObservation) string {
	if len(observations) == 0 {
		return ""
	}
	parts := make([]string, 0, 3)
	start := len(observations) - 3
	if start < 0 {
		start = 0
	}
	for _, observation := range observations[start:] {
		part := string(observation.Stage)
		if text := strings.TrimSpace(observation.Error); text != "" {
			part += ":" + summarizeText(text)
		} else if text := strings.TrimSpace(observation.Content); text != "" {
			part += ":" + summarizeText(text)
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, " | ")
}

func summarizeLastOutput(output *Output, observations []LoopObservation) string {
	if output != nil {
		if text := strings.TrimSpace(output.Content); text != "" {
			return summarizeText(text)
		}
	}
	for i := len(observations) - 1; i >= 0; i-- {
		observation := observations[i]
		if observation.Stage == LoopStageAct {
			if text := strings.TrimSpace(observation.Content); text != "" {
				return summarizeText(text)
			}
		}
	}
	return ""
}

func summarizeLastError(observations []LoopObservation) string {
	for i := len(observations) - 1; i >= 0; i-- {
		if text := strings.TrimSpace(observations[i].Error); text != "" {
			return summarizeText(text)
		}
	}
	return ""
}

func summarizeValidationState(status LoopValidationStatus, unresolvedItems, remainingRisks []string) string {
	if len(unresolvedItems) == 0 && len(remainingRisks) == 0 {
		switch status {
		case LoopValidationStatusPassed:
			return "validation passed"
		case LoopValidationStatusPending:
			return "validation pending"
		case LoopValidationStatusFailed:
			return "validation failed"
		default:
			return ""
		}
	}
	parts := make([]string, 0, 2)
	if len(unresolvedItems) > 0 {
		parts = append(parts, "unresolved: "+strings.Join(unresolvedItems, ", "))
	}
	if len(remainingRisks) > 0 {
		parts = append(parts, "risks: "+strings.Join(remainingRisks, ", "))
	}
	return strings.Join(parts, "; ")
}

func summarizeText(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	runes := []rune(trimmed)
	if len(runes) <= 160 {
		return trimmed
	}
	return string(runes[:160]) + "..."
}

func loopContextString(values map[string]any, keys ...string) (string, bool) {
	for _, key := range keys {
		raw, ok := values[key]
		if !ok {
			continue
		}
		value, ok := raw.(string)
		if ok && value != "" {
			return value, true
		}
	}
	return "", false
}

func loopContextStrings(values map[string]any, keys ...string) ([]string, bool) {
	for _, key := range keys {
		raw, ok := values[key]
		if !ok {
			continue
		}
		switch typed := raw.(type) {
		case string:
			trimmed := strings.TrimSpace(typed)
			if trimmed != "" {
				return []string{trimmed}, true
			}
		case []string:
			return append([]string(nil), typed...), true
		case []any:
			result := make([]string, 0, len(typed))
			for _, item := range typed {
				text, ok := item.(string)
				if ok && text != "" {
					result = append(result, text)
				}
			}
			if len(result) > 0 {
				return result, true
			}
		}
	}
	return nil, false
}

func loopContextInt(values map[string]any, keys ...string) (int, bool) {
	for _, key := range keys {
		raw, ok := values[key]
		if !ok {
			continue
		}
		switch typed := raw.(type) {
		case int:
			return typed, true
		case int32:
			return int(typed), true
		case int64:
			return int(typed), true
		case float64:
			return int(typed), true
		}
	}
	return 0, false
}

func loopContextFloat(values map[string]any, keys ...string) (float64, bool) {
	for _, key := range keys {
		raw, ok := values[key]
		if !ok {
			continue
		}
		switch typed := raw.(type) {
		case float64:
			return typed, true
		case float32:
			return float64(typed), true
		case int:
			return float64(typed), true
		}
	}
	return 0, false
}

func loopContextBool(values map[string]any, keys ...string) (value, ok bool) {
	for _, key := range keys {
		raw, found := values[key]
		if !found {
			continue
		}
		value, ok = raw.(bool)
		if ok {
			return value, true
		}
	}
	return false, false
}

func loopContextObservations(values map[string]any, keys ...string) ([]LoopObservation, bool) {
	for _, key := range keys {
		raw, ok := values[key]
		if !ok {
			continue
		}
		observations, ok := raw.([]LoopObservation)
		if ok && len(observations) > 0 {
			return append([]LoopObservation(nil), observations...), true
		}
	}
	return nil, false
}

func cloneStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return append([]string(nil), values...)
}

func normalizeStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		normalized = append(normalized, trimmed)
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

// CheckpointManager 检查点管理器
type CheckpointManager struct {
	store  CheckpointStore
	logger *zap.Logger

	// 自动保存配置
	autoSaveEnabled  bool
	autoSaveInterval time.Duration
	autoSaveCancel   context.CancelFunc
	autoSaveDone     chan struct{}
	autoSaveMu       sync.Mutex
}

// NewCheckpointManager 创建检查点管理器
func NewCheckpointManager(store CheckpointStore, logger *zap.Logger) *CheckpointManager {
	return &CheckpointManager{
		store:           store,
		logger:          logger.With(zap.String("component", "checkpoint_manager")),
		autoSaveEnabled: false,
	}
}

// SaveCheckpoint 保存检查点
func (m *CheckpointManager) SaveCheckpoint(ctx context.Context, checkpoint *Checkpoint) error {
	if checkpoint.ID == "" {
		checkpoint.ID = generateCheckpointID()
	}
	if checkpoint.CreatedAt.IsZero() {
		checkpoint.CreatedAt = time.Now()
	}
	checkpoint.normalizeLoopPersistenceFields()

	m.logger.Debug("saving checkpoint",
		zap.String("checkpoint_id", checkpoint.ID),
		zap.String("thread_id", checkpoint.ThreadID),
		zap.String("agent_id", checkpoint.AgentID),
	)

	if err := m.store.Save(ctx, checkpoint); err != nil {
		return fmt.Errorf("failed to save checkpoint: %w", err)
	}

	return nil
}

// LoadCheckpoint 加载检查点
func (m *CheckpointManager) LoadCheckpoint(ctx context.Context, checkpointID string) (*Checkpoint, error) {
	m.logger.Debug("loading checkpoint", zap.String("checkpoint_id", checkpointID))

	checkpoint, err := m.store.Load(ctx, checkpointID)
	if err != nil {
		return nil, fmt.Errorf("failed to load checkpoint: %w", err)
	}
	checkpoint.normalizeLoopPersistenceFields()

	return checkpoint, nil
}

// LoadLatestCheckpoint 加载最新检查点
func (m *CheckpointManager) LoadLatestCheckpoint(ctx context.Context, threadID string) (*Checkpoint, error) {
	m.logger.Debug("loading latest checkpoint", zap.String("thread_id", threadID))

	checkpoint, err := m.store.LoadLatest(ctx, threadID)
	if err != nil {
		return nil, fmt.Errorf("failed to load latest checkpoint: %w", err)
	}
	checkpoint.normalizeLoopPersistenceFields()

	return checkpoint, nil
}

// ResumeFromCheckpoint 从检查点恢复执行
func (m *CheckpointManager) ResumeFromCheckpoint(ctx context.Context, agent Agent, checkpointID string) error {
	_, err := m.LoadCheckpointForAgent(ctx, agent, checkpointID)
	return err
}

// LoadCheckpointForAgent loads a checkpoint, validates ownership, and restores the agent state.
func (m *CheckpointManager) LoadCheckpointForAgent(ctx context.Context, agent Agent, checkpointID string) (*Checkpoint, error) {
	checkpoint, err := m.LoadCheckpoint(ctx, checkpointID)
	if err != nil {
		return nil, err
	}
	if err := m.restoreAgentFromCheckpoint(ctx, agent, checkpoint); err != nil {
		return nil, err
	}
	return checkpoint, nil
}

// LoadLatestCheckpointForAgent loads the latest checkpoint for the thread and restores the agent state.
func (m *CheckpointManager) LoadLatestCheckpointForAgent(ctx context.Context, agent Agent, threadID string) (*Checkpoint, error) {
	checkpoint, err := m.LoadLatestCheckpoint(ctx, threadID)
	if err != nil {
		return nil, err
	}
	if err := m.restoreAgentFromCheckpoint(ctx, agent, checkpoint); err != nil {
		return nil, err
	}
	return checkpoint, nil
}

func (m *CheckpointManager) restoreAgentFromCheckpoint(ctx context.Context, agent Agent, checkpoint *Checkpoint) error {
	if checkpoint == nil {
		return fmt.Errorf("checkpoint is nil")
	}
	m.logger.Info("resuming from checkpoint",
		zap.String("checkpoint_id", checkpoint.ID),
		zap.String("agent_id", checkpoint.AgentID),
		zap.String("state", string(checkpoint.State)),
	)

	if agent.ID() != checkpoint.AgentID {
		return fmt.Errorf("agent ID mismatch: expected %s, got %s", checkpoint.AgentID, agent.ID())
	}

	type transitioner interface {
		Transition(ctx context.Context, newState State) error
	}
	if t, ok := agent.(transitioner); ok {
		if err := t.Transition(ctx, checkpoint.State); err != nil {
			return fmt.Errorf("failed to restore state: %w", err)
		}
	}

	m.logger.Info("checkpoint restored successfully")
	return nil
}

// 启用自动保存以指定间隔自动保存检查点
func (m *CheckpointManager) EnableAutoSave(ctx context.Context, agent Agent, threadID string, interval time.Duration) error {
	m.autoSaveMu.Lock()
	defer m.autoSaveMu.Unlock()

	if m.autoSaveEnabled {
		return fmt.Errorf("auto-save already enabled")
	}

	if interval <= 0 {
		return fmt.Errorf("invalid interval: must be positive")
	}

	m.autoSaveInterval = interval
	m.autoSaveEnabled = true

	autoSaveCtx, cancel := context.WithCancel(ctx)
	m.autoSaveCancel = cancel
	m.autoSaveDone = make(chan struct{})

	go m.autoSaveLoop(autoSaveCtx, m.autoSaveDone, agent, threadID)

	m.logger.Info("auto-save enabled",
		zap.Duration("interval", interval),
		zap.String("thread_id", threadID),
	)

	return nil
}

// 禁用自动保存停止自动检查
func (m *CheckpointManager) DisableAutoSave() {
	m.autoSaveMu.Lock()
	if !m.autoSaveEnabled {
		m.autoSaveMu.Unlock()
		return
	}

	cancel := m.autoSaveCancel
	done := m.autoSaveDone
	m.autoSaveCancel = nil
	m.autoSaveDone = nil
	m.autoSaveEnabled = false
	m.autoSaveMu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}

	m.logger.Info("auto-save disabled")
}

// 自动保存环路运行自动检查点保存环路
func (m *CheckpointManager) autoSaveLoop(ctx context.Context, done chan struct{}, agent Agent, threadID string) {
	ticker := time.NewTicker(m.autoSaveInterval)
	defer func() {
		ticker.Stop()
		close(done)
	}()

	for {
		select {
		case <-ctx.Done():
			m.logger.Debug("auto-save loop stopped")
			return
		case <-ticker.C:
			if err := m.CreateCheckpoint(ctx, agent, threadID); err != nil {
				m.logger.Error("auto-save failed", zap.Error(err))
			} else {
				m.logger.Debug("auto-save checkpoint created", zap.String("thread_id", threadID))
			}
		}
	}
}

// 创建检查点来抓取当前代理状态并将其保存为检查点
func (m *CheckpointManager) CreateCheckpoint(ctx context.Context, agent Agent, threadID string) error {
	m.logger.Debug("creating checkpoint",
		zap.String("agent_id", agent.ID()),
		zap.String("thread_id", threadID),
	)

	state := agent.State()
	messages := []CheckpointMessage{}
	var executionContext *ExecutionContext

	checkpoint := &Checkpoint{
		ID:               generateCheckpointID(),
		ThreadID:         threadID,
		AgentID:          agent.ID(),
		State:            state,
		Messages:         messages,
		Metadata:         make(map[string]any),
		CreatedAt:        time.Now(),
		ExecutionContext: executionContext,
	}

	if err := m.SaveCheckpoint(ctx, checkpoint); err != nil {
		return fmt.Errorf("failed to create checkpoint: %w", err)
	}

	m.logger.Info("checkpoint created",
		zap.String("checkpoint_id", checkpoint.ID),
		zap.String("thread_id", threadID),
		zap.Int("version", checkpoint.Version),
	)

	return nil
}

// Rollback ToVersion 将代理拖回特定检查点版本
func (m *CheckpointManager) RollbackToVersion(ctx context.Context, agent Agent, threadID string, version int) error {
	m.logger.Info("rolling back to version",
		zap.String("thread_id", threadID),
		zap.Int("version", version),
	)

	checkpoint, err := m.store.LoadVersion(ctx, threadID, version)
	if err != nil {
		return fmt.Errorf("failed to load version %d: %w", version, err)
	}

	if agent.ID() != checkpoint.AgentID {
		return fmt.Errorf("agent ID mismatch: expected %s, got %s", checkpoint.AgentID, agent.ID())
	}

	type transitioner interface {
		Transition(ctx context.Context, newState State) error
	}

	if t, ok := agent.(transitioner); ok {
		if err := t.Transition(ctx, checkpoint.State); err != nil {
			return fmt.Errorf("failed to restore state: %w", err)
		}
	} else {
		m.logger.Warn("agent does not support Transition method, state restoration skipped",
			zap.String("agent_id", agent.ID()),
		)
	}

	if err := m.store.Rollback(ctx, threadID, version); err != nil {
		return fmt.Errorf("failed to rollback in store: %w", err)
	}

	m.logger.Info("rollback completed",
		zap.String("thread_id", threadID),
		zap.Int("version", version),
	)

	return nil
}

// 比较Version 比较两个检查点版本并返回差异
func (m *CheckpointManager) CompareVersions(ctx context.Context, threadID string, version1, version2 int) (*CheckpointDiff, error) {
	m.logger.Debug("comparing versions",
		zap.String("thread_id", threadID),
		zap.Int("version1", version1),
		zap.Int("version2", version2),
	)

	cp1, err := m.store.LoadVersion(ctx, threadID, version1)
	if err != nil {
		return nil, fmt.Errorf("failed to load version %d: %w", version1, err)
	}

	cp2, err := m.store.LoadVersion(ctx, threadID, version2)
	if err != nil {
		return nil, fmt.Errorf("failed to load version %d: %w", version2, err)
	}

	diff := &CheckpointDiff{
		ThreadID:     threadID,
		Version1:     version1,
		Version2:     version2,
		StateChanged: cp1.State != cp2.State,
		OldState:     cp1.State,
		NewState:     cp2.State,
		MessagesDiff: m.compareMessages(cp1.Messages, cp2.Messages),
		MetadataDiff: m.compareMetadata(cp1.Metadata, cp2.Metadata),
		TimeDiff:     cp2.CreatedAt.Sub(cp1.CreatedAt),
	}

	return diff, nil
}

// ListVersion 列出用于线索的所有检查点版本
func (m *CheckpointManager) ListVersions(ctx context.Context, threadID string) ([]CheckpointVersion, error) {
	m.logger.Debug("listing versions", zap.String("thread_id", threadID))

	versions, err := m.store.ListVersions(ctx, threadID)
	if err != nil {
		return nil, fmt.Errorf("failed to list versions: %w", err)
	}

	return versions, nil
}

// 比较Messages 比较两个消息切片并返回摘要
func (m *CheckpointManager) compareMessages(msgs1, msgs2 []CheckpointMessage) string {
	if len(msgs1) == len(msgs2) {
		return fmt.Sprintf("No change (%d messages)", len(msgs1))
	}
	return fmt.Sprintf("Changed from %d to %d messages", len(msgs1), len(msgs2))
}

// 比较Metadata 比较两个元数据地图并返回一个摘要
func (m *CheckpointManager) compareMetadata(meta1, meta2 map[string]any) string {
	added := 0
	removed := 0
	changed := 0

	for k, v2 := range meta2 {
		if v1, exists := meta1[k]; !exists {
			added++
		} else if fmt.Sprintf("%v", v1) != fmt.Sprintf("%v", v2) {
			changed++
		}
	}

	for k := range meta1 {
		if _, exists := meta2[k]; !exists {
			removed++
		}
	}

	if added == 0 && removed == 0 && changed == 0 {
		return "No changes"
	}

	return fmt.Sprintf("Added: %d, Removed: %d, Changed: %d", added, removed, changed)
}

// CheckpointDiff 代表两个检查点版本之间的差异
type CheckpointDiff struct {
	ThreadID     string        `json:"thread_id"`
	Version1     int           `json:"version1"`
	Version2     int           `json:"version2"`
	StateChanged bool          `json:"state_changed"`
	OldState     State         `json:"old_state"`
	NewState     State         `json:"new_state"`
	MessagesDiff string        `json:"messages_diff"`
	MetadataDiff string        `json:"metadata_diff"`
	TimeDiff     time.Duration `json:"time_diff"`
}
