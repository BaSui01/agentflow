package runtime

import (
	"context"
	"fmt"
	memorycore "github.com/BaSui01/agentflow/agent/capabilities/memory"
	promptcap "github.com/BaSui01/agentflow/agent/capabilities/prompt"
	agentfeatures "github.com/BaSui01/agentflow/agent/integration"
	agentcheckpoint "github.com/BaSui01/agentflow/agent/persistence/checkpoint"
	checkpointcore "github.com/BaSui01/agentflow/agent/persistence/checkpoint/core"
	"go.uber.org/zap"
	"sync"
	"time"
)

type MemoryRecallOptions = memorycore.MemoryRecallOptions

type MemoryObservationInput = memorycore.MemoryObservationInput

type MemoryRuntime = memorycore.MemoryRuntime

type UnifiedMemoryFacade = memorycore.UnifiedMemoryFacade

type PromptBundle = promptcap.PromptBundle

type SystemPrompt = promptcap.SystemPrompt

type Example = promptcap.Example

type MemoryConfig = promptcap.MemoryConfig

type PlanConfig = promptcap.PlanConfig

type ReflectionConfig = promptcap.ReflectionConfig

type PromptEnhancerConfig = promptcap.PromptEnhancerConfig

type PromptEnhancer = promptcap.PromptEnhancer

type PromptTemplateLibrary = promptcap.PromptTemplateLibrary

type PromptTemplate = promptcap.PromptTemplate

type DefensivePromptConfig = promptcap.DefensivePromptConfig

type FailureMode = promptcap.FailureMode

type OutputSchema = promptcap.OutputSchema

type GuardRail = promptcap.GuardRail

type InjectionDefenseConfig = promptcap.InjectionDefenseConfig

type DefensivePromptEnhancer = promptcap.DefensivePromptEnhancer

type EnhancedExecutionOptions = agentfeatures.EnhancedExecutionOptions

type CheckpointDiff = agentcheckpoint.CheckpointDiff

type Checkpoint = agentcheckpoint.Checkpoint

type CheckpointMessage = agentcheckpoint.CheckpointMessage

type CheckpointToolCall = agentcheckpoint.CheckpointToolCall

type ExecutionContext = agentcheckpoint.ExecutionContext

type CheckpointVersion = agentcheckpoint.CheckpointVersion

type CheckpointStore = agentcheckpoint.Store

type CheckpointManager struct {
	inner  *agentcheckpoint.Manager
	store  CheckpointStore
	logger *zap.Logger

	autoSaveEnabled  bool
	autoSaveInterval time.Duration
	autoSaveCancel   context.CancelFunc
	autoSaveDone     chan struct{}
	autoSaveMu       sync.Mutex
}

func NewPromptBundleFromIdentity(version, identity string) PromptBundle {
	return promptcap.NewPromptBundleFromIdentity(version, identity)
}

func DefaultPromptEnhancerConfig() *PromptEnhancerConfig {
	return promptcap.DefaultPromptEnhancerConfig()
}

func NewPromptEnhancer(config PromptEnhancerConfig) *PromptEnhancer {
	return promptcap.NewPromptEnhancer(config)
}

type PromptOptimizer struct{}

func NewPromptOptimizer() *PromptOptimizer {
	return &PromptOptimizer{}
}

func NewPromptTemplateLibrary() *PromptTemplateLibrary {
	return promptcap.NewPromptTemplateLibrary()
}

func DefaultDefensivePromptConfig() DefensivePromptConfig {
	return promptcap.DefaultDefensivePromptConfig()
}

func DefaultEnhancedExecutionOptions() EnhancedExecutionOptions {
	return agentfeatures.DefaultEnhancedExecutionOptions()
}

func NewDefensivePromptEnhancer(config DefensivePromptConfig) *DefensivePromptEnhancer {
	return promptcap.NewDefensivePromptEnhancer(config)
}

func NewCheckpointManager(store CheckpointStore, logger *zap.Logger) *CheckpointManager {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &CheckpointManager{
		inner:  agentcheckpoint.NewManager(store, logger),
		store:  store,
		logger: logger.With(zap.String("component", "checkpoint_manager")),
	}
}

func NewCheckpointManagerFromNativeStore(store agentcheckpoint.Store, logger *zap.Logger) *CheckpointManager {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &CheckpointManager{
		inner:  agentcheckpoint.NewManager(store, logger),
		logger: logger.With(zap.String("component", "checkpoint_manager")),
	}
}

func GenerateCheckpointID() string {
	return agentcheckpoint.GenerateID()
}

func generateCheckpointID() string {
	return GenerateCheckpointID()
}

func (m *CheckpointManager) SaveCheckpoint(ctx context.Context, checkpoint *Checkpoint) error {
	return m.ensureInner().SaveCheckpoint(ctx, checkpoint)
}

func (m *CheckpointManager) LoadCheckpoint(ctx context.Context, checkpointID string) (*Checkpoint, error) {
	return m.ensureInner().LoadCheckpoint(ctx, checkpointID)
}

func (m *CheckpointManager) LoadLatestCheckpoint(ctx context.Context, threadID string) (*Checkpoint, error) {
	return m.ensureInner().LoadLatestCheckpoint(ctx, threadID)
}

func (m *CheckpointManager) ResumeFromCheckpoint(ctx context.Context, agent Agent, checkpointID string) error {
	_, err := m.LoadCheckpointForAgent(ctx, agent, checkpointID)
	return err
}

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
	m.loggerOrNop().Info("resuming from checkpoint",
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
		if err := t.Transition(ctx, State(checkpoint.State)); err != nil {
			return fmt.Errorf("failed to restore state: %w", err)
		}
	}

	m.loggerOrNop().Info("checkpoint restored successfully")
	return nil
}

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

	m.loggerOrNop().Info("auto-save enabled",
		zap.Duration("interval", interval),
		zap.String("thread_id", threadID),
	)
	return nil
}

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

	m.loggerOrNop().Info("auto-save disabled")
}

func (m *CheckpointManager) autoSaveLoop(ctx context.Context, done chan struct{}, agent Agent, threadID string) {
	ticker := time.NewTicker(m.autoSaveInterval)
	defer func() {
		ticker.Stop()
		close(done)
	}()

	for {
		select {
		case <-ctx.Done():
			m.loggerOrNop().Debug("auto-save loop stopped")
			return
		case <-ticker.C:
			if err := m.CreateCheckpoint(ctx, agent, threadID); err != nil {
				m.loggerOrNop().Error("auto-save failed", zap.Error(err))
			} else {
				m.loggerOrNop().Debug("auto-save checkpoint created", zap.String("thread_id", threadID))
			}
		}
	}
}

func (m *CheckpointManager) CreateCheckpoint(ctx context.Context, agent Agent, threadID string) error {
	_, err := m.ensureInner().CreateCheckpoint(ctx, threadID, agent.ID(), string(agent.State()))
	return err
}

func (m *CheckpointManager) RollbackToVersion(ctx context.Context, agent Agent, threadID string, version int) error {
	m.loggerOrNop().Info("rolling back to version",
		zap.String("thread_id", threadID),
		zap.Int("version", version),
	)

	checkpoint, err := m.ensureInner().LoadVersion(ctx, threadID, version)
	if err != nil {
		return err
	}
	if err := m.restoreAgentFromCheckpoint(ctx, agent, checkpoint); err != nil {
		return err
	}
	if err := m.ensureInner().RollbackToVersion(ctx, threadID, version); err != nil {
		return err
	}

	m.loggerOrNop().Info("rollback completed",
		zap.String("thread_id", threadID),
		zap.Int("version", version),
	)
	return nil
}

func (m *CheckpointManager) CompareVersions(ctx context.Context, threadID string, version1, version2 int) (*CheckpointDiff, error) {
	return m.ensureInner().CompareVersions(ctx, threadID, version1, version2)
}

func (m *CheckpointManager) ListVersions(ctx context.Context, threadID string) ([]CheckpointVersion, error) {
	return m.ensureInner().ListVersions(ctx, threadID)
}

func (m *CheckpointManager) compareMessages(msgs1, msgs2 []CheckpointMessage) string {
	return checkpointcore.CompareMessageCounts(len(msgs1), len(msgs2))
}

func (m *CheckpointManager) compareMetadata(meta1, meta2 map[string]any) string {
	return checkpointcore.CompareMetadata(meta1, meta2)
}

func (m *CheckpointManager) ensureInner() *agentcheckpoint.Manager {
	if m.inner == nil {
		m.inner = agentcheckpoint.NewManager(m.store, m.loggerOrNop())
	}
	return m.inner
}

func (m *CheckpointManager) loggerOrNop() *zap.Logger {
	if m != nil && m.logger != nil {
		return m.logger
	}
	return zap.NewNop()
}

func (o *PromptOptimizer) OptimizePrompt(prompt string) string {
	optimized := prompt
	if len(prompt) < 20 {
		optimized = o.makeMoreSpecific(optimized)
	}
	if !o.hasTaskDescription(optimized) {
		optimized = o.addTaskDescription(optimized)
	}
	if !o.hasConstraints(optimized) {
		optimized = o.addBasicConstraints(optimized)
	}
	return optimized
}

func (o *PromptOptimizer) makeMoreSpecific(prompt string) string {
	return promptcap.MakeMoreSpecific(prompt)
}

func (o *PromptOptimizer) hasTaskDescription(prompt string) bool {
	return promptcap.HasTaskDescription(prompt)
}

func (o *PromptOptimizer) addTaskDescription(prompt string) string {
	return promptcap.AddTaskDescription(prompt)
}

func (o *PromptOptimizer) hasConstraints(prompt string) bool {
	return promptcap.HasConstraints(prompt)
}

func (o *PromptOptimizer) addBasicConstraints(prompt string) string {
	return fmt.Sprintf("%s\n\n要求：\n- 回答要准确、完整\n- 使用清晰的语言\n- 提供必要的解释", prompt)
}

func formatBulletSection(title string, items []string) string {
	return promptcap.FormatBulletSection(title, items)
}

func replaceTemplateVars(text string, vars map[string]string) string {
	return promptcap.ReplaceTemplateVars(text, vars)
}

func NewUnifiedMemoryFacade(base MemoryManager, enhanced EnhancedMemoryRunner, logger *zap.Logger) *UnifiedMemoryFacade {
	return memorycore.NewUnifiedMemoryFacade(base, enhanced, logger)
}

func NewDefaultMemoryRuntime(facadeProvider func() *UnifiedMemoryFacade, baseProvider func() MemoryManager, logger *zap.Logger) *memorycore.DefaultMemoryRuntime {
	return memorycore.NewDefaultMemoryRuntime(facadeProvider, baseProvider, logger)
}
