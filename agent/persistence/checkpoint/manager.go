package checkpoint

import (
	"context"
	"fmt"
	"time"

	checkpointcore "github.com/BaSui01/agentflow/agent/persistence/checkpoint/core"
	"go.uber.org/zap"
)

// Manager 管理检查点的持久化读写与版本比较。
type Manager struct {
	store  Store
	logger *zap.Logger
}

func NewManager(store Store, logger *zap.Logger) *Manager {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Manager{
		store:  store,
		logger: logger.With(zap.String("component", "checkpoint_manager")),
	}
}

func (m *Manager) SaveCheckpoint(ctx context.Context, checkpoint *Checkpoint) error {
	if checkpoint.ID == "" {
		checkpoint.ID = GenerateID()
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

func (m *Manager) LoadCheckpoint(ctx context.Context, checkpointID string) (*Checkpoint, error) {
	m.logger.Debug("loading checkpoint", zap.String("checkpoint_id", checkpointID))

	checkpoint, err := m.store.Load(ctx, checkpointID)
	if err != nil {
		return nil, fmt.Errorf("failed to load checkpoint: %w", err)
	}
	checkpoint.normalizeLoopPersistenceFields()
	return checkpoint, nil
}

func (m *Manager) LoadLatestCheckpoint(ctx context.Context, threadID string) (*Checkpoint, error) {
	m.logger.Debug("loading latest checkpoint", zap.String("thread_id", threadID))

	checkpoint, err := m.store.LoadLatest(ctx, threadID)
	if err != nil {
		return nil, fmt.Errorf("failed to load latest checkpoint: %w", err)
	}
	checkpoint.normalizeLoopPersistenceFields()
	return checkpoint, nil
}

func (m *Manager) LoadVersion(ctx context.Context, threadID string, version int) (*Checkpoint, error) {
	checkpoint, err := m.store.LoadVersion(ctx, threadID, version)
	if err != nil {
		return nil, fmt.Errorf("failed to load version %d: %w", version, err)
	}
	checkpoint.normalizeLoopPersistenceFields()
	return checkpoint, nil
}

func (m *Manager) CreateCheckpoint(ctx context.Context, threadID, agentID, state string) (*Checkpoint, error) {
	checkpoint := &Checkpoint{
		ID:               GenerateID(),
		ThreadID:         threadID,
		AgentID:          agentID,
		State:            state,
		Messages:         []CheckpointMessage{},
		Metadata:         make(map[string]any),
		CreatedAt:        time.Now(),
		ExecutionContext: nil,
	}

	if err := m.SaveCheckpoint(ctx, checkpoint); err != nil {
		return nil, fmt.Errorf("failed to create checkpoint: %w", err)
	}

	m.logger.Info("checkpoint created",
		zap.String("checkpoint_id", checkpoint.ID),
		zap.String("thread_id", threadID),
		zap.Int("version", checkpoint.Version),
	)
	return checkpoint, nil
}

func (m *Manager) RollbackToVersion(ctx context.Context, threadID string, version int) error {
	if err := m.store.Rollback(ctx, threadID, version); err != nil {
		return fmt.Errorf("failed to rollback in store: %w", err)
	}
	return nil
}

func (m *Manager) CompareVersions(ctx context.Context, threadID string, version1, version2 int) (*CheckpointDiff, error) {
	m.logger.Debug("comparing versions",
		zap.String("thread_id", threadID),
		zap.Int("version1", version1),
		zap.Int("version2", version2),
	)

	cp1, err := m.LoadVersion(ctx, threadID, version1)
	if err != nil {
		return nil, err
	}
	cp2, err := m.LoadVersion(ctx, threadID, version2)
	if err != nil {
		return nil, err
	}

	diff := &CheckpointDiff{
		ThreadID:     threadID,
		Version1:     version1,
		Version2:     version2,
		StateChanged: cp1.State != cp2.State,
		OldState:     cp1.State,
		NewState:     cp2.State,
		MessagesDiff: checkpointcore.CompareMessageCounts(len(cp1.Messages), len(cp2.Messages)),
		MetadataDiff: checkpointcore.CompareMetadata(cp1.Metadata, cp2.Metadata),
		TimeDiff:     cp2.CreatedAt.Sub(cp1.CreatedAt),
	}

	return diff, nil
}

func (m *Manager) ListVersions(ctx context.Context, threadID string) ([]CheckpointVersion, error) {
	m.logger.Debug("listing versions", zap.String("thread_id", threadID))

	versions, err := m.store.ListVersions(ctx, threadID)
	if err != nil {
		return nil, fmt.Errorf("failed to list versions: %w", err)
	}
	return versions, nil
}
