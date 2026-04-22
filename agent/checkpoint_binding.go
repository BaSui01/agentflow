package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	checkpointcore "github.com/BaSui01/agentflow/agent/persistence/checkpoint/core"
	"go.uber.org/zap"
)

func (s *LoopState) CheckpointVariables() map[string]any {
	if s == nil {
		return nil
	}
	s.normalizeCheckpointFields()
	data := loopStateCheckpointCore(s)
	variables := data.Variables()
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
	if observations, ok := loopContextObservations(values, "loop_observations", "observations"); ok {
		data := loopStateCheckpointCore(s)
		data.Observations = checkpointCoreObservations(observations)
		data.RestoreFromContext(values, func() string {
			s.SyncCurrentStep()
			return s.CurrentStepID
		})
		applyLoopStateCheckpointCore(s, data)
		return
	}
	data := loopStateCheckpointCore(s)
	data.RestoreFromContext(values, func() string {
		s.SyncCurrentStep()
		return s.CurrentStepID
	})
	applyLoopStateCheckpointCore(s, data)
}

func (s *LoopState) normalizeCheckpointFields() {
	if s == nil {
		return
	}
	hadPlanSlice := s.Plan != nil
	hadObservationsSlice := s.Observations != nil
	data := loopStateCheckpointCore(s)
	data.Normalize(func() string {
		s.SyncCurrentStep()
		return s.CurrentStepID
	})
	applyLoopStateCheckpointCore(s, data)
	if hadPlanSlice && s.Plan == nil {
		s.Plan = []string{}
	}
	if hadObservationsSlice && s.Observations == nil {
		s.Observations = []LoopObservation{}
	}
}

func (s *LoopState) ApplyValidationResult(result *LoopValidationResult) {
	if s == nil || result == nil {
		return
	}
	data := loopStateCheckpointCore(s)
	data.ApplyValidationResult(checkpointcore.ValidationResult{
		AcceptanceCriteria: result.AcceptanceCriteria,
		UnresolvedItems:    result.UnresolvedItems,
		RemainingRisks:     result.RemainingRisks,
		Status:             string(result.Status),
		Summary:            result.Summary,
		Reason:             result.Reason,
	}, func() string {
		s.SyncCurrentStep()
		return s.CurrentStepID
	})
	applyLoopStateCheckpointCore(s, data)
}

func buildLoopPlanID(loopStateID string, planVersion int) string {
	return checkpointcore.BuildLoopPlanID(loopStateID, planVersion)
}

func derivePlanVersion(observations []LoopObservation) int {
	return checkpointcore.DerivePlanVersion(checkpointCoreObservations(observations))
}

func summarizeObservations(observations []LoopObservation) string {
	return checkpointcore.SummarizeObservations(checkpointCoreObservations(observations))
}

func summarizeLastOutput(output *Output, observations []LoopObservation) string {
	lastOutputContent := ""
	if output != nil {
		lastOutputContent = output.Content
	}
	return checkpointcore.SummarizeLastOutput(lastOutputContent, checkpointCoreObservations(observations))
}

func summarizeLastError(observations []LoopObservation) string {
	return checkpointcore.SummarizeLastError(checkpointCoreObservations(observations))
}

func summarizeValidationState(status LoopValidationStatus, unresolvedItems, remainingRisks []string) string {
	return checkpointcore.SummarizeValidationState(string(status), unresolvedItems, remainingRisks)
}

func summarizeText(text string) string {
	return checkpointcore.SummarizeText(text)
}

func loopContextString(values map[string]any, keys ...string) (string, bool) {
	return checkpointcore.ContextString(values, keys...)
}

func loopContextStrings(values map[string]any, keys ...string) ([]string, bool) {
	return checkpointcore.ContextStrings(values, keys...)
}

func loopContextInt(values map[string]any, keys ...string) (int, bool) {
	return checkpointcore.ContextInt(values, keys...)
}

func loopContextFloat(values map[string]any, keys ...string) (float64, bool) {
	return checkpointcore.ContextFloat(values, keys...)
}

func loopContextBool(values map[string]any, keys ...string) (value, ok bool) {
	return checkpointcore.ContextBool(values, keys...)
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
	return checkpointcore.CloneStringSlice(values)
}

func normalizeStringSlice(values []string) []string {
	return checkpointcore.NormalizeStringSlice(values)
}

func loopStateCheckpointCore(state *LoopState) checkpointcore.LoopStateData {
	lastOutputContent := ""
	if state != nil && state.LastOutput != nil {
		lastOutputContent = state.LastOutput.Content
	}
	return checkpointcore.LoopStateData{
		LoopStateID:           state.LoopStateID,
		RunID:                 state.RunID,
		AgentID:               state.AgentID,
		Goal:                  state.Goal,
		Plan:                  append([]string(nil), state.Plan...),
		AcceptanceCriteria:    cloneStringSlice(state.AcceptanceCriteria),
		UnresolvedItems:       cloneStringSlice(state.UnresolvedItems),
		RemainingRisks:        cloneStringSlice(state.RemainingRisks),
		CurrentPlanID:         state.CurrentPlanID,
		PlanVersion:           state.PlanVersion,
		CurrentStepID:         state.CurrentStepID,
		CurrentStage:          string(state.CurrentStage),
		Iteration:             state.Iteration,
		MaxIterations:         state.MaxIterations,
		Decision:              string(state.Decision),
		StopReason:            string(state.StopReason),
		SelectedReasoningMode: state.SelectedReasoningMode,
		Confidence:            state.Confidence,
		NeedHuman:             state.NeedHuman,
		CheckpointID:          state.CheckpointID,
		Resumable:             state.Resumable,
		ValidationStatus:      string(state.ValidationStatus),
		ValidationSummary:     state.ValidationSummary,
		ObservationsSummary:   state.ObservationsSummary,
		LastOutputSummary:     state.LastOutputSummary,
		LastError:             state.LastError,
		Observations:          checkpointCoreObservations(state.Observations),
		LastOutputContent:     lastOutputContent,
	}
}

func applyLoopStateCheckpointCore(state *LoopState, data checkpointcore.LoopStateData) {
	state.LoopStateID = data.LoopStateID
	state.RunID = data.RunID
	state.AgentID = data.AgentID
	state.Goal = data.Goal
	state.Plan = append([]string(nil), data.Plan...)
	state.AcceptanceCriteria = cloneStringSlice(data.AcceptanceCriteria)
	state.UnresolvedItems = cloneStringSlice(data.UnresolvedItems)
	state.RemainingRisks = cloneStringSlice(data.RemainingRisks)
	state.CurrentPlanID = data.CurrentPlanID
	state.PlanVersion = data.PlanVersion
	state.CurrentStepID = data.CurrentStepID
	state.CurrentStage = LoopStage(data.CurrentStage)
	state.Iteration = data.Iteration
	state.MaxIterations = data.MaxIterations
	state.Decision = LoopDecision(data.Decision)
	state.StopReason = StopReason(data.StopReason)
	state.SelectedReasoningMode = data.SelectedReasoningMode
	state.Confidence = data.Confidence
	state.NeedHuman = data.NeedHuman
	state.CheckpointID = data.CheckpointID
	state.Resumable = data.Resumable
	state.ValidationStatus = LoopValidationStatus(data.ValidationStatus)
	state.ValidationSummary = data.ValidationSummary
	state.ObservationsSummary = data.ObservationsSummary
	state.LastOutputSummary = data.LastOutputSummary
	state.LastError = data.LastError
	state.Observations = loopObservationsFromCore(data.Observations)
}

func checkpointCoreObservations(observations []LoopObservation) []checkpointcore.Observation {
	if len(observations) == 0 {
		return nil
	}
	converted := make([]checkpointcore.Observation, 0, len(observations))
	for _, observation := range observations {
		converted = append(converted, checkpointcore.Observation{
			Stage:     string(observation.Stage),
			Content:   observation.Content,
			Error:     observation.Error,
			Metadata:  cloneMetadata(observation.Metadata),
			Iteration: observation.Iteration,
		})
	}
	return converted
}

func loopObservationsFromCore(observations []checkpointcore.Observation) []LoopObservation {
	if len(observations) == 0 {
		return nil
	}
	converted := make([]LoopObservation, 0, len(observations))
	for _, observation := range observations {
		converted = append(converted, LoopObservation{
			Stage:     LoopStage(observation.Stage),
			Content:   observation.Content,
			Error:     observation.Error,
			Metadata:  cloneMetadata(observation.Metadata),
			Iteration: observation.Iteration,
		})
	}
	return converted
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
	if logger == nil {
		logger = zap.NewNop()
	}
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
	return checkpointcore.CompareMessageCounts(len(msgs1), len(msgs2))
}

// 比较Metadata 比较两个元数据地图并返回一个摘要
func (m *CheckpointManager) compareMetadata(meta1, meta2 map[string]any) string {
	return checkpointcore.CompareMetadata(meta1, meta2)
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

// Checkpoint Agent 执行检查点（基于 LangGraph 2026 标准）
type Checkpoint struct {
	ID                  string               `json:"id"`
	ThreadID            string               `json:"thread_id"` // 会话线程 ID
	AgentID             string               `json:"agent_id"`
	LoopStateID         string               `json:"loop_state_id,omitempty"`
	RunID               string               `json:"run_id,omitempty"`
	Goal                string               `json:"goal,omitempty"`
	AcceptanceCriteria  []string             `json:"acceptance_criteria,omitempty"`
	UnresolvedItems     []string             `json:"unresolved_items,omitempty"`
	RemainingRisks      []string             `json:"remaining_risks,omitempty"`
	CurrentPlanID       string               `json:"current_plan_id,omitempty"`
	PlanVersion         int                  `json:"plan_version,omitempty"`
	CurrentStepID       string               `json:"current_step_id,omitempty"`
	ValidationStatus    LoopValidationStatus `json:"validation_status,omitempty"`
	ValidationSummary   string               `json:"validation_summary,omitempty"`
	ObservationsSummary string               `json:"observations_summary,omitempty"`
	LastOutputSummary   string               `json:"last_output_summary,omitempty"`
	LastError           string               `json:"last_error,omitempty"`
	Version             int                  `json:"version"` // 版本号（线程内递增）
	State               State                `json:"state"`
	Messages            []CheckpointMessage  `json:"messages"`
	Metadata            map[string]any       `json:"metadata"`
	CreatedAt           time.Time            `json:"created_at"`
	ParentID            string               `json:"parent_id,omitempty"` // 父检查点 ID

	// ExecutionContext 工作流执行上下文
	ExecutionContext *ExecutionContext `json:"execution_context,omitempty"`
}

// CheckpointMessage 检查点消息
type CheckpointMessage struct {
	Role      string               `json:"role"`
	Content   string               `json:"content"`
	ToolCalls []CheckpointToolCall `json:"tool_calls,omitempty"`
	Metadata  map[string]any       `json:"metadata,omitempty"`
}

// CheckpointToolCall 工具调用记录
type CheckpointToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     string          `json:"error,omitempty"`
}

// ExecutionContext 工作流执行上下文
type ExecutionContext struct {
	WorkflowID          string               `json:"workflow_id,omitempty"`
	CurrentNode         string               `json:"current_node,omitempty"`
	NodeResults         map[string]any       `json:"node_results,omitempty"`
	Variables           map[string]any       `json:"variables,omitempty"`
	LoopStateID         string               `json:"loop_state_id,omitempty"`
	RunID               string               `json:"run_id,omitempty"`
	AgentID             string               `json:"agent_id,omitempty"`
	Goal                string               `json:"goal,omitempty"`
	AcceptanceCriteria  []string             `json:"acceptance_criteria,omitempty"`
	UnresolvedItems     []string             `json:"unresolved_items,omitempty"`
	RemainingRisks      []string             `json:"remaining_risks,omitempty"`
	CurrentPlanID       string               `json:"current_plan_id,omitempty"`
	PlanVersion         int                  `json:"plan_version,omitempty"`
	CurrentStepID       string               `json:"current_step_id,omitempty"`
	ValidationStatus    LoopValidationStatus `json:"validation_status,omitempty"`
	ValidationSummary   string               `json:"validation_summary,omitempty"`
	ObservationsSummary string               `json:"observations_summary,omitempty"`
	LastOutputSummary   string               `json:"last_output_summary,omitempty"`
	LastError           string               `json:"last_error,omitempty"`
}

func (c *Checkpoint) LoopContextValues() map[string]any {
	if c == nil {
		return nil
	}
	c.normalizeLoopPersistenceFields()
	data := checkpointPersistenceCore(c)
	return data.LoopContextValues()
}

func (c *ExecutionContext) LoopContextValues() map[string]any {
	if c == nil {
		return nil
	}
	return executionContextPersistenceCore(c).LoopContextValues()
}

func (c *Checkpoint) normalizeLoopPersistenceFields() {
	if c == nil {
		return
	}
	data := checkpointPersistenceCore(c)
	data.Normalize()
	applyCheckpointPersistenceCore(c, data)
}

func checkpointPersistenceCore(checkpoint *Checkpoint) checkpointcore.CheckpointData {
	return checkpointcore.CheckpointData{
		LoopStateID:         checkpoint.LoopStateID,
		RunID:               checkpoint.RunID,
		AgentID:             checkpoint.AgentID,
		Goal:                checkpoint.Goal,
		AcceptanceCriteria:  cloneStringSlice(checkpoint.AcceptanceCriteria),
		UnresolvedItems:     cloneStringSlice(checkpoint.UnresolvedItems),
		RemainingRisks:      cloneStringSlice(checkpoint.RemainingRisks),
		CurrentPlanID:       checkpoint.CurrentPlanID,
		PlanVersion:         checkpoint.PlanVersion,
		CurrentStepID:       checkpoint.CurrentStepID,
		ValidationStatus:    string(checkpoint.ValidationStatus),
		ValidationSummary:   checkpoint.ValidationSummary,
		ObservationsSummary: checkpoint.ObservationsSummary,
		LastOutputSummary:   checkpoint.LastOutputSummary,
		LastError:           checkpoint.LastError,
		Metadata:            cloneMetadata(checkpoint.Metadata),
		ExecutionContext:    executionContextPersistenceCore(checkpoint.ExecutionContext),
	}
}

func executionContextPersistenceCore(ctx *ExecutionContext) *checkpointcore.ExecutionContextData {
	if ctx == nil {
		return nil
	}
	return &checkpointcore.ExecutionContextData{
		CurrentNode:         ctx.CurrentNode,
		Variables:           cloneMetadata(ctx.Variables),
		LoopStateID:         ctx.LoopStateID,
		RunID:               ctx.RunID,
		AgentID:             ctx.AgentID,
		Goal:                ctx.Goal,
		AcceptanceCriteria:  cloneStringSlice(ctx.AcceptanceCriteria),
		UnresolvedItems:     cloneStringSlice(ctx.UnresolvedItems),
		RemainingRisks:      cloneStringSlice(ctx.RemainingRisks),
		CurrentPlanID:       ctx.CurrentPlanID,
		PlanVersion:         ctx.PlanVersion,
		CurrentStepID:       ctx.CurrentStepID,
		ValidationStatus:    string(ctx.ValidationStatus),
		ValidationSummary:   ctx.ValidationSummary,
		ObservationsSummary: ctx.ObservationsSummary,
		LastOutputSummary:   ctx.LastOutputSummary,
		LastError:           ctx.LastError,
	}
}

func applyCheckpointPersistenceCore(checkpoint *Checkpoint, data checkpointcore.CheckpointData) {
	checkpoint.LoopStateID = data.LoopStateID
	checkpoint.RunID = data.RunID
	checkpoint.AgentID = data.AgentID
	checkpoint.Goal = data.Goal
	checkpoint.AcceptanceCriteria = cloneStringSlice(data.AcceptanceCriteria)
	checkpoint.UnresolvedItems = cloneStringSlice(data.UnresolvedItems)
	checkpoint.RemainingRisks = cloneStringSlice(data.RemainingRisks)
	checkpoint.CurrentPlanID = data.CurrentPlanID
	checkpoint.PlanVersion = data.PlanVersion
	checkpoint.CurrentStepID = data.CurrentStepID
	checkpoint.ValidationStatus = LoopValidationStatus(data.ValidationStatus)
	checkpoint.ValidationSummary = data.ValidationSummary
	checkpoint.ObservationsSummary = data.ObservationsSummary
	checkpoint.LastOutputSummary = data.LastOutputSummary
	checkpoint.LastError = data.LastError
	checkpoint.Metadata = data.Metadata
	if data.ExecutionContext == nil {
		checkpoint.ExecutionContext = nil
		return
	}
	if checkpoint.ExecutionContext == nil {
		checkpoint.ExecutionContext = &ExecutionContext{}
	}
	checkpoint.ExecutionContext.CurrentNode = data.ExecutionContext.CurrentNode
	checkpoint.ExecutionContext.Variables = data.ExecutionContext.Variables
	checkpoint.ExecutionContext.LoopStateID = data.ExecutionContext.LoopStateID
	checkpoint.ExecutionContext.RunID = data.ExecutionContext.RunID
	checkpoint.ExecutionContext.AgentID = data.ExecutionContext.AgentID
	checkpoint.ExecutionContext.Goal = data.ExecutionContext.Goal
	checkpoint.ExecutionContext.AcceptanceCriteria = cloneStringSlice(data.ExecutionContext.AcceptanceCriteria)
	checkpoint.ExecutionContext.UnresolvedItems = cloneStringSlice(data.ExecutionContext.UnresolvedItems)
	checkpoint.ExecutionContext.RemainingRisks = cloneStringSlice(data.ExecutionContext.RemainingRisks)
	checkpoint.ExecutionContext.CurrentPlanID = data.ExecutionContext.CurrentPlanID
	checkpoint.ExecutionContext.PlanVersion = data.ExecutionContext.PlanVersion
	checkpoint.ExecutionContext.CurrentStepID = data.ExecutionContext.CurrentStepID
	checkpoint.ExecutionContext.ValidationStatus = LoopValidationStatus(data.ExecutionContext.ValidationStatus)
	checkpoint.ExecutionContext.ValidationSummary = data.ExecutionContext.ValidationSummary
	checkpoint.ExecutionContext.ObservationsSummary = data.ExecutionContext.ObservationsSummary
	checkpoint.ExecutionContext.LastOutputSummary = data.ExecutionContext.LastOutputSummary
	checkpoint.ExecutionContext.LastError = data.ExecutionContext.LastError
}

// CheckpointVersion 检查点版本元数据
type CheckpointVersion struct {
	Version   int       `json:"version"`
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	State     State     `json:"state"`
	Summary   string    `json:"summary"`
}

// CheckpointStore 检查点存储接口（Agent 层）。
//
// 注意：项目中存在两个 CheckpointStore 接口，操作不同的检查点类型：
//   - agent.CheckpointStore（本接口）   — 操作 *agent.Checkpoint，含 List/DeleteThread/Rollback
//   - workflow.CheckpointStore          — 操作 *workflow.EnhancedCheckpoint，用于 DAG 工作流时间旅行
//
// 两者的检查点结构体字段不同（Agent 状态 vs DAG 节点结果），无法统一。
type CheckpointStore interface {
	// Save 保存检查点
	Save(ctx context.Context, checkpoint *Checkpoint) error

	// Load 加载检查点
	Load(ctx context.Context, checkpointID string) (*Checkpoint, error)

	// LoadLatest 加载线程最新检查点
	LoadLatest(ctx context.Context, threadID string) (*Checkpoint, error)

	// List 列出线程的所有检查点
	List(ctx context.Context, threadID string, limit int) ([]*Checkpoint, error)

	// Delete 删除检查点
	Delete(ctx context.Context, checkpointID string) error

	// DeleteThread 删除整个线程
	DeleteThread(ctx context.Context, threadID string) error

	// LoadVersion 加载指定版本的检查点
	LoadVersion(ctx context.Context, threadID string, version int) (*Checkpoint, error)

	// ListVersions 列出线程的所有版本
	ListVersions(ctx context.Context, threadID string) ([]CheckpointVersion, error)

	// Rollback 回滚到指定版本
	Rollback(ctx context.Context, threadID string, version int) error
}

var checkpointIDCounter uint64

// GenerateCheckpointID exposes checkpoint ID generation to checkpoint persistence subpackages.
func GenerateCheckpointID() string {
	return generateCheckpointID()
}

// generateCheckpointID 生成检查点 ID
func generateCheckpointID() string {
	return checkpointcore.NextCheckpointID(&checkpointIDCounter)
}
